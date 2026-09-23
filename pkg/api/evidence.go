package api

import (
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/game"
	"github.com/elias/operation-x/pkg/schema"
)

type evidenceEntry struct {
	ID          string `json:"id"`
	Team        string `json:"team"`
	Callsign    string `json:"callsign"`
	Mission     string `json:"mission"`
	Seq         int    `json:"seq"`
	HotspotNo   int    `json:"hotspotNumber"`
	HotspotName string `json:"hotspotName"`
	// Die Aufgabe, die an diesem Ort gestellt war. Ohne sie prüft die Zentrale
	// ein Foto, ohne zu wissen, was darauf zu sehen sein sollte.
	Task       string  `json:"task,omitempty"`
	DistanceM  float64 `json:"distanceM"`
	Passcode   string  `json:"passcodeEntered,omitempty"`
	PhotoURL   string  `json:"photoUrl,omitempty"`
	CapturedAt string  `json:"capturedAt"`
	Status     string  `json:"status"`
	Note       string  `json:"note,omitempty"`
}

// handleEvidenceQueue liefert der Zentrale die offenen Nachweise.
//
// Ein Foto zu prüfen dauert Sekunden, aber es muss zuverlässig geschehen:
// Solange nichts entschieden ist, steht Mister X am Ziel und wartet, während
// seine Frist läuft. Deshalb stehen die wartenden Einreichungen oben.
func handleEvidenceQueue(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	records, err := e.App.FindRecordsByFilter(
		schema.ColEvidence, "game = {:g}", "-captured_at", 60, 0,
		map[string]any{"g": gameRec.Id},
	)
	if err != nil {
		return err
	}

	out := make([]evidenceEntry, 0, len(records))
	for _, r := range records {
		entry := evidenceEntry{
			ID:         r.Id,
			Team:       r.GetString("team"),
			Mission:    r.GetString("mission"),
			DistanceM:  r.GetFloat("distance_m"),
			Passcode:   r.GetString("passcode_entered"),
			CapturedAt: r.GetDateTime("captured_at").Time().UTC().Format(time.RFC3339),
			Status:     r.GetString("status"),
			Note:       r.GetString("note"),
		}

		if t, err := e.App.FindRecordById(schema.ColTeams, entry.Team); err == nil {
			entry.Callsign = t.GetString("callsign")
		}
		if h, err := e.App.FindRecordById(schema.ColHotspots, r.GetString("hotspot")); err == nil {
			entry.HotspotNo = h.GetInt("number")
			entry.HotspotName = h.GetString("name")
			entry.Task = h.GetString("notes")
		}
		if m, err := e.App.FindRecordById(schema.ColMissions, entry.Mission); err == nil {
			entry.Seq = m.GetInt("seq")
		}
		if file := r.GetString("photo"); file != "" {
			entry.PhotoURL = "/api/opx/hq/evidence/" + r.Id + "/foto"
		}

		out = append(out, entry)
	}

	// Wartende zuerst – sie halten das Spiel auf.
	pending := make([]evidenceEntry, 0, len(out))
	rest := make([]evidenceEntry, 0, len(out))
	for _, x := range out {
		if x.Status == "pending" {
			pending = append(pending, x)
		} else {
			rest = append(rest, x)
		}
	}

	return e.JSON(http.StatusOK, map[string]any{"evidence": append(pending, rest...)})
}

type reviewRequest struct {
	Accept bool   `json:"accept"`
	Note   string `json:"note"`
}

// handleReviewEvidence nimmt ein eingereichtes Foto an oder lehnt es ab.
func handleReviewEvidence(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	rec, err := ausSpiel(e, schema.ColEvidence, e.Request.PathValue("id"), gameRec.Id)
	if err != nil {
		return err
	}
	if rec.GetString("status") != "pending" {
		return e.BadRequestError("Über diesen Nachweis wurde bereits entschieden.", nil)
	}

	var req reviewRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}

	misterX, err := e.App.FindRecordById(schema.ColTeams, rec.GetString("team"))
	if err != nil {
		return e.InternalServerError("Das Team fehlt.", err)
	}

	rec.Set("note", req.Note)
	if req.Accept {
		rec.Set("status", "accepted")
	} else {
		rec.Set("status", "rejected")
	}
	if err := e.App.Save(rec); err != nil {
		return e.InternalServerError("Entscheidung konnte nicht gespeichert werden.", err)
	}

	mission, err := e.App.FindRecordById(schema.ColMissions, rec.GetString("mission"))
	if err != nil {
		return e.JSON(http.StatusOK, map[string]any{"ok": true})
	}

	if req.Accept {
		if err := completeMission(e.App, gameRec, misterX, mission); err != nil {
			return e.InternalServerError("Mission konnte nicht abgeschlossen werden.", err)
		}
		_, _ = game.Book(e.App, game.Booking{
			Game: gameRec.Id, Team: misterX.Id,
			Type:      game.EventEvidenceOK,
			Reason:    "Nachweis von der Zentrale angenommen",
			DedupeKey: "evidence.ok:" + rec.Id,
		})
	} else {
		// Die Mission läuft weiter – wer abgelehnt wird, kann es erneut
		// versuchen, solange die Frist läuft.
		_, _ = game.Book(e.App, game.Booking{
			Game: gameRec.Id, Team: misterX.Id,
			Type:      game.EventEvidenceNo,
			Reason:    "Nachweis abgelehnt: " + req.Note,
			DedupeKey: "evidence.no:" + rec.Id,
		})
	}

	return e.JSON(http.StatusOK, map[string]any{"ok": true, "accepted": req.Accept})
}

// handleEvidencePhoto liefert ein Beweisfoto aus.
//
// Bis hierher lief das über den allgemeinen Dateiweg von PocketBase. Der
// fragt bei unbewachten Feldern niemanden nach einer Anmeldung: Wer die
// Adresse kennt, bekommt das Bild — und diese Bilder entstehen im öffentlichen
// Raum, mit Leuten darauf, die von diesem Spiel nichts wissen. Die Adresse ist
// zwar nicht zu erraten, aber "nicht zu erraten" ist keine Zugangsprüfung.
//
// Deshalb derselbe Weg wie für alles andere: über /api/opx, mit Rolle und
// Spiel geprüft.
func handleEvidencePhoto(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	rec, err := ausSpiel(e, schema.ColEvidence, e.Request.PathValue("id"), gameRec.Id)
	if err != nil {
		return err
	}

	name := rec.GetString("photo")
	if name == "" {
		return e.NotFoundError("Zu dieser Einreichung gibt es kein Foto.", nil)
	}

	fsys, err := e.App.NewFilesystem()
	if err != nil {
		return e.InternalServerError("Der Dateispeicher ist nicht lesbar.", err)
	}
	defer fsys.Close()

	return fsys.Serve(e.Response, e.Request, rec.BaseFilesPath()+"/"+name, name)
}
