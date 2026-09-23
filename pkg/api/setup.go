package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/elias/operation-x/pkg/citysets"
	"github.com/elias/operation-x/pkg/geo"
	"github.com/elias/operation-x/pkg/schema"
)

// minSectorAreaM2 ist die Untergrenze für einen Sektor.
//
// Tausend Quadratmeter sind ein Hausgrundstück – kleiner ist mit Sicherheit
// ein Versehen beim Zeichnen und kein gewollter Spielabschnitt.
const minSectorAreaM2 = 1000

// Farben für neu angelegte Sektoren. Bewusst gedämpft und untereinander gut
// unterscheidbar – sie liegen später halbtransparent über der Karte und dürfen
// die Parteifarben nicht nachahmen.
var sectorColors = []string{
	"#4f8ea3", "#7a9e5b", "#a8834a", "#8a6fa8", "#5b9e93",
	"#a35f5f", "#6f86b8", "#9e8a4a", "#5f8f6b", "#8f5f7a",
	"#4a8ba8", "#94a35f", "#a3714a", "#7f5fa3", "#4a9e8a",
}

// Sektorkürzel: A, B, C … Z, dann AA, AB …
func sectorCode(i int) string {
	if i < 26 {
		return string(rune('A' + i))
	}
	return string(rune('A'+(i/26)-1)) + string(rune('A'+(i%26)))
}

type citySearchResponse struct {
	Places []geo.Place `json:"places"`
}

// handleCitySearch sucht die Stadt, in der gespielt wird.
func handleCitySearch(e *core.RequestEvent) error {
	q := strings.TrimSpace(e.Request.URL.Query().Get("q"))
	if len(q) < 2 {
		return e.BadRequestError("Bitte mindestens zwei Zeichen eingeben.", nil)
	}

	ctx, cancel := context.WithTimeout(e.Request.Context(), 40*time.Second)
	defer cancel()

	places, err := nominatim.SearchCity(ctx, q)
	if err != nil {
		return e.Error(http.StatusBadGateway, "Die Ortssuche antwortet gerade nicht: "+err.Error(), nil)
	}

	// Nur Flächen taugen als Spielgebiet.
	usable := make([]geo.Place, 0, len(places))
	for _, p := range places {
		if _, ok := p.AreaID(); ok {
			usable = append(usable, p)
		}
	}

	return e.JSON(http.StatusOK, citySearchResponse{Places: usable})
}

type districtsRequest struct {
	OSMID  int64 `json:"osmId"`
	Levels []int `json:"levels"`
}

type districtLevel struct {
	Level     int            `json:"level"`
	Count     int            `json:"count"`
	MedianKM2 float64        `json:"medianKm2"`
	Districts []geo.District `json:"districts"`
}

type districtsResponse struct {
	Levels []districtLevel `json:"levels"`
}

// handleDistricts holt die Ortsteile einer Stadt und gruppiert sie nach
// Verwaltungsebene.
//
// Welche Ebene brauchbare Sektoren ergibt, unterscheidet sich je Stadt. Statt
// eine zu raten, kommen alle zurück – mit Anzahl und typischer Größe, damit die
// Wahl im Assistenten offensichtlich ist.
func handleDistricts(e *core.RequestEvent) error {
	var req districtsRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}
	if req.OSMID == 0 {
		return e.BadRequestError("Es wurde keine Stadt ausgewählt.", nil)
	}
	if len(req.Levels) == 0 {
		req.Levels = []int{9, 10, 11}
	}

	ctx, cancel := context.WithTimeout(e.Request.Context(), 3*time.Minute)
	defer cancel()

	areaID := 3600000000 + req.OSMID
	districts, err := overpass.FetchDistricts(ctx, areaID, req.Levels)
	if err != nil {
		return e.Error(http.StatusBadGateway, err.Error(), nil)
	}

	byLevel := map[int][]geo.District{}
	for _, d := range districts {
		byLevel[d.AdminLevel] = append(byLevel[d.AdminLevel], d)
	}

	res := districtsResponse{}
	for _, lvl := range req.Levels {
		list := byLevel[lvl]
		if len(list) == 0 {
			continue
		}
		res.Levels = append(res.Levels, districtLevel{
			Level:     lvl,
			Count:     len(list),
			MedianKM2: medianArea(list),
			Districts: list,
		})
	}

	if len(res.Levels) == 0 {
		return e.Error(http.StatusNotFound,
			"Für diese Stadt sind keine Ortsteilgrenzen hinterlegt. Sektoren lassen sich von Hand zeichnen.", nil)
	}

	return e.JSON(http.StatusOK, res)
}

