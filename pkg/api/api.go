// Package api stellt die rollengefilterten Endpunkte bereit.
//
// Sämtliche Collections sind für Clients gesperrt (siehe internal/schema).
// Was ein Spieler zu sehen bekommt, wird ausschließlich hier zusammengestellt –
// das ist die einzige Stelle, an der sich zuverlässig durchsetzen lässt, dass
// Mister X keine exakten Detektivpositionen erhält und Detektive nicht erfahren,
// welcher Hinweis gefälscht ist.
package api

import (
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/pocketbase/pocketbase/tools/hook"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/game"
	"github.com/elias/operation-x/pkg/geo"
	"github.com/elias/operation-x/pkg/hub"
	"github.com/elias/operation-x/pkg/schema"
)

// Die Dienste für Ortssuche und Grenzen. Beide halten ihre eigenen Grenzwerte
// ein und werden nur beim Einrichten befragt, nie während des Spiels.
var (
	nominatim = geo.NewNominatim()
	overpass  = geo.NewOverpass()
)

// Register hängt alle Endpunkte unter /api/opx ein.
func Register(se *core.ServeEvent) {
	watchForStream(se.App)
	guardAdminUI(se)

	// Die zweite Verschlüsselung. Scheitert sie beim Einrichten, läuft das
	// Spiel trotzdem – dann eben nur mit der Verschlüsselung des Tunnels.
	if err := setupCrypto(se.App); err != nil {
		se.App.Logger().Warn("Lagefunk nicht verfügbar", "fehler", err)
	}
	secureTraffic(se)

	g := se.Router.Group("/api/opx")

	// Öffentlich: Zustand des Servers, damit die Anmeldeseite weiß, ob
	// überhaupt ein Spiel eingerichtet ist.
	g.GET("/status", handleStatus)
	g.GET("/key", handleKey)

	// Kartenkacheln ohne Anmeldung: Die Karte fragt sie ohne Zugangstoken ab,
	// so ist das Format gebaut. Die Grenze sind Zoomstufe und Spielgebiet.
	setupTiles(se.App)
	g.GET("/tiles/{z}/{x}/{y}", handleTile)
	// Nicht "/setup": Darunter liegt bereits der Einrichtungsassistent der
	// Einsatzzentrale. Hier geht es um den allerersten Start.
	g.POST("/firstrun", handleSetup)

	// Alles Weitere setzt ein angemeldetes Team voraus.
	auth := g.Group("")
	auth.Bind(apis.RequireAuth(schema.ColTeams))
	auth.GET("/me", handleMe)
	auth.GET("/map", handleMap)
	auth.GET("/live", handleLive)
	auth.GET("/stream", handleStream)
	auth.POST("/position", handlePosition)
	auth.POST("/transit", handleTransit)
	auth.GET("/puzzles", handlePuzzles)
	auth.POST("/puzzles/{id}/solve", action(handleSolvePuzzle))
	auth.GET("/intel", handleIntel)
	auth.GET("/jokers", handleJokers)
	auth.POST("/jokers", action(handleUseJoker))
	auth.GET("/radio", handleRadio)
	auth.POST("/radio", handlePostRadio)
	auth.POST("/onboarded", handleOnboarded)
	// Regelnachschlag: für alle lesbar, nach Rolle gefiltert.
	auth.GET("/rules", handleRules)
	auth.GET("/ledger", handleLedger)

	// Zugriff – nur für die Fahndungsteams.
	det := g.Group("")
	det.Bind(apis.RequireAuth(schema.ColTeams))
	det.BindFunc(requireRole(schema.RoleDetective))
	det.POST("/sighting", action(handleSighting))
	det.POST("/arrest", action(handleArrest))

	// Missionsbuch – nur für Mister X.
	x := g.Group("/mission")
	x.Bind(apis.RequireAuth(schema.ColTeams))
	x.BindFunc(requireRole(schema.RoleMisterX))
	x.GET("", handleMission)
	x.POST("/choose", action(handleChooseOption))
	x.POST("/evidence", action(handleEvidence))
	x.POST("/abort", action(handleAbortMission))
	x.POST("/delay", action(handleReportDelay))

	// Der Einrichtungsassistent ist der Einsatzzentrale vorbehalten.
	setup := g.Group("/setup")
	setup.Bind(apis.RequireAuth(schema.ColTeams))
	setup.BindFunc(requireRole(schema.RoleHQ))
	setup.GET("/cities", handleCitySearch)
	setup.GET("/citysets", handleCitySets)
	setup.GET("/citysets/{slug}", handleCitySet)
	setup.GET("/plan", handlePlanPrompt)
	setup.POST("/plan", handleParsePlan)
	setup.POST("/districts", handleDistricts)
	setup.POST("/sectors", handleSaveSectors)
	setup.GET("/pois", handlePOIs)
	setup.POST("/hotspots", handleSaveHotspots)
	setup.GET("/puzzle-meta", handlePuzzleMeta)
	setup.POST("/puzzles", handleCreatePuzzle)
	setup.PATCH("/puzzles/{id}", handleUpdatePuzzle)
	setup.DELETE("/puzzles/{id}", handleDeletePuzzle)
	setup.POST("/puzzles/examples", handleSeedPuzzles)
	setup.POST("/rules", handleSetRules)
	setup.GET("/teamcards", handleTeamCards)
	setup.POST("/credentials", handleIssueCredentials)
	setup.PATCH("/hotspots/{id}", handleUpdateHotspot)
	setup.DELETE("/hotspots/{id}", handleDeleteHotspot)

	// Spielsteuerung und Protokoll – ebenfalls nur für die Einsatzzentrale.
	hq := g.Group("/hq")
	hq.Bind(apis.RequireAuth(schema.ColTeams))
	hq.BindFunc(requireRole(schema.RoleHQ))
	hq.POST("/status", handleSetStatus)
	hq.POST("/duration", handleSetDuration)
	hq.GET("/events", handleEvents)
	hq.GET("/evidence", handleEvidenceQueue)
	hq.POST("/evidence/{id}", handleReviewEvidence)
	hq.GET("/evidence/{id}/foto", handleEvidencePhoto)
	hq.GET("/puzzles", handleHQPuzzles)
	hq.POST("/puzzles/{id}/unlock", handleUnlockPuzzle)
	hq.GET("/penalties", handleActivePenalties)
	hq.POST("/penalties/{id}/lift", handleLiftPenalty)
	hq.POST("/adjust", handleAdjust)
	hq.POST("/mission/extend", handleExtendMission)
	hq.POST("/teams", handleCreateTeam)
	hq.DELETE("/teams/{id}", handleDeleteTeam)
	// Datenschutz zum Anfassen: Frist ändern, Spur sofort löschen.
	hq.POST("/aufbewahrung", handleSetRetention)
	hq.POST("/spur-loeschen", handlePurgeNow)
	hq.GET("/readiness", handleReadiness)
	hq.POST("/backup", handleBackup)
	hq.GET("/backups", handleBackupList)
	hq.GET("/tiles", handleTileStat)
	hq.POST("/starts", handleDrawStarts)
	hq.GET("/starts", handleStarts)
	hq.GET("/training", handleTrainingStatus)
	hq.POST("/training/start", handleTrainingStart)
	hq.POST("/training/stop", handleTrainingStop)
	hq.POST("/training/cleanup", handleTrainingCleanup)

	registerJoin(se)

	// Und zuletzt, was ein aufbauendes Programm hinzufügt.
	if Zusatzrouten != nil {
		Zusatzrouten(se)
	}
}

