package game

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/schema"
)

// Aufbewahrung der Standortdaten.
//
// Das Konzept sagt zu, dass sich nach dem Spiel alles Ortsbezogene von selbst
// löscht. Diese Zusage ist der Grund, warum acht Leute bereit sind, einen Tag
// lang ihre Position an einen Laptop zu schicken – sie einzulösen ist deshalb
// keine Aufräumarbeit, sondern Teil des Versprechens.
//
// Gelöscht wird die Bewegungsspur: wer wann wo war. Nicht gelöscht wird, was
// das Spiel ausgemacht hat – Punkte, Buchungen, der Ausgang. Ein Protokoll
// ohne Koordinaten verrät niemanden mehr, taugt aber noch für die Frage,
// warum ein Team am Ende zwanzig Punkte weniger hatte.

// purgeDue löscht die Bewegungsdaten aller Spiele, deren Frist abgelaufen ist.
//
// Läuft über alle Spiele, nicht nur über laufende – ein beendetes Spiel ist ja
// gerade der Normalfall für diese Prüfung.
func (e *Engine) purgeDue(now time.Time) error {
	games, err := e.app.FindRecordsByFilter(
		schema.ColGames,
		"purge_at != '' && purge_at <= {:now}",
		"", 0, 0,
		map[string]any{"now": DBTime(now)},
	)
	if err != nil {
		return fmt.Errorf("fällige Löschungen suchen: %w", err)
	}

	for _, gameRec := range games {
		if err := e.purgeGame(gameRec); err != nil {
			e.log.Error("Löschen fehlgeschlagen", "spiel", gameRec.Id, "fehler", err)
			continue
		}
	}
	return nil
}

// purgeGame entfernt die Bewegungsspur eines Spiels.
func (e *Engine) purgeGame(gameRec *core.Record) error {
	// Die Reihenfolge ist gleichgültig, die Vollständigkeit nicht: Positionen
	// tragen die Koordinaten, Meldungen und Sichtkontakte den Ort der
	// Handlung. Alle drei zusammen ergeben die Spur.
	var removed int

	for _, collection := range []string{
		schema.ColPositions,
		schema.ColPings,
		schema.ColSightings,
	} {
		records, err := e.app.FindRecordsByFilter(
			collection, "game = {:g}", "", 0, 0,
			map[string]any{"g": gameRec.Id},
		)
		if err != nil {
			return fmt.Errorf("%s lesen: %w", collection, err)
		}
		for _, rec := range records {
			if err := e.app.Delete(rec); err != nil {
				return fmt.Errorf("%s löschen: %w", collection, err)
			}
			removed++
		}
	}

	// Was nicht gelöscht, sondern entschärft wird.
	//
	// Diese drei Stellen tragen einen Ort mit sich, gehören aber zur
	// Begründung des Punktestands: ein Beweisfoto samt Aufnahmeort, eine
	// Sperre und der Ort, an dem sie gilt, und die Meldung über eine
	// vorgetäuschte Position. Sie ganz zu löschen nähme dem Protokoll die
	// Antwort auf "warum habe ich minus 15?". Also fällt der Ort, der Eintrag
	// bleibt.
	entschaerft, err := e.spurenEntfernen(gameRec.Id)
	if err != nil {
		return err
	}
	removed += entschaerft

	// Die Frist zurücksetzen, sonst liefe die Prüfung bei jedem Takt erneut
	// über dasselbe Spiel.
	gameRec.Set("purge_at", "")
	gameRec.Set("purged_at", time.Now())
	if err := e.app.Save(gameRec); err != nil {
		return err
	}

	_, err = Book(e.app, Booking{
		Game:      gameRec.Id,
		Type:      EventPurged,
		Reason:    fmt.Sprintf("Standortdaten gelöscht (%d Datensätze)", removed),
		DedupeKey: "purge:" + gameRec.Id,
		Payload:   map[string]any{"geloescht": removed},
	})
	if err != nil {
		return err
	}

	e.log.Info("Standortdaten gelöscht", "spiel", gameRec.Id, "datensaetze", removed)
	return nil
}

