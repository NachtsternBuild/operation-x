package game

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/schema"
)

// tickInterval ist der Takt, in dem geprüft wird, was fällig geworden ist.
//
// Fünf Sekunden reichen: Die kürzeste Frist im Regelwerk sind fünf Minuten, und
// niemand merkt den Unterschied zwischen einer Strafe, die auf die Sekunde
// genau greift, und einer, die es fünf Sekunden später tut. Ein Sekundentakt
// wäre auf einem Laptop, der nebenbei sechs Stunden laufen soll, verschwendete
// Batterie.
const tickInterval = 5 * time.Second

// Engine prüft im Hintergrund, welche Regelfolgen fällig sind.
//
// Jeder Durchlauf fragt „welche Konsequenz ist fällig und noch nicht gebucht?“
// und schreibt das Ergebnis als Ereignis. Zweimal derselbe Durchlauf verhängt
// deshalb keine zwei Strafen, und ein Neustart mitten im Spiel holt Verpasstes
// nach.
type Engine struct {
	app core.App
	log *slog.Logger

	// Wann zuletzt nach liegengebliebenen Spielen gesehen wurde. Die Prüfung
	// kostet mehrere Abfragen je Spiel und gehört nicht in jeden Takt.
	lastIdleCheck time.Time
}

func New(app core.App) *Engine {
	return &Engine{app: app, log: slog.Default()}
}

// Start lässt die Engine laufen, bis der Kontext endet.
func (e *Engine) Start(ctx context.Context) {
	ticker := time.NewTicker(tickInterval)

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := e.Tick(time.Now()); err != nil {
					// Ein Fehler darf die Engine nicht anhalten – beim nächsten
					// Durchlauf wird ohnehin alles erneut geprüft.
					e.log.Error("Engine-Durchlauf fehlgeschlagen", "fehler", err)
				}
			}
		}
	}()
}

// Tick führt einen Durchlauf aus. Öffentlich, damit Tests ihn mit einer
// festen Zeit aufrufen können.
func (e *Engine) Tick(now time.Time) error {
	games, err := e.app.FindRecordsByFilter(
		schema.ColGames,
		"status = {:running}",
		"", 0, 0,
		map[string]any{"running": schema.GameRunning},
	)
	if err != nil {
		return fmt.Errorf("laufende Spiele suchen: %w", err)
	}

	for _, game := range games {
		cfg := ConfigOf(game)

		if err := e.checkPings(game, cfg, now); err != nil {
			e.log.Error("Fristprüfung fehlgeschlagen", "spiel", game.Id, "fehler", err)
		}
		if err := e.checkMissions(game, cfg, now); err != nil {
			e.log.Error("Missionsfristen fehlgeschlagen", "spiel", game.Id, "fehler", err)
		}
		if err := e.checkZones(game, cfg, now); err != nil {
			e.log.Error("Fallen prüfen fehlgeschlagen", "spiel", game.Id, "fehler", err)
		}
		if err := e.checkFinale(game, cfg, now); err != nil {
			e.log.Error("Finale fehlgeschlagen", "spiel", game.Id, "fehler", err)
		}
		if err := e.checkVictory(game, cfg, now); err != nil {
			e.log.Error("Siegermittlung fehlgeschlagen", "spiel", game.Id, "fehler", err)
		}
		if err := e.checkCamping(game, cfg, now); err != nil {
			e.log.Error("Camping-Prüfung fehlgeschlagen", "spiel", game.Id, "fehler", err)
		}
		if err := e.expirePenalties(game, now); err != nil {
			e.log.Error("Sperren aufräumen fehlgeschlagen", "spiel", game.Id, "fehler", err)
		}
	}

	// Außerhalb der Schleife: Die Löschung betrifft gerade die *beendeten*
	// Spiele, die oben nicht mitgesucht werden.
	if err := e.purgeDue(now); err != nil {
		e.log.Error("Löschlauf fehlgeschlagen", "fehler", err)
	}

	return nil
}

