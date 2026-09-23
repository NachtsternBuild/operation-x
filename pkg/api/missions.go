package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/game"
	"github.com/elias/operation-x/pkg/geo"
	"github.com/elias/operation-x/pkg/schema"
)

type missionOption struct {
	ID           string  `json:"id"`
	Kind         string  `json:"kind"`
	Label        string  `json:"label"`
	Description  string  `json:"description"`
	HotspotID    string  `json:"hotspotId"`
	Number       int     `json:"number"`
	Name         string  `json:"name"`
	Lat          float64 `json:"lat"`
	Lng          float64 `json:"lng"`
	DistanceM    float64 `json:"distanceM"`
	TimeLimitMin int     `json:"timeLimitMin"`
	RewardPoints int     `json:"rewardPoints"`
	RewardFP     int     `json:"rewardFp"`

	// Was am Ziel zu tun ist, sofern die Spielleitung etwas hinterlegt hat.
	// Ohne diese Zeile stünde die Aufgabe in der Datenbank und nirgends sonst.
	Task string `json:"task,omitempty"`
}

type missionResponse struct {
	ID       string `json:"id"`
	Seq      int    `json:"seq"`
	Status   string `json:"status"`
	IsFinal  bool   `json:"isFinal"`
	Planned  int    `json:"planned"`
	Finished int    `json:"finished"`

	Options []missionOption `json:"options,omitempty"`

	// Bei laufender Mission: das gewählte Ziel.
	Target     *missionOption `json:"target,omitempty"`
	DeadlineAt string         `json:"deadlineAt,omitempty"`
	LeftSec    int            `json:"leftSec,omitempty"`

	// Abstand zum Ziel und ob nah genug für den Nachweis.
	DistanceM float64 `json:"distanceM"`
	InRange   bool    `json:"inRange"`
	RangeM    float64 `json:"rangeM"`

	Evidence *evidenceState `json:"evidence,omitempty"`

	// Kulanzzeit: wie viel schon gewährt wurde und wie viel bliebe. Die
	// Oberfläche zeigt daraus, ob "Ich brauche länger" noch etwas bringt.
	Grace *game.GraceState `json:"grace,omitempty"`
}

type evidenceState struct {
	Status string `json:"status"`
	Note   string `json:"note,omitempty"`
}

// handleMission liefert Mister X sein Missionsbuch.
//
// Gibt es keine offene Mission, wird eine erzeugt: Der Server bewertet die
// Hotspots im Umkreis und stellt drei Varianten zur Wahl.
func handleMission(e *core.RequestEvent) error {
	me := e.Auth
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	mission, err := openMission(e.App, gameRec.Id)
	if err != nil {
		return err
	}

	if mission == nil {
		mission, err = createMission(e.App, gameRec, me)
		if errors.Is(err, errNoPosition) {
			return e.BadRequestError(
				"Es liegt noch keine Standortmeldung vor. Erst den Standort "+
					"melden, dann gibt es Zwischenziele.", nil)
		}
		if err != nil {
			return e.InternalServerError(
				"Das nächste Zwischenziel ließ sich nicht bestimmen.", err)
		}
		if mission == nil {
			return e.JSON(http.StatusOK, map[string]any{
				"status":  "none",
				"message": "Es sind keine erreichbaren Ziele mehr übrig.",
			})
		}
	}

	return e.JSON(http.StatusOK, buildMissionResponse(e.App, gameRec, me, mission))
}

// openMission liefert die vorgeschlagene oder laufende Mission eines Spiels.
func openMission(app core.App, gameID string) (*core.Record, error) {
	recs, err := app.FindRecordsByFilter(
		schema.ColMissions,
		"game = {:g} && (status = 'proposed' || status = 'active')",
		"-seq", 1, 0,
		map[string]any{"g": gameID},
	)
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, nil
	}
	return recs[0], nil
}

// errNoPosition sagt, dass noch keine Standortmeldung vorliegt.
//
// Als eigener Fehlerwert, weil das keine Störung ist, sondern eine Bedingung,
// die der Spieler selbst herstellen kann. Ohne diese Unterscheidung landete er
// als "500 Interner Serverfehler" beim Client – eine Antwort, die sagt "hier
// ist etwas kaputt", obwohl nur der erste Ping fehlt.
var errNoPosition = errors.New("es liegt noch keine Standortmeldung vor – zuerst den Standort melden")

