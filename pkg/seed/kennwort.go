package seed

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// Sprechbare Kennwörter.
//
// Am Spieltag wird ein Kennwort vorgelesen, über Funk durchgegeben und auf
// einem Telefon abgetippt – meist im Stehen, oft bei Sonne. Zufallszeichen
// scheitern an genau dieser Kette. Die Wörter sind bewusst kurz, eindeutig
// hörbar und ohne Umlaute, weil Umlaute auf mancher Tastatur einen Umweg
// kosten.
//
// Sie stehen in diesem Paket und nicht bei den Zugängen, weil beide sie
// brauchen: die Einsatzzentrale beim Ausstellen der Teamkarten und das
// Testspiel beim Anlegen. Andersherum ginge es nicht – internal/api liest
// dieses Paket bereits, ein Verweis zurück wäre ein Ring.
var woerter = []string{
	"anker", "biber", "distel", "falke", "hirsch", "kranich", "linde",
	"otter", "rabe", "zeder", "amsel", "birke", "dachs", "erle", "fuchs",
	"gams", "hafen", "iltis", "kiefer", "luchs", "marder", "nebel",
	"olive", "pappel", "quelle", "reiher", "specht", "tanne", "ulme", "wiesel",
}

// SpeakablePassword würfelt ein Kennwort der Form "biber-tanne-47".
//
// Aus crypto/rand und nicht aus math/rand: Das hier ist ein Zugang, kein
// Würfelspiel. Bei rund 30 Wörtern und 90 Zahlen sind das etwa 81.000
// Möglichkeiten je Wortpaar – genug für einen Spieltag, an dem niemand raten
// kann, und wenig genug, dass es sich durchsagen lässt.
func SpeakablePassword() (string, error) {
	zieh := func(n int) (int, error) {
		v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
		if err != nil {
			return 0, err
		}
		return int(v.Int64()), nil
	}

	a, err := zieh(len(woerter))
	if err != nil {
		return "", err
	}
	b, err := zieh(len(woerter))
	if err != nil {
		return "", err
	}
	n, err := zieh(90)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%s-%d", woerter[a], woerter[b], 10+n), nil
}
