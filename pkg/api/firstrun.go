package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/schema"
	"github.com/elias/operation-x/pkg/seed"
)

// Die Ersteinrichtung.
//
// Ein frisch entpackter Server hat eine leere Datenbank: kein Spiel, kein
// Zugang, niemand, der sich anmelden könnte. Ohne diesen Weg müsste die
// Spielleitung auf der Kommandozeile ein Testspiel erzeugen – genau das, was
// laut Anforderung niemand können muss.
//
// Der Endpunkt ist öffentlich, aber nur solange es noch keine Einsatzzentrale
// gibt. Danach ist er tot: Wer später Zugänge braucht, legt sie im Regelpult
// an, wo mitprotokolliert wird, wer was getan hat.

const (
	setupCallsign = "HQ"

	// Vier Stunden: lang genug für eine vollständige Route durch eine Stadt,
	// kurz genug, dass niemand vorher aufgibt.
	defaultDurationMin = 240
)

type setupRequest struct {
	Name        string `json:"name"`
	City        string `json:"city"`
	Password    string `json:"password"`
	DurationMin int    `json:"durationMin"`
}

// setupNeeded meldet, ob dieser Server noch nie eingerichtet wurde.
//
// Ausschlaggebend ist der Zugang, nicht das Spiel: Ein Spiel ohne Zugang wäre
// so unerreichbar wie gar keines. Solange es keine Einsatzzentrale gibt, steht
// die Tür offen.
func setupNeeded(app core.App) bool {
	if EinrichtungOffen != nil {
		if offen, uebernommen := EinrichtungOffen(app); uebernommen {
			return offen
		}
	}

	records, err := app.FindRecordsByFilter(
		schema.ColTeams, "role = {:r}", "", 1, 0,
		map[string]any{"r": schema.RoleHQ},
	)
	if err != nil {
		// Im Zweifel nichts anbieten: Ein fälschlich offener Endpunkt wäre
		// schlimmer als eine Einrichtung, die man wiederholen muss.
		return false
	}
	return len(records) == 0
}

// handleSetup legt das erste Spiel und den Zugang der Einsatzzentrale an.
func handleSetup(e *core.RequestEvent) error {
	if !setupNeeded(e.App) {
		return e.ForbiddenError(
			"Die Einrichtung ist bereits erfolgt. Weitere Zugänge legt die "+
				"Einsatzzentrale im Regelpult an.", nil)
	}

	var req setupRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}

	req.Name = strings.TrimSpace(req.Name)
	req.City = strings.TrimSpace(req.City)

	if req.Name == "" {
		return e.BadRequestError("Das Spiel braucht einen Namen.", nil)
	}
	if req.City == "" {
		return e.BadRequestError("Das Spiel braucht eine Stadt.", nil)
	}
	// Ohne Dauer gäbe es kein Spielende nach Zeit – und damit nur noch den
	// Zugriff als Ausgang. Eine Voreinstellung ist hier besser als ein Feld,
	// das leer bleiben darf.
	if req.DurationMin <= 0 {
		req.DurationMin = defaultDurationMin
	}
	if req.DurationMin < 30 || req.DurationMin > 24*60 {
		return e.BadRequestError(
			"Die Spieldauer muss zwischen 30 Minuten und 24 Stunden liegen.", nil)
	}
	// Acht Zeichen, weil dieses Kennwort am Spieltag vorgelesen und abgetippt
	// wird. Länger hilft hier niemandem, kürzer ist zu wenig.
	if len(req.Password) < 8 {
		return e.BadRequestError("Das Kennwort braucht mindestens acht Zeichen.", nil)
	}

	var team *core.Record

	err := e.App.RunInTransaction(func(tx core.App) error {
		// Der zweite Aufruf darf nicht auch noch durchgehen, wenn zwei Browser
		// gleichzeitig auf der Einrichtungsseite stehen.
		if !setupNeeded(tx) {
			return fmt.Errorf("die Einrichtung ist bereits erfolgt")
		}

		// Ein Spiel kann schon dastehen, etwa aus "operationx seed". Dann
		// bekommt es den Zugang, statt daneben ein zweites zu erzeugen –
		// currentGame nähme ohnehin nur eines von beiden.
		game, err := currentGame(tx)
		if err != nil {
			return err
		}
		if game == nil {
			gamesCol, err := tx.FindCollectionByNameOrId(schema.ColGames)
			if err != nil {
				return err
			}

			game = core.NewRecord(gamesCol)
			game.Set("name", req.Name)
			game.Set("city", req.City)
			game.Set("status", schema.GameSetup)
			game.Set("duration_min", req.DurationMin)
			game.Set("retention_hours", 24)
			game.Set("final_active", false)
			game.Set("config", seed.DefaultConfig())
			// Das Spielgebiet bleibt offen: Es ergibt sich aus den Sektoren,
			// die als Nächstes gewählt werden, und wird dort gesetzt.
			if err := tx.Save(game); err != nil {
				return fmt.Errorf("Spiel anlegen: %w", err)
			}
		}

		teamsCol, err := tx.FindCollectionByNameOrId(schema.ColTeams)
		if err != nil {
			return err
		}

		team = core.NewRecord(teamsCol)
		team.Set("callsign", setupCallsign)
		team.Set("display", "Einsatzzentrale")
		team.Set("role", schema.RoleHQ)
		team.Set("color", "#d9a441")
		team.Set("game", game.Id)
		team.Set("fp", 0)
		team.Set("points", 0)
		team.Set("active", true)
		team.Set("in_transit", false)
		team.SetPassword(req.Password)

		if err := tx.Save(team); err != nil {
			return fmt.Errorf("Zugang anlegen: %w", err)
		}
		return nil
	})
	if err != nil {
		return e.BadRequestError(err.Error(), nil)
	}

	// Das Kennwort geht bewusst nicht zurück: Es kam von dieser Seite und steht
	// dort noch im Formular. Zurückgeschickt landete es zusätzlich im Protokoll
	// jedes Zwischenstücks.
	return e.JSON(http.StatusOK, map[string]any{
		"callsign":    setupCallsign,
		"durationMin": req.DurationMin,
	})
}
