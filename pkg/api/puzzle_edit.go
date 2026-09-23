package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/game"
	"github.com/elias/operation-x/pkg/schema"
	"github.com/elias/operation-x/pkg/seed"
)

// Rätsel schreiben.
//
// Bisher kamen Rätsel nur aus dem Befehl "operationx puzzles". Für jemanden,
// der eine Eingabeaufforderung nie geöffnet hat, war das eine Sackgasse: Ohne
// Rätsel haben die Fahndungsteams nichts zu tun, und das Spiel zerfällt in
// Herumlaufen.
//
// Die Rätsel sind zugleich die Stelle, an der die Spielleitung ihre Stadt
// einbringt. Deshalb ist das hier kein Formular für Pflichtfelder, sondern ein
// Schreibplatz – mit den Beispielen als Starthilfe, nicht als Vorschrift.

// puzzleTypes beschreibt die sechs Typen des Regelwerks im Klartext.
//
// Die Bedeutung steht hier und nicht nur in der Oberfläche, weil sie zur Regel
// gehört: Wer ein Rätsel vom Typ B schreibt, muss wissen, dass es ein Foto
// braucht.
var puzzleTypes = []map[string]string{
	{"type": "A", "name": "Ausschluss", "hint": "Mehrere Angaben schließen nach und nach alles bis auf einen Ort aus."},
	{"type": "B", "name": "Foto", "hint": "Ein Bildausschnitt, der erkannt werden muss. Braucht ein Bild."},
	{"type": "C", "name": "Rechnung", "hint": "Zahlen aus der Stadt, die zu einer Lösung führen."},
	{"type": "D", "name": "Geometrie", "hint": "Richtungen, Abstände, Schnittpunkte auf der Karte."},
	{"type": "E", "name": "Vor Ort", "hint": "Nur zu lösen, wenn jemand tatsächlich dort steht."},
	{"type": "F", "name": "Kombination", "hint": "Setzt Ergebnisse mehrerer anderer Rätsel zusammen."},
}

