package game

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/geo"
	"github.com/elias/operation-x/pkg/hub"
	"github.com/elias/operation-x/pkg/schema"
)

// Ereignisarten. Der Typ steht im Protokoll und begründet jede Buchung.
const (
	EventPingLate     = "ping.late"
	EventPingWarning  = "ping.warning"
	EventPingLockout  = "ping.lockout"
	EventPosition     = "position.received"
	EventTransitStart = "transit.start"
	EventTransitEnd   = "transit.end"
	EventImplausible  = "position.implausible"
	EventMocked       = "position.mocked"

	EventMissionStart  = "mission.start"
	EventMissionDone   = "mission.done"
	EventMissionAbort  = "mission.abort"
	EventMissionFailed = "mission.failed"
	EventEvidenceOK    = "evidence.accepted"
	EventEvidenceNo    = "evidence.rejected"

	EventSighting      = "sighting.reported"
	EventSightingOK    = "sighting.confirmed"
	EventArrestPartial = "arrest.partial"
	EventArrestWin     = "arrest.success"
	EventArrestFail    = "arrest.failed"
	EventFinaleStart   = "finale.start"
	EventFinaleShrink  = "finale.shrink"
	EventEscape        = "game.escaped"
	EventGameOver      = "game.over"

	EventCamping      = "camping"
	EventStartFP      = "team.start"
	EventPurged       = "data.purged"
	EventMissionGrace = "mission.grace"
	EventMissionDelay = "mission.delay"
)

// distanceBetween liefert den Abstand zwischen einer Positionsmeldung und einem
// Hotspot in Metern.
func distanceBetween(pos, hotspot *core.Record) float64 {
	return geo.DistanceM(
		geo.Point{Lat: pos.GetFloat("lat"), Lng: pos.GetFloat("lng")},
		geo.Point{Lat: hotspot.GetFloat("lat"), Lng: hotspot.GetFloat("lng")},
	)
}

// Booking ist eine Buchung im Protokoll.
//
// Jede Regelanwendung schreibt eine – Punktestände und Fluchtpunkte werden
// daraus abgeleitet, nie einfach überschrieben. Das kostet etwas mehr Arbeit
// und zahlt sich dreifach aus: Bei Streit ("warum habe ich minus 15?") ist jede
// Buchung begründbar, ein Neustart mitten im Spiel verliert nichts, und am Ende
// fällt eine vollständige Nachbetrachtung ab.
type Booking struct {
	Game   string
	Team   string
	Type   string
	Reason string

	DeltaPoints int
	DeltaFP     int

	Payload map[string]any

	// DedupeKey verhindert, dass ein wiederholter Durchlauf der Engine
	// dieselbe Konsequenz zweimal bucht. Leer heißt: immer schreiben.
	DedupeKey string

	OccurredAt time.Time
}

