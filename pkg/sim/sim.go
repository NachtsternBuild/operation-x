// Package sim ist die Trockenübung.
//
// Sie steuert simulierte Spieler durch ein laufendes Spiel: Die Zielperson
// arbeitet Zwischenziele ab, die Fahndung bewegt sich auf Hinweise zu, Fristen
// laufen, Strafen greifen. Die Spielleitung kann dabei zusehen und lernt die
// Oberfläche kennen, ohne dass jemand vor die Tür muss.
//
// Der zweite Zweck ist mindestens so wichtig: Statt für jede Regeländerung
// sechs Stunden durch Dresden zu laufen, spielt der Simulator den Ablauf in
// wenigen Minuten durch – mitsamt Ping-Verzug, verfehlten Zielen und Zugriffen.
package sim

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/game"
	"github.com/elias/operation-x/pkg/geo"
	"github.com/elias/operation-x/pkg/schema"
)

// Takt der Simulation. Alle zwei Sekunden bewegen sich die Bots ein Stück.
const stepInterval = 2 * time.Second

// Gehgeschwindigkeit der Bots in Metern je Simulationsschritt.
//
// Bei zwei Sekunden Takt entspricht das rund 25 km/h – also ungefähr
// Zeitraffer zwanzigfach gegenüber einem Fußgänger. Schnell genug, dass eine
// Trockenübung in Minuten statt Stunden durchläuft, langsam genug, dass man
// auf der Karte sieht, was passiert.
const stepDistanceM = 90

// PuzzleSolver löst ein Rätsel für ein Team.
//
// Wie beim MissionMaker gilt: Der Simulator benutzt die echte Logik, statt eine
// zweite zu haben – nur die Antwort kennt er, weil ein Bot nicht raten kann.
type PuzzleSolver func(app core.App, puzzleID, teamID, answer string) error

// MissionMaker erzeugt das nächste Zwischenziel.
//
// Wird von außen hereingereicht, damit der Simulator dieselbe Routenlogik
// benutzt wie ein echter Client und nicht eine zweite, die auseinanderdriftet.
type MissionMaker func(app core.App, gameID, teamID string) error

// Runner steuert die Trockenübung.
type Runner struct {
	app     core.App
	log     *slog.Logger
	mission MissionMaker
	solve   PuzzleSolver

	// Je Spiel höchstens ein Lauf.
	//
	// Auf einem Server mit mehreren Spielen wäre ein einzelner Lauf ein
	// gemeinsames Möbelstück: Die zweite Gruppe bekäme "läuft bereits" zu
	// hören, obwohl bei ihr nichts läuft, und ihr Stopp-Knopf hielte die
	// Übung der ersten an.
	mu     sync.Mutex
	laeufe map[string]*lauf
}

// lauf ist eine laufende Trockenübung eines Spiels.
type lauf struct {
	cancel  context.CancelFunc
	bots    []*bot
	started time.Time
}

type bot struct {
	teamID   string
	callsign string
	role     string

	at     geo.Point
	target *geo.Point

	// Bei der Zielperson: die aktuelle Mission.
	missionID string
	// Wartezeit vor der nächsten Entscheidung, in Schritten.
	cooldown int
}

func New(app core.App) *Runner {
	return &Runner{app: app, log: slog.Default(), laeufe: map[string]*lauf{}}
}

// SetMissionMaker hinterlegt die Routenlogik.
func (r *Runner) SetMissionMaker(f MissionMaker) { r.mission = f }

// SetPuzzleSolver hinterlegt die Rätsellogik.
func (r *Runner) SetPuzzleSolver(f PuzzleSolver) { r.solve = f }

// Running meldet, ob die Trockenübung dieses Spiels läuft.
func (r *Runner) Running(gameID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.laeufe[gameID] != nil
}

// Status beschreibt den Lauf dieses Spiels für die Oberfläche.
func (r *Runner) Status(gameID string) map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()

	l := r.laeufe[gameID]
	if l == nil {
		return map[string]any{"running": false, "bots": []string{}}
	}

	names := make([]string, 0, len(l.bots))
	for _, b := range l.bots {
		names = append(names, b.callsign)
	}

	return map[string]any{
		"running":    true,
		"bots":       names,
		"runningSec": int(time.Since(l.started).Seconds()),
	}
}

