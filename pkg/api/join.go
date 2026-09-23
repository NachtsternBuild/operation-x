package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	qrcode "github.com/skip2/go-qrcode"

	"github.com/elias/operation-x/pkg/tunnel"
)

// tun ist der Tunnel dieses Serverlaufs.
var tun *tunnel.Tunnel

// SetTunnel hinterlegt die Tunnelverwaltung. Wird beim Serverstart aufgerufen.
func SetTunnel(t *tunnel.Tunnel) { tun = t }

// StopTunnel schließt einen laufenden Tunnel.
//
// Aufzurufen beim Beenden des Servers. cloudflared ist ein eigener Prozess:
// Stirbt der Spielserver, läuft er weiter und hält eine öffentliche Adresse
// offen, hinter der nichts mehr ist. Wer das Fenster schließt, geht davon aus,
// dass alles aus ist – und soll damit recht behalten.
func StopTunnel() {
	if tun != nil {
		tun.Stop()
	}
}

// registerJoin hängt die Endpunkte für Beitritt und Verteilung ein.
func registerJoin(se *core.ServeEvent) {
	g := se.Router.Group("/api/opx")

	// Öffentlich: Wer den QR-Code scannt, ist noch nicht angemeldet.
	g.GET("/join", handleJoinInfo)
	g.GET("/qr", handleQRCode)

	// Die App liegt neben der Serverdatei und wird von dort ausgeliefert.
	// Damit reicht ein QR-Code für alles: Wer ihn scannt, kommt auf die
	// Beitrittsseite und findet dort den Download.
	se.Router.GET("/operation-x.apk", handleAppDownload)

	// Die Tunnelsteuerung gehört der Einsatzzentrale.
	hq := g.Group("/hq")
	hq.Bind(requireAuthTeams())
	hq.BindFunc(requireRole("hq"))
	hq.GET("/tunnel", handleTunnelState)
	hq.POST("/tunnel/start", handleTunnelStart)
	hq.POST("/tunnel/stop", handleTunnelStop)
}

type joinInfo struct {
	Ready     bool   `json:"ready"`
	GameName  string `json:"gameName,omitempty"`
	City      string `json:"city,omitempty"`
	JoinURL   string `json:"joinUrl"`
	HasApp    bool   `json:"hasApp"`
	AppURL    string `json:"appUrl,omitempty"`
	Tunnel    bool   `json:"tunnelActive"`
	LocalOnly bool   `json:"localOnly"`
}

// handleJoinInfo beschreibt, wie man in dieses Spiel kommt.
func handleJoinInfo(e *core.RequestEvent) error {
	info := joinInfo{}

	if game, err := SpielDesServers(e.App); err == nil && game != nil {
		info.Ready = true
		info.GameName = game.GetString("name")
		info.City = game.GetString("city")
	}

	info.JoinURL = publicURL(e)
	info.Tunnel = tun != nil && tun.URL() != ""
	// Ohne Tunnel ist der Server nur im eigenen Netz erreichbar – und Browser
	// geben über eine unverschlüsselte Adresse keine Standortdaten heraus.
	info.LocalOnly = !info.Tunnel

	if _, ok := apkPath(); ok {
		info.HasApp = true
		info.AppURL = info.JoinURL + "/operation-x.apk"
	}

	return e.JSON(http.StatusOK, info)
}

// handleQRCode liefert den Beitritts-QR-Code als Bild.
//
// Er trägt die öffentliche Adresse. Wer ihn scannt, landet auf der
// Beitrittsseite – im Browser oder, wenn die App installiert ist, dort.
func handleQRCode(e *core.RequestEvent) error {
	target := e.Request.URL.Query().Get("url")
	if target == "" {
		target = publicURL(e)
	}
	if target == "" {
		return e.BadRequestError("Es ist noch keine Adresse bekannt.", nil)
	}

	png, err := qrcode.Encode(target, qrcode.Medium, 640)
	if err != nil {
		return e.InternalServerError("QR-Code konnte nicht erzeugt werden.", err)
	}

	e.Response.Header().Set("Cache-Control", "no-store")
	return e.Blob(http.StatusOK, "image/png", png)
}

