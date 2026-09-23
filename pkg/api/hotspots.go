package api

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/geo"
	"github.com/elias/operation-x/pkg/schema"
)

// Wortliste für die Vor-Ort-Codes.
//
// Der Code beweist, dass jemand wirklich am Hotspot stand. Er wird vor Ort
// angebracht oder vom Spielleiter aus etwas abgelesen, das dort ohnehin steht –
// deshalb muss er sich vorlesen, auf einen Zettel schreiben und ohne Nachfrage
// eintippen lassen. Keine Umlaute, keine Verwechslungsgefahr, kurz genug für
// die Handytastatur im Gehen.
var codeWords = []string{
	"ANKER", "AMSEL", "BIBER", "BLITZ", "BRUNNEN", "DACHS", "DISTEL", "DOMPFAFF",
	"EICHE", "ELSTER", "ESPE", "FALKE", "FIBEL", "FLIEDER", "FUCHS", "GIEBEL",
	"GLOCKE", "GRANIT", "HAFEN", "HAMMER", "HIRSCH", "HOLUNDER", "IGEL", "KIEFER",
	"KOMPASS", "KRANICH", "KUPFER", "LATERNE", "LERCHE", "LINDE", "LOTSE", "LUCHS",
	"MARDER", "MOEWE", "NEBEL", "OTTER", "PAPPEL", "PFEIL", "QUARZ", "RABE",
	"REIHER", "SALBEI", "SCHIEFER", "SEGEL", "SPATZ", "STORCH", "TAUBE", "TURMALIN",
	"ULME", "UHU", "WACHOLDER", "WEIHER", "WIESEL", "ZEDER", "ZIRBE", "ZUNDER",
}

// generateCode liefert einen unverbrauchten Vor-Ort-Code.
func generateCode(used map[string]bool, rng *rand.Rand) string {
	for attempt := 0; attempt < 200; attempt++ {
		word := codeWords[rng.Intn(len(codeWords))]
		if !used[word] {
			used[word] = true
			return word
		}
	}
	// Mehr Hotspots als Wörter: mit einer Ziffer verlängern.
	for i := 2; ; i++ {
		word := fmt.Sprintf("%s%d", codeWords[rng.Intn(len(codeWords))], i)
		if !used[word] {
			used[word] = true
			return word
		}
	}
}

// handlePOIs schlägt markante Orte im Spielgebiet vor.
func handlePOIs(e *core.RequestEvent) error {
	game, err := gameOf(e)
	if err != nil {
		return err
	}

	bounds, err := gameBounds(e.App, game)
	if err != nil {
		return e.BadRequestError(
			"Das Spielgebiet steht noch nicht fest. Zuerst Sektoren übernehmen.", nil)
	}

	ctx, cancel := context.WithTimeout(e.Request.Context(), 2*time.Minute)
	defer cancel()

	pois, err := overpass.FetchPOIs(ctx, bounds, 80)
	if err != nil {
		return e.Error(http.StatusBadGateway, err.Error(), nil)
	}

	// Bereits gesetzte Hotspots nicht erneut vorschlagen.
	existing, err := e.App.FindRecordsByFilter(schema.ColHotspots, "game = {:g}", "", 0, 0,
		map[string]any{"g": game.Id})
	if err != nil {
		return err
	}

	filtered := make([]geo.POI, 0, len(pois))
	for _, p := range pois {
		if isDuplicate(p, existing) {
			continue
		}
		filtered = append(filtered, p)
	}

	return e.JSON(http.StatusOK, map[string]any{"pois": filtered})
}

// isDuplicate erkennt einen Vorschlag, der schon als Hotspot gesetzt ist –
// über den Namen oder über die Nähe, weil OSM denselben Ort mehrfach führen kann.
func isDuplicate(p geo.POI, existing []*core.Record) bool {
	for _, h := range existing {
		if strings.EqualFold(h.GetString("name"), p.Name) {
			return true
		}
		d := geo.DistanceM(
			geo.Point{Lat: p.Lat, Lng: p.Lng},
			geo.Point{Lat: h.GetFloat("lat"), Lng: h.GetFloat("lng")},
		)
		if d < 40 {
			return true
		}
	}
	return false
}

