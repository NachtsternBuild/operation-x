package game

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/geo"
	"github.com/elias/operation-x/pkg/schema"
)

// Anti-Camping.
//
// Das Problem, gegen das die Regel sich richtet: Ein Fahndungsteam, das sich
// an den Hauptbahnhof setzt und wartet, spielt nicht – es sitzt das Spiel aus.
// Dasselbe gilt für eine Zielperson, die sich in einem Hinterhof verschanzt.
//
// Entscheidend ist, was als "Camping" zählt. Zwei naheliegende Definitionen
// sind beide für sich genommen unfair:
//
//   - Nur nach Spielhandlungen zu gehen bestraft, wer zwanzig Minuten quer
//     durch die Stadt läuft. Das ist der anstrengendste Teil des Spiels.
//   - Nur nach Bewegung zu gehen bestraft, wer an einem Ort steht und
//     konzentriert ein Rätsel löst.
//
// Deshalb zählt hier beides zusammen: Camping ist, wer *weder* handelt *noch*
// sich bewegt. Wer eines von beidem tut, ist im Spiel.
//
// Die Folge ist bewusst mild und wiederholt sich: ein Fluchtpunkt je Intervall,
// nicht ein Schlag. Die Regel soll aufscheuchen, nicht erledigen.

// MarkAction hält fest, dass ein Team etwas getan hat.
//
// Aufzurufen an jeder Stelle, die eine echte Spielhandlung darstellt – nicht
// bei einer Standortmeldung, denn die läuft automatisch und wäre damit ein
// Freibrief fürs Stillsitzen.
func MarkAction(app core.App, teamID string) {
	if teamID == "" {
		return
	}
	team, err := app.FindRecordById(schema.ColTeams, teamID)
	if err != nil || team == nil {
		return
	}
	team.Set("last_action_at", time.Now())
	// Ein Fehler hier darf die Spielhandlung selbst nicht scheitern lassen:
	// Lieber eine verpasste Camping-Messung als ein abgelehnter Zugriff.
	_ = app.Save(team)
}

// checkCamping bucht die Folgen für Teams, die weder handeln noch sich bewegen.
func (e *Engine) checkCamping(gameRec *core.Record, cfg Config, now time.Time) error {
	teams, err := e.app.FindRecordsByFilter(
		schema.ColTeams, "game = {:g} && role != {:hq}", "", 0, 0,
		map[string]any{"g": gameRec.Id, "hq": schema.RoleHQ},
	)
	if err != nil {
		return err
	}

	for _, team := range teams {
		role := team.GetString("role")

		window := campingWindow(cfg, role)
		if window <= 0 {
			continue
		}

		// Die Ausnahme des Regelwerks: "Mister X im Stillstand außerhalb von
		// Mission und Transit."
		//
		// Sie fehlte, und das drehte die Regel um. Solange ein Zwischenziel
		// läuft, misst bereits die Frist, ob sich jemand bewegt – wer zu lange
		// steht, verliert die Mission. Noch einmal für dasselbe zu zahlen wäre
		// doppelt, und zwar an der Stelle, an der die Zielperson gerade
		// arbeitet: an einem Ziel anstehen, ein Foto einreichen, auf die
		// Bahn warten.
		//
		// Übrig bleibt genau der Fall, den die Regel meint: keine Mission
		// offen, kein Transit – und trotzdem passiert nichts.
		if role == schema.RoleMisterX {
			if team.GetBool("in_transit") {
				continue
			}
			if aktiv, err := e.missionRunning(gameRec.Id); err != nil {
				return err
			} else if aktiv {
				continue
			}
		}

		// Der Bezugspunkt ist die letzte Handlung, ersatzweise der Spielstart.
		// Ohne Ersatz wäre ein Team, das noch nie etwas getan hat, von der
		// ersten Minute an im Verzug.
		since := team.GetDateTime("last_action_at").Time()
		if since.IsZero() {
			since = gameRec.GetDateTime("starts_at").Time()
		}
		if since.IsZero() || now.Sub(since) < window {
			continue
		}

		moved, err := e.movedWithin(team.Id, now.Add(-window), cfg.CampingRadiusM)
		if err != nil {
			return err
		}
		if moved {
			continue
		}

		if err := e.bookCamping(gameRec, team, cfg, now, window); err != nil {
			return err
		}
	}

	return nil
}

