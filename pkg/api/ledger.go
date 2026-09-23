package api

import (
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/elias/operation-x/pkg/schema"
)

// Das Punktekonto.
//
// "Warum habe ich minus fünfzehn?" ist die häufigste Frage am Spieltag. Bis
// hierher gab es darauf keine Antwort: Die Kopfleiste zeigte eine Zahl, das
// Kontobuch lag in der Datenbank, und gelesen hat es niemand – es gab keinen
// Weg dorthin. Wer wissen wollte, woher der Stand kommt, musste die Zentrale
// über Funk fragen, und die musste in ihrer Zeitleiste suchen.
//
// Bewusst aus dem Kontobuch und nicht aus dem Ereignisprotokoll: Aus dem
// Kontobuch wird der Stand abgeleitet, es ist die Wahrheit. Ein Protokolleintrag
// ohne Punktänderung – eine Verwarnung, ein gemeldeter Sichtkontakt – gehört in
// die Zeitleiste, nicht in ein Konto; er würde hier nur die Frage aufwerfen,
// warum er nichts verändert hat.
//
// Der Stand wird mitgerechnet und nicht gespeichert. Das ist mehr als Bequemlichkeit:
// Fluchtpunkte fallen nie unter null, der Abzug im Kontobuch steht aber in
// voller Höhe. Nur wer die Buchungen in ihrer Reihenfolge nachrechnet, kommt
// auf denselben Stand, den das Team oben in der Ecke sieht.

type ledgerEntry struct {
	ID          string `json:"id"`
	Reason      string `json:"reason"`
	DeltaPoints int    `json:"deltaPoints"`
	DeltaFP     int    `json:"deltaFp"`

	// Der Stand unmittelbar nach dieser Buchung.
	Points int `json:"points"`
	FP     int `json:"fp"`

	OccurredAt string `json:"occurredAt"`
	RefType    string `json:"refType,omitempty"`
}

type ledgerResponse struct {
	Team     string        `json:"team"`
	Callsign string        `json:"callsign"`
	Points   int           `json:"points"`
	FP       int           `json:"fp"`
	Entries  []ledgerEntry `json:"entries"`
}

// handleLedger liefert das Konto eines Teams.
//
// Ein Team sieht ausschließlich sein eigenes: Der Kontostand der Gegenseite
// verriete, was sie getan hat – ein Missionsabschluss, ein gekaufter Hinweis,
// ein Fehlzugriff. Die Zentrale darf jedes einsehen und braucht das auch, denn
// bei ihr landet die Frage, wenn jemand mit seinem Stand nicht einverstanden ist.
func handleLedger(e *core.RequestEvent) error {
	me := e.Auth
	if me == nil {
		return e.UnauthorizedError("Nicht angemeldet.", nil)
	}

	team := me
	if me.GetString("role") == schema.RoleHQ {
		wanted := e.Request.URL.Query().Get("team")
		if wanted == "" {
			return e.BadRequestError("Welches Team? Die Zentrale führt kein eigenes Konto.", nil)
		}

		other, err := e.App.FindRecordById(schema.ColTeams, wanted)
		if err != nil || other == nil {
			return e.NotFoundError("Dieses Team gibt es nicht.", nil)
		}
		// Kein Blick in ein anderes Spiel.
		if other.GetString("game") != me.GetString("game") {
			return e.NotFoundError("Dieses Team gibt es nicht.", nil)
		}
		team = other
	}

	records, err := e.App.FindRecordsByFilter(
		schema.ColLedger, "team = {:t}", "occurred_at", 0, 0,
		map[string]any{"t": team.Id},
	)
	if err != nil {
		return err
	}

	rows := make([]ledgerRow, 0, len(records))
	for _, r := range records {
		rows = append(rows, ledgerRow{
			ID:          r.Id,
			Reason:      r.GetString("reason"),
			RefType:     r.GetString("ref_type"),
			OccurredAt:  r.GetDateTime("occurred_at").Time().UTC().Format(time.RFC3339),
			DeltaPoints: r.GetInt("delta_points"),
			DeltaFP:     r.GetInt("delta_fp"),
		})
	}

	return e.JSON(http.StatusOK, ledgerResponse{
		Team:     team.Id,
		Callsign: team.GetString("callsign"),
		Points:   team.GetInt("points"),
		FP:       team.GetInt("fp"),
		Entries:  ledgerView(rows),
	})
}

// ledgerRow ist eine Buchung, so wie sie im Kontobuch steht.
type ledgerRow struct {
	ID          string
	Reason      string
	RefType     string
	OccurredAt  string
	DeltaPoints int
	DeltaFP     int
}

// ledgerView rechnet den Stand nach jeder Buchung mit.
//
// Erwartet die Buchungen in ihrer zeitlichen Reihenfolge und liefert sie
// umgekehrt zurück – neueste zuerst, denn wer nachsieht, sucht fast immer die
// letzte.
func ledgerView(rows []ledgerRow) []ledgerEntry {
	entries := make([]ledgerEntry, 0, len(rows))
	points, fp := 0, 0

	for _, r := range rows {
		points += r.DeltaPoints
		// Dieselbe Grenze wie beim Buchen: Ein Guthaben kennt kein Minus.
		// Ohne sie liefe das Konto unter dem Stand her, den das Team oben in
		// der Ecke sieht – und beide hätten recht, was niemandem hilft.
		fp = max(0, fp+r.DeltaFP)

		entries = append(entries, ledgerEntry{
			ID:          r.ID,
			Reason:      r.Reason,
			DeltaPoints: r.DeltaPoints,
			DeltaFP:     r.DeltaFP,
			Points:      points,
			FP:          fp,
			OccurredAt:  r.OccurredAt,
			RefType:     r.RefType,
		})
	}

	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries
}
