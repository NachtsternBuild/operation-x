// Package tiles liefert die Kartenkacheln des Spielgebiets.
//
// Bisher holte sich jedes Gerät die Kacheln direkt von den Servern von
// OpenStreetMap. Das hat drei Nachteile, und alle drei treffen genau am
// Spieltag zu:
//
//   - Acht Telefone laden dieselben Kacheln achtmal, über Mobilfunk, auf
//     Kosten der Akkus und eines fremden Servers, der die Karte kostenlos
//     bereitstellt.
//   - Im Funkloch bleibt die Karte leer – auch an einer Stelle, die eine
//     Minute vorher schon jemand angesehen hat.
//   - Wer zu Hause ohne Internet spielen will (eigener WLAN-Hotspot, kein
//     Mobilfunk), bekommt gar keine Karte.
//
// Deshalb geht der Weg jetzt über den Spielserver: Er liefert eine Kachel aus
// seinem Speicher, und nur wenn er sie noch nicht hat, holt er sie einmal von
// OpenStreetMap. Danach hat sie jedes Gerät, ohne dass ein einziges Byte
// nochmal aus dem Netz kommt.
//
// Was hier bewusst NICHT passiert: das Vorabladen einer ganzen Stadt. Die
// Nutzungsbedingungen von OpenStreetMap erlauben die interaktive Nutzung und
// verbieten das massenhafte Herunterladen von Gebieten. Gespeichert wird also
// nur, was jemand tatsächlich angesehen hat – das ist Zwischenspeicherung, und
// die ist ausdrücklich erwünscht.
//
// Wer vollständig ohne Netz spielen will, legt seine eigenen Kacheln in einen
// Ordner "kacheln" neben das Programm (Aufbau z/x/y.png). Die werden immer
// zuerst genommen, und das Spiel läuft dann ohne jede Verbindung nach außen.
package tiles

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/elias/operation-x/pkg/geo"
)

const (
	// Unterhalb davon zeigt eine Kachel mehr als das Spielgebiet, darüber
	// beginnt eine Genauigkeit, die im Laufen niemand braucht. Die Grenzen
	// halten den Speicher klein und verhindern, dass der Server sich als
	// allgemeiner Kartenvermittler benutzen lässt.
	minZoom = 9
	maxZoom = 19

	// Wie viele Kacheln gleichzeitig von außen geholt werden.
	//
	// Vier reichen für einen flüssigen Kartenaufbau und halten die Last auf
	// dem fremden Server in dem Rahmen, den seine Bedingungen vorsehen.
	maxParallel = 4

	fetchTimeout = 15 * time.Second

	// OpenStreetMap verlangt eine Kennung, an der die Anwendung erkennbar ist.
	userAgent = "OperationX/1.0 (selbstgehosteter Spielserver; " + geo.ProjektURL + ")"
)

// Source ist die Bezugsquelle, wenn eine Kachel fehlt.
var Source = "https://tile.openstreetmap.org/%d/%d/%d.png"

// Store verwaltet den Kachelspeicher.
type Store struct {
	// Wohin geholte Kacheln geschrieben werden.
	cacheDir string
	// Ein mitgebrachter Kachelsatz, falls vorhanden. Hat Vorrang.
	localDir string

	// Das Spielgebiet begrenzt, was überhaupt geholt wird.
	area func() (geo.Bounds, bool)

	client *http.Client
	slots  chan struct{}

	// Damit dieselbe Kachel nicht von acht Geräten gleichzeitig geholt wird.
	pending sync.Map
}

// New legt einen Speicher an. [cacheDir] gehört in den Datenordner, [localDir]
// ist der mitgebrachte Kachelsatz neben dem Programm.
func New(cacheDir, localDir string, area func() (geo.Bounds, bool)) *Store {
	return &Store{
		cacheDir: cacheDir,
		localDir: localDir,
		area:     area,
		client:   &http.Client{Timeout: fetchTimeout},
		slots:    make(chan struct{}, maxParallel),
	}
}

// ErrOutside meldet eine Kachel außerhalb des Spielgebiets.
var ErrOutside = errors.New("Kachel liegt außerhalb des Spielgebiets")

