package game

import (
	"math"
	"sort"

	"github.com/elias/operation-x/pkg/geo"
)

/*
Dynamische Routenwahl für Mister X.

Das Regelwerk verlangt, dass der Server nach jedem Erfolg das nächste
Zwischenziel bestimmt – nach Entfernung, Nahverkehrstakt, Restzeit und
Fahndungslage. Ein echter Fahrplananschluss wäre ein eigenes Projekt und würde
für ein Geburtstagsspiel nichts hinzufügen, was man am Ziel merkt.

Stattdessen bewertet der Server die Hotspots im Umkreis und schlägt die drei
bestbewerteten als Varianten vor. Am Spieltisch fühlt sich das identisch an: Man
bekommt drei Vorschläge, von denen einer sicher, einer schnell und einer
lohnend ist.
*/

// Spielbare Entfernung zu Fuß zwischen zwei Zwischenzielen.
//
// Unter 400 Metern ist ein Ziel keine Reise, sondern ein Schritt zur Seite –
// die Detektive bekämen keine Gelegenheit, die Lücke zu schließen. Über drei
// Kilometern wird es zäh, wenn jemand nicht gerade eine Straßenbahn erwischt.
const (
	minLegM   = 400
	idealLegM = 1200
	maxLegM   = 3000
)

// Gehgeschwindigkeit für die Zeitberechnung, in Metern pro Minute.
// Bewusst niedrig angesetzt: gerechnet wird mit Ampeln, Umwegen und der
// Tatsache, dass niemand auf einem Geburtstag im Laufschritt unterwegs ist.
const walkSpeedMPerMin = 65

// Candidate ist ein bewerteter Hotspot.
type Candidate struct {
	HotspotID string
	Number    int
	Name      string
	Point     geo.Point
	SectorID  string

	DistanceM float64
	// Abstand zum nächsten bekannten Detektivteam. Groß heißt sicher.
	DetectiveM float64
	Score      float64
}

// RouteInput ist die Lage, aus der die Vorschläge entstehen.
type RouteInput struct {
	From       geo.Point
	Candidates []Candidate
	// Positionen der Detektivteams, soweit bekannt.
	Detectives []geo.Point
	// Hotspots, die schon besucht wurden.
	Visited map[string]bool
	// Kennung des geheimen Fluchtziels; Ziele in seiner Richtung werden
	// bevorzugt, damit die Route nicht ziellos mäandert.
	FinalTarget *geo.Point
	// Anteil der bereits verstrichenen Spielzeit, 0 bis 1.
	Progress float64
}

// Option ist eine Variante, die Mister X vor Ort zur Wahl gestellt bekommt.
type Option struct {
	Kind         string // safe, fast, bonus
	Label        string
	Description  string
	Candidate    Candidate
	TimeLimitMin int
	RewardPoints int
	RewardFP     int
}

// Propose bewertet die Kandidaten und liefert bis zu drei Varianten.
//
// Die drei unterscheiden sich nicht nur im Text: „Unauffällig“ wählt das Ziel
// mit dem größten Abstand zur Fahndung, „Schnell“ das nächstgelegene mit
// knapper Frist, und „Bonus“ eines, das etwas kostet – weiter weg oder näher an
// den Detektiven – und dafür einen Fluchtpunkt einbringt.
func Propose(in RouteInput, cfg Config) []Option {
	scored := make([]Candidate, 0, len(in.Candidates))

	for _, c := range in.Candidates {
		if in.Visited[c.HotspotID] {
			continue
		}

		c.DistanceM = geo.DistanceM(in.From, c.Point)
		if c.DistanceM < minLegM || c.DistanceM > maxLegM {
			continue
		}

		c.DetectiveM = nearestDistance(c.Point, in.Detectives)
		c.Score = score(c, in)
		scored = append(scored, c)
	}

	if len(scored) == 0 {
		// Kein Ziel im brauchbaren Ring: Der Umkreis wird notfalls geöffnet,
		// damit das Spiel nicht stehenbleibt.
		for _, c := range in.Candidates {
			if in.Visited[c.HotspotID] {
				continue
			}
			c.DistanceM = geo.DistanceM(in.From, c.Point)
			c.DetectiveM = nearestDistance(c.Point, in.Detectives)
			c.Score = score(c, in)
			scored = append(scored, c)
		}
	}
	if len(scored) == 0 {
		return nil
	}

	options := make([]Option, 0, 3)
	used := map[string]bool{}

	// Unauffällig: möglichst weit weg von der Fahndung.
	if c, ok := pick(scored, used, func(a, b Candidate) bool {
		return a.DetectiveM > b.DetectiveM
	}); ok {
		options = append(options, Option{
			Kind:         "safe",
			Label:        "Unauffällig",
			Description:  "Größter Abstand zur bekannten Fahndungslage. Ruhige Route, volle Zeit.",
			Candidate:    c,
			TimeLimitMin: timeLimit(c.DistanceM, 1.6),
			RewardPoints: cfg.PointsMission,
		})
		used[c.HotspotID] = true
	}

	// Schnell: das nächstgelegene Ziel, dafür knappe Frist.
	if c, ok := pick(scored, used, func(a, b Candidate) bool {
		return a.DistanceM < b.DistanceM
	}); ok {
		options = append(options, Option{
			Kind:         "fast",
			Label:        "Schnell und riskant",
			Description:  "Kurzer Weg, knappe Frist. Bringt Zeit für das Finale – wenn es klappt.",
			Candidate:    c,
			TimeLimitMin: timeLimit(c.DistanceM, 1.15),
			RewardPoints: cfg.PointsMission + 5,
		})
		used[c.HotspotID] = true
	}

	// Bonus: das Ziel mit der höchsten Gesamtbewertung, das noch übrig ist.
	if c, ok := pick(scored, used, func(a, b Candidate) bool {
		return a.Score > b.Score
	}); ok {
		options = append(options, Option{
			Kind:         "bonus",
			Label:        "Bonusauftrag",
			Description:  "Anspruchsvoller Weg. Bringt zusätzlich einen Fluchtpunkt.",
			Candidate:    c,
			TimeLimitMin: timeLimit(c.DistanceM, 1.45),
			RewardPoints: cfg.PointsMission,
			RewardFP:     1,
		})
		used[c.HotspotID] = true
	}

	return options
}

