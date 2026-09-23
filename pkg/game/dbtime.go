package game

import (
	"time"

	"github.com/pocketbase/pocketbase/tools/types"
)

// DBTime formatiert einen Zeitpunkt so, wie PocketBase ihn speichert.
//
// Diese Funktion ist wichtiger, als sie aussieht. Datumsfelder liegen in SQLite
// als Zeichenketten der Form "2006-01-02 15:04:05.000Z" – mit Leerzeichen
// zwischen Datum und Uhrzeit. Vergleiche in Filtern laufen deshalb als
// Zeichenkettenvergleich.
//
// Wer stattdessen RFC3339 einsetzt ("2006-01-02T15:04:05Z"), vergleicht an
// Position elf ein Leerzeichen mit einem "T". Da das Leerzeichen kleiner ist,
// ist *jeder* gespeicherte Wert kleiner als der Filterwert – und zwar immer,
// unabhängig von der tatsächlichen Zeit. Bedingungen wie "expires_at > jetzt"
// treffen dann nie zu und "deadline_at <= jetzt" immer.
//
// In dieser Anwendung hieß das konkret: Sperren wirkten nie, und jede laufende
// Mission galt sofort als verfehlt. Für Filter also ausschließlich diese
// Funktion verwenden; für JSON-Antworten an die Clients bleibt RFC3339 richtig.
func DBTime(t time.Time) string {
	// ParseDateTime nimmt eine time.Time entgegen und liefert die von
	// PocketBase verwendete Darstellung. Ein Fehler kann dabei nicht auftreten;
	// falls doch, ist der Nullwert die sichere Antwort – ein leerer Filterwert
	// findet nichts, statt versehentlich alles zu treffen.
	dt, err := types.ParseDateTime(t)
	if err != nil {
		return ""
	}
	return dt.String()
}