// checkPings verhängt die Folgen verpasster Pflichtmeldungen.
//
// Kaskade nach Regelwerk: erster Verzug eine Warnung, zweiter ein Punktabzug,
// ab dem dritten zusätzlich eine Standortsperre.
func (e *Engine) checkPings(game *core.Record, cfg Config, now time.Time) error {
	teams, err := e.app.FindRecordsByFilter(
		schema.ColTeams,
		"game = {:g} && active = true",
		"", 0, 0,
		map[string]any{"g": game.Id},
	)
	if err != nil {
		return err
	}

	fx := ActiveEffects(e.app, game.Id, now)

	for _, team := range teams {
		// Die Einsatzzentrale läuft nicht durch die Stadt.
		if team.GetString("role") == schema.RoleHQ {
			continue
		}

		// Der U-Bahn-Geist setzt die Meldepflicht aus – dafür ist er da.
		if team.GetString("role") == schema.RoleMisterX && fx.GhostActive(now) {
			continue
		}

		due := team.GetDateTime("ping_due_at").Time()
		if due.IsZero() {
			// Erste Frist setzen, sobald ein Team ins Spiel kommt.
			team.Set("ping_due_at", now.Add(cfg.PingInterval(team.GetBool("in_transit"))))
			if err := e.app.Save(team); err != nil {
				return err
			}
			continue
		}

		if now.Before(due) {
			continue
		}

		interval := cfg.PingInterval(team.GetBool("in_transit"))
		violation := team.GetInt("ping_violations") + 1

		booking := Booking{
			Game:       game.Id,
			Team:       team.Id,
			OccurredAt: due,
			// Ein Schlüssel je verpasstem Intervall: Ein Team, das eine Stunde
			// offline war, bekommt für jedes Intervall genau einen Verstoß.
			DedupeKey: fmt.Sprintf("ping:%s:%d", team.Id, due.Unix()),
			Payload: map[string]any{
				"faellig_um":  due.Format(time.RFC3339),
				"verstoss_nr": violation,
				"intervall":   int(interval.Minutes()),
			},
		}

		// Die Schwellen ergeben sich aus der Zahl der folgenlosen Verwarnungen.
		// Sie war bisher fest verdrahtet, während der Regelwert daneben stand
		// und nichts bewirkte – wer ihn änderte, bekam keine Wirkung und keine
		// Meldung.
		warnUntil := cfg.PingWarnAtViolation
		if warnUntil < 0 {
			warnUntil = 0
		}
		lockoutFrom := warnUntil + 2

		switch {
		case violation <= warnUntil:
			booking.Type = EventPingWarning
			booking.Reason = "Standortmeldung überfällig – Verwarnung"
		case violation < lockoutFrom:
			booking.Type = EventPingLate
			booking.Reason = "Standortmeldung erneut überfällig"
			booking.DeltaPoints = cfg.PingPenaltyPoints
		default:
			booking.Type = EventPingLockout
			booking.Reason = fmt.Sprintf("Standortmeldung zum %d. Mal überfällig", violation)
			booking.DeltaPoints = cfg.PingLockoutPoints
		}

		written, err := Book(e.app, booking)
		if err != nil {
			return err
		}

		if written && violation >= lockoutFrom {
			until := now.Add(time.Duration(cfg.PingLockoutMin) * time.Minute)
			if err := Penalize(e.app, game.Id, team.Id, "lockout_location",
				booking.Reason, until,
				lastLat(e.app, team.Id), lastLng(e.app, team.Id)); err != nil {
				return err
			}
		}

		// Frist um genau ein Intervall weiterschieben, nicht auf „jetzt“ setzen:
		// Sonst verschenkte ein Team durch langes Wegbleiben Zeit, statt sie zu
		// verlieren.
		next := due.Add(interval)
		for !next.After(now) {
			next = next.Add(interval)
		}

		// Frisch laden, bevor geschrieben wird: Book() hat den Punktestand
		// dieses Teams in einer eigenen Transaktion verändert, unser Exemplar
		// stammt von davor. Würden wir es zurückschreiben, überschriebe die
		// Fristverwaltung stillschweigend jede gerade gebuchte Strafe.
		fresh, err := e.app.FindRecordById(schema.ColTeams, team.Id)
		if err != nil {
			return err
		}

		fresh.Set("ping_due_at", next)
		if written {
			fresh.Set("ping_violations", violation)
		}
		if err := e.app.Save(fresh); err != nil {
			return err
		}
	}

	return nil
}

