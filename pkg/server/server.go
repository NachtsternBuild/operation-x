// Package server setzt den Spielserver zusammen.
//
// Alles, was ein lauffähiger Server braucht: Datenmodell, Endpunkte, Engine,
// Trockenübung, Tunnel, die Unterbefehle und die Oberfläche. Aus einer
// main-Funktion herausgelöst, damit ein aufbauendes Programm denselben Server
// starten kann, ohne ihn abzuschreiben – es übergibt nur seine eigene
// Oberfläche und setzt vorher die Haken aus pkg/api.
package server

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"

	"github.com/elias/operation-x/pkg/api"
	"github.com/elias/operation-x/pkg/game"
	"github.com/elias/operation-x/pkg/schema"
	"github.com/elias/operation-x/pkg/seed"
	"github.com/elias/operation-x/pkg/sim"
	"github.com/elias/operation-x/pkg/tunnel"
	"github.com/elias/operation-x/web"
)

// Optionen sind die Stellschrauben, an denen sich ein aufbauendes Programm
// von diesem hier unterscheidet.
type Optionen struct {
	// Version steht in "operationx --version".
	Version string

	// Oberflaeche ist die ausgelieferte Weboberfläche. Leer heißt: die
	// eingebaute. Ein aufbauendes Programm hat eine eigene, die zusätzliche
	// Ansichten kennt.
	Oberflaeche fs.FS

	// Begruessung ist, was nach dem Start im Fenster steht. Leer heißt: die
	// eingebaute.
	Begruessung func(se *core.ServeEvent)
}

// greet sagt in zwei Sätzen, was jetzt zu tun ist.
//
// Das ist für viele Leute die einzige Zeile Text, die sie von diesem Programm
// je zu Gesicht bekommen – das schwarze Fenster bleibt den ganzen Spieltag
// offen. Sie muss deshalb die eine Frage beantworten, die sich in diesem
// Moment stellt: Wohin jetzt?
func greet(se *core.ServeEvent) {
	addr := se.Server.Addr
	if strings.HasPrefix(addr, "0.0.0.0") {
		addr = strings.Replace(addr, "0.0.0.0", "127.0.0.1", 1)
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}

	fmt.Println()
	fmt.Println("  Operation X läuft.")
	fmt.Println()
	fmt.Printf("  Im Browser öffnen:  http://%s\n", addr)
	fmt.Println()
	fmt.Println("  Dieses Fenster offen lassen – wird es geschlossen, ist der")
	fmt.Println("  Server aus. Zum Beenden: Strg+C")
	fmt.Println()
}

