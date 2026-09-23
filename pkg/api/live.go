package api

import (
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/game"
	"github.com/elias/operation-x/pkg/geo"
	"github.com/elias/operation-x/pkg/schema"
)

// Positionen, die älter sind als dies, gelten als veraltet und werden nicht
// mehr ausgeliefert – eine zwei Stunden alte Markierung auf der Karte wäre
// irreführender als gar keine.
const staleAfter = 45 * time.Minute

type livePosition struct {
	Team     string  `json:"team"`
	Callsign string  `json:"callsign"`
	Display  string  `json:"display"`
	Role     string  `json:"role"`
	Color    string  `json:"color"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`

	// BlurM ist der Radius, um den die Position verschoben wurde. Null heißt
	// exakt. Der Client zeichnet daraus den Unschärfekreis.
	BlurM float64 `json:"blurM"`

	Accuracy   float64 `json:"accuracy,omitempty"`
	CapturedAt string  `json:"capturedAt"`
	AgeSec     int     `json:"ageSec"`
	InTransit  bool    `json:"inTransit"`
	Stale      bool    `json:"stale"`

	// Self kennzeichnet die eigene Position. Auf einer Karte mit mehreren
	// gleichfarbigen Punkten ist die Frage "welcher bin ich?" sonst eine
	// Denkaufgabe – im Gehen, bei Sonne, die niemand lösen will.
	Self bool `json:"self,omitempty"`
}

type liveSelf struct {
	FP         int    `json:"fp"`
	Points     int    `json:"points"`
	InTransit  bool   `json:"inTransit"`
	NextDueAt  string `json:"nextDueAt,omitempty"`
	DueInSec   int    `json:"dueInSec"`
	Violations int    `json:"violations"`
	Overdue    bool   `json:"overdue"`

	// Der ausgeloste Startpunkt. Er steht in der Lage und nicht in den
	// Zugangsdaten, weil er genau dann gebraucht wird, wenn alle auf ihre
	// Geräte sehen: kurz vor dem Start, auf dem Weg dorthin.
	Start *liveStart `json:"start,omitempty"`

	Lockouts []liveLockout `json:"lockouts"`
}

// liveStart ist der ausgeloste Startpunkt eines Teams.
type liveStart struct {
	Number int     `json:"number"`
	Name   string  `json:"name"`
	Lat    float64 `json:"lat"`
	Lng    float64 `json:"lng"`
}

// livePause beschreibt eine laufende Pause.
type livePause struct {
	Reason  string `json:"reason,omitempty"`
	Since   string `json:"since,omitempty"`
	Until   string `json:"until,omitempty"`
	LeftSec int    `json:"leftSec,omitempty"`
}

// liveAlert ist ein Ereignis, das dem Team gemeldet gehört.
type liveAlert struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Reason string `json:"reason"`
	At     string `json:"at"`
	Urgent bool   `json:"urgent"`
}

type liveLockout struct {
	Kind      string `json:"kind"`
	Reason    string `json:"reason"`
	ExpiresAt string `json:"expiresAt"`
	LeftSec   int    `json:"leftSec"`
}

