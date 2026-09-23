package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/game"
	"github.com/elias/operation-x/pkg/schema"
)

// Verzögerung melden.
//
// Die Kulanz am Ziel (siehe internal/game/grace.go) greift automatisch, wenn
// die Frist abläuft, während die Zielperson am Ziel steht. Das deckt den
// häufigsten Fall ab, aber nicht alle:
//
//   - Der Laden liegt zweihundert Meter neben dem Punkt, weil die Nadel auf
//     dem Eingang der Passage sitzt.
//   - Die Straßenbahn fällt aus, und die nächste kommt in zwölf Minuten.
//   - Das GPS steht zwischen Häusern um dreihundert Meter daneben.
//
// In diesen Fällen soll die Zielperson sagen können, was los ist, statt die
// Frist verstreichen zu sehen. Steht sie ohnehin am Ziel, wird die Zeit sofort
// gewährt – das ist derselbe Fall wie automatisch, nur früher bemerkt. Steht
// sie woanders, geht die Meldung an die Zentrale, die entscheidet.
//
// Bewusst kein stiller Automatismus für alles: Eine Verzögerung, die niemand
// prüfen kann, wäre ein Knopf zum Anhalten der Uhr.

type delayRequest struct {
	Reason string `json:"reason"`
}

type delayResponse struct {
	Granted   int    `json:"grantedMin"`
	Pending   bool   `json:"pending"`
	Message   string `json:"message"`
	LeftMin   int    `json:"graceLeftMin"`
	AtTarget  bool   `json:"atTarget"`
	Deadline  string `json:"deadlineAt,omitempty"`
	LeftSec   int    `json:"leftSec"`
	UsedTotal int    `json:"graceUsedMin"`
}

// handleReportDelay nimmt die Meldung der Zielperson entgegen.
func handleReportDelay(e *core.RequestEvent) error {
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

	var req delayRequest
	_ = e.BindBody(&req)
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "ohne Angabe"
	}
	reason = Kuerzen(reason, 200)

	now := time.Now()
	atTarget := game.AtTarget(e.App, mission, me, cfg)

	res := delayResponse{AtTarget: atTarget}

	if atTarget {
		granted, err := game.GrantGrace(
			e.App, gameRec, mission, me, cfg,
			cfg.MissionGraceMin, game.GraceReported+": "+reason, now,
		)
		if err != nil {
			return e.InternalServerError("Die Frist ließ sich nicht verlängern.", err)
		}

		res.Granted = granted
		if granted > 0 {
			res.Message = fmt.Sprintf(
				"%d Minuten mehr. Ihr steht am Ziel – das zählt nicht als Verzug.", granted)
		} else {
			res.Message = "Die Kulanzzeit für dieses Zwischenziel ist aufgebraucht. " +
				"Die Zentrale kann noch verlängern."
			res.Pending = true
		}
	} else {
		res.Pending = true
		res.Message = "Gemeldet. Ihr seid nicht am Ziel, deshalb entscheidet die " +
			"Zentrale – meldet euch nötigenfalls über den Funk."
	}

	// In jedem Fall ins Protokoll: Die Zentrale muss die Meldung sehen, auch
	// wenn die Zeit automatisch gewährt wurde. Sonst fiele ihr am Spielende
	// eine verlängerte Frist auf, die sie sich nicht erklären kann.
	_, _ = game.Book(e.App, game.Booking{
		Game: gameRec.Id, Team: me.Id,
		Type: game.EventMissionDelay,
		Reason: fmt.Sprintf("Verzögerung gemeldet: %s (%s)",
			reason,
			map[bool]string{true: "am Ziel", false: "unterwegs"}[atTarget]),
		Payload: map[string]any{
			"grund":    reason,
			"am_ziel":  atTarget,
			"gewaehrt": res.Granted,
			"mission":  mission.Id,
		},
	})

	mission.Set("grace_reason", reason)
	_ = e.App.Save(mission)

	fillGrace(&res, mission, cfg, now)
	return e.JSON(http.StatusOK, res)
}

type extendRequest struct {
	Minutes int    `json:"minutes"`
	Reason  string `json:"reason"`
}

// handleExtendMission verlängert die Frist auf Anweisung der Zentrale.
//
// Sie darf über die Obergrenze hinaus: Die Grenze schützt vor einem
// Dauerknopf in der Hand der Zielperson, nicht vor einer Entscheidung der
// Spielleitung. Wenn die Straßenbahn wirklich ausgefallen ist, weiß das die
// Zentrale besser als jede Regel.
func handleExtendMission(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}
	cfg := game.ConfigOf(gameRec)

	mission, err := openMission(e.App, gameRec.Id)
	if err != nil || mission == nil || mission.GetString("status") != "active" {
		return e.BadRequestError("Es läuft gerade keine Mission.", nil)
	}

	var req extendRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}
	if req.Minutes < 1 || req.Minutes > 120 {
		return e.BadRequestError("Zwischen einer Minute und zwei Stunden.", nil)
	}

	misterX, err := e.App.FindFirstRecordByFilter(schema.ColTeams,
		"game = {:g} && role = {:r}",
		map[string]any{"g": gameRec.Id, "r": schema.RoleMisterX})
	if err != nil || misterX == nil {
		return e.BadRequestError("In diesem Spiel gibt es keine Zielperson.", nil)
	}

	now := time.Now()
	base := mission.GetDateTime("deadline_at").Time()
	if base.Before(now) {
		base = now
	}
	mission.Set("deadline_at", base.Add(time.Duration(req.Minutes)*time.Minute))
	mission.Set("grace_last_at", now)

	if err := e.App.Save(mission); err != nil {
		return e.InternalServerError("Die Frist ließ sich nicht setzen.", err)
	}

	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = game.GraceGrantedHQ
	}

	_, _ = game.Book(e.App, game.Booking{
		Game: gameRec.Id, Team: misterX.Id,
		Type:   game.EventMissionGrace,
		Reason: fmt.Sprintf("Zentrale verlängert um %d Minuten: %s", req.Minutes, reason),
		Payload: map[string]any{
			"minuten": req.Minutes, "grund": reason, "von": "hq",
		},
	})

	res := delayResponse{Granted: req.Minutes, Message: "Frist verlängert."}
	fillGrace(&res, mission, cfg, now)
	return e.JSON(http.StatusOK, res)
}

func fillGrace(res *delayResponse, mission *core.Record, cfg game.Config, now time.Time) {
	state := game.GraceOf(mission, cfg)
	res.LeftMin = state.Left
	res.UsedTotal = state.UsedMin

	if deadline := mission.GetDateTime("deadline_at").Time(); !deadline.IsZero() {
		res.Deadline = deadline.UTC().Format(time.RFC3339)
		res.LeftSec = int(deadline.Sub(now).Seconds())
	}
}
