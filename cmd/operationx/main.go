// Kommando operationx ist der Spielserver von Operation X.
//
// Eine einzige Binary enthält Datenbank, Webserver, Spiel-Engine und die
// Weboberfläche. Start ohne Argumente: "operationx serve".
//
// Der eigentliche Server steht in pkg/server – hier bleibt nur, was diese
// Ausgabe von einer anderen unterscheidet: ihre Version.
package main

import (
	"log"

	"github.com/elias/operation-x/pkg/server"
)

// version wird beim Bauen gesetzt (siehe scripts/build-release.sh). Ohne diese
// Variable liefe das -X im Linker ins Leere: Go meldet ein unbekanntes Ziel
// nicht, es passiert dann einfach nichts.
var version = "dev"

func main() {
	app := server.Neu(server.Optionen{Version: version})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
