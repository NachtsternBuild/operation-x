package api

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/crypt"
)

// Der Lagefunk: eine zweite Verschlüsselung, die erst hier aufgeht.
//
// Der Tunnel endet nicht auf diesem Server, sondern bei seinem Betreiber. Dort
// wird entschlüsselt und neu verschlüsselt – deshalb funktioniert er ohne
// eigenes Zertifikat, und deshalb sieht der Betreiber den Klartext. Bei diesem
// Spiel wäre das der Standortverlauf von zwanzig Leuten über sechs Stunden.
//
// Also verschlüsseln die Geräte ihre Anfragen ein zweites Mal, mit einem
// Schlüssel, den nur sie und dieser Server kennen. Wer dazwischensitzt, sieht
// ab dann Blöcke.
//
// Drei Entscheidungen, die das Verfahren einfach halten:
//
//  1. Der Schlüssel des Servers ist dauerhaft und liegt neben der Datenbank.
//     Die Geräte bekommen ihn beim Verbinden – und in der App über den
//     QR-Code der Teamkarte, der auf dem Rechner der Spielleitung gedruckt
//     wird und damit nicht durch den Tunnel läuft.
//  2. Jedes Gerät erzeugt beim Verbinden ein eigenes, kurzlebiges Paar. Daraus
//     und aus dem Serverschlüssel entsteht ein Sitzungsschlüssel, den kein
//     zweites Gerät kennt.
//  3. Verschlüsselt wird der Rumpf, nicht die Adresse. Der Weg der Anfrage
//     geht als geprüfte Zugabe mit: Eine abgefangene Standortmeldung lässt
//     sich damit nicht auf einen anderen Weg umbiegen.
//
// Ohne Kopfzeile geht alles wie bisher im Klartext. Das ist Absicht: Die
// Weboberfläche lädt zuerst, bevor sie verschlüsseln kann, und ein Spiel darf
// nicht daran scheitern, dass ein Gerät die Verschlüsselung nicht beherrscht.

const (
	// Die Kopfzeilen der verschlüsselten Anfrage.
	headerKey   = "X-Opx-Key"   // öffentlicher Schlüssel des Geräts
	headerNonce = "X-Opx-Nonce" // Zufallswert dieser Nachricht
	headerSeq   = "X-Opx-Seq"   // laufende Nummer, gegen Wiedereinspielen
	headerType  = "X-Opx-Type"  // ursprünglicher Inhaltstyp des Rumpfes

	// Wie lange eine Sitzung im Gedächtnis bleibt. Danach beginnt die Zählung
	// von vorn – das kostet nichts außer einem neuen Eintrag.
	sessionTTL = 2 * time.Hour
)

var (
	serverIdentity *crypt.Identity

	sessions   = map[string]*sessionState{}
	sessionsMu sync.Mutex
)

type sessionState struct {
	session *crypt.Session

	// Schutz gegen Wiedereinspielen, mit Fenster.
	//
	// Eine laufende Nummer allein genügt nicht: Der Browser schickt mehrere
	// Anfragen gleichzeitig, und sie kommen in beliebiger Reihenfolge an. Mit
	// "muss größer sein als die letzte" wurde deshalb reihenweise gültiger
	// Verkehr abgewiesen – im Versuch scheiterte die Anmeldung, weil der
	// Lagestrom sich nebenher eine höhere Nummer genommen hatte.
	//
	// Also dasselbe Verfahren wie bei IPsec: die höchste gesehene Nummer plus
	// eine Maske der 64 davor. Alles Ältere und alles Doppelte fliegt raus,
	// eine verspätete Nachricht aus dem Fenster geht durch.
	lastSeq int64
	fenster uint64

	seen time.Time
}

// setupCrypto lädt den Serverschlüssel oder legt ihn an.
//
// Er liegt neben der Datenbank und überlebt damit einen Neustart: Die
// Fingerabdrücke auf den gedruckten Teamkarten sollen noch stimmen, wenn
// jemand das schwarze Fenster versehentlich geschlossen hat.
func setupCrypto(app core.App) error {
	path := filepath.Join(app.DataDir(), "lagefunk.key")

	if raw, err := os.ReadFile(path); err == nil {
		if id, err := crypt.IdentityFromSeed(raw); err == nil {
			serverIdentity = id
			return nil
		}
		// Unbrauchbar: neu erzeugen statt den Start zu verweigern. Ein Spiel
		// ohne Verschlüsselung ist besser als kein Spiel.
	}

	id, err := crypt.NewIdentity()
	if err != nil {
		return err
	}
	serverIdentity = id

	if err := os.WriteFile(path, id.Seed(), 0o600); err != nil {
		return fmt.Errorf("Schlüssel speichern: %w", err)
	}
	return nil
}

