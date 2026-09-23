package game

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Die Regelseite darf nicht altern.
//
// docs/regelwerk.html nennt Zahlen im Fließtext – "alle 10 Minuten", "15
// Punkte Abzug". Solche Zahlen sind die erste Stelle, an der eine
// Dokumentation still falsch wird: Wer eine Voreinstellung ändert, denkt nicht
// an eine HTML-Datei zwei Verzeichnisse weiter.
//
// Deshalb trägt jede Zahl dort den Schlüssel ihres Regelwerts, und dieser Test
// vergleicht sie mit der Voreinstellung. Ein geänderter Wert bricht ihn, und
// wer ihn repariert, hat die Seite mitgezogen.
func TestRegelseiteNenntDieEchtenWerte(t *testing.T) {
	roh, err := os.ReadFile("../../docs/regelwerk.html")
	if err != nil {
		t.Fatalf("Regelseite nicht lesbar: %v", err)
	}
	seite := string(roh)

	werte := map[string]float64{}
	kodiert, err := json.Marshal(Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(kodiert, &werte); err != nil {
		t.Fatal(err)
	}

	muster := regexp.MustCompile(`data-regel="(\w+)"[^>]*>([^<]+)<`)
	treffer := muster.FindAllStringSubmatch(seite, -1)
	if len(treffer) < 40 {
		t.Fatalf("nur %d verankerte Zahlen gefunden – ist die Seite noch vollständig?", len(treffer))
	}

	gesehen := map[string]bool{}
	for _, m := range treffer {
		key, text := m[1], strings.TrimSpace(m[2])
		gesehen[key] = true

		soll, bekannt := werte[key]
		if !bekannt {
			t.Errorf("%s: die Seite nennt einen Regelwert, den es nicht gibt", key)
			continue
		}

		ist, err := strconv.ParseFloat(strings.ReplaceAll(text, ",", "."), 64)
		if err != nil {
			t.Errorf("%s: %q ist keine Zahl", key, text)
			continue
		}

		// Im Fließtext steht "15 Punkte Abzug", in der Konfiguration -15. Beide
		// Schreibweisen gelten; das Vorzeichen trägt dort der Satz.
		if ist != soll && ist != -soll {
			t.Errorf("%s: Seite sagt %v, Voreinstellung ist %v", key, ist, soll)
		}
	}

	// Die Übersicht am Ende der Seite soll vollständig sein.
	var fehlend []string
	for key := range werte {
		if !gesehen[key] {
			fehlend = append(fehlend, key)
		}
	}
	if len(fehlend) > 0 {
		t.Errorf("Diese Regelwerte kommen auf der Seite nicht vor: %s",
			strings.Join(fehlend, ", "))
	}

	// Und jeder Wert des Verzeichnisses muss eine Voreinstellung haben, sonst
	// stünde im Regelpult ein Feld ohne Inhalt.
	for _, r := range RuleIndex {
		if _, ok := werte[r.Key]; !ok {
			t.Errorf("Regelverzeichnis kennt %q, die Voreinstellung nicht", r.Key)
		}
	}
	fmt.Fprintf(os.Stderr, "Regelseite: %d Zahlen geprüft, %d Regelwerte abgedeckt\n",
		len(treffer), len(gesehen))
}