type liveFinale struct {
	Active  bool    `json:"active"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	RadiusM float64 `json:"radiusM"`
	Sector  string  `json:"sector,omitempty"`
}

type liveOutcome struct {
	Winner string `json:"winner"`
	Reason string `json:"reason"`
}

type liveResponse struct {
	Now       string         `json:"now"`
	Status    string         `json:"status"`
	Self      liveSelf       `json:"self"`
	Positions []livePosition `json:"positions"`

	Finale  *liveFinale  `json:"finale,omitempty"`
	Outcome *liveOutcome `json:"outcome,omitempty"`

	// Läuft gerade eine Pause, steht hier, wofür und wie lange geplant.
	Pause *livePause `json:"pause,omitempty"`

	// Was diesem Team seit Kurzem widerfahren ist. Die App macht daraus
	// Benachrichtigungen; die Weboberfläche zeigt es als Meldung.
	Alerts []liveAlert `json:"alerts,omitempty"`

	// Nur für die Einsatzzentrale: das geheime Fluchtziel. Sie muss den
	// Spielstand beurteilen können, ohne mitzuspielen.
	Secret *liveSecret `json:"secret,omitempty"`
}

type liveSecret struct {
	FinalTargetNo   int    `json:"finalTargetNumber"`
	FinalTargetName string `json:"finalTargetName"`
	MissionsDone    int    `json:"missionsDone"`
	MissionsPlanned int    `json:"missionsPlanned"`
}

// handleLive liefert die Lage, wie sie die anfragende Rolle sehen darf.
//
// Hier entscheidet sich die Fairness des ganzen Spiels. Ein neugieriger
// Mitspieler mit offener Entwicklerkonsole darf im Netzwerkverkehr keine
// Wahrheit finden, die auf seinem Bildschirm verborgen ist – deshalb wird
// serverseitig zugeschnitten und nicht im Client ausgeblendet.
func handleLive(e *core.RequestEvent) error {
	res, err := buildLive(e.App, e.Auth, time.Now())
	if err != nil {
		return err
	}
	return e.JSON(http.StatusOK, res)
}

// buildLive stellt die Lage für ein Team zusammen.
//
// Als eigene Funktion, weil zwei Wege sie brauchen: die einmalige Abfrage und
// der offene Strom. Zwei Zusammenstellungen wären zwei Wahrheiten – und in
// einem Spiel, dessen Fairness genau hier hängt, wäre das der schlechteste
// Ort für eine Abweichung.
func buildLive(app core.App, me *core.Record, now time.Time) (*liveResponse, error) {
	gameID := me.GetString("game")
	if gameID == "" {
		return nil, apis.NewBadRequestError("Dieser Zugang gehört zu keinem Spiel.", nil)
	}

	gameRec, err := app.FindRecordById(schema.ColGames, gameID)
	if err != nil {
		return nil, apis.NewNotFoundError("Das Spiel wurde nicht gefunden.", nil)
	}

	cfg := game.ConfigOf(gameRec)
	myRole := me.GetString("role")
	fx := game.ActiveEffects(app, gameID, now)

	teams, err := app.FindRecordsByFilter(
		schema.ColTeams, "game = {:g} && active = true", "callsign", 0, 0,
		map[string]any{"g": gameID},
	)
	if err != nil {
		return nil, err
	}

	positions := make([]livePosition, 0, len(teams))

	for _, team := range teams {
		role := team.GetString("role")
		if role == schema.RoleHQ {
			continue // Die Zentrale sitzt zu Hause und hat keine Spielposition.
		}

		visible, blur := visibility(myRole, role, me.Id == team.Id, cfg, fx, now)
		if !visible {
			continue
		}

		pos := latestPosition(app, team.Id)
		if pos == nil {
			continue
		}

		captured := pos.GetDateTime("captured_at").Time()
		age := now.Sub(captured)
		if age > staleAfter {
			continue
		}

		point := geo.Point{Lat: pos.GetFloat("lat"), Lng: pos.GetFloat("lng")}
		if blur > 0 {
			point = game.Blur(point, team.Id, now, blur)
		}

		entry := livePosition{
			Team:       team.Id,
			Callsign:   team.GetString("callsign"),
			Display:    team.GetString("display"),
			Role:       role,
			Color:      team.GetString("color"),
			Lat:        point.Lat,
			Lng:        point.Lng,
			BlurM:      blur,
			CapturedAt: captured.UTC().Format(time.RFC3339),
			AgeSec:     int(age.Seconds()),
			InTransit:  pos.GetBool("in_transit"),
			Stale:      age > 15*time.Minute,
			Self:       team.Id == me.Id,
		}

		// Die Messgenauigkeit verrät bei einer unscharfen Position nichts
		// Nützliches, würde aber suggerieren, der Kreis sei genauer als er ist.
		if blur == 0 {
			entry.Accuracy = pos.GetFloat("accuracy")
		}

		positions = append(positions, entry)
	}

	self := liveSelf{
		FP:         me.GetInt("fp"),
		Points:     me.GetInt("points"),
		InTransit:  me.GetBool("in_transit"),
		Violations: me.GetInt("ping_violations"),
		Lockouts:   []liveLockout{},
	}

	if due := me.GetDateTime("ping_due_at").Time(); !due.IsZero() {
		self.NextDueAt = due.UTC().Format(time.RFC3339)
		self.DueInSec = int(due.Sub(now).Seconds())
		self.Overdue = self.DueInSec < 0
	}

	if id := me.GetString("start_hotspot"); id != "" {
		if h, err := app.FindRecordById(schema.ColHotspots, id); err == nil && h != nil {
			self.Start = &liveStart{
				Number: h.GetInt("number"),
				Name:   h.GetString("name"),
				Lat:    h.GetFloat("lat"),
				Lng:    h.GetFloat("lng"),
			}
		}
	}

	locks, err := game.ActivePenalties(app, me.Id)
	if err == nil {
		for _, l := range locks {
			expires := l.GetDateTime("expires_at").Time()
			self.Lockouts = append(self.Lockouts, liveLockout{
				Kind:      l.GetString("kind"),
				Reason:    l.GetString("reason"),
				ExpiresAt: expires.UTC().Format(time.RFC3339),
				LeftSec:   int(expires.Sub(now).Seconds()),
			})
		}
	}

	res := liveResponse{
		Now:       now.UTC().Format(time.RFC3339),
		Status:    gameRec.GetString("status"),
		Self:      self,
		Positions: positions,
	}

	// Die Pause gehört auf jedes Gerät, mit Grund und geplantem Ende: Wer
	// draußen steht und nur "angehalten" liest, kann eine Mittagspause nicht
	// von einer Störung unterscheiden.
	if res.Status == schema.GamePaused {
		res.Pause = &livePause{
			Reason: gameRec.GetString("pause_reason"),
			Since:  gameRec.GetDateTime("paused_at").Time().UTC().Format(time.RFC3339),
		}
		if until := gameRec.GetDateTime("pause_until").Time(); !until.IsZero() {
			res.Pause.Until = until.UTC().Format(time.RFC3339)
			res.Pause.LeftSec = int(until.Sub(now).Seconds())
		}
	}

	// Im Finale wird der Suchbereich offengelegt – das ist der Bruch mit dem
	// Versteckspiel und der Übergang zur reinen Verfolgung. Die Zielperson
	// bekommt ihn ebenfalls angezeigt: Sie soll wissen, wie eng es wird.
	if gameRec.GetBool("final_active") {
		if target, err := app.FindRecordById(schema.ColHotspots, gameRec.GetString("final_target")); err == nil {
			finale := &liveFinale{
				Active:  true,
				Lat:     target.GetFloat("lat"),
				Lng:     target.GetFloat("lng"),
				RadiusM: gameRec.GetFloat("final_radius_m"),
			}
			if sec, err := app.FindRecordById(schema.ColSectors, target.GetString("sector")); err == nil {
				finale.Sector = sec.GetString("code") + " " + sec.GetString("name")
			}
			res.Finale = finale
		}
	}

	if myRole == schema.RoleHQ {
		secret := &liveSecret{MissionsPlanned: gameRec.GetInt("planned_missions")}

		if target, err := app.FindRecordById(schema.ColHotspots, gameRec.GetString("final_target")); err == nil {
			secret.FinalTargetNo = target.GetInt("number")
			secret.FinalTargetName = target.GetString("name")
		}
		if done, err := app.FindRecordsByFilter(schema.ColMissions,
			"game = {:g} && status = 'done'", "", 0, 0,
			map[string]any{"g": gameRec.Id}); err == nil {
			secret.MissionsDone = len(done)
		}

		res.Secret = secret
	}

	if winner := gameRec.GetString("winner"); winner != "" {
		res.Outcome = &liveOutcome{
			Winner: winner,
			Reason: gameRec.GetString("win_reason"),
		}
	}

	res.Alerts = recentAlerts(app, gameID, me.Id, now)

	return &res, nil
}

// visibility beantwortet die Kernfrage des Spiels: Wer sieht wen, und wie genau?
//
//   - Die Einsatzzentrale sieht alles ungefiltert.
//   - Detektivteams sehen einander exakt, um Zangenbewegungen abzustimmen –
//     von Mister X sehen sie nichts, das müssen sie sich erarbeiten.
//   - Mister X sieht die Teams nur als unscharfe Kreise, sich selbst exakt.
func visibility(viewer, target string, isSelf bool, cfg game.Config, fx game.Effects, now time.Time) (visible bool, blurM float64) {
	if isSelf {
		return true, 0
	}

	switch viewer {
	case schema.RoleHQ:
		return true, 0

	case schema.RoleDetective:
		if target == schema.RoleDetective {
			return true, 0
		}
		// Ein laufender GPS-Ping legt die Zielperson offen – es sei denn, sie
		// hat eine Nebelkerze gezündet. Das ist der einzige Weg, auf dem die
		// Fahndung sie ohne Sichtkontakt zu sehen bekommt.
		if target == schema.RoleMisterX && fx.GPSPingActive(now) {
			return true, 0
		}
		return false, 0

	case schema.RoleMisterX:
		if target == schema.RoleDetective {
			// Nach einem Bereichsscan steigt die Unschärfe kurzzeitig: Die
			// Fahndung hat etwas erfahren und zahlt mit einer schlechteren
			// eigenen Ortung dafür.
			if fx.ScanBlurActive(now) {
				return true, cfg.RadarBlurScanM
			}
			return true, cfg.RadarBlurM
		}
		return false, 0

	default:
		return false, 0
	}
}

// alertTypes sind die Ereignisse, die ein Team erfahren soll, sobald sie
// eintreten – und nicht erst, wenn es das nächste Mal auf den Bildschirm sieht.
//
// Die Liste ist bewusst kurz. Jede Benachrichtigung, die man auch hätte
// weglassen können, kostet Aufmerksamkeit für die, auf die es ankommt.
var alertTypes = map[string]bool{
	game.EventSighting:      true, // Mister X: jemand meldet, euch zu sehen
	game.EventPingLockout:   true, // gesperrt
	game.EventMissionFailed: true, // Frist verstrichen
	game.EventArrestPartial: true, // jemand hat euch teilweise richtig verortet
	game.EventCamping:       true, // ihr steht zu lange still
	game.EventFinaleStart:   true,
}

// urgentAlerts läuten und rütteln; der Rest erscheint still.
var urgentAlerts = map[string]bool{
	game.EventSighting:      true,
	game.EventArrestPartial: true,
}

// recentAlerts liefert die letzten meldenswerten Ereignisse dieses Teams.
//
// Das Fenster ist kurz: Was älter als ein paar Minuten ist, hat der Client
// entweder schon gesehen oder es ist nicht mehr dringend. Welche davon schon
// gemeldet wurden, entscheidet der Client anhand der Kennung – der Server
// führt dafür kein Buch, denn ein Team kann mit zwei Geräten unterwegs sein.
func recentAlerts(app core.App, gameID, teamID string, now time.Time) []liveAlert {
	events, err := app.FindRecordsByFilter(
		schema.ColEvents,
		"game = {:g} && team = {:t} && occurred_at >= {:since}",
		"-occurred_at", 20, 0,
		map[string]any{
			"g": gameID, "t": teamID,
			"since": game.DBTime(now.Add(-10 * time.Minute)),
		},
	)
	if err != nil {
		return nil
	}

	out := make([]liveAlert, 0, len(events))
	for _, ev := range events {
		kind := ev.GetString("type")
		if !alertTypes[kind] {
			continue
		}
		out = append(out, liveAlert{
			ID:     ev.Id,
			Type:   kind,
			Reason: alertText(kind, ev.GetString("reason")),
			At:     ev.GetDateTime("occurred_at").Time().UTC().Format(time.RFC3339),
			Urgent: urgentAlerts[kind],
		})
	}
	return out
}

// alertText übersetzt ein Ereignis in einen Satz, der auf einen Sperrbildschirm
// passt. Die Begründung aus dem Protokoll ist für die Zentrale geschrieben und
// dafür oft zu umständlich.
func alertText(kind, reason string) string {
	switch kind {
	case game.EventSighting:
		return "Ein Fahndungsteam meldet, euch zu sehen. Bewegt euch."
	case game.EventPingLockout:
		return "Ihr seid gesperrt. Der Grund: " + reason
	case game.EventMissionFailed:
		return "Die Frist für das Zwischenziel ist abgelaufen."
	case game.EventArrestPartial:
		return "Ein Zugriff hat euren Aufenthalt richtig benannt."
	case game.EventCamping:
		return "Zu lange keine Bewegung und keine Handlung. Das kostet."
	case game.EventFinaleStart:
		return "Das Finale läuft. Der Suchbereich ist offen und wird enger."
	}
	return reason
}