// Start legt die Bot-Teams an und setzt sie in Bewegung.
//
// withMisterX steuert, ob auch die Zielperson simuliert wird. Übt die
// Spielleitung allein, ist das erwünscht; spielt jemand die Zielperson selbst,
// bleibt sie außen vor.
func (r *Runner) Start(gameID string, detectives int, withMisterX bool) error {
	r.mu.Lock()
	if r.laeufe[gameID] != nil {
		r.mu.Unlock()
		return fmt.Errorf("die Trockenübung läuft bereits")
	}
	r.mu.Unlock()

	gameRec, err := r.app.FindRecordById(schema.ColGames, gameID)
	if err != nil {
		return fmt.Errorf("Spiel nicht gefunden: %w", err)
	}

	hotspots, err := r.hotspots(gameID)
	if err != nil || len(hotspots) < 4 {
		return fmt.Errorf("für eine Trockenübung braucht es mindestens vier Hotspots")
	}

	bots, err := r.ensureBots(gameRec, detectives, withMisterX, hotspots)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())

	r.mu.Lock()
	r.laeufe[gameID] = &lauf{cancel: cancel, bots: bots, started: time.Now()}
	r.mu.Unlock()

	go r.loop(ctx, gameID)

	return nil
}

// Stop beendet die Trockenübung. Die Bot-Teams bleiben bestehen, damit sich
// das Protokoll danach in Ruhe durchsehen lässt.
func (r *Runner) Stop(gameID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if l := r.laeufe[gameID]; l != nil {
		l.cancel()
		delete(r.laeufe, gameID)
	}
}

// Cleanup entfernt die Bot-Teams samt ihrer Spuren.
func (r *Runner) Cleanup(gameID string) (int, error) {
	r.Stop(gameID)

	bots, err := r.app.FindRecordsByFilter(schema.ColTeams,
		"game = {:g} && bot = true", "", 0, 0,
		map[string]any{"g": gameID})
	if err != nil {
		return 0, err
	}

	for _, b := range bots {
		if err := r.app.Delete(b); err != nil {
			return 0, err
		}
	}

	return len(bots), nil
}

func (r *Runner) loop(ctx context.Context, gameID string) {
	ticker := time.NewTicker(stepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.step(gameID); err != nil {
				r.log.Error("Trockenübung", "fehler", err)
			}
		}
	}
}

// step bewegt jeden Bot ein Stück und lässt ihn handeln, wenn er angekommen ist.
func (r *Runner) step(gameID string) error {
	gameRec, err := r.app.FindRecordById(schema.ColGames, gameID)
	if err != nil {
		return err
	}
	if gameRec.GetString("status") != schema.GameRunning {
		return nil // Bei Pause und nach Spielende ruhen auch die Bots.
	}

	r.mu.Lock()
	l := r.laeufe[gameID]
	if l == nil {
		// Angehalten, während dieser Schritt schon unterwegs war. Das Schloss
		// muss trotzdem aufgehen, sonst steht der ganze Simulator.
		r.mu.Unlock()
		return nil
	}
	bots := append([]*bot(nil), l.bots...)
	r.mu.Unlock()

	for _, b := range bots {
		if b.cooldown > 0 {
			b.cooldown--
			continue
		}

		if b.target != nil {
			b.at = stepToward(b.at, *b.target, stepDistanceM)
			if geo.DistanceM(b.at, *b.target) < 25 {
				b.at = *b.target
				b.target = nil
			}
		}

		if err := r.report(gameRec, b); err != nil {
			r.log.Error("Bot-Meldung", "bot", b.callsign, "fehler", err)
		}

		if b.target == nil {
			r.decide(gameRec, b)
		}
	}

	return nil
}

// report schreibt die Position des Bots wie eine echte Standortmeldung.
func (r *Runner) report(gameRec *core.Record, b *bot) error {
	col, err := r.app.FindCollectionByNameOrId(schema.ColPositions)
	if err != nil {
		return err
	}

	now := time.Now()

	rec := core.NewRecord(col)
	rec.Set("game", gameRec.Id)
	rec.Set("team", b.teamID)
	rec.Set("lat", b.at.Lat)
	rec.Set("lng", b.at.Lng)
	rec.Set("accuracy", 8.0)
	rec.Set("captured_at", now)
	rec.Set("source", "sim")
	rec.Set("mocked", false)

	if err := r.app.Save(rec); err != nil {
		return err
	}

	// Frist zurücksetzen, wie es die echte Meldung auch tut.
	team, err := r.app.FindRecordById(schema.ColTeams, b.teamID)
	if err != nil {
		return err
	}
	cfg := game.ConfigOf(gameRec)
	team.Set("last_seen_at", now)
	team.Set("ping_due_at", now.Add(cfg.PingInterval(false)))

	return r.app.Save(team)
}

