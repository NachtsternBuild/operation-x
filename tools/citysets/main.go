// Erzeugt die Stadtpakete, die im Programm mitgeliefert werden.
//
// Aufruf (braucht Internet, dauert wegen der Höflichkeitspausen bei
// OpenStreetMap ein paar Minuten):
//
//	go run ./tools/citysets Dresden Leipzig Berlin …
//
// Das Ergebnis liegt unter internal/citysets/data und wird in die Binary
// eingebettet. Damit lässt sich ein Spiel ohne Internet vorbereiten – und ohne
// dass jemand zwanzig Hotspots von Hand auf eine Karte klickt.
//
// Bewusst ein eigenes Werkzeug und kein Befehl des Spielservers: Die Abfragen
// gehören zur Entwicklung, nicht zum Spieltag. Auf dem Rechner der
// Spielleitung hat ein Werkzeug, das fremde Server befragt, nichts zu suchen.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/elias/operation-x/pkg/citysets"
	"github.com/elias/operation-x/pkg/geo"
)

// Wie viele Hotspots ein Paket höchstens enthält.
//
// Zwanzig sind eine gute Spiellänge: Bei neun Zwischenzielen bleibt genug
// Auswahl, damit sich keine Route wiederholt, und die Liste passt noch auf ein
// gedrucktes Blatt.
const maxHotspots = 24

// Mindestabstand zwischen zwei Hotspots.
//
// Ohne ihn liegen alle zwanzig in der Altstadt, weil dort die meisten
// Sehenswürdigkeiten stehen – und ein Spiel, das sich auf vier Straßenzügen
// abspielt, ist keins.
// Dreihundertfünfzig Meter sind vier Minuten Fußweg. Bei sechshundert fiel
// in Dresden die halbe Altstadt weg: Wer die Brühlsche Terrasse aufnimmt,
// verliert damit Frauenkirche, Semperoper und Zwinger – und die sind für ein
// Spiel in Dresden nicht verzichtbar.
const minSpacingM = 350

// Wie weit ein Ortsteil höchstens vom Stadtzentrum weg liegen darf.
const kernRadiusM = 5000

// Und wie weit ein Hotspot. Enger als die Sektoren, weil ein Sektor auch
// Randgebiet enthalten darf, ein Zwischenziel aber nicht.
const hotspotRadiusM = 3500

// Wie bekannt ein Ort mindestens sein muss.
//
// Die Bewertung in FetchPOIs gibt vier Punkte für einen Wikipedia-Artikel und
// drei dafür, dass das Objekt eine Fläche ist und kein Schild. Acht verlangt
// beides: einen Ort, über den jemand einen Artikel geschrieben hat, und ein
// Gebäude statt eines Schilds. Ohne diese Schwelle standen ein "Bürgerbüro
// Klotzsche" und ein "Ökumenisches Seelsorgezentrum – Haus 50" auf der Liste.
//
// Reicht das nicht für genug Punkte, wird die Schwelle gesenkt: Eine kleinere
// Stadt hat weniger Wikipedia-Artikel, aber trotzdem Orte, die jeder kennt.
const minScore = 8
const minScoreNotfall = 5

func main() {
	cities := os.Args[1:]
	if len(cities) == 0 {
		log.Fatal("Aufruf: go run ./tools/citysets <Stadt> [<Stadt> …]")
	}

	out := filepath.Join("internal", "citysets", "data")
	if err := os.MkdirAll(out, 0o755); err != nil {
		log.Fatal(err)
	}

	for _, city := range cities {
		fmt.Printf("── %s ───────────────────────────────\n", city)
		pack, err := build(city)
		if err != nil {
			log.Printf("   %s: %v", city, err)
			continue
		}

		raw, err := json.MarshalIndent(pack, "", " ")
		if err != nil {
			log.Fatal(err)
		}

		path := filepath.Join(out, pack.Slug+".json")
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("   %d Sektoren, %d Hotspots → %s (%.0f kB)\n\n",
			len(pack.Sectors), len(pack.Hotspots), path, float64(len(raw))/1024)
	}
}

