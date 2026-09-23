// Package game enthält die Spielregeln.
//
// Alles, was Zeit misst, Punkte vergibt oder bestraft, läuft hier – auf dem
// Server. Die Clients zeigen nur an. Anders ließe sich bei einem
// Verfolgungsspiel weder Mogeln ausschließen noch verhindern, dass ein Handy
// mit schlechtem Empfang seinen Träger benachteiligt.
package game

import (
	"encoding/json"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// Config sind die Regelwerte eines Spiels.
//
// Sie stehen in der Datenbank und nicht im Code, damit sich im
// Einrichtungsassistenten jeder Wert verstellen lässt – die Balance eines
// Verfolgungsspiels hängt an der Stadt, der Gruppengröße und der Spieldauer und
// wird sich nach dem ersten Durchlauf ändern.
type Config struct {
	PingIntervalMin        int `json:"ping_interval_min"`
	PingIntervalTransitMin int `json:"ping_interval_transit_min"`
	// So viele Verstöße bleiben folgenlos. Bei 1 ist der erste eine
	// Verwarnung, der zweite kostet Punkte, ab dem dritten kommt die Sperre.
	PingWarnAtViolation int `json:"ping_warn_at_violation"`
	PingPenaltyPoints   int `json:"ping_penalty_points"`
	PingLockoutMin      int `json:"ping_lockout_min"`
	PingLockoutPoints   int `json:"ping_lockout_points"`

	// Anti-Camping. "Camping" heißt hier: weder eine Spielhandlung noch eine
	// nennenswerte Bewegung. Beides zusammen, weil eines allein unfair wäre –
	// wer zwischen zwei Hotspots läuft, handelt nicht, campt aber auch nicht;
	// wer an einem Ort steht und Rätsel löst, bewegt sich nicht, campt aber
	// ebenso wenig.
	CampingDetectiveMin int     `json:"camping_detective_min"`
	CampingDetectiveFP  int     `json:"camping_detective_fp"`
	CampingMisterXMin   int     `json:"camping_misterx_min"`
	CampingMisterXFP    int     `json:"camping_misterx_fp"`
	CampingMisterXBonus int     `json:"camping_misterx_bonus"`
	CampingRadiusM      float64 `json:"camping_radius_m"`

	HotspotMaxDistanceM  float64 `json:"hotspot_max_distance_m"`
	SightingMaxDistanceM float64 `json:"sighting_max_distance_m"`
	SightingDwellSec     int     `json:"sighting_dwell_sec"`
	RadarBlurM           float64 `json:"radar_blur_m"`
	RadarBlurScanM       float64 `json:"radar_blur_scan_m"`

	IntelHotMin  int `json:"intel_hot_min"`
	IntelWarmMin int `json:"intel_warm_min"`

	StartFPMisterX   int `json:"start_fp_misterx"`
	StartFPDetective int `json:"start_fp_detective"`

	PointsMission       int `json:"points_mission"`
	PointsPuzzle        int `json:"points_puzzle"`
	PointsPuzzleWithFP  int `json:"points_puzzle_with_fp"`
	PointsFalseTrail    int `json:"points_false_trail"`
	PointsArrestLevel1  int `json:"points_arrest_level1"`
	PointsArrestLevel2  int `json:"points_arrest_level2"`
	PointsVictory       int `json:"points_victory"`
	PointsMissionFailed int `json:"points_mission_failed"`
	PointsArrestFailed  int `json:"points_arrest_failed"`

	CostReroute    int `json:"cost_reroute"`
	CostFalseTrail int `json:"cost_false_trail"`
	CostDelay      int `json:"cost_delay"`
	CostPhantom    int `json:"cost_phantom"`
	CostSmoke      int `json:"cost_smoke"`
	CostGhost      int `json:"cost_ghost"`
	CostGPSPing    int `json:"cost_gps_ping"`
	CostLockdown   int `json:"cost_lockdown"`
	CostBug        int `json:"cost_bug"`
	CostScan       int `json:"cost_scan"`
	CostUnlock     int `json:"cost_unlock"`

	// Kulanzzeit für Aufträge, die am Ziel selbst Zeit kosten. Siehe
	// internal/game/grace.go.
	MissionGraceMin    int `json:"mission_grace_min"`
	MissionGraceMaxMin int `json:"mission_grace_max_min"`

	MissionFailLockoutMin  int `json:"mission_fail_lockout_min"`
	ArrestFailLockoutMin   int `json:"arrest_fail_lockout_min"`
	FinalShrinkIntervalMin int `json:"final_shrink_interval_min"`
}

// Defaults sind die Werte aus dem Regelwerk v3.2.
func Defaults() Config {
	return Config{
		PingIntervalMin:        10,
		PingIntervalTransitMin: 13,
		PingWarnAtViolation:    1,
		PingPenaltyPoints:      -10,
		PingLockoutMin:         5,
		PingLockoutPoints:      -15,

		CampingDetectiveMin: 5,
		CampingDetectiveFP:  -1,
		CampingMisterXMin:   20,
		CampingMisterXFP:    -1,
		// Aus: Das Regelwerk kennt keine Prämie für die Fahndung, wenn die
		// Zielperson stillsteht – es nennt nur ihren eigenen Verlust. Die
		// Möglichkeit bleibt im Regelpult stehen, weil sie sich als Hausregel
		// anbietet; eingeschaltet ist sie nicht.
		CampingMisterXBonus: 0,
		CampingRadiusM:      75,

		HotspotMaxDistanceM:  150,
		SightingMaxDistanceM: 50,
		SightingDwellSec:     20,
		RadarBlurM:           200,
		RadarBlurScanM:       300,

		IntelHotMin:  3,
		IntelWarmMin: 15,

		StartFPMisterX:   5,
		StartFPDetective: 4,

		PointsMission:       15,
		PointsPuzzle:        15,
		PointsPuzzleWithFP:  8,
		PointsFalseTrail:    20,
		PointsArrestLevel1:  5,
		PointsArrestLevel2:  10,
		PointsVictory:       150,
		PointsMissionFailed: -20,
		PointsArrestFailed:  -15,

		CostReroute:    2,
		CostFalseTrail: 1,
		CostDelay:      2,
		CostPhantom:    1,
		CostSmoke:      2,
		CostGhost:      3,
		CostGPSPing:    4,
		CostLockdown:   3,
		CostBug:        2,
		CostScan:       1,
		CostUnlock:     1,

		MissionGraceMin:        10,
		MissionGraceMaxMin:     30,
		MissionFailLockoutMin:  10,
		ArrestFailLockoutMin:   10,
		FinalShrinkIntervalMin: 5,
	}
}

// ConfigOf liest die Regelwerte eines Spiels.
//
// Fehlende oder unlesbare Werte fallen auf das Regelwerk zurück: Ein Spiel mit
// halb gefüllter Konfiguration soll laufen, nicht stehenbleiben.
func ConfigOf(game *core.Record) Config {
	cfg := Defaults()

	raw := game.Get("config")
	if raw == nil {
		return cfg
	}

	var data []byte
	switch v := raw.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	case json.RawMessage:
		data = v
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return cfg
		}
		data = encoded
	}

	// Unmarshal überschreibt nur die Felder, die auch dastehen.
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Defaults()
	}

	return cfg
}

// PingInterval liefert die Frist für ein Team, je nach Transitstatus.
func (c Config) PingInterval(inTransit bool) time.Duration {
	minutes := c.PingIntervalMin
	if inTransit {
		minutes = c.PingIntervalTransitMin
	}
	if minutes <= 0 {
		minutes = 10
	}
	return time.Duration(minutes) * time.Minute
}
