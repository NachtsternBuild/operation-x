package game

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/elias/operation-x/pkg/geo"
)

// Hinweiskategorien nach Regelwerk.
const (
	IntelGreen  = "green"  // verifizierte Tatsache
	IntelYellow = "yellow" // Bereichs- oder Sektoreingrenzung
	IntelRed    = "red"    // taktische Beobachtung
)

// Frischestufen. Ein Hinweis altert, und wie alt er ist, entscheidet darüber,
// ob er noch etwas wert ist.
const (
	FreshHot  = "hot"
	FreshWarm = "warm"
	FreshCold = "cold"
)

// Freshness stuft einen Hinweis nach seinem Alter ein.
func Freshness(occurredAt time.Time, cfg Config, now time.Time) string {
	age := now.Sub(occurredAt)

	if age < time.Duration(cfg.IntelHotMin)*time.Minute {
		return FreshHot
	}
	if age < time.Duration(cfg.IntelWarmMin)*time.Minute {
		return FreshWarm
	}
	return FreshCold
}

// Trace ist die Bewegungsspur, aus der Hinweise abgeleitet werden.
type Trace struct {
	// Positionen, jüngste zuerst.
	Points []TracePoint
	// Zuletzt bestätigt besuchter Hotspot.
	LastHotspot     string
	LastHotspotNo   int
	LastHotspotTime time.Time
	// Sektor, in dem sich Mister X aktuell aufhält.
	SectorCode string
	SectorName string
	// Benachbarte Sektoren, für die unscharfe Eingrenzung.
	NeighborCodes []string

	// Was schon auf der Hinweistafel steht.
	//
	// Ohne dieses Gedächtnis liefert jedes gelöste Rätsel denselben Satz,
	// solange Mister X im selben Sektor bleibt: "Mister X befindet sich in
	// Sektor N3", dreimal untereinander. Für die Fahndung sieht das aus, als
	// wäre etwas kaputt – und die Belohnung fürs Lösen ist verpufft, obwohl
	// die Auskunft stimmt.
	Recent []string
}

// erstesNeues nimmt die erste Formulierung, die noch nicht auf der Tafel steht.
//
// Bleibt keine übrig, wird gezählt: "zum dritten Mal in Folge" ist eine Aussage,
// die es vorher nicht gab, und sie sagt der Fahndung etwas Wahres – dass sich
// nichts bewegt. [vorlage] bekommt dafür die laufende Nummer.
func (t Trace) erstesNeues(varianten []string, vorlage string) string {
	for _, v := range varianten {
		if v != "" && !t.kennt(v) {
			return v
		}
	}

	for n := 2; n < 60; n++ {
		text := fmt.Sprintf(vorlage, n)
		if !t.kennt(text) {
			return text
		}
	}
	return fmt.Sprintf(vorlage, 60)
}

// kennt sagt, ob dieser Satz schon auf der Tafel steht.
func (t Trace) kennt(text string) bool {
	for _, seen := range t.Recent {
		if seen == text {
			return true
		}
	}
	return false
}

// TracePoint ist eine Position mit Zeitstempel.
type TracePoint struct {
	At    time.Time
	Point geo.Point
	Speed float64
}

// Intel ist ein erzeugter Hinweis.
type Intel struct {
	Category   string
	Text       string
	SectorID   string
	HotspotID  string
	OccurredAt time.Time
	Fabricated bool
}

// GenerateIntel erzeugt einen Hinweis der gewünschten Kategorie aus der Lage.
//
// Erzeugt wird erst im Moment der Freischaltung, nicht vorab: Wer ein Rätsel
// schnell löst, bekommt einen frischen Hinweis, wer lange braucht, einen alten.
// Damit ist Tempo eine eigene Belohnung, ohne dass es dafür eine Extraregel
// braucht.
func GenerateIntel(category string, trace Trace, now time.Time) (Intel, bool) {
	if len(trace.Points) == 0 {
		return Intel{}, false
	}

	switch category {
	case IntelGreen:
		return greenIntel(trace, now)
	case IntelYellow:
		return yellowIntel(trace, now)
	case IntelRed:
		return redIntel(trace, now)
	default:
		return yellowIntel(trace, now)
	}
}

