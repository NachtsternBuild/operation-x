package game

import (
	"strings"
	"testing"
	"time"

	"github.com/elias/operation-x/pkg/geo"
)

func TestNormalizeAnswer(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Kreuzkirche", "kreuzkirche"},
		{"  KREUZKIRCHE  ", "kreuzkirche"},
		{"Königstraße", "koenigstrasse"},
		{"Koenigstrasse", "koenigstrasse"},
		{"Blaues  Wunder", "blaues wunder"},
		{"Blaues-Wunder", "blaues wunder"},
		{"1730.", "1730"},
	}

	for _, c := range cases {
		if got := NormalizeAnswer(c.in); got != c.want {
			t.Errorf("NormalizeAnswer(%q) = %q, erwartet %q", c.in, got, c.want)
		}
	}
}

func TestAnswerMatches(t *testing.T) {
	// Am Handy getippt darf Schreibweise nicht über richtig und falsch entscheiden.
	if !AnswerMatches("königstrasse", "Königstraße") {
		t.Error("Umlautschreibweise wurde abgelehnt")
	}
	if !AnswerMatches("  Zwinger ", "Zwinger") {
		t.Error("führende Leerzeichen wurden nicht entfernt")
	}

	// Mehrere zulässige Lösungen.
	if !AnswerMatches("Kreuzkirche Dresden", "Kreuzkirche|Kreuzkirche Dresden") {
		t.Error("zweite zulässige Lösung wurde abgelehnt")
	}
	if !AnswerMatches("kreuzkirche", "Kreuzkirche|Kreuzkirche Dresden") {
		t.Error("erste zulässige Lösung wurde abgelehnt")
	}

	if AnswerMatches("Frauenkirche", "Kreuzkirche") {
		t.Error("falsche Antwort wurde angenommen")
	}
	if AnswerMatches("", "Kreuzkirche") {
		t.Error("leere Antwort wurde angenommen")
	}
	if AnswerMatches("   ", "Kreuzkirche") {
		t.Error("Antwort aus Leerzeichen wurde angenommen")
	}
}

func TestFreshness(t *testing.T) {
	cfg := Defaults()
	now := time.Date(2026, 9, 9, 15, 0, 0, 0, time.UTC)

	cases := []struct {
		ageMin int
		want   string
	}{
		{0, FreshHot},
		{2, FreshHot},
		{4, FreshWarm},
		{14, FreshWarm},
		{16, FreshCold},
		{120, FreshCold},
	}

	for _, c := range cases {
		got := Freshness(now.Add(-time.Duration(c.ageMin)*time.Minute), cfg, now)
		if got != c.want {
			t.Errorf("%d Minuten alt ergab %q, erwartet %q", c.ageMin, got, c.want)
		}
	}
}

// traceFrom baut eine Spur aus Punkten, jüngste zuerst.
func traceFrom(now time.Time, points ...geo.Point) Trace {
	tp := make([]TracePoint, len(points))
	for i, p := range points {
		tp[i] = TracePoint{At: now.Add(-time.Duration(i*3) * time.Minute), Point: p}
	}
	return Trace{Points: tp}
}

func TestGreenIntelNamesHotspotAndTime(t *testing.T) {
	now := time.Now()
	trace := traceFrom(now, geo.Point{Lat: 51.0519, Lng: 13.7414})
	trace.LastHotspot = "h12"
	trace.LastHotspotNo = 12
	trace.LastHotspotTime = now.Add(-8 * time.Minute)

	intel, ok := GenerateIntel(IntelGreen, trace, now)
	if !ok {
		t.Fatal("kein Hinweis erzeugt")
	}

	if !strings.Contains(intel.Text, "#12") {
		t.Errorf("Hotspot-Nummer fehlt im Text: %q", intel.Text)
	}
	if intel.HotspotID != "h12" {
		t.Errorf("Hotspot-Bezug ist %q, erwartet h12", intel.HotspotID)
	}
	// Der Zeitstempel muss der Beobachtung entsprechen, nicht dem Jetzt –
	// daran hängt die Frischestufe.
	if !intel.OccurredAt.Equal(trace.LastHotspotTime) {
		t.Error("der Hinweis trägt nicht den Beobachtungszeitpunkt")
	}
}

