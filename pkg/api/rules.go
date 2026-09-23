package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/elias/operation-x/pkg/game"
	"github.com/elias/operation-x/pkg/schema"
)

// Regelwerte lesen und stellen.
//
// Zwei Ansichten auf dieselbe Quelle: Die Zentrale darf ändern, alle anderen
// dürfen nachschlagen. Das Nachschlagen ist kein Beiwerk – die häufigste Frage
// am Spieltag ist, was ein Joker kostet, und sie soll niemanden zwingen, dafür
// den Funk zu belegen.

type ruleValue struct {
	game.Rule
	Value int `json:"value"`
}

// handleRules liefert das Regelverzeichnis mit den Werten dieses Spiels.
//
// Für Spieler gefiltert: Die Preise der Gegenseite gehören nicht auf den
// eigenen Zettel. Für die Zentrale vollständig.
func handleRules(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	values := configValues(gameRec)
	role := ""
	if e.Auth != nil {
		role = e.Auth.GetString("role")
	}

	groups := map[string][]ruleValue{}
	for _, rule := range game.RuleIndex {
		if role != schema.RoleHQ && rule.Side != "" && rule.Side != role {
			continue
		}
		v, ok := values[rule.Key]
		if !ok {
			continue
		}
		groups[rule.Group] = append(groups[rule.Group], ruleValue{Rule: rule, Value: v})
	}

	// Die Reihenfolge der Gruppen kommt aus dem Verzeichnis, nicht aus der
	// Landkarte der Map – sonst stünde bei jedem Aufruf etwas anderes oben.
	ordered := make([]map[string]any, 0, len(groups))
	for _, name := range game.RuleGroups() {
		if rules, ok := groups[name]; ok {
			ordered = append(ordered, map[string]any{"group": name, "rules": rules})
		}
	}

	return e.JSON(http.StatusOK, map[string]any{
		"groups":   ordered,
		"editable": role == schema.RoleHQ,
	})
}

type ruleWrite struct {
	Values map[string]int `json:"values"`
}

// handleSetRules ändert Regelwerte.
//
// Bewusst mehrere auf einmal: Wer den Meldeabstand verlängert, will meist auch
// die Strafe anpassen, und zwei getrennte Speichervorgänge ließen dazwischen
// einen Zustand entstehen, den niemand gewollt hat.
func handleSetRules(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	var req ruleWrite
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}
	if len(req.Values) == 0 {
		return e.BadRequestError("Es wurde kein Wert übergeben.", nil)
	}

	current := configValues(gameRec)
	changed := map[string][2]int{}

	for key, value := range req.Values {
		rule, ok := game.RuleByKey(key)
		if !ok {
			return e.BadRequestError(fmt.Sprintf("Unbekannter Regelwert %q.", key), nil)
		}
		if value < rule.Min || value > rule.Max {
			return e.BadRequestError(fmt.Sprintf(
				"%s: %d liegt außerhalb des zulässigen Bereichs %d bis %d.",
				rule.Name, value, rule.Min, rule.Max), nil)
		}
		if old, ok := current[key]; !ok || old != value {
			changed[key] = [2]int{current[key], value}
			current[key] = value
		}
	}

	if len(changed) == 0 {
		return e.JSON(http.StatusOK, map[string]any{"changed": 0})
	}

	raw, err := json.Marshal(current)
	if err != nil {
		return err
	}
	gameRec.Set("config", types.JSONRaw(raw))
	if err := e.App.Save(gameRec); err != nil {
		return e.InternalServerError("Die Regelwerte ließen sich nicht speichern.", err)
	}

	// Jede Änderung ins Protokoll: Am Spieltag beantwortet das die Frage,
	// warum eine Strafe plötzlich anders ausfällt als eine Stunde zuvor.
	for key, pair := range changed {
		rule, _ := game.RuleByKey(key)
		_, _ = game.Book(e.App, game.Booking{
			Game:   gameRec.Id,
			Type:   "rule.changed",
			Reason: fmt.Sprintf("%s: %d → %d", rule.Name, pair[0], pair[1]),
			Payload: map[string]any{
				"schluessel": key, "vorher": pair[0], "nachher": pair[1],
			},
		})
	}

	return e.JSON(http.StatusOK, map[string]any{"changed": len(changed)})
}

// configValues liest die Regelwerte eines Spiels als flache Zahlenliste.
//
// Der Umweg über die Config-Struktur ist Absicht: Er füllt fehlende Werte mit
// der Voreinstellung auf, sodass ein Spiel aus einer älteren Fassung des
// Programms nicht mit Nullen dasteht.
func configValues(gameRec *core.Record) map[string]int {
	cfg := game.ConfigOf(gameRec)

	raw, err := json.Marshal(cfg)
	if err != nil {
		return map[string]int{}
	}

	var asFloat map[string]float64
	if err := json.Unmarshal(raw, &asFloat); err != nil {
		return map[string]int{}
	}

	out := make(map[string]int, len(asFloat))
	for k, v := range asFloat {
		out[k] = int(v)
	}
	return out
}