// decide entscheidet, was ein Bot als Nächstes tut.
func (r *Runner) decide(gameRec *core.Record, b *bot) {
	if b.role == schema.RoleMisterX {
		r.decideMisterX(gameRec, b)
		return
	}
	r.decideDetective(gameRec, b)
}

// decideMisterX arbeitet Zwischenziele ab: Mission holen, hingehen, einchecken.
func (r *Runner) decideMisterX(gameRec *core.Record, b *bot) {
	mission, err := r.app.FindFirstRecordByFilter(schema.ColMissions,
		"game = {:g} && (status = 'proposed' || status = 'active')",
		map[string]any{"g": gameRec.Id})

	// Keine offene Mission: eine neue anfordern. Der Simulator benutzt dafür
	// dieselbe Routenlogik wie ein echter Client, statt eine eigene zu haben.
	if err != nil || mission == nil {
		if r.mission != nil {
			if err := r.mission(r.app, gameRec.Id, b.teamID); err != nil {
				r.log.Warn("Trockenübung: keine Route", "fehler", err)
				b.cooldown = 5
				r.wander(gameRec, b)
			}
		} else {
			b.cooldown = 5
			r.wander(gameRec, b)
		}
		return
	}

	switch mission.GetString("status") {
	case "proposed":
		options, err := r.app.FindRecordsByFilter(schema.ColOptions,
			"mission = {:m}", "kind", 0, 0, map[string]any{"m": mission.Id})
		if err != nil || len(options) == 0 {
			b.cooldown = 3
			return
		}

		choice := options[rand.Intn(len(options))]
		choice.Set("chosen", true)
		_ = r.app.Save(choice)

		mission.Set("status", "active")
		mission.Set("target", choice.GetString("hotspot"))
		mission.Set("started_at", time.Now())
		mission.Set("deadline_at", time.Now().Add(
			time.Duration(choice.GetInt("time_limit_min"))*time.Minute))
		_ = r.app.Save(mission)

		b.missionID = mission.Id
		r.aimAtHotspot(b, choice.GetString("hotspot"))

	case "active":
		target, err := r.app.FindRecordById(schema.ColHotspots, mission.GetString("target"))
		if err != nil {
			b.cooldown = 3
			return
		}

		here := geo.Point{Lat: target.GetFloat("lat"), Lng: target.GetFloat("lng")}
		if geo.DistanceM(b.at, here) > 60 {
			b.target = &here
			return
		}

		// Angekommen: einchecken, als wäre der Vor-Ort-Code eingegeben worden.
		r.checkIn(gameRec, b, mission, target)
		b.cooldown = 2
	}
}

// checkIn schließt ein Zwischenziel ab.
func (r *Runner) checkIn(gameRec *core.Record, b *bot, mission, target *core.Record) {
	col, err := r.app.FindCollectionByNameOrId(schema.ColEvidence)
	if err != nil {
		return
	}

	ev := core.NewRecord(col)
	ev.Set("game", gameRec.Id)
	ev.Set("team", b.teamID)
	ev.Set("mission", mission.Id)
	ev.Set("hotspot", target.Id)
	ev.Set("lat", b.at.Lat)
	ev.Set("lng", b.at.Lng)
	ev.Set("distance_m", geo.DistanceM(b.at,
		geo.Point{Lat: target.GetFloat("lat"), Lng: target.GetFloat("lng")}))
	ev.Set("captured_at", time.Now())
	ev.Set("passcode_entered", target.GetString("passcode"))
	ev.Set("status", "accepted")
	_ = r.app.Save(ev)

	var points, fp int
	if chosen, err := r.app.FindFirstRecordByFilter(schema.ColOptions,
		"mission = {:m} && chosen = true", map[string]any{"m": mission.Id}); err == nil && chosen != nil {
		points = chosen.GetInt("reward_points")
		fp = chosen.GetInt("reward_fp")
	}

	mission.Set("status", "done")
	mission.Set("completed_at", time.Now())
	_ = r.app.Save(mission)

	_, _ = game.Book(r.app, game.Booking{
		Game: gameRec.Id, Team: b.teamID,
		Type:        game.EventMissionDone,
		Reason:      fmt.Sprintf("Zwischenziel %d erreicht (Trockenübung)", mission.GetInt("seq")),
		DeltaPoints: points,
		DeltaFP:     fp,
		DedupeKey:   "mission.done:" + mission.Id,
	})
}

