package game

import "testing"

// Jede Jokerkennung muss im Katalog stehen.
//
// Ausklinken gab es von Anfang an: eine Kennung, ein Preis in der
// Konfiguration, ein Endpunkt, der ihn abbucht. Nur im Katalog fehlte er –
// und damit in beiden Jokerdecks. Wer seine Fluchtpunkte zählte, sah zehn
// Karten und wusste nicht, dass die elfte ihn zwei kostet.
func TestJedeKennungStehtImKatalog(t *testing.T) {
	kennungen := []string{
		JokerReroute, JokerFalseTrail, JokerDelay, JokerPhantom, JokerSmoke, JokerGhost,
		JokerGPSPing, JokerLockdown, JokerBug, JokerScan, JokerUnlock,
	}

	for _, k := range kennungen {
		if _, ok := FindJoker(k); !ok {
			t.Errorf("Joker %q fehlt im Katalog", k)
		}
	}

	if len(JokerCatalog()) != len(kennungen) {
		t.Errorf("Katalog hat %d Einträge, Kennungen gibt es %d",
			len(JokerCatalog()), len(kennungen))
	}
}

// Ein Joker ohne Preis wäre umsonst, einer ohne Beschreibung unbenutzbar.
func TestJederJokerHatPreisUndBeschreibung(t *testing.T) {
	cfg := Defaults()

	for _, spec := range JokerCatalog() {
		if spec.Cost == nil {
			t.Errorf("%s hat keine Preisfunktion", spec.Name)
			continue
		}
		if spec.Cost(cfg) <= 0 {
			t.Errorf("%s kostet %d FP", spec.Name, spec.Cost(cfg))
		}
		if spec.Description == "" {
			t.Errorf("%s hat keine Beschreibung", spec.Name)
		}
		// Wer nicht im Deck ausgelöst wird, muss sagen, wo sonst.
		if spec.Manual && spec.Where == "" {
			t.Errorf("%s wird anderswo ausgelöst, sagt aber nicht wo", spec.Name)
		}
	}
}
