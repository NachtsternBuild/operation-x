// Package seed legt ein spielfertiges Testspiel an.
//
// Gedacht für die Entwicklung und die Trockenübung: Ein Aufruf, und es gibt ein
// Spielfeld in Dresden mit Sektoren, echten Hotspots und Zugängen für alle Rollen.
// Der Einrichtungsassistent aus Abschnitt 11 des Konzepts erzeugt später dasselbe
// interaktiv – dieser Datensatz bleibt als Testgrundlage bestehen.
package seed

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	gamepkg "github.com/elias/operation-x/pkg/game"
	"github.com/elias/operation-x/pkg/schema"
)

// Die Zugänge des Testspiels.
//
// Die Kennwörter stehen hier bewusst *nicht*: Sie werden bei jedem Aufruf neu
// gewürfelt und einmal ausgegeben. Feste Kennwörter im Quelltext eines
// offenen Repositories sind bekannte Kennwörter – und dieser Befehl legt ein
// Spiel an, das über denselben Tunnel erreichbar ist wie ein echtes. Wer ihn
// zum Ausprobieren auf einem Server laufen lässt, der am Netz hängt, hätte
// sonst fünf offene Türen, deren Schlüssel jeder nachlesen kann.
var testTeams = []struct {
	Callsign string
	Display  string
	Role     string
	Color    string
	FP       int
}{
	{"HQ", "Einsatzzentrale", schema.RoleHQ, "#d9a441", 0},
	{"MisterX", "Mister X", schema.RoleMisterX, "#ff5c47", 5},
	{"Team_Alpha", "Team Alpha", schema.RoleDetective, "#35c0d6", 4},
	{"Team_Bravo", "Team Bravo", schema.RoleDetective, "#5ad1a0", 4},
	{"Team_Charlie", "Team Charlie", schema.RoleDetective, "#8ea6ff", 4},
}

// Sektoren als grobe Rechtecke. In Abschnitt 1 ersetzt der Einrichtungsassistent
// sie durch die echten Dresdner Stadtteilgrenzen aus den OSM-Daten.
var testSectors = []struct {
	Code       string
	Name       string
	Color      string
	W, S, E, N float64 // Bounding-Box: West, Süd, Ost, Nord
}{
	{"A", "Altstadt", "#d9a441", 13.725, 51.043, 13.755, 51.058},
	{"B", "Äußere Neustadt", "#35c0d6", 13.735, 51.058, 13.775, 51.080},
	{"C", "Johannstadt", "#5ad1a0", 13.755, 51.043, 13.790, 51.062},
	{"D", "Friedrichstadt", "#8ea6ff", 13.695, 51.048, 13.725, 51.070},
	{"E", "Blasewitz & Striesen", "#c88ae0", 13.790, 51.035, 13.830, 51.065},
	{"F", "Südvorstadt", "#e08a8a", 13.710, 51.020, 13.760, 51.043},
}

// Echte Dresdner Orte – markant genug, um sie vor Ort eindeutig zu finden.
var testHotspots = []struct {
	Number int
	Name   string
	Lat    float64
	Lng    float64
	Sector string
	Kind   string
	Code   string
}{
	{1, "Frauenkirche", 51.0519, 13.7414, "A", "landmark", "KUPPEL"},
	{2, "Zwinger, Kronentor", 51.0533, 13.7343, "A", "landmark", "KRONE"},
	{3, "Semperoper", 51.0544, 13.7351, "A", "landmark", "APOLL"},
	{4, "Fürstenzug", 51.0530, 13.7398, "A", "landmark", "REITER"},
	{5, "Hauptbahnhof", 51.0400, 13.7322, "F", "transit", "GLEIS"},
	{6, "Yenidze", 51.0546, 13.7237, "D", "building", "TABAK"},
	{7, "Kraftwerk Mitte", 51.0510, 13.7220, "D", "building", "TURBINE"},
	{8, "Goldener Reiter", 51.0577, 13.7420, "B", "landmark", "AUGUST"},
	{9, "Albertplatz, Artesischer Brunnen", 51.0640, 13.7460, "B", "landmark", "QUELLE"},
	{10, "Pfunds Molkerei", 51.0641, 13.7551, "B", "building", "KACHEL"},
	{11, "Alaunpark", 51.0716, 13.7541, "B", "park", "WIESE"},
	{12, "Militärhistorisches Museum", 51.0715, 13.7457, "B", "building", "KEIL"},
	{13, "Großer Garten, Palais", 51.0400, 13.7660, "C", "park", "PALAIS"},
	{14, "Dresdner Zoo, Eingang", 51.0387, 13.7628, "C", "park", "FLAMINGO"},
	{15, "Waldschlösschenbrücke", 51.0674, 13.7841, "C", "transit", "BOGEN"},
	{16, "Schloss Albrechtsberg", 51.0631, 13.7867, "E", "landmark", "TERRASSE"},
	{17, "Blaues Wunder", 51.0540, 13.8090, "E", "transit", "STAHL"},
	{18, "Schillerplatz", 51.0533, 13.8065, "E", "transit", "SCHILLER"},
	{19, "Panometer", 51.0308, 13.7614, "F", "building", "RUNDBILD"},
	{20, "Nürnberger Ei", 51.0304, 13.7291, "F", "transit", "KREISEL"},
}

