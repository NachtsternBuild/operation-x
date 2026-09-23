// Package crypt verschlüsselt den Verkehr zwischen Geräten und Spielserver –
// unabhängig davon, ob unterwegs HTTPS im Spiel ist.
//
// Warum überhaupt, wo doch alles durch einen HTTPS-Tunnel läuft:
//
// Der Tunnel endet nicht auf dem Spielserver, sondern bei Cloudflare. Dort
// wird entschlüsselt und neu verschlüsselt – das ist der Sinn eines Tunnels,
// und es ist der Grund, warum er überhaupt funktioniert, ohne dass jemand ein
// Zertifikat beschaffen muss. Es heißt aber auch: Wer diesen Zwischenpunkt
// betreibt, sieht alles im Klartext. Bei einem Stadtspiel ist das der
// Standortverlauf von zwanzig Leuten über sechs Stunden – unter ihnen
// Jugendliche, und das Spiel läuft an ihren Wohnorten.
//
// Deshalb eine zweite Schicht, die erst auf dem Spielserver aufgeht.
//
// Was sie leistet:
//
//   - Der Tunnelbetreiber und jeder Mitleser dazwischen sehen verschlüsselte
//     Blöcke statt Koordinaten.
//   - Jede Sitzung hat ihren eigenen Schlüssel; er entsteht beim Verbinden und
//     verlässt das Gerät nie.
//
// Was sie nicht leistet, und das gehört dazu:
//
//   - Der Spielserver sieht weiterhin alles. Er muss – er ist das Spiel.
//   - Wer mitliest, sieht weiterhin, WANN jemand WIE VIEL an WELCHE Adresse
//     schickt. Verkehrsdaten bleiben sichtbar.
//   - Wer den langlebigen Serverschlüssel später erbeutet, kann damit
//     mitgeschnittenen alten Verkehr aufmachen. Für ein Spiel, dessen Daten
//     nach einem Tag gelöscht werden, ist das vertretbar.
//
// Verfahren: ECDH auf P-256 gegen den festen Schlüssel des Servers, daraus per
// HKDF-SHA256 ein Sitzungsschlüssel, damit AES-256-GCM. Alles davon steckt in
// der Go-Standardbibliothek, im Browser in der Web-Crypto-Schnittstelle und in
// Android in der Java-Kryptografie – bewusst kein eigenes Verfahren und keine
// fremde Bibliothek, die am Spieltag fehlen könnte.
//
// P-256 und nicht das modernere X25519: Letzteres können Browser erst seit
// Kurzem. Ein Spiel, bei dem die Verschlüsselung auf dem Telefon des einen
// Gastes klappt und auf dem des anderen nicht, wäre schlimmer als keine.
package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// Info bindet die abgeleiteten Schlüssel an diese Anwendung und Fassung.
// Ändert sich das Verfahren, ändert sich diese Zeichenkette – dann passen alte
// und neue Seite nicht mehr zusammen, statt sich halb zu verstehen.
const Info = "operation-x/lagefunk/1"

// NonceLen ist die Länge des Zufallswerts je Nachricht (AES-GCM Standard).
const NonceLen = 12

var (
	ErrKeyFormat  = errors.New("der öffentliche Schlüssel der Gegenstelle ist unbrauchbar")
	ErrNonceLen   = errors.New("der Zufallswert hat die falsche Länge")
	ErrNotSealed  = errors.New("die Nachricht ist nicht verschlüsselt")
	ErrOpenFailed = errors.New("die Nachricht ließ sich nicht entschlüsseln")
)

// Identity ist das dauerhafte Schlüsselpaar des Servers.
type Identity struct {
	priv *ecdh.PrivateKey
}

// NewIdentity erzeugt ein frisches Schlüsselpaar.
func NewIdentity() (*Identity, error) {
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("Schlüsselpaar erzeugen: %w", err)
	}
	return &Identity{priv: priv}, nil
}

// IdentityFromSeed stellt ein Schlüsselpaar aus gespeicherten Bytes her.
func IdentityFromSeed(raw []byte) (*Identity, error) {
	priv, err := ecdh.P256().NewPrivateKey(raw)
	if err != nil {
		return nil, fmt.Errorf("gespeicherter Schlüssel unbrauchbar: %w", err)
	}
	return &Identity{priv: priv}, nil
}

// Seed liefert die Bytes zum Speichern.
func (i *Identity) Seed() []byte { return i.priv.Bytes() }

// PublicKey liefert den öffentlichen Schlüssel in der Form, die die Geräte
// bekommen: unkomprimierte Punktdarstellung, Base64 ohne Polsterung.
func (i *Identity) PublicKey() string {
	return base64.RawURLEncoding.EncodeToString(i.priv.PublicKey().Bytes())
}

