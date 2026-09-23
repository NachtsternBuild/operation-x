// Package hub sagt Bescheid, wenn sich an der Lage etwas geändert hat.
//
// Bisher fragte jeder Client im Takt von vier bis zwanzig Sekunden nach, ob es
// etwas Neues gibt. Das funktioniert, hat aber zwei Kosten, die genau dort
// anfallen, wo beides knapp ist: Mobilfunkdaten und Akku. Acht Telefone, die
// sechs Stunden lang alle fünf Sekunden fragen, sind rund 35.000 Anfragen für
// ein Spiel, in dem vielleicht zweihundertmal wirklich etwas passiert.
//
// Umgekehrt herum ist es billiger: Die Verbindung bleibt offen, und der Server
// schickt, wenn es etwas zu schicken gibt.
//
// Das Paket steht bewusst für sich. Die Spiel-Engine muss melden können, dass
// sie etwas gebucht hat, und die Endpunkte müssen darauf warten können – ohne
// dass eines der beiden das andere kennt.
package hub

import "sync"

// Hub verteilt Änderungsmeldungen an alle Zuhörer.
type Hub struct {
	mu        sync.Mutex
	listeners map[int]chan struct{}
	next      int
}

func New() *Hub {
	return &Hub{listeners: map[int]chan struct{}{}}
}

// Default ist der Hub des laufenden Servers.
//
// Ein Paket-Zustand, weil es genau einen Server je Prozess gibt und die
// Alternative – ihn durch jede Funktionssignatur zu reichen – die Engine und
// jeden Endpunkt mit einem Argument belasten würde, das sie sonst nichts
// angeht. Gemeldet wird nur "es hat sich etwas geändert"; was, holt sich jeder
// Zuhörer selbst über seine eigenen Endpunkte – und die filtern.
var Default = New()

// Subscribe meldet einen Zuhörer an und liefert seinen Kanal samt Abmelder.
func (h *Hub) Subscribe() (<-chan struct{}, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	id := h.next
	h.next++

	// Puffer von eins: Mehrere Meldungen zwischen zwei Abrufen fallen zu einer
	// zusammen. Der Zuhörer holt sich ohnehin den aktuellen Stand – ob sich
	// dreimal oder einmal etwas geändert hat, ändert daran nichts.
	ch := make(chan struct{}, 1)
	h.listeners[id] = ch

	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if c, ok := h.listeners[id]; ok {
			delete(h.listeners, id)
			close(c)
		}
	}
}

// Notify meldet allen Zuhörern, dass sich etwas geändert hat.
//
// Blockiert nie: Ein Zuhörer, dessen Puffer voll ist, weiß bereits Bescheid.
// Eine Meldung darf niemals die Spiel-Engine aufhalten.
func (h *Hub) Notify() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, ch := range h.listeners {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Listeners sagt, wie viele Verbindungen gerade offen sind. Für die Anzeige in
// der Einsatzzentrale und für Tests.
func (h *Hub) Listeners() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.listeners)
}

// Notify auf dem Standard-Hub.
func Notify() { Default.Notify() }

// Subscribe auf dem Standard-Hub.
func Subscribe() (<-chan struct{}, func()) { return Default.Subscribe() }