type saveHotspotsRequest struct {
	Hotspots []struct {
		Name     string  `json:"name"`
		Lat      float64 `json:"lat"`
		Lng      float64 `json:"lng"`
		Kind     string  `json:"kind"`
		Passcode string  `json:"passcode"`
		Notes    string  `json:"notes"`
	} `json:"hotspots"`
	Replace bool `json:"replace"`
}

// handleSaveHotspots legt Hotspots an.
//
// Nummern und Vor-Ort-Codes vergibt der Server: Die Nummern müssen lückenlos
// sein, weil sie im Zugriffsformular als Antwort eingetippt werden, und die
// Codes dürfen sich innerhalb eines Spiels nicht wiederholen.
func handleSaveHotspots(e *core.RequestEvent) error {
	game, err := gameOf(e)
	if err != nil {
		return err
	}

	var req saveHotspotsRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}
	if len(req.Hotspots) == 0 {
		return e.BadRequestError("Es wurden keine Hotspots übergeben.", nil)
	}

	for i, h := range req.Hotspots {
		if h.Lat == 0 && h.Lng == 0 {
			return e.BadRequestError(fmt.Sprintf("Hotspot %d hat keine Koordinaten.", i+1), nil)
		}
		if math.Abs(h.Lat) > 90 || math.Abs(h.Lng) > 180 {
			return e.BadRequestError(fmt.Sprintf("Hotspot %d liegt außerhalb der Erde.", i+1), nil)
		}
	}

	col, err := e.App.FindCollectionByNameOrId(schema.ColHotspots)
	if err != nil {
		return err
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	var created int

	err = e.App.RunInTransaction(func(tx core.App) error {
		existing, err := tx.FindRecordsByFilter(schema.ColHotspots, "game = {:g}", "number", 0, 0,
			map[string]any{"g": game.Id})
		if err != nil {
			return err
		}

		if req.Replace {
			for _, rec := range existing {
				if err := tx.Delete(rec); err != nil {
					return err
				}
			}
			existing = nil
		}

		used := map[string]bool{}
		next := 1
		for _, rec := range existing {
			used[rec.GetString("passcode")] = true
			if n := rec.GetInt("number"); n >= next {
				next = n + 1
			}
		}

		for _, h := range req.Hotspots {
			code := strings.ToUpper(strings.TrimSpace(h.Passcode))
			if code == "" {
				code = generateCode(used, rng)
			} else {
				used[code] = true
			}

			kind := h.Kind
			if kind == "" {
				kind = "other"
			}

			rec := core.NewRecord(col)
			rec.Set("game", game.Id)
			rec.Set("number", next)
			rec.Set("name", strings.TrimSpace(h.Name))
			rec.Set("lat", h.Lat)
			rec.Set("lng", h.Lng)
			rec.Set("kind", kind)
			rec.Set("passcode", code)
			rec.Set("notes", h.Notes)

			if err := tx.Save(rec); err != nil {
				return fmt.Errorf("Hotspot %q speichern: %w", h.Name, err)
			}
			next++
			created++
		}

		_, err = reassignHotspots(tx, game.Id)
		return err
	})
	if err != nil {
		return e.InternalServerError("Hotspots konnten nicht gespeichert werden.", err)
	}

	return e.JSON(http.StatusOK, map[string]any{"saved": created})
}

type updateHotspotRequest struct {
	Name     *string  `json:"name"`
	Lat      *float64 `json:"lat"`
	Lng      *float64 `json:"lng"`
	Kind     *string  `json:"kind"`
	Passcode *string  `json:"passcode"`
	Notes    *string  `json:"notes"`
}

