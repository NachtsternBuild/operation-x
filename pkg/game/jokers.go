package game

import (
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/schema"
)

// Die Joker beider Seiten. Die Kennungen entsprechen den Werten im Datenmodell.
const (
	// Mister X
	JokerFalseTrail = "false_trail" // Falsche Fährte
	JokerDelay      = "delay"       // Zeitverzögerung
	JokerPhantom    = "phantom"     // Phantom
	JokerSmoke      = "smoke"       // Nebelkerze
	JokerGhost      = "ghost"       // U-Bahn-Geist
	JokerReroute    = "reroute"     // Ausklinken

	// Detektive
	JokerGPSPing  = "gps_ping" // Exakter GPS-Ping
	JokerLockdown = "lockdown" // Sperrzone
	JokerBug      = "bug"      // Wanzen-Falle
	JokerScan     = "scan"     // Bereichs-Scan
	JokerUnlock   = "unlock"   // Hinweis-Freischaltung
)

// JokerSpec beschreibt einen Joker.
type JokerSpec struct {
	Kind     string
	Name     string
	Role     string
	Cost     func(Config) int
	Duration time.Duration
	// MaxUses begrenzt, wie oft ein Joker im Spiel eingesetzt werden darf.
	// Null heißt: so oft, wie die Fluchtpunkte reichen.
	MaxUses     int
	Description string
	// BlockedByFinale: Täuschungsmanöver sind im Finale gesperrt. Ab dort gibt
	// es kein Versteckspiel mehr, nur noch Verfolgung.
	BlockedByFinale bool
	// Manual: Dieser Joker wird nicht im Kartendeck ausgelöst, sondern dort, wo
	// er hingehört – siehe Ausklinken. Er steht trotzdem im Katalog, denn wer
	// seine Fluchtpunkte zählt, muss alles sehen, was sie kosten kann.
	Manual bool
	// Where sagt, wo er stattdessen liegt.
	Where string
}

// JokerCatalog ist die vollständige Liste nach Regelwerk v3.2.
func JokerCatalog() []JokerSpec {
	return []JokerSpec{
		{
			// Ausklinken gehört ins Missionsbuch und nicht hierher: Es wirft
			// ein bestimmtes Ziel weg, und welches das ist, steht dort und
			// nirgends sonst. Ein zweiter Knopf im Deck – ohne das Ziel
			// danebenzuschreiben – wäre die teuerste Fehlbedienung des Spiels.
			Kind: JokerReroute, Name: "Ausklinken", Role: schema.RoleMisterX,
			Cost:        func(c Config) int { return c.CostReroute },
			Manual:      true,
			Where:       "Im Missionsbuch: „Ausklinken und neue Route“.",
			Description: "Das laufende Zwischenziel abwählen und ein neues anfordern.",
		},
		{
			Kind: JokerFalseTrail, Name: "Falsche Fährte", Role: schema.RoleMisterX,
			Cost: func(c Config) int { return c.CostFalseTrail }, MaxUses: 2,
			Description:     "Speist einen gefälschten Sektorhinweis in die Fahndung ein. Bringt zusätzlich Punkte.",
			BlockedByFinale: true,
		},
		{
			Kind: JokerPhantom, Name: "Phantom", Role: schema.RoleMisterX,
			Cost: func(c Config) int { return c.CostPhantom }, MaxUses: 1,
			Description:     "Erzeugt eine gefälschte Verhaltensbeobachtung, etwa eine falsche Himmelsrichtung.",
			BlockedByFinale: true,
		},
		{
			Kind: JokerDelay, Name: "Zeitverzögerung", Role: schema.RoleMisterX,
			Cost: func(c Config) int { return c.CostDelay }, Duration: 5 * time.Minute,
			Description: "Hält alle neu freigeschalteten Hinweise fünf Minuten zurück.",
		},
		{
			Kind: JokerSmoke, Name: "Nebelkerze", Role: schema.RoleMisterX,
			Cost: func(c Config) int { return c.CostSmoke }, Duration: 15 * time.Minute,
			Description:     "Stört jede Ortung: GPS-Ping, Bereichsscan, Sperrzone und Wanze laufen ins Leere.",
			BlockedByFinale: true,
		},
		{
			Kind: JokerGhost, Name: "U-Bahn-Geist", Role: schema.RoleMisterX,
			Cost: func(c Config) int { return c.CostGhost }, Duration: 20 * time.Minute,
			Description: "Setzt die Meldepflicht für eine lange Fahrt aus.",
		},

		{
			Kind: JokerGPSPing, Name: "Exakter GPS-Ping", Role: schema.RoleDetective,
			Cost: func(c Config) int { return c.CostGPSPing }, Duration: 60 * time.Second,
			Description: "Deckt den genauen Standort der Zielperson eine Minute lang auf.",
		},
		{
			Kind: JokerLockdown, Name: "Sperrzone", Role: schema.RoleDetective,
			Cost: func(c Config) int { return c.CostLockdown }, Duration: 15 * time.Minute,
			Description: "Markiert einen Sektor. Betritt die Zielperson ihn, schlägt das System Alarm.",
		},
		{
			Kind: JokerBug, Name: "Wanzen-Falle", Role: schema.RoleDetective,
			Cost: func(c Config) int { return c.CostBug }, Duration: 60 * time.Minute,
			Description: "Legt eine digitale Wanze an einem Hotspot aus.",
		},
		{
			Kind: JokerScan, Name: "Bereichs-Scan", Role: schema.RoleDetective,
			Cost: func(c Config) int { return c.CostScan },
			Description: "Fragt ab, ob sich die Zielperson in einem Sektor aufhält. " +
				"Erhöht dafür kurzzeitig ihre Radar-Unschärfe.",
		},
		{
			Kind: JokerUnlock, Name: "Hinweis-Freischaltung", Role: schema.RoleDetective,
			Cost:        func(c Config) int { return c.CostUnlock },
			Description: "Schaltet einen Hinweis sofort frei, ohne das Rätsel zu lösen.",
		},
	}
}

