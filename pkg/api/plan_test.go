package api

import (
	"strings"
	"testing"
)

// Eine echte Antwort steht selten allein: davor eine Anrede, drumherum drei
// Backticks, danach ein "Viel Spaß!". Das muss durchgehen.
const antwortMitDrumherum = "Klar, hier ist der Plan!\n\n```json\n" + `{
  "city": "Dresden",
  "sectors": [
    {"name": "Altstadt", "geometry": {"type":"Polygon","coordinates":[[[13.73,51.04],[13.75,51.04],[13.75,51.06],[13.73,51.06],[13.73,51.04]]]}}
  ],
  "hotspots": [
    {"name": "Goldener Reiter", "lat": 51.05752, "lng": 13.74198, "kind": "landmark", "task": "Schreibe die Jahreszahl ab."},
    {"name": "Zwinger", "lat": 51.05306, "lng": 13.73361, "kind": "LANDMARK"}
  ],
  "puzzles": [
    {"type":"c","title":"Zahlenschloss","question":"Rechne.","answer":"24","hint":"Quersumme.","category":"ROT","points":15}
  ]
}` + "\n```\n\nViel Spaß!"

func TestParsePlanHoltDenBlockHeraus(t *testing.T) {
	p, err := parsePlan(antwortMitDrumherum)
	if err != nil {
		t.Fatalf("parsePlan: %v", err)
	}

	if p.City != "Dresden" {
		t.Errorf("Stadt = %q, erwartet Dresden", p.City)
	}
	if len(p.Sectors) != 1 || len(p.Hotspots) != 2 || len(p.Puzzles) != 1 {
		t.Fatalf("Sektoren/Hotspots/Rätsel = %d/%d/%d, erwartet 1/2/1",
			len(p.Sectors), len(p.Hotspots), len(p.Puzzles))
	}

	// Die Aufgabe für die Zielperson landet im Notizfeld, aus dem sie das
	// Missionsbuch liest.
	if p.Hotspots[0].Notes != "Schreibe die Jahreszahl ab." {
		t.Errorf("Aufgabe = %q", p.Hotspots[0].Notes)
	}
	// Ein Hotspot ohne Aufgabe ist erlaubt – ein Sektor ohne Fläche nicht.
	if p.Hotspots[1].Kind != "landmark" {
		t.Errorf("Art = %q, erwartet landmark (kleingeschrieben)", p.Hotspots[1].Kind)
	}
	// Unbekannte Hinweisqualität wird nicht abgelehnt, sondern auf die mittlere
	// gesetzt: Die Spielleitung ändert sie in zwei Klicks.
	if p.Puzzles[0].Category != "yellow" || p.Puzzles[0].Type != "C" {
		t.Errorf("Rätsel = %+v", p.Puzzles[0])
	}
	// Zwei Hotspots sind zu wenig für ein Spiel, und das gehört gesagt.
	if len(p.Warnings) == 0 {
		t.Error("keine Warnung bei zwei Hotspots")
	}
}

func TestParsePlanWirftKaputteGrenzenWeg(t *testing.T) {
	// Eine Linie statt einer Fläche – die häufigste Art, wie eine Maschine ein
	// Polygon verfehlt. Würde sie durchgehen, hätte die Zentrale einen Sektor
	// auf der Liste, in dem sich nie jemand aufhält.
	p, err := parsePlan(`{
      "sectors": [
        {"name": "Strich", "geometry": {"type":"Polygon","coordinates":[[[13.73,51.04],[13.74,51.04],[13.73,51.04]]]}},
        {"name": "Echt", "geometry": {"type":"Polygon","coordinates":[[[13.73,51.04],[13.75,51.04],[13.75,51.06],[13.73,51.06],[13.73,51.04]]]}}
      ],
      "hotspots": [{"name":"Zwinger","lat":51.05306,"lng":13.73361}]
    }`)
	if err != nil {
		t.Fatalf("parsePlan: %v", err)
	}
	if len(p.Sectors) != 1 || p.Sectors[0].Name != "Echt" {
		t.Fatalf("Sektoren = %+v", p.Sectors)
	}
	if !hatWarnung(p.Warnings, "Strich") {
		t.Errorf("keine Warnung zum weggelassenen Sektor: %v", p.Warnings)
	}
}

func TestParsePlanMeldetAusreisser(t *testing.T) {
	// Hamburg in einem Dresdner Sektorschnitt: auf der Karte nur zu sehen, wenn
	// jemand weit genug herauszoomt.
	p, err := parsePlan(`{
      "sectors": [
        {"name": "Altstadt", "geometry": {"type":"Polygon","coordinates":[[[13.73,51.04],[13.75,51.04],[13.75,51.06],[13.73,51.06],[13.73,51.04]]]}}
      ],
      "hotspots": [
        {"name":"Zwinger","lat":51.05306,"lng":13.73361},
        {"name":"Landungsbrücken","lat":53.5459,"lng":9.9686}
      ]
    }`)
	if err != nil {
		t.Fatalf("parsePlan: %v", err)
	}
	if len(p.Hotspots) != 2 {
		t.Fatalf("Ausreißer sollen bleiben, damit man sie abwählen kann: %+v", p.Hotspots)
	}
	if !hatWarnung(p.Warnings, "Landungsbrücken") {
		t.Errorf("Ausreißer nicht gemeldet: %v", p.Warnings)
	}
	if hatWarnung(p.Warnings, "Zwinger") {
		t.Errorf("Punkt im Sektor fälschlich gemeldet: %v", p.Warnings)
	}
}

func TestParsePlanOhneJSON(t *testing.T) {
	for _, text := range []string{
		"",
		"Ich kann dir dabei leider nicht helfen.",
		`{"city": "Dresden", "hotspots": [`, // abgeschnittene Antwort
	} {
		if _, err := parsePlan(text); err == nil {
			t.Errorf("parsePlan(%q) lieferte keinen Fehler", text)
		}
	}
}

func TestPlanPromptNenntDieStadt(t *testing.T) {
	p := planPrompt("Leipzig")
	if strings.Contains(p, "{{STADT}}") {
		t.Error("Platzhalter blieb stehen")
	}
	if strings.Count(p, "Leipzig") < 2 {
		t.Error("Die Stadt steht nicht im Text und nicht im Beispiel-JSON")
	}
}

func hatWarnung(warnungen []string, teil string) bool {
	for _, w := range warnungen {
		if strings.Contains(w, teil) {
			return true
		}
	}
	return false
}
