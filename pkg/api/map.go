package api

import (
	"encoding/json"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/schema"
)

type mapSector struct {
	ID       string          `json:"id"`
	Code     string          `json:"code"`
	Name     string          `json:"name"`
	Color    string          `json:"color"`
	Geometry json.RawMessage `json:"geometry"`
}

type mapHotspot struct {
	ID     string  `json:"id"`
	Number int     `json:"number"`
	Name   string  `json:"name"`
	Lat    float64 `json:"lat"`
	Lng    float64 `json:"lng"`
	Sector string  `json:"sector,omitempty"`
	Kind   string  `json:"kind,omitempty"`

	// Passcode steht vor Ort am Hotspot und beweist, dass jemand dort war.
	// Er geht ausschließlich an die Einsatzzentrale – bekäme ihn ein Spieler
	// vorab, ließe sich jede Anwesenheit vom Sofa aus behaupten.
	Passcode string `json:"passcode,omitempty"`

	// Die Aufgabe, die die Zielperson vor Ort erledigt. Ebenfalls nur für die
	// Zentrale: Wüsste die Fahndung im Voraus, was an welchem Hotspot zu tun
	// ist, müsste sie nicht mehr suchen, sondern nur noch warten. Die
	// Zielperson bekommt sie über das Missionsbuch – und nur die eine, die
	// gerade dran ist.
	Task string `json:"task,omitempty"`
}

type mapResponse struct {
	Game     string          `json:"game"`
	City     string          `json:"city"`
	Area     json.RawMessage `json:"area,omitempty"`
	Sectors  []mapSector     `json:"sectors"`
	Hotspots []mapHotspot    `json:"hotspots"`
}

// handleMap liefert das Spielfeld: Gebiet, Sektoren und Hotspots.
//
// Sektoren und Hotspots sind allen Rollen bekannt – sie stehen auch auf dem
// gedruckten Kartenblatt. Was nicht auf dem Papier steht, bleibt hier ebenfalls
// draußen.
func handleMap(e *core.RequestEvent) error {
	game, err := gameOf(e)
	if err != nil {
		return err
	}

	isHQ := e.Auth != nil && e.Auth.GetString("role") == schema.RoleHQ

	res := mapResponse{
		Game:     game.GetString("name"),
		City:     game.GetString("city"),
		Sectors:  []mapSector{},
		Hotspots: []mapHotspot{},
	}
	if raw := game.Get("area"); raw != nil {
		res.Area = toRaw(raw)
	}

	sectors, err := e.App.FindRecordsByFilter(schema.ColSectors, "game = {:g}", "code", 0, 0,
		map[string]any{"g": game.Id})
	if err != nil {
		return err
	}
	for _, s := range sectors {
		res.Sectors = append(res.Sectors, mapSector{
			ID:       s.Id,
			Code:     s.GetString("code"),
			Name:     s.GetString("name"),
			Color:    s.GetString("color"),
			Geometry: toRaw(s.Get("geometry")),
		})
	}

	hotspots, err := e.App.FindRecordsByFilter(schema.ColHotspots, "game = {:g}", "number", 0, 0,
		map[string]any{"g": game.Id})
	if err != nil {
		return err
	}
	for _, h := range hotspots {
		spot := mapHotspot{
			ID:     h.Id,
			Number: h.GetInt("number"),
			Name:   h.GetString("name"),
			Lat:    h.GetFloat("lat"),
			Lng:    h.GetFloat("lng"),
			Sector: h.GetString("sector"),
			Kind:   h.GetString("kind"),
		}
		if isHQ {
			spot.Passcode = h.GetString("passcode")
			spot.Task = h.GetString("notes")
		}
		res.Hotspots = append(res.Hotspots, spot)
	}

	return e.JSON(http.StatusOK, res)
}

// toRaw wandelt ein JSON-Feld aus der Datenbank in rohes JSON für die Antwort.
func toRaw(v any) json.RawMessage {
	switch t := v.(type) {
	case nil:
		return nil
	case json.RawMessage:
		return t
	case []byte:
		return json.RawMessage(t)
	case string:
		return json.RawMessage(t)
	default:
		raw, err := json.Marshal(t)
		if err != nil {
			return nil
		}
		return raw
	}
}

// jsonUnmarshal ist ein kleiner Helfer, damit die Aufrufer nicht überall
// encoding/json einbinden müssen.
func jsonUnmarshal(raw json.RawMessage, dst any) error {
	return json.Unmarshal(raw, dst)
}
