package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/geo"
)

// Stadtplan aus einer Sprachmaschine.
//
// Ein Spiel vorzubereiten heißt: Sektoren schneiden, zwanzig Hotspots setzen,
// zu jedem etwas ausdenken und ein Dutzend Rätsel schreiben. Das ist ein
// Nachmittag Arbeit, und es ist der Grund, warum ein Spiel nie in der Stadt
// stattfindet, in der gerade jemand Lust hätte.
//
// Wer eine Sprachmaschine zur Hand hat, kann sich den ersten Entwurf von dort
// holen: Der Text unten beschreibt das Spiel vollständig und verlangt eine
// Antwort in genau dem Format, das parsePlan wieder einliest. Beides steht
// bewusst in derselben Datei – ändert sich das Format, fällt sofort auf, dass
// der Text mitgeändert werden muss.
//
// Was dabei herauskommt, ist ein Entwurf und nichts weiter. Eine Maschine war
// nie in dieser Stadt: Sie kennt die berühmten Orte und erfindet die übrigen,
// und ihre Koordinaten sind manchmal einen Häuserblock daneben. Deshalb landet
// das Ergebnis nicht in der Datenbank, sondern auf der Karte, mit einem Haken
// vor jedem Eintrag.

// Obergrenzen. Nicht gegen Angriffe – hier ist die Einsatzzentrale angemeldet –,
// sondern gegen ein verrutschtes Einfügen aus der Zwischenablage.
const (
	maxPlanChars    = 400_000
	maxPlanSectors  = 60
	maxPlanHotspots = 200
	maxPlanPuzzles  = 100

	// Ein Sektor, den man zu Fuß durchquert, hat ein bis drei Quadratkilometer.
	// Ab dieser Größe stimmt etwas nicht – meistens hat die Maschine statt des
	// Stadtteils den halben Landkreis umrandet.
	maxPlanSectorKM2 = 25

	// Wie weit ein Hotspot außerhalb der Sektoren liegen darf, bevor er
	// auffällt. Etwas Luft muss sein: Ein Punkt am Sektorrand liegt schnell
	// ein paar Hundert Meter außerhalb des umschließenden Rechtecks.
	planStrayM = 2000
)

type planSector struct {
	Name     string          `json:"name"`
	Geometry json.RawMessage `json:"geometry"`
	AreaKM2  float64         `json:"areaKm2"`
}

type planHotspot struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
	Kind string  `json:"kind"`
	// Die Aufgabe für die Zielperson vor Ort. Im Datenmodell heißt das Feld
	// "notes" – der Name ist älter als die Idee, und umbenennen hieße, eine
	// Spalte zu wandern, die längst in Sicherungen steht.
	Notes string `json:"notes"`
}

type planPuzzle struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
	Hint     string `json:"hint"`
	Category string `json:"category"`
	Points   int    `json:"points"`
}

type plan struct {
	City     string        `json:"city"`
	Sectors  []planSector  `json:"sectors"`
	Hotspots []planHotspot `json:"hotspots"`
	Puzzles  []planPuzzle  `json:"puzzles"`

	// Was auffiel, ohne den Entwurf unbrauchbar zu machen. Die Oberfläche
	// zeigt es über der Liste an; entscheiden muss es ein Mensch.
	Warnings []string `json:"warnings,omitempty"`
}

// handlePlanPrompt liefert den Text zum Kopieren.
func handlePlanPrompt(e *core.RequestEvent) error {
	city := strings.TrimSpace(e.Request.URL.Query().Get("city"))
	if city == "" {
		city = "eurer Stadt"
	}
	return e.JSON(http.StatusOK, map[string]any{"prompt": planPrompt(Kuerzen(city, 80))})
}

type planRequest struct {
	Text string `json:"text"`
}