// requireAuthTeams kapselt die Anmeldeprüfung, damit sie auch außerhalb von
// Register() verwendet werden kann.
func requireAuthTeams() *hook.Handler[*core.RequestEvent] {
	return apis.RequireAuth(schema.ColTeams)
}

// guardAdminUI sperrt die Datenbankverwaltung für alles außer den eigenen Rechner.
//
// PocketBase bringt unter /_/ eine vollständige Administrationsoberfläche mit.
// Sie ist beim Entwickeln nützlich und im Spielbetrieb ein Problem: Sobald der
// Tunnel steht, hinge sie unter einer öffentlichen Adresse – eine
// Datenbankverwaltung neben einem Spiel, das acht Leuten offensteht, von denen
// mindestens einer neugierig ist.
//
// Vom eigenen Rechner aus bleibt sie erreichbar. Wer am Server sitzt, hat
// ohnehin Zugriff auf die Datei.
func guardAdminUI(se *core.ServeEvent) {
	se.Router.BindFunc(func(e *core.RequestEvent) error {
		if !strings.HasPrefix(e.Request.URL.Path, "/_/") {
			return e.Next()
		}
		if isLocalRequest(e.Request) {
			return e.Next()
		}
		return e.NotFoundError("Nicht gefunden.", nil)
	})
}

