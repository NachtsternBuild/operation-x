// Package geo enthält die Geometrie des Spielfelds: Koordinaten, Flächen,
// Entfernungen – und die Beschaffung echter Stadtgrenzen aus OpenStreetMap.
//
// Alle Berechnungen laufen auf dem Server. Ob ein Spieler nah genug an einem
// Hotspot steht, entscheidet nie sein Gerät.
package geo

import (
	"encoding/json"
	"fmt"
	"math"
)

// Erdradius in Metern, Mittelwert – für Entfernungen im Stadtgebiet genau genug.
const earthRadiusM = 6371008.8

// Point ist eine Koordinate in Grad.
type Point struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// Ring ist ein geschlossener Linienzug. Erster und letzter Punkt sind gleich.
type Ring []Point

// Area ist eine Fläche mit optionalen Aussparungen – ein Stadtteil kann
// Enklaven enthalten.
type Area struct {
	Outer Ring
	Holes []Ring
}

// Bounds ist ein achsenparalleles Rechteck.
type Bounds struct {
	West  float64 `json:"west"`
	South float64 `json:"south"`
	East  float64 `json:"east"`
	North float64 `json:"north"`
}

// DistanceM liefert die Entfernung zweier Punkte in Metern (Haversine).
func DistanceM(a, b Point) float64 {
	φ1 := rad(a.Lat)
	φ2 := rad(b.Lat)
	dφ := rad(b.Lat - a.Lat)
	dλ := rad(b.Lng - a.Lng)

	h := math.Sin(dφ/2)*math.Sin(dφ/2) +
		math.Cos(φ1)*math.Cos(φ2)*math.Sin(dλ/2)*math.Sin(dλ/2)

	return 2 * earthRadiusM * math.Asin(math.Sqrt(h))
}

// Offset verschiebt einen Punkt um die angegebenen Meter nach Norden und Osten.
// Wird für die Radar-Unschärfe gebraucht.
func Offset(p Point, northM, eastM float64) Point {
	dLat := northM / earthRadiusM
	dLng := eastM / (earthRadiusM * math.Cos(rad(p.Lat)))

	return Point{
		Lat: p.Lat + deg(dLat),
		Lng: p.Lng + deg(dLng),
	}
}

// Contains prüft, ob ein Punkt in der Fläche liegt (Strahlenverfahren).
// Punkte in einer Aussparung liegen außerhalb.
func (a Area) Contains(p Point) bool {
	if !a.Outer.contains(p) {
		return false
	}
	for _, h := range a.Holes {
		if h.contains(p) {
			return false
		}
	}
	return true
}

// contains arbeitet auf ebenen Koordinaten. Über die Ausdehnung einer Stadt
// ist der Fehler durch die Erdkrümmung bedeutungslos.
func (r Ring) contains(p Point) bool {
	inside := false
	n := len(r)
	if n < 3 {
		return false
	}

	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		pi, pj := r[i], r[j]
		if (pi.Lat > p.Lat) == (pj.Lat > p.Lat) {
			continue
		}
		x := (pj.Lng-pi.Lng)*(p.Lat-pi.Lat)/(pj.Lat-pi.Lat) + pi.Lng
		if p.Lng < x {
			inside = !inside
		}
	}
	return inside
}

// Bounds liefert das umschließende Rechteck.
func (r Ring) Bounds() Bounds {
	b := Bounds{West: 180, South: 90, East: -180, North: -90}
	for _, p := range r {
		b.West = math.Min(b.West, p.Lng)
		b.East = math.Max(b.East, p.Lng)
		b.South = math.Min(b.South, p.Lat)
		b.North = math.Max(b.North, p.Lat)
	}
	return b
}

// Centroid liefert den Flächenschwerpunkt – Ankerpunkt für die Sektorbeschriftung
// auf der Karte.
func (r Ring) Centroid() Point {
	if len(r) == 0 {
		return Point{}
	}

	var area, cx, cy float64
	n := len(r)
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		cross := r[i].Lng*r[j].Lat - r[j].Lng*r[i].Lat
		area += cross
		cx += (r[i].Lng + r[j].Lng) * cross
		cy += (r[i].Lat + r[j].Lat) * cross
	}

	// Entartete Fläche (alle Punkte auf einer Linie): Mittelwert nehmen.
	if math.Abs(area) < 1e-12 {
		var sx, sy float64
		for _, p := range r {
			sx += p.Lng
			sy += p.Lat
		}
		return Point{Lat: sy / float64(n), Lng: sx / float64(n)}
	}

	area *= 0.5
	return Point{Lat: cy / (6 * area), Lng: cx / (6 * area)}
}

