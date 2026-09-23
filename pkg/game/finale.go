package game

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/schema"
)

// Radius, auf den der Suchbereich im Finale zusammenschrumpft.
//
// Er beginnt großzügig und wird alle paar Minuten enger. Am Ende ist er so
// klein, dass die Zielperson gefunden werden muss – ein Finale, das im Sande
// verläuft, ist für beide Seiten das schlechteste Ergebnis.
const (
	finaleStartRadiusM = 1500
	finaleMinRadiusM   = 250
	finaleShrinkM      = 250
)

// FinishGame beendet das Spiel und hält fest, wer gewonnen hat.
func FinishGame(app core.App, gameRec *core.Record, winner, reason string) error {
	if gameRec.GetString("status") == schema.GameFinished {
		return nil // schon entschieden
	}

	gameRec.Set("status", schema.GameFinished)
	gameRec.Set("winner", winner)
	gameRec.Set("win_reason", reason)
	gameRec.Set("ended_at", time.Now())

	if err := app.Save(gameRec); err != nil {
		return err
	}

	if _, err := Book(app, Booking{
		Game:      gameRec.Id,
		Type:      EventGameOver,
		Reason:    reason,
		DedupeKey: "game.over:" + gameRec.Id,
		Payload:   map[string]any{"sieger": winner},
	}); err != nil {
		return err
	}

	// Ab hier läuft die Uhr für die Standortdaten. Genau hier, weil das
	// Spielende der einzige Zeitpunkt ist, an dem sicher feststeht, dass die
	// Spur nicht mehr gebraucht wird.
	return SchedulePurge(app, gameRec)
}

// checkFinale steuert den Endspurt.
//
// Sobald Mister X sein letztes Zwischenziel abgeschlossen hat, wird der
// Suchbereich um das Fluchtziel offengelegt und schrumpft danach fortlaufend.
// Ab hier gibt es kein Verstecken mehr, nur noch Verfolgung.
func (e *Engine) checkFinale(gameRec *core.Record, cfg Config, now time.Time) error {
	planned := gameRec.GetInt("planned_missions")
	if planned <= 0 {
		return nil
	}

	done, err := e.app.FindRecordsByFilter(
		schema.ColMissions, "game = {:g} && status = 'done'", "", 0, 0,
		map[string]any{"g": gameRec.Id},
	)
	if err != nil {
		return err
	}

	active := gameRec.GetBool("final_active")

	// Auslösen, sobald alle geplanten Zwischenziele abgearbeitet sind.
	if !active {
		if len(done) < planned {
			return nil
		}

		gameRec.Set("final_active", true)
		gameRec.Set("final_started_at", now)
		gameRec.Set("final_radius_m", float64(finaleStartRadiusM))
		if err := e.app.Save(gameRec); err != nil {
			return err
		}

		_, err := Book(e.app, Booking{
			Game:      gameRec.Id,
			Type:      EventFinaleStart,
			Reason:    "Letztes Zwischenziel abgeschlossen – das Finale beginnt",
			DedupeKey: "finale.start:" + gameRec.Id,
			Payload:   map[string]any{"radius_m": finaleStartRadiusM},
		})
		return err
	}

	// Läuft bereits: schrittweise enger ziehen.
	started := gameRec.GetDateTime("final_started_at").Time()
	if started.IsZero() {
		return nil
	}

	interval := time.Duration(cfg.FinalShrinkIntervalMin) * time.Minute
	if interval <= 0 {
		interval = 5 * time.Minute
	}

	steps := int(now.Sub(started) / interval)
	wanted := float64(finaleStartRadiusM - steps*finaleShrinkM)
	if wanted < finaleMinRadiusM {
		wanted = finaleMinRadiusM
	}

	if current := gameRec.GetFloat("final_radius_m"); wanted < current {
		gameRec.Set("final_radius_m", wanted)
		if err := e.app.Save(gameRec); err != nil {
			return err
		}

		_, err := Book(e.app, Booking{
			Game:      gameRec.Id,
			Type:      EventFinaleShrink,
			Reason:    fmt.Sprintf("Suchbereich auf %.0f m eingegrenzt", wanted),
			DedupeKey: fmt.Sprintf("finale.shrink:%s:%d", gameRec.Id, steps),
			Payload:   map[string]any{"radius_m": wanted},
		})
		return err
	}

	return nil
}