// Tile liefert eine Kachel als Bytes.
func (s *Store) Tile(ctx context.Context, z, x, y int) ([]byte, error) {
	if z < minZoom || z > maxZoom {
		return nil, ErrOutside
	}
	max := 1 << z
	if x < 0 || y < 0 || x >= max || y >= max {
		return nil, ErrOutside
	}
	if !s.inArea(z, x, y) {
		return nil, ErrOutside
	}

	rel := filepath.Join(strconv.Itoa(z), strconv.Itoa(x), strconv.Itoa(y)+".png")

	// Mitgebrachte Kacheln zuerst: Wer einen eigenen Satz danebengelegt hat,
	// will genau den sehen – und dann auch ohne Netz.
	if s.localDir != "" {
		if data, err := os.ReadFile(filepath.Join(s.localDir, rel)); err == nil {
			return data, nil
		}
	}

	path := filepath.Join(s.cacheDir, rel)
	if data, err := os.ReadFile(path); err == nil {
		return data, nil
	}

	// Nur einer holt, die anderen warten darauf.
	//
	// LoadOrStore meldet mit dem zweiten Rückgabewert, ob der Eintrag schon da
	// war – nicht, ob man der Erste ist. Andersherum gelesen wartete der erste
	// Abrufer auf ein Signal, das nur er selbst hätte geben können: Die Karte
	// blieb stehen, ohne Fehler, ohne Zeitüberschreitung. Aufgefallen beim
	// ersten Abruf gegen den echten Kachelserver.
	wait, schonUnterwegs := s.pending.LoadOrStore(rel, make(chan struct{}))
	ch := wait.(chan struct{})

	if schonUnterwegs {
		select {
		case <-ch:
			return os.ReadFile(path)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	defer func() {
		s.pending.Delete(rel)
		close(ch)
	}()

	data, err := s.fetch(ctx, z, x, y)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
		// Erst daneben schreiben, dann umbenennen: Bricht der Server mitten
		// im Schreiben ab, liegt sonst eine halbe Kachel im Speicher und wird
		// von da an als gültig ausgeliefert.
		tmp := path + ".teil"
		if os.WriteFile(tmp, data, 0o644) == nil {
			_ = os.Rename(tmp, path)
		}
	}

	return data, nil
}

func (s *Store) fetch(ctx context.Context, z, x, y int) ([]byte, error) {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf(Source, z, x, y), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	res, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Kachelserver antwortete mit %d", res.StatusCode)
	}

	return io.ReadAll(io.LimitReader(res.Body, 2<<20))
}

// inArea prüft, ob eine Kachel das Spielgebiet berührt.
//
// Ohne diese Prüfung wäre der Server ein Kachelvermittler für die ganze Welt –
// über den Tunnel für jeden erreichbar, der die Adresse kennt, und auf Kosten
// eines fremden Servers.
func (s *Store) inArea(z, x, y int) bool {
	if s.area == nil {
		return true
	}
	area, ok := s.area()
	if !ok {
		// Noch kein Gebiet eingerichtet: Dann ist die Karte beim Vorbereiten
		// zu sehen, und genau dafür wird sie gebraucht.
		return true
	}

	// Ein großzügiger Rand, damit der Kartenrand nicht abgeschnitten wirkt.
	area = area.Grow(3000)

	west, north := tileToLatLng(z, x, y)
	east, south := tileToLatLng(z, x+1, y+1)

	return west <= area.East && east >= area.West &&
		south <= area.North && north >= area.South
}

// tileToLatLng liefert die Nordwest-Ecke einer Kachel.
func tileToLatLng(z, x, y int) (lng, lat float64) {
	n := math.Pow(2, float64(z))
	lng = float64(x)/n*360 - 180
	lat = math.Atan(math.Sinh(math.Pi*(1-2*float64(y)/n))) * 180 / math.Pi
	return lng, lat
}

// Stat beschreibt den Speicher für die Anzeige in der Zentrale.
type Stat struct {
	Count int   `json:"count"`
	Bytes int64 `json:"bytes"`
	Local bool  `json:"local"`
}

// Stat zählt, was gespeichert ist.
func (s *Store) Stat() Stat {
	out := Stat{Local: s.localDir != ""}

	_ = filepath.Walk(s.cacheDir, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		out.Count++
		out.Bytes += info.Size()
		return nil
	})

	return out
}
