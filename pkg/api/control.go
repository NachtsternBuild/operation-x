package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/game"
	"github.com/elias/operation-x/pkg/schema"
)

type statusRequest struct {
	Status string `json:"status"`

	// Nur beim Pausieren: Wofür, und für wie lange geplant.
	//
	// Eine Pause ohne Ansage ist für die, die draußen stehen, von einer
	// Störung nicht zu unterscheiden – und genau dann fangen sie an, auf ihren
	// Geräten herumzudrücken. "Mittagessen, 60 Minuten" beantwortet beides.
	Reason  string `json:"reason,omitempty"`
	Minutes int    `json:"minutes,omitempty"`
}

// handleSetStatus startet, pausiert oder beendet das Spiel.
//
// Die Pause ist kein Komfort, sondern Teil des Regelwerks: Bei Bahnausfällen
// oder Verkehrschaos setzt das HQ die Uhr an, und in dieser Zeit werden keine
// Fristen geahndet. Die Engine prüft nur laufende Spiele, deshalb genügt das
// Umschalten des Zustands.
func handleSetStatus(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	var req statusRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}

	allowed := map[string]bool{
		schema.GameSetup:    true,
		schema.GameReady:    true,
		schema.GameRunning:  true,
		schema.GamePaused:   true,
		schema.GameFinished: true,
	}
	if !allowed[req.Status] {
		return e.BadRequestError("Unbekannter Spielzustand.", nil)
	}

	previous := gameRec.GetString("status")
	now := time.Now()

	// Beim Start die Fristen aller Teams neu setzen: Sonst wäre die erste
	// Meldung sofort überfällig, weil die Frist noch aus der Vorbereitung
	// stammt.
	if req.Status == schema.GameRunning && previous != schema.GameRunning {
		cfg := game.ConfigOf(gameRec)

		teams, err := e.App.FindRecordsByFilter(
			schema.ColTeams, "game = {:g}", "", 0, 0,
			map[string]any{"g": gameRec.Id},
		)
		if err != nil {
			return err
		}

		for _, team := range teams {
			if team.GetString("role") == schema.RoleHQ {
				continue
			}
			team.Set("ping_due_at", now.Add(cfg.PingInterval(team.GetBool("in_transit"))))
			if err := e.App.Save(team); err != nil {
				return err
			}
		}

		// Die Spieluhr beginnt hier, nicht bei der Anlage.
		//
		// Ein bei der Vorbereitung eingetragener Termin ist eine Verabredung,
		// keine Uhrzeit: Wer für 14 Uhr einlädt und um 14:20 tatsächlich
		// startet, hat zwanzig Minuten weniger Spiel, wenn man den Plan als
		// Wahrheit nimmt. Schlimmer noch, ein in der Zukunft liegender Start
		// lässt die halbe Engine ins Leere greifen – der Spielfortschritt
		// bliebe bei null, und das Anti-Camping fände keinen Bezugspunkt.
		if previous != schema.GamePaused {
			gameRec.Set("starts_at", now)

			minutes := gameRec.GetInt("duration_min")
			if minutes <= 0 {
				minutes = defaultDurationMin
			}
			gameRec.Set("ends_at", now.Add(time.Duration(minutes)*time.Minute))
		}
	}

	// Beim Pausieren den Zeitpunkt festhalten – gleich wird er gebraucht.
	if req.Status == schema.GamePaused && previous != schema.GamePaused {
		gameRec.Set("paused_at", now)
		gameRec.Set("pause_reason", Kuerzen(strings.TrimSpace(req.Reason), 120))

		// Die geplante Dauer ist eine Ansage, keine Schaltuhr: Das Spiel läuft
		// nicht von selbst wieder an. Wer zwanzig Minuten später noch beim
		// Nachtisch sitzt, soll nicht plötzlich wieder unter Frist stehen.
		if req.Minutes > 0 && req.Minutes <= 480 {
			gameRec.Set("pause_until", now.Add(time.Duration(req.Minutes)*time.Minute))
		} else {
			gameRec.Set("pause_until", "")
		}
	}

	// Nach einer Pause bekommen alle Teams die volle Frist erneut – die Zeit
	// stand still, also darf sie niemandem angelastet werden.
	if previous == schema.GamePaused && req.Status == schema.GameRunning {
		cfg := game.ConfigOf(gameRec)

		// Aus demselben Grund wandert das Spielende um die Dauer der Pause
		// nach hinten. Ohne das fräße ein halbstündiger Bahnausfall eine
		// halbe Stunde Spielzeit – und zwar von allen.
		if pausedAt := gameRec.GetDateTime("paused_at").Time(); !pausedAt.IsZero() {
			pause := now.Sub(pausedAt)

			if ends := gameRec.GetDateTime("ends_at").Time(); !ends.IsZero() {
				gameRec.Set("ends_at", ends.Add(pause))
			}

			// Und die Frist des laufenden Zwischenziels genauso.
			//
			// Ohne das fräße jede Pause die Missionszeit auf: Die Zentrale hält
			// wegen eines Bahnausfalls zwanzig Minuten an, und die Zielperson
			// steht danach vor einer Frist, die zwanzig Minuten kürzer ist –
			// bestraft für eine Unterbrechung, die sie nicht zu verantworten
			// hat. Bei langen Pausen wäre das Zwischenziel schon beim
			// Fortsetzen verloren.
			if mission, err := openMission(e.App, gameRec.Id); err == nil && mission != nil {
				if deadline := mission.GetDateTime("deadline_at").Time(); !deadline.IsZero() {
					mission.Set("deadline_at", deadline.Add(pause))
					if err := e.App.Save(mission); err != nil {
						return e.InternalServerError("Die Missionsfrist ließ sich nicht nachführen.", err)
					}
				}
			}

			gameRec.Set("paused_at", "")
			gameRec.Set("pause_reason", "")
			gameRec.Set("pause_until", "")
		}

		teams, err := e.App.FindRecordsByFilter(
			schema.ColTeams, "game = {:g}", "", 0, 0,
			map[string]any{"g": gameRec.Id},
		)
		if err != nil {
			return err
		}
		for _, team := range teams {
			if team.GetString("role") == schema.RoleHQ {
				continue
			}
			team.Set("ping_due_at", now.Add(cfg.PingInterval(team.GetBool("in_transit"))))
			if err := e.App.Save(team); err != nil {
				return err
			}
		}
	}

	gameRec.Set("status", req.Status)

	// Beendet die Zentrale das Spiel von Hand, muss die Aufbewahrungsfrist
	// genauso laufen wie nach einem regulären Sieg. Ohne diesen Zweig bliebe
	// die Bewegungsspur genau der Spiele liegen, die nicht zu Ende gespielt
	// wurden – und das dürften die meisten sein.
	if req.Status == schema.GameFinished && previous != schema.GameFinished {
		// Wann es zu Ende war, gehört festgehalten, gleich wie es endete.
		// Daran hängen die Aufbewahrungsfristen – und die Frage, seit wann ein
		// Spiel eigentlich herumliegt.
		if gameRec.GetDateTime("ended_at").Time().IsZero() {
			gameRec.Set("ended_at", now)
		}
		if err := game.SchedulePurge(e.App, gameRec); err != nil {
			return e.InternalServerError("Löschfrist konnte nicht gesetzt werden.", err)
		}
	} else if err := e.App.Save(gameRec); err != nil {
		return e.InternalServerError("Spielzustand konnte nicht gesetzt werden.", err)
	}

	_, _ = game.Book(e.App, game.Booking{
		Game:   gameRec.Id,
		Type:   "game." + req.Status,
		Reason: "Spielzustand von " + previous + " auf " + req.Status,
	})

	return e.JSON(http.StatusOK, map[string]any{
		"status": req.Status,
		"since":  now.UTC().Format(time.RFC3339),
	})
}

