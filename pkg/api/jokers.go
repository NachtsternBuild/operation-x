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

type jokerEntry struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Cost        int    `json:"cost"`
	Used        int    `json:"used"`
	MaxUses     int    `json:"maxUses,omitempty"`
	Available   bool   `json:"available"`
	Reason      string `json:"reason,omitempty"`
	DurationSec int    `json:"durationSec,omitempty"`
	ActiveUntil string `json:"activeUntil,omitempty"`
}

// handleJokers listet die Joker der eigenen Rolle mit Preis und Verfügbarkeit.
func handleJokers(e *core.RequestEvent) error {
	me := e.Auth
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	cfg := game.ConfigOf(gameRec)
	now := time.Now()
	fx := game.ActiveEffects(e.App, gameRec.Id, now)
	role := me.GetString("role")
	fp := me.GetInt("fp")
	finale := gameRec.GetBool("final_active")

	out := []jokerEntry{}
	for _, spec := range game.JokerCatalog() {
		if spec.Role != role {
			continue
		}

		used := game.JokerUsage(e.App, me.Id, spec.Kind)
		cost := spec.Cost(cfg)

		entry := jokerEntry{
			Kind:        spec.Kind,
			Name:        spec.Name,
			Description: spec.Description,
			Cost:        cost,
			Used:        used,
			MaxUses:     spec.MaxUses,
			DurationSec: int(spec.Duration.Seconds()),
			Available:   true,
		}

		switch {
		case spec.MaxUses > 0 && used >= spec.MaxUses:
			entry.Available = false
			entry.Reason = "Aufgebraucht."
		case fp < cost:
			entry.Available = false
			entry.Reason = fmt.Sprintf("Kostet %d FP, vorhanden sind %d.", cost, fp)
		case finale && spec.BlockedByFinale:
			entry.Available = false
			entry.Reason = "Im Finale gesperrt."
		case spec.Manual:
			// Zuletzt geprüft: Wer zu wenig Fluchtpunkte hat, soll das zuerst
			// lesen – der Weg dorthin nützt ihm dann nichts.
			entry.Available = false
			entry.Reason = spec.Where
		}

		// Laufende Wirkung anzeigen.
		switch spec.Kind {
		case game.JokerSmoke:
			if fx.SmokeActive(now) {
				entry.ActiveUntil = fx.SmokeUntil.UTC().Format(time.RFC3339)
			}
		case game.JokerDelay:
			if fx.DelayActive(now) {
				entry.ActiveUntil = fx.DelayUntil.UTC().Format(time.RFC3339)
			}
		case game.JokerGhost:
			if fx.GhostActive(now) {
				entry.ActiveUntil = fx.GhostUntil.UTC().Format(time.RFC3339)
			}
		case game.JokerGPSPing:
			if fx.GPSPingActive(now) {
				entry.ActiveUntil = fx.GPSPingUntil.UTC().Format(time.RFC3339)
			}
		}

		out = append(out, entry)
	}

	return e.JSON(http.StatusOK, map[string]any{"jokers": out, "fp": fp})
}

type useJokerRequest struct {
	Kind      string `json:"kind"`
	SectorID  string `json:"sectorId"`
	HotspotID string `json:"hotspotId"`
	PuzzleID  string `json:"puzzleId"`
}