// Regelwerte aus dem Regelwerk v3.2. Die Engine liest ausschließlich hieraus,
// damit sich jeder Wert im Einrichtungsassistenten verstellen lässt, ohne
// dass Code angefasst werden muss.
// DefaultConfig liefert die Regelwerte, mit denen ein neues Spiel startet.
// Auch die Ersteinrichtung greift darauf zu – ein Spiel ohne Regelwerte wäre
// von der ersten Sekunde an unbrauchbar.
func DefaultConfig() map[string]any {
	return map[string]any{
		"ping_interval_min":         10,
		"ping_interval_transit_min": 13,
		"ping_warn_at_violation":    1,
		"ping_penalty_points":       -10,
		"ping_lockout_min":          5,
		"ping_lockout_points":       -15,

		// Mister X bekommt die längere Leine: Er soll sich verstecken dürfen,
		// nur eben nicht die ganze Runde an einem Fleck.
		"camping_detective_min": 5,
		"camping_detective_fp":  -1,
		"camping_misterx_min":   20,
		"camping_misterx_fp":    -1,
		"camping_misterx_bonus": 20,
		"camping_radius_m":      75,

		"hotspot_max_distance_m":  150,
		"sighting_max_distance_m": 50,
		"sighting_dwell_sec":      20,
		"radar_blur_m":            200,
		"radar_blur_scan_m":       300,

		"intel_hot_min":  3,
		"intel_warm_min": 15,

		"start_fp_misterx":   5,
		"start_fp_detective": 4,

		"points_mission":        15,
		"points_puzzle":         15,
		"points_puzzle_with_fp": 8,
		"points_false_trail":    20,
		"points_arrest_level1":  5,
		"points_arrest_level2":  10,
		"points_victory":        150,
		"points_mission_failed": -20,
		"points_arrest_failed":  -15,

		"cost_reroute":     2,
		"cost_false_trail": 1,
		"cost_delay":       2,
		"cost_phantom":     1,
		"cost_smoke":       2,
		"cost_ghost":       3,
		"cost_gps_ping":    4,
		"cost_lockdown":    3,
		"cost_bug":         2,
		"cost_scan":        1,
		"cost_unlock":      1,

		"mission_grace_min":         10,
		"mission_grace_max_min":     30,
		"mission_fail_lockout_min":  10,
		"arrest_fail_lockout_min":   10,
		"final_shrink_interval_min": 5,
	}
}

// Dresden legt das Testspiel an. Läuft vollständig in einer Transaktion:
// Bricht irgendetwas ab, bleibt die Datenbank unverändert statt halb gefüllt.
func Dresden(app core.App) error {
	return app.RunInTransaction(func(tx core.App) error {
		return seedDresden(tx)
	})
}

