package api

import (
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"time"

	"github.com/pocketbase/pocketbase/core"

)

// Sicherung auf Knopfdruck.
//
// Ein Spieltag steckt in einer einzigen Datei: Sektoren, Hotspots, Rätsel,
// Zugangsdaten, jede Buchung. Geht sie kaputt – ein voller Datenträger, ein
// Stromausfall im falschen Moment, ein versehentliches Löschen des Ordners –,
// ist die Vorbereitung von mehreren Abenden weg, und zwar an dem Tag, an dem
// zwanzig Leute vor der Tür stehen.
//
// Deshalb ein Knopf, der eine Kopie herunterlädt. Nicht in den Spielordner,
// sondern in den Ordner "Downloads": Eine Sicherung, die neben dem Original
// liegt, ist keine.
//
// Die Aufnahme selbst macht PocketBase; sie ist im Betrieb sicher, weil die
// Datenbank dabei nur für den Augenblick des Kopierens gesperrt wird.

// backupName baut den Dateinamen. Er enthält den Zeitpunkt, damit sich mehrere
// Sicherungen nicht gegenseitig überschreiben und man in der Dateiliste sieht,
// welche die jüngere ist.
func backupName(gameName string, now time.Time) string {
	safe := regexp.MustCompile(`[^\p{L}\p{N}]+`).ReplaceAllString(gameName, "-")
	safe = regexp.MustCompile(`^-+|-+$`).ReplaceAllString(safe, "")
	if safe == "" {
		safe = "operation-x"
	}
	return fmt.Sprintf("%s-%s.zip", safe, now.Format("2006-01-02-1504"))
}

// handleBackup legt eine Sicherung an und liefert sie sofort aus.
//
// Beides in einem Aufruf, weil beides zusammengehört: Eine Sicherung, die auf
// dem Server liegen bleibt, hilft genau in dem Fall nicht, für den man sie
// gemacht hat.
func handleBackup(e *core.RequestEvent) error {
	if err := darfSichern(e); err != nil {
		return err
	}

	name := backupName("operation-x", time.Now())
	if gameRec, err := SpielDesServers(e.App); err == nil && gameRec != nil {
		name = backupName(gameRec.GetString("name"), time.Now())
	}

	if err := e.App.CreateBackup(e.Request.Context(), name); err != nil {
		return e.InternalServerError(
			"Die Sicherung ließ sich nicht anlegen. Läuft gerade eine andere?", err)
	}

	fsys, err := e.App.NewBackupsFilesystem()
	if err != nil {
		return e.InternalServerError("Der Sicherungsordner ist nicht lesbar.", err)
	}
	defer fsys.Close()

	e.Response.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=%q", filepath.Base(name)))

	return fsys.Serve(e.Response, e.Request, name, name)
}

type backupEntry struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"sizeBytes"`
	CreatedAt string `json:"createdAt"`
}

// handleBackupList zeigt, was schon gesichert wurde.
//
// Die Frage dahinter ist nicht "welche Dateien gibt es", sondern "habe ich
// heute schon gesichert" – deshalb steht die Zeit dabei.
func handleBackupList(e *core.RequestEvent) error {
	if err := darfSichern(e); err != nil {
		return err
	}

	fsys, err := e.App.NewBackupsFilesystem()
	if err != nil {
		return e.InternalServerError("Der Sicherungsordner ist nicht lesbar.", err)
	}
	defer fsys.Close()

	objects, err := fsys.List("")
	if err != nil {
		return e.InternalServerError("Der Sicherungsordner ist nicht lesbar.", err)
	}

	out := make([]backupEntry, 0, len(objects))
	for _, o := range objects {
		out = append(out, backupEntry{
			Name:      o.Key,
			SizeBytes: o.Size,
			CreatedAt: o.ModTime.UTC().Format(time.RFC3339),
		})
	}

	// Jüngste zuerst.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}

	return e.JSON(http.StatusOK, map[string]any{"backups": out})
}

// darfSichern entscheidet, wer eine Sicherung ziehen darf.
//
// Eine Sicherung ist die ganze Datenbank – auf einem Server mit mehreren
// Spielen. Auf diesem Server ist die Zentrale zugleich der Betreiber, also
// darf sie das. Führt ein aufbauendes Programm mehrere Spiele, sieht das
// anders aus — dann entscheidet dessen Haken (siehe aufsatz.go).
func darfSichern(e *core.RequestEvent) error {
	return serverSache(e)
}
