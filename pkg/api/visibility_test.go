package api

import (
	"testing"
	"time"

	"github.com/elias/operation-x/pkg/game"
	"github.com/elias/operation-x/pkg/schema"
)

// Wer sieht wen, und wie genau?
//
// Diese Funktion entscheidet die Fairness des ganzen Spiels, und sie war bis
// zum Prüfdurchgang die größte ungetestete Stelle im Bestand. Ein Fehler hier
// fällt im Betrieb nicht auf – die Karte zeigt einfach etwas an, und niemand
// merkt, dass es zu viel war.
func TestSichtbarkeitJeRolle(t *testing.T) {
	cfg := game.Defaults()
	now := time.Now()
	keine := game.Effects{}

	faelle := []struct {
		name        string
		viewer      string
		target      string
		self        bool
		fx          game.Effects
		wantVisible bool
		wantBlur    float64
	}{
		{"sich selbst sieht jeder genau", schema.RoleDetective, schema.RoleDetective, true, keine, true, 0},
		{"die Zentrale sieht alles genau", schema.RoleHQ, schema.RoleMisterX, false, keine, true, 0},
		{"Fahndungsteams sehen einander genau", schema.RoleDetective, schema.RoleDetective, false, keine, true, 0},

		// Der Kern: Ohne laufende Ortung sieht die Fahndung die Zielperson NICHT.
		{"Fahndung sieht die Zielperson nicht", schema.RoleDetective, schema.RoleMisterX, false, keine, false, 0},

		// Die Zielperson sieht die Fahndung immer, aber unscharf.
		{"Zielperson sieht die Fahndung unscharf", schema.RoleMisterX, schema.RoleDetective, false, keine, true, cfg.RadarBlurM},
		{"Zielperson sieht andere Zielpersonen nicht", schema.RoleMisterX, schema.RoleMisterX, false, keine, false, 0},

		{"unbekannte Rolle sieht nichts", "irgendwas", schema.RoleDetective, false, keine, false, 0},
	}

	for _, f := range faelle {
		visible, blur := visibility(f.viewer, f.target, f.self, cfg, f.fx, now)

		if visible != f.wantVisible {
			t.Errorf("%s: sichtbar=%v, erwartet %v", f.name, visible, f.wantVisible)
		}
		if blur != f.wantBlur {
			t.Errorf("%s: Unschärfe=%v, erwartet %v", f.name, blur, f.wantBlur)
		}
	}
}

// Ein laufender GPS-Ping ist der einzige Weg, auf dem die Fahndung die
// Zielperson ohne Sichtkontakt zu sehen bekommt – und die Nebelkerze ist die
// Antwort darauf. Beides zusammen ist die taktische Ebene des Spiels; kippt
// eine der beiden Richtungen, kippt das Spiel.
func TestOrtungUndNebelkerze(t *testing.T) {
	cfg := game.Defaults()
	now := time.Now()

	ortung := game.Effects{GPSPingUntil: now.Add(time.Minute)}
	visible, blur := visibility(schema.RoleDetective, schema.RoleMisterX, false, cfg, ortung, now)
	if !visible || blur != 0 {
		t.Errorf("mit laufender Ortung: sichtbar=%v Unschärfe=%v – erwartet genau sichtbar", visible, blur)
	}

	// Nebelkerze: dieselbe Ortung, aber sie läuft ins Leere.
	vernebelt := game.Effects{
		GPSPingUntil: now.Add(time.Minute),
		SmokeUntil:   now.Add(15 * time.Minute),
	}
	visible, _ = visibility(schema.RoleDetective, schema.RoleMisterX, false, cfg, vernebelt, now)
	if visible {
		t.Error("die Nebelkerze hat die laufende Ortung nicht neutralisiert")
	}

	// Abgelaufene Ortung wirkt nicht mehr.
	alt := game.Effects{GPSPingUntil: now.Add(-time.Second)}
	visible, _ = visibility(schema.RoleDetective, schema.RoleMisterX, false, cfg, alt, now)
	if visible {
		t.Error("eine abgelaufene Ortung deckt die Zielperson weiterhin auf")
	}
}

// Nach einem Bereichsscan steigt die Unschärfe: Die Fahndung hat etwas
// erfahren und zahlt mit einer schlechteren eigenen Ortung dafür.
func TestScanErhoehtDieUnschaerfe(t *testing.T) {
	cfg := game.Defaults()
	now := time.Now()

	nachScan := game.Effects{ScanBlurUntil: now.Add(5 * time.Minute)}
	_, blur := visibility(schema.RoleMisterX, schema.RoleDetective, false, cfg, nachScan, now)

	if blur != cfg.RadarBlurScanM {
		t.Errorf("Unschärfe nach Scan: %v, erwartet %v", blur, cfg.RadarBlurScanM)
	}
	if cfg.RadarBlurScanM <= cfg.RadarBlurM {
		t.Errorf("die Scan-Unschärfe (%v) müsste über der normalen (%v) liegen",
			cfg.RadarBlurScanM, cfg.RadarBlurM)
	}
}