// spurenEntfernen nimmt den Ort aus Einträgen, die bleiben sollen.
//
// Das Beweisfoto wird dabei wirklich gelöscht, nicht nur entkoppelt: Es ist
// im öffentlichen Raum entstanden und zeigt im Zweifel Leute, die von diesem
// Spiel nichts wissen. Was bleibt, ist "angenommen" oder "abgelehnt" – und
// das ist alles, was der Punktestand davon braucht.
func (e *Engine) spurenEntfernen(gameID string) (int, error) {
	var berührt int

	ortLoeschen := func(collection string, zusatz func(*core.Record)) error {
		records, err := e.app.FindRecordsByFilter(
			collection, "game = {:g}", "", 0, 0,
			map[string]any{"g": gameID},
		)
		if err != nil {
			return fmt.Errorf("%s lesen: %w", collection, err)
		}
		for _, rec := range records {
			rec.Set("lat", 0)
			rec.Set("lng", 0)
			if zusatz != nil {
				zusatz(rec)
			}
			if err := e.app.Save(rec); err != nil {
				return fmt.Errorf("%s entschärfen: %w", collection, err)
			}
			berührt++
		}
		return nil
	}

	if err := ortLoeschen(schema.ColEvidence, func(rec *core.Record) {
		// Leeres Dateifeld: PocketBase räumt die Datei beim Speichern weg.
		rec.Set("photo", "")
		rec.Set("distance_m", 0)
	}); err != nil {
		return berührt, err
	}

	if err := ortLoeschen(schema.ColPenalties, nil); err != nil {
		return berührt, err
	}

	// Die Meldung über eine vorgetäuschte Position trägt die Koordinate im
	// Anhang. Nur dieser eine Ereignistyp tut das.
	events, err := e.app.FindRecordsByFilter(
		schema.ColEvents, "game = {:g} && type = {:t}", "", 0, 0,
		map[string]any{"g": gameID, "t": string(EventMocked)},
	)
	if err != nil {
		return berührt, fmt.Errorf("Ereignisse lesen: %w", err)
	}
	for _, rec := range events {
		rec.Set("payload", map[string]any{})
		if err := e.app.Save(rec); err != nil {
			return berührt, fmt.Errorf("Ereignis entschärfen: %w", err)
		}
		berührt++
	}

	return berührt, nil
}

// PurgeNow löscht die Bewegungsspur eines Spiels sofort.
//
// Für zwei Fälle, die beide keine Frist abwarten können: Jemand widerruft
// seine Einwilligung, oder die Spielleitung will nach dem Ausklang nicht
// warten, bis der Laptop zufällig noch einmal läuft. Beides muss mit einem
// Knopf gehen, nicht über die Datenbankverwaltung.
func PurgeNow(app core.App, gameRec *core.Record) error {
	return New(app).purgeGame(gameRec)
}

// SchedulePurge legt fest, wann die Bewegungsdaten eines Spiels verschwinden.
//
// Wird beim Spielende gerufen. Ohne Eintrag im Spiel wird eine Frist von
// 24 Stunden angenommen – lieber eine Frist zu viel als eine Spur, die bleibt.
// Ein ausdrückliches "sofort" (-1) wird dagegen befolgt: Die Frist steht dann
// in der Vergangenheit und der nächste Takt räumt auf.
func SchedulePurge(app core.App, gameRec *core.Record) error {
	hours := gameRec.GetInt("retention_hours")
	switch {
	case hours < 0:
		hours = 0
	case hours == 0:
		hours = 24
	}
	gameRec.Set("purge_at", time.Now().Add(time.Duration(hours)*time.Hour))
	return app.Save(gameRec)
}
