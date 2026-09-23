package game

// Das Regelverzeichnis.
//
// Jeder Regelwert bekommt hier einen Klartextnamen und einen Satz, der sagt,
// was er bewirkt. Das dient zwei Zwecken auf einmal:
//
//   - Die Zentrale kann die Werte in der Oberfläche ändern, statt eine
//     Eingabeaufforderung zu öffnen. Ein Zahlenfeld ohne Erklärung wäre dabei
//     wertlos – "ping_lockout_points: -15" sagt niemandem etwas.
//   - Die Spieler können während des Spiels nachschlagen, was ein Joker kostet
//     und was ein verpasster Ping bedeutet. Das ist die häufigste Frage am
//     Spieltag, und sie soll niemanden dazu zwingen, den Funk zu benutzen.
//
// Beide lesen dieselbe Quelle. Ein Wert, der hier fehlt, taucht in keiner der
// beiden Ansichten auf – deshalb prüft ein Test, dass das Verzeichnis alle
// Werte der Voreinstellung kennt.

// RuleUnit sagt, wie ein Wert zu lesen ist. Die Oberfläche hängt danach die
// Einheit an und wählt die passende Eingabe.
type RuleUnit string

const (
	UnitMinutes RuleUnit = "min"
	// Sekunden gibt es genau einmal – die Verweildauer beim Sichtkontakt. Sie
	// stand vorher als blanke Zahl da, und "Verweildauer 20" liest sich wie 20
	// Minuten, was das Dreifache eines ganzen Meldeabstands wäre.
	UnitSeconds RuleUnit = "sek"
	UnitMeters  RuleUnit = "m"
	UnitPoints  RuleUnit = "punkte"
	UnitFP      RuleUnit = "fp"
	UnitCount   RuleUnit = "anzahl"
)

// Rule beschreibt einen einzelnen Regelwert.
type Rule struct {
	Key   string   `json:"key"`
	Group string   `json:"group"`
	Name  string   `json:"name"`
	What  string   `json:"what"`
	Unit  RuleUnit `json:"unit"`

	// Wen der Wert betrifft. Steuert, wem er im Nachschlagewerk gezeigt wird –
	// die Preise der Fahndung gehen Mister X nichts an und umgekehrt.
	Side string `json:"side"` // "" = alle, sonst misterx / detective

	Min int `json:"min"`
	Max int `json:"max"`
}

// Gruppen des Verzeichnisses, in der Reihenfolge, in der sie erscheinen.
const (
	GroupPing    = "Meldepflicht"
	GroupCamping = "Stillstand"
	GroupMission = "Missionen"
	GroupPuzzle  = "Rätsel und Hinweise"
	GroupJoker   = "Einsatzmittel"
	GroupArrest  = "Zugriff und Sieg"
	GroupField   = "Abstände"
)