// createMission erzeugt das nächste Zwischenziel mit drei Varianten.
func createMission(app core.App, gameRec *core.Record, me *core.Record) (*core.Record, error) {
	cfg := game.ConfigOf(gameRec)

	pos := latestPosition(app, me.Id)
	if pos == nil {
		return nil, errNoPosition
	}
	from := geo.Point{Lat: pos.GetFloat("lat"), Lng: pos.GetFloat("lng")}

	hotspots, err := app.FindRecordsByFilter(schema.ColHotspots, "game = {:g}", "number", 0, 0,
		map[string]any{"g": gameRec.Id})
	if err != nil {
		return nil, err
	}

	candidates := make([]game.Candidate, 0, len(hotspots))
	byID := map[string]*core.Record{}
	for _, h := range hotspots {
		candidates = append(candidates, game.Candidate{
			HotspotID: h.Id,
			Number:    h.GetInt("number"),
			Name:      h.GetString("name"),
			Point:     geo.Point{Lat: h.GetFloat("lat"), Lng: h.GetFloat("lng")},
			SectorID:  h.GetString("sector"),
		})
		byID[h.Id] = h
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("für dieses Spiel sind keine Hotspots angelegt")
	}

	// Fluchtziel einmalig festlegen und danach geheim halten.
	finalID := gameRec.GetString("final_target")
	if finalID == "" {
		if target, ok := game.PickFinalTarget(from, candidates); ok {
			finalID = target.HotspotID
			gameRec.Set("final_target", finalID)
			if gameRec.GetInt("planned_missions") == 0 {
				gameRec.Set("planned_missions", plannedMissions(gameRec))
			}
			if err := app.Save(gameRec); err != nil {
				return nil, err
			}
		}
	}

	var finalPoint *geo.Point
	if h, ok := byID[finalID]; ok {
		finalPoint = &geo.Point{Lat: h.GetFloat("lat"), Lng: h.GetFloat("lng")}
	}

	visited, err := visitedHotspots(app, gameRec.Id)
	if err != nil {
		return nil, err
	}
	// Das Fluchtziel selbst ist kein Zwischenziel.
	if finalID != "" {
		visited[finalID] = true
	}

	input := game.RouteInput{
		From:        from,
		Candidates:  candidates,
		Detectives:  detectivePositions(app, gameRec.Id),
		Visited:     visited,
		FinalTarget: finalPoint,
		Progress:    progressOf(gameRec),
	}

	options := game.Propose(input, cfg)
	if len(options) == 0 {
		return nil, nil
	}

	missionsCol, err := app.FindCollectionByNameOrId(schema.ColMissions)
	if err != nil {
		return nil, err
	}
	optionsCol, err := app.FindCollectionByNameOrId(schema.ColOptions)
	if err != nil {
		return nil, err
	}

	var mission *core.Record

	err = app.RunInTransaction(func(tx core.App) error {
		done, err := tx.FindRecordsByFilter(schema.ColMissions, "game = {:g} && status = 'done'", "", 0, 0,
			map[string]any{"g": gameRec.Id})
		if err != nil {
			return err
		}

		mission = core.NewRecord(missionsCol)
		mission.Set("game", gameRec.Id)
		mission.Set("seq", len(done)+1)
		mission.Set("status", "proposed")
		mission.Set("is_final", false)
		if err := tx.Save(mission); err != nil {
			return err
		}

		for _, o := range options {
			rec := core.NewRecord(optionsCol)
			rec.Set("mission", mission.Id)
			rec.Set("hotspot", o.Candidate.HotspotID)
			rec.Set("kind", o.Kind)
			rec.Set("label", o.Label)
			rec.Set("description", o.Description)
			rec.Set("time_limit_min", o.TimeLimitMin)
			rec.Set("reward_points", o.RewardPoints)
			rec.Set("reward_fp", o.RewardFP)
			rec.Set("chosen", false)
			if err := tx.Save(rec); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return mission, nil
}

type chooseRequest struct {
	OptionID string `json:"optionId"`
}

// handleChooseOption übernimmt die Wahl von Mister X und startet die Frist.
func handleChooseOption(e *core.RequestEvent) error {
	me := e.Auth
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	var req chooseRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}

	mission, err := openMission(e.App, gameRec.Id)
	if err != nil || mission == nil {
		return e.BadRequestError("Es liegt keine Mission zur Wahl vor.", nil)
	}
	if mission.GetString("status") != "proposed" {
		return e.BadRequestError("Diese Mission läuft bereits.", nil)
	}

	option, err := e.App.FindRecordById(schema.ColOptions, req.OptionID)
	if err != nil || option.GetString("mission") != mission.Id {
		return e.BadRequestError("Diese Variante gehört nicht zur aktuellen Mission.", nil)
	}

	now := time.Now()
	deadline := now.Add(time.Duration(option.GetInt("time_limit_min")) * time.Minute)

	err = e.App.RunInTransaction(func(tx core.App) error {
		option.Set("chosen", true)
		if err := tx.Save(option); err != nil {
			return err
		}

		mission.Set("status", "active")
		mission.Set("target", option.GetString("hotspot"))
		mission.Set("started_at", now)
		mission.Set("deadline_at", deadline)
		return tx.Save(mission)
	})
	if err != nil {
		return e.InternalServerError("Wahl konnte nicht gespeichert werden.", err)
	}

	_, _ = game.Book(e.App, game.Booking{
		Game: gameRec.Id, Team: me.Id,
		Type:   game.EventMissionStart,
		Reason: "Zwischenziel gewählt: " + option.GetString("label"),
		Payload: map[string]any{
			"variante": option.GetString("kind"),
			"frist":    option.GetInt("time_limit_min"),
		},
	})

	fresh, _ := e.App.FindRecordById(schema.ColMissions, mission.Id)
	return e.JSON(http.StatusOK, buildMissionResponse(e.App, gameRec, me, fresh))
}

type evidenceRequest struct {
	Passcode string `json:"passcode"`
}

// handleEvidence nimmt den Nachweis entgegen, dass Mister X am Ziel war.
//
// Zwei Wege: der Vor-Ort-Code, der sofort entscheidet, oder ein Foto, das die
// Einsatzzentrale prüft. In beiden Fällen wird zuerst der Abstand geprüft –
// vom Sofa aus lässt sich kein Zwischenziel abhaken.
func handleEvidence(e *core.RequestEvent) error {
	me := e.Auth
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}
	cfg := game.ConfigOf(gameRec)

	mission, err := openMission(e.App, gameRec.Id)
	if err != nil || mission == nil || mission.GetString("status") != "active" {
		return e.BadRequestError("Es läuft gerade keine Mission.", nil)
	}

	target, err := e.App.FindRecordById(schema.ColHotspots, mission.GetString("target"))
	if err != nil {
		return e.InternalServerError("Das Ziel der Mission fehlt.", err)
	}

	pos := latestPosition(e.App, me.Id)
	if pos == nil {
		return e.BadRequestError("Es liegt keine Standortmeldung vor.", nil)
	}

	here := geo.Point{Lat: pos.GetFloat("lat"), Lng: pos.GetFloat("lng")}
	there := geo.Point{Lat: target.GetFloat("lat"), Lng: target.GetFloat("lng")}
	distance := geo.DistanceM(here, there)

	// Die gemeldete Messgenauigkeit wird angerechnet: Zwischen Häuserzeilen
	// liefert ein Telefon schon mal fünfzig Meter Unsicherheit, und dafür soll
	// niemand abgewiesen werden, der tatsächlich davorsteht.
	allowed := cfg.HotspotMaxDistanceM + pos.GetFloat("accuracy")
	if distance > allowed {
		// Die Zahlen stehen im Klartext in der Meldung. Als strukturierte
		// Zusatzdaten würde PocketBase sie als Feldvalidierungsfehler ausgeben,
		// und der Client zeigte sie als Formularfehler an – den aktuellen
		// Abstand liefert ohnehin /api/opx/mission.
		return e.BadRequestError(
			fmt.Sprintf("Zu weit entfernt: %.0f m zu %s. Erlaubt sind %.0f m.",
				distance, target.GetString("name"), allowed),
			nil,
		)
	}

	var req evidenceRequest
	_ = e.BindBody(&req) // bei einem Foto-Upload gibt es keinen JSON-Rumpf

	files, _ := e.FindUploadedFiles("photo")
	code := strings.ToUpper(strings.TrimSpace(req.Passcode))
	if code == "" {
		code = strings.ToUpper(strings.TrimSpace(e.Request.FormValue("passcode")))
	}

	if code == "" && len(files) == 0 {
		return e.BadRequestError("Bitte den Vor-Ort-Code eingeben oder ein Foto hochladen.", nil)
	}

	col, err := e.App.FindCollectionByNameOrId(schema.ColEvidence)
	if err != nil {
		return err
	}

	rec := core.NewRecord(col)
	rec.Set("game", gameRec.Id)
	rec.Set("team", me.Id)
	rec.Set("mission", mission.Id)
	rec.Set("hotspot", target.Id)
	rec.Set("lat", here.Lat)
	rec.Set("lng", here.Lng)
	rec.Set("distance_m", distance)
	rec.Set("captured_at", time.Now())
	rec.Set("passcode_entered", code)
	if len(files) > 0 {
		rec.Set("photo", files[0])
	}

	// Der Code entscheidet sofort. Ein Foto muss die Zentrale ansehen.
	expected := strings.ToUpper(strings.TrimSpace(target.GetString("passcode")))
	codeCorrect := code != "" && expected != "" && code == expected

	switch {
	case codeCorrect:
		rec.Set("status", "accepted")
	case code != "" && len(files) == 0:
		return e.BadRequestError("Der Code stimmt nicht. Steht er wirklich an diesem Punkt?", nil)
	default:
		rec.Set("status", "pending")
	}

	if err := e.App.Save(rec); err != nil {
		return e.InternalServerError("Nachweis konnte nicht gespeichert werden.", err)
	}

	if codeCorrect {
		if err := completeMission(e.App, gameRec, me, mission); err != nil {
			return e.InternalServerError("Mission konnte nicht abgeschlossen werden.", err)
		}
		fresh, _ := e.App.FindRecordById(schema.ColMissions, mission.Id)
		return e.JSON(http.StatusOK, map[string]any{
			"accepted": true,
			"message":  "Zwischenziel bestätigt.",
			"mission":  buildMissionResponse(e.App, gameRec, me, fresh),
		})
	}

	return e.JSON(http.StatusOK, map[string]any{
		"accepted": false,
		"pending":  true,
		"message":  "Foto eingereicht. Die Einsatzzentrale prüft es.",
	})
}

// completeMission schreibt die Belohnung gut und schließt die Mission ab.
func completeMission(app core.App, gameRec *core.Record, me *core.Record, mission *core.Record) error {
	var points, fp int

	chosen, err := app.FindFirstRecordByFilter(schema.ColOptions,
		"mission = {:m} && chosen = true", map[string]any{"m": mission.Id})
	if err == nil && chosen != nil {
		points = chosen.GetInt("reward_points")
		fp = chosen.GetInt("reward_fp")
	} else {
		points = game.ConfigOf(gameRec).PointsMission
	}

	mission.Set("status", "done")
	mission.Set("completed_at", time.Now())
	if err := app.Save(mission); err != nil {
		return err
	}

	_, err = game.Book(app, game.Booking{
		Game: gameRec.Id, Team: me.Id,
		Type:        game.EventMissionDone,
		Reason:      fmt.Sprintf("Zwischenziel %d erreicht", mission.GetInt("seq")),
		DeltaPoints: points,
		DeltaFP:     fp,
		DedupeKey:   "mission.done:" + mission.Id,
	})
	return err
}

// handleAbortMission bricht das gewählte Ziel ab und fordert eine neue Route an.
func handleAbortMission(e *core.RequestEvent) error {
	me := e.Auth
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}
	cfg := game.ConfigOf(gameRec)

	mission, err := openMission(e.App, gameRec.Id)
	if err != nil || mission == nil || mission.GetString("status") != "active" {
		return e.BadRequestError("Es läuft gerade keine Mission.", nil)
	}

	cost := cfg.CostReroute
	if me.GetInt("fp") < cost {
		return e.BadRequestError(
			fmt.Sprintf("Ausklinken kostet %d Fluchtpunkte, vorhanden sind %d.", cost, me.GetInt("fp")), nil)
	}

	mission.Set("status", "aborted")
	if err := e.App.Save(mission); err != nil {
		return e.InternalServerError("Abbruch fehlgeschlagen.", err)
	}

	if _, err := game.Book(e.App, game.Booking{
		Game: gameRec.Id, Team: me.Id,
		Type:    game.EventMissionAbort,
		Reason:  "Zwischenziel abgebrochen, neue Route angefordert",
		DeltaFP: -cost,
	}); err != nil {
		return e.InternalServerError("Buchung fehlgeschlagen.", err)
	}

	fresh, _ := e.App.FindRecordById(schema.ColTeams, me.Id)
	next, err := createMission(e.App, gameRec, fresh)
	if err != nil || next == nil {
		return e.JSON(http.StatusOK, map[string]any{"aborted": true})
	}

	return e.JSON(http.StatusOK, map[string]any{
		"aborted": true,
		"mission": buildMissionResponse(e.App, gameRec, fresh, next),
	})
}