// handleUpdateHotspot ändert einen Hotspot. Nur die übergebenen Felder.
func handleUpdateHotspot(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	rec, err := ausSpiel(e, schema.ColHotspots, e.Request.PathValue("id"), gameRec.Id)
	if err != nil {
		return err
	}

	var req updateHotspotRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}

	moved := false
	if req.Name != nil {
		rec.Set("name", strings.TrimSpace(*req.Name))
	}
	if req.Lat != nil {
		rec.Set("lat", *req.Lat)
		moved = true
	}
	if req.Lng != nil {
		rec.Set("lng", *req.Lng)
		moved = true
	}
	if req.Kind != nil {
		rec.Set("kind", *req.Kind)
	}
	if req.Passcode != nil {
		rec.Set("passcode", strings.ToUpper(strings.TrimSpace(*req.Passcode)))
	}
	if req.Notes != nil {
		rec.Set("notes", *req.Notes)
	}

	if err := e.App.Save(rec); err != nil {
		return e.BadRequestError("Änderung konnte nicht gespeichert werden.", err)
	}

	// Wurde der Punkt verschoben, kann er in einem anderen Sektor liegen.
	if moved {
		if _, err := reassignHotspots(e.App, rec.GetString("game")); err != nil {
			return e.InternalServerError("Sektorzuordnung fehlgeschlagen.", err)
		}
	}

	return e.JSON(http.StatusOK, map[string]any{"ok": true})
}

// handleDeleteHotspot entfernt einen Hotspot und schließt die Lücke in der
// Nummerierung.
func handleDeleteHotspot(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	rec, err := ausSpiel(e, schema.ColHotspots, e.Request.PathValue("id"), gameRec.Id)
	if err != nil {
		return err
	}
	gameID := rec.GetString("game")

	err = e.App.RunInTransaction(func(tx core.App) error {
		if err := tx.Delete(rec); err != nil {
			return err
		}
		return renumberHotspots(tx, gameID)
	})
	if err != nil {
		return e.InternalServerError("Hotspot konnte nicht gelöscht werden.", err)
	}

	return e.JSON(http.StatusOK, map[string]any{"ok": true})
}

// renumberHotspots vergibt die Nummern lückenlos neu.
//
// Lücken wären kein Schönheitsfehler: Die Nummer ist die Antwort, die ein Team
// im Zugriffsformular einträgt, und sie steht so auf dem gedruckten Kartenblatt.
func renumberHotspots(app core.App, gameID string) error {
	list, err := app.FindRecordsByFilter(schema.ColHotspots, "game = {:g}", "number", 0, 0,
		map[string]any{"g": gameID})
	if err != nil {
		return err
	}

	for i, rec := range list {
		want := i + 1
		if rec.GetInt("number") == want {
			continue
		}
		rec.Set("number", want)
		if err := app.Save(rec); err != nil {
			return err
		}
	}
	return nil
}

// gameBounds liefert das Rechteck, in dem gespielt wird – aus dem Spielgebiet,
// ersatzweise aus der Ausdehnung der Sektoren.
func gameBounds(app core.App, game *core.Record) (geo.Bounds, error) {
	if raw := toRaw(game.Get("area")); len(raw) > 0 {
		if area, err := geo.AreaFromGeoJSON(raw); err == nil && len(area.Outer) > 0 {
			return area.Outer.Bounds(), nil
		}
	}

	sectors, err := app.FindRecordsByFilter(schema.ColSectors, "game = {:g}", "", 0, 0,
		map[string]any{"g": game.Id})
	if err != nil {
		return geo.Bounds{}, err
	}
	if len(sectors) == 0 {
		return geo.Bounds{}, fmt.Errorf("weder Spielgebiet noch Sektoren vorhanden")
	}

	out := geo.Bounds{West: 180, South: 90, East: -180, North: -90}
	found := false
	for _, s := range sectors {
		area, err := geo.AreaFromGeoJSON(toRaw(s.Get("geometry")))
		if err != nil || len(area.Outer) == 0 {
			continue
		}
		b := area.Outer.Bounds()
		out.West = math.Min(out.West, b.West)
		out.South = math.Min(out.South, b.South)
		out.East = math.Max(out.East, b.East)
		out.North = math.Max(out.North, b.North)
		found = true
	}
	if !found {
		return geo.Bounds{}, fmt.Errorf("kein Sektor mit brauchbarer Grenze")
	}

	return out, nil
}
