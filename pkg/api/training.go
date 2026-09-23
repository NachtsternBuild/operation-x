package api

import (
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/schema"
	"github.com/elias/operation-x/pkg/sim"
)

// runner steuert die Trockenübung. Eine Instanz je Serverlauf.
var runner *sim.Runner

// SetRunner hinterlegt den Simulator. Wird beim Serverstart aufgerufen.
func SetRunner(r *sim.Runner) { runner = r }

type trainingRequest struct {
	Detectives  int   `json:"detectives"`
	WithMisterX *bool `json:"withMisterX"`
}

// handleTrainingStatus meldet, ob die Trockenübung dieses Spiels läuft.
func handleTrainingStatus(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}
	if runner == nil {
		return e.JSON(http.StatusOK, map[string]any{"running": false})
	}
	return e.JSON(http.StatusOK, runner.Status(gameRec.Id))
}

// handleTrainingStart setzt simulierte Spieler in Bewegung.
//
// Sie laufen durch dasselbe Spiel wie echte Teilnehmer und unterliegen
// denselben Regeln – nur dass niemand dafür vor die Tür muss. Gedacht zum
// Üben vor dem Spieltag und zum Prüfen von Regeländerungen.
func handleTrainingStart(e *core.RequestEvent) error {
	if runner == nil {
		return e.InternalServerError("Die Trockenübung steht nicht bereit.", nil)
	}

	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}
	if gameRec.GetString("status") != schema.GameRunning {
		return e.BadRequestError(
			"Die Trockenübung braucht ein laufendes Spiel. Zuerst im Regelpult starten.", nil)
	}

	var req trainingRequest
	_ = e.BindBody(&req)

	// Ohne ausdrückliche Angabe wird die Zielperson mitsimuliert – wer allein
	// übt, will genau das.
	withX := true
	if req.WithMisterX != nil {
		withX = *req.WithMisterX
	}

	if err := runner.Start(gameRec.Id, req.Detectives, withX); err != nil {
		return e.BadRequestError(err.Error(), nil)
	}

	return e.JSON(http.StatusOK, runner.Status(gameRec.Id))
}

// handleTrainingStop hält die Bots an, lässt sie aber im Spiel.
func handleTrainingStop(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}
	if runner == nil {
		return e.JSON(http.StatusOK, map[string]any{"running": false})
	}
	runner.Stop(gameRec.Id)
	return e.JSON(http.StatusOK, map[string]any{"running": false})
}

// handleTrainingCleanup entfernt die simulierten Teams wieder.
func handleTrainingCleanup(e *core.RequestEvent) error {
	if runner == nil {
		return e.JSON(http.StatusOK, map[string]any{"removed": 0})
	}

	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	removed, err := runner.Cleanup(gameRec.Id)
	if err != nil {
		return e.InternalServerError("Aufräumen fehlgeschlagen.", err)
	}

	return e.JSON(http.StatusOK, map[string]any{"removed": removed})
}

// --- Einweisung ---------------------------------------------------------------

// handleOnboarded vermerkt, dass ein Team die Einweisung durchlaufen hat.
func handleOnboarded(e *core.RequestEvent) error {
	// Frisch laden statt e.Auth zu speichern: Sonst schriebe diese Anfrage den
	// Punktestand zurück, den sie beim Anmelden gesehen hat, und machte jede
	// Buchung rückgängig, die inzwischen dazwischenkam. Siehe die Fallstricke
	// im README.
	fresh, err := e.App.FindRecordById(schema.ColTeams, e.Auth.Id)
	if err != nil {
		return e.InternalServerError("Das Team wurde nicht gefunden.", err)
	}

	fresh.Set("onboarded_at", time.Now())
	if err := e.App.Save(fresh); err != nil {
		return e.InternalServerError("Konnte nicht gespeichert werden.", err)
	}

	return e.JSON(http.StatusOK, map[string]any{"ok": true})
}

// handleReadiness zeigt der Zentrale, wer eingewiesen ist und wer nicht.
//
// Vor dem Start die wichtigste Liste überhaupt: Ein Team, das die Oberfläche
// zum ersten Mal auf der Straße sieht, verliert die erste halbe Stunde.
func handleReadiness(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	teams, err := e.App.FindRecordsByFilter(schema.ColTeams,
		"game = {:g} && active = true", "role,callsign", 0, 0,
		map[string]any{"g": gameRec.Id})
	if err != nil {
		return err
	}

	type entry struct {
		ID        string `json:"id"`
		Callsign  string `json:"callsign"`
		Display   string `json:"display"`
		Role      string `json:"role"`
		Bot       bool   `json:"bot"`
		Onboarded bool   `json:"onboarded"`
		SeenAgo   int    `json:"seenAgoSec"`
		HasFix    bool   `json:"hasFix"`
	}

	now := time.Now()
	out := make([]entry, 0, len(teams))

	for _, t := range teams {
		item := entry{
			ID:        t.Id,
			Callsign:  t.GetString("callsign"),
			Display:   t.GetString("display"),
			Role:      t.GetString("role"),
			Bot:       t.GetBool("bot"),
			Onboarded: !t.GetDateTime("onboarded_at").Time().IsZero(),
			SeenAgo:   -1,
		}

		if pos := latestPosition(e.App, t.Id); pos != nil {
			item.HasFix = true
			item.SeenAgo = int(now.Sub(pos.GetDateTime("captured_at").Time()).Seconds())
		}

		out = append(out, item)
	}

	return e.JSON(http.StatusOK, map[string]any{"teams": out})
}
