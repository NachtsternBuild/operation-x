// Package schema definiert das vollständige Datenmodell von Operation X und legt es
// beim Start idempotent an.
//
// Grundsatz des Projekts (siehe docs/struktur.html, Abschnitt 04): Kein Client greift jemals
// direkt auf diese Collections zu. Sämtliche API-Regeln bleiben gesperrt (nil bedeutet
// bei PocketBase "nur Superuser"). Alles, was ein Spieler zu sehen bekommt, läuft über
// die rollengefilterten Endpunkte in internal/api — nur dort lässt sich zuverlässig
// zuschneiden, wer welche Position, welchen Hinweis und welche Wahrheit erfährt.
package schema

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// Rollen. Jedes Team hat genau eine.
const (
	RoleHQ        = "hq"
	RoleMisterX   = "misterx"
	RoleDetective = "detective"
)

// Spielzustände.
const (
	GameSetup    = "setup"
	GameReady    = "ready"
	GameRunning  = "running"
	GamePaused   = "paused"
	GameFinished = "finished"
)

// Namen aller Collections – zentral, damit Tippfehler beim Bauen auffallen
// und nicht erst zur Laufzeit.
const (
	ColGames     = "games"
	ColTeams     = "teams"
	ColSectors   = "sectors"
	ColHotspots  = "hotspots"
	ColPositions = "positions"
	ColPings     = "pings"
	ColMissions  = "missions"
	ColOptions   = "mission_options"
	ColEvidence  = "evidence"
	ColPuzzles   = "puzzles"
	ColAttempts  = "attempts"
	ColIntel     = "intel"
	ColLedger    = "ledger"
	ColJokers    = "jokers"
	ColZones     = "zones"
	ColSightings = "sightings"
	ColArrests   = "arrests"
	ColRadio     = "radio"
	ColEvents    = "events"
	ColPenalties = "penalties"
)

// builder löst Relationen auf Collections auf, die in derselben Runde erst
// angelegt werden. Deshalb ist die Reihenfolge in definitions() bedeutsam:
// eine Collection darf nur auf bereits definierte verweisen.
type builder struct {
	ids map[string]string
}

// rel legt eine Verknüpfung an.
//
// Ob ein Datensatz mitgelöscht wird, hängt allein daran, ob der Verweis Pflicht
// ist: Ein Hotspot gehört zu einem Spiel und verschwindet mit ihm, aber er
// überlebt eine Änderung des Sektorschnitts und verliert dabei nur seine
// Zuordnung. Andersherum – Mitlöschen auch bei optionalen Verweisen – räumte das
// Neuaufteilen der Sektoren stillschweigend sämtliche Hotspots mit weg.
func (b *builder) rel(target, name string, required bool) core.Field {
	id, ok := b.ids[target]
	if !ok {
		// Programmierfehler in definitions() – lieber laut scheitern als still
		// eine kaputte Relation anlegen.
		panic(fmt.Sprintf("schema: Relation %q verweist auf die noch nicht definierte Collection %q", name, target))
	}
	return &core.RelationField{
		Name:          name,
		CollectionId:  id,
		MaxSelect:     1,
		Required:      required,
		CascadeDelete: required,
	}
}

// --- Feld-Kurzschreibweisen ---------------------------------------------------

func text(name string, required bool) core.Field {
	return &core.TextField{Name: name, Required: required}
}

func longtext(name string) core.Field {
	return &core.TextField{Name: name, Max: 20000}
}

func num(name string) core.Field {
	return &core.NumberField{Name: name}
}

func intnum(name string) core.Field {
	return &core.NumberField{Name: name, OnlyInt: true}
}

func boolean(name string) core.Field {
	return &core.BoolField{Name: name}
}

func date(name string) core.Field {
	return &core.DateField{Name: name}
}

func jsonf(name string) core.Field {
	return &core.JSONField{Name: name, MaxSize: 2_000_000}
}

func sel(name string, values ...string) core.Field {
	return &core.SelectField{Name: name, Values: values, MaxSelect: 1}
}

