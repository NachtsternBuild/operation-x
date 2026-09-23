package game

import (
	"encoding/binary"
	"hash/fnv"
	"math"
	"math/rand"
	"time"

	"github.com/elias/operation-x/pkg/geo"
)

// blurWindow ist die Zeitspanne, in der eine unscharfe Position gleich bleibt.
//
// Der Kreis wandert dadurch in ruhigen Sprüngen, statt bei jeder Abfrage zu
// flackern – und, wichtiger, die Unschärfe hält, was sie verspricht.
const blurWindow = 60 * time.Second

// Blur verschiebt eine Position um einen zufälligen Betrag innerhalb des
// angegebenen Radius.
//
// Entscheidend ist, dass der Versatz *deterministisch* ist. Zufälliges Rauschen
// ließe sich herausmitteln: Wer die unscharfe Position eines Teams zwanzig Mal
// abfragt und den Mittelwert bildet, landet ziemlich genau auf der Wahrheit.
// Der Versatz wird deshalb aus Team-Kennung und Zeitfenster berechnet – innerhalb
// desselben Fensters liefert jede Abfrage exakt dieselbe verschobene Position,
// egal wie oft jemand nachfragt.
func Blur(p geo.Point, teamID string, at time.Time, radiusM float64) geo.Point {
	if radiusM <= 0 {
		return p
	}

	window := at.Truncate(blurWindow).Unix()

	h := fnv.New64a()
	h.Write([]byte(teamID))
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(window))
	h.Write(buf[:])

	rng := rand.New(rand.NewSource(int64(h.Sum64())))

	// Gleichverteilt über die Kreisfläche. Ohne die Wurzel häuften sich die
	// Punkte in der Mitte – und damit läge die wahre Position im Schnitt
	// näher am angezeigten Kreismittelpunkt als versprochen.
	angle := rng.Float64() * 2 * math.Pi
	dist := radiusM * math.Sqrt(rng.Float64())

	return geo.Offset(p, dist*math.Sin(angle), dist*math.Cos(angle))
}
