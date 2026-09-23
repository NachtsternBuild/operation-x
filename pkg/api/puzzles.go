package api

import (
	"net/http"
	"sort"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/game"
	"github.com/elias/operation-x/pkg/geo"
	"github.com/elias/operation-x/pkg/schema"
)

type puzzleEntry struct {
	ID       string `json:"id"`
	Code     string `json:"code"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Question string `json:"question"`
	Hint     string `json:"hint,omitempty"`
	ImageURL string `json:"imageUrl,omitempty"`
	Points   int    `json:"points"`
	Category string `json:"intelCategory"`

	Solved     bool `json:"solved"`
	SolvedByMe bool `json:"solvedByMe"`
	Attempts   int  `json:"attempts"`
}

// handlePuzzles liefert den Detektiven die freigeschalteten Rätsel.
//
// Die Lösung geht nie an den Client – sonst stünde sie im Netzwerkverkehr und
// das Rätsel wäre in dem Moment wertlos, in dem jemand die Entwicklerkonsole
// öffnet.
func handlePuzzles(e *core.RequestEvent) error {
	me := e.Auth
	gameID := me.GetString("game")

	puzzles, err := e.App.FindRecordsByFilter(
		schema.ColPuzzles, "game = {:g} && unlocked = true", "order,code", 0, 0,
		map[string]any{"g": gameID},
	)
	if err != nil {
		return err
	}

	attempts, err := e.App.FindRecordsByFilter(
		schema.ColAttempts, "game = {:g}", "", 0, 0,
		map[string]any{"g": gameID},
	)
	if err != nil {
		return err
	}

	solvedByAnyone := map[string]bool{}
	solvedByMe := map[string]bool{}
	tries := map[string]int{}
	for _, a := range attempts {
		pid := a.GetString("puzzle")
		tries[pid]++
		if a.GetBool("correct") {
			solvedByAnyone[pid] = true
			if a.GetString("team") == me.Id {
				solvedByMe[pid] = true
			}
		}
	}

	out := make([]puzzleEntry, 0, len(puzzles))
	for _, p := range puzzles {
		entry := puzzleEntry{
			ID:         p.Id,
			Code:       p.GetString("code"),
			Type:       p.GetString("type"),
			Title:      p.GetString("title"),
			Question:   p.GetString("question"),
			Hint:       p.GetString("hint"),
			Points:     p.GetInt("points"),
			Category:   p.GetString("intel_category"),
			Solved:     solvedByAnyone[p.Id],
			SolvedByMe: solvedByMe[p.Id],
			Attempts:   tries[p.Id],
		}
		if file := p.GetString("image"); file != "" {
			entry.ImageURL = "/api/files/" + p.Collection().Id + "/" + p.Id + "/" + file
		}
		out = append(out, entry)
	}

	return e.JSON(http.StatusOK, map[string]any{"puzzles": out})
}

type solveRequest struct {
	Answer string `json:"answer"`
	UseFP  bool   `json:"useFp"`
}

// handleSolvePuzzle nimmt eine Lösung entgegen und schaltet bei Erfolg den
// Hinweis frei.
func handleSolvePuzzle(e *core.RequestEvent) error {
	me := e.Auth

	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	puzzle, err := ausSpiel(e, schema.ColPuzzles, e.Request.PathValue("id"), gameRec.Id)
	if err != nil {
		return err
	}
	if !puzzle.GetBool("unlocked") {
		return e.BadRequestError("Dieses Rätsel ist noch nicht freigeschaltet.", nil)
	}
	cfg := game.ConfigOf(gameRec)

	var req solveRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}

	// Bereits gelöst? Dann bringt ein zweiter Versuch nichts – der Hinweis ist
	// für alle Teams schon draußen.
	if prev, err := e.App.FindFirstRecordByFilter(schema.ColAttempts,
		"puzzle = {:p} && correct = true", map[string]any{"p": puzzle.Id}); err == nil && prev != nil {
		return e.BadRequestError("Dieses Rätsel ist bereits gelöst.", nil)
	}

	correct := game.AnswerMatches(req.Answer, puzzle.GetString("answer"))

	attemptsCol, err := e.App.FindCollectionByNameOrId(schema.ColAttempts)
	if err != nil {
		return err
	}

	attempt := core.NewRecord(attemptsCol)
	attempt.Set("game", gameRec.Id)
	attempt.Set("puzzle", puzzle.Id)
	attempt.Set("team", me.Id)
	attempt.Set("answer", req.Answer)
	attempt.Set("correct", correct)
	attempt.Set("used_fp", req.UseFP)

	points := 0
	if correct {
		points = cfg.PointsPuzzle
		if req.UseFP {
			points = cfg.PointsPuzzleWithFP
		}
		if custom := puzzle.GetInt("points"); custom > 0 {
			points = custom
			if req.UseFP {
				points = custom * cfg.PointsPuzzleWithFP / max(1, cfg.PointsPuzzle)
			}
		}
	}
	attempt.Set("awarded_points", points)

	if err := e.App.Save(attempt); err != nil {
		return e.InternalServerError("Versuch konnte nicht gespeichert werden.", err)
	}

	if !correct {
		return e.JSON(http.StatusOK, map[string]any{
			"correct": false,
			"message": "Das stimmt nicht. Noch einmal nachdenken.",
		})
	}

	if _, err := game.Book(e.App, game.Booking{
		Game: gameRec.Id, Team: me.Id,
		Type:        "puzzle.solved",
		Reason:      "Rätsel gelöst: " + puzzle.GetString("title"),
		DeltaPoints: points,
		DedupeKey:   "puzzle.solved:" + puzzle.Id,
	}); err != nil {
		return e.InternalServerError("Buchung fehlgeschlagen.", err)
	}

	intel, err := unlockIntel(e.App, gameRec, puzzle)
	if err != nil {
		// Der Punktgewinn steht bereits – ein fehlender Hinweis darf die
		// Antwort nicht scheitern lassen.
		return e.JSON(http.StatusOK, map[string]any{
			"correct": true,
			"points":  points,
			"message": "Richtig. Ein Hinweis ließ sich daraus gerade nicht ableiten.",
		})
	}

	return e.JSON(http.StatusOK, map[string]any{
		"correct": true,
		"points":  points,
		"intel":   intel,
		"message": "Richtig. Der Hinweis steht auf der Tafel.",
	})
}

// unlockIntel erzeugt den Hinweis, den ein gelöstes Rätsel freischaltet.
func unlockIntel(app core.App, gameRec, puzzle *core.Record) (map[string]any, error) {
	category := puzzle.GetString("intel_category")
	if category == "" {
		category = game.IntelYellow
	}

	trace, err := buildTrace(app, gameRec)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	intel, ok := game.GenerateIntel(category, trace, now)
	if !ok {
		return nil, errNoIntel
	}

	// Eine im Rätsel hinterlegte Vorlage hat Vorrang: So lassen sich
	// handgeschriebene Hinweise einstreuen.
	if template := puzzle.GetString("intel_template"); template != "" {
		intel.Text = template
	}

	col, err := app.FindCollectionByNameOrId(schema.ColIntel)
	if err != nil {
		return nil, err
	}

	rec := core.NewRecord(col)
	rec.Set("game", gameRec.Id)
	rec.Set("category", intel.Category)
	rec.Set("text", intel.Text)
	rec.Set("fabricated", false)
	rec.Set("source", "puzzle")
	rec.Set("occurred_at", intel.OccurredAt)
	rec.Set("deliver_at", now)
	rec.Set("delivered", true)
	if intel.HotspotID != "" {
		rec.Set("hotspot", intel.HotspotID)
	}
	if intel.SectorID != "" {
		rec.Set("sector", intel.SectorID)
	}

	if err := app.Save(rec); err != nil {
		return nil, err
	}

	return map[string]any{
		"category":   intel.Category,
		"text":       intel.Text,
		"occurredAt": intel.OccurredAt.UTC().Format(time.RFC3339),
	}, nil
}

type intelEntry struct {
	ID         string `json:"id"`
	Category   string `json:"category"`
	Text       string `json:"text"`
	OccurredAt string `json:"occurredAt"`
	AgeSec     int    `json:"ageSec"`
	Freshness  string `json:"freshness"`
	Source     string `json:"source"`

	// Nur die Zentrale erfährt, ob ein Hinweis gefälscht ist.
	Fabricated *bool `json:"fabricated,omitempty"`
}

// handleIntel liefert die Hinweistafel.
func handleIntel(e *core.RequestEvent) error {
	me := e.Auth
	gameID := me.GetString("game")

	gameRec, err := e.App.FindRecordById(schema.ColGames, gameID)
	if err != nil {
		return e.NotFoundError("Das Spiel wurde nicht gefunden.", nil)
	}
	cfg := game.ConfigOf(gameRec)
	now := time.Now()
	isHQ := me.GetString("role") == schema.RoleHQ

	// Zurückgehaltene Hinweise erscheinen erst, wenn ihre Zustellzeit erreicht
	// ist – so wirkt der Störungs-Joker aus Abschnitt 5.
	records, err := e.App.FindRecordsByFilter(
		schema.ColIntel,
		"game = {:g} && delivered = true && deliver_at <= {:now}",
		"-occurred_at", 100, 0,
		map[string]any{"g": gameID, "now": game.DBTime(now)},
	)
	if err != nil {
		return err
	}

	out := make([]intelEntry, 0, len(records))
	for _, r := range records {
		occurred := r.GetDateTime("occurred_at").Time()

		entry := intelEntry{
			ID:         r.Id,
			Category:   r.GetString("category"),
			Text:       r.GetString("text"),
			OccurredAt: occurred.UTC().Format(time.RFC3339),
			AgeSec:     int(now.Sub(occurred).Seconds()),
			Freshness:  game.Freshness(occurred, cfg, now),
			Source:     r.GetString("source"),
		}
		if isHQ {
			fab := r.GetBool("fabricated")
			entry.Fabricated = &fab
		}
		out = append(out, entry)
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].AgeSec < out[j].AgeSec })

	return e.JSON(http.StatusOK, map[string]any{"intel": out})
}

// buildTrace stellt die Bewegungsspur von Mister X zusammen.
func buildTrace(app core.App, gameRec *core.Record) (game.Trace, error) {
	var trace game.Trace

	misterX, err := app.FindFirstRecordByFilter(schema.ColTeams,
		"game = {:g} && role = {:r}",
		map[string]any{"g": gameRec.Id, "r": schema.RoleMisterX})
	if err != nil || misterX == nil {
		return trace, errNoIntel
	}

	positions, err := app.FindRecordsByFilter(schema.ColPositions,
		"team = {:t}", "-captured_at", 20, 0,
		map[string]any{"t": misterX.Id})
	if err != nil || len(positions) == 0 {
		return trace, errNoIntel
	}

	for _, p := range positions {
		trace.Points = append(trace.Points, game.TracePoint{
			At:    p.GetDateTime("captured_at").Time(),
			Point: geo.Point{Lat: p.GetFloat("lat"), Lng: p.GetFloat("lng")},
			Speed: p.GetFloat("speed"),
		})
	}

	// Was schon auf der Tafel steht.
	//
	// Die letzten Hinweise gehen in die Erzeugung ein, damit der nächste nicht
	// derselbe Satz wird: Bleibt Mister X im selben Sektor, lieferte jedes
	// gelöste Rätsel bisher wortgleich dieselbe Auskunft. Zwölf genügen – wer
	// so weit zurückliegt, ist ohnehin kalt, und dieselbe Aussage nach einer
	// Stunde noch einmal ist eine neue Aussage.
	if seen, err := app.FindRecordsByFilter(schema.ColIntel,
		"game = {:g}", "-occurred_at", 12, 0,
		map[string]any{"g": gameRec.Id}); err == nil {
		for _, i := range seen {
			trace.Recent = append(trace.Recent, i.GetString("text"))
		}
	}

	// Zuletzt bestätigter Hotspot: Das ist die einzige wirklich gesicherte
	// Tatsache über den Aufenthalt von Mister X.
	if done, err := app.FindRecordsByFilter(schema.ColMissions,
		"game = {:g} && status = 'done'", "-completed_at", 1, 0,
		map[string]any{"g": gameRec.Id}); err == nil && len(done) > 0 {
		if h, err := app.FindRecordById(schema.ColHotspots, done[0].GetString("target")); err == nil {
			trace.LastHotspot = h.Id
			trace.LastHotspotNo = h.GetInt("number")
			trace.LastHotspotTime = done[0].GetDateTime("completed_at").Time()
		}
	}

	// Aktueller Sektor und dessen Nachbarn.
	current := trace.Points[0].Point
	sectors, err := app.FindRecordsByFilter(schema.ColSectors, "game = {:g}", "code", 0, 0,
		map[string]any{"g": gameRec.Id})
	if err == nil {
		type sectorInfo struct {
			code, name string
			area       geo.Area
		}
		var all []sectorInfo

		for _, s := range sectors {
			area, err := geo.AreaFromGeoJSON(toRaw(s.Get("geometry")))
			if err != nil {
				continue
			}
			info := sectorInfo{code: s.GetString("code"), name: s.GetString("name"), area: area}
			all = append(all, info)

			if area.Contains(current) {
				trace.SectorCode = info.code
				trace.SectorName = info.name
			}
		}

		// Als Nachbarn gelten Sektoren, deren Mittelpunkt nah genug liegt, um
		// im selben Atemzug genannt zu werden.
		if trace.SectorCode != "" {
			for _, s := range all {
				if s.code == trace.SectorCode {
					continue
				}
				if geo.DistanceM(current, s.area.Outer.Centroid()) < 2500 {
					trace.NeighborCodes = append(trace.NeighborCodes, s.code)
				}
			}
		}
	}

	return trace, nil
}

// errNoIntel bedeutet: Aus der aktuellen Lage lässt sich kein Hinweis ableiten.
var errNoIntel = errNoIntelType{}

type errNoIntelType struct{}

func (errNoIntelType) Error() string {
	return "aus der aktuellen Lage lässt sich kein Hinweis ableiten"
}

// --- Rätselpult der Einsatzzentrale -------------------------------------------

type hqPuzzleEntry struct {
	puzzleEntry
	// Die Zentrale sieht als Einzige die Lösung – sie muss beurteilen können,
	// ob ein Rätsel zu schwer ist, und im Zweifel nachhelfen.
	Answer   string `json:"answer"`
	Unlocked bool   `json:"unlocked"`
}

// handleHQPuzzles zeigt der Zentrale alle Rätsel, auch die gesperrten.
func handleHQPuzzles(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	puzzles, err := e.App.FindRecordsByFilter(
		schema.ColPuzzles, "game = {:g}", "order,code", 0, 0,
		map[string]any{"g": gameRec.Id},
	)
	if err != nil {
		return err
	}

	attempts, err := e.App.FindRecordsByFilter(
		schema.ColAttempts, "game = {:g}", "-created", 0, 0,
		map[string]any{"g": gameRec.Id},
	)
	if err != nil {
		return err
	}

	solved := map[string]bool{}
	tries := map[string]int{}
	for _, a := range attempts {
		pid := a.GetString("puzzle")
		tries[pid]++
		if a.GetBool("correct") {
			solved[pid] = true
		}
	}

	out := make([]hqPuzzleEntry, 0, len(puzzles))
	for _, p := range puzzles {
		out = append(out, hqPuzzleEntry{
			puzzleEntry: puzzleEntry{
				ID:       p.Id,
				Code:     p.GetString("code"),
				Type:     p.GetString("type"),
				Title:    p.GetString("title"),
				Question: p.GetString("question"),
				Hint:     p.GetString("hint"),
				Points:   p.GetInt("points"),
				Category: p.GetString("intel_category"),
				Solved:   solved[p.Id],
				Attempts: tries[p.Id],
			},
			Answer:   p.GetString("answer"),
			Unlocked: p.GetBool("unlocked"),
		})
	}

	// Zusätzlich die letzten Lösungsversuche, damit die Zentrale mitbekommt,
	// wo eine Gruppe feststeckt – und notfalls einen Hinweis nachschieben kann.
	type tryEntry struct {
		Puzzle   string `json:"puzzle"`
		Callsign string `json:"callsign"`
		Answer   string `json:"answer"`
		Correct  bool   `json:"correct"`
		At       string `json:"at"`
	}
	recent := make([]tryEntry, 0, 20)
	for i, a := range attempts {
		if i >= 20 {
			break
		}
		entry := tryEntry{
			Puzzle:  a.GetString("puzzle"),
			Answer:  a.GetString("answer"),
			Correct: a.GetBool("correct"),
			At:      a.GetDateTime("created").Time().UTC().Format(time.RFC3339),
		}
		if t, err := e.App.FindRecordById(schema.ColTeams, a.GetString("team")); err == nil {
			entry.Callsign = t.GetString("callsign")
		}
		recent = append(recent, entry)
	}

	return e.JSON(http.StatusOK, map[string]any{"puzzles": out, "attempts": recent})
}

type unlockRequest struct {
	Unlocked bool `json:"unlocked"`
}

// handleUnlockPuzzle schaltet ein Rätsel frei oder sperrt es wieder.
func handleUnlockPuzzle(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	puzzle, err := ausSpiel(e, schema.ColPuzzles, e.Request.PathValue("id"), gameRec.Id)
	if err != nil {
		return err
	}

	var req unlockRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}

	puzzle.Set("unlocked", req.Unlocked)
	if req.Unlocked && puzzle.GetDateTime("unlock_at").Time().IsZero() {
		puzzle.Set("unlock_at", time.Now())
	}

	if err := e.App.Save(puzzle); err != nil {
		return e.InternalServerError("Freischaltung fehlgeschlagen.", err)
	}

	return e.JSON(http.StatusOK, map[string]any{"unlocked": req.Unlocked})
}

// SolvePuzzleFor löst ein Rätsel im Namen eines Teams.
//
// Öffentlich für die Trockenübung, damit sie dieselbe Auswertung und
// Hinweiserzeugung durchläuft wie ein echter Lösungsversuch.
func SolvePuzzleFor(app core.App, puzzleID, teamID, answer string) error {
	puzzle, err := app.FindRecordById(schema.ColPuzzles, puzzleID)
	if err != nil {
		return err
	}
	gameRec, err := app.FindRecordById(schema.ColGames, puzzle.GetString("game"))
	if err != nil {
		return err
	}
	if !game.AnswerMatches(answer, puzzle.GetString("answer")) {
		return errNoIntel
	}

	cfg := game.ConfigOf(gameRec)
	points := puzzle.GetInt("points")
	if points <= 0 {
		points = cfg.PointsPuzzle
	}

	col, err := app.FindCollectionByNameOrId(schema.ColAttempts)
	if err != nil {
		return err
	}

	attempt := core.NewRecord(col)
	attempt.Set("game", gameRec.Id)
	attempt.Set("puzzle", puzzle.Id)
	attempt.Set("team", teamID)
	attempt.Set("answer", answer)
	attempt.Set("correct", true)
	attempt.Set("awarded_points", points)
	if err := app.Save(attempt); err != nil {
		return err
	}

	if _, err := game.Book(app, game.Booking{
		Game: gameRec.Id, Team: teamID,
		Type:        "puzzle.solved",
		Reason:      "Rätsel gelöst: " + puzzle.GetString("title"),
		DeltaPoints: points,
		DedupeKey:   "puzzle.solved:" + puzzle.Id,
	}); err != nil {
		return err
	}

	_, err = unlockIntel(app, gameRec, puzzle)
	return err
}