// handleUseJoker setzt einen Joker ein.
//
// Die Fluchtpunkte werden zuerst abgebucht und erst danach wirkt der Joker.
// Das ist bewusst so herum: Ein Einsatz, der ins Leere läuft – etwa eine Ortung
// gegen eine Nebelkerze – hat trotzdem stattgefunden und muss bezahlt werden.
// Sonst wäre jede Ortung risikolos, und die Nebelkerze verlöre ihren Sinn.
func handleUseJoker(e *core.RequestEvent) error {
	me := e.Auth
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}
	if gameRec.GetString("status") != schema.GameRunning {
		return e.BadRequestError("Das Spiel läuft gerade nicht.", nil)
	}

	var req useJokerRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}

	spec, ok := game.FindJoker(req.Kind)
	if !ok {
		return e.BadRequestError("Diesen Joker gibt es nicht.", nil)
	}
	if spec.Role != me.GetString("role") {
		return e.ForbiddenError("Dieser Joker gehört der anderen Seite.", nil)
	}
	if spec.Manual {
		return e.BadRequestError(spec.Where, nil)
	}

	cfg := game.ConfigOf(gameRec)
	cost := spec.Cost(cfg)
	now := time.Now()

	if me.GetInt("fp") < cost {
		return e.BadRequestError(
			fmt.Sprintf("%s kostet %d FP, vorhanden sind %d.", spec.Name, cost, me.GetInt("fp")), nil)
	}
	if spec.MaxUses > 0 && game.JokerUsage(e.App, me.Id, spec.Kind) >= spec.MaxUses {
		return e.BadRequestError(spec.Name+" ist aufgebraucht.", nil)
	}
	if gameRec.GetBool("final_active") && spec.BlockedByFinale {
		return e.BadRequestError("Im Finale sind Täuschungsmanöver gesperrt.", nil)
	}

	fx := game.ActiveEffects(e.App, gameRec.Id, now)

	col, err := e.App.FindCollectionByNameOrId(schema.ColJokers)
	if err != nil {
		return err
	}

	rec := core.NewRecord(col)
	rec.Set("game", gameRec.Id)
	rec.Set("team", me.Id)
	rec.Set("kind", spec.Kind)
	rec.Set("cost_fp", cost)
	rec.Set("started_at", now)
	rec.Set("active", true)
	if spec.Duration > 0 {
		rec.Set("expires_at", now.Add(spec.Duration))
	} else {
		// Einmalwirkungen laufen sofort ab, bleiben aber im Protokoll.
		rec.Set("expires_at", now)
		rec.Set("active", false)
	}

	payload := map[string]any{}
	result := map[string]any{"kind": spec.Kind, "name": spec.Name, "cost": cost}

	// --- Wirkung je Joker ---
	switch spec.Kind {
	case game.JokerFalseTrail, game.JokerPhantom:
		category := game.IntelYellow
		if spec.Kind == game.JokerPhantom {
			category = game.IntelRed
		}

		codes, err := sectorCodes(e.App, gameRec.Id)
		if err != nil {
			return e.InternalServerError("Sektoren nicht lesbar.", err)
		}

		fake, ok := game.FabricateIntel(category, codes, now)
		if !ok {
			return e.BadRequestError("Für eine Fälschung fehlen die Voraussetzungen – zu wenige Sektoren.", nil)
		}
		if err := storeIntel(e.App, gameRec.Id, fake, "joker", fx, now); err != nil {
			return e.InternalServerError("Hinweis konnte nicht eingespeist werden.", err)
		}

		payload["text"] = fake.Text
		result["text"] = fake.Text
		result["message"] = "Die Fahndung bekommt eine falsche Spur."

	case game.JokerSmoke:
		result["message"] = "Nebelkerze gezündet. Jede Ortung läuft 15 Minuten ins Leere."

	case game.JokerDelay:
		result["message"] = "Störung aktiv. Neue Hinweise erreichen die Fahndung fünf Minuten später."

	case game.JokerGhost:
		// Die Frist wird nach hinten geschoben, damit die Engine nicht
		// zuschlägt. Das *ist* die Wirkung dieses Jokers – schlägt der Schritt
		// fehl, hat das Team drei Fluchtpunkte für nichts bezahlt und läuft
		// zusätzlich in die Strafe. Deshalb wird der Fehler gemeldet, statt
		// eine gute Nachricht zu drucken, die nicht stimmt.
		fresh, err := e.App.FindRecordById(schema.ColTeams, me.Id)
		if err != nil {
			return e.InternalServerError(
				"Die Meldepflicht ließ sich nicht aussetzen. Der Fluchtpunkt ist "+
					"gebucht – bitte an die Zentrale wenden.", err)
		}
		fresh.Set("ping_due_at", now.Add(spec.Duration+cfg.PingInterval(true)))
		if err := e.App.Save(fresh); err != nil {
			return e.InternalServerError(
				"Die Meldepflicht ließ sich nicht aussetzen. Der Fluchtpunkt ist "+
					"gebucht – bitte an die Zentrale wenden.", err)
		}
		result["message"] = "Meldepflicht ausgesetzt. Gute Fahrt."

	case game.JokerGPSPing:
		if fx.SmokeActive(now) {
			// Der Einsatz zählt trotzdem: Wer ortet, während eine Nebelkerze
			// brennt, hat Pech gehabt – und weiß jetzt immerhin, dass eine brennt.
			result["message"] = "Ortung gestört. Die Zielperson hat Gegenmaßnahmen ergriffen."
			result["blocked"] = true
		} else {
			result["message"] = "Ortung läuft. Die Zielperson ist eine Minute lang offen sichtbar."
		}

	case game.JokerScan:
		if req.SectorID == "" {
			return e.BadRequestError("Für den Scan fehlt der Sektor.", nil)
		}
		sector, err := ausSpiel(e, schema.ColSectors, req.SectorID, gameRec.Id)
		if err != nil {
			return err
		}

		inside := false
		if fx.SmokeActive(now) {
			result["message"] = "Scan gestört. Kein verwertbares Ergebnis."
			result["blocked"] = true
		} else {
			inside, err = misterXInSector(e.App, gameRec.Id, sector)
			if err != nil {
				return e.InternalServerError("Scan fehlgeschlagen.", err)
			}
			result["inside"] = inside
			if inside {
				result["message"] = fmt.Sprintf("Treffer. Die Zielperson ist in Sektor %s.", sector.GetString("code"))
			} else {
				result["message"] = fmt.Sprintf("Kein Treffer in Sektor %s.", sector.GetString("code"))
			}
		}
		payload["sector"] = sector.GetString("code")
		payload["inside"] = inside
		rec.Set("expires_at", now.Add(3*time.Minute)) // erhöhte Unschärfe
		rec.Set("active", true)

	case game.JokerLockdown, game.JokerBug:
		if err := placeZone(e, gameRec, me, spec, req, now); err != nil {
			return e.BadRequestError(err.Error(), nil)
		}
		if spec.Kind == game.JokerLockdown {
			result["message"] = "Sperrzone aktiv. Bei Betreten schlägt das System Alarm."
		} else {
			result["message"] = "Wanze ausgelegt."
		}

	case game.JokerUnlock:
		if req.PuzzleID == "" {
			return e.BadRequestError("Für die Freischaltung fehlt das Rätsel.", nil)
		}
		puzzle, err := ausSpiel(e, schema.ColPuzzles, req.PuzzleID, gameRec.Id)
		if err != nil {
			return err
		}
		intel, err := unlockIntel(e.App, gameRec, puzzle)
		if err != nil {
			return e.BadRequestError("Aus der Lage lässt sich gerade kein Hinweis ableiten.", nil)
		}
		puzzle.Set("unlocked", true)
		if err := e.App.Save(puzzle); err != nil {
			return e.InternalServerError(
				"Der Hinweis ließ sich nicht freischalten.", err)
		}

		result["intel"] = intel
		result["message"] = "Hinweis freigeschaltet."
	}

	if len(payload) > 0 {
		rec.Set("payload", payload)
	}
	if err := e.App.Save(rec); err != nil {
		return e.InternalServerError("Jokereinsatz konnte nicht gespeichert werden.", err)
	}

	// Abbuchen und protokollieren. Punkte für die falsche Fährte gibt es
	// obendrauf – sie ist der einzige Joker, der selbst etwas einbringt.
	bonus := 0
	if spec.Kind == game.JokerFalseTrail {
		bonus = cfg.PointsFalseTrail
	}

	if _, err := game.Book(e.App, game.Booking{
		Game: gameRec.Id, Team: me.Id,
		Type:        "joker." + spec.Kind,
		Reason:      spec.Name + " eingesetzt",
		DeltaFP:     -cost,
		DeltaPoints: bonus,
		DedupeKey:   "joker:" + rec.Id,
		Payload:     payload,
	}); err != nil {
		return e.InternalServerError("Buchung fehlgeschlagen.", err)
	}

	fresh, _ := e.App.FindRecordById(schema.ColTeams, me.Id)
	if fresh != nil {
		result["fp"] = fresh.GetInt("fp")
	}

	return e.JSON(http.StatusOK, result)
}