// Book schreibt ein Ereignis und, falls es Punkte oder Fluchtpunkte bewegt,
// die zugehörige Kontobuchung. Läuft in einer Transaktion.
//
// Liefert false zurück, wenn die Buchung wegen ihres DedupeKey übersprungen
// wurde – der Aufrufer weiß dann, dass nichts passiert ist.
//
// ACHTUNG: Book lädt den Team-Datensatz selbst und schreibt Punkte und
// Fluchtpunkte darauf. Wer vor dem Aufruf ein eigenes Exemplar desselben Teams
// in der Hand hält, muss es danach als veraltet betrachten und neu laden – sonst
// überschreibt das Zurückspeichern die gerade gebuchte Strafe wieder.
func Book(app core.App, b Booking) (bool, error) {
	if b.OccurredAt.IsZero() {
		b.OccurredAt = time.Now()
	}

	var written bool

	err := app.RunInTransaction(func(tx core.App) error {
		if b.DedupeKey != "" {
			existing, err := tx.FindFirstRecordByFilter(
				schema.ColEvents,
				"dedupe_key = {:k}",
				map[string]any{"k": b.DedupeKey},
			)
			if err == nil && existing != nil {
				return nil // schon gebucht
			}
		}

		eventsCol, err := tx.FindCollectionByNameOrId(schema.ColEvents)
		if err != nil {
			return err
		}

		ev := core.NewRecord(eventsCol)
		ev.Set("game", b.Game)
		if b.Team != "" {
			ev.Set("team", b.Team)
		}
		ev.Set("type", b.Type)
		ev.Set("reason", b.Reason)
		ev.Set("delta_points", b.DeltaPoints)
		ev.Set("delta_fp", b.DeltaFP)
		ev.Set("occurred_at", b.OccurredAt)
		ev.Set("dedupe_key", b.DedupeKey)
		if b.Payload != nil {
			ev.Set("payload", b.Payload)
		}

		if err := tx.Save(ev); err != nil {
			return fmt.Errorf("Ereignis %q schreiben: %w", b.Type, err)
		}

		written = true

		if b.Team == "" || (b.DeltaPoints == 0 && b.DeltaFP == 0) {
			return nil
		}

		ledgerCol, err := tx.FindCollectionByNameOrId(schema.ColLedger)
		if err != nil {
			return err
		}

		entry := core.NewRecord(ledgerCol)
		entry.Set("game", b.Game)
		entry.Set("team", b.Team)
		entry.Set("delta_points", b.DeltaPoints)
		entry.Set("delta_fp", b.DeltaFP)
		entry.Set("reason", b.Reason)
		entry.Set("ref_type", "event")
		entry.Set("ref_id", ev.Id)
		entry.Set("occurred_at", b.OccurredAt)

		if err := tx.Save(entry); err != nil {
			return fmt.Errorf("Kontobuchung schreiben: %w", err)
		}

		// Der Stand am Team ist eine Zwischenspeicherung für die Anzeige;
		// die Wahrheit steht im Kontobuch.
		team, err := tx.FindRecordById(schema.ColTeams, b.Team)
		if err != nil {
			return err
		}
		team.Set("points", team.GetInt("points")+b.DeltaPoints)
		team.Set("fp", max(0, team.GetInt("fp")+b.DeltaFP))

		return tx.Save(team)
	})

	// Wer offen mithört, erfährt sofort davon. Nach der Transaktion, damit
	// niemand einen Zwischenstand zu sehen bekommt, und nur bei Erfolg.
	if err == nil && written {
		hub.Notify()
	}

	return written, err
}

// BookStart schreibt das Startguthaben ins Kontobuch.
//
// Es stand vorher nur am Team, wie eine Zahl, die schon immer da war. Damit war
// das Kontobuch aber nicht mehr die Wahrheit, für die es angelegt wurde: Wer es
// nachrechnete, kam auf ein anderes Guthaben als das angezeigte, und die erste
// Zeile im Punktekonto fehlte – ausgerechnet die, die erklärt, wovon jemand
// überhaupt ausgegangen ist.
//
// Der Wiederholungsschutz ist kein Beiwerk: Ohne ihn buchte ein zweiter Aufruf
// dasselbe Guthaben noch einmal obendrauf.
func BookStart(app core.App, gameID, teamID string, fp int) error {
	if fp == 0 {
		return nil
	}

	_, err := Book(app, Booking{
		Game:      gameID,
		Team:      teamID,
		Type:      EventStartFP,
		Reason:    "Startguthaben",
		DeltaFP:   fp,
		DedupeKey: "start:" + teamID,
	})
	return err
}

// Penalize verhängt eine Sperre.
func Penalize(app core.App, gameID, teamID, kind, reason string, until time.Time, lat, lng float64) error {
	col, err := app.FindCollectionByNameOrId(schema.ColPenalties)
	if err != nil {
		return err
	}

	rec := core.NewRecord(col)
	rec.Set("game", gameID)
	rec.Set("team", teamID)
	rec.Set("kind", kind)
	rec.Set("reason", reason)
	rec.Set("started_at", time.Now())
	rec.Set("expires_at", until)
	rec.Set("active", true)
	if lat != 0 || lng != 0 {
		rec.Set("lat", lat)
		rec.Set("lng", lng)
	}

	return app.Save(rec)
}

// ActivePenalties liefert die laufenden Sperren eines Teams.
func ActivePenalties(app core.App, teamID string) ([]*core.Record, error) {
	return app.FindRecordsByFilter(
		schema.ColPenalties,
		"team = {:t} && active = true && expires_at > {:now}",
		"-expires_at", 0, 0,
		map[string]any{"t": teamID, "now": DBTime(time.Now())},
	)
}