// checkVictory prüft die beiden Wege, auf denen das Spiel von selbst endet.
func (e *Engine) checkVictory(gameRec *core.Record, cfg Config, now time.Time) error {
	// 1. Mister X hat sein Fluchtziel erreicht.
	if finalID := gameRec.GetString("final_target"); finalID != "" && gameRec.GetBool("final_active") {
		misterX, err := e.app.FindFirstRecordByFilter(schema.ColTeams,
			"game = {:g} && role = {:r}",
			map[string]any{"g": gameRec.Id, "r": schema.RoleMisterX})
		if err == nil && misterX != nil {
			if reached, err := e.reachedFinalTarget(gameRec, misterX, finalID, cfg); err == nil && reached {
				if _, err := Book(e.app, Booking{
					Game: gameRec.Id, Team: misterX.Id,
					Type:        EventEscape,
					Reason:      "Fluchtziel erreicht",
					DeltaPoints: cfg.PointsVictory,
					DedupeKey:   "escape:" + gameRec.Id,
				}); err != nil {
					return err
				}
				return FinishGame(e.app, gameRec, "misterx", "Die Zielperson hat das Fluchtziel erreicht")
			}
		}
	}

	// 2. Die Spielzeit ist abgelaufen – dann entscheidet der Punktestand.
	ends := gameRec.GetDateTime("ends_at").Time()
	if ends.IsZero() || now.Before(ends) {
		return nil
	}

	teams, err := e.app.FindRecordsByFilter(schema.ColTeams, "game = {:g}", "", 0, 0,
		map[string]any{"g": gameRec.Id})
	if err != nil {
		return err
	}

	var xPoints, detectivePoints int
	for _, t := range teams {
		switch t.GetString("role") {
		case schema.RoleMisterX:
			xPoints += t.GetInt("points")
		case schema.RoleDetective:
			detectivePoints += t.GetInt("points")
		}
	}

	winner := "draw"
	reason := fmt.Sprintf("Spielzeit abgelaufen – unentschieden bei %d zu %d Punkten", xPoints, detectivePoints)

	switch {
	case xPoints > detectivePoints:
		winner = "misterx"
		reason = fmt.Sprintf("Spielzeit abgelaufen – die Zielperson führt mit %d zu %d Punkten", xPoints, detectivePoints)
	case detectivePoints > xPoints:
		winner = "detectives"
		reason = fmt.Sprintf("Spielzeit abgelaufen – die Fahndung führt mit %d zu %d Punkten", detectivePoints, xPoints)
	}

	return FinishGame(e.app, gameRec, winner, reason)
}

// reachedFinalTarget prüft, ob Mister X am Fluchtziel angekommen ist.
//
// Anders als bei einem Zwischenziel braucht es dafür keinen Nachweis: Wer im
// Finale am Ziel steht, während die Fahndung im Nacken sitzt, hat es geschafft.
// Alles andere hieße, den entscheidenden Moment an einem Foto-Upload aufzuhängen.
func (e *Engine) reachedFinalTarget(gameRec, misterX *core.Record, finalID string, cfg Config) (bool, error) {
	target, err := e.app.FindRecordById(schema.ColHotspots, finalID)
	if err != nil {
		return false, err
	}

	pos := lastPosition(e.app, misterX.Id)
	if pos == nil {
		return false, nil
	}

	// Die Meldung muss frisch sein, sonst gewänne jemand durch Stillstand im
	// Funkloch.
	if time.Since(pos.GetDateTime("captured_at").Time()) > 5*time.Minute {
		return false, nil
	}

	d := distanceBetween(pos, target)
	return d <= cfg.HotspotMaxDistanceM, nil
}