// isLocalRequest sagt, ob eine Anfrage vom selben Rechner kommt.
//
// Bewusst ohne Rücksicht auf X-Forwarded-For: Dieser Kopfzeile darf man genau
// dann trauen, wenn ein eigener Zwischenserver sie setzt – und der Tunnel ist
// keiner. Wer über cloudflared kommt, kommt von außen, egal was in der Anfrage
// steht.
func isLocalRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// watchForStream meldet dem Lagestrom jede Änderung, die auf einem Bildschirm
// sichtbar wird.
//
// Als Haken am Datenmodell und nicht als Zeile in jedem Endpunkt: Eine
// vergessene Meldung wäre ein Fehler, den niemand bemerkt – die Anzeige bliebe
// einfach stehen, bis zufällig etwas anderes passiert. So kann sie nicht
// vergessen werden.
//
// Buchungen melden sich selbst (siehe game.Book); hier stehen die Änderungen,
// die ohne Buchung auskommen.
func watchForStream(app core.App) {
	for _, collection := range []string{
		schema.ColPositions, // die Punkte auf der Karte
		schema.ColGames,     // Spielzustand, Finale, Ausgang
		schema.ColPenalties, // Sperren laufen an und aus
		schema.ColTeams,     // Transit, Punktestand
		schema.ColZones,     // Sperrzonen und Wanzen
		schema.ColIntel,     // neue Hinweise
	} {
		app.OnRecordAfterCreateSuccess(collection).BindFunc(func(e *core.RecordEvent) error {
			hub.Notify()
			return e.Next()
		})
		app.OnRecordAfterUpdateSuccess(collection).BindFunc(func(e *core.RecordEvent) error {
			hub.Notify()
			return e.Next()
		})
		app.OnRecordAfterDeleteSuccess(collection).BindFunc(func(e *core.RecordEvent) error {
			hub.Notify()
			return e.Next()
		})
	}
}

// action umschließt einen Endpunkt, der eine echte Spielhandlung darstellt,
// und hält deren Zeitpunkt für das Anti-Camping fest.
//
// Als Umschlag statt als Zeile in jedem Handler: So kann keine Handlung
// vergessen werden, und die Handler bleiben von einer Regel unberührt, die mit
// ihrer eigentlichen Aufgabe nichts zu tun hat.
//
// Vermerkt wird nur bei Erfolg. Eine abgelehnte Anfrage ist keine Handlung –
// sonst hielte sich jemand mit falschen Rätselantworten wach.
func action(h func(*core.RequestEvent) error) func(*core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		err := h(e)
		if err == nil && e.Auth != nil {
			game.MarkAction(e.App, e.Auth.Id)
		}
		return err
	}
}

// kuerzen schneidet einen Text auf eine Höchstlänge – nach Zeichen, nicht nach
// Bytes.
//
// text[:500] schneidet Bytes. Trifft die Grenze mitten in ein Zeichen, das
// mehrere Bytes belegt, entsteht ungültiges UTF-8 – und im Deutschen liegt an
// jeder dritten Stelle ein Umlaut. Aus "Königsbrücker Straße" würde dann ein
// Fragezeichen mitten im Wort.
func kuerzen(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes])
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// requireRole lässt nur bestimmte Rollen durch.
func requireRole(allowed ...string) func(*core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.UnauthorizedError("Nicht angemeldet.", nil)
		}

		role := e.Auth.GetString("role")
		for _, want := range allowed {
			if role == want {
				return e.Next()
			}
		}

		return e.ForbiddenError("Diese Ansicht ist der Einsatzzentrale vorbehalten.", nil)
	}
}

// handleStatus verrät bewusst nur, ob und welches Spiel läuft – keine Spieldaten.
//
// Die Antwort wird als Karte zusammengesetzt statt als Struktur verschickt,
// damit ein aufbauendes Programm eigene Felder beilegen kann, ohne dass dieses
// Verzeichnis sie kennen muss.
func handleStatus(e *core.RequestEvent) error {
	antwort := map[string]any{
		"ready": false,
	}
	if setupNeeded(e.App) {
		antwort["setupNeeded"] = true
	}

	if game, err := SpielDesServers(e.App); err == nil && game != nil {
		antwort["ready"] = true
		antwort["gameName"] = game.GetString("name")
		antwort["status"] = game.GetString("status")
		antwort["city"] = game.GetString("city")
	}

	if StatusZusatz != nil {
		for k, v := range StatusZusatz(e) {
			antwort[k] = v
		}
	}

	return e.JSON(http.StatusOK, antwort)
}

type meResponse struct {
	ID       string `json:"id"`
	Callsign string `json:"callsign"`
	Display  string `json:"display"`
	Role     string `json:"role"`
	Color    string `json:"color"`
	FP       int    `json:"fp"`
	Points   int    `json:"points"`

	// Ob die Einweisung durchlaufen wurde – danach richtet sich, ob der
	// Client sie beim Anmelden zeigt.
	Onboarded bool `json:"onboarded"`

	Game *meGame `json:"game,omitempty"`
}

