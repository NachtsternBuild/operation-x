// Package citysets liefert fertige Stadtpakete.
//
// Ein Spiel vorzubereiten heißt: Sektoren schneiden und zwanzig Hotspots
// setzen. Beides geht in der Oberfläche, beides dauert – und beides braucht
// Internet, weil die Grenzen und die Ortsvorschläge von OpenStreetMap kommen.
//
// Für die Städte, die hier liegen, geht es ohne: Sektorgrenzen und Hotspots
// sind fertig dabei und in wenigen Sekunden eingerichtet. Danach ist alles
// ganz normal änderbar – ein Paket ist ein Anfang, keine Vorschrift. Wer seine
// Stadt nicht findet, macht es wie bisher von Hand.
//
// Die Pakete entstehen mit tools/citysets aus echten OSM-Daten und werden hier
// eingebettet; zur Laufzeit fragt niemand irgendwo nach.
package citysets

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Der ganze Ordner, nicht nur die Pakete: So übersetzt sich das Programm auch,
// bevor das erste Paket erzeugt wurde. Gelesen werden nur die JSON-Dateien.
//
//go:embed data
var files embed.FS

// Sector ist ein fertiger Sektor mit Grenze.
type Sector struct {
	Code     string          `json:"code"`
	Name     string          `json:"name"`
	Geometry json.RawMessage `json:"geometry"`
}

// Hotspot ist ein vorgeschlagener Punkt. Nummer und Vor-Ort-Code vergibt der
// Server beim Anlegen – sie gehören zum Spiel, nicht zur Stadt.
type Hotspot struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}

// Pack ist ein Stadtpaket.
type Pack struct {
	Slug     string    `json:"slug"`
	City     string    `json:"city"`
	Sectors  []Sector  `json:"sectors"`
	Hotspots []Hotspot `json:"hotspots"`
}

// Summary beschreibt ein Paket, ohne die Geometrie mitzuschicken – für die
// Auswahlliste genügt das, und es spart auf dem Handy ein halbes Megabyte.
type Summary struct {
	Slug     string `json:"slug"`
	City     string `json:"city"`
	Sectors  int    `json:"sectors"`
	Hotspots int    `json:"hotspots"`
}

// List liefert alle mitgelieferten Pakete, nach Stadtnamen sortiert.
func List() []Summary {
	entries, err := files.ReadDir("data")
	if err != nil {
		return nil
	}

	out := make([]Summary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		pack, err := load(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			continue
		}
		out = append(out, Summary{
			Slug:     pack.Slug,
			City:     pack.City,
			Sectors:  len(pack.Sectors),
			Hotspots: len(pack.Hotspots),
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].City < out[j].City })
	return out
}

// Get liefert ein Paket.
func Get(slug string) (*Pack, error) {
	return load(slug)
}

func load(slug string) (*Pack, error) {
	// Kein Pfad, keine Punkte: Der Name kommt aus einer Anfrage von außen.
	if slug == "" || strings.ContainsAny(slug, "/\\.") {
		return nil, fmt.Errorf("unbekanntes Stadtpaket %q", slug)
	}

	raw, err := files.ReadFile("data/" + slug + ".json")
	if err != nil {
		return nil, fmt.Errorf("unbekanntes Stadtpaket %q", slug)
	}

	var pack Pack
	if err := json.Unmarshal(raw, &pack); err != nil {
		return nil, fmt.Errorf("Stadtpaket %q ist beschädigt: %w", slug, err)
	}
	return &pack, nil
}
