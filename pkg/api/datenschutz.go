package api

import (
	"fmt"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/game"
	"github.com/elias/operation-x/pkg/schema"
)

// Die Bedienung für das, was das Konzept zusagt.
//
// Die Bewegungsspur löscht sich ohnehin von selbst. Zwei Fälle kann eine
// Frist aber nicht abdecken, und beide kommen vor:
//
//   - Jemand zieht seine Einwilligung zurück, mitten im Spiel. Dann muss
//     seine Spur weg, und zwar jetzt (Art. 7 Abs. 3, Art. 17 DSGVO).
//   - Die Spielleitung will nach dem Ausklang nicht darauf hoffen, dass der
//     Laptop am nächsten Tag noch einmal läuft.
//
// Ohne diese Endpunkte bliebe für beides nur die Datenbankverwaltung — und
// die ist für jemanden, der eine Jugendgruppe betreut, keine Antwort.

type spurfristRequest struct {
	Hours int `json:"hours"`
}

// handleSetRetention ändert die Frist für die Bewegungsspur.
//
// Null bedeutet "sofort nach Spielende", nicht "nie" – das Konzept nennt
// diesen Wert ausdrücklich. Gespeichert wird er als -1, weil ein leeres Feld
// in der Datenbank ebenfalls null ergibt und dann die Voreinstellung von 24
// Stunden gälte.
func handleSetRetention(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	var req spurfristRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}
	if req.Hours < 0 || req.Hours > 7*24 {
		return e.BadRequestError(
			"Die Frist muss zwischen sofort (0) und sieben Tagen liegen.", nil)
	}

	gespeichert := req.Hours
	if gespeichert == 0 {
		gespeichert = -1
	}
	gameRec.Set("retention_hours", gespeichert)
	if err := e.App.Save(gameRec); err != nil {
		return e.InternalServerError("Die Frist ließ sich nicht ändern.", err)
	}

	// Ist das Spiel schon vorbei, läuft die neue Frist ab sofort – sonst
	// stünde eine geänderte Frist erst beim nächsten Spielende zur Wirkung.
	if !gameRec.GetDateTime("ended_at").IsZero() {
		if err := game.SchedulePurge(e.App, gameRec); err != nil {
			return e.InternalServerError("Die Frist ließ sich nicht neu setzen.", err)
		}
	}

	_, _ = game.Book(e.App, game.Booking{
		Game:   gameRec.Id,
		Type:   "game.retention",
		Reason: fmt.Sprintf("Aufbewahrung der Standortdaten auf %d Stunden gesetzt", req.Hours),
	})

	return e.JSON(http.StatusOK, map[string]any{"hours": req.Hours})
}

// handlePurgeNow löscht die Bewegungsspur dieses Spiels sofort.
func handlePurgeNow(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	if err := game.PurgeNow(e.App, gameRec); err != nil {
		return e.InternalServerError("Die Standortdaten ließen sich nicht löschen.", err)
	}

	return e.JSON(http.StatusOK, map[string]any{
		"geloescht": true,
		"purgedAt":  gameRec.GetDateTime("purged_at").String(),
	})
}

// handleDeleteTeam entfernt ein Team vollständig.
//
// Der Weg für einen Widerruf: Mit dem Team fällt alles, was an ihm hängt –
// Positionen, Meldungen, Buchungen, Funksprüche. Die Datenbank erledigt das
// selbst, weil diese Datensätze das Team als Pflichtfeld führen.
//
// Deshalb auch keine Rückfrage im Server: Wer hier hinkommt, hat in der
// Oberfläche bereits bestätigt, und ein zweites "wirklich?" an dieser Stelle
// hülfe niemandem.
func handleDeleteTeam(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	team, err := ausSpiel(e, schema.ColTeams, e.Request.PathValue("id"), gameRec.Id)
	if err != nil {
		return err
	}

	// Die eigene Zentrale darf sich nicht selbst abschaffen – danach käme
	// niemand mehr an das Spiel heran.
	if team.GetString("role") == schema.RoleHQ {
		return e.BadRequestError(
			"Die Einsatzzentrale lässt sich nicht löschen. Sonst wäre das Spiel "+
				"herrenlos.", nil)
	}

	rufzeichen := team.GetString("callsign")
	if err := e.App.Delete(team); err != nil {
		return e.InternalServerError("Das Team ließ sich nicht löschen.", err)
	}

	_, _ = game.Book(e.App, game.Booking{
		Game:   gameRec.Id,
		Type:   "team.deleted",
		Reason: fmt.Sprintf("Team %s gelöscht, samt allen zugehörigen Daten", rufzeichen),
	})

	return e.JSON(http.StatusOK, map[string]any{"geloescht": rufzeichen})
}
