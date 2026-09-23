package api

import (
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/game"
	"github.com/elias/operation-x/pkg/geo"
	"github.com/elias/operation-x/pkg/schema"
)

// Oberhalb dieser Geschwindigkeit ist eine Positionsänderung nicht mehr zu Fuß,
// mit Rad oder Nahverkehr erklärbar. Die Meldung wird dann markiert und dem HQ
// vorgelegt – nicht automatisch bestraft, denn ein Sprung entsteht auch, wenn
// ein Gerät nach längerem Funkloch wieder einen Fix bekommt.
const implausibleSpeedKmh = 160

type positionReport struct {
	Lat        float64 `json:"lat"`
	Lng        float64 `json:"lng"`
	Accuracy   float64 `json:"accuracy"`
	Speed      float64 `json:"speed"`
	Heading    float64 `json:"heading"`
	CapturedAt string  `json:"capturedAt"`
	Mocked     bool    `json:"mocked"`
}

type positionRequest struct {
	// Mehrere Meldungen auf einmal: Die App puffert im Funkloch und reicht sie
	// später nach.
	Positions []positionReport `json:"positions"`
}

type positionResponse struct {
	Accepted  int      `json:"accepted"`
	NextDueAt string   `json:"nextDueAt"`
	Warnings  []string `json:"warnings,omitempty"`
	// Während einer Pause wird nichts aufgezeichnet; die App hört damit auf zu
	// senden, statt den Puffer volllaufen zu lassen.
	Paused bool `json:"paused,omitempty"`
}

