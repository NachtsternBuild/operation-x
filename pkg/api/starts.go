package api

import (
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/game"
	"github.com/elias/operation-x/pkg/geo"
	"github.com/elias/operation-x/pkg/schema"
)

// Die Startpunkte.
//
// Am Anfang steht eine Frage, die sich am Spieltag nicht von selbst löst: Wo
// fängt wer an? Überlässt man sie den Leuten, stehen am Ende alle an derselben
// Haltestelle – und wenn die Zielperson und ein Fahndungsteam zufällig
// denselben Ort erwischen, ist das Spiel in der ersten Minute entschieden,
// ohne dass jemand etwas richtig oder falsch gemacht hätte.
//
// Deshalb zieht die Zentrale, und jeder Punkt wird nur einmal vergeben. Das
// Ergebnis steht anschließend auf jedem Gerät – der Weg dorthin ist die
// Anreise, und für die gibt es die Pause (siehe control.go).

type startEntry struct {
	Team     string  `json:"team"`
	Callsign string  `json:"callsign"`
	Display  string  `json:"display,omitempty"`
	Role     string  `json:"role"`
	Hotspot  string  `json:"hotspot,omitempty"`
	Number   int     `json:"number,omitempty"`
	Name     string  `json:"name,omitempty"`
	Lat      float64 `json:"lat,omitempty"`
	Lng      float64 `json:"lng,omitempty"`

	// Der Anreisestand: wie weit das Team noch von seinem Startpunkt entfernt
	// ist, und ob es schon dort steht. Known sagt, ob überhaupt eine Meldung
	// vorliegt – "noch keine Ortung" ist etwas anderes als "weit weg".
	DistanceM float64 `json:"distanceM,omitempty"`
	Arrived   bool    `json:"arrived"`
	Known     bool    `json:"known"`
}

// handleDrawStarts lost die Startpunkte neu aus.
func handleDrawStarts(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	// Nach dem Start wäre eine neue Auslosung ein Eingriff ins laufende Spiel:
	// Die Teams stehen dann längst irgendwo. Vorher ist sie beliebig oft
	// wiederholbar – wem die Verteilung nicht gefällt, der zieht neu.
	if gameRec.GetString("status") == schema.GameRunning {
		return e.BadRequestError(
			"Das Spiel läuft bereits. Die Startpunkte werden vor dem Start gezogen.", nil)
	}

	hotspots, err := e.App.FindRecordsByFilter(schema.ColHotspots, "game = {:g}", "number", 0, 0,
		map[string]any{"g": gameRec.Id})
	if err != nil {
		return err
	}

	teams, err := e.App.FindRecordsByFilter(schema.ColTeams,
		"game = {:g} && role != {:hq}", "", 0, 0,
		map[string]any{"g": gameRec.Id, "hq": schema.RoleHQ})
	if err != nil {
		return err
	}
	if len(teams) == 0 {
		return e.BadRequestError("Es sind noch keine Teams angelegt.", nil)
	}

	candidates := make([]game.StartCandidate, 0, len(hotspots))
	byID := map[string]*core.Record{}
	for _, h := range hotspots {
		candidates = append(candidates, game.StartCandidate{
			HotspotID: h.Id,
			Number:    h.GetInt("number"),
			Name:      h.GetString("name"),
			Point:     geo.Point{Lat: h.GetFloat("lat"), Lng: h.GetFloat("lng")},
		})
		byID[h.Id] = h
	}

	// Die Zielperson zieht zuerst: Wird es eng, soll sie die größere Auswahl
	// haben – sie muss von ihrem Startpunkt aus als Erste weg.
	order := make([]string, 0, len(teams))
	byTeam := map[string]*core.Record{}
	for _, t := range teams {
		byTeam[t.Id] = t
		if t.GetString("role") == schema.RoleMisterX {
			order = append([]string{t.Id}, order...)
		} else {
			order = append(order, t.Id)
		}
	}

	draws := game.DrawStarts(candidates, order, rand.New(rand.NewSource(time.Now().UnixNano())))
	if draws == nil {
		return e.BadRequestError(fmt.Sprintf(
			"Für %d Teams braucht es mindestens %d Hotspots, angelegt sind %d.",
			len(teams), len(teams), len(hotspots)), nil)
	}

	for _, d := range draws {
		team := byTeam[d.TeamID]
		team.Set("start_hotspot", d.HotspotID)
		if err := e.App.Save(team); err != nil {
			return e.InternalServerError("Der Startpunkt ließ sich nicht speichern.", err)
		}

		_, _ = game.Book(e.App, game.Booking{
			Game: gameRec.Id, Team: d.TeamID,
			Type:   "start.drawn",
			Reason: fmt.Sprintf("Startpunkt ausgelost: #%02d %s", d.Number, d.Name),
			Payload: map[string]any{
				"hotspot": d.HotspotID, "nummer": d.Number, "name": d.Name,
			},
		})
	}

	return startList(e, gameRec)
}

// handleStarts zeigt die aktuelle Verteilung.
func handleStarts(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}
	return startList(e, gameRec)
}

func startList(e *core.RequestEvent, gameRec *core.Record) error {
	teams, err := e.App.FindRecordsByFilter(schema.ColTeams,
		"game = {:g} && role != {:hq}", "callsign", 0, 0,
		map[string]any{"g": gameRec.Id, "hq": schema.RoleHQ})
	if err != nil {
		return err
	}

	out := make([]startEntry, 0, len(teams))
	for _, t := range teams {
		entry := startEntry{
			Team:     t.Id,
			Callsign: t.GetString("callsign"),
			Display:  t.GetString("display"),
			Role:     t.GetString("role"),
		}

		if id := t.GetString("start_hotspot"); id != "" {
			if h, err := e.App.FindRecordById(schema.ColHotspots, id); err == nil && h != nil {
				entry.Hotspot = h.Id
				entry.Number = h.GetInt("number")
				entry.Name = h.GetString("name")
				entry.Lat = h.GetFloat("lat")
				entry.Lng = h.GetFloat("lng")

				// Die Anreise: Solange die läuft, ist die Frage der Zentrale
				// nicht "wo sind alle", sondern "sind alle da".
				if pos := latestPosition(e.App, t.Id); pos != nil {
					entry.Known = true
					entry.DistanceM = geo.DistanceM(
						geo.Point{Lat: pos.GetFloat("lat"), Lng: pos.GetFloat("lng")},
						geo.Point{Lat: entry.Lat, Lng: entry.Lng},
					)
					// Derselbe Abstand wie für den Nachweis am Zwischenziel:
					// Wer dort den Code eingeben dürfte, steht auch hier.
					entry.Arrived = entry.DistanceM <=
						game.ConfigOf(gameRec).HotspotMaxDistanceM+pos.GetFloat("accuracy")
				}
			}
		}

		out = append(out, entry)
	}

	return e.JSON(http.StatusOK, map[string]any{"starts": out})
}