// checkMissions ahndet verfehlte Zwischenziele.
//
// Nach Regelwerk kostet das Punkte, sperrt Aktionen für zehn Minuten – und löst
// einen sofortigen Live-Ping aus: Wer seine Frist reißt, verrät der Fahndung
// dafür, wo er steht. Das ist die schärfste Konsequenz im ganzen Regelwerk und
// der Grund, warum die Wahl der Variante wirklich eine Entscheidung ist.
func (e *Engine) checkMissions(game *core.Record, cfg Config, now time.Time) error {
	missions, err := e.app.FindRecordsByFilter(
		schema.ColMissions,
		"game = {:g} && status = 'active' && deadline_at <= {:now}",
		"seq", 0, 0,
		map[string]any{"g": game.Id, "now": DBTime(now)},
	)
	if err != nil {
		return err
	}

	for _, mission := range missions {
		// Liegt ein Nachweis zur Prüfung, ruht die Frist. Wer rechtzeitig ein
		// Foto einreicht, darf nicht dafür bestraft werden, dass die Zentrale
		// nicht sofort hinsieht – die Uhr läuft weiter, sobald abgelehnt wurde.
		if pending, err := e.app.FindFirstRecordByFilter(
			schema.ColEvidence,
			"mission = {:m} && status = 'pending'",
			map[string]any{"m": mission.Id},
		); err == nil && pending != nil {
			continue
		}

		misterX, err := e.app.FindFirstRecordByFilter(
			schema.ColTeams,
			"game = {:g} && role = {:r}",
			map[string]any{"g": game.Id, "r": schema.RoleMisterX},
		)
		if err != nil || misterX == nil {
			continue
		}

		// Steht die Zielperson am Ziel, wird die Frist verlängert statt
		// gerissen. Der Auftrag am Ziel kostet Zeit, die niemand vorhersagen
		// kann – wer in der Schlange an der Kasse steht, hat alles richtig
		// gemacht. Siehe grace.go für die Begründung im Ganzen.
		if AtTarget(e.app, mission, misterX, cfg) {
			granted, err := GrantGrace(
				e.app, game, mission, misterX, cfg,
				cfg.MissionGraceMin, GraceOnSite, now,
			)
			if err != nil {
				return err
			}
			if granted > 0 {
				continue
			}
			// Nichts mehr übrig: Ab hier gilt die Frist wieder. Wer eine halbe
			// Stunde am Ziel steht, steht nicht mehr in der Schlange.
		}

		mission.Set("status", "failed")
		if err := e.app.Save(mission); err != nil {
			return err
		}

		written, err := Book(e.app, Booking{
			Game: game.Id, Team: misterX.Id,
			Type:        EventMissionFailed,
			Reason:      fmt.Sprintf("Zwischenziel %d nicht rechtzeitig erreicht", mission.GetInt("seq")),
			DeltaPoints: cfg.PointsMissionFailed,
			DeltaFP:     -1,
			DedupeKey:   "mission.failed:" + mission.Id,
			OccurredAt:  mission.GetDateTime("deadline_at").Time(),
		})
		if err != nil {
			return err
		}

		if !written {
			continue
		}

		until := now.Add(time.Duration(cfg.MissionFailLockoutMin) * time.Minute)
		if err := Penalize(e.app, game.Id, misterX.Id, "lockout_action",
			"Zwischenziel verfehlt", until, 0, 0); err != nil {
			return err
		}

		// Zwangs-Ping: Die letzte bekannte Position wird als gesicherte
		// Erkenntnis ins Protokoll geschrieben.
		if pos := lastPosition(e.app, misterX.Id); pos != nil {
			_, _ = Book(e.app, Booking{
				Game: game.Id, Team: misterX.Id,
				Type:      "mission.forced_ping",
				Reason:    "Zwangsmeldung nach verfehltem Zwischenziel",
				DedupeKey: "mission.forced_ping:" + mission.Id,
				Payload: map[string]any{
					"lat": pos.GetFloat("lat"),
					"lng": pos.GetFloat("lng"),
				},
			})
		}
	}

	return nil
}

// expirePenalties setzt abgelaufene Sperren inaktiv, damit die Oberfläche
// nicht selbst rechnen muss, was noch gilt.
func (e *Engine) expirePenalties(game *core.Record, now time.Time) error {
	stale, err := e.app.FindRecordsByFilter(
		schema.ColPenalties,
		"game = {:g} && active = true && expires_at <= {:now}",
		"", 0, 0,
		map[string]any{"g": game.Id, "now": DBTime(now)},
	)
	if err != nil {
		return err
	}

	for _, rec := range stale {
		rec.Set("active", false)
		if err := e.app.Save(rec); err != nil {
			return err
		}
	}

	return nil
}

// lastLat und lastLng liefern die zuletzt gemeldete Position eines Teams –
// eine Standortsperre gilt dort, wo das Team gerade steht.
func lastLat(app core.App, teamID string) float64 {
	if p := lastPosition(app, teamID); p != nil {
		return p.GetFloat("lat")
	}
	return 0
}

func lastLng(app core.App, teamID string) float64 {
	if p := lastPosition(app, teamID); p != nil {
		return p.GetFloat("lng")
	}
	return 0
}

func lastPosition(app core.App, teamID string) *core.Record {
	recs, err := app.FindRecordsByFilter(
		schema.ColPositions,
		"team = {:t}",
		"-captured_at", 1, 0,
		map[string]any{"t": teamID},
	)
	if err != nil || len(recs) == 0 {
		return nil
	}
	return recs[0]
}
