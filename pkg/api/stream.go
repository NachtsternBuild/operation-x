package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/crypt"
	"github.com/elias/operation-x/pkg/hub"
	"github.com/elias/operation-x/pkg/schema"
)

// Der Lagestrom.
//
// Statt dass jedes Gerät im Takt nachfragt, bleibt die Verbindung offen und der
// Server schickt, wenn es etwas zu schicken gibt. Acht Telefone, die sechs
// Stunden lang alle fünf Sekunden fragen, wären rund 35.000 Anfragen für ein
// Spiel, in dem vielleicht zweihundertmal wirklich etwas passiert – und zwar
// über Mobilfunk, mit dem Funkgerät des Telefons als größtem Stromverbraucher
// nach dem Bildschirm.
//
// Bewusst Server-Sent Events und nicht WebSocket: Der Verkehr fließt hier nur
// in eine Richtung, SSE ist gewöhnliches HTTP und kommt damit durch jeden
// Tunnel und jeden Zwischenspeicher, und ein Verbindungsabbruch wird vom
// Protokoll selbst behandelt statt von uns.

const (
	// Wie lange höchstens Ruhe herrschen darf, bevor ein Lebenszeichen geht.
	//
	// Zwischenstellen und Mobilfunknetze schließen eine stille Verbindung nach
	// einer Weile. Fünfzehn Sekunden liegen deutlich unter jeder üblichen
	// Grenze und kosten fast nichts – ein Doppelpunkt ist ein gültiger
	// SSE-Kommentar und wird vom Client verworfen.
	streamHeartbeat = 15 * time.Second

	// Kürzester Abstand zwischen zwei Sendungen an denselben Client.
	//
	// Ohne das würde eine Kette von Buchungen – die Engine bucht bei einem
	// Verstoß mehrere Ereignisse hintereinander – ebenso viele Sendungen
	// auslösen. Eine Sekunde später wäre die Lage dieselbe.
	streamMinInterval = 900 * time.Millisecond

	// Wann der Strom von sich aus endet.
	//
	// Nicht aus Sparsamkeit, sondern weil eine Verbindung, die stundenlang
	// offen steht, irgendwann an einer Zwischenstelle hängt, ohne dass eine
	// der beiden Seiten es merkt. Der Client verbindet danach neu – das
	// kostet ihn eine Anfrage je Stunde.
	streamMaxAge = time.Hour

	// Die geprüfte Zugabe jeder Sendung im Strom. Sie bindet den verschlüsselten
	// Text an diesen Weg: Eine abgefangene Lage lässt sich damit nicht als
	// Antwort auf etwas anderes ausgeben.
	streamZusatz = "GET /api/opx/stream"
)

