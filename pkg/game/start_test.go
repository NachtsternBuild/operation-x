package game

import (
	"math/rand"
	"testing"

	"github.com/elias/operation-x/pkg/geo"
)

func punkte(n int) []StartCandidate {
	out := make([]StartCandidate, n)
	for i := 0; i < n; i++ {
		// Auf einer Linie im Abstand von rund 800 Metern.
		out[i] = StartCandidate{
			HotspotID: string(rune('a' + i)),
			Number:    i + 1,
			Point:     geo.Point{Lat: 51.05 + float64(i)*0.0072, Lng: 13.74},
		}
	}
	return out
}

// Der Grund für die ganze Auslosung: Niemand darf dort anfangen, wo schon
// jemand anfängt. Starten Zielperson und ein Fahndungsteam am selben Ort, ist
// das Spiel in der ersten Minute entschieden.
func TestJederPunktNurEinmal(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	draws := DrawStarts(punkte(8), []string{"x", "a", "b", "c"}, rnd)

	if len(draws) != 4 {
		t.Fatalf("%d Zuordnungen, erwartet 4", len(draws))
	}

	gesehen := map[string]bool{}
	teams := map[string]bool{}
	for _, d := range draws {
		if gesehen[d.HotspotID] {
			t.Errorf("Hotspot %q zweimal vergeben", d.HotspotID)
		}
		gesehen[d.HotspotID] = true
		teams[d.TeamID] = true
	}
	if len(teams) != 4 {
		t.Errorf("%d Teams bedacht, erwartet 4", len(teams))
	}
}

// Zu wenige Punkte sind keine Panne, sondern eine Frage an die Spielleitung.
func TestZuWenigePunkteErgibtNichts(t *testing.T) {
	rnd := rand.New(rand.NewSource(2))
	if got := DrawStarts(punkte(2), []string{"x", "a", "b"}, rnd); got != nil {
		t.Errorf("bei zu wenigen Punkten kam eine Auslosung heraus: %v", got)
	}
}

// Der Mindestabstand wird angestrebt, solange er zu haben ist.
func TestStartpunkteLiegenAuseinander(t *testing.T) {
	kandidaten := punkte(10)
	rnd := rand.New(rand.NewSource(3))
	draws := DrawStarts(kandidaten, []string{"x", "a", "b", "c"}, rnd)

	stelle := map[string]geo.Point{}
	for _, c := range kandidaten {
		stelle[c.HotspotID] = c.Point
	}

	for i := 0; i < len(draws); i++ {
		for j := i + 1; j < len(draws); j++ {
			d := geo.DistanceM(stelle[draws[i].HotspotID], stelle[draws[j].HotspotID])
			if d < StartMinDistanceM {
				t.Errorf("Startpunkte %d und %d liegen nur %.0f m auseinander",
					draws[i].Number, draws[j].Number, d)
			}
		}
	}
}

// Enges Spielgebiet: Dann gibt es den Abstand nicht mehr – und trotzdem muss
// jedes Team einen Startpunkt bekommen.
func TestEngesGebietVerteiltTrotzdem(t *testing.T) {
	// Vier Punkte, alle innerhalb von hundert Metern.
	eng := []StartCandidate{
		{HotspotID: "a", Number: 1, Point: geo.Point{Lat: 51.0500, Lng: 13.7400}},
		{HotspotID: "b", Number: 2, Point: geo.Point{Lat: 51.0503, Lng: 13.7402}},
		{HotspotID: "c", Number: 3, Point: geo.Point{Lat: 51.0506, Lng: 13.7404}},
		{HotspotID: "d", Number: 4, Point: geo.Point{Lat: 51.0508, Lng: 13.7406}},
	}

	rnd := rand.New(rand.NewSource(4))
	draws := DrawStarts(eng, []string{"x", "a", "b", "c"}, rnd)

	if len(draws) != 4 {
		t.Fatalf("%d Zuordnungen, erwartet 4", len(draws))
	}
	gesehen := map[string]bool{}
	for _, d := range draws {
		if gesehen[d.HotspotID] {
			t.Errorf("auch im engen Gebiet darf kein Punkt doppelt vergeben werden")
		}
		gesehen[d.HotspotID] = true
	}
}

// Zweimal ziehen ergibt zwei verschiedene Verteilungen – sonst wäre es keine
// Auslosung, sondern eine Liste.
func TestAuslosungIstZufaellig(t *testing.T) {
	a := DrawStarts(punkte(12), []string{"x", "a", "b"}, rand.New(rand.NewSource(1)))
	b := DrawStarts(punkte(12), []string{"x", "a", "b"}, rand.New(rand.NewSource(99)))

	gleich := true
	for i := range a {
		if a[i].HotspotID != b[i].HotspotID {
			gleich = false
			break
		}
	}
	if gleich {
		t.Error("zwei Ziehungen ergaben dieselbe Verteilung")
	}
}