// file legt ein Dateifeld an – bewacht.
//
// Ohne Protected liefert PocketBase die Datei jedem aus, der ihre Adresse
// kennt, ohne jede Anmeldung. Für Beweisfotos aus dem öffentlichen Raum ist
// das zu wenig. Mit Protected greift die Zugriffsregel der Collection, und die
// ist hier für alle gesperrt: Der allgemeine Dateiweg ist damit dicht, und die
// Zentrale bekommt das Bild über ihren eigenen Endpunkt.
func file(name string) core.Field {
	return &core.FileField{
		Name: name, MaxSelect: 1, MaxSize: 12_000_000, Protected: true,
	}
}

// timestamps liefert die beiden automatischen Zeitstempel, die jede Collection bekommt.
func timestamps() []core.Field {
	return []core.Field{
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	}
}

// --- Definition ---------------------------------------------------------------

type def struct {
	name    string
	auth    bool
	indexes []string
	fields  func(b *builder) []core.Field
	// identity ist das Feld, mit dem man sich anmeldet. Leer heißt: Rufzeichen,
	// wie bei den Teams. "email" gibt es für Zugänge, die Personen gehören und
	// Wochen später wiederkommen – hier keine, aber ein aufbauendes Programm
	// legt solche an.
	identity string
}

// definitions beschreibt das gesamte Datenmodell in Abhängigkeitsreihenfolge.
func definitions() []def {
	return []def{
		// Ein Spieldurchlauf. Alles Weitere hängt daran und wird mit ihm gelöscht.
		{
			name: ColGames,
			fields: func(b *builder) []core.Field {
				return []core.Field{
					text("name", true),
					text("city", false),
					sel("status", GameSetup, GameReady, GameRunning, GamePaused, GameFinished),
					date("starts_at"),
					date("ends_at"),
					date("paused_at"),
					// Wofür und bis wann. Eine Pause ohne Ansage ist für alle,
					// die draußen stehen, nicht von einer Störung zu
					// unterscheiden.
					text("pause_reason", false),
					date("pause_until"), // Beginn der laufenden Pause
					intnum("duration_min"),
					jsonf("area"),   // GeoJSON-Polygon des Spielgebiets
					jsonf("config"), // Regelwerte: Intervalle, Punkte, FP-Preise
					boolean("final_active"),
					intnum("retention_hours"), // Aufbewahrung der Standortdaten
					date("purge_at"),          // wann die Bewegungsspur fällt
					date("purged_at"),         // wann sie gefallen ist

					// Das geheime Fluchtziel als Hotspot-Kennung. Bewusst ein
					// Textfeld statt einer Verknüpfung: Hotspots werden erst
					// nach den Spielen definiert, und eine Verknüpfung zurück
					// ergäbe einen Ring in der Anlagereihenfolge.
					text("final_target", false),
					intnum("planned_missions"),

					// Finale und Ausgang.
					date("final_started_at"),
					num("final_radius_m"), // schrumpft im Finale schrittweise
					sel("winner", "misterx", "detectives", "draw"),
					text("win_reason", false),
					date("ended_at"),
				}
			},
		},

		// Sektoren A, B, C … – entweder aus echten Stadtteilgrenzen oder selbst gezeichnet.
		{
			name: ColSectors,
			fields: func(b *builder) []core.Field {
				return []core.Field{
					b.rel(ColGames, "game", true),
					text("code", true), // A, B, C …
					text("name", false),
					jsonf("geometry"), // GeoJSON-Polygon
					text("color", false),
				}
			},
		},

		// Nummerierte Hotspots #01, #02 … mit Vor-Ort-Code.
		{
			name:    ColHotspots,
			indexes: []string{"CREATE INDEX idx_hotspots_game ON hotspots (game)"},
			fields: func(b *builder) []core.Field {
				return []core.Field{
					b.rel(ColGames, "game", true),
					intnum("number"),
					text("name", false),
					num("lat"),
					num("lng"),
					b.rel(ColSectors, "sector", false),
					text("passcode", false), // vor Ort sichtbar, beweist Anwesenheit
					sel("kind", "landmark", "transit", "park", "building", "other"),
					longtext("notes"),
				}
			},
		},

		// Die Teams stehen hinter den Hotspots, weil sie auf einen verweisen:
		// den ausgelosten Startpunkt. Die Reihenfolge in dieser Liste ist
		// bedeutsam – eine Collection darf nur auf bereits definierte verweisen.
		// Ein Login je Team. Angemeldet wird sich mit dem Rufzeichen, nicht mit E-Mail –
		// siehe ensureAuthOptions().
		{
			name:    ColTeams,
			auth:    true,
			indexes: []string{"CREATE UNIQUE INDEX idx_teams_callsign ON teams (callsign)"},
			fields: func(b *builder) []core.Field {
				return []core.Field{
					text("callsign", true), // z. B. "Team_Alpha"
					text("display", false), // Anzeigename im Funkkanal
					sel("role", RoleHQ, RoleMisterX, RoleDetective),
					// Zugänge gehören zu einem Spiel und verschwinden mit ihm –
					// sonst sammeln sich über die Spiele hinweg Logins an, die
					// zu nichts mehr gehören.
					b.rel(ColGames, "game", true),
					text("color", false),
					intnum("fp"), // Fluchtpunkte bzw. Fahndungspunkte
					intnum("points"),
					boolean("active"),
					boolean("in_transit"),
					date("last_seen_at"),
					date("onboarded_at"), // Einweisung abgeschlossen

					// Fristverwaltung der Pflichtmeldungen. ping_due_at wird bei
					// jeder Meldung neu gesetzt und bei einem Verstoß um genau ein
					// Intervall weitergeschoben – so bucht auch ein Team, das eine
					// Stunde offline war, für jedes verpasste Intervall genau einen
					// Verstoß, statt bei jedem Durchlauf der Engine einen neuen.
					date("ping_due_at"),
					intnum("ping_violations"),
					date("last_action_at"), // letzte Spielhandlung, für Anti-Camping
					// Der ausgeloste Startpunkt. Ohne ihn verabreden sich alle
					// an derselben Haltestelle, und wenn Zielperson und
					// Fahndung am selben Ort anfangen, ist das Spiel in der
					// ersten Minute entschieden.
					b.rel(ColHotspots, "start_hotspot", false),

					// Von der Trockenübung gesteuert, nicht von einem Menschen.
					boolean("bot"),
				}
			},
		},

		// Standortverlauf. Häufigste Schreiboperation im Spiel.
		{
			name: ColPositions,
			indexes: []string{
				"CREATE INDEX idx_positions_team_time ON positions (team, captured_at)",
				"CREATE INDEX idx_positions_game_time ON positions (game, captured_at)",
			},
			fields: func(b *builder) []core.Field {
				return []core.Field{
					b.rel(ColGames, "game", true),
					b.rel(ColTeams, "team", true),
					num("lat"),
					num("lng"),
					num("accuracy"), // Meter, fließt in die 150-m-Prüfung ein
					num("speed"),
					num("heading"),
					// captured_at ist die Spielwahrheit, nicht der Eingang beim Server:
					// gepufferte Meldungen aus dem Funkloch behalten ihre Originalzeit.
					date("captured_at"),
					sel("source", "gps", "manual", "sim"),
					boolean("mocked"), // von einer Fake-GPS-App erzeugt
					boolean("in_transit"),
				}
			},
		},

		// Pflichtmeldungen mit Frist (10 Min., im Transit 13).
		{
			name: ColPings,
			fields: func(b *builder) []core.Field {
				return []core.Field{
					b.rel(ColGames, "game", true),
					b.rel(ColTeams, "team", true),
					b.rel(ColPositions, "position", false),
					date("due_at"),
					date("captured_at"),
					intnum("late_by_sec"),
					intnum("violation_no"), // der wievielte Verstoß in Folge
					sel("status", "pending", "ok", "late", "missed"),
				}
			},
		},

		// Zwischenziele von Mister X.
		{
			name: ColMissions,
			fields: func(b *builder) []core.Field {
				return []core.Field{
					b.rel(ColGames, "game", true),
					intnum("seq"),
					b.rel(ColHotspots, "target", false),
					sel("status", "proposed", "active", "done", "failed", "aborted"),
					date("deadline_at"),
					date("started_at"),
					date("completed_at"),
					boolean("is_final"), // letztes Zwischenziel vor dem Hauptziel

					// Kulanzzeit: wie viele Minuten die Frist schon verlängert
					// wurde, wann zuletzt und warum. Siehe internal/game/grace.go.
					intnum("grace_used_min"),
					date("grace_last_at"),
					text("grace_reason", false),
				}
			},
		},

		// Je Zwischenziel drei Varianten zur Wahl.
		{
			name: ColOptions,
			fields: func(b *builder) []core.Field {
				return []core.Field{
					b.rel(ColMissions, "mission", true),
					b.rel(ColHotspots, "hotspot", false),
					sel("kind", "safe", "fast", "bonus"),
					text("label", false),
					longtext("description"),
					intnum("time_limit_min"),
					intnum("reward_points"),
					intnum("reward_fp"),
					boolean("chosen"),
				}
			},
		},

		// Beweispflicht: Live-Foto oder Vor-Ort-Code.
		{
			name: ColEvidence,
			fields: func(b *builder) []core.Field {
				return []core.Field{
					b.rel(ColGames, "game", true),
					b.rel(ColTeams, "team", true),
					b.rel(ColMissions, "mission", false),
					b.rel(ColHotspots, "hotspot", false),
					file("photo"),
					text("passcode_entered", false),
					num("lat"),
					num("lng"),
					num("distance_m"), // Abstand zum Hotspot bei Einreichung
					date("captured_at"),
					sel("status", "pending", "accepted", "rejected"),
					longtext("note"),
				}
			},
		},

		// Rätsel der Typen A–F. Ein gelöstes Rätsel schaltet einen Hinweis frei.
		{
			name: ColPuzzles,
			fields: func(b *builder) []core.Field {
				return []core.Field{
					b.rel(ColGames, "game", true),
					text("code", false),
					sel("type", "A", "B", "C", "D", "E", "F"), // Logik, Foto, Code, Geometrie, Umgebung, Kombination
					text("title", false),
					longtext("question"),
					text("answer", false), // Vergleich normalisiert, nicht zeichengenau
					longtext("hint"),
					file("image"),
					date("unlock_at"),
					boolean("unlocked"),
					sel("intel_category", "green", "yellow", "red"),
					longtext("intel_template"), // Vorlage für den freigeschalteten Hinweis
					intnum("points"),
					intnum("order"),
				}
			},
		},

		// Lösungsversuche der Detektivteams.
		{
			name: ColAttempts,
			fields: func(b *builder) []core.Field {
				return []core.Field{
					b.rel(ColGames, "game", true),
					b.rel(ColPuzzles, "puzzle", true),
					b.rel(ColTeams, "team", true),
					text("answer", false),
					boolean("correct"),
					boolean("used_fp"), // Hinweis mit Fluchtpunkt freigekauft
					intnum("awarded_points"),
				}
			},
		},

		// Freigeschaltete Hinweise. Gefälschte sehen für Detektive identisch aus –
		// unterscheidbar nur über fabricated, das ausschließlich das HQ zu sehen bekommt.
		{
			name: ColIntel,
			fields: func(b *builder) []core.Field {
				return []core.Field{
					b.rel(ColGames, "game", true),
					sel("category", "green", "yellow", "red"),
					longtext("text"),
					b.rel(ColSectors, "sector", false),
					b.rel(ColHotspots, "hotspot", false),
					boolean("fabricated"),
					sel("source", "system", "puzzle", "joker", "hq"),
					date("occurred_at"), // Zeitpunkt der Beobachtung – steuert HOT/WARM/KALT
					date("deliver_at"),  // durch Störungs-Joker nach hinten geschoben
					boolean("delivered"),
					jsonf("visible_to"), // Team-Kennungen, oder leer für alle Detektive
				}
			},
		},

		// Kontobuch. Punkte und Fluchtpunkte werden hieraus abgeleitet, nie überschrieben.
		{
			name:    ColLedger,
			indexes: []string{"CREATE INDEX idx_ledger_team ON ledger (team)"},
			fields: func(b *builder) []core.Field {
				return []core.Field{
					b.rel(ColGames, "game", true),
					b.rel(ColTeams, "team", true),
					intnum("delta_fp"),
					intnum("delta_points"),
					text("reason", false),   // Regelbezug, im Punktekonto aufklappbar
					text("ref_type", false), // z. B. "mission", "arrest", "ping"
					text("ref_id", false),
					date("occurred_at"),
				}
			},
		},

		// Laufende Joker-Wirkungen mit Ablaufzeit.
		{
			name: ColJokers,
			fields: func(b *builder) []core.Field {
				return []core.Field{
					b.rel(ColGames, "game", true),
					b.rel(ColTeams, "team", true),
					sel("kind",
						"false_trail", // Falsche Fährte
						"delay",       // Zeitverzögerung / Störung
						"phantom",     // Phantom
						"smoke",       // Nebelkerze
						"ghost",       // U-Bahn-Geist
						"gps_ping",    // Exakter GPS-Ping
						"lockdown",    // Sperrzone
						"bug",         // Wanzen-Falle
						"scan",        // Bereichs-Scan
						"unlock",      // Hinweis-Freischaltung
						"reroute",     // Ausklinken / Routenänderung
					),
					intnum("cost_fp"),
					date("started_at"),
					date("expires_at"),
					boolean("active"),
					jsonf("payload"), // Ziel-Sektor, erzeugter Hinweis, Scan-Ergebnis …
				}
			},
		},

		// Sperrzonen und Wanzen der Detektive.
		{
			name: ColZones,
			fields: func(b *builder) []core.Field {
				return []core.Field{
					b.rel(ColGames, "game", true),
					b.rel(ColTeams, "team", true),
					sel("kind", "lockdown", "bug"),
					b.rel(ColSectors, "sector", false),
					b.rel(ColHotspots, "hotspot", false),
					num("lat"),
					num("lng"),
					num("radius_m"),
					date("expires_at"),
					boolean("triggered"),
					date("triggered_at"),
				}
			},
		},

		// Sichtkontakte – gültig erst nach 20 Sekunden Verweildauer oder Code-Eingabe.
		{
			name: ColSightings,
			fields: func(b *builder) []core.Field {
				return []core.Field{
					b.rel(ColGames, "game", true),
					b.rel(ColTeams, "team", true),
					num("lat"),
					num("lng"),
					num("distance_m"),
					date("reported_at"),
					date("confirmed_at"),
					boolean("confirmed"),
					sel("method", "dwell", "code"),
				}
			},
		},

		// Zugriffsformular, Stufe 1 bis 3.
		{
			name: ColArrests,
			fields: func(b *builder) []core.Field {
				return []core.Field{
					b.rel(ColGames, "game", true),
					b.rel(ColTeams, "team", true),
					intnum("level"), // 1 = Lokalisierung, 2 = Rekonstruktion, 3 = Vollzugriff
					b.rel(ColHotspots, "hotspot", false),
					intnum("claimed_hotspot"),
					date("claimed_time"),
					text("claimed_target", false), // vermutetes Fluchtziel
					boolean("correct"),
					jsonf("detail"), // welche Teilangabe stimmte, welche nicht
					date("resolved_at"),
				}
			},
		},

		// HQ-Funkkanal mit Vertrauensstufen.
		{
			name: ColRadio,
			fields: func(b *builder) []core.Field {
				return []core.Field{
					b.rel(ColGames, "game", true),
					b.rel(ColTeams, "team", false), // leer = Systemmeldung
					text("author", false),
					longtext("text"),
					sel("trust", "confirmed", "unconfirmed", "rumor"),
					boolean("pinned"),
					sel("audience", "detectives", "misterx", "all"),
				}
			},
		},

		// Lückenlose Ereigniskette. Grundlage für Punktestand, Nachbetrachtung
		// und die Antwort auf "warum habe ich minus 15?".
		{
			name:    ColEvents,
			indexes: []string{"CREATE INDEX idx_events_game_time ON events (game, occurred_at)"},
			fields: func(b *builder) []core.Field {
				return []core.Field{
					b.rel(ColGames, "game", true),
					b.rel(ColTeams, "team", false),
					text("type", true),
					// Der Klartext zur Buchung: "Standortmeldung zum 3. Mal
					// überfällig", "Schlange an der Kasse". Er wurde bisher nur
					// im Kontobuch abgelegt, und dorthin kommt er nur, wenn
					// sich auch Punkte ändern. Eine gemeldete Verzögerung, eine
					// Verwarnung, ein Sichtkontakt – alles ohne Punktabzug –
					// verloren ihre Begründung damit vollständig. Die
					// Zeitleiste der Zentrale zeigte den nackten Ereignistyp,
					// und die Warnung auf dem Telefon lautete "Ihr seid
					// gesperrt. Der Grund: " und dann nichts mehr.
					text("reason", false),
					jsonf("payload"),
					intnum("delta_points"),
					intnum("delta_fp"),
					date("occurred_at"),
					// Verhindert, dass ein wiederholter Engine-Durchlauf dieselbe
					// Konsequenz zweimal bucht.
					text("dedupe_key", false),
				}
			},
		},

		// Laufende Sperren und Strafzeiten.
		{
			name: ColPenalties,
			fields: func(b *builder) []core.Field {
				return []core.Field{
					b.rel(ColGames, "game", true),
					b.rel(ColTeams, "team", true),
					sel("kind", "lockout_location", "lockout_action", "warning"),
					text("reason", false),
					date("started_at"),
					date("expires_at"),
					num("lat"), // Ort, an dem die Sperre gilt
					num("lng"),
					boolean("active"),
				}
			},
		},
	}
}