// greenIntel nennt eine gesicherte Tatsache: wo Mister X wann war.
func greenIntel(trace Trace, now time.Time) (Intel, bool) {
	if trace.LastHotspot != "" && !trace.LastHotspotTime.IsZero() {
		text := fmt.Sprintf("Mister X war um %s Uhr an Hotspot #%02d.",
			clockOf(trace.LastHotspotTime), trace.LastHotspotNo)

		// Steht der Satz schon da, ist die Nachricht eine andere – nämlich
		// dass sich nichts getan hat. Das ist für die Fahndung durchaus etwas
		// wert: Wer seit einer halben Stunde keinen neuen Punkt bestätigt hat,
		// hängt fest oder macht einen Umweg.
		text = trace.erstesNeues(
			[]string{
				text,
				fmt.Sprintf("Stand %s Uhr: Hotspot #%02d (%s Uhr) ist weiterhin der letzte bestätigte Punkt.",
					clockOf(now), trace.LastHotspotNo, clockOf(trace.LastHotspotTime)),
			},
			fmt.Sprintf("Zum %%d. Mal in Folge ohne neuen Punkt: Zuletzt bestätigt ist Hotspot #%02d.",
				trace.LastHotspotNo),
		)

		return Intel{
			Category:   IntelGreen,
			Text:       text,
			HotspotID:  trace.LastHotspot,
			OccurredAt: trace.LastHotspotTime,
		}, true
	}

	// Noch kein bestätigter Hotspot: Dann ist die gesicherte Tatsache der
	// Sektor zu einem genauen Zeitpunkt.
	p := trace.Points[0]
	if trace.SectorCode == "" {
		return Intel{}, false
	}

	// Auch hier: Wiederholt sich der Satz, ist die eigentliche Auskunft, dass
	// sich nichts geändert hat. Dieser Zweig fehlte beim ersten Anlauf, und
	// prompt standen im Versuch zwei wortgleiche grüne Hinweise untereinander.
	return Intel{
		Category: IntelGreen,
		Text: trace.erstesNeues(
			[]string{
				fmt.Sprintf("Mister X war um %s Uhr in Sektor %s (%s).",
					clockOf(p.At), trace.SectorCode, trace.SectorName),
				fmt.Sprintf("Auch um %s Uhr noch in Sektor %s (%s).",
					clockOf(p.At), trace.SectorCode, trace.SectorName),
			},
			fmt.Sprintf("Zum %%d. Mal bestätigt: Sektor %s (%s), zuletzt um "+clockOf(p.At)+" Uhr.",
				trace.SectorCode, trace.SectorName),
		),
		OccurredAt: p.At,
	}, true
}

// yellowIntel grenzt einen Bereich ein, ohne ihn festzunageln.
func yellowIntel(trace Trace, now time.Time) (Intel, bool) {
	p := trace.Points[0]

	if trace.SectorCode == "" {
		return Intel{
			Category:   IntelYellow,
			Text:       "Mister X hält sich außerhalb der markierten Sektoren auf.",
			OccurredAt: p.At,
		}, true
	}

	// Mit einem Nachbarsektor zusammen genannt: Die Eingrenzung stimmt, ist
	// aber nicht eindeutig – genau das meint „wahrscheinlich“.
	if len(trace.NeighborCodes) > 0 {
		// Erst die Paarungen durchgehen, die noch nicht auf der Tafel stehen:
		// Zwei verschiedene Paare mit demselben Sektor darin grenzen die Lage
		// zusammen enger ein als eine Wiederholung.
		varianten := make([]string, 0, len(trace.NeighborCodes)+1)
		for _, other := range trace.NeighborCodes {
			varianten = append(varianten, paarSatz(trace.SectorCode, other))
		}

		gewaehlt := trace.NeighborCodes[rand.Intn(len(trace.NeighborCodes))]
		codes := []string{trace.SectorCode, gewaehlt}
		sort.Strings(codes)
		varianten = append(varianten, fmt.Sprintf(
			"Um %s Uhr hält sich Mister X weiterhin in Sektor %s oder %s auf.",
			clockOf(now), codes[0], codes[1]))

		return Intel{
			Category: IntelYellow,
			Text: trace.erstesNeues(varianten, fmt.Sprintf(
				"Zum %%d. Mal in Folge dieselbe Eingrenzung: Sektor %s oder %s.",
				codes[0], codes[1])),
			OccurredAt: p.At,
		}, true
	}

	return Intel{
		Category: IntelYellow,
		Text: trace.erstesNeues(
			[]string{
				fmt.Sprintf("Mister X befindet sich in Sektor %s (%s).", trace.SectorCode, trace.SectorName),
				fmt.Sprintf("Um %s Uhr ist Mister X immer noch in Sektor %s (%s).",
					clockOf(now), trace.SectorCode, trace.SectorName),
			},
			fmt.Sprintf("Zum %%d. Mal in Folge: Sektor %s (%s).", trace.SectorCode, trace.SectorName),
		),
		OccurredAt: p.At,
	}, true
}

