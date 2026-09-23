package api

import "testing"

// Der Kontostand muss dem entsprechen, was oben in der Ecke steht.
//
// Die Falle sind die Fluchtpunkte: Sie fallen nie unter null, im Kontobuch
// steht ein Abzug aber in voller Höhe. Wer stumpf aufsummiert, bekommt ein
// Konto, das dem angezeigten Guthaben widerspricht – und dann diskutieren am
// Spieltag zwei Leute über zwei Zahlen, die beide stimmen.
func TestKontostandFolgtDerGrenzeBeiNull(t *testing.T) {
	rows := []ledgerRow{
		{Reason: "Startguthaben", DeltaFP: 5},
		{Reason: "Nebelkerze", DeltaFP: -3},
		{Reason: "Zu lange still", DeltaFP: -3}, // mehr, als vorhanden ist
		{Reason: "Zwischenziel erreicht", DeltaPoints: 15, DeltaFP: 1},
	}

	entries := ledgerView(rows)
	if len(entries) != 4 {
		t.Fatalf("%d Einträge, erwartet 4", len(entries))
	}

	// Neueste zuerst.
	if entries[0].Reason != "Zwischenziel erreicht" {
		t.Errorf("oben steht %q, erwartet die jüngste Buchung", entries[0].Reason)
	}

	// Nach dem dritten Eintrag: 5 - 3 = 2, dann -3 → nicht -1, sondern 0.
	nachStillstand := entries[1]
	if nachStillstand.FP != 0 {
		t.Errorf("Guthaben nach Abzug unter null: %d, erwartet 0", nachStillstand.FP)
	}

	// Und danach wieder aufwärts von null.
	if entries[0].FP != 1 {
		t.Errorf("Guthaben nach der Belohnung: %d, erwartet 1", entries[0].FP)
	}
	if entries[0].Points != 15 {
		t.Errorf("Punktestand: %d, erwartet 15", entries[0].Points)
	}
}

func TestLeeresKontoBleibtLeer(t *testing.T) {
	if got := ledgerView(nil); len(got) != 0 {
		t.Errorf("%d Einträge aus einem leeren Kontobuch", len(got))
	}
}

// Punkte dürfen ins Minus: Wer mehr Strafe kassiert als er verdient hat, steht
// unter null, und genau das soll das Konto auch zeigen.
func TestPunkteDuerfenNegativWerden(t *testing.T) {
	entries := ledgerView([]ledgerRow{
		{Reason: "Fehlzugriff", DeltaPoints: -15},
		{Reason: "Meldung verpasst", DeltaPoints: -10},
	})

	if entries[0].Points != -25 {
		t.Errorf("Punktestand %d, erwartet -25", entries[0].Points)
	}
}