// decideDetective lässt ein Fahndungsteam arbeiten: gelegentlich ein Rätsel
// lösen, sonst dem jüngsten Hinweis nachgehen.
func (r *Runner) decideDetective(gameRec *core.Record, b *bot) {
	// Etwa jeder vierte Zug ist Denkarbeit. Ohne gelöste Rätsel entstehen keine
	// Hinweise, und die Übung zeigte nur die halbe Schleife.
	if r.solve != nil && rand.Intn(4) == 0 {
		if r.solveOne(gameRec, b) {
			b.cooldown = 2
			return
		}
	}

	// Der jüngste Hinweis mit Ortsbezug gibt die Richtung vor.
	intel, err := r.app.FindRecordsByFilter(schema.ColIntel,
		"game = {:g} && delivered = true", "-occurred_at", 5, 0,
		map[string]any{"g": gameRec.Id})

	if err == nil {
		for _, i := range intel {
			if hid := i.GetString("hotspot"); hid != "" {
				r.aimAtHotspot(b, hid)
				return
			}
			if sid := i.GetString("sector"); sid != "" {
				if s, err := r.app.FindRecordById(schema.ColSectors, sid); err == nil {
					if area, err := geo.AreaFromGeoJSON(rawJSON(s.Get("geometry"))); err == nil {
						c := area.Outer.Centroid()
						b.target = &c
						return
					}
				}
			}
		}
	}

	r.wander(gameRec, b)
}

// solveOne löst ein noch offenes Rätsel. Liefert false, wenn keines übrig ist.
func (r *Runner) solveOne(gameRec *core.Record, b *bot) bool {
	puzzles, err := r.app.FindRecordsByFilter(schema.ColPuzzles,
		"game = {:g} && unlocked = true", "order", 0, 0,
		map[string]any{"g": gameRec.Id})
	if err != nil {
		return false
	}

	for _, p := range puzzles {
		if done, err := r.app.FindFirstRecordByFilter(schema.ColAttempts,
			"puzzle = {:p} && correct = true",
			map[string]any{"p": p.Id}); err == nil && done != nil {
			continue
		}

		// Die erste zulässige Schreibweise genügt; mehrere stehen mit
		// senkrechtem Strich getrennt im Feld.
		answer := p.GetString("answer")
		if i := strings.Index(answer, "|"); i > 0 {
			answer = answer[:i]
		}

		if err := r.solve(r.app, p.Id, b.teamID, answer); err != nil {
			r.log.Warn("Trockenübung: Rätsel", "fehler", err)
			return false
		}
		return true
	}

	return false
}

// wander schickt einen Bot zu einem zufälligen Hotspot – so bleibt Bewegung
// auf der Karte, auch wenn es gerade nichts zu verfolgen gibt.
func (r *Runner) wander(gameRec *core.Record, b *bot) {
	hotspots, err := r.hotspots(gameRec.Id)
	if err != nil || len(hotspots) == 0 {
		return
	}

	pick := hotspots[rand.Intn(len(hotspots))]
	p := geo.Point{Lat: pick.GetFloat("lat"), Lng: pick.GetFloat("lng")}
	b.target = &p
}

func (r *Runner) aimAtHotspot(b *bot, hotspotID string) {
	h, err := r.app.FindRecordById(schema.ColHotspots, hotspotID)
	if err != nil {
		return
	}
	p := geo.Point{Lat: h.GetFloat("lat"), Lng: h.GetFloat("lng")}
	b.target = &p
}

