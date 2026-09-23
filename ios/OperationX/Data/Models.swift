import Foundation

// Die Antworten des Servers, eins zu eins.
//
// Dieselben Felder wie in android/.../data/Models.kt – beide Apps sprechen
// dieselbe Schnittstelle unter /api/opx, und die ist die einzige gemeinsame
// Grundlage. Wer hier ein Feld ändert, ändert es dort auch.
//
// Warum hier alles optional ist, obwohl die Kotlin-Fassung Vorgabewerte hat:
// Swift benutzt die Vorgabewerte einer Struktur beim Dekodieren nicht. Ein
// Feld, das der Server weglässt – und er lässt jedes leere Feld weg –, würde
// den ganzen Dekodiervorgang werfen und damit die komplette Antwort
// verschlucken. Ein optionales Feld darf fehlen. Der Preis sind die "?? 0" an
// den Lesestellen; er ist an dieser Stelle der richtige.

// MARK: - Server und Anmeldung

struct ServerStatus: Decodable {
    var ready: Bool?
    var gameName: String?
    var status: String?
    var city: String?
}

struct AuthResponse: Decodable {
    var token: String?
}

/// Der öffentliche Schlüssel des Servers für die zweite Verschlüsselung.
struct ServerKey: Decodable {
    var available: Bool?
    var publicKey: String?
    var fingerprint: String?
}

struct GameInfo: Decodable {
    var id: String?
    var name: String?
    var city: String?
    var status: String?
    var startsAt: String?
    var endsAt: String?
    var durationMin: Int?
}

struct Me: Decodable {
    var id: String?
    var callsign: String?
    var display: String?
    var role: String?
    var color: String?
    var fp: Int?
    var points: Int?
    var onboarded: Bool?
    var game: GameInfo?
}

// MARK: - Lage

struct LivePosition: Decodable, Identifiable {
    var team: String?
    var callsign: String?
    var display: String?
    var role: String?
    var color: String?
    var lat: Double?
    var lng: Double?
    var blurM: Double?
    var ageSec: Int?
    var stale: Bool?
    var inTransit: Bool?
    /// Der Server nennt das Feld "self"; das Wort ist in Swift belegt.
    var isSelf: Bool?

    var id: String { team ?? callsign ?? "" }

    enum CodingKeys: String, CodingKey {
        case team, callsign, display, role, color, lat, lng, blurM, ageSec, stale, inTransit
        case isSelf = "self"
    }
}

struct Lockout: Decodable {
    var kind: String?
    var reason: String?
    var leftSec: Int?
}

/// Der ausgeloste Startpunkt eines Teams.
struct StartPoint: Decodable {
    var number: Int?
    var name: String?
    var lat: Double?
    var lng: Double?
}

/// Eine laufende Pause, mit Grund und geplantem Ende.
struct PauseInfo: Decodable {
    var reason: String?
    var since: String?
    var until: String?
    var leftSec: Int?
}

struct SelfState: Decodable {
    var fp: Int?
    var points: Int?
    var inTransit: Bool?
    var nextDueAt: String?
    var dueInSec: Int?
    var violations: Int?
    var overdue: Bool?
    var start: StartPoint?
    var lockouts: [Lockout]?
}

struct Finale: Decodable {
    var active: Bool?
    var lat: Double?
    var lng: Double?
    var radiusM: Double?
    var sector: String?
}

struct Outcome: Decodable {
    var winner: String?
    var reason: String?
}

/// Ein Ereignis, das dem Team gemeldet gehört.
struct LiveAlert: Decodable, Identifiable {
    var id: String?
    var type: String?
    var reason: String?
    var at: String?
    var urgent: Bool?
}

struct LiveState: Decodable {
    var now: String?
    var status: String?
    var selfState: SelfState?
    var positions: [LivePosition]?
    var finale: Finale?
    var outcome: Outcome?
    var pause: PauseInfo?
    var alerts: [LiveAlert]?

    enum CodingKeys: String, CodingKey {
        case now, status, positions, finale, outcome, pause, alerts
        case selfState = "self"
    }
}

// MARK: - Spielfeld

struct Hotspot: Decodable, Identifiable {
    var id: String?
    var number: Int?
    var name: String?
    var lat: Double?
    var lng: Double?
    var sector: String?
}

struct Sector: Decodable, Identifiable {
    var id: String?
    var code: String?
    var name: String?
    var color: String?
    /// Die Grenze als GeoJSON, erst auf der Karte gelesen.
    var geometry: GeoJSONGeometry?
}

struct FieldMap: Decodable {
    var game: String?
    var city: String?
    var sectors: [Sector]?
    var hotspots: [Hotspot]?
}

/// Nur so viel GeoJSON, wie die Karte braucht: Polygon und MultiPolygon.
///
/// Beides endet hier als flache Liste von Ringen. Ein Sektor ist ein Umriss,
/// und ob die Verwaltung ihn in einem oder in drei Stücken führt, ändert an
/// der Darstellung nichts.
struct GeoJSONGeometry: Decodable {
    var type: String = ""
    var rings: [[[Double]]] = []

