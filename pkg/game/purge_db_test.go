package game

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/schema"
	"github.com/elias/operation-x/pkg/testhilfe"
)

// Nach der Frist darf kein Ort mehr in der Datenbank stehen.
//
// Geprüft wird beides: dass die Spur weg ist, und dass die Begründung des
// Punktestands bleibt. Ein Löschen, das auch die Buchungen mitnimmt, wäre
// kein Datenschutz, sondern Datenverlust.
func TestSpurWirdGeloescht(t *testing.T) {
	app := testhilfe.App(t)
	e := New(app)

	spiel := testhilfe.Neu(t, app, schema.ColGames, map[string]any{
		"name": "Prüflauf", "city": "Dresden", "status": schema.GameFinished,
		"retention_hours": 24,
	})
	team := testhilfe.Neu(t, app, schema.ColTeams, map[string]any{
		"game": spiel.Id, "callsign": "Team_Pruef", "role": "detective",
		"password": "pruef-lauf-12",
	})

	testhilfe.Neu(t, app, schema.ColPositions, map[string]any{
		"game": spiel.Id, "team": team.Id, "lat": 51.05, "lng": 13.74,
		"captured_at": time.Now(),
	})
	beweis := testhilfe.Neu(t, app, schema.ColEvidence, map[string]any{
		"game": spiel.Id, "team": team.Id, "lat": 51.06, "lng": 13.75,
		"distance_m": 12, "status": "accepted",
	})
	sperre := testhilfe.Neu(t, app, schema.ColPenalties, map[string]any{
		"game": spiel.Id, "team": team.Id, "lat": 51.07, "lng": 13.76,
		"kind": "lockout_location",
	})
	if _, err := Book(app, Booking{
		Game: spiel.Id, Team: team.Id, Type: EventMocked,
		Reason: "Fake-GPS", OccurredAt: time.Now(),
		DedupeKey: "mocked:pruef",
		Payload:   map[string]any{"lat": 51.08, "lng": 13.77},
	}); err != nil {
		t.Fatalf("Buchung: %v", err)
	}

	// Frist auf gestern setzen und den Takt einmal laufen lassen.
	spiel.Set("purge_at", time.Now().Add(-time.Hour))
	if err := app.Save(spiel); err != nil {
		t.Fatal(err)
	}
	if err := e.purgeDue(time.Now()); err != nil {
		t.Fatalf("Löschen: %v", err)
	}

	positionen, err := app.FindRecordsByFilter(schema.ColPositions, "game = {:g}", "", 0, 0,
		map[string]any{"g": spiel.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(positionen) != 0 {
		t.Errorf("Positionen: %d übrig, erwartet 0", len(positionen))
	}

	frisch, err := app.FindRecordById(schema.ColEvidence, beweis.Id)
	if err != nil {
		t.Fatalf("Beweis verschwunden, sollte bleiben: %v", err)
	}
	if frisch.GetFloat("lat") != 0 || frisch.GetFloat("lng") != 0 {
		t.Errorf("Beweis trägt noch einen Ort: %v/%v",
			frisch.GetFloat("lat"), frisch.GetFloat("lng"))
	}
	if frisch.GetString("photo") != "" {
		t.Errorf("Beweisfoto liegt noch da: %q", frisch.GetString("photo"))
	}
	if frisch.GetString("status") != "accepted" {
		t.Errorf("Der Ausgang des Beweises ging verloren: %q", frisch.GetString("status"))
	}

	frischeSperre, err := app.FindRecordById(schema.ColPenalties, sperre.Id)
	if err != nil {
		t.Fatalf("Sperre verschwunden, sollte bleiben: %v", err)
	}
	if frischeSperre.GetFloat("lat") != 0 {
		t.Errorf("Sperre trägt noch einen Ort: %v", frischeSperre.GetFloat("lat"))
	}

	meldungen, err := app.FindRecordsByFilter(schema.ColEvents,
		"game = {:g} && type = {:t}", "", 0, 0,
		map[string]any{"g": spiel.Id, "t": string(EventMocked)})
	if err != nil {
		t.Fatal(err)
	}
	if len(meldungen) == 0 {
		t.Fatal("Die Meldung über die vorgetäuschte Position fehlt ganz")
	}
	for _, m := range meldungen {
		if payload := m.GetString("payload"); payload != "" && payload != "{}" &&
			payload != "null" {
			t.Errorf("Die Meldung trägt noch Koordinaten: %s", payload)
		}
	}
}

// Ein Widerruf nimmt alles mit.
//
// Die Oberfläche löscht dafür das Team — und verlässt sich darauf, dass die
// Datenbank den Rest erledigt, weil Positionen, Meldungen und Buchungen das
// Team als Pflichtfeld führen. Verlässt man sich auf etwas, prüft man es:
// Bliebe hier eine Position liegen, wäre der Widerruf eine Behauptung.
func TestTeamLoeschenNimmtSpurMit(t *testing.T) {
	app := testhilfe.App(t)

	spiel := testhilfe.Neu(t, app, schema.ColGames, map[string]any{
		"name": "Widerruf", "city": "Dresden", "status": schema.GameRunning,
	})
	team := testhilfe.Neu(t, app, schema.ColTeams, map[string]any{
		"game": spiel.Id, "callsign": "Team_Weg", "role": "detective",
		"password": "pruef-lauf-13",
	})
	bleibt := testhilfe.Neu(t, app, schema.ColTeams, map[string]any{
		"game": spiel.Id, "callsign": "Team_Bleibt", "role": "detective",
		"password": "pruef-lauf-14",
	})

	for _, t2 := range []*core.Record{team, bleibt} {
		testhilfe.Neu(t, app, schema.ColPositions, map[string]any{
			"game": spiel.Id, "team": t2.Id, "lat": 51.05, "lng": 13.74,
			"captured_at": time.Now(),
		})
	}
	if _, err := Book(app, Booking{
		Game: spiel.Id, Team: team.Id, Type: EventMocked,
		Reason: "Fake-GPS", OccurredAt: time.Now(), DedupeKey: "mocked:weg",
	}); err != nil {
		t.Fatalf("Buchung: %v", err)
	}

	if err := app.Delete(team); err != nil {
		t.Fatalf("Team löschen: %v", err)
	}

	uebrig, err := app.FindRecordsByFilter(schema.ColPositions, "team = {:t}", "", 0, 0,
		map[string]any{"t": team.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(uebrig) != 0 {
		t.Errorf("Nach dem Löschen liegen noch %d Positionen des Teams da", len(uebrig))
	}

	andere, err := app.FindRecordsByFilter(schema.ColPositions, "team = {:t}", "", 0, 0,
		map[string]any{"t": bleibt.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(andere) != 1 {
		t.Errorf("Das andere Team hat %d Positionen, erwartet 1 — es wurde zu viel gelöscht",
			len(andere))
	}
}

// Beweisfotos dürfen nicht am offenen Dateiweg hängen.
//
// PocketBase liefert Dateien unbewachter Felder jedem aus, der die Adresse
// kennt — ohne Anmeldung. Für Fotos aus dem öffentlichen Raum ist das zu
// wenig. Diese Prüfung hält fest, dass das Feld bewacht bleibt; sie schlägt
// fehl, sobald jemand das Flag wieder herausnimmt.
func TestBeweisfotoIstBewacht(t *testing.T) {
	app := testhilfe.App(t)

	col, err := app.FindCollectionByNameOrId(schema.ColEvidence)
	if err != nil {
		t.Fatal(err)
	}

	feld, ok := col.Fields.GetByName("photo").(*core.FileField)
	if !ok {
		t.Fatal("Das Feld photo ist kein Dateifeld mehr")
	}
	if !feld.Protected {
		t.Error("Das Beweisfoto hängt am offenen Dateiweg: jeder mit der Adresse " +
			"bekommt es ohne Anmeldung")
	}
	if col.ViewRule != nil {
		t.Errorf("Die Sichtregel der Beweise ist offen (%q) — dann nützt auch "+
			"Protected nichts", *col.ViewRule)
	}
}