// Ensure legt alle Collections an bzw. gleicht bestehende an die Definition an.
// Die Funktion ist idempotent und läuft bei jedem Start.
// Nacharbeit läuft am Ende von Ensure.
//
// Dort legt ein aufbauendes Programm eigene Sammlungen an und hängt Felder an
// bestehende. Bewusst ein einzelner Haken und keine ausgebaute Schnittstelle:
// Wer hier eingreift, kennt PocketBase – sonst käme er nicht auf die Idee.
var Nacharbeit func(app core.App) error

func Ensure(app core.App) error {
	if err := EnsureSettings(app); err != nil {
		return err
	}
	if err := EntferneBeispielsammlung(app); err != nil {
		return err
	}

	b := &builder{ids: map[string]string{}}

	for _, d := range definitions() {
		col, err := app.FindCollectionByNameOrId(d.name)
		if err != nil {
			// Noch nicht vorhanden – neu anlegen.
			if d.auth {
				col = core.NewAuthCollection(d.name)
			} else {
				col = core.NewBaseCollection(d.name)
			}
		}

		// Erst die ID registrieren, damit Relationen auf diese Collection
		// innerhalb derselben Runde auflösbar sind. Bei einer neuen Collection
		// vergibt PocketBase die ID beim ersten Save, deshalb hier zweistufig.
		if col.Id == "" {
			if err := app.Save(col); err != nil {
				return fmt.Errorf("Collection %q anlegen: %w", d.name, err)
			}
		}
		b.ids[d.name] = col.Id

		col.Fields.Add(d.fields(b)...)
		col.Fields.Add(timestamps()...)

		// Alle Zugriffsregeln gesperrt: Spieler-Clients kommen ausschließlich
		// über die rollengefilterten Endpunkte an Daten.
		col.ListRule = nil
		col.ViewRule = nil
		col.CreateRule = nil
		col.UpdateRule = nil
		col.DeleteRule = nil

		if d.auth {
			col.PasswordAuth.Enabled = true
			col.AuthRule = ptr("") // wer einen Zugang hat, darf sich anmelden
			col.OAuth2.Enabled = false
			col.MFA.Enabled = false
			col.OTP.Enabled = false

			if d.identity == "email" {
				col.PasswordAuth.IdentityFields = []string{core.FieldNameEmail}
			} else {
				// Anmeldung mit Rufzeichen statt E-Mail.
				col.PasswordAuth.IdentityFields = []string{"callsign"}

				// Teams sind keine Personen mit Postfach – ein Team heißt "Team_Alpha"
				// und hat keine E-Mail-Adresse. PocketBase legt das Feld als Pflichtfeld
				// an; wir geben es frei, statt Adressen zu erfinden. Der zugehörige
				// Unique-Index greift ohnehin nur für nicht-leere Werte.
				col.Fields.Add(&core.EmailField{
					Name:     core.FieldNameEmail,
					System:   true,
					Required: false,
				})
			}
		}

		for _, idx := range d.indexes {
			if !hasIndex(col.Indexes, idx) {
				col.Indexes = append(col.Indexes, idx)
			}
		}

		if err := app.Save(col); err != nil {
			return fmt.Errorf("Collection %q speichern: %w", d.name, err)
		}
	}

	if Nacharbeit != nil {
		if err := Nacharbeit(app); err != nil {
			return err
		}
	}

	return nil
}

func hasIndex(existing []string, idx string) bool {
	for _, e := range existing {
		if e == idx {
			return true
		}
	}
	return false
}

func ptr(s string) *string {
	return &s
}
