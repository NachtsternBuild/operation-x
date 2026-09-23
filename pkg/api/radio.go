package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/game"
	"github.com/elias/operation-x/pkg/schema"
)

// Vertrauensstufen nach Regelwerk. Sie vergibt ausschließlich die Zentrale.
const (
	TrustConfirmed   = "confirmed"   // verifizierte System- oder HQ-Information
	TrustUnconfirmed = "unconfirmed" // möglicher Zeugenbericht
	TrustRumor       = "rumor"       // Gerücht, womöglich Fehlinformation
)

type radioEntry struct {
	ID       string `json:"id"`
	Author   string `json:"author"`
	Role     string `json:"role,omitempty"`
	Color    string `json:"color,omitempty"`
	Text     string `json:"text"`
	Trust    string `json:"trust,omitempty"`
	Audience string `json:"audience"`
	Pinned   bool   `json:"pinned"`
	At       string `json:"at"`
	Mine     bool   `json:"mine"`
}

// handleRadio liefert den Funkkanal, wie ihn die anfragende Rolle sehen darf.
//
// Der Kanal gehört der Fahndung und der Zentrale. Die Zielperson liest nicht
// mit – sie bekommt nur, was ausdrücklich an sie gerichtet ist, etwa ein
// Sichtkontakt-Alarm oder eine Ansage der Spielleitung.
func handleRadio(e *core.RequestEvent) error {
	me := e.Auth
	gameID := me.GetString("game")
	role := me.GetString("role")

	filter := "game = {:g}"
	params := map[string]any{"g": gameID}

	switch role {
	case schema.RoleMisterX:
		filter += " && (audience = 'misterx' || audience = 'all')"
	case schema.RoleDetective:
		filter += " && (audience = 'detectives' || audience = 'all')"
	}

	records, err := e.App.FindRecordsByFilter(schema.ColRadio, filter, "-created", 120, 0, params)
	if err != nil {
		return err
	}

	// Rufzeichen einmal nachschlagen statt je Nachricht.
	teams := map[string]*core.Record{}
	if list, err := e.App.FindRecordsByFilter(schema.ColTeams, "game = {:g}", "", 0, 0,
		map[string]any{"g": gameID}); err == nil {
		for _, t := range list {
			teams[t.Id] = t
		}
	}

	out := make([]radioEntry, 0, len(records))
	for i := len(records) - 1; i >= 0; i-- { // älteste zuerst, wie in einem Chat
		r := records[i]

		entry := radioEntry{
			ID:       r.Id,
			Author:   r.GetString("author"),
			Text:     r.GetString("text"),
			Trust:    r.GetString("trust"),
			Audience: r.GetString("audience"),
			Pinned:   r.GetBool("pinned"),
			At:       r.GetDateTime("created").Time().UTC().Format(time.RFC3339),
		}

		if teamID := r.GetString("team"); teamID != "" {
			entry.Mine = teamID == me.Id
			if t, ok := teams[teamID]; ok {
				entry.Role = t.GetString("role")
				entry.Color = t.GetString("color")
				if entry.Author == "" {
					entry.Author = t.GetString("display")
				}
			}
		} else if entry.Author == "" {
			entry.Author = "System"
		}

		out = append(out, entry)
	}

	return e.JSON(http.StatusOK, map[string]any{"messages": out})
}

type radioPostRequest struct {
	Text     string `json:"text"`
	Trust    string `json:"trust"`
	Audience string `json:"audience"`
	Pinned   bool   `json:"pinned"`
}

// handlePostRadio schickt eine Nachricht in den Kanal.
//
// Vertrauensstufe und Empfängerkreis darf nur die Zentrale bestimmen. Ein
// Fahndungsteam, das seine eigene Vermutung als „bestätigt“ kennzeichnen
// könnte, würde den ganzen Sinn der Abstufung aushebeln.
func handlePostRadio(e *core.RequestEvent) error {
	me := e.Auth
	gameID := me.GetString("game")
	role := me.GetString("role")

	if role == schema.RoleMisterX {
		return e.ForbiddenError("Die Zielperson hat keinen Zugang zum Funkkanal.", nil)
	}

	var req radioPostRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}

	text := strings.TrimSpace(req.Text)
	if text == "" {
		return e.BadRequestError("Die Nachricht ist leer.", nil)
	}
	if len([]rune(text)) > 500 {
		text = kuerzen(text, 500)
	}

	col, err := e.App.FindCollectionByNameOrId(schema.ColRadio)
	if err != nil {
		return err
	}

	rec := core.NewRecord(col)
	rec.Set("game", gameID)
	rec.Set("team", me.Id)
	rec.Set("author", me.GetString("display"))
	rec.Set("text", text)

	if role == schema.RoleHQ {
		trust := req.Trust
		switch trust {
		case TrustConfirmed, TrustUnconfirmed, TrustRumor:
		default:
			trust = TrustConfirmed
		}
		rec.Set("trust", trust)
		rec.Set("pinned", req.Pinned)

		audience := req.Audience
		switch audience {
		case "detectives", "misterx", "all":
		default:
			audience = "detectives"
		}
		rec.Set("audience", audience)
	} else {
		// Teamnachrichten tragen keine Stufe: Sie sind Absprachen, keine
		// Lagemeldungen.
		rec.Set("audience", "detectives")
	}

	if err := e.App.Save(rec); err != nil {
		return e.InternalServerError("Nachricht konnte nicht gesendet werden.", err)
	}

	return e.JSON(http.StatusOK, map[string]any{"ok": true, "id": rec.Id})
}

