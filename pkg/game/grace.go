package game

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/geo"
	"github.com/elias/operation-x/pkg/schema"
)

// Kulanzzeit.
//
// Das Regelwerk setzt Fristen, damit die Wahl der Route eine Entscheidung ist:
// Der schnelle Weg bringt mehr Punkte und weniger Luft. Diese Rechnung geht
// aber nur auf, solange die Frist misst, was sie messen soll – nämlich, ob
// jemand rechtzeitig losgelaufen ist.
//
// Sie misst etwas anderes, sobald der Auftrag am Ziel selbst Zeit kostet:
// "Kauf zwei Tüten Gummibärchen" dauert eine Minute oder zwölf, je nachdem,
// wie viele Leute vor der Kasse stehen. Das hat mit der Entscheidung, die
// bestraft werden soll, nichts zu tun. Wer am Ziel steht und ansteht, hat
// alles richtig gemacht.
//
// Deshalb: Steht die Zielperson beim Ablauf der Frist am Ziel, wird die Frist
// verlängert statt gerissen. Begrenzt, protokolliert, und die Fahndung erfährt
// davon nichts – für sie ändert sich nichts, außer dass ein Zwangs-Ping
// ausbleibt, den es ohne die Warteschlange auch nicht gegeben hätte.
//
// Die Grenze ist wichtig: Ohne sie wäre "am Ziel stehen" ein Freibrief, die
// Uhr anzuhalten. Mit ihr bleibt es das, was es sein soll – Nachsicht für eine
// Verzögerung, die niemand zu verantworten hat.

// GraceReason unterscheidet, woher die Kulanz kam. Beide landen im Protokoll,
// aber sie beantworten unterschiedliche Fragen der Spielleitung.
const (
	GraceOnSite    = "vor Ort beschäftigt"
	GraceReported  = "Verzögerung gemeldet"
	GraceGrantedHQ = "von der Zentrale gewährt"
)

// GraceState beschreibt, wie viel Nachsicht eine Mission schon erfahren hat.
type GraceState struct {
	UsedMin int  `json:"usedMin"`
	MaxMin  int  `json:"maxMin"`
	Left    int  `json:"leftMin"`
	Active  bool `json:"active"`
}

// GraceOf liest den Stand einer Mission.
func GraceOf(mission *core.Record, cfg Config) GraceState {
	used := mission.GetInt("grace_used_min")
	return GraceState{
		UsedMin: used,
		MaxMin:  cfg.MissionGraceMaxMin,
		Left:    max(0, cfg.MissionGraceMaxMin-used),
		Active:  !mission.GetDateTime("grace_last_at").Time().IsZero(),
	}
}

// AtTarget sagt, ob ein Team nah genug am Ziel der Mission steht.
//
// Derselbe Abstand wie für den Nachweis: Wer den Vor-Ort-Code eingeben dürfte,
// steht auch nah genug, um in der Schlange zu stehen.
func AtTarget(app core.App, mission, team *core.Record, cfg Config) bool {
	target, err := app.FindRecordById(schema.ColHotspots, mission.GetString("target"))
	if err != nil || target == nil {
		return false
	}

	pos := lastPosition(app, team.Id)
	if pos == nil {
		return false
	}

	distance := geo.DistanceM(
		geo.Point{Lat: pos.GetFloat("lat"), Lng: pos.GetFloat("lng")},
		geo.Point{Lat: target.GetFloat("lat"), Lng: target.GetFloat("lng")},
	)
	return distance <= cfg.HotspotMaxDistanceM+pos.GetFloat("accuracy")
}

// GrantGrace verlängert die Frist einer Mission.
//
// Liefert die gewährten Minuten zurück; null heißt, dass nichts mehr übrig war.
// Der Aufrufer entscheidet daraufhin, ob die Mission scheitert.
func GrantGrace(
	app core.App, gameRec, mission, team *core.Record,
	cfg Config, minutes int, reason string, now time.Time,
) (int, error) {
	if minutes <= 0 {
		return 0, nil
	}

	used := mission.GetInt("grace_used_min")
	left := cfg.MissionGraceMaxMin - used
	if left <= 0 {
		return 0, nil
	}
	if minutes > left {
		minutes = left
	}

	// Die Frist wandert vom späteren der beiden Zeitpunkte aus: Wer eine
	// Verzögerung meldet, bevor die Frist abgelaufen ist, soll die Minuten
	// obendrauf bekommen und nicht einen Teil davon verschenken.
	base := mission.GetDateTime("deadline_at").Time()
	if base.Before(now) {
		base = now
	}

	mission.Set("deadline_at", base.Add(time.Duration(minutes)*time.Minute))
	mission.Set("grace_used_min", used+minutes)
	mission.Set("grace_last_at", now)
	mission.Set("grace_reason", reason)

	if err := app.Save(mission); err != nil {
		return 0, fmt.Errorf("Kulanzzeit speichern: %w", err)
	}

	// Ohne Punkte und ohne Fluchtpunkte: Das hier ist keine Strafe und keine
	// Belohnung, sondern eine Korrektur an der Messung.
	_, err := Book(app, Booking{
		Game: gameRec.Id, Team: team.Id,
		Type: EventMissionGrace,
		Reason: fmt.Sprintf("Frist um %d Minuten verlängert (%s)",
			minutes, reason),
		DedupeKey: fmt.Sprintf("grace:%s:%d", mission.Id, used+minutes),
		Payload: map[string]any{
			"minuten": minutes,
			"grund":   reason,
			"gesamt":  used + minutes,
		},
	})
	if err != nil {
		return 0, err
	}

	return minutes, nil
}
