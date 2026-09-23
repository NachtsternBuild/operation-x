package schema_test

import (
	"testing"

	"github.com/elias/operation-x/pkg/testhilfe"
)

// Zwei Voreinstellungen von PocketBase, die für dieses Spiel nicht taugen —
// und die deshalb bei jedem Start gesetzt werden. Beide sind leicht zu
// verlieren: Es genügt, dass jemand sie in der Datenbankverwaltung
// zurückstellt oder dass eine neue Fassung von PocketBase sie anders
// vorbelegt.

func TestKeineIPAdressenImProtokoll(t *testing.T) {
	app := testhilfe.App(t)

	if app.Settings().Logs.LogIP {
		t.Error("Der Server schreibt IP-Adressen mit. Für dieses Spiel werden " +
			"sie nirgends gebraucht — und sie sind personenbezogen.")
	}
	if app.Settings().Logs.LogAuthId {
		t.Error("Der Server schreibt mit, welcher Zugang welche Anfrage stellte")
	}
}

func TestGrenzeGegenDurchprobieren(t *testing.T) {
	app := testhilfe.App(t)
	limits := app.Settings().RateLimits

	if !limits.Enabled {
		t.Fatal("Die Ratenbegrenzung ist aus. Bei 81.000 sprechbaren " +
			"Kennwörtern und sechs Stunden Spielzeit ist das zu wenig.")
	}

	regel, ok := limits.FindRateLimitRule([]string{"*:auth"})
	if !ok {
		t.Fatal("Für die Anmeldung gibt es keine eigene Regel")
	}
	if regel.MaxRequests > 30 {
		t.Errorf("%d Anmeldeversuche je %d Sekunden sind zu viele",
			regel.MaxRequests, regel.Duration)
	}
	if regel.MaxRequests < 10 {
		t.Errorf("%d Anmeldeversuche je %d Sekunden sind zu wenige — eine Gruppe "+
			"meldet sich gleichzeitig über denselben Anschluss an",
			regel.MaxRequests, regel.Duration)
	}

	// Die Kacheln brauchen eine eigene, weite Regel: Wer die Karte
	// verschiebt, holt dreißig Bilder auf einmal.
	kacheln, ok := limits.FindRateLimitRule([]string{"/api/opx/tiles/14/8800/5400"})
	if !ok {
		t.Fatal("Für die Kacheln gibt es keine Regel")
	}
	if kacheln.MaxRequests < 1000 {
		t.Errorf("Nur %d Kacheln je %d Sekunden — damit hakt die Karte, sobald "+
			"mehrere Geräte hinter einem Anschluss sitzen",
			kacheln.MaxRequests, kacheln.Duration)
	}
}

// Alle Sammlungen sind von außen gesperrt: Daten gibt es ausschließlich über
// die Endpunkte unter /api/opx, und die filtern nach Rolle und Spiel. Diese
// Prüfung schlägt an, sobald irgendwo eine Regel aufgeht.
func TestKeineSammlungIstVonAussenOffen(t *testing.T) {
	app := testhilfe.App(t)

	collections, err := app.FindAllCollections()
	if err != nil {
		t.Fatal(err)
	}

	for _, col := range collections {
		if col.System {
			continue
		}
		for name, regel := range map[string]*string{
			"List":   col.ListRule,
			"View":   col.ViewRule,
			"Create": col.CreateRule,
			"Update": col.UpdateRule,
			"Delete": col.DeleteRule,
		} {
			if regel != nil {
				t.Errorf("%s: Die %s-Regel ist offen (%q) — damit kommt jeder "+
					"angemeldete Client über /api/collections an die Daten",
					col.Name, name, *regel)
			}
		}
	}
}
