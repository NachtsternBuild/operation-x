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

// Zeitfenster, in dem eine Uhrzeitangabe bei Stufe 2 noch als richtig gilt.
//
// Auf die Minute genau wäre eine Gedächtnisprüfung, keine Ermittlung: Die Teams
// rekonstruieren aus Hinweisen, und deren Zeitstempel sind selbst gerundet.
const arrestTimeToleranceMin = 10

type arrestRequest struct {
	Level          int    `json:"level"`
	ClaimedHotspot int    `json:"claimedHotspot"`
	ClaimedTime    string `json:"claimedTime"`
	ClaimedTarget  int    `json:"claimedTarget"`
}

type arrestResponse struct {
	Level   int            `json:"level"`
	Correct bool           `json:"correct"`
	Points  int            `json:"points"`
	Detail  map[string]any `json:"detail"`
	Victory bool           `json:"victory"`
	Message string         `json:"message"`
}

// handleArrest wertet das Zugriffsformular aus.
//
// Drei Stufen mit steigendem Einsatz: Stufe 1 und 2 bringen Punkte und kosten
// nichts, Stufe 3 entscheidet das Spiel – oder kostet das Team eine Sperre,
// einen Fluchtpunkt und fünfzehn Punkte. Das ist die einzige Stelle im
// Regelwerk, an der ein Team alles auf eine Karte setzen kann.
func handleArrest(e *core.RequestEvent) error {
	me := e.Auth
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}
	if gameRec.GetString("status") == schema.GameFinished {
		return e.BadRequestError("Das Spiel ist bereits entschieden.", nil)
	}
	cfg := game.ConfigOf(gameRec)

	var req arrestRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}
	if req.Level < 1 || req.Level > 3 {
		return e.BadRequestError("Stufe muss zwischen 1 und 3 liegen.", nil)
	}

	// Laufende Sperre? Dann ist das Team gerade nicht zugriffsfähig.
	if locks, err := game.ActivePenalties(e.App, me.Id); err == nil && len(locks) > 0 {
		return e.BadRequestError("Das Team ist gesperrt: "+locks[0].GetString("reason"), nil)
	}

	myPos := latestPosition(e.App, me.Id)
	if myPos == nil {
		return e.BadRequestError("Es liegt keine Standortmeldung vor.", nil)
	}
	here := geo.Point{Lat: myPos.GetFloat("lat"), Lng: myPos.GetFloat("lng")}

	// Das Team muss an dem Hotspot stehen, den es benennt. Ein Zugriff ist eine
	// Handlung im Feld, keine Behauptung vom Sofa.
	claimed, err := hotspotByNumber(e.App, gameRec.Id, req.ClaimedHotspot)
	if err != nil {
		return e.BadRequestError(fmt.Sprintf("Es gibt keinen Hotspot mit der Nummer %d.", req.ClaimedHotspot), nil)
	}

	toClaimed := geo.DistanceM(here, geo.Point{Lat: claimed.GetFloat("lat"), Lng: claimed.GetFloat("lng")})
	allowed := cfg.HotspotMaxDistanceM + myPos.GetFloat("accuracy")
	if toClaimed > allowed {
		return e.BadRequestError(
			fmt.Sprintf("Der Zugriff muss am angegebenen Punkt erfolgen. Bis #%02d sind es %.0f m.",
				req.ClaimedHotspot, toClaimed), nil)
	}

	misterX, err := e.App.FindFirstRecordByFilter(schema.ColTeams,
		"game = {:g} && role = {:r}",
		map[string]any{"g": gameRec.Id, "r": schema.RoleMisterX})
	if err != nil || misterX == nil {
		return e.BadRequestError("In diesem Spiel gibt es keine Zielperson.", nil)
	}

	detail := map[string]any{}
	now := time.Now()

	// --- Stufe 1: Ist die Zielperson tatsächlich hier? ---
	xPos := latestPosition(e.App, misterX.Id)
	locationOK := false
	if xPos != nil {
		d := geo.DistanceM(
			geo.Point{Lat: xPos.GetFloat("lat"), Lng: xPos.GetFloat("lng")},
			geo.Point{Lat: claimed.GetFloat("lat"), Lng: claimed.GetFloat("lng")},
		)
		locationOK = d <= cfg.HotspotMaxDistanceM
	}
	detail["lokalisierung"] = locationOK

	// --- Stufe 2: Stimmt auch die Uhrzeit? ---
	timeOK := false
	if req.Level >= 2 {
		claimedTime := parseTime(req.ClaimedTime, time.Time{})
		if !claimedTime.IsZero() {
			timeOK = wasThereAt(e.App, misterX.Id, claimed, cfg.HotspotMaxDistanceM, claimedTime)
		}
		detail["rekonstruktion"] = timeOK
	}

	// --- Stufe 3: Und das nächste Fluchtziel? ---
	targetOK := false
	if req.Level >= 3 {
		if final, err := e.App.FindRecordById(schema.ColHotspots, gameRec.GetString("final_target")); err == nil {
			targetOK = final.GetInt("number") == req.ClaimedTarget
		}
		detail["fluchtziel"] = targetOK
	}

	correct := locationOK &&
		(req.Level < 2 || timeOK) &&
		(req.Level < 3 || targetOK)

	// Formular protokollieren, unabhängig vom Ausgang.
	col, err := e.App.FindCollectionByNameOrId(schema.ColArrests)
	if err != nil {
		return err
	}
	rec := core.NewRecord(col)
	rec.Set("game", gameRec.Id)
	rec.Set("team", me.Id)
	rec.Set("level", req.Level)
	rec.Set("hotspot", claimed.Id)
	rec.Set("claimed_hotspot", req.ClaimedHotspot)
	rec.Set("claimed_target", fmt.Sprint(req.ClaimedTarget))
	rec.Set("correct", correct)
	rec.Set("detail", detail)
	rec.Set("resolved_at", now)
	if t := parseTime(req.ClaimedTime, time.Time{}); !t.IsZero() {
		rec.Set("claimed_time", t)
	}
	if err := e.App.Save(rec); err != nil {
		return e.InternalServerError("Zugriff konnte nicht gespeichert werden.", err)
	}

	res := arrestResponse{Level: req.Level, Correct: correct, Detail: detail}

	switch {
	case req.Level == 3 && correct:
		res.Points = cfg.PointsVictory
		res.Victory = true
		res.Message = "Zugriff erfolgreich. Die Zielperson ist gestellt."

		if _, err := game.Book(e.App, game.Booking{
			Game: gameRec.Id, Team: me.Id,
			Type:        game.EventArrestWin,
			Reason:      "Vollständiger Zugriff gelungen",
			DeltaPoints: cfg.PointsVictory,
			DedupeKey:   "arrest.win:" + rec.Id,
			Payload:     detail,
		}); err != nil {
			return e.InternalServerError("Buchung fehlgeschlagen.", err)
		}

		if err := game.FinishGame(e.App, gameRec, "detectives",
			fmt.Sprintf("Zugriff Stufe 3 durch %s", me.GetString("callsign"))); err != nil {
			return e.InternalServerError("Spielende konnte nicht gesetzt werden.", err)
		}

	case req.Level == 3:
		// Fehlzugriff: die teuerste Fehlentscheidung im Regelwerk.
		res.Points = cfg.PointsArrestFailed
		res.Message = "Fehlzugriff. Sperre, Punktabzug und ein Fluchtpunkt weniger."

		if _, err := game.Book(e.App, game.Booking{
			Game: gameRec.Id, Team: me.Id,
			Type:        game.EventArrestFail,
			Reason:      "Fehlzugriff auf Stufe 3",
			DeltaPoints: cfg.PointsArrestFailed,
			DeltaFP:     -1,
			DedupeKey:   "arrest.fail:" + rec.Id,
			Payload:     detail,
		}); err != nil {
			return e.InternalServerError("Buchung fehlgeschlagen.", err)
		}

		until := now.Add(time.Duration(cfg.ArrestFailLockoutMin) * time.Minute)
		_ = game.Penalize(e.App, gameRec.Id, me.Id, "lockout_location",
			"Fehlzugriff auf Stufe 3", until, here.Lat, here.Lng)

	case correct:
		points := cfg.PointsArrestLevel1
		label := "Lokalisierung bestätigt"
		if req.Level == 2 {
			points = cfg.PointsArrestLevel2
			label = "Rekonstruktion bestätigt"
		}
		res.Points = points
		res.Message = label + "."

		if _, err := game.Book(e.App, game.Booking{
			Game: gameRec.Id, Team: me.Id,
			Type:        game.EventArrestPartial,
			Reason:      label,
			DeltaPoints: points,
			DedupeKey:   "arrest.partial:" + rec.Id,
			Payload:     detail,
		}); err != nil {
			return e.InternalServerError("Buchung fehlgeschlagen.", err)
		}

	default:
		// Stufe 1 und 2 kosten bei einem Fehlschlag nichts – sie sind das
		// Werkzeug, mit dem sich eine Vermutung gefahrlos prüfen lässt.
		res.Message = "Die Angaben treffen nicht zu."
	}

	return e.JSON(http.StatusOK, res)
}

