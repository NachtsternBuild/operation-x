package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/schema"
	"github.com/elias/operation-x/pkg/testhilfe"
)

// Die Trennung zwischen zwei Spielen auf demselben Server.
//
// Sie hängt an zwei Funktionen: gameOf nimmt das Spiel aus dem angemeldeten
// Zugang, nie aus der Anfrage; ausSpiel besteht darauf, dass eine Kennung aus
// der Anfrage auch zu diesem Spiel gehört. Fällt eine der beiden aus, sieht
// man es im Betrieb nicht — bis eine Gruppe die Rätsel der anderen löst.

func ereignis(app core.App, auth *core.Record) *core.RequestEvent {
	e := &core.RequestEvent{}
	e.App = app
	e.Auth = auth
	e.Request = httptest.NewRequest("GET", "/api/opx/test", nil)
	e.Response = httptest.NewRecorder()
	return e
}

func TestFremdeKennungWirdNichtGefunden(t *testing.T) {
	app := testhilfe.App(t)

	spielA := testhilfe.Spiel(t, app, "Spiel A")
	spielB := testhilfe.Spiel(t, app, "Spiel B")
	teamA := testhilfe.Team(t, app, spielA.Id, "Team_A", schema.RoleDetective)

	// Ein Rätsel in jedem Spiel.
	raetselA := testhilfe.Neu(t, app, schema.ColPuzzles, map[string]any{
		"game": spielA.Id, "title": "Eigenes", "kind": "A", "answer": "x",
	})
	raetselB := testhilfe.Neu(t, app, schema.ColPuzzles, map[string]any{
		"game": spielB.Id, "title": "Fremdes", "kind": "A", "answer": "x",
	})

	e := ereignis(app, teamA)

	if _, err := ausSpiel(e, schema.ColPuzzles, raetselA.Id, spielA.Id); err != nil {
		t.Fatalf("Das eigene Rätsel war nicht erreichbar: %v", err)
	}

	_, err := ausSpiel(e, schema.ColPuzzles, raetselB.Id, spielA.Id)
	if err == nil {
		t.Fatal("Das Rätsel des anderen Spiels war erreichbar — die Trennung " +
			"zwischen zwei Gruppen auf demselben Server ist damit hinfällig")
	}
	if !strings.Contains(err.Error(), "gibt es in diesem Spiel nicht") {
		t.Errorf("Unerwartete Begründung: %v", err)
	}

	// Und für etwas, das es gar nicht gibt, muss dieselbe Antwort kommen —
	// sonst verriete der Unterschied, dass die Kennung existiert.
	_, errFehlt := ausSpiel(e, schema.ColPuzzles, "gibtesnichtxyz", spielA.Id)
	if errFehlt == nil || errFehlt.Error() != err.Error() {
		t.Errorf("„gehört einem anderen“ und „gibt es nicht“ antworten "+
			"verschieden:\n  fremd: %v\n  fehlt: %v", err, errFehlt)
	}
}

// gameOf darf das Spiel niemals aus der Anfrage nehmen, sondern nur aus dem
// angemeldeten Datensatz.
func TestSpielKommtAusDemZugangNichtAusDerAnfrage(t *testing.T) {
	app := testhilfe.App(t)

	spielA := testhilfe.Spiel(t, app, "Spiel A")
	spielB := testhilfe.Spiel(t, app, "Spiel B")
	teamA := testhilfe.Team(t, app, spielA.Id, "Team_A", schema.RoleDetective)

	e := ereignis(app, teamA)
	// Eine Anfrage, die behauptet, zu Spiel B zu gehören.
	e.Request = httptest.NewRequest("GET", "/api/opx/live?game="+spielB.Id, nil)

	gefunden, err := gameOf(e)
	if err != nil {
		t.Fatalf("gameOf scheiterte: %v", err)
	}
	if gefunden.Id != spielA.Id {
		t.Errorf("gameOf lieferte %q, erwartet war das Spiel des Zugangs (%q)",
			gefunden.Id, spielA.Id)
	}
}

func TestOhneAnmeldungKeinSpiel(t *testing.T) {
	app := testhilfe.App(t)
	testhilfe.Spiel(t, app, "Spiel A")

	if _, err := gameOf(ereignis(app, nil)); err == nil {
		t.Error("Ohne angemeldeten Zugang lieferte gameOf trotzdem ein Spiel")
	}
}

// Ein Zugang, dessen Spiel gelöscht wurde, darf nicht auf ein beliebiges
// anderes ausweichen.
func TestZugangOhneSpielFaelltNichtZurueck(t *testing.T) {
	app := testhilfe.App(t)

	spielA := testhilfe.Spiel(t, app, "Spiel A")
	teamA := testhilfe.Team(t, app, spielA.Id, "Team_A", schema.RoleDetective)
	testhilfe.Spiel(t, app, "Spiel B")

	// Den Zugang künstlich verwaisen lassen.
	teamA.Set("game", "gibtesnichtxyz")

	if spiel, err := gameOf(ereignis(app, teamA)); err == nil {
		t.Errorf("Ein verwaister Zugang bekam Spiel %q zugeteilt", spiel.Id)
	}
}
