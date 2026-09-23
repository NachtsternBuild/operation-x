package de.operationx.app.data

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement

@Serializable
data class ServerStatus(
    val ready: Boolean = false,
    val gameName: String = "",
    val status: String = "",
    val city: String = "",
)

@Serializable
data class AuthResponse(val token: String = "")

/** Der öffentliche Schlüssel des Servers für die zweite Verschlüsselung. */
@Serializable
data class ServerKey(
    val available: Boolean = false,
    val publicKey: String = "",
    val fingerprint: String = "",
)

@Serializable
data class GameInfo(
    val id: String = "",
    val name: String = "",
    val city: String = "",
    val status: String = "",
)

@Serializable
data class Me(
    val id: String = "",
    val callsign: String = "",
    val display: String = "",
    val role: String = "detective",
    val color: String = "",
    val fp: Int = 0,
    val points: Int = 0,
    val onboarded: Boolean = false,
    val game: GameInfo? = null,
)

@Serializable
data class LivePosition(
    val self: Boolean = false,
    val capturedAt: String = "",
    val team: String = "",
    val callsign: String = "",
    val display: String = "",
    val role: String = "",
    val color: String = "",
    val lat: Double = 0.0,
    val lng: Double = 0.0,
    val blurM: Double = 0.0,
    val ageSec: Int = 0,
    val stale: Boolean = false,
    val inTransit: Boolean = false,
)

@Serializable
data class Lockout(
    val kind: String = "",
    val reason: String = "",
    val leftSec: Int = 0,
)

/** Der ausgeloste Startpunkt eines Teams. */
@Serializable
data class StartPoint(
    val number: Int = 0,
    val name: String = "",
    val lat: Double = 0.0,
    val lng: Double = 0.0,
)

/** Eine laufende Pause, mit Grund und geplantem Ende. */
@Serializable
data class PauseInfo(
    val reason: String = "",
    val since: String = "",
    val until: String = "",
    val leftSec: Int = 0,
)

@Serializable
data class SelfState(
    val fp: Int = 0,
    val points: Int = 0,
    val inTransit: Boolean = false,
    val nextDueAt: String = "",
    val dueInSec: Int = 0,
    val violations: Int = 0,
    val overdue: Boolean = false,
    val start: StartPoint? = null,
    val lockouts: List<Lockout> = emptyList(),
)

@Serializable
data class Finale(
    val active: Boolean = false,
    val lat: Double = 0.0,
    val lng: Double = 0.0,
    val radiusM: Double = 0.0,
    val sector: String = "",
)

@Serializable
data class Outcome(val winner: String = "", val reason: String = "")

/** Ein Ereignis, das dem Team gemeldet gehört. */
@Serializable
data class LiveAlert(
    val id: String = "",
    val type: String = "",
    val reason: String = "",
    val at: String = "",
    val urgent: Boolean = false,
)

@Serializable
data class LiveState(
    val now: String = "",
    val status: String = "",
    val self: SelfState = SelfState(),
    val positions: List<LivePosition> = emptyList(),
    val finale: Finale? = null,
    val outcome: Outcome? = null,
    val pause: PauseInfo? = null,
    val alerts: List<LiveAlert> = emptyList(),
)

@Serializable
data class IntelItem(
    val id: String = "",
    val category: String = "",
    val text: String = "",
    val occurredAt: String = "",
    val ageSec: Int = 0,
    val freshness: String = "",
)

@Serializable
data class IntelList(val intel: List<IntelItem> = emptyList())

@Serializable
data class Puzzle(
    val id: String = "",
    val code: String = "",
    val type: String = "",
    val title: String = "",
    val question: String = "",
    val hint: String = "",
    val points: Int = 0,
    val solved: Boolean = false,
    val attempts: Int = 0,
)

@Serializable
data class PuzzleList(val puzzles: List<Puzzle> = emptyList())

@Serializable
data class SolveResult(
    val correct: Boolean = false,
    val points: Int = 0,
    val message: String = "",
)

@Serializable
data class MissionOption(
    val id: String = "",
    val kind: String = "",
    val label: String = "",
    val description: String = "",
    val number: Int = 0,
    val name: String = "",
    val lat: Double = 0.0,
    val lng: Double = 0.0,
    val distanceM: Double = 0.0,
    val timeLimitMin: Int = 0,
    val rewardPoints: Int = 0,
    val rewardFp: Int = 0,
    /** Was vor Ort zu tun ist, sofern die Spielleitung etwas hinterlegt hat. */
    val task: String = "",
)

/** Wie viel Kulanzzeit ein Zwischenziel schon erfahren hat. */
@Serializable
data class GraceState(
    val usedMin: Int = 0,
    val maxMin: Int = 0,
    val leftMin: Int = 0,
    val active: Boolean = false,
)