// handleStream hält die Verbindung offen und schickt die Lage bei Änderungen.
func handleStream(e *core.RequestEvent) error {
	me := e.Auth
	if me == nil {
		return e.UnauthorizedError("Nicht angemeldet.", nil)
	}

	flusher, ok := e.Response.(http.Flusher)
	if !ok {
		// Ohne Flusher käme alles erst am Ende an, und das Ende kommt nie.
		// Dann ist die Abfrage der ehrlichere Weg.
		return e.InternalServerError("Dieser Server kann keinen Strom liefern.", nil)
	}

	h := e.Response.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	// Verhindert, dass ein Zwischenspeicher die Antwort sammelt, statt sie
	// durchzureichen – bei nginx die übliche Ursache für einen Strom, der
	// erst nach Minuten ankommt.
	h.Set("X-Accel-Buffering", "no")
	e.Response.WriteHeader(http.StatusOK)

	changed, unsubscribe := hub.Subscribe()
	defer unsubscribe()

	ctx := e.Request.Context()
	deadline := time.NewTimer(streamMaxAge)
	defer deadline.Stop()

	heartbeat := time.NewTicker(streamHeartbeat)
	defer heartbeat.Stop()

	var lastPayload string
	var lastSent time.Time

	// Läuft die Verbindung verschlüsselt, wird jede Sendung einzeln
	// verschlüsselt: Der Strom steht über Stunden offen und lässt sich nicht
	// als Ganzes einpacken. Die Lage ist der heikelste Inhalt überhaupt – hier
	// stehen die Positionen aller sichtbaren Teams.
	var funk *crypt.Session
	if s, ok := e.Get(streamSessionKey).(*crypt.Session); ok {
		funk = s
	}

	send := func(force bool) error {
		// Der eigene Datensatz muss frisch aus der Datenbank kommen.
		//
		// e.Auth ist die Kopie vom Verbindungsaufbau, und diese Verbindung
		// steht eine Stunde. Alles, was auf dem Team gespeichert ist – Punkte,
		// Fluchtpunkte, die nächste Meldefrist, Verstöße, Transit –, bliebe
		// darin für diese Stunde eingefroren. Der Strom hätte dann genau das
		// nicht mehr geliefert, wofür es ihn gibt: Auf dem Gerät stand die
		// Frist von vorhin, während der Server längst eine neue kannte.
		self, err := e.App.FindRecordById(schema.ColTeams, me.Id)
		if err != nil {
			// Team gelöscht oder Datenbank weg: Der Strom endet, der Client
			// verbindet neu und landet bei der Anmeldung.
			return err
		}

		res, err := buildLive(e.App, self, time.Now())
		if err != nil {
			return err
		}

		raw, err := json.Marshal(res)
		if err != nil {
			return err
		}

		if !force {
			key := changeKey(res)
			if key == lastPayload {
				return nil
			}
			lastPayload = key
		} else {
			lastPayload = changeKey(res)
		}
		lastSent = time.Now()

		if funk != nil {
			chiffre, nonce, err := funk.Seal(raw, []byte(streamZusatz))
			if err != nil {
				return err
			}
			// Zufallswert und Text in einem Stück: Ein Ereignis des Stroms ist
			// eine Zeile, und eine zweite Kopfzeile gibt es dafür nicht.
			raw = []byte(base64.RawURLEncoding.EncodeToString(append(nonce, chiffre...)))
		}

		if _, err := fmt.Fprintf(e.Response, "event: live\ndata: %s\n\n", raw); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	// Der erste Stand geht sofort raus: Der Client soll nicht auf die erste
	// Änderung warten müssen, um überhaupt etwas anzuzeigen.
	if err := send(true); err != nil {
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-deadline.C:
			// Freundlich verabschieden, damit der Client weiß, dass er neu
			// verbinden soll, statt auf einen toten Kanal zu starren.
			fmt.Fprint(e.Response, "event: bye\ndata: {}\n\n")
			flusher.Flush()
			return nil

		case <-changed:
			if wait := streamMinInterval - time.Since(lastSent); wait > 0 {
				select {
				case <-time.After(wait):
				case <-ctx.Done():
					return nil
				}
			}
			if err := send(false); err != nil {
				return nil
			}

		case <-heartbeat.C:
			if _, err := fmt.Fprint(e.Response, ": weiter\n\n"); err != nil {
				return nil
			}
			flusher.Flush()
		}
	}
}

// changeKey macht aus der Lage einen Vergleichswert, der nur echte Änderungen
// abbildet.
//
// Ohne ihn wäre der Strom kaum besser als das Nachfragen: Die Antwort enthält
// die aktuelle Uhrzeit sowie Alter und Restfristen in Sekunden, und die ändern
// sich jede Sekunde von selbst. Jede Buchung im Spiel löste dann eine Sendung
// aus, obwohl sich für dieses Team nichts geändert hat.
//
// Herausgenommen wird genau das, was der Client aus dem übrigen Inhalt selbst
// ausrechnen kann: Aus capturedAt folgt das Alter, aus nextDueAt die Restfrist.
// Deshalb bleiben diese beiden drin – sie sind die Wahrheit, die Sekundenwerte
// nur ihre Ableitung.
func changeKey(res *liveResponse) string {
	copy := *res

	copy.Now = ""
	copy.Self.DueInSec = 0

	// Auch die Restzeit der Pause zählt sich von selbst herunter; sie ist aus
	// "until" ableitbar und würde sonst jede Sekunde eine Sendung auslösen.
	if copy.Pause != nil {
		ohneRest := *copy.Pause
		ohneRest.LeftSec = 0
		copy.Pause = &ohneRest
	}

	positions := make([]livePosition, len(res.Positions))
	for i, p := range res.Positions {
		p.AgeSec = 0
		positions[i] = p
	}
	copy.Positions = positions

	lockouts := make([]liveLockout, len(res.Self.Lockouts))
	for i, l := range res.Self.Lockouts {
		l.LeftSec = 0
		lockouts[i] = l
	}
	copy.Self.Lockouts = lockouts

	raw, err := json.Marshal(&copy)
	if err != nil {
		// Im Zweifel senden: Eine überflüssige Sendung ist harmlos, eine
		// unterschlagene Änderung nicht.
		return ""
	}
	return string(raw)
}