// --- Hilfsfunktionen ----------------------------------------------------------

func buildMissionResponse(app core.App, gameRec, me, mission *core.Record) missionResponse {
	res := missionResponse{
		ID:      mission.Id,
		Seq:     mission.GetInt("seq"),
		Status:  mission.GetString("status"),
		IsFinal: mission.GetBool("is_final"),
		Planned: gameRec.GetInt("planned_missions"),
		RangeM:  game.ConfigOf(gameRec).HotspotMaxDistanceM,
	}

	if done, err := app.FindRecordsByFilter(schema.ColMissions,
		"game = {:g} && status = 'done'", "", 0, 0,
		map[string]any{"g": gameRec.Id}); err == nil {
		res.Finished = len(done)
	}

	if mission.GetString("status") == "active" {
		state := game.GraceOf(mission, game.ConfigOf(gameRec))
		res.Grace = &state
	}

	pos := latestPosition(app, me.Id)
	var here *geo.Point
	if pos != nil {
		here = &geo.Point{Lat: pos.GetFloat("lat"), Lng: pos.GetFloat("lng")}
	}

	if res.Status == "proposed" {
		opts, err := app.FindRecordsByFilter(schema.ColOptions, "mission = {:m}", "kind", 0, 0,
			map[string]any{"m": mission.Id})
		if err == nil {
			for _, o := range opts {
				if entry, ok := optionEntry(app, o, here); ok {
					res.Options = append(res.Options, entry)
				}
			}
		}
		return res
	}

	if targetID := mission.GetString("target"); targetID != "" {
		if h, err := app.FindRecordById(schema.ColHotspots, targetID); err == nil {
			entry := missionOption{
				HotspotID: h.Id,
				Number:    h.GetInt("number"),
				Name:      h.GetString("name"),
				Lat:       h.GetFloat("lat"),
				Lng:       h.GetFloat("lng"),
				Task:      h.GetString("notes"),
			}
			if chosen, err := app.FindFirstRecordByFilter(schema.ColOptions,
				"mission = {:m} && chosen = true", map[string]any{"m": mission.Id}); err == nil && chosen != nil {
				entry.ID = chosen.Id
				entry.Kind = chosen.GetString("kind")
				entry.Label = chosen.GetString("label")
				entry.TimeLimitMin = chosen.GetInt("time_limit_min")
				entry.RewardPoints = chosen.GetInt("reward_points")
				entry.RewardFP = chosen.GetInt("reward_fp")
			}
			if here != nil {
				entry.DistanceM = geo.DistanceM(*here, geo.Point{Lat: entry.Lat, Lng: entry.Lng})
				res.DistanceM = entry.DistanceM

				allowed := res.RangeM
				if pos != nil {
					allowed += pos.GetFloat("accuracy")
				}
				res.InRange = entry.DistanceM <= allowed
			}
			res.Target = &entry
		}
	}

	if deadline := mission.GetDateTime("deadline_at").Time(); !deadline.IsZero() {
		res.DeadlineAt = deadline.UTC().Format(time.RFC3339)
		res.LeftSec = int(time.Until(deadline).Seconds())
	}

	if ev, err := app.FindFirstRecordByFilter(schema.ColEvidence,
		"mission = {:m}", map[string]any{"m": mission.Id}); err == nil && ev != nil {
		res.Evidence = &evidenceState{
			Status: ev.GetString("status"),
			Note:   ev.GetString("note"),
		}
	}

	return res
}

