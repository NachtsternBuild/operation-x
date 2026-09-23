package game

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/elias/operation-x/pkg/geo"
)

var (
	testPoint = geo.Point{Lat: 51.0519, Lng: 13.7414} // Frauenkirche
	testTime  = time.Date(2026, 9, 9, 14, 30, 12, 0, time.UTC)
)

func TestBlurIsDeterministic(t *testing.T) {
	// Dieselbe Abfrage im selben Zeitfenster muss dieselbe Position liefern –
	// sonst ließe sich der Versatz durch Wiederholen herausrechnen.
	first := Blur(testPoint, "team_alpha", testTime, 200)

	for i := 0; i < 50; i++ {
		again := Blur(testPoint, "team_alpha", testTime.Add(time.Duration(i)*time.Millisecond), 200)
		if again != first {
			t.Fatalf("Abfrage %d lieferte %v statt %v", i, again, first)
		}
	}
}

func TestBlurDiffersPerTeamAndWindow(t *testing.T) {
	a := Blur(testPoint, "team_alpha", testTime, 200)
	b := Blur(testPoint, "team_bravo", testTime, 200)
	if a == b {
		t.Error("zwei Teams bekamen denselben Versatz")
	}

	later := Blur(testPoint, "team_alpha", testTime.Add(2*time.Minute), 200)
	if a == later {
		t.Error("der Versatz änderte sich über zwei Zeitfenster hinweg nicht")
	}
}

func TestBlurStaysWithinRadius(t *testing.T) {
	const radius = 200.0

	for i := 0; i < 500; i++ {
		at := testTime.Add(time.Duration(i) * time.Minute)
		got := Blur(testPoint, "team_alpha", at, radius)

		d := geo.DistanceM(testPoint, got)
		if d > radius+1 {
			t.Fatalf("Versatz %d lag %.1f m entfernt, erlaubt sind %.0f m", i, d, radius)
		}
	}
}

// TestBlurResistsAveraging ist der eigentliche Zweck der ganzen Konstruktion.
//
// Zufälliges Rauschen ließe sich herausmitteln: Wer eine unscharfe Position oft
// genug abfragt und den Mittelwert bildet, landet auf der Wahrheit. Weil der
// Versatz hier deterministisch aus Team und Zeitfenster stammt, liefern
// beliebig viele Abfragen im selben Fenster genau einen Wert – es gibt nichts
// zu mitteln.
func TestBlurResistsAveraging(t *testing.T) {
	const radius = 200.0

	var sumLat, sumLng float64
	const attempts = 1000

	for i := 0; i < attempts; i++ {
		// Ein Angreifer fragt so schnell er kann – alles im selben Zeitfenster.
		p := Blur(testPoint, "team_alpha", testTime.Add(time.Duration(i)*20*time.Millisecond), radius)
		sumLat += p.Lat
		sumLng += p.Lng
	}

	mean := geo.Point{Lat: sumLat / attempts, Lng: sumLng / attempts}
	d := geo.DistanceM(testPoint, mean)

	// Der Mittelwert muss so weit danebenliegen wie eine einzelne Abfrage.
	single := geo.DistanceM(testPoint, Blur(testPoint, "team_alpha", testTime, radius))
	if math.Abs(d-single) > 1 {
		t.Errorf("Mittelwert aus %d Abfragen lag %.1f m daneben, eine einzelne %.1f m – "+
			"das Rauschen ließ sich herausmitteln", attempts, d, single)
	}
}

// TestBlurFillsCircleEvenly prüft, dass die verschobenen Punkte über die
// Kreisfläche verteilt liegen und sich nicht in der Mitte häufen. Sonst läge
// die wahre Position im Schnitt näher am angezeigten Mittelpunkt als
// versprochen.
func TestBlurFillsCircleEvenly(t *testing.T) {
	const radius = 200.0
	const samples = 2000

	inner := 0 // innerhalb des halben Radius
	for i := 0; i < samples; i++ {
		at := testTime.Add(time.Duration(i) * time.Minute)
		if geo.DistanceM(testPoint, Blur(testPoint, fmt.Sprintf("t%d", i), at, radius)) < radius/2 {
			inner++
		}
	}

	// Der halbe Radius fasst ein Viertel der Fläche – also rund 25 % der Punkte.
	share := float64(inner) / samples
	if share < 0.20 || share > 0.30 {
		t.Errorf("%.1f %% der Punkte lagen im inneren Viertel der Fläche, erwartet rund 25 %%", share*100)
	}
}

func TestBlurZeroRadiusIsExact(t *testing.T) {
	if got := Blur(testPoint, "team_alpha", testTime, 0); got != testPoint {
		t.Errorf("ohne Radius kam %v statt der exakten Position zurück", got)
	}
}