// handlePosition nimmt Standortmeldungen entgegen.
//
// Maßgeblich ist der Erfassungszeitpunkt auf dem Gerät, nicht der Eingang beim
// Server: Eine im Funkloch gepufferte Meldung behält ihre Originalzeit, sonst
// bekäme ein Spieler eine Strafe dafür, dass die U-Bahn keinen Empfang hat.
func handlePosition(e *core.RequestEvent) error {
	team := e.Auth
	gameID := team.GetString("game")
	if gameID == "" {
		return e.BadRequestError("Dieser Zugang gehört zu keinem Spiel.", nil)
	}

	game_, err := e.App.FindRecordById(schema.ColGames, gameID)
	if err != nil {
		return e.NotFoundError("Das Spiel wurde nicht gefunden.", nil)
	}

	var req positionRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}
	if len(req.Positions) == 0 {
		return e.BadRequestError("Es wurden keine Positionen übergeben.", nil)
	}
	if len(req.Positions) > 200 {
		return e.BadRequestError("Zu viele Positionen auf einmal.", nil)
	}

	cfg := game.ConfigOf(game_)
	now := time.Now()

	col, err := e.App.FindCollectionByNameOrId(schema.ColPositions)
	if err != nil {
		return err
	}

	// Während einer Pause wird nicht mitgeschrieben.
	//
	// Die Pause ist für das Mittagessen da, für den Gang aufs Klo, für den Weg
	// zum Treffpunkt. In dieser Zeit einen Standortverlauf aufzuzeichnen wäre
	// für das Spiel nutzlos und für die Beteiligten übergriffig – es steht
	// auch nichts davon im Regelwerk. Die Prüfung sitzt hier und nicht in der
	// App, damit sie für jedes Gerät gilt, auch für den Browser.
	//
	// Maßgeblich ist der Erfassungszeitpunkt: Eine Meldung von vor der Pause,
	// die erst jetzt durchkommt, gehört ins Spiel und wird angenommen.
	pausedAt := game_.GetDateTime("paused_at").Time()
	inPause := game_.GetString("status") == schema.GamePaused

	previous := latestPosition(e.App, team.Id)
	var warnings []string
	accepted := 0
	skipped := 0
	var newest time.Time

	for _, p := range req.Positions {
		if math.Abs(p.Lat) > 90 || math.Abs(p.Lng) > 180 || (p.Lat == 0 && p.Lng == 0) {
			warnings = append(warnings, "Eine Meldung hatte unbrauchbare Koordinaten.")
			continue
		}

		captured := parseTime(p.CapturedAt, now)
		// Meldungen aus der Zukunft deuten auf eine falsch gestellte Uhr hin.
		// Sie zu übernehmen hieße, die nächste Frist zu verschenken.
		if captured.After(now.Add(2 * time.Minute)) {
			captured = now
			warnings = append(warnings, "Die Uhr des Geräts geht vor; Zeitstempel korrigiert.")
		}

		// Nicht After, sondern "nicht davor": Eine Meldung aus genau der
		// Sekunde, in der die Pause ausgerufen wurde, gehört schon zur Pause.
		if inPause && (pausedAt.IsZero() || !captured.Before(pausedAt)) {
			skipped++
			continue
		}

		rec := core.NewRecord(col)
		rec.Set("game", gameID)
		rec.Set("team", team.Id)
		rec.Set("lat", p.Lat)
		rec.Set("lng", p.Lng)
		rec.Set("accuracy", p.Accuracy)
		rec.Set("speed", p.Speed)
		rec.Set("heading", p.Heading)
		rec.Set("captured_at", captured)
		rec.Set("source", "gps")
		rec.Set("mocked", p.Mocked)
		rec.Set("in_transit", team.GetBool("in_transit"))

		if err := e.App.Save(rec); err != nil {
			return e.InternalServerError("Position konnte nicht gespeichert werden.", err)
		}

		accepted++
		if captured.After(newest) {
			newest = captured
		}

		if p.Mocked {
			_, _ = game.Book(e.App, game.Booking{
				Game: gameID, Team: team.Id,
				Type:       game.EventMocked,
				Reason:     "Position stammt laut Gerät aus einer Fake-GPS-App",
				OccurredAt: captured,
				DedupeKey:  fmt.Sprintf("mocked:%s:%d", team.Id, captured.Unix()),
				Payload:    map[string]any{"lat": p.Lat, "lng": p.Lng},
			})
			warnings = append(warnings, "Simulierter Standort erkannt und dem HQ gemeldet.")
		}

		// Plausibilität gegen die vorherige Meldung.
		if previous != nil {
			if kmh, ok := speedBetween(previous, p.Lat, p.Lng, captured); ok && kmh > implausibleSpeedKmh {
				_, _ = game.Book(e.App, game.Booking{
					Game: gameID, Team: team.Id,
					Type:       game.EventImplausible,
					Reason:     fmt.Sprintf("Positionssprung mit rechnerisch %.0f km/h", kmh),
					OccurredAt: captured,
					DedupeKey:  fmt.Sprintf("jump:%s:%d", team.Id, captured.Unix()),
					Payload:    map[string]any{"kmh": math.Round(kmh)},
				})
			}
		}

		previous = rec
	}

	// Nur verworfen, weil Pause ist: Das ist kein Fehler, sondern die Regel.
	// Eine Fehlermeldung würde die App in eine Wiederholschleife schicken.
	if accepted == 0 && skipped > 0 {
		return e.JSON(http.StatusOK, positionResponse{
			Accepted: 0,
			Paused:   true,
			Warnings: []string{"Pause – währenddessen wird kein Standort aufgezeichnet."},
		})
	}

	if accepted == 0 {
		return e.BadRequestError("Keine der Meldungen war brauchbar.", map[string]any{"warnings": warnings})
	}

	// Frist neu setzen. Gemessen ab dem Erfassungszeitpunkt der jüngsten
	// Meldung, nicht ab dem Eingang.
	interval := cfg.PingInterval(team.GetBool("in_transit"))
	due := newest.Add(interval)

	// Frisch laden: Oben können Buchungen gelaufen sein, die den Teamstand
	// verändert haben (siehe game.Book). Unser Exemplar stammt vom Anfang der
	// Anfrage und würde diese Änderungen überschreiben.
	fresh, err := e.App.FindRecordById(schema.ColTeams, team.Id)
	if err != nil {
		return e.InternalServerError("Teamstand konnte nicht gelesen werden.", err)
	}
	fresh.Set("last_seen_at", newest)
	fresh.Set("ping_due_at", due)
	if err := e.App.Save(fresh); err != nil {
		return e.InternalServerError("Teamstand konnte nicht aktualisiert werden.", err)
	}

	return e.JSON(http.StatusOK, positionResponse{
		Accepted:  accepted,
		NextDueAt: due.UTC().Format(time.RFC3339),
		Warnings:  warnings,
	})
}

