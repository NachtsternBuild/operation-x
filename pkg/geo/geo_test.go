package geo

import (
	"math"
	"testing"
)

func TestDistanceM(t *testing.T) {
	// Frauenkirche und Semperoper: gut 500 Meter Luftlinie über den Neumarkt
	// und den Theaterplatz.
	frauenkirche := Point{51.0519, 13.7414}
	semperoper := Point{51.0544, 13.7351}

	got := DistanceM(frauenkirche, semperoper)
	if got < 480 || got > 570 {
		t.Errorf("Entfernung Frauenkirche–Semperoper = %.0f m, erwartet 480–570 m", got)
	}

	if d := DistanceM(frauenkirche, frauenkirche); d != 0 {
		t.Errorf("Entfernung zu sich selbst = %v, erwartet 0", d)
	}
}

func TestOffset(t *testing.T) {
	start := Point{51.05, 13.74}

	// 200 Meter nach Norden versetzt muss auch 200 Meter entfernt liegen –
	// die Grundlage der Radar-Unschärfe.
	moved := Offset(start, 200, 0)
	if d := DistanceM(start, moved); math.Abs(d-200) > 1 {
		t.Errorf("200 m Versatz ergab %.1f m Entfernung", d)
	}

	east := Offset(start, 0, 200)
	if d := DistanceM(start, east); math.Abs(d-200) > 1 {
		t.Errorf("200 m Versatz nach Osten ergab %.1f m Entfernung", d)
	}
}

func TestAreaContains(t *testing.T) {
	// Quadrat um die Dresdner Altstadt.
	square := Area{Outer: Ring{
		{51.04, 13.73}, {51.04, 13.75}, {51.06, 13.75}, {51.06, 13.73}, {51.04, 13.73},
	}}

	if !square.Contains(Point{51.05, 13.74}) {
		t.Error("Punkt in der Mitte wurde als außerhalb gewertet")
	}
	if square.Contains(Point{51.07, 13.74}) {
		t.Error("Punkt nördlich davon wurde als innerhalb gewertet")
	}

	// Mit Aussparung in der Mitte.
	withHole := Area{
		Outer: square.Outer,
		Holes: []Ring{{
			{51.048, 13.738}, {51.048, 13.742}, {51.052, 13.742}, {51.052, 13.738}, {51.048, 13.738},
		}},
	}
	if withHole.Contains(Point{51.05, 13.74}) {
		t.Error("Punkt in der Aussparung wurde als innerhalb gewertet")
	}
	if !withHole.Contains(Point{51.055, 13.745}) {
		t.Error("Punkt außerhalb der Aussparung wurde als außerhalb gewertet")
	}
}

// TestBuildRings prüft das Zusammensetzen der Grenzen, wie OSM sie liefert:
// lose Wegstücke in beliebiger Reihenfolge und Richtung.
func TestBuildRings(t *testing.T) {
	tests := []struct {
		name  string
		ways  [][]Point
		rings int
	}{
		{
			name: "bereits geschlossener Weg",
			ways: [][]Point{{
				{0, 0}, {0, 1}, {1, 1}, {1, 0}, {0, 0},
			}},
			rings: 1,
		},
		{
			name: "zwei Stücke, in Reihenfolge",
			ways: [][]Point{
				{{0, 0}, {0, 1}, {1, 1}},
				{{1, 1}, {1, 0}, {0, 0}},
			},
			rings: 1,
		},
		{
			name: "zweites Stück verkehrt herum",
			ways: [][]Point{
				{{0, 0}, {0, 1}, {1, 1}},
				{{0, 0}, {1, 0}, {1, 1}}, // endet dort, wo das erste endet
			},
			rings: 1,
		},
		{
			name: "unsortiert und teils gedreht",
			ways: [][]Point{
				{{1, 1}, {1, 0}},
				{{0, 0}, {0, 1}},
				{{1, 1}, {0, 1}}, // gedreht
				{{1, 0}, {0, 0}},
			},
			rings: 1,
		},
		{
			name: "zwei getrennte Flächen",
			ways: [][]Point{
				{{0, 0}, {0, 1}, {1, 1}, {1, 0}, {0, 0}},
				{{5, 5}, {5, 6}, {6, 6}, {6, 5}, {5, 5}},
			},
			rings: 2,
		},
		{
			name: "unvollständige Grenze wird verworfen",
			ways: [][]Point{
				{{0, 0}, {0, 1}},
				{{0, 1}, {1, 1}},
				// Der Rückweg fehlt.
			},
			rings: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rings := buildRings(tc.ways)
			if len(rings) != tc.rings {
				t.Fatalf("%d Ringe gebildet, erwartet %d", len(rings), tc.rings)
			}
			for i, r := range rings {
				if !samePoint(r[0], r[len(r)-1]) {
					t.Errorf("Ring %d ist nicht geschlossen: %v … %v", i, r[0], r[len(r)-1])
				}
			}
		})
	}
}