type eventEntry struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Team        string         `json:"team,omitempty"`
	Callsign    string         `json:"callsign,omitempty"`
	Reason      string         `json:"reason,omitempty"`
	DeltaPoints int            `json:"deltaPoints"`
	DeltaFP     int            `json:"deltaFp"`
	OccurredAt  string         `json:"occurredAt"`
	Payload     map[string]any `json:"payload,omitempty"`
}

// handleEvents liefert das Protokoll – die Zeitleiste der Einsatzzentrale.
//
// Jede Buchung ist hier nachlesbar, mitsamt der Regel, die sie ausgelöst hat.
// Das beantwortet am Spieltag die häufigste Frage überhaupt: „Warum habe ich
// minus fünfzehn?“
func handleEvents(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	records, err := e.App.FindRecordsByFilter(
		schema.ColEvents, "game = {:g}", "-occurred_at", 200, 0,
		map[string]any{"g": gameRec.Id},
	)
	if err != nil {
		return err
	}

	// Rufzeichen einmal nachschlagen statt je Ereignis.
	names := map[string]string{}
	if teams, err := e.App.FindRecordsByFilter(
		schema.ColTeams, "game = {:g}", "", 0, 0,
		map[string]any{"g": gameRec.Id},
	); err == nil {
		for _, t := range teams {
			names[t.Id] = t.GetString("callsign")
		}
	}

	out := make([]eventEntry, 0, len(records))
	for _, r := range records {
		entry := eventEntry{
			ID:          r.Id,
			Type:        r.GetString("type"),
			Team:        r.GetString("team"),
			Reason:      r.GetString("reason"),
			DeltaPoints: r.GetInt("delta_points"),
			DeltaFP:     r.GetInt("delta_fp"),
			OccurredAt:  r.GetDateTime("occurred_at").Time().UTC().Format(time.RFC3339),
		}
		entry.Callsign = names[entry.Team]

		if raw := toRaw(r.Get("payload")); len(raw) > 0 {
			var payload map[string]any
			if err := jsonUnmarshal(raw, &payload); err == nil {
				entry.Payload = payload
			}
		}

		out = append(out, entry)
	}

	return e.JSON(http.StatusOK, map[string]any{"events": out})
}

