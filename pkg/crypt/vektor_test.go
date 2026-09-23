package crypt

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Ein Prüfvektor für alle vier Umsetzungen.
//
// Die zweite Verschlüsselung steht viermal im Projekt: hier, im Browser, in
// der Android-App und in der iOS-App. Laufen sie auseinander, scheitert die
// Entschlüsselung lautlos — genau das ist schon einmal passiert, als eine
// Seite den Abfrageteil der Adresse mitversiegelte und die andere nicht.
//
// Deshalb diese Datei: feste Eingaben, feste Ausgaben, in testdaten/
// abgelegt. Jede Sprache rechnet sie nach. Wer an der Ableitung dreht, bricht
// gleichzeitig vier Tests — statt erst am Spieltag einen leeren Bildschirm zu
// sehen.
//
// Neu erzeugen (wenn sich das Verfahren absichtlich ändert):
//
//	OPX_VEKTOR_SCHREIBEN=1 go test ./internal/crypt -run Vektor

type vektor struct {
	Hinweis        string `json:"hinweis"`
	GeheimnisHex   string `json:"gemeinsamesGeheimnisHex"`
	ClientPublicB64 string `json:"clientPublicKey"`
	ServerPublicB64 string `json:"serverPublicKey"`
	Info           string `json:"info"`
	SchluesselHex  string `json:"schluesselHex"`
	NonceHex       string `json:"nonceHex"`
	Beiwerk        string `json:"beiwerk"`
	Klartext       string `json:"klartext"`
	ChiffreHex     string `json:"chiffreHex"`
}

const vektorPfad = "../../testdaten/lagefunk.json"

func TestVektorLagefunk(t *testing.T) {
	// Die Eingaben sind willkürlich, aber fest. Der gemeinsame Schlüssel
	// käme im Betrieb aus ECDH; für diesen Vektor beginnt die Rechnung eine
	// Stufe später, damit keine Sprache einen privaten Schlüssel einlesen
	// muss.
	geheimnis, _ := hex.DecodeString(
		"5d3c1f9a0b7e46528a91d4c06f3b28e77c15904ad8e263b1f0a94c7d85e21306")
	clientPub := "BLsMG0jA_CB9D61E7jp7QfVOX2C8q2tN-1lfrIOMIpHUGTQ6gTquQ4f1p0mLN3ltquIEWRr5_vNCvVf3ZldbH3k"
	serverPub := "BEYZdXIIgFbm7v6uhxmgXyur6Bk-q2TaAZjNCTWSpF2NUYY_YJV2liYYzF5Gcbp3WWJnMGLe3gw3nlnAmIpUDIg"
	nonce, _ := hex.DecodeString("a1b2c3d4e5f60718293a4b5c")
	beiwerk := "POST /api/opx/position?seit=3 7"
	klartext := "Lagemeldung: 51.0504, 13.7373 — Zielperson gesichtet."

	// Genau die Schritte, die newSession geht.
	salz := []byte(clientPub + "|" + serverPub)
	schluessel := hkdf(geheimnis, salz, []byte(Info), 32)

	block, err := aes.NewCipher(schluessel)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	chiffre := aead.Seal(nil, nonce, []byte(klartext), []byte(beiwerk))

	neu := vektor{
		Hinweis: "Prüfvektor für die zweite Verschlüsselung (Lagefunk). " +
			"Dieselben Zahlen prüfen Go, Kotlin und JavaScript. " +
			"Erzeugt von internal/crypt/vektor_test.go.",
		GeheimnisHex:    hex.EncodeToString(geheimnis),
		ClientPublicB64: clientPub,
		ServerPublicB64: serverPub,
		Info:            Info,
		SchluesselHex:   hex.EncodeToString(schluessel),
		NonceHex:        hex.EncodeToString(nonce),
		Beiwerk:         beiwerk,
		Klartext:        klartext,
		ChiffreHex:      hex.EncodeToString(chiffre),
	}

	if os.Getenv("OPX_VEKTOR_SCHREIBEN") == "1" {
		roh, err := json.MarshalIndent(neu, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(vektorPfad), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(vektorPfad, append(roh, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("Prüfvektor neu geschrieben:", vektorPfad)
		return
	}

	roh, err := os.ReadFile(vektorPfad)
	if err != nil {
		t.Fatalf("Prüfvektor fehlt (%v) — mit OPX_VEKTOR_SCHREIBEN=1 erzeugen", err)
	}
	var alt vektor
	if err := json.Unmarshal(roh, &alt); err != nil {
		t.Fatal(err)
	}

	if alt.SchluesselHex != neu.SchluesselHex {
		t.Errorf("Die Schlüsselableitung hat sich geändert.\n  war:  %s\n  ist:  %s\n"+
			"Damit reden Server und Geräte aneinander vorbei, bis alle vier "+
			"Umsetzungen nachgezogen sind.", alt.SchluesselHex, neu.SchluesselHex)
	}
	if alt.ChiffreHex != neu.ChiffreHex {
		t.Errorf("Die Verschlüsselung selbst hat sich geändert.\n  war:  %s\n  ist:  %s",
			alt.ChiffreHex, neu.ChiffreHex)
	}
	if alt.Info != Info {
		t.Errorf("Das Info-Feld von HKDF hat sich geändert: %q statt %q", Info, alt.Info)
	}

	// Und die Gegenrichtung: Was im Vektor steht, muss sich öffnen lassen.
	gespeichert, err := hex.DecodeString(alt.ChiffreHex)
	if err != nil {
		t.Fatal(err)
	}
	offen, err := aead.Open(nil, nonce, gespeichert, []byte(alt.Beiwerk))
	if err != nil {
		t.Fatalf("Der gespeicherte Vektor lässt sich nicht öffnen: %v", err)
	}
	if !bytes.Equal(offen, []byte(alt.Klartext)) {
		t.Errorf("Geöffnet kam etwas anderes heraus: %q", offen)
	}

	// Ein falsches Beiwerk darf nicht durchgehen — das ist der Schutz, der
	// verhindert, dass eine abgefangene Nachricht auf einen anderen Weg
	// umgebogen wird.
	if _, err := aead.Open(nil, nonce, gespeichert, []byte("GET /woanders 7")); err == nil {
		t.Error("Mit falschem Beiwerk ließ sich die Nachricht öffnen")
	}
}
