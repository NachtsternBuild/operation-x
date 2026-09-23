package hub

import (
	"sync"
	"testing"
	"time"
)

// Der Hub ist die Stelle, an der aus "etwas ist passiert" ein Bildschirm wird,
// der sich aktualisiert. Fällt eine Meldung aus, merkt es niemand sofort — die
// Zentrale sieht nur eine Lage, die stehenbleibt.

func TestZuhoererBekommtBescheid(t *testing.T) {
	h := New()
	ch, ab := h.Subscribe()
	defer ab()

	h.Notify()

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("Die Meldung kam nicht an")
	}
}

// Mehrere Meldungen zwischen zwei Abrufen dürfen zu einer zusammenfallen —
// aber nicht verlorengehen und den Sender auch nicht blockieren.
func TestMeldungenFallenZusammen(t *testing.T) {
	h := New()
	ch, ab := h.Subscribe()
	defer ab()

	for i := 0; i < 50; i++ {
		h.Notify()
	}

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("Nach fünfzig Meldungen kam keine an")
	}

	select {
	case <-ch:
		t.Error("Es kam eine zweite Meldung — der Puffer soll genau eine halten")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestAlleZuhoererWerdenErreicht(t *testing.T) {
	h := New()

	kanaele := make([]<-chan struct{}, 5)
	for i := range kanaele {
		ch, ab := h.Subscribe()
		defer ab()
		kanaele[i] = ch
	}

	h.Notify()

	for i, ch := range kanaele {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Errorf("Zuhörer %d ging leer aus", i)
		}
	}
}

// Nach dem Abmelden darf keine Meldung mehr kommen, und vor allem darf der
// Sender nicht auf einen geschlossenen Kanal schreiben.
func TestAbgemeldeteStoerenNicht(t *testing.T) {
	h := New()
	_, ab := h.Subscribe()
	bleibt, abBleibt := h.Subscribe()
	defer abBleibt()

	ab()
	ab() // zweimal abmelden darf nichts kaputtmachen

	h.Notify()

	select {
	case <-bleibt:
	case <-time.After(time.Second):
		t.Fatal("Der verbliebene Zuhörer bekam nichts")
	}
}

// Die Engine meldet aus ihrem Takt, die Endpunkte hören aus ihren Anfragen:
// beides gleichzeitig, aus vielen Goroutinen. Ohne Sperre gäbe das ein Rennen.
func TestGleichzeitigesAnUndAbmelden(t *testing.T) {
	h := New()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			ch, ab := h.Subscribe()
			<-time.After(time.Millisecond)
			select {
			case <-ch:
			default:
			}
			ab()
		}()
		go func() {
			defer wg.Done()
			h.Notify()
		}()
	}
	wg.Wait()
}
