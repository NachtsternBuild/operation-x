package api

import (
	"errors"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/geo"
	"github.com/elias/operation-x/pkg/schema"
	"github.com/elias/operation-x/pkg/tiles"
)

// Die Karte kommt jetzt vom Spielserver.
//
// Vorher holte sich jedes Gerät jede Kachel selbst von OpenStreetMap: acht
// Telefone, dieselben Kacheln, achtmal über Mobilfunk – und im Funkloch eine
// leere Fläche, auch dort, wo eine Minute vorher schon jemand hingesehen hat.
//
// Der Server legt jede Kachel ab, die er einmal geholt hat. Das zweite Gerät
// bekommt sie aus dem Ordner, und nach dem ersten Rundgang liegt das
// Spielgebiet vollständig dort – ohne dass irgendwo ein Gebiet vorab
// heruntergeladen wurde, was die Bedingungen von OpenStreetMap untersagen.
//
// Der Weg ist bewusst offen und ohne Anmeldung: Die Karte fragt ihre Kacheln
// ohne Zugangstoken ab, so ist das Kartenformat gebaut. Dafür sind Zoomstufe
// und Spielgebiet die Grenze – der Server ist kein Kartenvermittler für
// beliebige Gegenden.

var tileStore *tiles.Store

// setupTiles legt den Kachelspeicher an.
func setupTiles(app core.App) {
	// Ein mitgebrachter Kachelsatz liegt neben dem Programm, nicht im
	// Datenordner: Er gehört zur Ausstattung, nicht zum einzelnen Spiel, und
	// überlebt damit auch ein "pb_data löschen und neu einrichten".
	local := ""
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "kacheln")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			local = candidate
		}
	}

	tileStore = tiles.New(
		filepath.Join(app.DataDir(), "kacheln"),
		local,
		func() (geo.Bounds, bool) { return spielgebiet(app) },
	)
}

// spielgebiet liefert das Rechteck, in dem gespielt wird.
//
// Es begrenzt, welche Kacheln der Server überhaupt holen darf – die
// Nutzungsbedingungen von OpenStreetMap erlauben den laufenden Bedarf eines
// Spiels, aber kein Absaugen der Weltkarte.
//
// Gesucht wird über alle Spiele in der Datenbank – in der Regel eines, und
// dann ist es dessen Rechteck. Liegen mehrere darin, ist es das Rechteck um
// alle: grob, aber ein Deckel gegen Unfug und keine Abrechnung. Geholt wird
// ohnehin nur, was jemand wirklich ansieht.
func spielgebiet(app core.App) (geo.Bounds, bool) {
	games, err := app.FindRecordsByFilter(
		schema.ColGames, "status != {:finished}", "", 0, 0,
		map[string]any{"finished": schema.GameFinished},
	)
	if err != nil {
		return geo.Bounds{}, false
	}

	out := geo.Bounds{West: 180, South: 90, East: -180, North: -90}
	gefunden := false

	for _, gameRec := range games {
		b, err := gameBounds(app, gameRec)
		if err != nil {
			continue
		}
		out.West = math.Min(out.West, b.West)
		out.South = math.Min(out.South, b.South)
		out.East = math.Max(out.East, b.East)
		out.North = math.Max(out.North, b.North)
		gefunden = true
	}

	return out, gefunden
}

// handleTile liefert eine Kachel.
func handleTile(e *core.RequestEvent) error {
	if tileStore == nil {
		return e.NotFoundError("Kein Kachelspeicher eingerichtet.", nil)
	}

	z, err1 := strconv.Atoi(e.Request.PathValue("z"))
	x, err2 := strconv.Atoi(e.Request.PathValue("x"))
	y, err3 := strconv.Atoi(e.Request.PathValue("y"))
	if err1 != nil || err2 != nil || err3 != nil {
		return e.BadRequestError("Unlesbare Kachelnummer.", nil)
	}

	data, err := tileStore.Tile(e.Request.Context(), z, x, y)
	if err != nil {
		if errors.Is(err, tiles.ErrOutside) {
			return e.NotFoundError("Diese Kachel gehört nicht zum Spielgebiet.", nil)
		}
		// Kein Netz und nicht im Speicher: Das ist keine Störung des Servers,
		// sondern eine fehlende Kachel. Die Karte lässt die Fläche dann leer,
		// statt eine Fehlermeldung über das Spielfeld zu legen.
		return e.NotFoundError("Diese Kachel liegt nicht vor.", nil)
	}

	e.Response.Header().Set("Content-Type", "image/png")
	// Ein Tag im Browser: Kacheln ändern sich nicht im Lauf eines Spieltags,
	// und jede Anfrage weniger ist Akku.
	e.Response.Header().Set("Cache-Control", "public, max-age=86400")
	_, err = e.Response.Write(data)
	return err
}

// handleTileStat sagt der Zentrale, wie viel Karte schon im Haus ist.
func handleTileStat(e *core.RequestEvent) error {
	if tileStore == nil {
		return e.JSON(http.StatusOK, tiles.Stat{})
	}
	return e.JSON(http.StatusOK, tileStore.Stat())
}