// missionRunning sagt, ob gerade ein Zwischenziel läuft.
func (e *Engine) missionRunning(gameID string) (bool, error) {
	offen, err := e.app.FindRecordsByFilter(
		schema.ColMissions,
		"game = {:g} && status = 'active'",
		"", 1, 0,
		map[string]any{"g": gameID},
	)
	if err != nil {
		return false, err
	}
	return len(offen) > 0, nil
}

func campingWindow(cfg Config, role string) time.Duration {
	switch role {
	case schema.RoleDetective:
		return time.Duration(cfg.CampingDetectiveMin) * time.Minute
	case schema.RoleMisterX:
		return time.Duration(cfg.CampingMisterXMin) * time.Minute
	default:
		return 0
	}
}

// movedWithin sagt, ob sich ein Team seit einem Zeitpunkt nennenswert bewegt
// hat. Maßstab ist die größte Entfernung zur ältesten Meldung im Fenster –
// nicht die Summe der Schritte, denn GPS-Rauschen summiert sich auch im Stehen
// auf mehrere hundert Meter.
func (e *Engine) movedWithin(teamID string, from time.Time, radiusM float64) (bool, error) {
	positions, err := e.app.FindRecordsByFilter(
		schema.ColPositions,
		"team = {:t} && captured_at >= {:from}",
		"captured_at", 0, 0,
		map[string]any{"t": teamID, "from": DBTime(from)},
	)
	if err != nil {
		return false, err
	}
	// Ohne Meldungen im Fenster lässt sich nichts feststellen. Im Zweifel
	// nicht bestrafen – das Ausbleiben der Meldungen ahndet bereits die
	// Ping-Kaskade, und zweimal für dasselbe zu zahlen wäre unbillig.
	if len(positions) < 2 {
		return true, nil
	}

	first := geo.Point{
		Lat: positions[0].GetFloat("lat"),
		Lng: positions[0].GetFloat("lng"),
	}
	for _, p := range positions[1:] {
		here := geo.Point{Lat: p.GetFloat("lat"), Lng: p.GetFloat("lng")}
		if geo.DistanceM(first, here) > radiusM {
			return true, nil
		}
	}
	return false, nil
}

func (e *Engine) bookCamping(
	gameRec, team *core.Record, cfg Config, now time.Time, window time.Duration,
) error {
	role := team.GetString("role")

	fp := cfg.CampingDetectiveFP
	if role == schema.RoleMisterX {
		fp = cfg.CampingMisterXFP
	}

	minutes := int(window.Minutes())
	einheit := "Minuten"
	if minutes == 1 {
		einheit = "Minute"
	}
	reason := fmt.Sprintf("Seit %d %s weder Handlung noch Bewegung", minutes, einheit)

	// Der Schlüssel enthält das Zeitfenster, nicht den Zeitpunkt: Damit bucht
	// jeder angebrochene Abschnitt genau einmal, egal wie oft die Engine in
	// dieser Zeit läuft.
	slot := now.Truncate(window).Unix()
	booked, err := Book(e.app, Booking{
		Game:      gameRec.Id,
		Team:      team.Id,
		Type:      EventCamping,
		Reason:    reason,
		DeltaFP:   fp,
		DedupeKey: fmt.Sprintf("camping:%s:%d", team.Id, slot),
		Payload:   map[string]any{"minuten": minutes},
	})
	if err != nil || !booked {
		return err
	}

	// Steht die Zielperson still, ist das für die Fahndung eine Information
	// wert: Alle Fahndungsteams bekommen den Bonus gutgeschrieben.
	if role == schema.RoleMisterX && cfg.CampingMisterXBonus != 0 {
		hunters, err := e.app.FindRecordsByFilter(
			schema.ColTeams, "game = {:g} && role = {:r}", "", 0, 0,
			map[string]any{"g": gameRec.Id, "r": schema.RoleDetective},
		)
		if err != nil {
			return err
		}
		for _, hunter := range hunters {
			if _, err := Book(e.app, Booking{
				Game:        gameRec.Id,
				Team:        hunter.Id,
				Type:        EventCamping,
				Reason:      "Die Zielperson hat sich lange nicht bewegt",
				DeltaPoints: cfg.CampingMisterXBonus,
				DedupeKey:   fmt.Sprintf("camping.bonus:%s:%d", hunter.Id, slot),
			}); err != nil {
				return err
			}
		}
	}

	return nil
}
