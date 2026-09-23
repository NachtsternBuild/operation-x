package citysets

import "testing"

// Der Name kommt aus einer Anfrage von außen und landet in einem Dateipfad.
// Auch wenn eingebettete Dateien nicht aus dem Programm herausführen können:
// Diese Prüfung kostet nichts und muss stehen bleiben, wenn die Pakete
// irgendwann doch aus einem Ordner gelesen werden.
func TestPfadangabenWerdenAbgewiesen(t *testing.T) {
	böse := []string{
		"../../etc/passwd",
		"..",
		".",
		"dresden/../../geheim",
		"unter/ordner",
		`c:\pfad`,
		"",
	}

	for _, fall := range böse {
		if _, err := Get(fall); err == nil {
			t.Errorf("%q wurde angenommen", fall)
		}
	}
}

// Was mitgeliefert wird, muss auch lesbar sein – ein beschädigtes Paket fällt
// sonst erst am Spieltag auf, wenn jemand es anklickt.
func TestMitgeliefertePaketeSindBrauchbar(t *testing.T) {
	sets := List()
	if len(sets) == 0 {
		t.Skip("keine Pakete mitgeliefert")
	}

	for _, s := range sets {
		pack, err := Get(s.Slug)
		if err != nil {
			t.Errorf("%s ließ sich nicht lesen: %v", s.Slug, err)
			continue
		}

		if len(pack.Sectors) < 3 {
			t.Errorf("%s hat nur %d Sektoren – ein Hinweis auf den Sektor wäre wertlos",
				s.City, len(pack.Sectors))
		}
		if len(pack.Hotspots) < 8 {
			t.Errorf("%s hat nur %d Hotspots – zu wenig für neun Zwischenziele",
				s.City, len(pack.Hotspots))
		}

		for _, h := range pack.Hotspots {
			if h.Name == "" {
				t.Errorf("%s: ein Hotspot ohne Namen", s.City)
			}
			if h.Lat == 0 && h.Lng == 0 {
				t.Errorf("%s: Hotspot %q ohne Koordinaten", s.City, h.Name)
			}
		}

		codes := map[string]bool{}
		for _, sec := range pack.Sectors {
			if codes[sec.Code] {
				t.Errorf("%s: Sektorkennung %q doppelt", s.City, sec.Code)
			}
			codes[sec.Code] = true
			if len(sec.Geometry) < 10 {
				t.Errorf("%s: Sektor %q ohne Grenze", s.City, sec.Name)
			}
		}
	}
}