// wasThereAt prüft, ob sich ein Team zur angegebenen Zeit am Hotspot aufhielt.
func wasThereAt(app core.App, teamID string, hotspot *core.Record, radiusM float64, at time.Time) bool {
	from := at.Add(-arrestTimeToleranceMin * time.Minute)
	to := at.Add(arrestTimeToleranceMin * time.Minute)

	positions, err := app.FindRecordsByFilter(
		schema.ColPositions,
		"team = {:t} && captured_at >= {:from} && captured_at <= {:to}",
		"captured_at", 200, 0,
		map[string]any{
			"t":    teamID,
			"from": game.DBTime(from),
			"to":   game.DBTime(to),
		},
	)
	if err != nil {
		return false
	}

	target := geo.Point{Lat: hotspot.GetFloat("lat"), Lng: hotspot.GetFloat("lng")}
	for _, p := range positions {
		d := geo.DistanceM(target, geo.Point{Lat: p.GetFloat("lat"), Lng: p.GetFloat("lng")})
		if d <= radiusM {
			return true
		}
	}
	return false
}

func hotspotByNumber(app core.App, gameID string, number int) (*core.Record, error) {
	return app.FindFirstRecordByFilter(
		schema.ColHotspots,
		"game = {:g} && number = {:n}",
		map[string]any{"g": gameID, "n": number},
	)
}