/** Antwort auf eine gemeldete Verzögerung. */
@Serializable
data class DelayResult(
    val grantedMin: Int = 0,
    val pending: Boolean = false,
    val message: String = "",
    val graceLeftMin: Int = 0,
    val atTarget: Boolean = false,
)

@Serializable
data class MissionState(
    val id: String = "",
    val seq: Int = 0,
    val status: String = "",
    val planned: Int = 0,
    val finished: Int = 0,
    val options: List<MissionOption> = emptyList(),
    val target: MissionOption? = null,
    val deadlineAt: String = "",
    val leftSec: Int = 0,
    val distanceM: Double = 0.0,
    val inRange: Boolean = false,
    val rangeM: Double = 0.0,
    val message: String = "",
    val grace: GraceState? = null,
)

@Serializable
data class Joker(
    val kind: String = "",
    val name: String = "",
    val description: String = "",
    val cost: Int = 0,
    val used: Int = 0,
    val maxUses: Int = 0,
    val available: Boolean = false,
    val reason: String = "",
    val activeUntil: String = "",
)

@Serializable
data class JokerList(val jokers: List<Joker> = emptyList(), val fp: Int = 0)

@Serializable
data class RadioMessage(
    val id: String = "",
    val author: String = "",
    val role: String = "",
    val text: String = "",
    val trust: String = "",
    val audience: String = "",
    val at: String = "",
    val mine: Boolean = false,
)

@Serializable
data class RadioList(@SerialName("messages") val messages: List<RadioMessage> = emptyList())

/** Ergebnis eines gemeldeten Sichtkontakts. */
@Serializable
data class SightingResult(
    val confirmed: Boolean = false,
    val message: String = "",
    val distanceM: Double = 0.0,
    val dwellSec: Int = 0,
)

/** Ergebnis eines Zugriffs. */
@Serializable
data class ArrestResult(
    val level: Int = 0,
    val correct: Boolean = false,
    val points: Int = 0,
    val victory: Boolean = false,
    val message: String = "",
)

/** Ein Regelwert mit Klartextnamen – für den Nachschlag. */
@Serializable
data class RuleEntry(
    val key: String = "",
    val name: String = "",
    val what: String = "",
    val unit: String = "",
    val side: String = "",
    val value: Int = 0,
)

@Serializable
data class RuleGroup(val group: String = "", val rules: List<RuleEntry> = emptyList())

@Serializable
data class RuleBook(val groups: List<RuleGroup> = emptyList())

/** Eine Buchung im Punktekonto. */
@Serializable
data class LedgerEntry(
    val id: String = "",
    val reason: String = "",
    val deltaPoints: Int = 0,
    val deltaFp: Int = 0,
    /** Der Stand unmittelbar nach dieser Buchung. */
    val points: Int = 0,
    val fp: Int = 0,
    val occurredAt: String = "",
    val refType: String = "",
)

@Serializable
data class LedgerBook(
    val team: String = "",
    val callsign: String = "",
    val points: Int = 0,
    val fp: Int = 0,
    val entries: List<LedgerEntry> = emptyList(),
)

@Serializable
data class Hotspot(
    val id: String = "",
    val number: Int = 0,
    val name: String = "",
    val lat: Double = 0.0,
    val lng: Double = 0.0,
    val sector: String = "",
)

@Serializable
data class Sector(
    val id: String = "",
    val code: String = "",
    val name: String = "",
    val color: String = "",
    // Die Grenze als rohes GeoJSON. Sie fehlte hier bisher ganz – der Server
    // schickte sie, die App warf sie beim Einlesen weg, und die Karte konnte
    // nie einen Sektor zeichnen.
    val geometry: JsonElement? = null,
)

@Serializable
data class FieldMap(
    val game: String = "",
    val city: String = "",
    val sectors: List<Sector> = emptyList(),
    val hotspots: List<Hotspot> = emptyList(),
)

/** Eine gepufferte Standortmeldung. */
@Serializable
data class PositionReport(
    val lat: Double,
    val lng: Double,
    val accuracy: Double = 0.0,
    val speed: Double = 0.0,
    val heading: Double = 0.0,
    val capturedAt: String,
    val mocked: Boolean = false,
)

@Serializable
data class PositionBatch(val positions: List<PositionReport>)

@Serializable
data class PositionAck(
    val accepted: Int = 0,
    val nextDueAt: String = "",
    val warnings: List<String> = emptyList(),
    /** Während einer Pause nimmt der Server nichts an – und das ist richtig so. */
    val paused: Boolean = false,
)