// PostSystemMessage schreibt eine Systemmeldung in den Kanal. Wird von der
// Engine benutzt, etwa bei einem Alarm.
func PostSystemMessage(app core.App, gameID, text, audience string) error {
	col, err := app.FindCollectionByNameOrId(schema.ColRadio)
	if err != nil {
		return err
	}

	rec := core.NewRecord(col)
	rec.Set("game", gameID)
	rec.Set("author", "System")
	rec.Set("text", text)
	rec.Set("trust", TrustConfirmed)
	rec.Set("audience", audience)

	return app.Save(rec)
}

// --- Eingriffe der Spielleitung ----------------------------------------------

type adjustRequest struct {
	TeamID string `json:"teamId"`
	Points int    `json:"points"`
	FP     int    `json:"fp"`
	Reason string `json:"reason"`
}

// handleAdjust korrigiert Punkte oder Fluchtpunkte von Hand.
//
// Gebraucht wird das öfter, als einem lieb ist: Ein Handy fällt aus, jemand
// steht im Funkloch, eine Regel wird vor Ort anders ausgelegt. Wichtig ist,
// dass auch eine Korrektur im Protokoll steht – mit Begründung, damit später
// niemand rätselt, woher die Punkte kamen.
func handleAdjust(e *core.RequestEvent) error {
	var req adjustRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}
	if req.TeamID == "" {
		return e.BadRequestError("Es wurde kein Team angegeben.", nil)
	}
	if req.Points == 0 && req.FP == 0 {
		return e.BadRequestError("Es wurde nichts zu korrigieren angegeben.", nil)
	}

	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		return e.BadRequestError("Bitte eine Begründung angeben.", nil)
	}

	team, err := e.App.FindRecordById(schema.ColTeams, req.TeamID)
	if err != nil {
		return e.NotFoundError("Dieses Team gibt es nicht.", nil)
	}

	if _, err := game.Book(e.App, game.Booking{
		Game: team.GetString("game"), Team: team.Id,
		Type:        "hq.adjust",
		Reason:      "Korrektur durch die Zentrale: " + reason,
		DeltaPoints: req.Points,
		DeltaFP:     req.FP,
	}); err != nil {
		return e.InternalServerError("Korrektur fehlgeschlagen.", err)
	}

	fresh, _ := e.App.FindRecordById(schema.ColTeams, team.Id)
	return e.JSON(http.StatusOK, map[string]any{
		"points": fresh.GetInt("points"),
		"fp":     fresh.GetInt("fp"),
	})
}

// handleLiftPenalty hebt eine Sperre vorzeitig auf.
func handleLiftPenalty(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	rec, err := ausSpiel(e, schema.ColPenalties, e.Request.PathValue("id"), gameRec.Id)
	if err != nil {
		return err
	}

	rec.Set("active", false)
	rec.Set("expires_at", time.Now())
	if err := e.App.Save(rec); err != nil {
		return e.InternalServerError("Sperre konnte nicht aufgehoben werden.", err)
	}

	_, _ = game.Book(e.App, game.Booking{
		Game: rec.GetString("game"), Team: rec.GetString("team"),
		Type:   "hq.lift_penalty",
		Reason: "Sperre von der Zentrale aufgehoben: " + rec.GetString("reason"),
	})

	return e.JSON(http.StatusOK, map[string]any{"ok": true})
}

// handleActivePenalties listet alle laufenden Sperren des Spiels.
func handleActivePenalties(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	now := time.Now()
	records, err := e.App.FindRecordsByFilter(
		schema.ColPenalties,
		"game = {:g} && active = true && expires_at > {:now}",
		"-started_at", 0, 0,
		map[string]any{"g": gameRec.Id, "now": game.DBTime(now)},
	)
	if err != nil {
		return err
	}

	type entry struct {
		ID       string `json:"id"`
		Callsign string `json:"callsign"`
		Kind     string `json:"kind"`
		Reason   string `json:"reason"`
		LeftSec  int    `json:"leftSec"`
	}

	out := make([]entry, 0, len(records))
	for _, r := range records {
		item := entry{
			ID:      r.Id,
			Kind:    r.GetString("kind"),
			Reason:  r.GetString("reason"),
			LeftSec: int(r.GetDateTime("expires_at").Time().Sub(now).Seconds()),
		}
		if t, err := e.App.FindRecordById(schema.ColTeams, r.GetString("team")); err == nil {
			item.Callsign = t.GetString("callsign")
		}
		out = append(out, item)
	}

	return e.JSON(http.StatusOK, map[string]any{"penalties": out})
}