func seedDresden(app core.App) error {
	const gameName = "Testlauf Dresden"

	if existing, _ := app.FindFirstRecordByData(schema.ColGames, "name", gameName); existing != nil {
		return fmt.Errorf("es gibt bereits ein Spiel namens %q – zum Neuanlegen zuerst löschen", gameName)
	}

	gamesCol, err := app.FindCollectionByNameOrId(schema.ColGames)
	if err != nil {
		return err
	}

	start := time.Now().Add(30 * time.Minute)
	duration := 6 * time.Hour

	game := core.NewRecord(gamesCol)
	game.Set("name", gameName)
	game.Set("city", "Dresden")
	game.Set("status", schema.GameSetup)
	game.Set("starts_at", start)
	game.Set("ends_at", start.Add(duration))
	game.Set("duration_min", int(duration.Minutes()))
	game.Set("retention_hours", 24)
	game.Set("final_active", false)
	game.Set("config", DefaultConfig())
	// Spielgebiet: grobe Umfassung der oben definierten Sektoren.
	game.Set("area", bbox(13.695, 51.020, 13.830, 51.080))

	if err := app.Save(game); err != nil {
		return fmt.Errorf("Spiel anlegen: %w", err)
	}

	// --- Sektoren ---
	sectorsCol, err := app.FindCollectionByNameOrId(schema.ColSectors)
	if err != nil {
		return err
	}
	sectorIDs := map[string]string{}
	for _, s := range testSectors {
		rec := core.NewRecord(sectorsCol)
		rec.Set("game", game.Id)
		rec.Set("code", s.Code)
		rec.Set("name", s.Name)
		rec.Set("color", s.Color)
		rec.Set("geometry", bbox(s.W, s.S, s.E, s.N))
		if err := app.Save(rec); err != nil {
			return fmt.Errorf("Sektor %s anlegen: %w", s.Code, err)
		}
		sectorIDs[s.Code] = rec.Id
	}

	// --- Hotspots ---
	hotspotsCol, err := app.FindCollectionByNameOrId(schema.ColHotspots)
	if err != nil {
		return err
	}
	for _, h := range testHotspots {
		rec := core.NewRecord(hotspotsCol)
		rec.Set("game", game.Id)
		rec.Set("number", h.Number)
		rec.Set("name", h.Name)
		rec.Set("lat", h.Lat)
		rec.Set("lng", h.Lng)
		rec.Set("sector", sectorIDs[h.Sector])
		rec.Set("kind", h.Kind)
		rec.Set("passcode", h.Code)
		if err := app.Save(rec); err != nil {
			return fmt.Errorf("Hotspot #%02d anlegen: %w", h.Number, err)
		}
	}

	// --- Teams ---
	teamsCol, err := app.FindCollectionByNameOrId(schema.ColTeams)
	if err != nil {
		return err
	}
	vergeben := make([]string, 0, len(testTeams))

	for _, t := range testTeams {
		kennwort, err := SpeakablePassword()
		if err != nil {
			return fmt.Errorf("Kennwort für %s: %w", t.Callsign, err)
		}
		vergeben = append(vergeben, kennwort)

		rec := core.NewRecord(teamsCol)
		rec.Set("callsign", t.Callsign)
		rec.Set("display", t.Display)
		rec.Set("role", t.Role)
		rec.Set("color", t.Color)
		rec.Set("game", game.Id)
		// Das Guthaben kommt aus dem Kontobuch, nicht von hier: Sonst stünde es
		// am Team, ohne dass eine Buchung es erklärt.
		rec.Set("fp", 0)
		rec.Set("points", 0)
		rec.Set("active", true)
		rec.Set("in_transit", false)
		rec.SetPassword(kennwort)
		if err := app.Save(rec); err != nil {
			return fmt.Errorf("Team %s anlegen: %w", t.Callsign, err)
		}
		if err := gamepkg.BookStart(app, game.Id, rec.Id, t.FP); err != nil {
			return fmt.Errorf("Startguthaben für %s: %w", t.Callsign, err)
		}
	}

	fmt.Printf("\nTestspiel %q angelegt.\n", gameName)
	fmt.Printf("  %d Sektoren, %d Hotspots, %d Zugänge\n\n", len(testSectors), len(testHotspots), len(testTeams))
	fmt.Println("  Rufzeichen        Rolle       Kennwort")
	fmt.Println("  ----------------------------------------------------")
	for i, t := range testTeams {
		fmt.Printf("  %-16s  %-10s  %s\n", t.Callsign, t.Role, vergeben[i])
	}
	fmt.Println()
	fmt.Println("  Diese Kennwörter stehen nirgends sonst. Jetzt notieren –")
	fmt.Println("  oder den Befehl auf einer leeren Datenbank wiederholen.")
	fmt.Println()

	return nil
}

// bbox baut aus einer Bounding-Box ein GeoJSON-Polygon.
func bbox(w, s, e, n float64) types.JSONRaw {
	return types.JSONRaw(fmt.Sprintf(
		`{"type":"Polygon","coordinates":[[[%g,%g],[%g,%g],[%g,%g],[%g,%g],[%g,%g]]]}`,
		w, s, e, s, e, n, w, n, w, s,
	))
}
