package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Overpass fragt die OSM-Datenbank ab.
//
// Anders als Nominatim liefert Overpass alle Ortsteile einer Stadt in einer
// einzigen Antwort – der schnelle Weg zu einem fertigen Sektorschnitt. Dafür
// ist der Dienst freiwillig betrieben und fällt regelmäßig aus, weshalb dieser
// Client mehrere Spiegel der Reihe nach durchprobiert. Ist keiner erreichbar,
// bleibt der Weg über Nominatim oder das Zeichnen von Hand.
type Overpass struct {
	Mirrors []string
	Client  *http.Client
}

func NewOverpass() *Overpass {
	return &Overpass{
		Mirrors: []string{
			"https://overpass-api.de/api/interpreter",
			"https://overpass.kumi.systems/api/interpreter",
			"https://overpass.private.coffee/api/interpreter",
			"https://overpass.osm.ch/api/interpreter",
		},
		Client: &http.Client{Timeout: 90 * time.Second},
	}
}

// District ist ein Ortsteil mit Grenze.
type District struct {
	Name       string          `json:"name"`
	AdminLevel int             `json:"adminLevel"`
	OSMID      int64           `json:"osmId"`
	Center     Point           `json:"center"`
	Bounds     Bounds          `json:"bounds"`
	AreaKM2    float64         `json:"areaKm2"`
	Geometry   json.RawMessage `json:"geometry"`
}