type newTeamRequest struct {
	Callsign string `json:"callsign"`
	Display  string `json:"display"`
	Password string `json:"password"`
	// Rolle: Fahndungsteam, sofern nichts anderes dasteht.
	//
	// Dass die Zielperson hier fehlte, war lange unsichtbar: Der Befehl
	// "operationx seed" legt sie an, und wer damit entwickelt, merkt nie, dass
	// ein Spiel aus der Oberfläche heraus keine bekommen kann. Wer die Datei
	// nur angeklickt hat, stand vor einem Spiel ohne Zielperson.
	Role string `json:"role"`
}

// handleCreateTeam legt mitten im Spiel ein weiteres Fahndungsteam an.
//
// Das Regelwerk sieht das ausdrücklich vor – und in der Praxis kommt es vor,
// dass jemand später dazustößt oder ein Handy den Geist aufgibt und ein Team
// einen neuen Zugang braucht.
func handleCreateTeam(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	var req newTeamRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}

	callsign := strings.TrimSpace(req.Callsign)
	if callsign == "" {
		return e.BadRequestError("Das Rufzeichen fehlt.", nil)
	}
	if len(req.Password) < 8 {
		return e.BadRequestError("Das Kennwort braucht mindestens acht Zeichen.", nil)
	}

	// Die Zentrale entsteht bei der Einrichtung und bleibt einmalig; alles
	// andere lässt sich hier anlegen.
	role := schema.RoleDetective
	if strings.TrimSpace(req.Role) == schema.RoleMisterX {
		role = schema.RoleMisterX

		// Zwei Zielpersonen wären kein Spiel: Jede Regel, jeder Hinweis und
		// jeder Zugriff bezieht sich auf genau eine.
		vorhanden, err := e.App.FindRecordsByFilter(schema.ColTeams,
			"game = {:g} && role = {:r}", "", 1, 0,
			map[string]any{"g": gameRec.Id, "r": schema.RoleMisterX})
		if err != nil {
			return err
		}
		if len(vorhanden) > 0 {
			return e.BadRequestError(
				"Dieses Spiel hat schon eine Zielperson. Für einen Wechsel das "+
					"Kennwort des vorhandenen Zugangs neu vergeben.", nil)
		}
	}

	farbe := "#8ea6ff"
	startFP := cfgStartFP(gameRec, role)
	if role == schema.RoleMisterX {
		farbe = "#ff5c47"
	}

	col, err := e.App.FindCollectionByNameOrId(schema.ColTeams)
	if err != nil {
		return err
	}

	cfg := game.ConfigOf(gameRec)
	now := time.Now()

	rec := core.NewRecord(col)
	rec.Set("callsign", Rufzeichen(callsign, gameRec))
	rec.Set("display", strings.TrimSpace(req.Display))
	rec.Set("role", role)
	rec.Set("game", gameRec.Id)
	rec.Set("color", farbe)
	// Das Guthaben kommt gleich aus dem Kontobuch – siehe game.BookStart.
	rec.Set("fp", 0)
	rec.Set("points", 0)
	rec.Set("active", true)
	rec.Set("ping_due_at", now.Add(cfg.PingInterval(false)))
	rec.SetPassword(req.Password)

	if err := e.App.Save(rec); err != nil {
		return e.BadRequestError("Team konnte nicht angelegt werden – ist das Rufzeichen schon vergeben?", err)
	}

	_, _ = game.Book(e.App, game.Booking{
		Game: gameRec.Id, Team: rec.Id,
		Type:   "hq.team_added",
		Reason: "Team " + callsign + " nachträglich angelegt",
	})
	if err := game.BookStart(e.App, gameRec.Id, rec.Id, startFP); err != nil {
		return e.InternalServerError("Startguthaben konnte nicht gebucht werden.", err)
	}

	return e.JSON(http.StatusOK, map[string]any{
		"id":       rec.Id,
		"callsign": rec.GetString("callsign"),
		"role":     role,
	})
}

// cfgStartFP liefert das Startguthaben der Rolle.
func cfgStartFP(gameRec *core.Record, role string) int {
	cfg := game.ConfigOf(gameRec)
	if role == schema.RoleMisterX {
		return cfg.StartFPMisterX
	}
	return cfg.StartFPDetective
}