type puzzleWrite struct {
	Code     string `json:"code"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
	Hint     string `json:"hint"`
	Category string `json:"category"`
	Points   int    `json:"points"`
	Order    int    `json:"order"`
	Unlocked bool   `json:"unlocked"`
}

// handlePuzzleMeta liefert, was die Oberfläche zum Schreiben braucht.
func handlePuzzleMeta(e *core.RequestEvent) error {
	return e.JSON(http.StatusOK, map[string]any{
		"types": puzzleTypes,
		"categories": []map[string]string{
			{"key": game.IntelGreen, "name": "Grün", "hint": "Belastbar. Der Hinweis stimmt."},
			{"key": game.IntelYellow, "name": "Gelb", "hint": "Ungenau. Stimmt im Kern, nicht im Detail."},
			{"key": game.IntelRed, "name": "Rot", "hint": "Vage. Grenzt allenfalls ein."},
		},
		"defaultPoints": game.Defaults().PointsPuzzle,
	})
}

// handleCreatePuzzle legt ein Rätsel an.
func handleCreatePuzzle(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	var req puzzleWrite
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}
	if err := validatePuzzle(&req); err != nil {
		return e.BadRequestError(err.Error(), nil)
	}

	// Ein leer gelassenes Kürzel wird vergeben, nicht angemahnt: Es dient der
	// Sortierung und dem Gespräch am Funk, nicht der Buchhaltung.
	if req.Code == "" {
		req.Code = nextPuzzleCode(e.App, gameRec.Id, req.Type)
	}
	if taken, err := puzzleCodeTaken(e.App, gameRec.Id, req.Code, ""); err != nil {
		return err
	} else if taken {
		return e.BadRequestError(
			fmt.Sprintf("Das Kürzel %q ist schon vergeben.", req.Code), nil)
	}

	col, err := e.App.FindCollectionByNameOrId(schema.ColPuzzles)
	if err != nil {
		return err
	}

	rec := core.NewRecord(col)
	rec.Set("game", gameRec.Id)
	applyPuzzle(rec, req)

	if err := e.App.Save(rec); err != nil {
		return e.InternalServerError("Das Rätsel ließ sich nicht speichern.", err)
	}

	return e.JSON(http.StatusOK, map[string]any{"id": rec.Id, "code": req.Code})
}

// handleUpdatePuzzle ändert ein bestehendes Rätsel.
func handleUpdatePuzzle(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}
	id := e.Request.PathValue("id")

	rec, err := ausSpiel(e, schema.ColPuzzles, id, gameRec.Id)
	if err != nil {
		return err
	}

	var req puzzleWrite
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}
	if err := validatePuzzle(&req); err != nil {
		return e.BadRequestError(err.Error(), nil)
	}
	if req.Code == "" {
		req.Code = rec.GetString("code")
	}
	if taken, err := puzzleCodeTaken(e.App, rec.GetString("game"), req.Code, rec.Id); err != nil {
		return err
	} else if taken {
		return e.BadRequestError(
			fmt.Sprintf("Das Kürzel %q ist schon vergeben.", req.Code), nil)
	}

	applyPuzzle(rec, req)
	if err := e.App.Save(rec); err != nil {
		return e.InternalServerError("Die Änderung ließ sich nicht speichern.", err)
	}

	return e.JSON(http.StatusOK, map[string]any{"id": rec.Id, "code": req.Code})
}

// handleDeletePuzzle entfernt ein Rätsel.
//
// Ein bereits gelöstes Rätsel bleibt stehen: Es hat Punkte und einen Hinweis
// erzeugt, und beides würde sonst auf ein Rätsel verweisen, das es nicht mehr
// gibt. Wer es loswerden will, sperrt es.
func handleDeletePuzzle(e *core.RequestEvent) error {
	gameRec, err := gameOf(e)
	if err != nil {
		return err
	}

	rec, err := ausSpiel(e, schema.ColPuzzles, e.Request.PathValue("id"), gameRec.Id)
	if err != nil {
		return err
	}

	attempts, err := e.App.FindRecordsByFilter(
		schema.ColAttempts, "puzzle = {:p}", "", 1, 0,
		map[string]any{"p": rec.Id},
	)
	if err != nil {
		return err
	}
	if len(attempts) > 0 {
		return e.BadRequestError(
			"An diesem Rätsel wurde schon gearbeitet. Es lässt sich sperren, "+
				"aber nicht mehr löschen – sonst zeigten die Buchungen ins Leere.", nil)
	}

	if err := e.App.Delete(rec); err != nil {
		return e.InternalServerError("Löschen fehlgeschlagen.", err)
	}
	return e.JSON(http.StatusOK, map[string]any{"deleted": true})
}

// handleSeedPuzzles legt den Beispielsatz an – dasselbe wie der Befehl
// "operationx puzzles", nur ohne Eingabeaufforderung.
func handleSeedPuzzles(e *core.RequestEvent) error {
	if err := seed.AddPuzzles(e.App); err != nil {
		return e.BadRequestError(err.Error(), nil)
	}
	return e.JSON(http.StatusOK, map[string]any{"added": true})
}

func validatePuzzle(req *puzzleWrite) error {
	req.Code = strings.TrimSpace(req.Code)
	req.Title = strings.TrimSpace(req.Title)
	req.Question = strings.TrimSpace(req.Question)
	req.Answer = strings.TrimSpace(req.Answer)
	req.Hint = strings.TrimSpace(req.Hint)

	// Obergrenzen, damit ein verrutschtes Einfügen aus der Zwischenablage nicht
	// ein Megabyte in die Datenbank schreibt. Großzügig genug für jedes Rätsel,
	// das jemand von Hand schreibt.
	req.Code = kuerzen(req.Code, 12)
	req.Title = kuerzen(req.Title, 120)
	req.Question = kuerzen(req.Question, 2000)
	req.Answer = kuerzen(req.Answer, 500)
	req.Hint = kuerzen(req.Hint, 500)

	if req.Title == "" {
		return fmt.Errorf("das Rätsel braucht einen Titel")
	}
	if req.Question == "" {
		return fmt.Errorf("das Rätsel braucht eine Frage")
	}
	if req.Answer == "" {
		return fmt.Errorf("ohne Lösung lässt sich nichts prüfen")
	}

	valid := false
	for _, t := range puzzleTypes {
		if t["type"] == req.Type {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("unbekannter Rätseltyp %q", req.Type)
	}

	switch req.Category {
	case game.IntelGreen, game.IntelYellow, game.IntelRed:
	case "":
		req.Category = game.IntelYellow
	default:
		return fmt.Errorf("unbekannte Hinweisqualität %q", req.Category)
	}

	if req.Points == 0 {
		req.Points = game.Defaults().PointsPuzzle
	}
	return nil
}

func applyPuzzle(rec *core.Record, req puzzleWrite) {
	rec.Set("code", req.Code)
	rec.Set("type", req.Type)
	rec.Set("title", req.Title)
	rec.Set("question", req.Question)
	rec.Set("answer", req.Answer)
	rec.Set("hint", req.Hint)
	rec.Set("intel_category", req.Category)
	rec.Set("points", req.Points)
	rec.Set("order", req.Order)
	rec.Set("unlocked", req.Unlocked)
}

// nextPuzzleCode vergibt das nächste freie Kürzel eines Typs: A1, A2, A3 …
func nextPuzzleCode(app core.App, gameID, kind string) string {
	existing, err := app.FindRecordsByFilter(
		schema.ColPuzzles, "game = {:g}", "", 0, 0,
		map[string]any{"g": gameID},
	)
	if err != nil {
		return kind + "1"
	}

	used := map[string]bool{}
	for _, p := range existing {
		used[p.GetString("code")] = true
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s%d", kind, i)
		if !used[candidate] {
			return candidate
		}
	}
}

func puzzleCodeTaken(app core.App, gameID, code, exceptID string) (bool, error) {
	if code == "" {
		return false, nil
	}
	found, err := app.FindRecordsByFilter(
		schema.ColPuzzles, "game = {:g} && code = {:c}", "", 0, 0,
		map[string]any{"g": gameID, "c": code},
	)
	if err != nil {
		return false, err
	}
	for _, p := range found {
		if p.Id != exceptID {
			return true, nil
		}
	}
	return false, nil
}
