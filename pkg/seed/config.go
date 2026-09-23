package seed

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/schema"
)

// ShowConfig gibt die Regelwerte des laufenden Spiels aus.
func ShowConfig(app core.App) error {
	game, err := currentGame(app)
	if err != nil {
		return err
	}

	values, err := configMap(game)
	if err != nil {
		return err
	}

	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Printf("\nRegelwerte von %q:\n\n", game.GetString("name"))
	for _, k := range keys {
		fmt.Printf("  %-28s %v\n", k, values[k])
	}
	fmt.Println()

	return nil
}

// SetConfig ändert einen Regelwert.
//
// Nützlich zum Nachjustieren der Balance zwischen zwei Durchläufen – und beim
// Entwickeln, um Fristen kurz genug für einen Test zu machen, ohne zehn Minuten
// zu warten.
func SetConfig(app core.App, key, value string) error {
	game, err := currentGame(app)
	if err != nil {
		return err
	}

	values, err := configMap(game)
	if err != nil {
		return err
	}

	if _, known := values[key]; !known {
		return fmt.Errorf("unbekannter Regelwert %q – „operationx config“ zeigt alle an", key)
	}

	parsed, err := parseValue(value)
	if err != nil {
		return err
	}

	old := values[key]
	values[key] = parsed
	game.Set("config", values)

	if err := app.Save(game); err != nil {
		return fmt.Errorf("Regelwert speichern: %w", err)
	}

	fmt.Printf("%s: %v → %v\n", key, old, parsed)
	return nil
}

func parseValue(value string) (any, error) {
	if i, err := strconv.Atoi(value); err == nil {
		return i, nil
	}
	if f, err := strconv.ParseFloat(value, 64); err == nil {
		return f, nil
	}
	if b, err := strconv.ParseBool(value); err == nil {
		return b, nil
	}
	return nil, fmt.Errorf("%q ist weder Zahl noch Wahrheitswert", value)
}

func configMap(game *core.Record) (map[string]any, error) {
	values := map[string]any{}

	raw := game.Get("config")
	if raw == nil {
		return values, nil
	}

	var data []byte
	switch v := raw.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	case json.RawMessage:
		data = v
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		data = encoded
	}

	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("Regelwerte lesen: %w", err)
	}
	return values, nil
}

func currentGame(app core.App) (*core.Record, error) {
	games, err := app.FindRecordsByFilter(
		schema.ColGames, "status != {:f}", "-created", 1, 0,
		map[string]any{"f": schema.GameFinished},
	)
	if err != nil {
		return nil, err
	}
	if len(games) == 0 {
		return nil, fmt.Errorf("es ist kein Spiel eingerichtet")
	}
	return games[0], nil
}