// --- Spieldauer ändern -----------------------------------------------------

type dauerRequest struct {
	Minutes int `json:"minutes"`
}

// handleSetDuration ändert die Spieldauer, auch mitten im Spiel.
//
// Der häufigste Satz eines Spieltags ist "wir hängen noch eine Stunde dran" –
// und bis hierher gab es dafür keinen Weg: Die Dauer wurde beim Anlegen
// gesetzt und danach nie wieder.
//
// Läuft das Spiel schon, wandert das Spielende um die Differenz, statt neu
// gerechnet zu werden. Der Unterschied ist wichtig: Eine Pause hat das Ende
// bereits nach hinten geschoben, und diese Verschiebung darf eine
// Dauerkorrektur nicht stillschweigend zurücknehmen.
func handleSetDuration(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	var req dauerRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}
	if req.Minutes < 30 || req.Minutes > 24*60 {
		return e.BadRequestError(
			"Die Spieldauer muss zwischen 30 Minuten und 24 Stunden liegen.", nil)
	}

	vorher := gameRec.GetInt("duration_min")
	if vorher <= 0 {
		vorher = defaultDurationMin
	}
	unterschied := time.Duration(req.Minutes-vorher) * time.Minute

	gameRec.Set("duration_min", req.Minutes)
	if ends := gameRec.GetDateTime("ends_at").Time(); !ends.IsZero() {
		gameRec.Set("ends_at", ends.Add(unterschied))
	}

	if err := e.App.Save(gameRec); err != nil {
		return e.InternalServerError("Die Spieldauer ließ sich nicht ändern.", err)
	}

	_, _ = game.Book(e.App, game.Booking{
		Game:   gameRec.Id,
		Type:   "game.duration",
		Reason: fmt.Sprintf("Spieldauer von %d auf %d Minuten geändert", vorher, req.Minutes),
	})

	res := map[string]any{"durationMin": req.Minutes}
	if ends := gameRec.GetDateTime("ends_at").Time(); !ends.IsZero() {
		res["endsAt"] = ends.UTC().Format(time.RFC3339)
	}
	return e.JSON(http.StatusOK, res)
}
