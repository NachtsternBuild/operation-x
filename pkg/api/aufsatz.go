package api

import (
	"github.com/pocketbase/pocketbase/core"
)

// Die Stellen, an denen ein aufbauendes Programm eingreift.
//
// Dieser Server führt genau ein Spiel. Das ist keine Einschränkung, sondern
// die Voreinstellung: Ein Nachmittag, ein Laptop, eine Gruppe. Wer mehrere
// Spiele auf einem Server führen will, braucht Konten, Trennung, Fristen und
// eine Übersicht — und das gehört nicht in ein Programm, das jemand für einen
// Geburtstag anklickt.
//
// Deshalb liegt es woanders: operation-x-server bindet dieses Programm ein und
// setzt die Haken hier. Solange sie leer sind, gilt jeweils die einfache
// Antwort, und im Quelltext dieses Verzeichnisses kommt das Wort
// "Mehrspielbetrieb" nicht vor.
//
// Die Haken sind bewusst wenige und grob. Jeder einzelne steht für eine Frage,
// die ein Server mit mehreren Spielen anders beantwortet als einer mit einem.
var (
	// Zusatzrouten hängt weitere Endpunkte ein. Läuft am Ende von Register,
	// also nachdem alle eigenen Endpunkte stehen.
	Zusatzrouten func(se *core.ServeEvent)

	// StatusZusatz ergänzt die Antwort auf /api/opx/status um weitere Felder.
	// Die Anmeldeseite richtet sich danach.
	StatusZusatz func(e *core.RequestEvent) map[string]any

	// EinrichtungOffen sagt, ob die Ersteinrichtung noch aussteht. Der zweite
	// Wert sagt, ob die Antwort übernommen wird; false heißt "entscheide
	// selbst".
	EinrichtungOffen func(app core.App) (bool, bool)

	// ServerSache prüft eine Anfrage, die nicht einem Spiel gilt, sondern dem
	// ganzen Server: die Sicherung und der Weg ins Internet. Führt ein Server
	// nur ein Spiel, ist dessen Zentrale zugleich der Betreiber und darf
	// beides — dann bleibt dieser Haken leer.
	ServerSache func(e *core.RequestEvent) error

	// Rufzeichen formt den Anmeldenamen eines neuen Zugangs.
	//
	// Der Anmeldename muss auf dem ganzen Server eindeutig sein – sonst wüsste
	// die Anmeldung nicht, welches "Team_Alpha" gemeint ist. Solange es ein
	// Spiel gibt, ist das von selbst erfüllt und der Name bleibt kurz.
	Rufzeichen = func(name string, gameRec *core.Record) string { return name }

	// SpielDesServers liefert das Spiel, das eine Anfrage ohne Anmeldung
	// meint: für die Beitrittsseite, den Namen einer Sicherung, die Kacheln.
	// Wer mehrere Spiele führt, hat darauf keine Antwort und setzt hier eine
	// Fassung ein, die nichts liefert.
	SpielDesServers = currentGame
)

// serverSache ist die Innenseite von ServerSache: ohne Haken erlaubt.
func serverSache(e *core.RequestEvent) error {
	if ServerSache == nil {
		return nil
	}
	return ServerSache(e)
}

// Was ein aufbauendes Programm unter eigenen Wegen anbieten darf.
//
// Diese vier gehören nicht einem Spiel, sondern dem ganzen Server: der Weg
// ins Internet und die Sicherung. Der Kern bietet sie der Einsatzzentrale an,
// weil sie hier zugleich der Betreiber ist. Wer das trennt, hängt sie unter
// einen eigenen Weg – und braucht dafür dieselben Funktionen.
var (
	TunnelZustand    = handleTunnelState
	TunnelStarten    = handleTunnelStart
	TunnelStoppen    = handleTunnelStop
	SicherungAnlegen = handleBackup
	SicherungenListe = handleBackupList
)

// EinrichtungNoetig sagt, ob dieser Server noch nie eingerichtet wurde.
//
// Nach außen gegeben, damit ein aufbauendes Programm dieselbe Frage stellen
// kann, bevor es seine eigene Antwort darauf gibt.
func EinrichtungNoetig(app core.App) bool { return setupNeeded(app) }
