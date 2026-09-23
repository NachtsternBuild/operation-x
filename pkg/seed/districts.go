package seed

import (
	"context"
	"fmt"
	"time"

	"github.com/elias/operation-x/pkg/geo"
)

// PrintDistricts sucht eine Stadt und gibt ihre Ortsteile aus.
//
// Werkzeug für die Vorbereitung: Damit lässt sich vor dem Einrichten eines
// Spiels sehen, welche Verwaltungsebene brauchbare Sektoren ergibt – und ob
// die Grenzdienste gerade antworten.
func PrintDistricts(city string, levels []int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	nom := geo.NewNominatim()

	fmt.Printf("Suche %q …\n", city)
	places, err := nom.SearchCity(ctx, city)
	if err != nil {
		return err
	}
	if len(places) == 0 {
		return fmt.Errorf("keine Stadt namens %q gefunden", city)
	}

	place := places[0]
	areaID, ok := place.AreaID()
	if !ok {
		return fmt.Errorf("%q ist keine Fläche, sondern ein Punkt – für ein Spielgebiet ungeeignet", place.Name)
	}

	fmt.Printf("Gefunden: %s (Relation %d)\n", place.Name, place.OSMID)
	fmt.Printf("Gebiet:   %.4f–%.4f N, %.4f–%.4f O\n\n",
		place.Bounds.South, place.Bounds.North, place.Bounds.West, place.Bounds.East)

	fmt.Printf("Frage Ortsteile der Ebenen %v ab …\n", levels)
	districts, err := geo.NewOverpass().FetchDistricts(ctx, areaID, levels)
	if err != nil {
		return err
	}

	fmt.Printf("\n%d Ortsteile mit Grenzen:\n\n", len(districts))
	fmt.Printf("  %-34s %-6s %9s  %s\n", "Name", "Ebene", "ca. km²", "Mittelpunkt")
	fmt.Printf("  %s\n", "───────────────────────────────────────────────────────────────────────")
	for _, d := range districts {
		fmt.Printf("  %-34s %-6d %9.1f  %.4f, %.4f\n",
			d.Name, d.AdminLevel, d.AreaKM2, d.Center.Lat, d.Center.Lng)
	}
	fmt.Println()

	return nil
}