// handleParsePlan liest die Antwort der Maschine ein und gibt sie geprüft
// zurück – gespeichert wird nichts. Das übernehmen die gewöhnlichen Endpunkte,
// nachdem ein Mensch die Haken gesetzt hat.
func handleParsePlan(e *core.RequestEvent) error {
	var req planRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Anfrage nicht lesbar.", err)
	}

	out, err := parsePlan(req.Text)
	if err != nil {
		return e.BadRequestError(err.Error(), nil)
	}
	return e.JSON(http.StatusOK, out)
}

// parsePlan holt den Entwurf aus dem, was in der Zwischenablage lag.
//
// Drumherum steht fast immer noch etwas – eine Anrede, eine Erklärung, drei
// Backticks. Gesucht wird deshalb nicht die ganze Eingabe, sondern der Block
// von der ersten geschweiften Klammer bis zur letzten.
func parsePlan(text string) (*plan, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("Es wurde nichts eingefügt.")
	}
	if len(text) > maxPlanChars {
		return nil, fmt.Errorf("Der eingefügte Text ist zu lang. Bitte nur die Antwort der KI einfügen.")
	}

	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf(
			"In dem Text steckt kein JSON. Hat die KI den Block mit { … } ausgegeben?")
	}

	var raw struct {
		City    string `json:"city"`
		Sectors []struct {
			Name     string          `json:"name"`
			Geometry json.RawMessage `json:"geometry"`
		} `json:"sectors"`
		Hotspots []struct {
			Name string  `json:"name"`
			Lat  float64 `json:"lat"`
			Lng  float64 `json:"lng"`
			Kind string  `json:"kind"`
			Task string  `json:"task"`
		} `json:"hotspots"`
		Puzzles []planPuzzle `json:"puzzles"`
	}

	if err := json.Unmarshal([]byte(text[start:end+1]), &raw); err != nil {
		return nil, fmt.Errorf(
			"Das JSON ist unvollständig oder beschädigt (%v). "+
				"Häufigster Grund: Die Antwort wurde mitten im Satz abgeschnitten – "+
				"dann hilft es, die KI um den Rest zu bitten.", err)
	}

	out := &plan{City: Kuerzen(strings.TrimSpace(raw.City), 80)}

	if len(raw.Sectors) > maxPlanSectors {
		out.warn("Mehr als %d Sektoren – die übrigen wurden weggelassen.", maxPlanSectors)
		raw.Sectors = raw.Sectors[:maxPlanSectors]
	}
	if len(raw.Hotspots) > maxPlanHotspots {
		out.warn("Mehr als %d Hotspots – die übrigen wurden weggelassen.", maxPlanHotspots)
		raw.Hotspots = raw.Hotspots[:maxPlanHotspots]
	}
	if len(raw.Puzzles) > maxPlanPuzzles {
		out.warn("Mehr als %d Rätsel – die übrigen wurden weggelassen.", maxPlanPuzzles)
		raw.Puzzles = raw.Puzzles[:maxPlanPuzzles]
	}

	// Sektoren. Eine Grenze, die der Server nicht versteht, würde später jede
	// Standortzuordnung stillschweigend falsch beantworten – deshalb fliegt
	// sie hier raus und nicht erst beim Speichern.
	bounds := geo.Bounds{West: 180, South: 90, East: -180, North: -90}
	haveBounds := false

	for _, s := range raw.Sectors {
		name := Kuerzen(strings.TrimSpace(s.Name), 120)
		if name == "" {
			name = "Sektor ohne Namen"
		}

		area, err := geo.AreaFromGeoJSON(s.Geometry)
		if err != nil || len(area.Outer) < 3 {
			out.warn("%s: keine brauchbare Grenze, weggelassen.", name)
			continue
		}
		km2 := area.AreaM2() / 1_000_000
		if area.AreaM2() < minSectorAreaM2 {
			out.warn("%s: umschließt keine Fläche, weggelassen.", name)
			continue
		}
		if km2 > maxPlanSectorKM2 {
			out.warn("%s ist mit %.0f km² viel zu groß für einen Sektor. Bitte auf der Karte ansehen.", name, km2)
		}

		b := area.Outer.Bounds()
		bounds.West = math.Min(bounds.West, b.West)
		bounds.South = math.Min(bounds.South, b.South)
		bounds.East = math.Max(bounds.East, b.East)
		bounds.North = math.Max(bounds.North, b.North)
		haveBounds = true

		out.Sectors = append(out.Sectors, planSector{Name: name, Geometry: s.Geometry, AreaKM2: km2})
	}

	// Hotspots.
	for _, h := range raw.Hotspots {
		name := Kuerzen(strings.TrimSpace(h.Name), 120)
		if name == "" {
			out.warn("Ein Hotspot ohne Namen wurde weggelassen.")
			continue
		}
		if h.Lat == 0 && h.Lng == 0 {
			out.warn("%s: keine Koordinaten, weggelassen.", name)
			continue
		}
		if math.Abs(h.Lat) > 90 || math.Abs(h.Lng) > 180 {
			out.warn("%s: unmögliche Koordinaten, weggelassen.", name)
			continue
		}

		out.Hotspots = append(out.Hotspots, planHotspot{
			Name:  name,
			Lat:   h.Lat,
			Lng:   h.Lng,
			Kind:  planKind(h.Kind),
			Notes: Kuerzen(strings.TrimSpace(h.Task), 2000),
		})
	}

	// Ausreißer. Eine Maschine, die eine Koordinate errät, landet selten knapp
	// daneben – sie landet im Nachbarland. Das fällt auf der Karte zwar auch
	// auf, aber erst, wenn jemand weit genug herauszoomt.
	if haveBounds {
		if stray := strayNames(out.Hotspots, bounds); len(stray) > 0 {
			out.warn("Außerhalb der Sektoren: %s. Koordinaten prüfen.", strings.Join(stray, ", "))
		}
	}

	// Rätsel.
	for i, p := range raw.Puzzles {
		p.Title = Kuerzen(strings.TrimSpace(p.Title), 120)
		p.Question = Kuerzen(strings.TrimSpace(p.Question), 2000)
		p.Answer = Kuerzen(strings.TrimSpace(p.Answer), 500)
		p.Hint = Kuerzen(strings.TrimSpace(p.Hint), 500)
		p.Type = strings.ToUpper(strings.TrimSpace(p.Type))
		p.Category = strings.ToLower(strings.TrimSpace(p.Category))

		if p.Title == "" || p.Question == "" || p.Answer == "" {
			out.warn("Rätsel %d ist unvollständig (Titel, Frage oder Lösung fehlt), weggelassen.", i+1)
			continue
		}
		if !strings.Contains("ABCDEF", p.Type) || p.Type == "" {
			out.warn("Rätsel %q hat den unbekannten Typ %q, weggelassen.", p.Title, p.Type)
			continue
		}
		switch p.Category {
		case "green", "yellow", "red":
		default:
			p.Category = "yellow"
		}

		out.Puzzles = append(out.Puzzles, p)
	}

	if len(out.Sectors) == 0 && len(out.Hotspots) == 0 {
		return nil, fmt.Errorf(
			"In der Antwort stand weder ein Sektor noch ein Hotspot. " +
				"Vermutlich hat die KI das Format nicht eingehalten.")
	}
	if len(out.Hotspots) > 0 && len(out.Hotspots) < 8 {
		out.warn("Nur %d Hotspots. Für ein Spiel sollten es etwa zwanzig sein.", len(out.Hotspots))
	}

	return out, nil
}