// paarSatz formuliert die unscharfe Eingrenzung auf zwei Sektoren.
func paarSatz(a, b string) string {
	codes := []string{a, b}
	sort.Strings(codes)
	return fmt.Sprintf("Mister X befindet sich in Sektor %s oder %s.", codes[0], codes[1])
}

// redIntel liest eine Eigenschaft aus dem Bewegungsmuster ab.
//
// Das ist die schwächste Hinweisart und zugleich die, aus der sich am meisten
// machen lässt: Sie sagt nicht, wo jemand ist, sondern wie er sich verhält.
func redIntel(trace Trace, now time.Time) (Intel, bool) {
	p := trace.Points[0]
	observations := []string{}

	// Fortbewegungsart aus der Geschwindigkeit.
	if speed, ok := averageSpeedKmh(trace.Points); ok {
		switch {
		case speed > 22:
			observations = append(observations, "Mister X bewegt sich schneller als zu Fuß möglich – vermutlich Bahn oder Bus.")
		case speed > 9:
			observations = append(observations, "Mister X kommt zügig voran, vermutlich mit dem Rad oder im Nahverkehr.")
		case speed < 1.5:
			observations = append(observations, "Mister X bewegt sich seit einer Weile kaum – er hält sich irgendwo auf.")
		default:
			observations = append(observations, "Mister X ist zu Fuß unterwegs.")
		}
	}

	// Grobe Himmelsrichtung der letzten Bewegung.
	if dir, ok := bearingLabel(trace.Points); ok {
		observations = append(observations, fmt.Sprintf("Die letzte Bewegung führte nach %s.", dir))
	}

	// Ob er sich innerhalb oder außerhalb der Sektoren hält.
	if trace.SectorCode == "" {
		observations = append(observations, "Mister X meidet derzeit die markierten Sektoren.")
	}

	if len(observations) == 0 {
		return Intel{}, false
	}

	// Von den möglichen Beobachtungen zuerst eine nehmen, die noch nicht auf
	// der Tafel steht.
	frisch := make([]string, 0, len(observations))
	for _, o := range observations {
		if !trace.kennt(o) {
			frisch = append(frisch, o)
		}
	}

	text := ""
	if len(frisch) > 0 {
		text = frisch[rand.Intn(len(frisch))]
	} else {
		// Alles schon gesagt. Dann ist die Auskunft, dass es dabei geblieben
		// ist – und die Zählung macht sie zu einer, die es so noch nicht gab.
		// Der Satz bleibt, wie er ist: Er fängt mit einem Namen an, und
		// "Stand 13:51 Uhr: mister X bewegt sich …" liest sich wie ein Fehler.
		kern := strings.TrimSuffix(observations[0], ".")
		text = trace.erstesNeues(
			[]string{fmt.Sprintf("Stand %s Uhr — %s.", clockOf(now), kern)},
			"Zum %d. Mal dieselbe Beobachtung — "+kern+".",
		)
	}

	return Intel{
		Category:   IntelRed,
		Text:       text,
		OccurredAt: p.At,
	}, true
}

// averageSpeedKmh mittelt die Geschwindigkeit über die letzten Meldungen.
func averageSpeedKmh(points []TracePoint) (float64, bool) {
	if len(points) < 2 {
		return 0, false
	}

	// Höchstens die letzten zehn Minuten betrachten – was davor war, sagt über
	// die aktuelle Fortbewegung nichts mehr.
	cutoff := points[0].At.Add(-10 * time.Minute)

	var meters float64
	var seconds float64

	for i := 0; i < len(points)-1; i++ {
		newer, older := points[i], points[i+1]
		if older.At.Before(cutoff) {
			break
		}
		d := newer.At.Sub(older.At).Seconds()
		if d <= 0 {
			continue
		}
		meters += geo.DistanceM(newer.Point, older.Point)
		seconds += d
	}

	if seconds < 60 {
		return 0, false
	}
	return (meters / seconds) * 3.6, true
}

