package api

import (
	"fmt"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/game"
	"github.com/elias/operation-x/pkg/schema"
	"github.com/elias/operation-x/pkg/seed"
)

// Zugänge ausgeben.
//
// Der Konzeptschritt "Zugänge erzeugen" endete bisher im Nichts: Kennwörter
// lassen sich nicht auslesen – sie liegen als Prüfsumme in der Datenbank, und
// das ist auch richtig so. Wer also eine Teamkarte drucken wollte, musste jedes
// Kennwort beim Anlegen mitschreiben und später abtippen.
//
// Deshalb dieser Weg: Die Zentrale vergibt auf Knopfdruck neue, sprechbare
// Kennwörter, bekommt sie genau einmal zurück und druckt daraus die Karten. Ab
// dann sind sie auch für den Server wieder unlesbar.

type credentialCard struct {
	ID       string `json:"id"`
	Callsign string `json:"callsign"`
	Display  string `json:"display"`
	Role     string `json:"role"`
	Color    string `json:"color"`
	Password string `json:"password,omitempty"`
}

// handleIssueCredentials vergibt neue Kennwörter und gibt sie einmalig aus.
func handleIssueCredentials(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	teams, err := e.App.FindRecordsByFilter(
		schema.ColTeams, "game = {:g} && bot != true", "role,callsign", 0, 0,
		map[string]any{"g": gameRec.Id},
	)
	if err != nil {
		return err
	}

	cards := make([]credentialCard, 0, len(teams))
	for _, team := range teams {
		password, err := seed.SpeakablePassword()
		if err != nil {
			return e.InternalServerError("Kennwort konnte nicht erzeugt werden.", err)
		}

		team.SetPassword(password)
		if err := e.App.Save(team); err != nil {
			return e.InternalServerError(
				"Kennwort für "+team.GetString("callsign")+" ließ sich nicht setzen.", err)
		}

		cards = append(cards, credentialCard{
			ID:       team.Id,
			Callsign: team.GetString("callsign"),
			Display:  team.GetString("display"),
			Role:     team.GetString("role"),
			Color:    team.GetString("color"),
			Password: password,
		})
	}

	// Ins Protokoll, ohne die Kennwörter: Dass alle Zugänge neu vergeben
	// wurden, ist eine Tatsache, die man später nachlesen können muss – der
	// Inhalt nicht.
	_, _ = game.Book(e.App, game.Booking{
		Game:   gameRec.Id,
		Type:   "hq.credentials_issued",
		Reason: fmt.Sprintf("Neue Kennwörter für %d Zugänge vergeben", len(cards)),
	})

	return e.JSON(http.StatusOK, map[string]any{
		"cards":   cards,
		"joinUrl": publicURL(e),
	})
}

// handleTeamCards liefert die Teams ohne Kennwörter – für die Vorschau, bevor
// jemand die Zugänge tatsächlich neu vergibt.
func handleTeamCards(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	teams, err := e.App.FindRecordsByFilter(
		schema.ColTeams, "game = {:g} && bot != true", "role,callsign", 0, 0,
		map[string]any{"g": gameRec.Id},
	)
	if err != nil {
		return err
	}

	cards := make([]credentialCard, 0, len(teams))
	for _, team := range teams {
		cards = append(cards, credentialCard{
			ID:       team.Id,
			Callsign: team.GetString("callsign"),
			Display:  team.GetString("display"),
			Role:     team.GetString("role"),
			Color:    team.GetString("color"),
		})
	}

	return e.JSON(http.StatusOK, map[string]any{
		"cards":   cards,
		"joinUrl": publicURL(e),
	})
}
