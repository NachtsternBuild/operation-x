package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/game"
	"github.com/elias/operation-x/pkg/geo"
	"github.com/elias/operation-x/pkg/schema"
)

type sightingResponse struct {
	ID        string  `json:"id"`
	Confirmed bool    `json:"confirmed"`
	DwellSec  int     `json:"dwellSec"`
	DistanceM float64 `json:"distanceM"`
	Message   string  `json:"message"`
}

// handleSighting meldet einen Sichtkontakt.
//
// Ein Sichtkontakt ist eine Behauptung, die etwas kostet: Mister X erfährt
// sofort, dass er gesehen wurde – aber nicht von wem – und kann darauf
// reagieren. Deshalb wird er nicht sofort anerkannt, sondern erst, wenn das
// Team eine Weile in seiner Nähe bleibt. Wer nur im Vorbeifahren auf den Knopf
// drückt, warnt die Zielperson, ohne etwas davon zu haben.
func handleSighting(e *core.RequestEvent) error {
	me := e.Auth
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}
	cfg := game.ConfigOf(gameRec)

	myPos := latestPosition(e.App, me.Id)
	if myPos == nil {
		return e.BadRequestError("Es liegt keine Standortmeldung vor.", nil)
	}

	misterX, err := e.App.FindFirstRecordByFilter(schema.ColTeams,
		"game = {:g} && role = {:r}",
		map[string]any{"g": gameRec.Id, "r": schema.RoleMisterX})
	if err != nil || misterX == nil {
		return e.BadRequestError("In diesem Spiel gibt es keine Zielperson.", nil)
	}

	xPos := latestPosition(e.App, misterX.Id)
	if xPos == nil {
		return e.BadRequestError("Von der Zielperson liegt keine Meldung vor.", nil)
	}

	here := geo.Point{Lat: myPos.GetFloat("lat"), Lng: myPos.GetFloat("lng")}
	there := geo.Point{Lat: xPos.GetFloat("lat"), Lng: xPos.GetFloat("lng")}
	distance := geo.DistanceM(here, there)

	// Ein laufender, noch unbestätigter Sichtkontakt desselben Teams wird
	// fortgeschrieben statt verdoppelt.
	existing, _ := e.App.FindFirstRecordByFilter(schema.ColSightings,
		"team = {:t} && confirmed = false", map[string]any{"t": me.Id})

	now := time.Now()
	allowed := cfg.SightingMaxDistanceM + myPos.GetFloat("accuracy")

	if distance > allowed {
		// Die Behauptung war falsch. Das kostet nichts, warnt aber auch
		// niemanden – Mister X erfährt davon nichts.
		if existing != nil {
			_ = e.App.Delete(existing)
		}
		return e.JSON(http.StatusOK, sightingResponse{
			Confirmed: false,
			DistanceM: distance,
			Message:   "Niemand in Sichtweite. Der Kontakt wurde nicht gemeldet.",
		})
	}

	if existing == nil {
		col, err := e.App.FindCollectionByNameOrId(schema.ColSightings)
		if err != nil {
			return err
		}

		rec := core.NewRecord(col)
		rec.Set("game", gameRec.Id)
		rec.Set("team", me.Id)
		rec.Set("lat", here.Lat)
		rec.Set("lng", here.Lng)
		rec.Set("distance_m", distance)
		rec.Set("reported_at", now)
		rec.Set("confirmed", false)
		rec.Set("method", "dwell")

		if err := e.App.Save(rec); err != nil {
			return e.InternalServerError("Sichtkontakt konnte nicht gespeichert werden.", err)
		}

		// Der Alarm geht sofort raus – bewusst schon vor der Bestätigung, denn
		// die Zielperson soll die Chance haben, sich abzusetzen. Welches Team
		// sie gesehen hat, erfährt sie nicht.
		_, _ = game.Book(e.App, game.Booking{
			Game: gameRec.Id, Team: misterX.Id,
			Type:      game.EventSighting,
			Reason:    "Sichtkontakt gemeldet",
			DedupeKey: "sighting:" + rec.Id,
		})

		return e.JSON(http.StatusOK, sightingResponse{
			ID:        rec.Id,
			Confirmed: false,
			DistanceM: distance,
			Message: fmt.Sprintf("Kontakt gemeldet. %d Sekunden in Sichtweite bleiben, dann gilt er als bestätigt.",
				cfg.SightingDwellSec),
		})
	}

	// Fortschreibung: Reicht die Verweildauer?
	dwell := int(now.Sub(existing.GetDateTime("reported_at").Time()).Seconds())
	if dwell < cfg.SightingDwellSec {
		return e.JSON(http.StatusOK, sightingResponse{
			ID:        existing.Id,
			Confirmed: false,
			DwellSec:  dwell,
			DistanceM: distance,
			Message:   fmt.Sprintf("Noch %d Sekunden.", cfg.SightingDwellSec-dwell),
		})
	}

	existing.Set("confirmed", true)
	existing.Set("confirmed_at", now)
	existing.Set("distance_m", distance)
	if err := e.App.Save(existing); err != nil {
		return e.InternalServerError("Bestätigung fehlgeschlagen.", err)
	}

	_, _ = game.Book(e.App, game.Booking{
		Game: gameRec.Id, Team: me.Id,
		Type:      game.EventSightingOK,
		Reason:    "Sichtkontakt bestätigt",
		DedupeKey: "sighting.ok:" + existing.Id,
		Payload:   map[string]any{"abstand_m": int(distance), "dauer_s": dwell},
	})

	return e.JSON(http.StatusOK, sightingResponse{
		ID:        existing.Id,
		Confirmed: true,
		DwellSec:  dwell,
		DistanceM: distance,
		Message:   "Sichtkontakt bestätigt. Jetzt zugreifen.",
	})
}
