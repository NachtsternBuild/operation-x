// Package tunnel macht den Server von unterwegs erreichbar.
//
// Das ist das erste der drei harten Probleme aus dem Konzept: Ein Laptop im
// heimischen WLAN ist aus dem Mobilfunknetz nicht erreichbar. Portfreigabe
// setzt Wissen voraus, das laut Anforderung niemand haben muss – und scheitert
// bei vielen Anschlüssen ohnehin an CGNAT, weil gar keine eigene öffentliche
// Adresse mehr existiert.
//
// Verschärfend kommt hinzu: Browser geben Standortdaten nur in einem sicheren
// Kontext heraus. Über http://192.168.x.x liefert die Ortung schlicht nichts,
// und ohne Ortung ist das ganze Spiel hinfällig.
//
// Die Lösung ist ein ausgehender Tunnel. Er baut die Verbindung von innen nach
// außen auf – durch jeden Router, ohne Konfiguration – und liefert eine
// öffentliche Adresse mit gültigem Zertifikat.
package tunnel

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// urlPattern findet die zugeteilte Adresse in der Ausgabe von cloudflared.
var urlPattern = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)

// State beschreibt den Zustand des Tunnels für die Oberfläche.
type State struct {
	Running bool   `json:"running"`
	URL     string `json:"url,omitempty"`
	Message string `json:"message,omitempty"`
	Binary  string `json:"binary,omitempty"`
}

// Tunnel verwaltet den cloudflared-Prozess.
type Tunnel struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	cancel  context.CancelFunc
	url     string
	message string
	running bool

	// Wird gerufen, sobald die Adresse feststeht.
	OnReady func(url string)
}

func New() *Tunnel { return &Tunnel{} }

// Binary sucht cloudflared auf dem Rechner.
//
// Bewusst kein automatischer Download: Ein Programm, das im Hintergrund
// Binärdateien nachlädt, ist genau das, was man einem Selbsthosting-Werkzeug
// nicht zutrauen möchte. Fehlt es, sagt die Oberfläche, woher es kommt.
func Binary() (string, error) {
	for _, name := range []string{"cloudflared", "cloudflared.exe"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("cloudflared nicht gefunden")
}

func (t *Tunnel) State() State {
	t.mu.Lock()
	defer t.mu.Unlock()

	state := State{Running: t.running, URL: t.url, Message: t.message}
	if path, err := Binary(); err == nil {
		state.Binary = path
	}
	return state
}

// Start öffnet einen Schnelltunnel auf den lokalen Port.
//
// Der Schnelltunnel braucht kein Konto und vergibt eine zufällige Adresse, die
// bis zum Beenden gilt. Für einen Spieltag ist das genau richtig; als
// Dauerlösung ist er nicht gedacht und auch nicht zugesagt.
func (t *Tunnel) Start(localAddr string) error {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return fmt.Errorf("der Tunnel läuft bereits")
	}
	t.mu.Unlock()

	binary, err := Binary()
	if err != nil {
		return fmt.Errorf(
			"cloudflared ist nicht installiert. Es stammt von Cloudflare und wird " +
				"gebraucht, um den Server von unterwegs erreichbar zu machen")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binary, "tunnel", "--url", localAddr, "--no-autoupdate")

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("cloudflared ließ sich nicht starten: %w", err)
	}

	t.mu.Lock()
	t.cmd = cmd
	t.cancel = cancel
	t.running = true
	t.url = ""
	t.message = "Tunnel wird aufgebaut …"
	t.mu.Unlock()

	// cloudflared schreibt die Adresse nach stderr, andere Fassungen nach
	// stdout – deshalb werden beide gelesen.
	go t.watch(stderr)
	go t.watch(stdout)

	go func() {
		_ = cmd.Wait()
		t.mu.Lock()
		t.running = false
		if t.url == "" {
			t.message = "Der Tunnel wurde beendet, bevor eine Adresse feststand."
		} else {
			t.message = "Tunnel beendet."
		}
		t.mu.Unlock()
	}()

	return nil
}

func (t *Tunnel) watch(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		if match := urlPattern.FindString(line); match != "" {
			t.mu.Lock()
			first := t.url == ""
			t.url = match
			t.message = "Erreichbar."
			ready := t.OnReady
			t.mu.Unlock()

			if first && ready != nil {
				ready(match)
			}
			continue
		}

		// Die häufigsten Störungen in Klartext übersetzen.
		lower := strings.ToLower(line)
		switch {
		case strings.Contains(lower, "failed to sufficiently increase receive buffer"):
			// Nur eine Warnung, keine Störung – nicht weitermelden.
		case strings.Contains(lower, "error") && strings.Contains(lower, "connect"):
			t.setMessage("Keine Verbindung zu Cloudflare. Internet prüfen.")
		}
	}
}

func (t *Tunnel) setMessage(msg string) {
	t.mu.Lock()
	t.message = msg
	t.mu.Unlock()
}

// Stop beendet den Tunnel.
func (t *Tunnel) Stop() {
	t.mu.Lock()
	cancel := t.cancel
	t.cancel = nil
	t.running = false
	t.url = ""
	t.message = ""
	t.mu.Unlock()

	if cancel != nil {
		cancel()
		// Kurz warten, damit der Prozess sauber abräumt.
		time.Sleep(200 * time.Millisecond)
	}
}

// URL liefert die öffentliche Adresse, sofern schon bekannt.
func (t *Tunnel) URL() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.url
}

// InstallHint erklärt, woher cloudflared kommt.
func InstallHint(goos string) string {
	switch goos {
	case "windows":
		return "winget install --id Cloudflare.cloudflared"
	case "darwin":
		return "brew install cloudflared"
	default:
		return "Von github.com/cloudflare/cloudflared/releases herunterladen und " +
			"nach /usr/local/bin legen"
	}
}
