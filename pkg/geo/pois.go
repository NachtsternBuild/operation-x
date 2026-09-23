package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// POI ist ein Ortsvorschlag für einen Hotspot.
type POI struct {
	Name  string  `json:"name"`
	Kind  string  `json:"kind"` // landmark, transit, park, building
	Lat   float64 `json:"lat"`
	Lng   float64 `json:"lng"`
	Score int     `json:"score"`
	OSMID int64   `json:"osmId"`
}

// FetchPOIs sucht markante Orte in einem Gebiet und gibt die besten zurück.
//
// Ein guter Hotspot ist vor Ort ohne Zweifel wiederzuerkennen: Man steht davor
// und weiß, dass man richtig ist. Die OSM-Daten liefern zu diesem Zweck sehr
// Gemischtes – neben der Frauenkirche auch vierzig Hotels und Gedenksteine von
// der Größe eines Pflastersteins. Deshalb wird hier nicht hart gefiltert,
// sondern bewertet und sortiert; die Auswahl trifft am Ende ein Mensch, der die
// Stadt kennt.
func (o *Overpass) FetchPOIs(ctx context.Context, b Bounds, limit int) ([]POI, error) {
	box := fmt.Sprintf("%.5f,%.5f,%.5f,%.5f", b.South, b.West, b.North, b.East)

	query := fmt.Sprintf(`[out:json][timeout:60];
(
  nwr(%[1]s)["name"]["wikidata"]["tourism"];
  nwr(%[1]s)["name"]["wikidata"]["historic"];
  nwr(%[1]s)["name"]["wikidata"]["amenity"~"^(theatre|museum|place_of_worship|townhall|arts_centre)$"];
  nwr(%[1]s)["name"]["railway"="station"];
  nwr(%[1]s)["name"]["public_transport"="station"];
  nwr(%[1]s)["name"]["wikidata"]["leisure"="park"];
);
out center;`, box)

	body, err := o.run(ctx, query)
	if err != nil {
		return nil, err
	}

	var res struct {
		Elements []struct {
			Type   string  `json:"type"`
			ID     int64   `json:"id"`
			Lat    float64 `json:"lat"`
			Lon    float64 `json:"lon"`
			Center *struct {
				Lat float64 `json:"lat"`
				Lon float64 `json:"lon"`
			} `json:"center"`
			Tags map[string]string `json:"tags"`
		} `json:"elements"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("Antwort der Ortsabfrage lesen: %w", err)
	}

	seen := map[string]bool{}
	pois := make([]POI, 0, len(res.Elements))

	for _, el := range res.Elements {
		name := el.Tags["name"]
		if name == "" || seen[name] {
			continue
		}

		lat, lng := el.Lat, el.Lon
		if el.Center != nil {
			lat, lng = el.Center.Lat, el.Center.Lon
		}
		if lat == 0 && lng == 0 {
			continue
		}

		score := rate(el.Tags, el.Type, name)
		if score <= 0 {
			continue // Unterkünfte und Beiwerk fallen hier heraus
		}

		seen[name] = true
		pois = append(pois, POI{
			Name:  name,
			Kind:  classify(el.Tags),
			Lat:   lat,
			Lng:   lng,
			Score: score,
			OSMID: el.ID,
		})
	}

	sort.Slice(pois, func(i, j int) bool {
		if pois[i].Score != pois[j].Score {
			return pois[i].Score > pois[j].Score
		}
		return pois[i].Name < pois[j].Name
	})

	if limit > 0 && len(pois) > limit {
		pois = pois[:limit]
	}

	return pois, nil
}

// rate bewertet, wie brauchbar ein Ort als Hotspot ist.
func rate(tags map[string]string, osmType, name string) int {
	// Unterkünfte sind keine Ziele – man steht davor und weiß nichts damit anzufangen.
	switch tags["tourism"] {
	case "hotel", "hostel", "apartment", "guest_house", "motel", "chalet", "camp_site":
		return 0
	}
	if tags["amenity"] == "restaurant" || tags["amenity"] == "cafe" {
		return 0
	}

	score := 1

	// Ein Wikipedia-Artikel ist der beste Hinweis darauf, dass ein Ort bekannt
	// genug ist, um ihn jemandem als Ziel zu nennen.
	if tags["wikipedia"] != "" {
		score += 4
	} else if tags["wikidata"] != "" {
		score += 1
	}

	// Flächige Objekte sind Gebäude und Plätze, punktförmige oft nur Schilder
	// und Kleinplastiken.
	if osmType == "way" || osmType == "relation" {
		score += 3
	}

	switch {
	case tags["railway"] == "station", tags["public_transport"] == "station":
		score += 4 // Verkehrsknoten: gut erreichbar und eindeutig
	case tags["tourism"] == "attraction", tags["tourism"] == "viewpoint":
		score += 3
	case tags["historic"] == "castle", tags["historic"] == "monument":
		score += 2
	case tags["amenity"] == "museum", tags["amenity"] == "theatre",
		tags["amenity"] == "place_of_worship", tags["amenity"] == "townhall":
		score += 2
	case tags["leisure"] == "park":
		score += 2
	case tags["tourism"] == "artwork", tags["historic"] == "memorial":
		score -= 1 // meist zu klein, um sie vor Ort sicher zu finden
	}

	// Sehr lange Namen gehören zu Gedenktafeln mit ganzen Sätzen darauf.
	if len(name) > 40 {
		score -= 2
	}

	return score
}

// classify ordnet einen Ort einer der Hotspot-Arten zu.
func classify(tags map[string]string) string {
	switch {
	case tags["railway"] == "station", tags["public_transport"] == "station":
		return "transit"
	case tags["leisure"] == "park":
		return "park"
	case tags["amenity"] == "museum", tags["amenity"] == "theatre",
		tags["amenity"] == "place_of_worship", tags["amenity"] == "townhall",
		tags["amenity"] == "arts_centre", tags["historic"] == "castle",
		strings.HasPrefix(tags["building"], "y"):
		return "building"
	default:
		return "landmark"
	}
}
