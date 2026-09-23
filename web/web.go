// Package web bindet die gebaute Weboberfläche in die Binary ein.
//
// Damit bleibt es bei einer einzigen ausführbaren Datei – dem Kern des
// Selbsthosting-Versprechens aus dem Konzept. Der Ordner dist entsteht durch
// "npm run build" in diesem Verzeichnis.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// DistFS liefert die gebaute Oberfläche als Dateisystem ab dem Wurzelverzeichnis.
func DistFS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// Kann nur bei einem kaputten Build auftreten – das ist ein
		// Programmierfehler, kein Laufzeitzustand.
		panic(err)
	}
	return sub
}
