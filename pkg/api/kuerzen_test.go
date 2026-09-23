package api

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// Kürzen muss Zeichen zählen, nicht Bytes.
//
// text[:500] schneidet Bytes ab. Trifft die Grenze mitten in ein Zeichen aus
// mehreren Bytes – und im Deutschen liegt an jeder dritten Stelle ein Umlaut –,
// entsteht ungültiges UTF-8. Aus "Königsbrücker Straße" würde dann ein
// Fragezeichen mitten im Wort.
func TestKuerzenBleibtGueltig(t *testing.T) {
	faelle := []struct {
		name string
		text string
		max  int
	}{
		{"Umlaut genau an der Grenze", strings.Repeat("a", 199) + "ü" + strings.Repeat("b", 50), 200},
		{"nur Umlaute", strings.Repeat("ö", 300), 100},
		{"Gedankenstrich", strings.Repeat("x", 199) + "–" + "yyy", 200},
		{"Emoji", strings.Repeat("x", 199) + "🙂zzz", 200},
	}

	for _, f := range faelle {
		got := Kuerzen(f.text, f.max)

		if !utf8.ValidString(got) {
			t.Errorf("%s: Ergebnis ist kein gültiges UTF-8", f.name)
		}
		if n := utf8.RuneCountInString(got); n > f.max {
			t.Errorf("%s: %d Zeichen, höchstens %d erlaubt", f.name, n, f.max)
		}
	}
}

func TestKuerzenLaesstKurzesInRuhe(t *testing.T) {
	text := "Schlange an der Kasse, Laden gerammelt voll"
	if got := Kuerzen(text, 200); got != text {
		t.Errorf("kurzer Text wurde verändert: %q", got)
	}
}

func TestKuerzenAufNullIstLeer(t *testing.T) {
	if got := Kuerzen("irgendwas", 0); got != "" {
		t.Errorf("erwartet leer, bekam %q", got)
	}
}
