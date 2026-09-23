package game

import (
	"testing"

	"github.com/elias/operation-x/pkg/geo"
)

// Echte Dresdner Orte, damit die Entfernungen im Test denen im Spiel entsprechen.
func dresdenCandidates() []Candidate {
	return []Candidate{
		{HotspotID: "h1", Number: 1, Name: "Frauenkirche", Point: geo.Point{Lat: 51.0519, Lng: 13.7414}},
		{HotspotID: "h2", Number: 2, Name: "Zwinger", Point: geo.Point{Lat: 51.0533, Lng: 13.7343}},
		{HotspotID: "h3", Number: 3, Name: "Semperoper", Point: geo.Point{Lat: 51.0544, Lng: 13.7351}},
		{HotspotID: "h5", Number: 5, Name: "Hauptbahnhof", Point: geo.Point{Lat: 51.0400, Lng: 13.7322}},
		{HotspotID: "h6", Number: 6, Name: "Yenidze", Point: geo.Point{Lat: 51.0546, Lng: 13.7237}},
		{HotspotID: "h8", Number: 8, Name: "Goldener Reiter", Point: geo.Point{Lat: 51.0577, Lng: 13.7420}},
		{HotspotID: "h11", Number: 11, Name: "Alaunpark", Point: geo.Point{Lat: 51.0716, Lng: 13.7541}},
		{HotspotID: "h13", Number: 13, Name: "Großer Garten", Point: geo.Point{Lat: 51.0400, Lng: 13.7660}},
		{HotspotID: "h17", Number: 17, Name: "Blaues Wunder", Point: geo.Point{Lat: 51.0540, Lng: 13.8090}},
		{HotspotID: "h19", Number: 19, Name: "Panometer", Point: geo.Point{Lat: 51.0308, Lng: 13.7614}},
	}
}

func TestProposeGivesThreeDistinctOptions(t *testing.T) {
	in := RouteInput{
		From:       geo.Point{Lat: 51.0519, Lng: 13.7414}, // Frauenkirche
		Candidates: dresdenCandidates(),
		Detectives: []geo.Point{{Lat: 51.0533, Lng: 13.7343}}, // Zwinger
		Visited:    map[string]bool{"h1": true},
	}

	opts := Propose(in, Defaults())
	if len(opts) != 3 {
		t.Fatalf("%d Varianten vorgeschlagen, erwartet 3", len(opts))
	}

	seen := map[string]bool{}
	for _, o := range opts {
		if seen[o.Candidate.HotspotID] {
			t.Errorf("Ziel %q wurde zweimal vorgeschlagen", o.Candidate.Name)
		}
		seen[o.Candidate.HotspotID] = true

		if o.Candidate.HotspotID == "h1" {
			t.Error("der aktuelle Standort wurde als Ziel vorgeschlagen")
		}
		if o.TimeLimitMin < 10 {
			t.Errorf("Variante %q hat nur %d Minuten Frist", o.Label, o.TimeLimitMin)
		}
	}
}

func TestSafeOptionAvoidsDetectives(t *testing.T) {
	// Zwei Detektivteams stehen westlich; das sichere Ziel muss ostwärts liegen.
	in := RouteInput{
		From:       geo.Point{Lat: 51.0519, Lng: 13.7414},
		Candidates: dresdenCandidates(),
		Detectives: []geo.Point{
			{Lat: 51.0533, Lng: 13.7343}, // Zwinger
			{Lat: 51.0546, Lng: 13.7237}, // Yenidze
		},
		Visited: map[string]bool{"h1": true},
	}

	opts := Propose(in, Defaults())

	var safe, fast *Option
	for i := range opts {
		switch opts[i].Kind {
		case "safe":
			safe = &opts[i]
		case "fast":
			fast = &opts[i]
		}
	}

	if safe == nil || fast == nil {
		t.Fatal("es fehlt eine der Varianten")
	}

	if safe.Candidate.DetectiveM <= fast.Candidate.DetectiveM {
		t.Errorf("die sichere Variante (%s, %.0f m zur Fahndung) liegt nicht weiter weg als die schnelle (%s, %.0f m)",
			safe.Candidate.Name, safe.Candidate.DetectiveM,
			fast.Candidate.Name, fast.Candidate.DetectiveM)
	}

	if fast.Candidate.DistanceM >= safe.Candidate.DistanceM {
		t.Errorf("die schnelle Variante (%s, %.0f m) ist nicht näher als die sichere (%s, %.0f m)",
			fast.Candidate.Name, fast.Candidate.DistanceM,
			safe.Candidate.Name, safe.Candidate.DistanceM)
	}
}

