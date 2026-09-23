package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Nominatim ist der Ortssuchdienst von OpenStreetMap.
//
// Er liefert für einen benannten Ort ein fertiges Polygon – anders als Overpass
// muss daraus nichts zusammengesetzt werden. Dafür braucht er den Namen und
// beantwortet nur eine Anfrage pro Sekunde. Diese Grenze setzt der Client selbst
// durch; sie ist Bedingung der Nutzungsrichtlinie, nicht bloß eine Empfehlung.
type Nominatim struct {
	BaseURL string
	Client  *http.Client

	mu   sync.Mutex
	last time.Time
}

// userAgent nennt sich gegenüber den Diensten von OpenStreetMap.
//
// Beide verlangen das ausdrücklich: Eine Anfrage ohne erkennbaren Absender
// wird abgewiesen, und zwar zu Recht – es sind Dienste, die von Spenden leben.
// Die Adresse muss auf das Projekt führen, damit jemand sich melden kann,
// wenn dieses Programm sich schlecht benimmt.
const userAgent = "OperationX/1.0 (selbstgehostetes Stadtspiel; " + ProjektURL + ")"

// ProjektURL steht an einer Stelle, weil sie sich beim Veröffentlichen ändert.
const ProjektURL = "https://github.com/elias/operation-x"

func NewNominatim() *Nominatim {
	return &Nominatim{
		BaseURL: "https://nominatim.openstreetmap.org",
		Client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Place ist ein Suchtreffer.
type Place struct {
	Name     string          `json:"name"`
	OSMType  string          `json:"osmType"`
	OSMID    int64           `json:"osmId"`
	Lat      float64         `json:"lat"`
	Lng      float64         `json:"lng"`
	Bounds   Bounds          `json:"bounds"`
	Geometry json.RawMessage `json:"geometry,omitempty"`
}

// SearchCity sucht Städte und Gemeinden.
func (n *Nominatim) SearchCity(ctx context.Context, query string) ([]Place, error) {
	q := url.Values{}
	q.Set("q", query)
	q.Set("format", "jsonv2")
	q.Set("limit", "6")
	q.Set("featureType", "settlement") // Städte und Ortschaften, keine Straßen
	q.Set("addressdetails", "1")

	return n.search(ctx, q)
}

// FetchDistrict holt die Grenze eines benannten Ortsteils, etwa
// "Äußere Neustadt, Dresden". Der Ausweichweg, wenn Overpass nicht antwortet.
func (n *Nominatim) FetchDistrict(ctx context.Context, name, city string) (*Place, error) {
	q := url.Values{}
	q.Set("q", name+", "+city)
	q.Set("format", "jsonv2")
	q.Set("limit", "1")
	q.Set("polygon_geojson", "1")
	q.Set("polygon_threshold", "0.0001") // leicht vereinfacht, spart Datenmenge

	places, err := n.search(ctx, q)
	if err != nil {
		return nil, err
	}
	if len(places) == 0 {
		return nil, fmt.Errorf("kein Treffer für %q in %s", name, city)
	}
	if len(places[0].Geometry) == 0 {
		return nil, fmt.Errorf("%q hat keine Grenze hinterlegt", name)
	}

	return &places[0], nil
}

func (n *Nominatim) search(ctx context.Context, q url.Values) ([]Place, error) {
	n.throttle()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, n.BaseURL+"/search?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept-Language", "de")

	res, err := n.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Ortssuche nicht erreichbar: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ortssuche antwortete mit %d", res.StatusCode)
	}

	var raw []struct {
		Name        string          `json:"name"`
		DisplayName string          `json:"display_name"`
		OSMType     string          `json:"osm_type"`
		OSMID       int64           `json:"osm_id"`
		Lat         string          `json:"lat"`
		Lon         string          `json:"lon"`
		BoundingBox []string        `json:"boundingbox"`
		GeoJSON     json.RawMessage `json:"geojson"`
	}
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("Antwort der Ortssuche lesen: %w", err)
	}

	places := make([]Place, 0, len(raw))
	for _, r := range raw {
		p := Place{
			Name:     firstNonEmpty(r.Name, r.DisplayName),
			OSMType:  r.OSMType,
			OSMID:    r.OSMID,
			Lat:      parseFloat(r.Lat),
			Lng:      parseFloat(r.Lon),
			Geometry: r.GeoJSON,
		}
		// Nominatim liefert die Box als [Süd, Nord, West, Ost].
		if len(r.BoundingBox) == 4 {
			p.Bounds = Bounds{
				South: parseFloat(r.BoundingBox[0]),
				North: parseFloat(r.BoundingBox[1]),
				West:  parseFloat(r.BoundingBox[2]),
				East:  parseFloat(r.BoundingBox[3]),
			}
		}
		places = append(places, p)
	}

	return places, nil
}

// throttle hält den Mindestabstand von einer Sekunde zwischen zwei Anfragen ein.
func (n *Nominatim) throttle() {
	n.mu.Lock()
	defer n.mu.Unlock()

	if wait := time.Second - time.Since(n.last); wait > 0 {
		time.Sleep(wait)
	}
	n.last = time.Now()
}

// AreaID rechnet eine OSM-Relation in die Gebietskennung um, die Overpass
// erwartet.
func (p Place) AreaID() (int64, bool) {
	if !strings.EqualFold(p.OSMType, "relation") {
		return 0, false
	}
	return 3600000000 + p.OSMID, true
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