// RuleIndex ist das Verzeichnis. Reihenfolge zählt: Sie bestimmt, wie die
// Oberfläche die Werte anordnet.
var RuleIndex = []Rule{
	// --- Meldepflicht ---
	{"ping_interval_min", GroupPing, "Meldeabstand",
		"So oft muss jedes Team seinen Standort melden.", UnitMinutes, "", 1, 120},
	{"ping_interval_transit_min", GroupPing, "Meldeabstand im Transit",
		"Längere Frist, solange ein Team in Bahn oder Bus sitzt.", UnitMinutes, "", 1, 120},
	{"ping_warn_at_violation", GroupPing, "Verwarnungen vorab",
		"So viele Verstöße bleiben folgenlos, bevor Punkte abgezogen werden.", UnitCount, "", 0, 5},
	{"ping_penalty_points", GroupPing, "Abzug bei Verzug",
		"Punkte, die ein verpasster Meldezeitpunkt kostet.", UnitPoints, "", -100, 0},
	{"ping_lockout_min", GroupPing, "Sperre bei Wiederholung",
		"Ab dem dritten Verstoß gesperrt – so lange.", UnitMinutes, "", 0, 60},
	{"ping_lockout_points", GroupPing, "Zusatzabzug bei Sperre",
		"Punkte zusätzlich zur Sperre.", UnitPoints, "", -100, 0},

	// --- Stillstand ---
	{"camping_detective_min", GroupCamping, "Geduldsfaden Fahndung",
		"So lange darf ein Fahndungsteam weder handeln noch sich bewegen.", UnitMinutes, "detective", 1, 120},
	{"camping_detective_fp", GroupCamping, "Preis fürs Stillsitzen",
		"Fahndungspunkte je angebrochenem Zeitraum ohne Regung.", UnitFP, "detective", -10, 0},
	{"camping_misterx_min", GroupCamping, "Geduldsfaden Zielperson",
		"So lange darf die Zielperson stillstehen – gezählt wird nur außerhalb " +
			"von Mission und Transit.", UnitMinutes, "misterx", 1, 240},
	{"camping_misterx_fp", GroupCamping, "Preis fürs Verschanzen",
		"Fluchtpunkte je angebrochenem Zeitraum ohne Regung.", UnitFP, "misterx", -10, 0},
	{"camping_misterx_bonus", GroupCamping, "Prämie für die Fahndung",
		"Hausregel, ab Werk aus: Punkte für jedes Fahndungsteam, wenn die " +
			"Zielperson zu lange steht. Das Regelwerk sieht das nicht vor.", UnitPoints, "", 0, 100},
	{"camping_radius_m", GroupCamping, "Was als Bewegung zählt",
		"Wer diesen Umkreis verlässt, gilt als in Bewegung.", UnitMeters, "", 10, 2000},

	// --- Missionen ---
	{"points_mission", GroupMission, "Ziel erreicht",
		"Punkte für ein abgeschlossenes Zwischenziel.", UnitPoints, "misterx", 0, 200},
	{"points_mission_failed", GroupMission, "Frist verstrichen",
		"Abzug, wenn ein Zwischenziel nicht rechtzeitig erreicht wird.", UnitPoints, "misterx", -200, 0},
	{"mission_grace_min", GroupMission, "Kulanz je Schritt",
		"So viel Zeit kommt dazu, wenn ihr am Ziel steht und der Auftrag länger dauert.", UnitMinutes, "misterx", 0, 60},
	{"mission_grace_max_min", GroupMission, "Kulanz insgesamt",
		"Mehr als das gibt es je Zwischenziel nicht – auch nicht in Schritten.", UnitMinutes, "misterx", 0, 180},
	{"mission_fail_lockout_min", GroupMission, "Sperre nach Fehlschlag",
		"So lange steht die Zielperson nach einer verpassten Frist still.", UnitMinutes, "misterx", 0, 60},
	{"hotspot_max_distance_m", GroupMission, "Nachweisabstand",
		"So nah muss jemand am Ziel sein, damit Code oder Foto gelten.", UnitMeters, "", 10, 1000},

	// --- Rätsel und Hinweise ---
	{"points_puzzle", GroupPuzzle, "Rätsel gelöst",
		"Punkte für eine richtige Antwort.", UnitPoints, "detective", 0, 200},
	{"points_puzzle_with_fp", GroupPuzzle, "Rätsel mit Hilfe gelöst",
		"Weniger Punkte, wenn ein Fahndungspunkt für den Hinweis draufging.", UnitPoints, "detective", 0, 200},
	{"intel_hot_min", GroupPuzzle, "Heiße Spur bis",
		"Ein Hinweis gilt so lange als frisch.", UnitMinutes, "", 1, 120},
	{"intel_warm_min", GroupPuzzle, "Laue Spur bis",
		"Danach gilt ein Hinweis als kalt.", UnitMinutes, "", 1, 240},
	{"points_false_trail", GroupPuzzle, "Falsche Fährte gelegt",
		"Punkte für die Zielperson, wenn eine gefälschte Spur greift.", UnitPoints, "misterx", 0, 200},

	// --- Einsatzmittel ---
	{"start_fp_misterx", GroupJoker, "Startguthaben Zielperson",
		"Fluchtpunkte zu Spielbeginn.", UnitFP, "misterx", 0, 50},
	{"start_fp_detective", GroupJoker, "Startguthaben Fahndung",
		"Fahndungspunkte je Team zu Spielbeginn.", UnitFP, "detective", 0, 50},
	{"cost_reroute", GroupJoker, "Ausklinken", "Ein Zwischenziel abwählen.", UnitFP, "misterx", 0, 20},
	{"cost_false_trail", GroupJoker, "Falsche Fährte", "Einen erfundenen Hinweis einspeisen.", UnitFP, "misterx", 0, 20},
	{"cost_delay", GroupJoker, "Zustellverzug", "Den nächsten Hinweis verzögern.", UnitFP, "misterx", 0, 20},
	{"cost_phantom", GroupJoker, "Phantom", "Ein zweites Signal an anderer Stelle.", UnitFP, "misterx", 0, 20},
	{"cost_smoke", GroupJoker, "Nebelkerze", "Ortungen wirkungslos machen, auch laufende.", UnitFP, "misterx", 0, 20},
	{"cost_ghost", GroupJoker, "U-Bahn-Geist", "Für kurze Zeit ganz vom Schirm verschwinden.", UnitFP, "misterx", 0, 20},
	{"cost_gps_ping", GroupJoker, "Ortung", "Den Standort der Zielperson abfragen.", UnitFP, "detective", 0, 20},
	{"cost_lockdown", GroupJoker, "Sperrzone", "Einen Sektor mit einer Falle belegen.", UnitFP, "detective", 0, 20},
	{"cost_bug", GroupJoker, "Wanze", "Einen Hotspot überwachen.", UnitFP, "detective", 0, 20},
	{"cost_scan", GroupJoker, "Sektorscan", "Prüfen, ob die Zielperson in einem Sektor ist.", UnitFP, "detective", 0, 20},
	{"cost_unlock", GroupJoker, "Hinweis kaufen", "Einen Rätselhinweis freikaufen.", UnitFP, "detective", 0, 20},

	// --- Zugriff und Sieg ---
	{"sighting_max_distance_m", GroupArrest, "Sichtweite",
		"So nah muss ein Team sein, damit ein Sichtkontakt zählt.", UnitMeters, "detective", 5, 500},
	{"sighting_dwell_sec", GroupArrest, "Verweildauer",
		"So lange muss der Abstand gehalten werden.", UnitSeconds, "detective", 0, 300},
	{"points_arrest_level1", GroupArrest, "Zugriff Stufe 1 (Lokalisierung)",
		"Punkte dafür, dass die Zielperson jetzt an dem benannten Punkt ist.", UnitPoints, "detective", 0, 200},
	{"points_arrest_level2", GroupArrest, "Zugriff Stufe 2 (Rekonstruktion)",
		"Punkte dafür, dass sie außerdem zur genannten Zeit dort war.", UnitPoints, "detective", 0, 200},
	{"points_arrest_failed", GroupArrest, "Fehlzugriff",
		"Abzug für einen misslungenen Zugriff der dritten Stufe.", UnitPoints, "detective", -200, 0},
	{"arrest_fail_lockout_min", GroupArrest, "Sperre nach Fehlzugriff",
		"So lange ist das Team danach gesperrt.", UnitMinutes, "detective", 0, 60},
	{"points_victory", GroupArrest, "Sieg",
		"Punkte für die Partei, die das Spiel entscheidet.", UnitPoints, "", 0, 1000},
	{"final_shrink_interval_min", GroupArrest, "Enger werdender Suchbereich",
		"In diesem Takt schrumpft der Bereich im Finale.", UnitMinutes, "", 1, 60},

	// --- Abstände ---
	{"radar_blur_m", GroupField, "Unschärfe",
		"So ungenau sieht die Zielperson die Fahndung.", UnitMeters, "", 0, 5000},
	{"radar_blur_scan_m", GroupField, "Unschärfe beim Scan",
		"Dasselbe für das Ergebnis eines Sektorscans.", UnitMeters, "", 0, 5000},
}

// RuleByKey findet einen Eintrag im Verzeichnis.
func RuleByKey(key string) (Rule, bool) {
	for _, r := range RuleIndex {
		if r.Key == key {
			return r, true
		}
	}
	return Rule{}, false
}

// RuleGroups liefert die Gruppen in ihrer Reihenfolge.
func RuleGroups() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, r := range RuleIndex {
		if !seen[r.Group] {
			seen[r.Group] = true
			out = append(out, r.Group)
		}
	}
	return out
}
