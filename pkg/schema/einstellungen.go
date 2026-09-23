package schema

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// Zwei Voreinstellungen von PocketBase, die für dieses Spiel nicht taugen.
//
// PocketBase ist als Baukasten für Anwendungen gedacht und stellt sich
// entsprechend neutral ein. Dieses Spiel ist keine neutrale Anwendung: Es
// läuft einen Tag lang, es verarbeitet die Standorte von Leuten, die oft noch
// keine achtzehn sind, und es steht am Ende an einer öffentlichen Adresse im
// Netz. Beides wird deshalb bei jedem Start festgelegt, nicht der Einstellung
// überlassen.
func EnsureSettings(app core.App) error {
	s := app.Settings()
	geaendert := false

	// Keine IP-Adressen im Protokoll.
	//
	// PocketBase schreibt sie ab Werk zu jeder Anfrage mit und hält sie fünf
	// Tage. Eine IP-Adresse ist ein personenbezogenes Datum; für dieses Spiel
	// wird sie nirgends gebraucht — weder für die Punkte noch für die
	// Fehlersuche. Was man nicht braucht, speichert man nicht (Art. 5 Abs. 1
	// lit. c DSGVO).
	if s.Logs.LogIP {
		s.Logs.LogIP = false
		geaendert = true
	}
	if s.Logs.LogAuthId {
		s.Logs.LogAuthId = false
		geaendert = true
	}

	// Eine Grenze gegen das Durchprobieren von Kennwörtern.
	//
	// Die Kennwörter dieses Spiels sind sprechbar, damit man sie über Funk
	// durchgeben kann — rund 81.000 Möglichkeiten. Ohne Begrenzung wäre das
	// in wenigen Stunden durchprobiert, und ein Spieltag dauert sechs. Mit
	// zwanzig Versuchen je Minute und Adresse dauert es Tage; eine Gruppe,
	// die sich zeitgleich über denselben Hotspot anmeldet, merkt davon
	// nichts.
	//
	// Die Kacheln brauchen eine eigene, weite Regel: Wer die Karte
	// verschiebt, holt dreißig Bilder auf einmal, und zehn Geräte hinter
	// einem Anschluss sind der Normalfall. Die spezifischere Regel muss
	// zuerst stehen, es gewinnt die erste passende.
	if !s.RateLimits.Enabled {
		s.RateLimits.Enabled = true
		geaendert = true
	}
	regeln := []core.RateLimitRule{
		{Label: "*:auth", MaxRequests: 20, Duration: 60},
		{Label: "/api/opx/tiles/", MaxRequests: 2000, Duration: 10},
		{Label: "/api/", MaxRequests: 600, Duration: 10},
	}
	if !gleicheRegeln(s.RateLimits.Rules, regeln) {
		s.RateLimits.Rules = regeln
		geaendert = true
	}

	if !geaendert {
		return nil
	}
	if err := app.Save(s); err != nil {
		return fmt.Errorf("Servereinstellungen speichern: %w", err)
	}
	return nil
}

// EntferneBeispielsammlung nimmt die Sammlung "users" weg.
//
// PocketBase legt sie bei einer neuen Datenbank als Beispiel an, mit
// Selbstbedienungsregeln: Jeder darf sich anlegen, jeder sieht und ändert
// seinen eigenen Eintrag. Für einen Baukasten ist das ein freundlicher
// Einstieg, für diesen Server eine offene Registrierung, die niemand braucht
// — dieses Spiel kennt Teams, keine "users".
//
// Enthält sie wider Erwarten Daten, wird sie nicht gelöscht, sondern
// zugesperrt. Ein Programm, das beim Start fremde Daten wegräumt, wäre
// schlimmer als das Problem.
func EntferneBeispielsammlung(app core.App) error {
	col, err := app.FindCollectionByNameOrId("users")
	if err != nil || col == nil || col.System {
		return nil // gibt es nicht – der Normalfall nach dem ersten Start
	}

	anzahl, err := app.CountRecords("users")
	if err != nil {
		return fmt.Errorf("users zählen: %w", err)
	}

	if anzahl == 0 {
		if err := app.Delete(col); err != nil {
			return fmt.Errorf("users entfernen: %w", err)
		}
		return nil
	}

	app.Logger().Warn("Die Sammlung users enthält Daten und wird nur gesperrt",
		"datensaetze", anzahl)
	col.ListRule = nil
	col.ViewRule = nil
	col.CreateRule = nil
	col.UpdateRule = nil
	col.DeleteRule = nil
	if err := app.Save(col); err != nil {
		return fmt.Errorf("users sperren: %w", err)
	}
	return nil
}

func gleicheRegeln(a, b []core.RateLimitRule) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Label != b[i].Label || a[i].MaxRequests != b[i].MaxRequests ||
			a[i].Duration != b[i].Duration || a[i].Audience != b[i].Audience {
			return false
		}
	}
	return true
}
