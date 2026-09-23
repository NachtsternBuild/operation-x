// Package testhilfe stellt einen echten Server auf Zeit bereit.
//
// Die meisten Tests dieses Projekts prüfen reine Funktionen und brauchen
// nichts weiter. Ein Teil der Zusagen steht aber gerade nicht im Rechenweg,
// sondern in der Datenbank: dass ein Spiel die Daten eines anderen nicht
// sieht, dass eine gelöschte Bewegungsspur wirklich weg ist, dass die
// Servereinstellungen bei jedem Start sitzen.
//
// Dafür braucht es einen laufenden PocketBase samt Datenmodell. Er entsteht
// hier in einem Wegwerfverzeichnis und verschwindet mit dem Test.
package testhilfe

import (
	"os"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/schema"
)

// App startet einen Server mit vollständigem Datenmodell.
func App(t *testing.T) core.App {
	t.Helper()

	dir, err := os.MkdirTemp("", "opx-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir:  dir,
		HideStartBanner: true,
	})
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("Server starten: %v", err)
	}
	t.Cleanup(func() { app.ResetBootstrapState() })

	if err := schema.Ensure(app); err != nil {
		t.Fatalf("Datenmodell anlegen: %v", err)
	}
	return app
}

// Neu legt einen Datensatz an und gibt ihn zurück.
func Neu(t *testing.T, app core.App, collection string, werte map[string]any) *core.Record {
	t.Helper()

	col, err := app.FindCollectionByNameOrId(collection)
	if err != nil {
		t.Fatalf("%s: %v", collection, err)
	}
	rec := core.NewRecord(col)
	for k, v := range werte {
		rec.Set(k, v)
	}
	if err := app.Save(rec); err != nil {
		t.Fatalf("%s anlegen: %v", collection, err)
	}
	return rec
}

// Spiel legt ein Spiel mit Namen an — die Klammer um fast jeden Test.
func Spiel(t *testing.T, app core.App, name string) *core.Record {
	t.Helper()
	return Neu(t, app, schema.ColGames, map[string]any{
		"name": name, "city": "Dresden", "status": schema.GameRunning,
	})
}

// Team legt einen Zugang in einem Spiel an.
func Team(t *testing.T, app core.App, gameID, rufzeichen, rolle string) *core.Record {
	t.Helper()
	return Neu(t, app, schema.ColTeams, map[string]any{
		"game": gameID, "callsign": rufzeichen, "role": rolle,
		"password": "pruef-lauf-42", "active": true,
	})
}
