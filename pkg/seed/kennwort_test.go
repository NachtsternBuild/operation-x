package seed

import (
	"regexp"
	"strings"
	"testing"
)

// Kennwörter, die am Spieltag über Funk durchgegeben werden.
//
// Sie müssen zwei Dinge gleichzeitig sein: sprechbar und nicht zu erraten.
// Das erste prüft man beim Vorlesen, das zweite hier.

var form = regexp.MustCompile(`^[a-z]+-[a-z]+-\d{2}$`)

func TestKennwortHatDieAbgesprocheneForm(t *testing.T) {
	for i := 0; i < 200; i++ {
		k, err := SpeakablePassword()
		if err != nil {
			t.Fatal(err)
		}
		if !form.MatchString(k) {
			t.Fatalf("Kennwort %q passt nicht auf wort-wort-zahl", k)
		}
		// Umlaute kosten auf mancher Tastatur einen Umweg — deshalb keine.
		if strings.ContainsAny(k, "äöüß") {
			t.Fatalf("Kennwort %q enthält einen Umlaut", k)
		}
		// Kurz genug, um es einmal vorzulesen.
		if len(k) > 24 {
			t.Fatalf("Kennwort %q ist zu lang zum Durchsagen", k)
		}
	}
}

// Zwei Teams mit demselben Kennwort wären am Spieltag nicht lustig, sondern
// eine Stunde Fehlersuche. Bei rund 81.000 Möglichkeiten kommen Dopplungen
// vor — aber nicht in dieser Größenordnung.
func TestKennwoerterWiederholenSichNichtStaendig(t *testing.T) {
	gesehen := map[string]int{}
	const zuege = 500

	for i := 0; i < zuege; i++ {
		k, err := SpeakablePassword()
		if err != nil {
			t.Fatal(err)
		}
		gesehen[k]++
	}

	if len(gesehen) < zuege*9/10 {
		t.Errorf("Von %d Kennwörtern waren nur %d verschieden — der Zufall "+
			"stimmt nicht", zuege, len(gesehen))
	}

	// Und der gröbste denkbare Fehler: immer dasselbe Wort an erster Stelle.
	ersteWoerter := map[string]bool{}
	for k := range gesehen {
		ersteWoerter[strings.SplitN(k, "-", 2)[0]] = true
	}
	if len(ersteWoerter) < 10 {
		t.Errorf("Nur %d verschiedene erste Wörter — die Auswahl greift nicht",
			len(ersteWoerter))
	}
}

// Die Wortliste selbst: keine Dopplungen, sonst schrumpft der Vorrat
// unbemerkt.
func TestWortlisteIstFreiVonDopplungen(t *testing.T) {
	gesehen := map[string]bool{}
	for _, w := range woerter {
		if gesehen[w] {
			t.Errorf("Das Wort %q steht zweimal in der Liste", w)
		}
		gesehen[w] = true
	}
	if len(woerter) < 25 {
		t.Errorf("Nur %d Wörter — das sind zu wenige Möglichkeiten", len(woerter))
	}
}