// storeIntel legt einen erzeugten Hinweis ab und berücksichtigt dabei eine
// laufende Zeitverzögerung.
func storeIntel(app core.App, gameID string, intel game.Intel, source string, fx game.Effects, now time.Time) error {
	col, err := app.FindCollectionByNameOrId(schema.ColIntel)
	if err != nil {
		return err
	}

	deliver := now
	if fx.DelayActive(now) {
		deliver = fx.DelayUntil
	}

	rec := core.NewRecord(col)
	rec.Set("game", gameID)
	rec.Set("category", intel.Category)
	rec.Set("text", intel.Text)
	rec.Set("fabricated", intel.Fabricated)
	rec.Set("source", source)
	rec.Set("occurred_at", intel.OccurredAt)
	rec.Set("deliver_at", deliver)
	rec.Set("delivered", true)
	if intel.HotspotID != "" {
		rec.Set("hotspot", intel.HotspotID)
	}
	if intel.SectorID != "" {
		rec.Set("sector", intel.SectorID)
	}

	return app.Save(rec)
}

// placeZone legt eine Sperrzone oder eine Wanze an.
func placeZone(e *core.RequestEvent, gameRec, me *core.Record, spec game.JokerSpec, req useJokerRequest, now time.Time) error {
	app := e.App

	col, err := app.FindCollectionByNameOrId(schema.ColZones)
	if err != nil {
		return err
	}

	rec := core.NewRecord(col)
	rec.Set("game", gameRec.Id)
	rec.Set("team", me.Id)
	rec.Set("expires_at", now.Add(spec.Duration))
	rec.Set("triggered", false)

	if spec.Kind == game.JokerLockdown {
		if req.SectorID == "" {
			return fmt.Errorf("für die Sperrzone fehlt der Sektor")
		}
		sector, err := ausSpiel(e, schema.ColSectors, req.SectorID, gameRec.Id)
		if err != nil {
			return fmt.Errorf("diesen Sektor gibt es nicht")
		}
		rec.Set("kind", "lockdown")
		rec.Set("sector", sector.Id)
	} else {
		if req.HotspotID == "" {
			return fmt.Errorf("für die Wanze fehlt der Hotspot")
		}
		hotspot, err := ausSpiel(e, schema.ColHotspots, req.HotspotID, gameRec.Id)
		if err != nil {
			return fmt.Errorf("diesen Hotspot gibt es nicht")
		}
		rec.Set("kind", "bug")
		rec.Set("hotspot", hotspot.Id)
		rec.Set("lat", hotspot.GetFloat("lat"))
		rec.Set("lng", hotspot.GetFloat("lng"))
		rec.Set("radius_m", 120.0)
	}

	return app.Save(rec)
}

// misterXInSector beantwortet die Frage des Bereichsscans.
func misterXInSector(app core.App, gameID string, sector *core.Record) (bool, error) {
	misterX, err := app.FindFirstRecordByFilter(schema.ColTeams,
		"game = {:g} && role = {:r}",
		map[string]any{"g": gameID, "r": schema.RoleMisterX})
	if err != nil || misterX == nil {
		return false, err
	}

	pos := latestPosition(app, misterX.Id)
	if pos == nil {
		return false, nil
	}

	area, err := geo.AreaFromGeoJSON(toRaw(sector.Get("geometry")))
	if err != nil {
		return false, err
	}

	return area.Contains(geo.Point{Lat: pos.GetFloat("lat"), Lng: pos.GetFloat("lng")}), nil
}

func sectorCodes(app core.App, gameID string) ([]string, error) {
	sectors, err := app.FindRecordsByFilter(schema.ColSectors, "game = {:g}", "code", 0, 0,
		map[string]any{"g": gameID})
	if err != nil {
		return nil, err
	}

	codes := make([]string, 0, len(sectors))
	for _, s := range sectors {
		codes = append(codes, s.GetString("code"))
	}
	return codes, nil
}