// ensureBots legt die simulierten Teams an oder greift auf bestehende zurück.
func (r *Runner) ensureBots(gameRec *core.Record, detectives int, withMisterX bool, hotspots []*core.Record) ([]*bot, error) {
	if detectives < 1 {
		detectives = 2
	}
	if detectives > 4 {
		detectives = 4
	}

	cfg := game.ConfigOf(gameRec)
	col, err := r.app.FindCollectionByNameOrId(schema.ColTeams)
	if err != nil {
		return nil, err
	}

	type wanted struct {
		callsign string
		display  string
		role     string
		color    string
		fp       int
	}

	list := []wanted{}
	bots := make([]*bot, 0, detectives+1)
	now := time.Now()

	// Die Zielperson wird nicht neu angelegt, sondern übernommen: Ein Spiel hat
	// genau eine, und ein zweiter Zugang mit derselben Rolle würde jede Abfrage
	// nach "der Zielperson" zufällig beantworten. Das übernommene Team wird
	// deshalb auch nicht als Bot markiert – sonst löschte das Aufräumen einen
	// echten Zugang.
	if withMisterX {
		existing, _ := r.app.FindFirstRecordByFilter(schema.ColTeams,
			"game = {:g} && role = {:r}",
			map[string]any{"g": gameRec.Id, "r": schema.RoleMisterX})

		if existing != nil {
			existing.Set("active", true)
			existing.Set("ping_due_at", now.Add(cfg.PingInterval(false)))
			if err := r.app.Save(existing); err != nil {
				return nil, err
			}

			start := hotspots[0]
			at := geo.Point{Lat: start.GetFloat("lat"), Lng: start.GetFloat("lng")}
			if pos := lastPositionOf(r.app, existing.Id); pos != nil {
				at = *pos
			}

			bots = append(bots, &bot{
				teamID:   existing.Id,
				callsign: existing.GetString("callsign"),
				role:     schema.RoleMisterX,
				at:       at,
			})
		}
	}

	for i := 0; i < detectives; i++ {
		name := fmt.Sprintf("Sim_%c", 'A'+i)
		list = append(list, wanted{name, "Übung: " + name, schema.RoleDetective, "#7fd4e0", cfg.StartFPDetective})
	}

	for i, w := range list {
		rec, err := r.app.FindFirstRecordByFilter(schema.ColTeams,
			"game = {:g} && callsign = {:c}",
			map[string]any{"g": gameRec.Id, "c": w.callsign})

		if err != nil || rec == nil {
			rec = core.NewRecord(col)
			rec.Set("callsign", w.callsign)
			rec.Set("display", w.display)
			rec.Set("role", w.role)
			rec.Set("game", gameRec.Id)
			rec.Set("color", w.color)
			// Auch im Übungsbetrieb über das Kontobuch, damit das Punktekonto
			// dort dasselbe zeigt wie im Ernstfall.
			rec.Set("fp", 0)
			rec.Set("points", 0)
			rec.Set("bot", true)
			// Ein Kennwort, mit dem sich niemand anmelden soll – die Bots
			// laufen im Server, nicht über die Anmeldung.
			rec.SetPassword(fmt.Sprintf("sim-%d-%d", now.UnixNano(), i))
		}

		rec.Set("active", true)
		rec.Set("bot", true)
		rec.Set("ping_due_at", now.Add(cfg.PingInterval(false)))

		if err := r.app.Save(rec); err != nil {
			return nil, fmt.Errorf("Bot %s anlegen: %w", w.callsign, err)
		}
		if err := game.BookStart(r.app, gameRec.Id, rec.Id, w.fp); err != nil {
			return nil, fmt.Errorf("Startguthaben für %s: %w", w.callsign, err)
		}

		// Verteilt starten, damit sich nicht alle am selben Punkt drängen.
		start := hotspots[(i*3)%len(hotspots)]

		bots = append(bots, &bot{
			teamID:   rec.Id,
			callsign: w.callsign,
			role:     w.role,
			at:       geo.Point{Lat: start.GetFloat("lat"), Lng: start.GetFloat("lng")},
		})
	}

	return bots, nil
}

// lastPositionOf liefert die zuletzt gemeldete Position eines Teams, damit ein
// übernommener Zugang dort weitermacht, wo er aufgehört hat.
func lastPositionOf(app core.App, teamID string) *geo.Point {
	recs, err := app.FindRecordsByFilter(schema.ColPositions,
		"team = {:t}", "-captured_at", 1, 0, map[string]any{"t": teamID})
	if err != nil || len(recs) == 0 {
		return nil
	}
	return &geo.Point{Lat: recs[0].GetFloat("lat"), Lng: recs[0].GetFloat("lng")}
}

func (r *Runner) hotspots(gameID string) ([]*core.Record, error) {
	return r.app.FindRecordsByFilter(schema.ColHotspots, "game = {:g}", "number", 0, 0,
		map[string]any{"g": gameID})
}

// stepToward bewegt einen Punkt um die angegebene Strecke auf ein Ziel zu.
func stepToward(from, to geo.Point, meters float64) geo.Point {
	d := geo.DistanceM(from, to)
	if d <= meters || d == 0 {
		return to
	}

	frac := meters / d
	return geo.Point{
		Lat: from.Lat + (to.Lat-from.Lat)*frac,
		Lng: from.Lng + (to.Lng-from.Lng)*frac,
	}
}

func rawJSON(v any) []byte {
	switch t := v.(type) {
	case []byte:
		return t
	case string:
		return []byte(t)
	default:
		return nil
	}
}