func (p *plan) warn(format string, args ...any) {
	p.Warnings = append(p.Warnings, fmt.Sprintf(format, args...))
}

// planKind hält die Art des Ortes innerhalb der Auswahlliste des Datenmodells.
func planKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "landmark", "transit", "park", "building":
		return strings.ToLower(strings.TrimSpace(kind))
	default:
		return "other"
	}
}

// strayNames nennt die Hotspots, die weit außerhalb der Sektoren liegen.
func strayNames(spots []planHotspot, b geo.Bounds) []string {
	var out []string
	for _, h := range spots {
		// Abstand zum umschließenden Rechteck, grob in Metern.
		dLat := math.Max(0, math.Max(b.South-h.Lat, h.Lat-b.North)) * 111_320
		dLng := math.Max(0, math.Max(b.West-h.Lng, h.Lng-b.East)) *
			111_320 * math.Cos(h.Lat*math.Pi/180)
		if math.Hypot(dLat, dLng) > planStrayM {
			out = append(out, h.Name)
		}
	}
	return out
}

// planPrompt beschreibt das Spiel und verlangt eine Antwort in genau dem
// Format, das parsePlan wieder einliest.
//
// Der Text ist lang, und das ist Absicht: Eine Maschine, die das Spiel nicht
// kennt, erfindet Aufgaben, die man nicht machen kann – Museen mit Eintritt,
// Türme mit Öffnungszeiten, Kirchen während des Gottesdienstes. Jeder Satz hier
// hat einen Fehlschlag verhindert, den es ohne ihn gäbe.
func planPrompt(city string) string {
	return strings.ReplaceAll(planPromptVorlage, "{{STADT}}", city)
}