// handleKey nennt den öffentlichen Schlüssel des Servers.
//
// Ohne Anmeldung, denn ohne ihn kommt keine verschlüsselte Anmeldung zustande.
// Ein öffentlicher Schlüssel ist zum Veröffentlichen da – wer ihn kennt, kann
// nichts damit anfangen außer verschlüsselt mit diesem Server zu reden.
func handleKey(e *core.RequestEvent) error {
	if serverIdentity == nil {
		return e.JSON(http.StatusOK, map[string]any{"available": false})
	}
	return e.JSON(http.StatusOK, map[string]any{
		"available":   true,
		"publicKey":   serverIdentity.PublicKey(),
		"fingerprint": serverIdentity.Fingerprint(),
	})
}

// secureTraffic entschlüsselt eingehende und verschlüsselt ausgehende Rümpfe.
func secureTraffic(se *core.ServeEvent) {
	se.Router.BindFunc(func(e *core.RequestEvent) error {
		peer := e.Request.Header.Get(headerKey)
		if peer == "" || serverIdentity == nil {
			return e.Next()
		}

		sess, err := sessionFor(peer)
		if err != nil {
			return e.BadRequestError("Der Schlüssel des Geräts ist unbrauchbar.", err)
		}

		seq, _ := strconv.ParseInt(e.Request.Header.Get(headerSeq), 10, 64)
		if !acceptSeq(peer, seq) {
			return e.BadRequestError("Diese Nachricht ist überholt oder doppelt.", nil)
		}

		zusatz := funkZusatz(e.Request, seq)

		// Eingehend: Der Rumpf wird durch den Klartext ersetzt, bevor
		// irgendein Handler ihn sieht. Damit bleibt der ganze übrige Server
		// unverändert – er weiß von der Verschlüsselung nichts.
		if e.Request.Body != nil && e.Request.ContentLength != 0 {
			roh, err := io.ReadAll(io.LimitReader(e.Request.Body, 8<<20))
			if err != nil {
				return e.BadRequestError("Anfrage nicht lesbar.", err)
			}

			nonce, err := base64.RawURLEncoding.DecodeString(e.Request.Header.Get(headerNonce))
			if err != nil {
				return e.BadRequestError("Zufallswert unbrauchbar.", err)
			}

			klartext, err := sess.Open(roh, nonce, zusatz)
			if err != nil {
				return e.BadRequestError("Die Nachricht ließ sich nicht entschlüsseln.", err)
			}

			e.Request.Body = &wiederlesbar{Reader: bytes.NewReader(klartext)}
			e.Request.ContentLength = int64(len(klartext))

			// Der ursprüngliche Inhaltstyp muss wiederhergestellt werden, sonst
			// liest der Server ein Beweisfoto als JSON: Verschlüsselt wird auch
			// der mehrteilige Rumpf eines Bilduploads, und der wird nur
			// erkannt, wenn die Trennmarke im Typ steht.
			if typ := e.Request.Header.Get(headerType); typ != "" {
				e.Request.Header.Set("Content-Type", typ)
			} else {
				e.Request.Header.Set("Content-Type", "application/json")
			}
		}

		// Ausgehend: Die Antwort wird gesammelt und verschlüsselt
		// weitergereicht. Der Strom ist ausgenommen – er schickt über Stunden
		// und ließe sich nicht sammeln; er verschlüsselt jede Sendung selbst.
		if strings.HasSuffix(e.Request.URL.Path, "/stream") {
			e.Set(streamSessionKey, sess)
			return e.Next()
		}

		puffer := &sammelnderSchreiber{header: http.Header{}, status: http.StatusOK}
		original := e.Response
		e.Response = puffer

		err = e.Next()

		e.Response = original
		if err != nil {
			// Fehler gehen unverschlüsselt zurück: Sie entstehen im Server,
			// enthalten keine Spieldaten, und ein Gerät, das gerade nicht
			// entschlüsseln kann, soll die Begründung trotzdem lesen können.
			return err
		}

		chiffre, nonce, sealErr := sess.Seal(puffer.buf.Bytes(), zusatz)
		if sealErr != nil {
			return sealErr
		}

		for name, werte := range puffer.header {
			if name == "Content-Length" || name == "Content-Type" {
				continue
			}
			for _, v := range werte {
				e.Response.Header().Add(name, v)
			}
		}
		e.Response.Header().Set("Content-Type", "application/octet-stream")
		e.Response.Header().Set(headerNonce, base64.RawURLEncoding.EncodeToString(nonce))
		e.Response.WriteHeader(puffer.status)
		_, writeErr := e.Response.Write(chiffre)
		return writeErr
	})
}