func TestYellowIntelNamesTwoSectors(t *testing.T) {
	now := time.Now()
	trace := traceFrom(now, geo.Point{Lat: 51.0519, Lng: 13.7414})
	trace.SectorCode = "C"
	trace.SectorName = "Innere Altstadt"
	trace.NeighborCodes = []string{"D"}

	intel, ok := GenerateIntel(IntelYellow, trace, now)
	if !ok {
		t.Fatal("kein Hinweis erzeugt")
	}

	// Der echte Sektor muss dabei sein, sonst wäre der Hinweis schlicht falsch.
	if !strings.Contains(intel.Text, "C") {
		t.Errorf("der tatsächliche Sektor fehlt: %q", intel.Text)
	}
	if !strings.Contains(intel.Text, "oder") {
		t.Errorf("die Eingrenzung ist nicht unscharf: %q", intel.Text)
	}
}

func TestRedIntelReadsMovement(t *testing.T) {
	now := time.Now()

	// Rund vier Kilometer in sechs Minuten – das geht nur mit einem Fahrzeug.
	fast := Trace{Points: []TracePoint{
		{At: now, Point: geo.Point{Lat: 51.0800, Lng: 13.7414}},
		{At: now.Add(-3 * time.Minute), Point: geo.Point{Lat: 51.0660, Lng: 13.7414}},
		{At: now.Add(-6 * time.Minute), Point: geo.Point{Lat: 51.0519, Lng: 13.7414}},
	}}

	found := false
	// Der Text wird zufällig aus den Beobachtungen gewählt; über mehrere
	// Versuche muss die Geschwindigkeitsaussage vorkommen.
	for i := 0; i < 40; i++ {
		intel, ok := GenerateIntel(IntelRed, fast, now)
		if !ok {
			t.Fatal("kein Hinweis erzeugt")
		}
		if strings.Contains(intel.Text, "Bahn oder Bus") {
			found = true
			break
		}
	}
	if !found {
		t.Error("die schnelle Fortbewegung wurde nie erkannt")
	}
}

func TestBearingLabel(t *testing.T) {
	now := time.Now()

	// Von der Frauenkirche gut zwei Kilometer nach Norden.
	north := []TracePoint{
		{At: now, Point: geo.Point{Lat: 51.0719, Lng: 13.7414}},
		{At: now.Add(-10 * time.Minute), Point: geo.Point{Lat: 51.0519, Lng: 13.7414}},
	}

	dir, ok := bearingLabel(north)
	if !ok {
		t.Fatal("keine Richtung bestimmt")
	}
	if dir != "Norden" {
		t.Errorf("Richtung ist %q, erwartet Norden", dir)
	}

	// Zu kleine Bewegung ist nur GPS-Rauschen und darf keine Richtung ergeben.
	tiny := []TracePoint{
		{At: now, Point: geo.Point{Lat: 51.05195, Lng: 13.74145}},
		{At: now.Add(-2 * time.Minute), Point: geo.Point{Lat: 51.0519, Lng: 13.7414}},
	}
	if _, ok := bearingLabel(tiny); ok {
		t.Error("aus wenigen Metern wurde eine Richtung abgeleitet")
	}
}