type transitRequest struct {
	Active bool `json:"active"`
}

// handleTransit schaltet den Transitmodus.
//
// Im Transit steigt die Ping-Toleranz von 10 auf 13 Minuten, damit
// Verbindungsabbrüche in Bahn und Bus nicht zu Strafen führen.
func handleTransit(e *core.RequestEvent) error {
	team := e.Auth
	gameID := team.GetString("game")

	var req transitRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}

	if team.GetBool("in_transit") == req.Active {
		return e.JSON(http.StatusOK, map[string]any{"inTransit": req.Active})
	}

	game_, err := e.App.FindRecordById(schema.ColGames, gameID)
	if err != nil {
		return e.NotFoundError("Das Spiel wurde nicht gefunden.", nil)
	}
	cfg := game.ConfigOf(game_)

	// ACHTUNG: Nicht e.Auth speichern.
	//
	// e.Auth ist die Kopie, die beim Anmelden der Anfrage geladen wurde. Bucht
	// die Engine in der Zwischenzeit etwas – und sie läuft alle fünf Sekunden –,
	// schriebe ein Save dieser Kopie den alten Punktestand zurück und machte
	// die Buchung stillschweigend rückgängig. Genau dieser Fehler hat schon
	// einmal Strafpunkte verschluckt; siehe die Fallstricke im README.
	fresh, err := e.App.FindRecordById(schema.ColTeams, team.Id)
	if err != nil {
		return e.InternalServerError("Das Team wurde nicht gefunden.", err)
	}

	fresh.Set("in_transit", req.Active)

	// Die laufende Frist an das neue Intervall anpassen, gerechnet ab der
	// letzten Meldung – wer in die Bahn steigt, soll die längere Toleranz
	// sofort haben und nicht erst nach dem nächsten Ping.
	last := fresh.GetDateTime("last_seen_at").Time()
	if last.IsZero() {
		last = time.Now()
	}
	fresh.Set("ping_due_at", last.Add(cfg.PingInterval(req.Active)))

	if err := e.App.Save(fresh); err != nil {
		return e.InternalServerError("Transitstatus konnte nicht gesetzt werden.", err)
	}

	evType := game.EventTransitEnd
	reason := "Transit beendet"
	if req.Active {
		evType = game.EventTransitStart
		reason = "Transit begonnen"
	}
	_, _ = game.Book(e.App, game.Booking{
		Game: gameID, Team: team.Id, Type: evType, Reason: reason,
	})

	return e.JSON(http.StatusOK, map[string]any{
		"inTransit": req.Active,
		"nextDueAt": team.GetDateTime("ping_due_at").Time().UTC().Format(time.RFC3339),
	})
}

// latestPosition liefert die jüngste Meldung eines Teams.
func latestPosition(app core.App, teamID string) *core.Record {
	recs, err := app.FindRecordsByFilter(
		schema.ColPositions, "team = {:t}", "-captured_at", 1, 0,
		map[string]any{"t": teamID},
	)
	if err != nil || len(recs) == 0 {
		return nil
	}
	return recs[0]
}

// speedBetween rechnet die Geschwindigkeit zwischen zwei Meldungen aus.
func speedBetween(prev *core.Record, lat, lng float64, at time.Time) (float64, bool) {
	prevAt := prev.GetDateTime("captured_at").Time()
	seconds := at.Sub(prevAt).Seconds()
	if seconds <= 1 {
		return 0, false
	}

	meters := geo.DistanceM(
		geo.Point{Lat: prev.GetFloat("lat"), Lng: prev.GetFloat("lng")},
		geo.Point{Lat: lat, Lng: lng},
	)

	return (meters / seconds) * 3.6, true
}

func parseTime(value string, fallback time.Time) time.Time {
	if value == "" {
		return fallback
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999Z"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}
	return fallback
}
