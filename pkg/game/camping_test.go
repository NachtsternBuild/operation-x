package game

import (
	"testing"
	"time"

	"github.com/elias/operation-x/pkg/schema"
)

// Die Fensterwahl ist der Kern der Regel: Fahndung kurz, Zielperson lang.
func TestCampingWindow(t *testing.T) {
	cfg := Defaults()
	if got := campingWindow(cfg, "detective"); got != 5*time.Minute {
		t.Errorf("Fahndung: %v, erwartet 5m", got)
	}
	if got := campingWindow(cfg, "misterx"); got != 20*time.Minute {
		t.Errorf("Zielperson: %v, erwartet 20m", got)
	}
	if got := campingWindow(cfg, "hq"); got != 0 {
		t.Errorf("Zentrale darf nicht campen können, bekam %v", got)
	}
}

// Das Regelwerk sagt: "Mister X im Stillstand außerhalb von Mission und
// Transit."
//
// Die Ausnahme fehlte, und das drehte die Regel um: Die Zielperson arbeitet
// fast durchgehend an einem Zwischenziel, und genau dabei steht sie
// zwangsläufig herum – an der Kasse, am Bahnsteig, vor dem Hotspot. Bestraft
// wurde damit der Normalfall, während die Missionsfrist dasselbe schon misst.
func TestZielpersonIstWaehrendMissionUndTransitAusgenommen(t *testing.T) {
	cfg := Defaults()

	// Der Geduldsfaden gilt weiterhin für beide Seiten.
	if campingWindow(cfg, schema.RoleDetective) <= 0 {
		t.Error("für die Fahndung gilt kein Zeitfenster mehr")
	}
	if campingWindow(cfg, schema.RoleMisterX) <= 0 {
		t.Error("für die Zielperson gilt kein Zeitfenster mehr")
	}

	// Die Fahndung hat keine solche Ausnahme: Ihr Zeitfenster ist kürzer, und
	// sie hat weder Mission noch Transitfrist, die dasselbe messen würde.
	if campingWindow(cfg, schema.RoleDetective) >= campingWindow(cfg, schema.RoleMisterX) {
		t.Error("die Zielperson darf länger stehen als die Fahndung – das ist die Absicht")
	}
}

// Die Prämie für die Fahndung steht nicht im Regelwerk und ist deshalb aus.
func TestPraemieIstAbWerkAus(t *testing.T) {
	if Defaults().CampingMisterXBonus != 0 {
		t.Errorf("Prämie ab Werk auf %d – das Regelwerk kennt sie nicht",
			Defaults().CampingMisterXBonus)
	}
}
