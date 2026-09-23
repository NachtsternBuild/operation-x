package tiles

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elias/operation-x/pkg/geo"
)

// Der Server darf kein Kachelvermittler für die ganze Welt werden.
//
// Über den Tunnel ist er für jeden erreichbar, der die Adresse kennt. Ohne
// Begrenzung könnte damit jemand fremde Gebiete über ihn abrufen – auf Kosten
// des Kachelservers, dessen Bedingungen genau das untersagen.
func TestNurKachelnImSpielgebiet(t *testing.T) {
	dresden := geo.Bounds{West: 13.70, South: 51.03, East: 13.80, North: 51.09}
	s := New(t.TempDir(), "", func() (geo.Bounds, bool) { return dresden, true })

	// Dresden, Zoomstufe 14: Die Kachel über der Altstadt muss durchgehen.
	zx, zy := latLngToTile(14, 51.05, 13.74)
	if !s.inArea(14, zx, zy) {
		t.Error("eine Kachel mitten im Spielgebiet wurde abgelehnt")
	}

	// Berlin liegt 160 Kilometer entfernt.
	bx, by := latLngToTile(14, 52.52, 13.40)
	if s.inArea(14, bx, by) {
		t.Error("eine Kachel aus einer anderen Stadt wurde durchgelassen")
	}

	// New York erst recht.
	nx, ny := latLngToTile(14, 40.71, -74.01)
	if s.inArea(14, nx, ny) {
		t.Error("eine Kachel von einem anderen Kontinent wurde durchgelassen")
	}
}

// Der Rand gehört dazu: Wer am Spielfeldrand steht, soll nicht auf eine
// abgeschnittene Karte sehen.
func TestRandUmDasSpielgebietIstErlaubt(t *testing.T) {
	klein := geo.Bounds{West: 13.74, South: 51.05, East: 13.75, North: 51.06}
	s := New(t.TempDir(), "", func() (geo.Bounds, bool) { return klein, true })

	// Gut einen Kilometer daneben – innerhalb des Randes von drei Kilometern.
	x, y := latLngToTile(14, 51.06, 13.76)
	if !s.inArea(14, x, y) {
		t.Error("die Kachel direkt neben dem Spielgebiet wurde abgelehnt")
	}
}

// Ohne eingerichtetes Gebiet muss die Karte trotzdem funktionieren – sonst
// ließe sich das Spielfeld gar nicht erst festlegen.
func TestOhneSpielgebietGehtAlles(t *testing.T) {
	s := New(t.TempDir(), "", func() (geo.Bounds, bool) { return geo.Bounds{}, false })
	x, y := latLngToTile(12, 48.14, 11.58) // München
	if !s.inArea(12, x, y) {
		t.Error("ohne Spielgebiet wurde eine Kachel abgelehnt")
	}
}

func TestZoomstufenSindBegrenzt(t *testing.T) {
	s := New(t.TempDir(), "", nil)

	if _, err := s.Tile(context.Background(), 3, 4, 4); err != ErrOutside {
		t.Errorf("Zoomstufe 3 wurde nicht abgelehnt: %v", err)
	}
	if _, err := s.Tile(context.Background(), 22, 4, 4); err != ErrOutside {
		t.Errorf("Zoomstufe 22 wurde nicht abgelehnt: %v", err)
	}
	// Unsinnige Koordinaten ebenso.
	if _, err := s.Tile(context.Background(), 12, -1, 0); err != ErrOutside {
		t.Errorf("negative Kachelnummer wurde nicht abgelehnt: %v", err)
	}
}

// Umkehrung von tileToLatLng, nur für die Tests.
func latLngToTile(z int, lat, lng float64) (int, int) {
	n := math.Pow(2, float64(z))
	x := int((lng + 180) / 360 * n)
	rad := lat * math.Pi / 180
	y := int((1 - math.Log(math.Tan(rad)+1/math.Cos(rad))/math.Pi) / 2 * n)
	return x, y
}

// Eine fehlende Kachel wird einmal geholt – und zwar wirklich.
//
// Der erste Anlauf hing: Der Abrufer wartete auf ein Signal, das nur er selbst
// hätte geben können. Die Karte blieb leer, ohne Fehlermeldung und ohne
// Zeitüberschreitung. Genau deshalb steht hier ein Test mit echtem Abruf.
func TestKachelWirdGeholtUndGespeichert(t *testing.T) {
	var abrufe int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&abrufe, 1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("kachelbytes"))
	}))
	defer server.Close()

	alt := Source
	Source = server.URL + "/%d/%d/%d.png"
	defer func() { Source = alt }()

	dir := t.TempDir()
	s := New(dir, "", nil)

	data, err := s.Tile(context.Background(), 14, 8817, 5484)
	if err != nil {
		t.Fatalf("Kachel nicht geholt: %v", err)
	}
	if string(data) != "kachelbytes" {
		t.Errorf("falscher Inhalt: %q", data)
	}

	// Der zweite Abruf kommt aus dem Speicher, nicht von außen.
	if _, err := s.Tile(context.Background(), 14, 8817, 5484); err != nil {
		t.Fatalf("zweiter Abruf scheiterte: %v", err)
	}
	if n := atomic.LoadInt32(&abrufe); n != 1 {
		t.Errorf("%d Abrufe nach außen, erwartet 1", n)
	}

	if _, err := os.Stat(filepath.Join(dir, "14", "8817", "5484.png")); err != nil {
		t.Errorf("Kachel liegt nicht im Speicher: %v", err)
	}
}

// Acht Telefone fragen dieselbe Kachel im selben Moment: Nach außen geht
// trotzdem nur eine Anfrage.
func TestGleichzeitigeAbrufeHolenNurEinmal(t *testing.T) {
	var abrufe int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&abrufe, 1)
		time.Sleep(50 * time.Millisecond) // der Abruf dauert
		_, _ = w.Write([]byte("kachelbytes"))
	}))
	defer server.Close()

	alt := Source
	Source = server.URL + "/%d/%d/%d.png"
	defer func() { Source = alt }()

	s := New(t.TempDir(), "", nil)

	var wg sync.WaitGroup
	fehler := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Tile(context.Background(), 14, 8817, 5484); err != nil {
				fehler <- err
			}
		}()
	}
	wg.Wait()
	close(fehler)

	for err := range fehler {
		t.Errorf("Abruf scheiterte: %v", err)
	}
	if n := atomic.LoadInt32(&abrufe); n != 1 {
		t.Errorf("%d Abrufe nach außen, erwartet 1", n)
	}
}

// Ein mitgebrachter Kachelsatz hat Vorrang – und braucht kein Netz.
func TestEigeneKachelnHabenVorrang(t *testing.T) {
	alt := Source
	Source = "http://127.0.0.1:1/%d/%d/%d.png" // nicht erreichbar
	defer func() { Source = alt }()

	lokal := t.TempDir()
	if err := os.MkdirAll(filepath.Join(lokal, "14", "8817"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lokal, "14", "8817", "5484.png"), []byte("eigene"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := New(t.TempDir(), lokal, nil)
	data, err := s.Tile(context.Background(), 14, 8817, 5484)
	if err != nil {
		t.Fatalf("eigene Kachel nicht gefunden: %v", err)
	}
	if string(data) != "eigene" {
		t.Errorf("nicht die eigene Kachel: %q", data)
	}
}
