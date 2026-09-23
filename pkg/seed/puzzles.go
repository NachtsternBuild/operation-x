package seed

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/game"
	"github.com/elias/operation-x/pkg/schema"
)

// Beispielrätsel für Dresden, einer je Typ des Regelwerks.
//
// Sie sind als Vorlage gedacht, nicht als fertiger Satz: Gute Rätsel kennen die
// Stadt und die Gruppe, und beides weiß nur die Spielleitung. Mehrere zulässige
// Schreibweisen werden mit einem senkrechten Strich getrennt.
var testPuzzles = []struct {
	Code     string
	Type     string
	Title    string
	Question string
	Answer   string
	Hint     string
	Category string
	Points   int
	Unlocked bool
}{
	{
		Code:     "A1",
		Type:     "A",
		Title:    "Ausschlussverfahren",
		Question: "Der gesuchte Hotspot liegt nördlich der Elbe, seine Nummer ist zweistellig und durch drei teilbar. An diesem Ort steht ein Reiterstandbild aus vergoldetem Kupfer. Wie heißt der Ort?",
		Answer:   "Goldener Reiter|Der Goldene Reiter",
		Hint:     "Neustädter Seite, direkt an der Augustusbrücke.",
		Category: game.IntelYellow,
		Points:   15,
		Unlocked: true,
	},
	{
		Code:     "B1",
		Type:     "B",
		Title:    "Fassadendetail",
		Question: "Mister X hat ein Nahaufnahmefoto hochgeladen: glasierte Keramikfliesen mit orientalischem Muster, dahinter ein Innenhof im maurischen Stil. In welchem Gebäude wurde das aufgenommen?",
		Answer:   "Pfunds Molkerei|Pfunds|Molkerei Pfund",
		Hint:     "Bautzner Straße. Gilt als schönster Milchladen der Welt.",
		Category: game.IntelGreen,
		Points:   15,
	},
	{
		Code:     "C1",
		Type:     "C",
		Title:    "Zahlenschloss",
		Question: "Die Frauenkirche wurde 1743 vollendet, 1945 zerstört und 2005 geweiht. Addiere die drei Jahreszahlen und bilde vom Ergebnis die Quersumme.",
		Answer:   "24",
		Hint:     "1743 + 1945 + 2005 = 5693. Und davon die Quersumme.",
		Category: game.IntelRed,
		Points:   15,
	},
	{
		Code:     "D1",
		Type:     "D",
		Title:    "Kartengeometrie",
		Question: "Gehe vom Zwinger aus rund 700 Meter nach Osten. Du stehst vor einem Bauwerk mit einer steinernen Kuppel, das über Jahrzehnte als Ruine dalag. Welches ist es?",
		Answer:   "Frauenkirche|Dresdner Frauenkirche",
		Hint:     "Am Neumarkt.",
		Category: game.IntelYellow,
		Points:   15,
	},
	{
		Code:     "E1",
		Type:     "E",
		Title:    "Vor Ort nachsehen",
		Question: "Am Fürstenzug an der Augustusstraße: Aus welchem Material bestehen die Kacheln, aus denen das Wandbild zusammengesetzt ist?",
		Answer:   "Meissner Porzellan|Porzellan|Meißner Porzellan",
		Hint:     "Sachsens berühmtestes Erzeugnis.",
		Category: game.IntelGreen,
		Points:   15,
	},
	{
		Code:     "F1",
		Type:     "F",
		Title:    "Zusammenführung",
		Question: "Nimm den Anfangsbuchstaben der Lösung von A1, den Ort aus B1 und die Zahl aus C1. Der gesuchte Hotspot liegt in demselben Sektor wie B1 und trägt im Namen eine Farbe. Wie heißt er?",
		Answer:   "Blaues Wunder|Das Blaue Wunder",
		Hint:     "Eine Brücke, die eigentlich Loschwitzer Brücke heißt.",
		Category: game.IntelRed,
		Points:   25,
	},
}

// AddPuzzles legt die Beispielrätsel für das laufende Spiel an.
func AddPuzzles(app core.App) error {
	gameRec, err := currentGame(app)
	if err != nil {
		return err
	}

	col, err := app.FindCollectionByNameOrId(schema.ColPuzzles)
	if err != nil {
		return err
	}

	existing, err := app.FindRecordsByFilter(schema.ColPuzzles, "game = {:g}", "", 0, 0,
		map[string]any{"g": gameRec.Id})
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return fmt.Errorf("für dieses Spiel sind bereits %d Rätsel angelegt", len(existing))
	}

	return app.RunInTransaction(func(tx core.App) error {
		for i, p := range testPuzzles {
			rec := core.NewRecord(col)
			rec.Set("game", gameRec.Id)
			rec.Set("code", p.Code)
			rec.Set("type", p.Type)
			rec.Set("title", p.Title)
			rec.Set("question", p.Question)
			rec.Set("answer", p.Answer)
			rec.Set("hint", p.Hint)
			rec.Set("intel_category", p.Category)
			rec.Set("points", p.Points)
			rec.Set("unlocked", p.Unlocked)
			rec.Set("order", i+1)

			if err := tx.Save(rec); err != nil {
				return fmt.Errorf("Rätsel %s anlegen: %w", p.Code, err)
			}
		}

		fmt.Printf("\n%d Rätsel angelegt (Typ A bis F).\n", len(testPuzzles))
		fmt.Println("Freigeschaltet ist zunächst nur A1 – der Rest wird im HQ nachgezogen.")
		fmt.Println()
		return nil
	})
}