func TestGeoJSONRoundtrip(t *testing.T) {
	original := Area{
		Outer: Ring{{51.04, 13.73}, {51.04, 13.75}, {51.06, 13.75}, {51.04, 13.73}},
		Holes: []Ring{{{51.048, 13.738}, {51.048, 13.742}, {51.052, 13.742}, {51.048, 13.738}}},
	}

	back, err := AreaFromGeoJSON(original.GeoJSON())
	if err != nil {
		t.Fatalf("Rückwandlung schlug fehl: %v", err)
	}

	if len(back.Outer) != len(original.Outer) {
		t.Errorf("Außenring hat %d Punkte, erwartet %d", len(back.Outer), len(original.Outer))
	}
	if len(back.Holes) != 1 {
		t.Fatalf("%d Aussparungen, erwartet 1", len(back.Holes))
	}
	// Vertauschte Reihenfolge von Länge und Breite wäre der klassische Fehler.
	if math.Abs(back.Outer[0].Lat-51.04) > 1e-9 || math.Abs(back.Outer[0].Lng-13.73) > 1e-9 {
		t.Errorf("erster Punkt kam als %v zurück, erwartet {51.04 13.73}", back.Outer[0])
	}
}

func TestCentroid(t *testing.T) {
	square := Ring{{0, 0}, {0, 2}, {2, 2}, {2, 0}, {0, 0}}
	c := square.Centroid()

	if math.Abs(c.Lat-1) > 1e-9 || math.Abs(c.Lng-1) > 1e-9 {
		t.Errorf("Schwerpunkt = %v, erwartet {1 1}", c)
	}
}

// Eine Stadtteilgrenze aus OpenStreetMap hat tausende Punkte und folgt jedem
// Grundstückszaun. Auf einer Spielkarte ist davon nichts zu sehen – und in
// einem Stadtpaket, das am Spieltag über Mobilfunk geladen wird, sind sie der
// Unterschied zwischen zwanzig Kilobyte und einem halben Megabyte.
func TestSimplifyDuenntAusOhneDieFormZuVerlieren(t *testing.T) {
	// Ein Quadrat von rund einem Kilometer Kantenlänge, dessen Kanten mit
	// vielen Zwischenpunkten und etwas Rauschen abgetastet sind.
	ring := Ring{}
	schritte := 40
	for i := 0; i <= schritte; i++ {
		ring = append(ring, Point{Lat: 51.05, Lng: 13.74 + float64(i)*0.0003})
	}
	for i := 1; i <= schritte; i++ {
		ring = append(ring, Point{Lat: 51.05 + float64(i)*0.00018, Lng: 13.752})
	}
	for i := 1; i <= schritte; i++ {
		ring = append(ring, Point{Lat: 51.0572, Lng: 13.752 - float64(i)*0.0003})
	}
	for i := 1; i <= schritte; i++ {
		ring = append(ring, Point{Lat: 51.0572 - float64(i)*0.00018, Lng: 13.74})
	}

	klein := Simplify(ring, 40)

	if len(klein) >= len(ring) {
		t.Errorf("nicht ausgedünnt: %d von %d Punkten übrig", len(klein), len(ring))
	}
	// Die vier Ecken müssen bleiben, sonst wäre es kein Quadrat mehr.
	if len(klein) < 5 {
		t.Errorf("zu stark ausgedünnt: nur %d Punkte", len(klein))
	}

	// Die Fläche darf sich kaum ändern.
	vorher, nachher := ring.AreaM2(), klein.AreaM2()
	if abw := math.Abs(vorher-nachher) / vorher; abw > 0.02 {
		t.Errorf("Fläche weicht um %.1f%% ab", abw*100)
	}

	// Und geschlossen bleibt geschlossen.
	if ring[0] == ring[len(ring)-1] && klein[0] != klein[len(klein)-1] {
		t.Error("der Ring ist nach dem Ausdünnen offen")
	}
}

func TestSimplifyLaesstKleineRingeInRuhe(t *testing.T) {
	dreieck := Ring{
		{Lat: 51.05, Lng: 13.74},
		{Lat: 51.06, Lng: 13.75},
		{Lat: 51.05, Lng: 13.76},
		{Lat: 51.05, Lng: 13.74},
	}
	if got := Simplify(dreieck, 40); len(got) != len(dreieck) {
		t.Errorf("aus %d Punkten wurden %d", len(dreieck), len(got))
	}
}