// Grow vergrößert ein Rechteck um den angegebenen Rand in Metern.
func (b Bounds) Grow(marginM float64) Bounds {
	midLat := (b.North + b.South) / 2
	dLat := deg(marginM / earthRadiusM)
	dLng := deg(marginM / (earthRadiusM * math.Cos(rad(midLat))))

	return Bounds{
		West:  b.West - dLng,
		South: b.South - dLat,
		East:  b.East + dLng,
		North: b.North + dLat,
	}
}

// Contains prüft, ob ein Punkt im Rechteck liegt.
func (b Bounds) Contains(p Point) bool {
	return p.Lat >= b.South && p.Lat <= b.North && p.Lng >= b.West && p.Lng <= b.East
}

// --- GeoJSON ------------------------------------------------------------------

// GeoJSON wandelt eine Fläche in ein GeoJSON-Polygon. So liegt sie in der
// Datenbank und so versteht sie die Karte im Browser.
func (a Area) GeoJSON() json.RawMessage {
	rings := make([][][2]float64, 0, 1+len(a.Holes))
	rings = append(rings, a.Outer.coords())
	for _, h := range a.Holes {
		rings = append(rings, h.coords())
	}

	raw, err := json.Marshal(map[string]any{
		"type":        "Polygon",
		"coordinates": rings,
	})
	if err != nil {
		// Kann bei diesen Typen nicht auftreten.
		return json.RawMessage(`{"type":"Polygon","coordinates":[]}`)
	}
	return raw
}

// coords liefert die Punkte in GeoJSON-Reihenfolge: Länge vor Breite.
func (r Ring) coords() [][2]float64 {
	out := make([][2]float64, len(r))
	for i, p := range r {
		out[i] = [2]float64{p.Lng, p.Lat}
	}
	return out
}

// AreaFromGeoJSON liest ein GeoJSON-Polygon oder -MultiPolygon zurück.
// Bei einem MultiPolygon wird der flächenmäßig größte Teil genommen – für
// Sektoren ist eine einzelne Fläche das, womit sich vor Ort arbeiten lässt.
func AreaFromGeoJSON(raw json.RawMessage) (Area, error) {
	var g struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		return Area{}, fmt.Errorf("GeoJSON lesen: %w", err)
	}

	switch g.Type {
	case "Polygon":
		var rings [][][2]float64
		if err := json.Unmarshal(g.Coordinates, &rings); err != nil {
			return Area{}, fmt.Errorf("Polygon lesen: %w", err)
		}
		return areaFromRings(rings), nil

	case "MultiPolygon":
		var polys [][][][2]float64
		if err := json.Unmarshal(g.Coordinates, &polys); err != nil {
			return Area{}, fmt.Errorf("MultiPolygon lesen: %w", err)
		}
		if len(polys) == 0 {
			return Area{}, fmt.Errorf("MultiPolygon ohne Flächen")
		}

		best := 0
		bestSize := -1.0
		for i, p := range polys {
			if len(p) == 0 {
				continue
			}
			a := areaFromRings(p)
			if s := a.Outer.approxArea(); s > bestSize {
				best, bestSize = i, s
			}
		}
		return areaFromRings(polys[best]), nil

	default:
		return Area{}, fmt.Errorf("nicht unterstützte Geometrie %q", g.Type)
	}
}

func areaFromRings(rings [][][2]float64) Area {
	var a Area
	for i, r := range rings {
		ring := make(Ring, len(r))
		for j, c := range r {
			ring[j] = Point{Lat: c[1], Lng: c[0]}
		}
		if i == 0 {
			a.Outer = ring
		} else {
			a.Holes = append(a.Holes, ring)
		}
	}
	return a
}

// approxArea liefert ein Größenmaß in Gradquadraten – reicht, um die größte
// Teilfläche eines MultiPolygons zu finden.
func (r Ring) approxArea() float64 {
	var sum float64
	n := len(r)
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		sum += r[i].Lng*r[j].Lat - r[j].Lng*r[i].Lat
	}
	return math.Abs(sum / 2)
}