// score bewertet einen Kandidaten als Zwischenziel.
func score(c Candidate, in RouteInput) float64 {
	s := 0.0

	// Entfernung: Am besten liegt ein Ziel im mittleren Bereich. Der Abstand
	// zum Idealwert zählt negativ.
	s -= math.Abs(c.DistanceM-idealLegM) / 1000

	// Abstand zur Fahndung: bis 1500 Meter zählt jeder Meter, danach bringt
	// mehr Abstand keinen zusätzlichen Vorteil.
	s += math.Min(c.DetectiveM, 1500) / 500

	// Richtung des Fluchtziels: Je weiter das Spiel fortgeschritten ist, desto
	// stärker zieht es Mister X dorthin. Am Anfang darf die Route mäandern.
	if in.FinalTarget != nil {
		toFinalNow := geo.DistanceM(in.From, *in.FinalTarget)
		toFinalThen := geo.DistanceM(c.Point, *in.FinalTarget)
		gain := (toFinalNow - toFinalThen) / 1000
		s += gain * (0.5 + 2*in.Progress)
	}

	return s
}

// pick sucht den besten noch unbenutzten Kandidaten nach einem Kriterium.
func pick(list []Candidate, used map[string]bool, better func(a, b Candidate) bool) (Candidate, bool) {
	var best Candidate
	found := false

	for _, c := range list {
		if used[c.HotspotID] {
			continue
		}
		if !found || better(c, best) {
			best = c
			found = true
		}
	}

	return best, found
}

// timeLimit rechnet die Frist aus der Entfernung. Der Faktor bestimmt, wie
// großzügig sie ausfällt.
func timeLimit(distanceM, slack float64) int {
	minutes := (distanceM / walkSpeedMPerMin) * slack

	// Unter zehn Minuten wird jede Frist zur Hetzjagd über eine rote Ampel.
	if minutes < 10 {
		minutes = 10
	}
	return int(math.Ceil(minutes/5) * 5) // auf volle fünf Minuten runden
}

func nearestDistance(p geo.Point, others []geo.Point) float64 {
	if len(others) == 0 {
		// Ohne bekannte Fahndungspositionen ist jedes Ziel gleich sicher.
		return 1500
	}

	best := math.MaxFloat64
	for _, o := range others {
		if d := geo.DistanceM(p, o); d < best {
			best = d
		}
	}
	return best
}

// PickFinalTarget wählt das geheime Fluchtziel zu Spielbeginn.
//
// Es soll weit genug vom Start entfernt liegen, dass die Route etwas zu
// erzählen hat – aber erreichbar bleiben.
func PickFinalTarget(start geo.Point, candidates []Candidate) (Candidate, bool) {
	type scored struct {
		c Candidate
		d float64
	}

	list := make([]scored, 0, len(candidates))
	for _, c := range candidates {
		list = append(list, scored{c: c, d: geo.DistanceM(start, c.Point)})
	}
	if len(list) == 0 {
		return Candidate{}, false
	}

	sort.Slice(list, func(i, j int) bool { return list[i].d > list[j].d })

	// Aus dem entferntesten Viertel eines nehmen, statt immer das allerweiteste:
	// Sonst läge das Fluchtziel bei jedem Spiel in derselben Ecke der Stadt.
	quarter := len(list) / 4
	if quarter < 1 {
		quarter = 1
	}
	return list[quarter/2].c, true
}
