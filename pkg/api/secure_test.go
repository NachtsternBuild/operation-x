package api

import (
	"net/http/httptest"
	"testing"
)

// Der Schutz gegen Wiedereinspielen muss zwei Dinge zugleich können: dieselbe
// Nachricht zweimal ablehnen und gleichzeitig eintreffende Anfragen in
// beliebiger Reihenfolge annehmen.
//
// Eine einfache "muss größer sein als die letzte"-Regel kann nur das Erste.
// Der Browser schickt mehrere Anfragen parallel, sie kommen durcheinander an –
// im Versuch scheiterte damit die Anmeldung, weil der Lagestrom sich nebenher
// eine höhere Nummer genommen hatte.
func TestWiedereinspielfenster(t *testing.T) {
	sessionsMu.Lock()
	sessions["prüfling"] = &sessionState{}
	sessionsMu.Unlock()
	defer func() {
		sessionsMu.Lock()
		delete(sessions, "prüfling")
		sessionsMu.Unlock()
	}()

	// Der Reihe nach: alles neu, alles angenommen.
	for i := int64(1); i <= 5; i++ {
		if !acceptSeq("prüfling", i) {
			t.Fatalf("Nummer %d wurde abgelehnt", i)
		}
	}

	// Dieselbe noch einmal: abgelehnt.
	for i := int64(1); i <= 5; i++ {
		if acceptSeq("prüfling", i) {
			t.Errorf("Nummer %d wurde zweimal angenommen", i)
		}
	}

	// Durcheinander: 9 vor 7 vor 8 – alle drei gültig.
	for _, n := range []int64{9, 7, 8} {
		if !acceptSeq("prüfling", n) {
			t.Errorf("Nummer %d wurde abgelehnt, obwohl sie neu ist", n)
		}
	}
	if acceptSeq("prüfling", 8) {
		t.Error("die verspätete 8 wurde zweimal angenommen")
	}

	// Weit in der Zukunft: gültig, und schließt das alte Fenster.
	if !acceptSeq("prüfling", 5000) {
		t.Error("ein großer Sprung wurde abgelehnt")
	}
	if acceptSeq("prüfling", 9) {
		t.Error("eine Nachricht weit außerhalb des Fensters wurde angenommen")
	}

	if acceptSeq("prüfling", 0) || acceptSeq("prüfling", -3) {
		t.Error("eine Nummer ohne Sinn wurde angenommen")
	}
}

// Das Beiwerk muss den Abfrageteil enthalten.
//
// Ohne ihn stimmte es nicht mit dem überein, was die Weboberfläche bildet: Sie
// nimmt den Pfad, den sie an fetch übergibt, und der trägt die Frage mit. Die
// Folge war eine leere Antwort ohne Fehlermeldung – die Stadtsuche und das
// Punktekonto eines fremden Teams gaben nichts zurück.
func TestFunkZusatzNimmtDieFrageMit(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/opx/setup/cities?q=Dresden", nil)

	if got, want := string(funkZusatz(r, 7)), "GET /api/opx/setup/cities?q=Dresden 7"; got != want {
		t.Errorf("Beiwerk = %q, erwartet %q", got, want)
	}
}