// Neu baut den Server zusammen. Gestartet wird er vom Aufrufer.
func Neu(o Optionen) *pocketbase.PocketBase {

	// Die Startmeldung von PocketBase bleibt aus.
	//
	// Sie ist für Entwickler geschrieben und schickt den Leser als Erstes in
	// ein Administratorkonto ("create your first superuser account"), das mit
	// dem Spiel nichts zu tun hat. Wer diese Datei anklickt, um einen
	// Geburtstag vorzubereiten, folgt genau diesem Hinweis und landet in einer
	// Datenbankverwaltung, die er nie wieder verlassen möchte.
	app := pocketbase.NewWithConfig(pocketbase.Config{
		HideStartBanner: true,
	})

	// Sonst stünde hier die Version von PocketBase, nicht die des Spielservers.
	app.RootCmd.Version = o.Version

	// Das Datenmodell wird bei jedem Start abgeglichen. Idempotent, damit ein
	// Update der Binary auch eine bestehende Datenbank mitzieht.
	app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		return schema.Ensure(e.App)
	})

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		api.Register(se)

		// Die Spiel-Engine läuft, solange der Server läuft: Sie prüft im
		// Hintergrund, welche Fristen abgelaufen und welche Folgen fällig sind.
		game.New(se.App).Start(context.Background())

		// Die Trockenübung wird von der Einsatzzentrale gestartet, läuft aber
		// im Serverprozess – siehe internal/sim.
		runner := sim.New(se.App)
		runner.SetMissionMaker(api.EnsureMission)
		runner.SetPuzzleSolver(api.SolvePuzzleFor)
		api.SetRunner(runner)

		// Der Tunnel macht den Server von unterwegs erreichbar. Gestartet wird
		// er von der Einsatzzentrale, nicht automatisch – wer nur einrichtet,
		// braucht ihn nicht.
		api.SetTunnel(tunnel.New())

		// Die Oberfläche liegt eingebettet in der Binary. Der Fallback auf
		// index.html macht daraus eine SPA – muss zuletzt gebunden werden,
		// damit die eigenen Routen Vorrang behalten.
		oberflaeche := o.Oberflaeche
		if oberflaeche == nil {
			oberflaeche = web.DistFS()
		}
		se.Router.GET("/{path...}", apis.Static(oberflaeche, true))

		// PocketBase öffnet beim ersten Start von sich aus einen Browser – auf
		// seiner eigenen Datenbankverwaltung, mit der Aufforderung, dort ein
		// Administratorkonto anzulegen. Wer diese Datei angeklickt hat, um
		// einen Geburtstag vorzubereiten, bekäme zwei Fenster zu sehen, und das
		// falsche zuerst.
		//
		// Ein Administratorkonto braucht dieses Programm nicht: Alles, was die
		// Spielleitung tut, tut sie in der eigenen Oberfläche.
		se.InstallerFunc = nil

		if o.Begruessung != nil {
			o.Begruessung(se)
		} else {
			greet(se)
		}

		return se.Next()
	})

	// Beim Beenden aufräumen: cloudflared ist ein eigener Prozess und liefe
	// sonst weiter, wenn jemand das Fenster schließt.
	app.OnTerminate().BindFunc(func(e *core.TerminateEvent) error {
		api.StopTunnel()
		return e.Next()
	})

	app.RootCmd.AddCommand(&cobra.Command{
		Use:   "seed",
		Short: "Legt ein Testspiel in Dresden mit Teams und Hotspots an",
		Run: func(cmd *cobra.Command, args []string) {
			if err := app.Bootstrap(); err != nil {
				log.Fatal(err)
			}
			if err := seed.Dresden(app); err != nil {
				log.Fatal(err)
			}
		},
	})

	districtsCmd := &cobra.Command{
		Use:   "districts <Stadt>",
		Short: "Zeigt die Ortsteile einer Stadt als Grundlage für den Sektorschnitt",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			levels, _ := cmd.Flags().GetIntSlice("levels")
			city := args[0]
			for _, extra := range args[1:] {
				city += " " + extra
			}
			if err := seed.PrintDistricts(city, levels); err != nil {
				log.Fatal(err)
			}
		},
	}
	app.RootCmd.AddCommand(&cobra.Command{
		Use:   "puzzles",
		Short: "Legt Beispielrätsel der Typen A bis F für das laufende Spiel an",
		Run: func(cmd *cobra.Command, args []string) {
			if err := app.Bootstrap(); err != nil {
				log.Fatal(err)
			}
			if err := seed.AddPuzzles(app); err != nil {
				log.Fatal(err)
			}
		},
	})

	configCmd := &cobra.Command{
		Use:   "config [Schlüssel] [Wert]",
		Short: "Zeigt die Regelwerte des Spiels oder ändert einen davon",
		Args:  cobra.MaximumNArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			if err := app.Bootstrap(); err != nil {
				log.Fatal(err)
			}
			var err error
			switch len(args) {
			case 0:
				err = seed.ShowConfig(app)
			case 2:
				err = seed.SetConfig(app, args[0], args[1])
			default:
				err = fmt.Errorf("entweder ohne Argumente aufrufen oder mit Schlüssel und Wert")
			}
			if err != nil {
				log.Fatal(err)
			}
		},
	}
	app.RootCmd.AddCommand(configCmd)

	districtsCmd.Flags().IntSlice("levels", []int{9, 10, 11},
		"Verwaltungsebenen: 9 grobe Bezirke, 11 meist die brauchbaren Stadtteile")
	app.RootCmd.AddCommand(districtsCmd)

	return app
}