func medianArea(list []geo.District) float64 {
	if len(list) == 0 {
		return 0
	}
	areas := make([]float64, len(list))
	for i, d := range list {
		areas[i] = d.AreaKM2
	}
	// Einfache Einfügesortierung – die Listen sind klein.
	for i := 1; i < len(areas); i++ {
		for j := i; j > 0 && areas[j] < areas[j-1]; j-- {
			areas[j], areas[j-1] = areas[j-1], areas[j]
		}
	}
	return areas[len(areas)/2]
}

type sectorInput struct {
	Name     string          `json:"name"`
	Color    string          `json:"color"`
	Geometry json.RawMessage `json:"geometry"`
}

type saveSectorsRequest struct {
	Sectors []sectorInput `json:"sectors"`
	Replace bool          `json:"replace"`
}

// handleSaveSectors legt die gewählten Sektoren an.
//
// Die Kürzel A, B, C … vergibt der Server, damit sie lückenlos sind und auf dem
// gedruckten Kartenblatt mit der Anzeige übereinstimmen.
func handleSaveSectors(e *core.RequestEvent) error {
	game, err := gameOf(e)
	if err != nil {
		return err
	}

	var req saveSectorsRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}
	if len(req.Sectors) == 0 {
		return e.BadRequestError("Es wurden keine Sektoren übergeben.", nil)
	}

	// Grenzen vorab prüfen: Ein Sektor, den der Server nicht versteht, würde
	// später jede Standortzuordnung stillschweigend falsch beantworten.
	for i, s := range req.Sectors {
		area, err := geo.AreaFromGeoJSON(s.Geometry)
		if err != nil {
			return e.BadRequestError(
				fmt.Sprintf("Sektor %d (%s) hat keine brauchbare Grenze: %v", i+1, s.Name, err), nil)
		}

		// Eine Fläche ohne Inhalt ist keine. Sie ließe sich anlegen, würde aber
		// nie einen Punkt enthalten – die Zentrale hätte einen Sektor auf der
		// Liste, in dem sich niemals jemand aufhält. Die Oberfläche verhindert
		// das schon, aber die Regel gehört auf den Server: Er ist die Instanz,
		// die entscheidet, was ein Sektor ist.
		if area.AreaM2() < minSectorAreaM2 {
			return e.BadRequestError(fmt.Sprintf(
				"Sektor %d (%s) umschließt keine Fläche. Mindestens drei Ecken, "+
					"die nicht auf einer Linie liegen.", i+1, s.Name), nil)
		}
	}

	col, err := e.App.FindCollectionByNameOrId(schema.ColSectors)
	if err != nil {
		return err
	}

	var created []map[string]any
	var assigned int

	err = e.App.RunInTransaction(func(tx core.App) error {
		if req.Replace {
			existing, err := tx.FindRecordsByFilter(schema.ColSectors, "game = {:g}", "", 0, 0,
				map[string]any{"g": game.Id})
			if err != nil {
				return err
			}
			for _, rec := range existing {
				if err := tx.Delete(rec); err != nil {
					return err
				}
			}
		}

		for i, s := range req.Sectors {
			color := s.Color
			if color == "" {
				color = sectorColors[i%len(sectorColors)]
			}

			rec := core.NewRecord(col)
			rec.Set("game", game.Id)
			rec.Set("code", sectorCode(i))
			rec.Set("name", s.Name)
			rec.Set("color", color)
			rec.Set("geometry", types.JSONRaw(s.Geometry))

			if err := tx.Save(rec); err != nil {
				return fmt.Errorf("Sektor %q speichern: %w", s.Name, err)
			}

			created = append(created, map[string]any{
				"id":   rec.Id,
				"code": rec.GetString("code"),
				"name": rec.GetString("name"),
			})
		}

		if assigned, err = reassignHotspots(tx, game.Id); err != nil {
			return err
		}

		// Das Spielgebiet ergibt sich aus den Sektoren – etwas anderes ist es
		// nicht. Ohne diesen Schritt wüsste die Karte beim ersten Öffnen nicht,
		// wohin sie schauen soll.
		if area := sectorBounds(req.Sectors); area != nil {
			game.Set("area", types.JSONRaw(area))
			if err := tx.Save(game); err != nil {
				return fmt.Errorf("Spielgebiet setzen: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return e.InternalServerError("Sektoren konnten nicht gespeichert werden.", err)
	}

	return e.JSON(http.StatusOK, map[string]any{
		"saved":      len(created),
		"sectors":    created,
		"reassigned": assigned,
	})
}

// sectorBounds umfasst alle Sektoren mit einem achsenparallelen Rechteck.
//
// Bewusst ein Rechteck und nicht die Vereinigung der Umrisse: Gebraucht wird
// nur ein Kartenausschnitt, und dafür ist die genaue Außenkante egal.
func sectorBounds(sectors []sectorInput) json.RawMessage {
	b := geo.Bounds{West: 180, South: 90, East: -180, North: -90}
	found := false

	for _, s := range sectors {
		area, err := geo.AreaFromGeoJSON(s.Geometry)
		if err != nil {
			continue
		}
		sb := area.Outer.Bounds()
		b.West = math.Min(b.West, sb.West)
		b.South = math.Min(b.South, sb.South)
		b.East = math.Max(b.East, sb.East)
		b.North = math.Max(b.North, sb.North)
		found = true
	}
	if !found {
		return nil
	}

	return json.RawMessage(fmt.Sprintf(
		`{"type":"Polygon","coordinates":[[[%g,%g],[%g,%g],[%g,%g],[%g,%g],[%g,%g]]]}`,
		b.West, b.South, b.East, b.South, b.East, b.North, b.West, b.North, b.West, b.South,
	))
}

// reassignHotspots ordnet jeden Hotspot dem Sektor zu, in dem er liegt.
//
// Nötig nach jeder Änderung des Sektorschnitts: Die alte Zuordnung ist dann
// hinfällig, und ein Hotspot ohne Sektor ließe später jede Frage nach dem
// Aufenthaltsbereich ins Leere laufen. Die Zuordnung rechnet der Server aus den
// Koordinaten aus – sie ist nichts, was jemand von Hand pflegen sollte.
func reassignHotspots(app core.App, gameID string) (int, error) {
	sectors, err := app.FindRecordsByFilter(schema.ColSectors, "game = {:g}", "code", 0, 0,
		map[string]any{"g": gameID})
	if err != nil {
		return 0, err
	}

	type sectorArea struct {
		id   string
		area geo.Area
	}
	areas := make([]sectorArea, 0, len(sectors))
	for _, s := range sectors {
		a, err := geo.AreaFromGeoJSON(toRaw(s.Get("geometry")))
		if err != nil {
			continue // unlesbare Grenze übergehen statt alles abzubrechen
		}
		areas = append(areas, sectorArea{id: s.Id, area: a})
	}

	hotspots, err := app.FindRecordsByFilter(schema.ColHotspots, "game = {:g}", "number", 0, 0,
		map[string]any{"g": gameID})
	if err != nil {
		return 0, err
	}

	count := 0
	for _, h := range hotspots {
		point := geo.Point{Lat: h.GetFloat("lat"), Lng: h.GetFloat("lng")}

		match := ""
		for _, s := range areas {
			if s.area.Contains(point) {
				match = s.id
				break
			}
		}

		if h.GetString("sector") == match {
			continue
		}

		h.Set("sector", match)
		if err := app.Save(h); err != nil {
			return count, fmt.Errorf("Hotspot #%02d zuordnen: %w", h.GetInt("number"), err)
		}
		if match != "" {
			count++
		}
	}

	return count, nil
}

// --- Stadtpakete ---------------------------------------------------------

// Ein Spiel vorzubereiten heißt: Sektoren schneiden und zwanzig Hotspots
// setzen. Beides geht in der Oberfläche, beides dauert, und beides braucht
// Internet – die Grenzen und die Ortsvorschläge kommen von OpenStreetMap.
//
// Für die mitgelieferten Städte geht es ohne. Das Paket ist ein Anfang, keine
// Vorschrift: Danach lässt sich alles ändern, löschen und ergänzen wie sonst
// auch.

// handleCitySets listet die mitgelieferten Stadtpakete.
func handleCitySets(e *core.RequestEvent) error {
	return e.JSON(http.StatusOK, map[string]any{"sets": citysets.List()})
}

// handleCitySet liefert ein Paket vollständig.
func handleCitySet(e *core.RequestEvent) error {
	pack, err := citysets.Get(e.Request.PathValue("slug"))
	if err != nil {
		return e.NotFoundError(err.Error(), nil)
	}
	return e.JSON(http.StatusOK, pack)
}