func optionEntry(app core.App, o *core.Record, here *geo.Point) (missionOption, bool) {
	h, err := app.FindRecordById(schema.ColHotspots, o.GetString("hotspot"))
	if err != nil {
		return missionOption{}, false
	}

	entry := missionOption{
		ID:           o.Id,
		Kind:         o.GetString("kind"),
		Label:        o.GetString("label"),
		Description:  o.GetString("description"),
		HotspotID:    h.Id,
		Number:       h.GetInt("number"),
		Name:         h.GetString("name"),
		Lat:          h.GetFloat("lat"),
		Lng:          h.GetFloat("lng"),
		TimeLimitMin: o.GetInt("time_limit_min"),
		RewardPoints: o.GetInt("reward_points"),
		RewardFP:     o.GetInt("reward_fp"),
		Task:         h.GetString("notes"),
	}
	if here != nil {
		entry.DistanceM = geo.DistanceM(*here, geo.Point{Lat: entry.Lat, Lng: entry.Lng})
	}
	return entry, true
}

func visitedHotspots(app core.App, gameID string) (map[string]bool, error) {
	visited := map[string]bool{}

	missions, err := app.FindRecordsByFilter(schema.ColMissions, "game = {:g}", "", 0, 0,
		map[string]any{"g": gameID})
	if err != nil {
		return nil, err
	}
	for _, m := range missions {
		if t := m.GetString("target"); t != "" {
			visited[t] = true
		}
	}
	return visited, nil
}

