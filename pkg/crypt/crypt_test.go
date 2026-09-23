package crypt

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
)

// Der Normalfall: Ein Gerät verbindet sich, schickt etwas, bekommt etwas
// zurück – und dazwischen steht nichts Lesbares auf der Leitung.
func TestHinUndZurueck(t *testing.T) {
	server, err := NewIdentity()
	if err != nil {
		t.Fatal(err)
	}

	client, geraet := verbinde(t, server)

	meldung := []byte(`{"lat":51.0504,"lng":13.7373}`)
	zusatz := []byte("POST /api/opx/position 7")

	chiffre, nonce, err := geraet.Seal(meldung, zusatz)
	if err != nil {
		t.Fatal(err)
	}

	// Auf der Leitung darf von den Koordinaten nichts zu sehen sein.
	if bytes.Contains(chiffre, []byte("51.05")) {
		t.Error("die Koordinaten stehen im Klartext in der Nachricht")
	}
	if len(chiffre) <= len(meldung) {
		t.Error("die verschlüsselte Nachricht ist nicht länger als der Klartext – kein Prüfanhang?")
	}

	zurueck, err := client.Open(chiffre, nonce, zusatz)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(zurueck, meldung) {
		t.Errorf("zurück kam %q", zurueck)
	}
}

// Wer den Weg der Anfrage ändert, bekommt keine gültige Nachricht mehr.
//
// Ohne diese Bindung ließe sich eine abgefangene Standortmeldung an einen
// anderen Weg schicken – der Text bliebe gültig, und der Server würde ihn
// annehmen.
func TestVeraenderterWegFliegtAuf(t *testing.T) {
	server, _ := NewIdentity()
	client, geraet := verbinde(t, server)

	chiffre, nonce, _ := geraet.Seal([]byte("geheim"), []byte("POST /api/opx/position 1"))

	if _, err := client.Open(chiffre, nonce, []byte("POST /api/opx/arrest 1")); err == nil {
		t.Error("die Nachricht wurde auf einem anderen Weg angenommen")
	}
}

func TestVeraenderteNachrichtFliegtAuf(t *testing.T) {
	server, _ := NewIdentity()
	client, geraet := verbinde(t, server)

	chiffre, nonce, _ := geraet.Seal([]byte("geheim"), nil)
	chiffre[3] ^= 0x40 // ein Bit kippen

	if _, err := client.Open(chiffre, nonce, nil); err == nil {
		t.Error("eine veränderte Nachricht wurde angenommen")
	}
}

// Zwei Geräte bekommen zwei verschiedene Schlüssel – sonst könnte ein Team das
// andere mitlesen, und das wäre schlimmer als gar keine Verschlüsselung.
func TestJedesGeraetHatSeinenEigenenSchluessel(t *testing.T) {
	server, _ := NewIdentity()
	einsServer, eins := verbinde(t, server)
	_, zwei := verbinde(t, server)

	chiffre, nonce, _ := zwei.Seal([]byte("Standort von Team Zwei"), nil)

	if _, err := einsServer.Open(chiffre, nonce, nil); err == nil {
		t.Error("Team Eins konnte die Nachricht von Team Zwei lesen")
	}
	_ = eins
}

func TestUnbrauchbarerSchluesselWirdAbgelehnt(t *testing.T) {
	server, _ := NewIdentity()

	for _, fall := range []string{"", "kein base64!!", base64.RawURLEncoding.EncodeToString([]byte("zu kurz"))} {
		if _, err := server.Accept(fall); err == nil {
			t.Errorf("Schlüssel %q wurde angenommen", fall)
		}
	}
}

// Der Fingerabdruck ist zum Vorlesen da: gleiche Eingabe, gleiche Ausgabe,
// und keine Zeichen, die man am Telefon verwechselt.
func TestFingerabdruck(t *testing.T) {
	server, _ := NewIdentity()
	fp := server.Fingerprint()

	if fp != server.Fingerprint() {
		t.Error("zwei Aufrufe, zwei Ergebnisse")
	}
	if strings.ContainsAny(fp, "01OIl") {
		t.Errorf("verwechselbare Zeichen im Fingerabdruck: %q", fp)
	}
	if n := len(strings.ReplaceAll(fp, "-", "")); n != 24 {
		t.Errorf("%d Zeichen, erwartet 24: %q", n, fp)
	}

	anderer, _ := NewIdentity()
	if anderer.Fingerprint() == fp {
		t.Error("zwei Server, derselbe Fingerabdruck")
	}
}

// Ein gespeichertes Schlüsselpaar muss nach einem Neustart dasselbe sein –
// sonst müssten alle Geräte neu verbinden, und die gedruckten Teamkarten
// stimmten nicht mehr.
func TestSchluesselUeberlebtDenNeustart(t *testing.T) {
	erst, _ := NewIdentity()
	wieder, err := IdentityFromSeed(erst.Seed())
	if err != nil {
		t.Fatal(err)
	}

	if wieder.PublicKey() != erst.PublicKey() {
		t.Error("nach dem Laden ein anderer öffentlicher Schlüssel")
	}
	if wieder.Fingerprint() != erst.Fingerprint() {
		t.Error("nach dem Laden ein anderer Fingerabdruck")
	}
}

// verbinde spielt, was ein Gerät beim Verbinden tut: eigenes Paar erzeugen,
// gemeinsames Geheimnis bilden. Liefert die Sitzung beider Seiten.
func verbinde(t *testing.T, server *Identity) (serverSeite, geraetSeite *Session) {
	t.Helper()

	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub := base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes())

	serverSeite, err = server.Accept(pub)
	if err != nil {
		t.Fatal(err)
	}

	rohServer, _ := base64.RawURLEncoding.DecodeString(server.PublicKey())
	serverPub, _ := ecdh.P256().NewPublicKey(rohServer)
	shared, err := priv.ECDH(serverPub)
	if err != nil {
		t.Fatal(err)
	}

	geraetSeite, err = newSession(shared, pub, server.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	return serverSeite, geraetSeite
}