// FetchDistricts holt alle Ortsteile innerhalb eines Gebiets.
//
// levels benennt die Verwaltungsebenen, und welche brauchbar ist, unterscheidet
// sich von Stadt zu Stadt. In Dresden etwa liegen auf Ebene 9 die zehn
// Ortsamtsbereiche samt Ortschaften – mit 20 bis 160 km² viel zu groß für einen
// Sektor. Erst Ebene 11 trägt dort die 61 statistischen Stadtteile wie die
// Äußere Neustadt, und deren ein bis vier Quadratkilometer sind der Zuschnitt,
// mit dem sich vor Ort arbeiten lässt.
//
// Deshalb werden mehrere Ebenen zugleich geholt und im Einrichtungsassistenten
// mit Anzahl und Fläche zur Auswahl gestellt, statt eine Ebene fest anzunehmen.
func (o *Overpass) FetchDistricts(ctx context.Context, areaID int64, levels []int) ([]District, error) {
	pattern := make([]string, len(levels))
	for i, l := range levels {
		pattern[i] = fmt.Sprint(l)
	}

	query := fmt.Sprintf(`[out:json][timeout:60];
area(%d)->.a;
relation(area.a)["boundary"="administrative"]["admin_level"~"^(%s)$"];
out geom;`, areaID, strings.Join(pattern, "|"))

	body, err := o.run(ctx, query)
	if err != nil {
		return nil, err
	}

	var res struct {
		Elements []struct {
			Type    string            `json:"type"`
			ID      int64             `json:"id"`
			Tags    map[string]string `json:"tags"`
			Members []struct {
				Type     string `json:"type"`
				Role     string `json:"role"`
				Geometry []struct {
					Lat float64 `json:"lat"`
					Lon float64 `json:"lon"`
				} `json:"geometry"`
			} `json:"members"`
		} `json:"elements"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("Antwort der Grenzabfrage lesen: %w", err)
	}

	districts := make([]District, 0, len(res.Elements))
	for _, el := range res.Elements {
		name := el.Tags["name"]
		if name == "" {
			continue
		}

		// Wegstücke nach Rolle sammeln. Eine leere Rolle behandelt OSM wie "outer".
		var outer, inner [][]Point
		for _, m := range el.Members {
			if m.Type != "way" || len(m.Geometry) < 2 {
				continue
			}
			pts := make([]Point, len(m.Geometry))
			for i, g := range m.Geometry {
				pts[i] = Point{Lat: g.Lat, Lng: g.Lon}
			}
			if m.Role == "inner" {
				inner = append(inner, pts)
			} else {
				outer = append(outer, pts)
			}
		}

		outerRings := buildRings(outer)
		if len(outerRings) == 0 {
			continue // Grenze unvollständig – überspringen statt Unsinn anzeigen
		}

		// Bei mehreren äußeren Ringen die größte Fläche nehmen: Sektoren sollen
		// zusammenhängende Gebiete sein, mit denen man vor Ort arbeiten kann.
		sort.Slice(outerRings, func(i, j int) bool {
			return outerRings[i].approxArea() > outerRings[j].approxArea()
		})

		area := Area{Outer: outerRings[0], Holes: buildRings(inner)}
		bounds := area.Outer.Bounds()

		districts = append(districts, District{
			Name:       name,
			AdminLevel: atoiSafe(el.Tags["admin_level"]),
			OSMID:      el.ID,
			Center:     area.Outer.Centroid(),
			Bounds:     bounds,
			AreaKM2:    area.AreaM2() / 1_000_000,
			Geometry:   area.GeoJSON(),
		})
	}

	if len(districts) == 0 {
		return nil, fmt.Errorf("keine Ortsteile mit Grenzen gefunden")
	}

	sort.Slice(districts, func(i, j int) bool { return districts[i].Name < districts[j].Name })
	return districts, nil
}

// run probiert die Spiegel der Reihe nach und meldet erst auf, wenn keiner
// antwortet. Overpass liefert Fehler häufig als HTML mit Status 200 aus,
// deshalb wird der Inhalt geprüft und nicht nur der Statuscode.
func (o *Overpass) run(ctx context.Context, query string) ([]byte, error) {
	var problems []string

	for _, mirror := range o.Mirrors {
		body, err := o.post(ctx, mirror, query)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", short(mirror), err))
			continue
		}
		return body, nil
	}

	return nil, fmt.Errorf(
		"kein Grenzdienst erreichbar (%s) – Ortsteile lassen sich einzeln über die Ortssuche holen oder von Hand zeichnen",
		strings.Join(problems, "; "),
	)
}

func (o *Overpass) post(ctx context.Context, mirror, query string) ([]byte, error) {
	form := url.Values{"data": {query}}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mirror, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := o.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("nicht erreichbar")
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("Antwort abgebrochen")
	}

	if res.StatusCode == http.StatusTooManyRequests || res.StatusCode == http.StatusGatewayTimeout {
		return nil, fmt.Errorf("ausgelastet (%d)", res.StatusCode)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Status %d", res.StatusCode)
	}

	// Der Dienst antwortet im Fehlerfall mit einer HTML-Seite.
	if trimmed := strings.TrimSpace(string(body[:min(len(body), 64)])); !strings.HasPrefix(trimmed, "{") {
		if msg := extractOverpassError(string(body)); msg != "" {
			return nil, fmt.Errorf("%s", msg)
		}
		return nil, fmt.Errorf("unerwartete Antwort")
	}

	return body, nil
}

// extractOverpassError zieht die Klartextmeldung aus der HTML-Fehlerseite.
func extractOverpassError(body string) string {
	const marker = "Error</strong>:"
	i := strings.Index(body, marker)
	if i < 0 {
		return ""
	}
	rest := body[i+len(marker):]
	if j := strings.Index(rest, "</p>"); j >= 0 {
		rest = rest[:j]
	}

	msg := strings.TrimSpace(rest)
	if strings.Contains(msg, "too busy") || strings.Contains(msg, "Dispatcher") {
		return "überlastet"
	}
	if len(msg) > 90 {
		msg = msg[:90] + "…"
	}
	return msg
}

// buildRings verkettet lose Wegstücke zu geschlossenen Ringen.
//
// OSM speichert eine Stadtteilgrenze nicht als fertiges Polygon, sondern als
// Sammlung einzelner Wege in beliebiger Reihenfolge und Richtung. Erst das
// Aneinanderhängen an gemeinsamen Endpunkten ergibt die Fläche.
func buildRings(ways [][]Point) []Ring {
	open := make([][]Point, 0, len(ways))
	for _, w := range ways {
		if len(w) >= 2 {
			open = append(open, w)
		}
	}

	var rings []Ring

	for len(open) > 0 {
		current := open[0]
		open = open[1:]

		// Anhängen, bis der Ring geschlossen ist oder nichts mehr passt.
		for !samePoint(current[0], current[len(current)-1]) {
			end := current[len(current)-1]

			idx, reversed := findNext(open, end)
			if idx < 0 {
				break
			}

			next := open[idx]
			open = append(open[:idx], open[idx+1:]...)

			if reversed {
				next = reverse(next)
			}
			current = append(current, next[1:]...)
		}

		if len(current) >= 4 && samePoint(current[0], current[len(current)-1]) {
			rings = append(rings, Ring(current))
		}
	}

	return rings
}

// findNext sucht ein Wegstück, das am gegebenen Punkt anschließt.
func findNext(ways [][]Point, end Point) (idx int, reversed bool) {
	for i, w := range ways {
		if samePoint(w[0], end) {
			return i, false
		}
		if samePoint(w[len(w)-1], end) {
			return i, true
		}
	}
	return -1, false
}

func reverse(pts []Point) []Point {
	out := make([]Point, len(pts))
	for i, p := range pts {
		out[len(pts)-1-i] = p
	}
	return out
}

// samePoint vergleicht mit Toleranz – OSM-Koordinaten kommen mit sieben
// Nachkommastellen, das sind gut anderthalb Zentimeter.
func samePoint(a, b Point) bool {
	const eps = 1e-7
	return math.Abs(a.Lat-b.Lat) < eps && math.Abs(a.Lng-b.Lng) < eps
}

func atoiSafe(s string) int {
	var v int
	fmt.Sscanf(s, "%d", &v)
	return v
}

func short(u string) string {
	if p, err := url.Parse(u); err == nil {
		return p.Host
	}
	return u
}