    enum CodingKeys: String, CodingKey { case type, coordinates }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        type = (try? c.decode(String.self, forKey: .type)) ?? ""

        if type == "MultiPolygon" {
            let polygone = (try? c.decode([[[[Double]]]].self, forKey: .coordinates)) ?? []
            rings = polygone.flatMap { $0 }
        } else {
            rings = (try? c.decode([[[Double]]].self, forKey: .coordinates)) ?? []
        }
    }
}

// MARK: - Standort

struct PositionReport: Encodable {
    var lat: Double
    var lng: Double
    var accuracy: Double = 0
    var speed: Double = 0
    var heading: Double = 0
    var capturedAt: String
    var mocked: Bool = false
}

struct PositionAck: Decodable {
    var accepted: Int?
    var nextDueAt: String?
    var warnings: [String]?
    /// Während einer Pause nimmt der Server nichts an – und das ist richtig so.
    var paused: Bool?
}

// MARK: - Hinweise und Rätsel

struct IntelItem: Decodable, Identifiable {
    var id: String?
    var category: String?
    var text: String?
    var occurredAt: String?
    var ageSec: Int?
    var freshness: String?
}

struct IntelList: Decodable {
    var intel: [IntelItem]?
}

struct Puzzle: Decodable, Identifiable {
    var id: String?
    var code: String?
    var type: String?
    var title: String?
    var question: String?
    var hint: String?
    var points: Int?
    var solved: Bool?
    var attempts: Int?
}

struct PuzzleList: Decodable {
    var puzzles: [Puzzle]?
}

struct SolveResult: Decodable {
    var correct: Bool?
    var points: Int?
    var message: String?
}

// MARK: - Missionen

struct MissionOption: Decodable, Identifiable {
    var id: String?
    var kind: String?
    var label: String?
    var description: String?
    var number: Int?
    var name: String?
    var lat: Double?
    var lng: Double?
    var distanceM: Double?
    var timeLimitMin: Int?
    var rewardPoints: Int?
    var rewardFp: Int?
    /// Was vor Ort zu tun ist, sofern die Spielleitung etwas hinterlegt hat.
    var task: String?
}

struct GraceState: Decodable {
    var usedMin: Int?
    var maxMin: Int?
    var leftMin: Int?
    var active: Bool?
}

struct DelayResult: Decodable {
    var grantedMin: Int?
    var pending: Bool?
    var message: String?
    var graceLeftMin: Int?
    var atTarget: Bool?
}

struct MissionState: Decodable {
    var id: String?
    var seq: Int?
    var status: String?
    var planned: Int?
    var finished: Int?
    var options: [MissionOption]?
    var target: MissionOption?
    var deadlineAt: String?
    var leftSec: Int?
    var distanceM: Double?
    var inRange: Bool?
    var rangeM: Double?
    var message: String?
    var grace: GraceState?
}

// MARK: - Einsatzmittel, Funk, Zugriff

struct Joker: Decodable, Identifiable {
    var kind: String?
    var name: String?
    var description: String?
    var cost: Int?
    var used: Int?
    var maxUses: Int?
    var available: Bool?
    var reason: String?
    var activeUntil: String?

    var id: String { kind ?? "" }
}

struct JokerList: Decodable {
    var jokers: [Joker]?
    var fp: Int?
}

struct RadioMessage: Decodable, Identifiable {
    var id: String?
    var author: String?
    var role: String?
    var text: String?
    var trust: String?
    var audience: String?
    var at: String?
    var mine: Bool?
}

struct RadioList: Decodable {
    var messages: [RadioMessage]?
}

struct SightingResult: Decodable {
    var confirmed: Bool?
    var message: String?
    var distanceM: Double?
    var dwellSec: Int?
}

struct ArrestResult: Decodable {
    var level: Int?
    var correct: Bool?
    var points: Int?
    var victory: Bool?
    var message: String?
}

// MARK: - Regeln und Konto

struct RuleEntry: Decodable, Identifiable {
    var key: String?
    var name: String?
    var what: String?
    var unit: String?
    var side: String?
    var value: Int?

    var id: String { key ?? "" }
}

struct RuleGroup: Decodable, Identifiable {
    var group: String?
    var rules: [RuleEntry]?

    var id: String { group ?? "" }
}

struct RuleBook: Decodable {
    var groups: [RuleGroup]?
}

struct LedgerEntry: Decodable, Identifiable {
    var id: String?
    var reason: String?
    var deltaPoints: Int?
    var deltaFp: Int?
    /// Der Stand unmittelbar nach dieser Buchung.
    var points: Int?
    var fp: Int?
    var occurredAt: String?
    var refType: String?
}

struct LedgerBook: Decodable {
    var team: String?
    var callsign: String?
    var points: Int?
    var fp: Int?
    var entries: [LedgerEntry]?
}