func TestVisitedTargetsAreSkipped(t *testing.T) {
	visited := map[string]bool{"h1": true, "h2": true, "h3": true, "h8": true}

	in := RouteInput{
		From:       geo.Point{Lat: 51.0519, Lng: 13.7414},
		Candidates: dresdenCandidates(),
		Visited:    visited,
	}

	for _, o := range Propose(in, Defaults()) {
		if visited[o.Candidate.HotspotID] {
			t.Errorf("bereits besuchtes Ziel %q wurde erneut vorgeschlagen", o.Candidate.Name)
		}
	}
}

// Gegen Ende des Spiels soll die Route auf das Fluchtziel zulaufen, statt
// weiter durch die Stadt zu mäandern.
func TestProgressPullsTowardFinalTarget(t *testing.T) {
	final := geo.Point{Lat: 51.0540, Lng: 13.8090} // Blaues Wunder, weit im Osten
	from := geo.Point{Lat: 51.0519, Lng: 13.7414}  // Frauenkirche

	base := RouteInput{
		From:        from,
		Candidates:  dresdenCandidates(),
		Visited:     map[string]bool{"h1": true},
		FinalTarget: &final,
	}

	early := base
	early.Progress = 0.05
	late := base
	late.Progress = 0.9

	distanceToFinal := func(opts []Option) float64 {
		// Bestbewertete Variante ist die Bonusvariante.
		for _, o := range opts {
			if o.Kind == "bonus" {
				return geo.DistanceM(o.Candidate.Point, final)
			}
		}
		return -1
	}

	dEarly := distanceToFinal(Propose(early, Defaults()))
	dLate := distanceToFinal(Propose(late, Defaults()))

	if dEarly < 0 || dLate < 0 {
		t.Fatal("keine Bonusvariante erzeugt")
	}

	if dLate > dEarly {
		t.Errorf("spät im Spiel führte der Vorschlag %.0f m ans Ziel heran, früh %.0f m – "+
			"der Sog zum Fluchtziel wirkt nicht", dLate, dEarly)
	}
}

func TestTimeLimitScalesWithDistance(t *testing.T) {
	short := timeLimit(500, 1.5)
	long := timeLimit(2500, 1.5)

	if short >= long {
		t.Errorf("500 m ergaben %d Minuten, 2500 m nur %d", short, long)
	}
	if short < 10 {
		t.Errorf("Mindestfrist unterschritten: %d Minuten", short)
	}

	// 2,5 km zu Fuß sind rund 38 Minuten; mit Zuschlag etwa eine Stunde.
	if long < 45 || long > 75 {
		t.Errorf("2500 m ergaben %d Minuten, erwartet 45 bis 75", long)
	}
}

func TestPickFinalTargetIsFarAway(t *testing.T) {
	start := geo.Point{Lat: 51.0519, Lng: 13.7414}

	target, ok := PickFinalTarget(start, dresdenCandidates())
	if !ok {
		t.Fatal("kein Fluchtziel gewählt")
	}

	if d := geo.DistanceM(start, target.Point); d < 2000 {
		t.Errorf("Fluchtziel %q liegt nur %.0f m entfernt", target.Name, d)
	}
}