const planPromptVorlage = `Du planst ein Stadtspiel in {{STADT}}. Recherchiere dafür, wenn du kannst, im Netz; verlasse dich nicht auf dein Gedächtnis.

DAS SPIEL

Eine Zielperson ("Mister X") bewegt sich über mehrere Stunden durch die Stadt und erfüllt unterwegs an markanten Orten Aufgaben. Zwei bis vier Fahndungsteams suchen sie, eine Einsatzzentrale koordiniert. Gespielt wird zu Fuß und mit Bus und Bahn, drei bis acht Stunden, meist am Wochenende, oft mit Jugendlichen und Erwachsenen gemischt.

Das Spielfeld besteht aus zwei Dingen:
- SEKTOREN: benannte Stadtbereiche. Hinweise lauten "gesehen in Sektor C" – die Sektoren müssen also so geschnitten sein, dass diese Aussage etwas wert ist.
- HOTSPOTS: nummerierte Orte, die alle auf der Karte haben. Die Zielperson reist von Hotspot zu Hotspot, die Fahndung versucht, den nächsten zu erraten.

Die Fahndungsteams lösen außerdem RÄTSEL, um Hinweise auf den Aufenthalt der Zielperson freizuschalten.

WAS DU LIEFERN SOLLST

1. SEKTOREN – fünf bis acht Stück
   - Jeder ein bis drei Quadratkilometer, zu Fuß in etwa zwanzig Minuten zu durchqueren.
   - Orientiere dich an echten Stadtteilen und an Kanten, die man sieht: Fluss, Bahnlinie, große Straße, Park. Ein Sektor, dessen Grenze niemand erkennt, taugt nicht für den Satz "gesehen in Sektor C".
   - Zusammenhängend, ohne Überlappung, um das lebendige Zentrum herum. Wohnsiedlungen, Gewerbegebiete und Wald gehören nicht dazu: Dort gibt es nichts zu finden und keine Bahn zurück.
   - Als Grenze ein GeoJSON-Polygon mit vier bis zwölf Ecken. Reihenfolge [Längengrad, Breitengrad], erster Punkt gleich letzter Punkt.

2. HOTSPOTS – zwanzig bis vierundzwanzig Stück
   - Öffentlich, jederzeit frei zugänglich, ohne Eintritt und ohne Öffnungszeiten. Also: Plätze, Brücken, Denkmäler, Parks, Haltestellen, Marktplätze, Vorplätze, Aussichtspunkte. Nicht: Museen, Türme, Kirchenschiffe, Ladengeschäfte, Privatgrundstücke.
   - Über alle Sektoren verteilt, je Sektor drei bis fünf.
   - Mit Bahn oder Bus erreichbar, untereinander 300 Meter bis 2 Kilometer auseinander.
   - Eindeutig erkennbar: Wer davorsteht, muss sicher sein, am richtigen Ort zu sein.
   - Koordinaten in Dezimalgrad (WGS84) mit mindestens fünf Nachkommastellen. Wenn du dir bei einem Ort nicht sicher bist, lass ihn weg – ein erfundener Hotspot schickt eine Gruppe echter Menschen an eine Kreuzung, an der nichts ist.

3. AUFGABEN FÜR DIE ZIELPERSON – eine je Hotspot, Feld "task"
   - Zwei bis fünf Minuten, allein zu erledigen, mit einem Foto zu belegen.
   - Muss an genau diesem Ort stattfinden und etwas mit ihm zu tun haben: eine Inschrift abschreiben, ein Detail zählen, ein Foto aus einem bestimmten Blickwinkel.
   - Nichts, was auffällt, nichts, wofür man jemanden ansprechen muss, nichts, was Geld kostet, nichts Verbotenes, nichts Gefährliches: kein Klettern, kein Betreten von Gleisen, kein Wasser.
   - Ein Satz, in der Du-Form, ohne Umschweife.

4. RÄTSEL FÜR DIE FAHNDUNG – acht bis zwölf Stück
   Jedes Rätsel führt auf einen Ort aus deiner Hotspot-Liste. Sechs Typen:
     A Ausschluss   – mehrere Angaben schließen nach und nach alles bis auf einen Ort aus
     B Foto         – ein Bildausschnitt, der erkannt werden muss (die Spielleitung ergänzt das Bild später selbst)
     C Rechnung     – Zahlen aus der Stadt, die zu einer Lösung führen: Jahreszahlen, Stufen, Hausnummern
     D Geometrie    – Richtungen und Abstände auf der Karte
     E Vor Ort      – nur zu lösen, wenn jemand tatsächlich dort steht und etwas abliest
     F Kombination  – setzt die Ergebnisse mehrerer anderer Rätsel zusammen
   Mische die Typen. Zu jedem Rätsel:
     - "answer": die Lösung. Mehrere zulässige Schreibweisen mit senkrechtem Strich trennen, z. B. "Goldener Reiter|Der Goldene Reiter". Verglichen wird ohne Rücksicht auf Groß- und Kleinschreibung.
     - "hint": ein Tipp, der weiterhilft, ohne die Lösung zu verraten.
     - "category": wie belastbar der Hinweis ist, den das Rätsel freischaltet – "green" (stimmt), "yellow" (stimmt im Kern, nicht im Detail) oder "red" (grenzt nur ein).
     - "points": 10 bis 25, schwierigere mehr.
   Prüfe jede Rechnung nach. Ein Rätsel mit falscher Lösung blockiert eine Gruppe für eine halbe Stunde.

FORMAT

Antworte mit genau einem JSON-Block und sonst nichts – keine Einleitung, keine Erklärung danach, keine Kommentare im JSON:

{
  "city": "{{STADT}}",
  "sectors": [
    {
      "name": "Innere Altstadt",
      "geometry": {"type":"Polygon","coordinates":[[[13.73156,51.04662],[13.74690,51.04732],[13.74674,51.05452],[13.73071,51.05432],[13.73156,51.04662]]]}
    }
  ],
  "hotspots": [
    {
      "name": "Goldener Reiter",
      "lat": 51.05752,
      "lng": 13.74198,
      "kind": "landmark",
      "task": "Schreibe die Jahreszahl vom Sockel des Standbilds ab und fotografiere sie."
    }
  ],
  "puzzles": [
    {
      "type": "C",
      "title": "Zahlenschloss",
      "question": "Die Frauenkirche wurde 1743 vollendet, 1945 zerstört und 2005 geweiht. Addiere die drei Jahreszahlen und bilde vom Ergebnis die Quersumme.",
      "answer": "24",
      "hint": "1743 + 1945 + 2005 = 5693. Und davon die Quersumme.",
      "category": "red",
      "points": 15
    }
  ]
}

Erlaubte Werte für "kind": landmark, transit, park, building, other.
Erlaubte Werte für "type": A, B, C, D, E, F. Für "category": green, yellow, red.

Lieber weniger Einträge als erfundene. Alles, was du lieferst, prüft am Ende ein Mensch auf einer Karte nach – aber nur das, was falsch aussieht, fällt dabei auf.`
