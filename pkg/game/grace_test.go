package game

import "testing"

// Die Obergrenze ist der Kern der Regel: Ohne sie wäre "am Ziel stehen" ein
// Freibrief, die Uhr anzuhalten.
func TestGraceGrenzeIstBindend(t *testing.T) {
	cfg := Defaults()

	if cfg.MissionGraceMin <= 0 {
		t.Fatal("ohne Schrittweite gibt es keine Kulanz")
	}
	if cfg.MissionGraceMaxMin < cfg.MissionGraceMin {
		t.Fatalf("Obergrenze %d liegt unter einem einzelnen Schritt %d",
			cfg.MissionGraceMaxMin, cfg.MissionGraceMin)
	}

	// So viele Schritte passen hinein, und keiner mehr.
	schritte := cfg.MissionGraceMaxMin / cfg.MissionGraceMin
	if schritte < 2 {
		t.Errorf("nur %d Schritte – zu wenig, um eine Warteschlange abzudecken", schritte)
	}
}