// Fingerprint ist die Kurzform zum Vorlesen und Vergleichen.
//
// Sechs Gruppen zu vier Zeichen aus dem Hash des öffentlichen Schlüssels. Wer
// wissen will, ob sein Gerät wirklich mit diesem Server spricht und nicht mit
// jemandem dazwischen, vergleicht diese Zeichen mit denen auf dem Bildschirm
// der Spielleitung. Das ist der einzige Weg, der ohne Zertifikate auskommt –
// und der einzige, der auch dann trägt, wenn der Tunnel selbst der Angreifer
// wäre.
func (i *Identity) Fingerprint() string {
	return FingerprintOf(i.PublicKey())
}

// FingerprintOf bildet den Fingerabdruck eines öffentlichen Schlüssels.
func FingerprintOf(publicKey string) string {
	sum := sha256.Sum256([]byte(publicKey))
	const alphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ" // ohne 0/O und 1/I/l

	out := make([]byte, 0, 29)
	for i := 0; i < 24; i++ {
		if i > 0 && i%4 == 0 {
			out = append(out, '-')
		}
		out = append(out, alphabet[int(sum[i])%len(alphabet)])
	}
	return string(out)
}

// Session ist der abgeleitete Schlüssel für eine Verbindung.
type Session struct {
	aead cipher.AEAD
	// Der öffentliche Schlüssel der Gegenstelle, wie er über die Leitung kam.
	Peer string
}

// Accept leitet den Sitzungsschlüssel aus dem öffentlichen Schlüssel der
// Gegenstelle ab. Das ist die Serverseite: Sie hält das dauerhafte Paar.
func (i *Identity) Accept(peerPublic string) (*Session, error) {
	raw, err := base64.RawURLEncoding.DecodeString(peerPublic)
	if err != nil {
		return nil, ErrKeyFormat
	}

	peer, err := ecdh.P256().NewPublicKey(raw)
	if err != nil {
		return nil, ErrKeyFormat
	}

	shared, err := i.priv.ECDH(peer)
	if err != nil {
		return nil, ErrKeyFormat
	}

	return newSession(shared, peerPublic, i.PublicKey())
}

// newSession leitet aus dem gemeinsamen Geheimnis den Arbeitsschlüssel ab.
//
// Das gemeinsame Geheimnis ist eine Koordinate auf einer Kurve und als
// Schlüssel ungeeignet – es ist nicht gleichverteilt. HKDF macht daraus
// gleichverteilte Bytes und bindet sie zugleich an beide Seiten: Wer denselben
// Server, aber ein anderes Gerät ist, bekommt einen anderen Schlüssel.
func newSession(shared []byte, clientPub, serverPub string) (*Session, error) {
	key := hkdf(shared, []byte(clientPub+"|"+serverPub), []byte(Info), 32)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return &Session{aead: aead, Peer: clientPub}, nil
}

// Seal verschlüsselt. Der Zufallswert wird zurückgegeben, nicht angehängt:
// Auf der Leitung steht er im Kopf der Anfrage, damit der verschlüsselte Teil
// unverändert der Rumpf bleibt.
func (s *Session) Seal(plain, zusatz []byte) (ciphertext, nonce []byte, err error) {
	nonce = make([]byte, NonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	return s.aead.Seal(nil, nonce, plain, zusatz), nonce, nil
}

// Open entschlüsselt und prüft zugleich, dass nichts verändert wurde.
//
// [zusatz] wird mitgeprüft, aber nicht mitverschlüsselt: Dort stehen Weg und
// Zähler der Anfrage. Damit lässt sich eine abgefangene Nachricht nicht auf
// einen anderen Weg umbiegen – der Text bliebe gültig, die Prüfsumme nicht.
func (s *Session) Open(ciphertext, nonce, zusatz []byte) ([]byte, error) {
	if len(nonce) != NonceLen {
		return nil, ErrNonceLen
	}
	plain, err := s.aead.Open(nil, nonce, ciphertext, zusatz)
	if err != nil {
		return nil, ErrOpenFailed
	}
	return plain, nil
}

// hkdf ist die Schlüsselableitung nach RFC 5869 mit SHA-256.
//
// Von Hand, weil es zwanzig Zeilen sind und weil dieselben zwanzig Zeilen im
// Browser und in der App noch einmal stehen müssen: Was hier passiert, muss
// dort Zeichen für Zeichen dasselbe sein, sonst passt kein Schlüssel zum
// anderen. Eine Bibliothek, die drei Sprachen gleich behandelt, gibt es nicht.
func hkdf(secret, salt, info []byte, laenge int) []byte {
	mac := hmac.New(sha256.New, salt)
	mac.Write(secret)
	prk := mac.Sum(nil)

	var out, block []byte
	for i := byte(1); len(out) < laenge; i++ {
		mac := hmac.New(sha256.New, prk)
		mac.Write(block)
		mac.Write(info)
		mac.Write([]byte{i})
		block = mac.Sum(nil)
		out = append(out, block...)
	}
	return out[:laenge]
}