// streamSessionKey benennt die Sitzung im Anfragekontext für den Lagestrom.
const streamSessionKey = "opx.session"

// sessionFor liefert die Sitzung zu einem Gerät und legt sie beim ersten Mal an.
func sessionFor(peer string) (*crypt.Session, error) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	if st, ok := sessions[peer]; ok {
		st.seen = time.Now()
		return st.session, nil
	}

	sess, err := serverIdentity.Accept(peer)
	if err != nil {
		return nil, err
	}

	aufraeumen()
	sessions[peer] = &sessionState{session: sess, seen: time.Now()}
	return sess, nil
}

// fensterBreite ist, wie weit eine Nachricht zurückliegen darf.
const fensterBreite = 64

// acceptSeq prüft die laufende Nummer gegen das Fenster.
func acceptSeq(peer string, seq int64) bool {
	if seq <= 0 {
		return false
	}

	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	st, ok := sessions[peer]
	if !ok {
		return true
	}

	switch {
	case seq > st.lastSeq:
		abstand := seq - st.lastSeq
		if abstand >= fensterBreite {
			st.fenster = 0
		} else {
			st.fenster <<= uint(abstand)
		}
		st.fenster |= 1
		st.lastSeq = seq
		return true

	case st.lastSeq-seq >= fensterBreite:
		// Zu alt: Entweder ein Wiedereinspielen oder eine Nachricht, die so
		// lange unterwegs war, dass sie ohnehin nichts mehr nützt.
		return false

	default:
		bit := uint64(1) << uint(st.lastSeq-seq)
		if st.fenster&bit != 0 {
			return false // schon dagewesen
		}
		st.fenster |= bit
		return true
	}
}

// aufraeumen wirft Sitzungen weg, von denen lange nichts mehr kam. Ohne das
// wüchse die Liste über einen Spieltag mit jedem Neuverbinden.
func aufraeumen() {
	grenze := time.Now().Add(-sessionTTL)
	for key, st := range sessions {
		if st.seen.Before(grenze) {
			delete(sessions, key)
		}
	}
}

// wiederlesbar ist der entschlüsselte Rumpf, den man mehrfach lesen kann.
//
// Das ist keine Bequemlichkeit, sondern Pflicht: PocketBase reicht jede
// Anfrage in einem Leser herum, der sich nach dem Ende selbst zurückspult, und
// eigene Endpunkte wie die Anmeldung lesen den Rumpf zweimal. Ein schlichter
// Leser liefert beim zweiten Mal nichts – die Anmeldung scheiterte damit mit
// "Something went wrong", ohne dass irgendwo stand, warum.
type wiederlesbar struct {
	*bytes.Reader
}

func (w *wiederlesbar) Reread() { _, _ = w.Seek(0, io.SeekStart) }

func (w *wiederlesbar) Close() error { return nil }

// sammelnderSchreiber hält die Antwort zurück, bis sie verschlüsselt ist.
type sammelnderSchreiber struct {
	buf     bytes.Buffer
	header  http.Header
	status  int
	written bool
}

func (s *sammelnderSchreiber) Header() http.Header { return s.header }

func (s *sammelnderSchreiber) WriteHeader(code int) {
	if !s.written {
		s.status = code
		s.written = true
	}
}

func (s *sammelnderSchreiber) Write(p []byte) (int, error) {
	s.written = true
	return s.buf.Write(p)
}

// funkZusatz bildet das Beiwerk, mit dem jede Nachricht versiegelt wird.
//
// Es bindet die Nachricht an ihren Platz: an die Methode, an die Adresse samt
// Abfrageteil und an die laufende Nummer. Ohne den Abfrageteil ließe sich
// unterwegs aus "?team=alpha" ein "?team=bravo" machen – die Antwort wäre dann
// sauber verschlüsselt und trotzdem die falsche.
//
// Diese Zeichenkette muss auf allen drei Seiten dieselbe sein: hier, in
// web/src/lib/funk.svelte.js und in android/.../data/Api.kt. Stimmt sie nicht
// überein, scheitert die Entschlüsselung – und zwar lautlos, weil eine
// unentschlüsselbare Antwort für den Browser einfach leer ist.
func funkZusatz(r *http.Request, seq int64) []byte {
	return []byte(r.Method + " " + r.URL.RequestURI() + " " + strconv.FormatInt(seq, 10))
}