// AreaM2 liefert die Fläche in Quadratmetern.
//
// Gerechnet wird auf einer örtlichen Ebene: Über die Ausdehnung eines
// Stadtteils ist der Fehler dieser Näherung weit kleiner als die Ungenauigkeit
// der Grenzen selbst. Die Zahl entscheidet im Einrichtungsassistenten darüber,
// welche Verwaltungsebene brauchbare Sektoren ergibt – ein Rechteck um den
// Umriss würde längliche Gebiete um ein Vielfaches überschätzen.
func (r Ring) AreaM2() float64 {
	n := len(r)
	if n < 3 {
		return 0
	}

	var latSum float64
	for _, p := range r {
		latSum += p.Lat
	}
	// Meter je Radiant, bei der mittleren Breite des Rings.
	mx := earthRadiusM * math.Cos(rad(latSum/float64(n)))
	my := earthRadiusM

	var sum float64
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		x1, y1 := rad(r[i].Lng)*mx, rad(r[i].Lat)*my
		x2, y2 := rad(r[j].Lng)*mx, rad(r[j].Lat)*my
		sum += x1*y2 - x2*y1
	}

	return math.Abs(sum / 2)
}

// AreaM2 liefert die Fläche abzüglich der Aussparungen.
func (a Area) AreaM2() float64 {
	total := a.Outer.AreaM2()
	for _, h := range a.Holes {
		total -= h.AreaM2()
	}
	if total < 0 {
		return 0
	}
	return total
}

func rad(d float64) float64 { return d * math.Pi / 180 }
func deg(r float64) float64 { return r * 180 / math.Pi }

// Simplify dünnt einen Linienzug aus (Douglas-Peucker).
//
// Eine Stadtteilgrenze aus OpenStreetMap folgt jedem Grundstückszaun und
// besteht aus tausenden Punkten. Auf einer Spielkarte ist davon nichts zu
// sehen: Dort ist der Sektor eine Fläche, die man im Vorbeigehen erkennen
// soll. Für die mitgelieferten Stadtpakete ist der Unterschied der zwischen
// einem halben Megabyte und zwanzig Kilobyte – und die lädt jemand am
// Spieltag über Mobilfunk.
//
// [toleranzM] ist der größte Fehler, der dabei entstehen darf, in Metern.
func Simplify(ring Ring, toleranzM float64) Ring {
	if len(ring) < 4 || toleranzM <= 0 {
		return ring
	}

	// Ein Ring ist geschlossen: Der letzte Punkt wiederholt den ersten. Er
	// wird abgetrennt, ausgedünnt und danach wieder geschlossen, sonst
	// verschiebt sich die Naht.
	offen := ring
	geschlossen := ring[0] == ring[len(ring)-1]
	if geschlossen {
		offen = ring[:len(ring)-1]
	}

	behalten := douglasPeucker(offen, toleranzM)

	// Unter drei Punkten ist es keine Fläche mehr.
	if len(behalten) < 3 {
		return ring
	}
	if geschlossen {
		behalten = append(behalten, behalten[0])
	}
	return behalten
}

func douglasPeucker(punkte Ring, toleranzM float64) Ring {
	if len(punkte) < 3 {
		return punkte
	}

	// Den Punkt mit dem größten Abstand zur Verbindungslinie suchen.
	maxAbstand := 0.0
	index := 0
	for i := 1; i < len(punkte)-1; i++ {
		d := abstandZurStrecke(punkte[i], punkte[0], punkte[len(punkte)-1])
		if d > maxAbstand {
			maxAbstand = d
			index = i
		}
	}

	if maxAbstand <= toleranzM {
		return Ring{punkte[0], punkte[len(punkte)-1]}
	}

	links := douglasPeucker(punkte[:index+1], toleranzM)
	rechts := douglasPeucker(punkte[index:], toleranzM)

	return append(links[:len(links)-1], rechts...)
}

// abstandZurStrecke liefert den Abstand eines Punktes zur Strecke a–b in Metern.
func abstandZurStrecke(p, a, b Point) float64 {
	// In dieser Größenordnung genügt eine ebene Näherung: Ein Stadtteil ist
	// wenige Kilometer groß, und dort ist die Erdkrümmung kleiner als die
	// Toleranz, um die es hier geht.
	mx := math.Cos(rad((a.Lat + b.Lat) / 2))
	ax, ay := a.Lng*mx, a.Lat
	bx, by := b.Lng*mx, b.Lat
	px, py := p.Lng*mx, p.Lat

	dx, dy := bx-ax, by-ay
	if dx == 0 && dy == 0 {
		return DistanceM(p, a)
	}

	t := ((px-ax)*dx + (py-ay)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))

	nächster := Point{Lat: ay + t*dy, Lng: (ax + t*dx) / mx}
	return DistanceM(p, nächster)
}