func TestFabricatedIntelLooksReal(t *testing.T) {
	now := time.Now()
	codes := []string{"A", "B", "C", "D"}

	fake, ok := FabricateIntel(IntelYellow, codes, now)
	if !ok {
		t.Fatal("kein gefälschter Hinweis erzeugt")
	}
	if !fake.Fabricated {
		t.Error("die Fälschung ist nicht als solche markiert")
	}

	// Ein echter Hinweis derselben Kategorie muss denselben Satzbau haben,
	// sonst verrät die Formulierung die Fälschung.
	trace := traceFrom(now, geo.Point{Lat: 51.0519, Lng: 13.7414})
	trace.SectorCode = "A"
	trace.SectorName = "Altstadt"
	trace.NeighborCodes = []string{"B"}

	real, _ := GenerateIntel(IntelYellow, trace, now)

	prefix := "Mister X befindet sich in Sektor "
	if !strings.HasPrefix(fake.Text, prefix) || !strings.HasPrefix(real.Text, prefix) {
		t.Errorf("echter und gefälschter Hinweis sind unterschiedlich gebaut:\n  echt:      %q\n  gefälscht: %q",
			real.Text, fake.Text)
	}
}

// Zweimal derselbe Satz ist kein Hinweis mehr.
//
// Bleibt Mister X im selben Sektor, lieferte jedes gelöste Rätsel wortgleich
// dieselbe Auskunft. Für die Fahndung sah das aus, als wäre etwas kaputt — und
// die Belohnung fürs Lösen war verpufft, obwohl die Auskunft stimmte.
func TestHinweisWiederholtSichNichtWortgleich(t *testing.T) {
	now := time.Now()
	trace := Trace{
		Points:     []TracePoint{{At: now.Add(-2 * time.Minute), Point: geo.Point{Lat: 51.05, Lng: 13.74}}},
		SectorCode: "N3",
		SectorName: "Neustadt",
	}

	erster, ok := GenerateIntel(IntelYellow, trace, now)
	if !ok {
		t.Fatal("kein Hinweis erzeugt")
	}

	// Derselbe Stand, aber der Satz steht jetzt schon auf der Tafel.
	trace.Recent = []string{erster.Text}
	zweiter, ok := GenerateIntel(IntelYellow, trace, now)
	if !ok {
		t.Fatal("kein zweiter Hinweis erzeugt")
	}

	if zweiter.Text == erster.Text {
		t.Errorf("zweimal wortgleich: %q", zweiter.Text)
	}
	// Und er muss trotzdem denselben Sektor nennen – sonst wäre er gelogen.
	if !strings.Contains(zweiter.Text, "N3") {
		t.Errorf("der zweite Hinweis nennt den Sektor nicht mehr: %q", zweiter.Text)
	}
}

// Auch der gesicherte Hinweis wiederholt sich nicht: Wenn kein neuer Punkt
// bestätigt wurde, ist genau das die Nachricht.
func TestGesicherterHinweisMeldetStillstand(t *testing.T) {
	now := time.Now()
	trace := Trace{
		Points:          []TracePoint{{At: now.Add(-time.Minute), Point: geo.Point{Lat: 51.05, Lng: 13.74}}},
		LastHotspot:     "h7",
		LastHotspotNo:   7,
		LastHotspotTime: now.Add(-40 * time.Minute),
		SectorCode:      "N3",
		SectorName:      "Neustadt",
	}

	erster, _ := GenerateIntel(IntelGreen, trace, now)
	trace.Recent = []string{erster.Text}
	zweiter, _ := GenerateIntel(IntelGreen, trace, now)

	if zweiter.Text == erster.Text {
		t.Errorf("zweimal wortgleich: %q", zweiter.Text)
	}
	if !strings.Contains(zweiter.Text, "07") {
		t.Errorf("der Punkt fehlt im zweiten Hinweis: %q", zweiter.Text)
	}
}