// bearingLabel beschreibt die Richtung der letzten Bewegung in Worten.
func bearingLabel(points []TracePoint) (string, bool) {
	if len(points) < 2 {
		return "", false
	}

	from := points[len(points)-1].Point
	to := points[0].Point

	// Unter 150 Metern ist eine Richtungsangabe nur GPS-Rauschen.
	if geo.DistanceM(from, to) < 150 {
		return "", false
	}

	dLat := to.Lat - from.Lat
	dLng := (to.Lng - from.Lng) * math.Cos(from.Lat*math.Pi/180)

	angle := math.Atan2(dLng, dLat) * 180 / math.Pi
	if angle < 0 {
		angle += 360
	}

	labels := []string{"Norden", "Nordosten", "Osten", "Südosten", "Süden", "Südwesten", "Westen", "Nordwesten"}
	idx := int((angle+22.5)/45) % 8

	return labels[idx], true
}

// FabricateIntel erzeugt einen gefälschten Hinweis für die Täuschungs-Joker.
//
// Er ist vom selben Bautyp wie ein echter und deshalb an der Formulierung nicht
// zu erkennen – für die Detektive äußerlich ununterscheidbar, im Protokoll für
// die Zentrale klar markiert.
func FabricateIntel(category string, sectorCodes []string, now time.Time) (Intel, bool) {
	switch category {
	case IntelYellow:
		if len(sectorCodes) < 2 {
			return Intel{}, false
		}
		a := sectorCodes[rand.Intn(len(sectorCodes))]
		b := sectorCodes[rand.Intn(len(sectorCodes))]
		for b == a {
			b = sectorCodes[rand.Intn(len(sectorCodes))]
		}
		codes := []string{a, b}
		sort.Strings(codes)

		return Intel{
			Category:   IntelYellow,
			Text:       fmt.Sprintf("Mister X befindet sich in Sektor %s oder %s.", codes[0], codes[1]),
			OccurredAt: now,
			Fabricated: true,
		}, true

	case IntelRed:
		claims := []string{
			"Mister X bewegt sich schneller als zu Fuß möglich – vermutlich Bahn oder Bus.",
			"Die letzte Bewegung führte nach Norden.",
			"Die letzte Bewegung führte nach Süden.",
			"Die letzte Bewegung führte nach Osten.",
			"Die letzte Bewegung führte nach Westen.",
			"Mister X meidet derzeit die markierten Sektoren.",
		}
		return Intel{
			Category:   IntelRed,
			Text:       claims[rand.Intn(len(claims))],
			OccurredAt: now,
			Fabricated: true,
		}, true

	default:
		return Intel{}, false
	}
}

func clockOf(t time.Time) string {
	return t.Local().Format("15:04")
}

// NormalizeAnswer bereitet eine Rätselantwort für den Vergleich auf.
//
// Am Handy, im Gehen, mit klammen Fingern getippt: Groß- und Kleinschreibung,
// doppelte Leerzeichen und die Frage, ob jemand „Straße“ oder „Strasse“
// schreibt, dürfen nicht über richtig und falsch entscheiden.
func NormalizeAnswer(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))

	replacer := strings.NewReplacer(
		"ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss",
		"-", " ", "_", " ", ".", "", ",", "", "!", "", "?", "",
	)
	s = replacer.Replace(s)

	return strings.Join(strings.Fields(s), " ")
}

// AnswerMatches vergleicht eine Eingabe mit der hinterlegten Lösung.
//
// Mehrere zulässige Lösungen werden mit einem senkrechten Strich getrennt
// hinterlegt – „Kreuzkirche|Kreuzkirche Dresden“ akzeptiert beides.
func AnswerMatches(input, expected string) bool {
	got := NormalizeAnswer(input)
	if got == "" {
		return false
	}

	for _, variant := range strings.Split(expected, "|") {
		if want := NormalizeAnswer(variant); want != "" && got == want {
			return true
		}
	}
	return false
}