type meGame struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	City     string `json:"city"`
	Status   string `json:"status"`
	StartsAt string `json:"startsAt,omitempty"`
	EndsAt   string `json:"endsAt,omitempty"`
	// Die geplante Dauer in Minuten. Das Regelpult zeigt sie an und ändert
	// sie – ohne diese Zeile wüsste es nicht, wovon es ausgeht.
	DurationMin int `json:"durationMin,omitempty"`
}

// handleMe liefert dem angemeldeten Team seinen eigenen Zustand.
func handleMe(e *core.RequestEvent) error {
	team := e.Auth

	res := meResponse{
		ID:        team.Id,
		Callsign:  team.GetString("callsign"),
		Display:   team.GetString("display"),
		Role:      team.GetString("role"),
		Color:     team.GetString("color"),
		FP:        team.GetInt("fp"),
		Points:    team.GetInt("points"),
		Onboarded: !team.GetDateTime("onboarded_at").Time().IsZero(),
	}

	if gameID := team.GetString("game"); gameID != "" {
		game, err := e.App.FindRecordById(schema.ColGames, gameID)
		if err == nil {
			res.Game = &meGame{
				ID:          game.Id,
				Name:        game.GetString("name"),
				City:        game.GetString("city"),
				Status:      game.GetString("status"),
				StartsAt:    game.GetDateTime("starts_at").String(),
				EndsAt:      game.GetDateTime("ends_at").String(),
				DurationMin: game.GetInt("duration_min"),
			}
		}
	}

	return e.JSON(http.StatusOK, res)
}

// gameOf liefert das Spiel, zu dem der Anmeldende gehört.
//
// Jeder angemeldete Zugang gehört zu genau einem Spiel – das steht im
// Datensatz des Teams und kommt nicht aus der Anfrage. Damit beantwortet ein
// Server mit mehreren gleichzeitigen Spielen jede Frage aus dem Spiel dessen,
// der sie stellt, und aus keinem anderen.
//
// Das ist der Grund, warum currentGame nur noch dort steht, wo niemand
// angemeldet ist: Ein Handler, der sich „das laufende Spiel“ greift, würde bei
// mehreren laufenden Spielen irgendeines erwischen.
func gameOf(e *core.RequestEvent) (*core.Record, error) {
	if e.Auth == nil {
		return nil, e.UnauthorizedError("Nicht angemeldet.", nil)
	}

	rec, err := e.App.FindRecordById(schema.ColGames, e.Auth.GetString("game"))
	if err != nil {
		return nil, e.NotFoundError("Es ist kein Spiel eingerichtet.", nil)
	}
	return rec, nil
}

// ausSpiel holt einen Datensatz und besteht darauf, dass er zu diesem Spiel
// gehört.
//
// Kennungen kommen aus der Anfrage – aus dem Pfad oder dem Rumpf –, und eine
// Kennung ist kein Ausweis. Ohne diese Prüfung könnte ein Team aus Spiel A ein
// Rätsel aus Spiel B freischalten, einen Hotspot aus Spiel B verschieben oder
// einen Nachweis aus Spiel B abnicken, indem es die Kennung errät oder
// aufschnappt. In einem Server mit einem einzigen Spiel fällt das nie auf; in
// einem mit dreien ist es die ganze Trennung.
func ausSpiel(e *core.RequestEvent, collection, id, gameID string) (*core.Record, error) {
	rec, err := e.App.FindRecordById(collection, id)
	if err != nil || rec.GetString("game") != gameID {
		// Dieselbe Antwort für „gibt es nicht“ und „gehört einem anderen
		// Spiel“: Der Unterschied verriete, dass es die Kennung gibt.
		return nil, e.NotFoundError("Das gibt es in diesem Spiel nicht.", nil)
	}
	return rec, nil
}

// currentGame liefert das jüngste nicht abgeschlossene Spiel.
//
// Nur noch für die Endpunkte ohne Anmeldung – Status, Beitrittsseite,
// Ersteinrichtung – und für den Kachelspeicher. Alles, wo jemand angemeldet
// ist, nimmt gameOf.
func currentGame(app core.App) (*core.Record, error) {
	records, err := app.FindRecordsByFilter(
		schema.ColGames,
		"status != {:finished}",
		"-created",
		1,
		0,
		map[string]any{"finished": schema.GameFinished},
	)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	return records[0], nil
}
