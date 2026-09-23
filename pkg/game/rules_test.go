package game

import (
	"encoding/json"
	"testing"
)

// Ein Regelwert, der im Verzeichnis fehlt, ist in der Oberfläche unsichtbar –
// weder änderbar noch nachschlagbar. Dieser Test ist die einzige Stelle, die
// das bemerkt, bevor es jemandem am Spieltag auffällt.
func TestRuleIndexKenntAlleWerte(t *testing.T) {
	raw, err := json.Marshal(Defaults())
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]any
	if err := json.Unmarshal(raw, &values); err != nil {
		t.Fatal(err)
	}

	for key := range values {
		if _, ok := RuleByKey(key); !ok {
			t.Errorf("Regelwert %q fehlt im Verzeichnis", key)
		}
	}

	for _, r := range RuleIndex {
		if _, ok := values[r.Key]; !ok {
			t.Errorf("Verzeichnis kennt %q, die Konfiguration nicht", r.Key)
		}
	}
}

// Grenzen, die den eigenen Vorgabewert ausschließen, sperren die Oberfläche
// gegen den Zustand, in dem sie ausgeliefert wird.
func TestRuleGrenzenSchliessenVorgabeEin(t *testing.T) {
	raw, _ := json.Marshal(Defaults())
	var values map[string]float64
	_ = json.Unmarshal(raw, &values)

	for _, r := range RuleIndex {
		v, ok := values[r.Key]
		if !ok {
			continue
		}
		if v < float64(r.Min) || v > float64(r.Max) {
			t.Errorf("%s: Vorgabe %v liegt außerhalb von %d..%d", r.Key, v, r.Min, r.Max)
		}
	}
}

func TestJedeRegelHatText(t *testing.T) {
	for _, r := range RuleIndex {
		if r.Name == "" || r.What == "" || r.Group == "" {
			t.Errorf("%s: unvollständig beschrieben", r.Key)
		}
		switch r.Side {
		case "", "misterx", "detective":
		default:
			t.Errorf("%s: unbekannte Seite %q", r.Key, r.Side)
		}
	}
}
