package game

import (
	"math/rand"
	"sort"

	"github.com/elias/operation-x/pkg/geo"
)

// Die Auslosung der Startpunkte.
//
// Am Anfang steht eine Frage, die sich am Spieltag nicht von selbst löst: Wo
// fängt wer an? Wer sie den Leuten überlässt, bekommt entweder eine
// Diskussion oder – schlimmer – alle an derselben Straßenbahnhaltestelle. Und
// wenn die Zielperson und ein Fahndungsteam zufällig am selben Ort starten,
// ist das Spiel in der ersten Minute entschieden, ohne dass jemand etwas
// richtig oder falsch gemacht hätte.
//
// Deshalb: gezogen wird, und jeder Punkt nur einmal.
//
// Reiner Zufall ohne Zurücklegen würde die Doppelung ausschließen, aber nicht
// den Fall "zwei Punkte, dreihundert Meter auseinander". Deshalb wird ein
// Mindestabstand angestrebt – angestrebt, nicht erzwungen: Wenn das
// Spielgebiet klein ist oder viele Teams mitspielen, gibt es ihn irgendwann
// nicht mehr, und dann ist ein enger Start besser als gar keiner.

// StartCandidate ist ein Punkt, der gezogen werden kann.
type StartCandidate struct {
	HotspotID string
	Number    int
	Name      string
	Point     geo.Point
}

// StartDraw ordnet einem Team seinen Startpunkt zu.
type StartDraw struct {
	TeamID    string
	HotspotID string
	Number    int
	Name      string
}

// StartMinDistanceM ist der angestrebte Abstand zwischen zwei Startpunkten.
//
// Fünfhundert Meter sind rund sechs Minuten Fußweg: nah genug, dass alle im
// selben Viertel anfangen, weit genug, dass niemand den anderen beim Loslaufen
// zusehen kann.
const StartMinDistanceM = 500

// DrawStarts verteilt Startpunkte auf Teams.
//
// [teamIDs] in der Reihenfolge, in der gezogen wird – die Zielperson gehört
// nach vorn, denn sie hat die größere Auswahl verdient, wenn es eng wird.
// Liefert nichts, wenn es weniger Punkte als Teams gibt; darüber entscheidet
// der Aufrufer, denn das ist eine Frage an die Spielleitung und kein Fehler.
func DrawStarts(candidates []StartCandidate, teamIDs []string, rnd *rand.Rand) []StartDraw {
	if len(candidates) < len(teamIDs) || len(teamIDs) == 0 {
		return nil
	}

	frei := make([]StartCandidate, len(candidates))
	copy(frei, candidates)
	rnd.Shuffle(len(frei), func(i, j int) { frei[i], frei[j] = frei[j], frei[i] })

	out := make([]StartDraw, 0, len(teamIDs))
	vergeben := make([]geo.Point, 0, len(teamIDs))

	for _, teamID := range teamIDs {
		gewaehlt := -1

		// Erst einen mit Abstand suchen …
		for i, c := range frei {
			if weitGenug(c.Point, vergeben, StartMinDistanceM) {
				gewaehlt = i
				break
			}
		}
		// … und wenn es keinen gibt, den nächstbesten nehmen. Ein enger Start
		// ist besser als ein Team ohne Startpunkt.
		if gewaehlt < 0 {
			gewaehlt = 0
		}

		c := frei[gewaehlt]
		frei = append(frei[:gewaehlt], frei[gewaehlt+1:]...)
		vergeben = append(vergeben, c.Point)

		out = append(out, StartDraw{
			TeamID:    teamID,
			HotspotID: c.HotspotID,
			Number:    c.Number,
			Name:      c.Name,
		})
	}

	return out
}

func weitGenug(p geo.Point, andere []geo.Point, minM float64) bool {
	for _, o := range andere {
		if geo.DistanceM(p, o) < minM {
			return false
		}
	}
	return true
}

// SortStarts bringt die Auslosung in eine lesbare Reihenfolge: nach
// Hotspot-Nummer, wie sie auch auf dem Kartenblatt steht.
func SortStarts(draws []StartDraw) {
	sort.Slice(draws, func(i, j int) bool { return draws[i].Number < draws[j].Number })
}