func build(city string) (*citysets.Pack, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	nom := geo.NewNominatim()
	places, err := nom.SearchCity(ctx, city)
	if err != nil {
		return nil, err
	}
	if len(places) == 0 {
		return nil, fmt.Errorf("nicht gefunden")
	}
	place := places[0]

	areaID, ok := place.AreaID()
	if !ok {
		return nil, fmt.Errorf("%q ist ein Punkt, keine Fläche", place.Name)
	}
	fmt.Printf("   gefunden: %s\n", place.Name)

	over := geo.NewOverpass()

	// Der Mittelpunkt der Stadt, nicht die Mitte ihres Rechtecks.
	//
	// Dresden reicht weit nach Norden und Osten; die Mitte des umschließenden
	// Rechtecks liegt deshalb in der Neustadt, nicht in der Altstadt. Die
	// Auswahl richtete sich danach – und Frauenkirche, Zwinger und Semperoper
	// fielen aus einem Dresden-Paket heraus. Nominatim liefert den Ortspunkt
	// gleich mit; der ist gemeint, wenn jemand "das Zentrum" sagt.
	mitte := geo.Point{Lat: place.Lat, Lng: place.Lng}
	if mitte.Lat == 0 && mitte.Lng == 0 {
		mitte = geo.Point{
			Lat: (place.Bounds.North + place.Bounds.South) / 2,
			Lng: (place.Bounds.East + place.Bounds.West) / 2,
		}
	}
	fmt.Printf("   Zentrum: %.4f, %.4f\n", mitte.Lat, mitte.Lng)

	// Die brauchbare Verwaltungsebene ist von Stadt zu Stadt verschieden.
	//
	// Der erste Anlauf nahm die erste Ebene mit vier bis vierzehn Ortsteilen –
	// und lag damit in Berlin und München daneben: Dort traf er eine Ebene mit
	// einer Handvoll Einträgen, die den Stadtkern gar nicht abdecken, und das
	// Paket hatte am Ende keinen einzigen Hotspot.
	//
	// Also wird gemessen statt geraten: Welche Ebene liefert die meisten
	// Ortsteile rund um das Zentrum? Das ist genau die Frage, auf die es
	// ankommt, denn dort wird gespielt.
	var districts []geo.District
	beste := 0

	for _, level := range []int{9, 10, 11} {
		// Eine Ebene ohne Grenzen ist kein Fehler, sondern eine Auskunft: Nicht
		// jede Stadt führt jede Verwaltungsebene. Vorher hat der erste solche
		// Fall die schon gefundenen Ortsteile mitgerissen.
		found, err := over.FetchDistricts(ctx, areaID, []int{level})
		if err != nil {
			fmt.Printf("   Ebene %d: %v\n", level, err)
			continue
		}

		nah := 0
		for _, d := range found {
			if geo.DistanceM(mitte, d.Center) <= kernRadiusM {
				nah++
			}
		}
		if nah > 8 {
			nah = 8
		}
		fmt.Printf("   Ebene %d: %d Ortsteile, davon %d im Kern\n", level, len(found), nah)

		if nah > beste {
			beste = nah
			districts = found
		}
	}
	if len(districts) == 0 {
		return nil, fmt.Errorf("keine Ortsteilgrenzen gefunden")
	}

	// Zu viele Ortsteile: die innenstadtnächsten nehmen.
	//
	// Nach Fläche zu sortieren wäre der naheliegende, aber falsche Weg: Die
	// größten Ortsteile einer Stadt sind die am Rand, mit Wald und Feldern
	// darin. Ein Spielgebiet, das von Weixdorf bis Pillnitz reicht, ist
	// zwanzig Kilometer breit – zu Fuß in sechs Stunden nicht zu bespielen.
	sort.Slice(districts, func(i, j int) bool {
		return geo.DistanceM(mitte, districts[i].Center) < geo.DistanceM(mitte, districts[j].Center)
	})

	// Von innen nach außen aufnehmen, solange das Gebiet begehbar bleibt.
	//
	// Maßstab ist die Breite des Spielfelds, nicht die Zahl der Ortsteile: Ein
	// Spiel zu Fuß über sechs Stunden braucht ein Gebiet, das man queren kann.
	// Acht Kilometer sind gut zwei Stunden Fußweg von Rand zu Rand – mehr
	// wäre kein Spielgebiet mehr, sondern eine Wanderung.
	// Eine einfache Regel statt einer Rechnerei: Was mit seinem Mittelpunkt
	// weiter als fünf Kilometer vom Stadtzentrum weg liegt, gehört nicht mehr
	// zu einem Spiel, das zu Fuß stattfindet.
	kern := districts[:0:0]
	for _, d := range districts {
		if geo.DistanceM(mitte, d.Center) <= kernRadiusM {
			kern = append(kern, d)
		}
	}
	if len(kern) >= 3 {
		districts = kern
	}
	if len(districts) > 8 {
		districts = districts[:8]
	}
	sort.Slice(districts, func(i, j int) bool { return districts[i].Name < districts[j].Name })

	for _, d := range districts {
		fmt.Printf("     %-28s %5.1f km²  %4.1f km vom Zentrum\n",
			d.Name, d.AreaKM2, geo.DistanceM(mitte, d.Center)/1000)
	}

	pack := &citysets.Pack{
		Slug: slugify(place.Name),
		City: place.Name,
	}

	areas := make([]geo.Area, 0, len(districts))
	for i, d := range districts {
		area, err := geo.AreaFromGeoJSON(d.Geometry)
		if err != nil || len(area.Outer) == 0 {
			continue
		}

		vereinfacht := geo.Area{Outer: geo.Simplify(area.Outer, 40)}
		areas = append(areas, vereinfacht)

		pack.Sectors = append(pack.Sectors, citysets.Sector{
			Code:     string(rune('A' + i)),
			Name:     d.Name,
			Geometry: vereinfacht.GeoJSON(),
		})
	}

	// Höflichkeitspause und ein zweiter Versuch.
	//
	// Overpass ist ein fremder, kostenlos betriebener Dienst; mehrere Abfragen
	// hintereinander laufen in eine Sperre. Dieses Werkzeug läuft einmal bei
	// der Entwicklung – da kosten ein paar Sekunden Warten nichts, und eine
	// leere Antwort kostet den ganzen Lauf.
	var pois []geo.POI
	for versuch := 1; versuch <= 4; versuch++ {
		time.Sleep(time.Duration(versuch*20) * time.Second)

		pois, err = over.FetchPOIs(ctx, place.Bounds, 400)
		if err != nil {
			fmt.Printf("   Ortsabfrage (Versuch %d): %v\n", versuch, err)
			continue
		}
		if len(pois) > 0 {
			break
		}
		fmt.Printf("   Ortsabfrage (Versuch %d): leer\n", versuch)
	}
	fmt.Printf("   %d Ortsvorschläge\n", len(pois))

	// Keine Obergrenze je Art.
	//
	// Ein Versuch damit ging schief: Die bekanntesten Orte einer Stadt sind
	// fast alle "landmark", und nach fünf davon rutschten Seelsorgezentren und
	// S-Bahn-Haltepunkte in die Liste, während Zwinger und Semperoper fehlten.
	// Für die Verteilung über die Stadt sorgt der Mindestabstand, für die
	// Auswahl die Bewertung – beides besser als eine Quote.
	// Bei gleicher Bewertung entscheidet die Nähe zum Zentrum, nicht das
	// Alphabet.
	//
	// Die Quelle sortiert gleichwertige Orte nach Namen, und das ist für eine
	// Auswahl willkürlich: In Dresden gewann so der "Alter Jüdischer Friedhof"
	// gegen die Frauenkirche, weil A vor F kommt. Ein Stadtspiel spielt in der
	// Mitte – und was dort steht, kennt auch jeder.
	sort.SliceStable(pois, func(i, j int) bool {
		if pois[i].Score != pois[j].Score {
			return pois[i].Score > pois[j].Score
		}
		return geo.DistanceM(mitte, geo.Point{Lat: pois[i].Lat, Lng: pois[i].Lng}) <
			geo.DistanceM(mitte, geo.Point{Lat: pois[j].Lat, Lng: pois[j].Lng})
	})

	sammle(pack, pois, areas, mitte, minScore)
	if len(pack.Hotspots) < 12 {
		sammle(pack, pois, areas, mitte, minScoreNotfall)
	}

	if len(pack.Hotspots) < 8 {
		return nil, fmt.Errorf("nur %d brauchbare Hotspots gefunden", len(pack.Hotspots))
	}

	// Sektoren ohne einen einzigen Hotspot fliegen raus: Sie stehen auf der
	// Karte herum, ohne dass je etwas in ihnen passiert – und in einem
	// Hinweis "Sektor C" zu lesen, in dem nichts liegt, führt die Fahndung
	// vor.
	benutzt := map[string]bool{}
	for _, h := range pack.Hotspots {
		punkt := geo.Point{Lat: h.Lat, Lng: h.Lng}
		for i, a := range areas {
			if a.Contains(punkt) {
				benutzt[pack.Sectors[i].Code] = true
			}
		}
	}

	gefiltert := pack.Sectors[:0:0]
	for _, sec := range pack.Sectors {
		if benutzt[sec.Code] {
			gefiltert = append(gefiltert, sec)
		}
	}
	// Codes neu vergeben, damit keine Lücke entsteht: A, B, C …
	for i := range gefiltert {
		gefiltert[i].Code = string(rune('A' + i))
	}
	pack.Sectors = gefiltert

	// Ein Paket mit einem einzigen Sektor ist wertlos: Der Hinweis "Sektor A"
	// benennt dann die ganze Karte. Im ersten Lauf kam Köln so heraus.
	if len(pack.Sectors) < 3 {
		return nil, fmt.Errorf("nur %d brauchbare Sektoren", len(pack.Sectors))
	}

	return pack, nil
}

