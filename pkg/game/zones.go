package game

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/geo"
	"github.com/elias/operation-x/pkg/schema"
)

// checkZones löst Sperrzonen und Wanzen aus.
//
// Beide sind gestellte Fallen: Sie kosten im Voraus Fahndungspunkte und zahlen
// sich nur aus, wenn die Zielperson tatsächlich hineinläuft. Eine Nebelkerze
// setzt sie außer Kraft – das ist der Grund, warum sie überhaupt etwas kostet.
func (e *Engine) checkZones(gameRec *core.Record, cfg Config, now time.Time) error {
	zones, err := e.app.FindRecordsByFilter(
		schema.ColZones,
		"game = {:g} && triggered = false && expires_at > {:now}",
		"", 0, 0,
		map[string]any{"g": gameRec.Id, "now": DBTime(now)},
	)
	if err != nil || len(zones) == 0 {
		return err
	}

	misterX, err := e.app.FindFirstRecordByFilter(schema.ColTeams,
		"game = {:g} && role = {:r}",
		map[string]any{"g": gameRec.Id, "r": schema.RoleMisterX})
	if err != nil || misterX == nil {
		return nil
	}

	pos := lastPosition(e.app, misterX.Id)
	if pos == nil {
		return nil
	}

	// Eine Meldung, die älter ist als das Ping-Intervall, sagt über den
	// jetzigen Aufenthalt nichts mehr aus.
	captured := pos.GetDateTime("captured_at").Time()
	if now.Sub(captured) > cfg.PingInterval(false) {
		return nil
	}

	fx := ActiveEffects(e.app, gameRec.Id, now)
	if fx.SmokeActive(now) {
		return nil // Die Nebelkerze verschluckt jede Auslösung.
	}

	where := geo.Point{Lat: pos.GetFloat("lat"), Lng: pos.GetFloat("lng")}

	for _, zone := range zones {
		hit, label := zoneHit(e.app, zone, where)
		if !hit {
			continue
		}

		zone.Set("triggered", true)
		zone.Set("triggered_at", captured)
		if err := e.app.Save(zone); err != nil {
			return err
		}

		kind := zone.GetString("kind")
		reason := fmt.Sprintf("Sperrzone ausgelöst: %s", label)
		intelText := fmt.Sprintf("Alarm: Die Zielperson hat um %s Uhr Sektor %s betreten.",
			captured.Local().Format("15:04"), label)
		if kind == "bug" {
			reason = fmt.Sprintf("Wanze ausgelöst an %s", label)
			intelText = fmt.Sprintf("Alarm: Die Wanze an %s hat um %s Uhr angeschlagen.",
				label, captured.Local().Format("15:04"))
		}

		// Der Treffer ist eine gesicherte Tatsache – also ein grüner Hinweis.
		if err := e.storeAlarmIntel(gameRec.Id, intelText, captured, fx, now); err != nil {
			return err
		}

		if _, err := Book(e.app, Booking{
			Game: gameRec.Id, Team: zone.GetString("team"),
			Type:      "zone." + kind,
			Reason:    reason,
			DedupeKey: "zone:" + zone.Id,
			Payload:   map[string]any{"ort": label},
		}); err != nil {
			return err
		}
	}

	return nil
}

// zoneHit prüft, ob eine Position eine Falle auslöst.
func zoneHit(app core.App, zone *core.Record, where geo.Point) (bool, string) {
	switch zone.GetString("kind") {
	case "lockdown":
		sector, err := app.FindRecordById(schema.ColSectors, zone.GetString("sector"))
		if err != nil {
			return false, ""
		}
		area, err := geo.AreaFromGeoJSON(toRawValue(sector.Get("geometry")))
		if err != nil {
			return false, ""
		}
		return area.Contains(where), sector.GetString("code") + " " + sector.GetString("name")

	case "bug":
		radius := zone.GetFloat("radius_m")
		if radius <= 0 {
			radius = 120
		}
		target := geo.Point{Lat: zone.GetFloat("lat"), Lng: zone.GetFloat("lng")}
		if geo.DistanceM(where, target) > radius {
			return false, ""
		}

		label := "einem Hotspot"
		if h, err := app.FindRecordById(schema.ColHotspots, zone.GetString("hotspot")); err == nil {
			label = fmt.Sprintf("#%02d %s", h.GetInt("number"), h.GetString("name"))
		}
		return true, label

	default:
		return false, ""
	}
}

// storeAlarmIntel legt den Hinweis ab, den eine ausgelöste Falle erzeugt.
func (e *Engine) storeAlarmIntel(gameID, text string, occurred time.Time, fx Effects, now time.Time) error {
	col, err := e.app.FindCollectionByNameOrId(schema.ColIntel)
	if err != nil {
		return err
	}

	deliver := now
	if fx.DelayActive(now) {
		deliver = fx.DelayUntil
	}

	rec := core.NewRecord(col)
	rec.Set("game", gameID)
	rec.Set("category", IntelGreen)
	rec.Set("text", text)
	rec.Set("fabricated", false)
	rec.Set("source", "system")
	rec.Set("occurred_at", occurred)
	rec.Set("deliver_at", deliver)
	rec.Set("delivered", true)

	return e.app.Save(rec)
}

// toRawValue wandelt ein JSON-Feld in rohes JSON.
func toRawValue(v any) []byte {
	switch t := v.(type) {
	case nil:
		return nil
	case []byte:
		return t
	case string:
		return []byte(t)
	default:
		return nil
	}
}