func detectivePositions(app core.App, gameID string) []geo.Point {
	teams, err := app.FindRecordsByFilter(schema.ColTeams,
		"game = {:g} && role = 'detective' && active = true", "", 0, 0,
		map[string]any{"g": gameID})
	if err != nil {
		return nil
	}

	points := make([]geo.Point, 0, len(teams))
	for _, t := range teams {
		if p := latestPosition(app, t.Id); p != nil {
			points = append(points, geo.Point{Lat: p.GetFloat("lat"), Lng: p.GetFloat("lng")})
		}
	}
	return points
}

// progressOf liefert den Anteil der verstrichenen Spielzeit, 0 bis 1.
func progressOf(gameRec *core.Record) float64 {
	start := gameRec.GetDateTime("starts_at").Time()
	end := gameRec.GetDateTime("ends_at").Time()
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return 0
	}

	elapsed := time.Since(start).Seconds()
	total := end.Sub(start).Seconds()

	p := elapsed / total
	if p < 0 {
		return 0
	}
	if p > 1 {
		return 1
	}
	return p
}

// plannedMissions schätzt, wie viele Zwischenziele in die Spielzeit passen.
func plannedMissions(gameRec *core.Record) int {
	minutes := gameRec.GetInt("duration_min")
	if minutes <= 0 {
		minutes = 360
	}

	// Grob 35 Minuten je Zwischenziel samt Weg, und das Finale braucht Luft.
	count := (minutes - 45) / 35
	if count < 3 {
		count = 3
	}
	if count > 12 {
		count = 12
	}
	return count
}

// EnsureMission erzeugt bei Bedarf das nächste Zwischenziel für ein Team.
//
// Öffentlich, damit die Trockenübung dieselbe Routenlogik benutzt wie ein
// echter Client – zwei getrennte Wege würden über kurz oder lang
// auseinanderlaufen, und dann prüft die Übung etwas anderes als das Spiel.
func EnsureMission(app core.App, gameID, teamID string) error {
	gameRec, err := app.FindRecordById(schema.ColGames, gameID)
	if err != nil {
		return err
	}

	if open, err := openMission(app, gameID); err == nil && open != nil {
		return nil
	}

	team, err := app.FindRecordById(schema.ColTeams, teamID)
	if err != nil {
		return err
	}

	mission, err := createMission(app, gameRec, team)
	if err != nil {
		return err
	}
	if mission == nil {
		return fmt.Errorf("keine erreichbaren Ziele mehr")
	}
	return nil
}