// round kürzt auf fünf Nachkommastellen – etwa ein Meter. Mehr wäre eine
// Genauigkeit, die weder die Quelle hat noch das Spiel braucht, und sie
// verdoppelt die Dateigröße.
func round(v float64) float64 {
	return float64(int64(v*1e5+0.5)) / 1e5
}

func slugify(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			switch r {
			case 'ä':
				b.WriteString("ae")
			case 'ö':
				b.WriteString("oe")
			case 'ü':
				b.WriteString("ue")
			case 'ß':
				b.WriteString("ss")
			default:
				b.WriteRune(r)
			}
		case r == ' ' || r == '-' || r == '/':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// sammle nimmt Hotspots auf, die gut genug, nah genug und weit genug
// voneinander entfernt sind.
func sammle(pack *citysets.Pack, pois []geo.POI, areas []geo.Area, mitte geo.Point, schwelle int) {
	vorhanden := map[string]bool{}
	for _, h := range pack.Hotspots {
		vorhanden[h.Name] = true
	}

	for _, p := range pois {
		if len(pack.Hotspots) >= maxHotspots {
			return
		}
		if p.Score < schwelle || vorhanden[p.Name] {
			continue
		}

		punkt := geo.Point{Lat: p.Lat, Lng: p.Lng}

		// Ein Hotspot am Stadtrand macht aus einem Zwischenziel einen Ausflug.
		if geo.DistanceM(mitte, punkt) > hotspotRadiusM {
			continue
		}

		// Nur, was in einem der Sektoren liegt: Ein Hotspot außerhalb des
		// Spielgebiets ist ein Ziel, das niemand anlaufen darf.
		drin := false
		for _, a := range areas {
			if a.Contains(punkt) {
				drin = true
				break
			}
		}
		if !drin {
			continue
		}

		// Der Mindestabstand ersetzt jede Quote: Ohne ihn stünden zwanzig
		// Hotspots auf demselben Altstadtplatz, weil dort die meisten
		// Sehenswürdigkeiten stehen.
		zuNah := false
		for _, h := range pack.Hotspots {
			if geo.DistanceM(punkt, geo.Point{Lat: h.Lat, Lng: h.Lng}) < minSpacingM {
				zuNah = true
				break
			}
		}
		if zuNah {
			continue
		}

		vorhanden[p.Name] = true
		pack.Hotspots = append(pack.Hotspots, citysets.Hotspot{
			Name: p.Name,
			Lat:  round(p.Lat),
			Lng:  round(p.Lng),
		})
	}
}