// Die rote Beobachtung sucht sich eine noch nicht gesagte aus.
func TestRoterHinweisNimmtEineNochNichtGesagteBeobachtung(t *testing.T) {
	now := time.Now()
	trace := Trace{
		Points: []TracePoint{
			{At: now, Point: geo.Point{Lat: 51.0600, Lng: 13.7400}},
			{At: now.Add(-5 * time.Minute), Point: geo.Point{Lat: 51.0500, Lng: 13.7400}},
		},
	}

	gesehen := map[string]bool{}
	for i := 0; i < 12; i++ {
		got, ok := GenerateIntel(IntelRed, trace, now)
		if !ok {
			t.Fatal("kein Hinweis erzeugt")
		}
		gesehen[got.Text] = true
		trace.Recent = append(trace.Recent, got.Text)
	}

	if len(gesehen) < 2 {
		t.Errorf("zwölf Anläufe, nur %d verschiedene Beobachtungen", len(gesehen))
	}
}

// Der grüne Hinweis ohne bestätigten Hotspot ist ein eigener Zweig – und
// genau der fiel beim ersten Anlauf durch: Im Versuch mit echten Rätseln
// standen zwei wortgleiche grüne Hinweise untereinander, während die gelben
// und roten schon variierten. Ein Test je Zweig, nicht je Farbe.
func TestGruenerSektorHinweisWiederholtSichNicht(t *testing.T) {
	now := time.Now()
	trace := Trace{
		Points:     []TracePoint{{At: now.Add(-3 * time.Minute), Point: geo.Point{Lat: 51.06, Lng: 13.75}}},
		SectorCode: "B",
		SectorName: "Äußere Neustadt",
		// Kein LastHotspot: Es wurde noch kein Zwischenziel bestätigt.
	}

	erster, ok := GenerateIntel(IntelGreen, trace, now)
	if !ok {
		t.Fatal("kein grüner Hinweis erzeugt")
	}

	trace.Recent = []string{erster.Text}
	zweiter, ok := GenerateIntel(IntelGreen, trace, now)
	if !ok {
		t.Fatal("kein zweiter grüner Hinweis erzeugt")
	}

	if zweiter.Text == erster.Text {
		t.Errorf("zweimal wortgleich: %q", zweiter.Text)
	}
	if !strings.Contains(zweiter.Text, "B") {
		t.Errorf("der zweite Hinweis nennt den Sektor nicht mehr: %q", zweiter.Text)
	}

	// Und beim dritten Mal ebenso wenig.
	trace.Recent = []string{erster.Text, zweiter.Text}
	dritter, _ := GenerateIntel(IntelGreen, trace, now)
	if dritter.Text == erster.Text || dritter.Text == zweiter.Text {
		t.Errorf("dritter Hinweis wiederholt einen früheren: %q", dritter.Text)
	}
}

// Ein Hinweis, der mit einem Namen beginnt, behält seinen großen Anfang.
// "Stand 13:51 Uhr: mister X bewegt sich …" stand so im Versuch auf der Tafel.
func TestBeobachtungBleibtLesbar(t *testing.T) {
	now := time.Now()
	trace := Trace{
		Points: []TracePoint{
			{At: now.Add(-6 * time.Minute), Point: geo.Point{Lat: 51.050, Lng: 13.740}},
			{At: now.Add(-3 * time.Minute), Point: geo.Point{Lat: 51.070, Lng: 13.790}},
			{At: now.Add(-1 * time.Minute), Point: geo.Point{Lat: 51.090, Lng: 13.840}},
		},
	}

	erster, ok := GenerateIntel(IntelRed, trace, now)
	if !ok {
		t.Fatal("kein roter Hinweis erzeugt")
	}

	// Alle möglichen Beobachtungen stehen schon da – jetzt greift die
	// Ausweichformulierung.
	trace.Recent = []string{}
	for i := 0; i < 12; i++ {
		h, _ := GenerateIntel(IntelRed, trace, now)
		trace.Recent = append(trace.Recent, h.Text)
	}

	for _, text := range append(trace.Recent, erster.Text) {
		if strings.Contains(text, "mister X") {
			t.Errorf("Name klein geschrieben: %q", text)
		}
	}
}