// FindJoker sucht einen Joker im Katalog.
func FindJoker(kind string) (JokerSpec, bool) {
	for _, j := range JokerCatalog() {
		if j.Kind == kind {
			return j, true
		}
	}
	return JokerSpec{}, false
}

// Effects fasst zusammen, welche Joker gerade wirken.
//
// Wird bei jeder Lageabfrage gebraucht: Ob die Zielperson sichtbar ist, ob eine
// Ortung durchkommt und ob Hinweise sofort zugestellt werden, hängt daran.
type Effects struct {
	SmokeUntil    time.Time // Nebelkerze: jede Ortung läuft ins Leere
	DelayUntil    time.Time // Zeitverzögerung: Hinweise werden zurückgehalten
	GhostUntil    time.Time // U-Bahn-Geist: Meldepflicht ausgesetzt
	GPSPingUntil  time.Time // Exakter GPS-Ping: Zielperson offen sichtbar
	ScanBlurUntil time.Time // Bereichsscan: erhöhte Unschärfe für die Zielperson
}

func (e Effects) SmokeActive(now time.Time) bool    { return now.Before(e.SmokeUntil) }
func (e Effects) DelayActive(now time.Time) bool    { return now.Before(e.DelayUntil) }
func (e Effects) GhostActive(now time.Time) bool    { return now.Before(e.GhostUntil) }
func (e Effects) ScanBlurActive(now time.Time) bool { return now.Before(e.ScanBlurUntil) }

// GPSPingActive ist der einzige Effekt, den die Nebelkerze aushebelt: Sie
// stört die Ortung, also nützt ein laufender Ping nichts mehr.
func (e Effects) GPSPingActive(now time.Time) bool {
	return now.Before(e.GPSPingUntil) && !e.SmokeActive(now)
}

// ActiveEffects liest die laufenden Jokerwirkungen eines Spiels.
func ActiveEffects(app core.App, gameID string, now time.Time) Effects {
	var fx Effects

	records, err := app.FindRecordsByFilter(
		schema.ColJokers,
		"game = {:g} && active = true && expires_at > {:now}",
		"", 0, 0,
		map[string]any{"g": gameID, "now": DBTime(now)},
	)
	if err != nil {
		return fx
	}

	for _, r := range records {
		until := r.GetDateTime("expires_at").Time()

		switch r.GetString("kind") {
		case JokerSmoke:
			if until.After(fx.SmokeUntil) {
				fx.SmokeUntil = until
			}
		case JokerDelay:
			if until.After(fx.DelayUntil) {
				fx.DelayUntil = until
			}
		case JokerGhost:
			if until.After(fx.GhostUntil) {
				fx.GhostUntil = until
			}
		case JokerGPSPing:
			if until.After(fx.GPSPingUntil) {
				fx.GPSPingUntil = until
			}
		case JokerScan:
			if until.After(fx.ScanBlurUntil) {
				fx.ScanBlurUntil = until
			}
		}
	}

	return fx
}

// JokerUsage zählt, wie oft ein Team einen Joker bereits eingesetzt hat.
func JokerUsage(app core.App, teamID, kind string) int {
	records, err := app.FindRecordsByFilter(
		schema.ColJokers,
		"team = {:t} && kind = {:k}",
		"", 0, 0,
		map[string]any{"t": teamID, "k": kind},
	)
	if err != nil {
		return 0
	}
	return len(records)
}