func handleTunnelState(e *core.RequestEvent) error {
	if tun == nil {
		return e.JSON(http.StatusOK, map[string]any{"running": false})
	}

	state := tun.State()
	return e.JSON(http.StatusOK, map[string]any{
		"running":     state.Running,
		"url":         state.URL,
		"message":     state.Message,
		"binary":      state.Binary,
		"installHint": tunnel.InstallHint(runtime.GOOS),
	})
}

func handleTunnelStart(e *core.RequestEvent) error {
	if err := darfTunnelSchalten(e); err != nil {
		return err
	}
	if tun == nil {
		return e.InternalServerError("Die Tunnelverwaltung steht nicht bereit.", nil)
	}

	// Auf denselben Port, auf dem dieser Server lauscht.
	local := "http://" + e.Request.Host
	if strings.HasPrefix(e.Request.Host, "0.0.0.0") {
		local = strings.Replace(local, "0.0.0.0", "127.0.0.1", 1)
	}

	if err := tun.Start(local); err != nil {
		return e.BadRequestError(err.Error(), map[string]any{
			"installHint": tunnel.InstallHint(runtime.GOOS),
		})
	}

	return e.JSON(http.StatusOK, map[string]any{"running": true})
}

func handleTunnelStop(e *core.RequestEvent) error {
	if err := darfTunnelSchalten(e); err != nil {
		return err
	}
	if tun != nil {
		tun.Stop()
	}
	return e.JSON(http.StatusOK, map[string]any{"running": false})
}

// handleAppDownload liefert die Android-App.
func handleAppDownload(e *core.RequestEvent) error {
	path, ok := apkPath()
	if !ok {
		return e.NotFoundError(
			"Die App liegt nicht bereit. Sie gehört als operation-x.apk neben die Serverdatei.", nil)
	}

	e.Response.Header().Set("Content-Type", "application/vnd.android.package-archive")
	e.Response.Header().Set("Content-Disposition", `attachment; filename="operation-x.apk"`)
	http.ServeFile(e.Response, e.Request, path)
	return nil
}

// publicURL liefert die Adresse, unter der Spieler den Server erreichen.
func publicURL(e *core.RequestEvent) string {
	if tun != nil {
		if url := tun.URL(); url != "" {
			return url
		}
	}

	scheme := "http"
	if e.Request.TLS != nil || e.Request.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, e.Request.Host)
}

// apkPath sucht die Android-App, damit der Server sie zum Herunterladen
// anbieten kann.
//
// Gesucht wird neben der Serverdatei, nicht im Arbeitsverzeichnis: Wer das
// Programm anklickt statt es in einer Eingabeaufforderung zu starten, hat
// selten das Verzeichnis stehen, in dem es liegt. PocketBase legt pb_data aus
// demselben Grund neben die Serverdatei; die App folgt dieser Regel.
func apkPath() (string, bool) {
	dirs := []string{""}
	if base := execDir(); base != "" {
		dirs = append([]string{base}, dirs...)
	}

	for _, dir := range dirs {
		for _, name := range []string{
			filepath.Join("pb_public", "operation-x.apk"),
			"operation-x.apk",
		} {
			candidate := filepath.Join(dir, name)
			if fileExists(candidate) {
				return candidate, true
			}
		}
	}
	return "", false
}

// execDir liefert das Verzeichnis der laufenden Serverdatei.
func execDir() string {
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		return filepath.Dir(exe)
	}
	return ""
}

// darfTunnelSchalten entscheidet, wer den Weg ins Internet auf- und zumacht.
//
// Es gibt einen Tunnel je Server, nicht je Spiel. Auf diesem Server ist die
// Zentrale zugleich der Betreiber, also darf sie ihn schalten – wie bei der
// Sicherung entscheidet sonst der Haken aus aufsatz.go.
func darfTunnelSchalten(e *core.RequestEvent) error {
	return serverSache(e)
}
