package de.operationx.app.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import de.operationx.app.UiState
import de.operationx.app.regel
import de.operationx.app.data.Hotspot
import de.operationx.app.data.IntelItem
import de.operationx.app.data.LivePosition
import de.operationx.app.ui.theme.MonoStyle
import de.operationx.app.ui.theme.PartyColors
import kotlinx.coroutines.delay

/**
 * Die Feldansicht.
 *
 * Was hier steht, muss im Gehen bedienbar sein: Der Countdown bis zur nächsten
 * Pflichtmeldung ganz oben, der Melde-Knopf darunter, alles Weitere danach.
 * Die Reihenfolge ändert sich nie, damit der Daumen sie findet, ohne dass
 * jemand hinsieht.
 */
@Composable
fun FieldScreen(
    state: UiState,
    onPing: () -> Unit,
    onTransit: () -> Unit,
    onTracking: (Boolean) -> Unit,
    onFieldMode: (Boolean) -> Unit,
    onLogout: () -> Unit,
    onChooseOption: (String) -> Unit,
    onPasscode: (String) -> Unit,
    onPhoto: () -> Unit,
    onDelay: (String) -> Unit,
    onSolve: (String, String) -> Unit,
    onSighting: () -> Unit,
    onArrest: (Int, Int, String?, Int?) -> Unit,
    onUseJoker: (String, String?, String?) -> Unit,
    onSendRadio: (String) -> Unit,
    onOpenRules: () -> Unit,
    onOpenAccount: () -> Unit,
) {
    val me = state.me ?: return
    val roleColor = PartyColors.forRole(me.role)
    val isMisterX = me.role == "misterx"

    // Die Karte gehört dem ganzen Bildschirm und nicht dem Reiter "Lage".
    // Sonst wäre sie bei jedem Wechsel neu zu bauen: schwarze Sekunde,
    // Kacheln noch einmal holen, Ausschnitt zurück auf Anfang.
    val karte = rememberKarte(state.serverUrl)

    // Die Reiter unterscheiden sich je Rolle: Die Zielperson hat ein
    // Missionsbuch, die Fahndung eine Ermittlungsseite. Der Rest ist gleich.
    val tabs = remember(me.role) {
        buildList {
            add("Lage")
            add(if (isMisterX) "Mission" else "Ermittlung")
            add("Mittel")
            add("Funk")
        }
    }
    var tab by remember { mutableIntStateOf(0) }

    Column(Modifier.fillMaxSize()) {
        TopBar(state, roleColor, onFieldMode, onLogout, onOpenRules, onOpenAccount)
        PingBar(state, onPing, onTransit, onTracking)

        state.live?.outcome?.let { OutcomeBanner(it.winner, it.reason) }
        state.live?.finale?.takeIf { it.active }?.let { FinaleBanner(it.radiusM, it.sector) }

        // Die Pause gehört nach oben, und sie muss sagen, wofür: "Angehalten"
        // allein ist draußen von einer Störung nicht zu unterscheiden – und
        // genau dann drückt jemand am Gerät herum, statt in Ruhe zu essen.
        if (state.live?.status == "paused") {
            PauseBanner(state.live?.pause)
        }

        // Der Startpunkt, solange das Spiel nicht läuft: Bis dahin ist er die
        // einzige Auskunft, die jemand braucht.
        state.live?.self?.start
            ?.takeIf { state.live?.status != "running" && state.live?.status != "finished" }
            ?.let { StartBanner(it) }

        TabRow(selectedTabIndex = tab) {
            tabs.forEachIndexed { index, title ->
                Tab(
                    selected = tab == index,
                    onClick = { tab = index },
                    text = { Text(title, style = MaterialTheme.typography.labelLarge) },
                )
            }
        }

        // Die Tastatur nimmt sich ihren Platz vom Inhalt, nicht vom Bildschirm.
        //
        // Ohne das lag sie über den Eingabefeldern aller Reiter: Beim Melden
        // einer Verzögerung standen Feld und "Melden"-Knopf dahinter, und weil
        // der Inhalt in voller Höhe weiterlief, ließ sich auch nichts
        // heranscrollen. Man tippte blind und kam an den Knopf nur, indem man
        // die Tastatur wieder wegwischte.
        Box(
            Modifier
                .weight(1f)
                .imePadding()
        ) {
            when (tab) {
                0 -> SituationTab(state, karte)
                1 -> if (isMisterX) {
                    MissionPanel(
                        mission = state.mission,
                        delay = state.delay,
                        busy = state.busy,
                        onChoose = onChooseOption,
                        onPasscode = onPasscode,
                        onPhoto = onPhoto,
                        onDelay = onDelay,
                    )
                } else {
                    PuzzlePanel(
                        puzzles = state.puzzles,
                        regeln = ZugriffsRegeln(
                            abzug = state.regel("points_arrest_failed", -15),
                            sperreMin = state.regel("arrest_fail_lockout_min", 10),
                            nachweisM = state.regel("hotspot_max_distance_m", 150),
                            verweilSek = state.regel("sighting_dwell_sec", 20),
                        ),
                        hotspotNumbers = state.field?.hotspots.orEmpty().map { it.number },
                        sighting = state.sighting,
                        arrest = state.arrest,
                        busy = state.busy,
                        onSolve = onSolve,
                        onSighting = onSighting,
                        onArrest = onArrest,
                    )
                }
                2 -> JokerPanel(
                    jokers = state.jokers,
                    fp = state.live?.self?.fp ?: me.fp,
                    sectors = state.field?.sectors.orEmpty(),
                    hotspots = state.field?.hotspots.orEmpty(),
                    onUse = onUseJoker,
                )
                else -> RadioPanel(
                    messages = state.radio,
                    canWrite = !isMisterX,
                    onSend = onSendRadio,
                )
            }
        }
    }
}

/** Karte oben, Hinweise und Teamliste darunter. */
@Composable
private fun SituationTab(state: UiState, karte: MapHolder) {
    val isMisterX = state.me?.role == "misterx"
    Column(Modifier.fillMaxSize()) {
        MapPanel(
            karte = karte,
            field = state.field,
            live = state.live,
            modifier = Modifier
                .fillMaxWidth()
                .weight(1f),
        )

        LazyColumn(
            modifier = Modifier
                .fillMaxWidth()
                .weight(1f)
                .padding(horizontal = 12.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
            contentPadding = PaddingValues(vertical = 10.dp),
        ) {
            if (state.intel.isNotEmpty()) {
                item { SectionLabel("Hinweise") }
                items(state.intel, key = { it.id }) { IntelCard(it) }
            }

            item { SectionLabel("Lage") }
            val others = state.live?.positions.orEmpty()
            if (others.isEmpty()) {
                item {
                    Text(
                        if (state.tracking) {
                            "Noch keine Meldung eingegangen. Sobald eine Ortung " +
                                "gelingt, erscheint ihr hier."
                        } else {
                            "Die Erfassung ist aus. Ohne sie meldet niemand einen " +
                                "Standort — auch ihr nicht."
                        },
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            } else {
                items(others, key = { it.team }) { TeamRow(it) }
            }

            // Was die Karte *nicht* zeigt, ist am Spieltag die wichtigere
            // Auskunft: Ein leerer Fleck kann heißen "keine Daten" oder "nicht
            // für euch". Ohne diesen Satz hält man das eine für das andere und
            // wartet auf etwas, das nie kommt.
            item {
                Text(
                    if (isMisterX) {
                        "Die Fahndung erscheint unscharf — rund " +
                            "${state.regel("radar_blur_m", 200)} Meter. Das ist keine " +
                            "schlechte Ortung, sondern die Regel."
                    } else {
                        "Die Zielperson erscheint hier nur, solange eine Ortung " +
                            "läuft. Ansonsten müsst ihr sie euch erarbeiten."
                    },
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(top = 6.dp),
                )
            }
        }
    }
}

@Composable
private fun TopBar(
    state: UiState,
    roleColor: Color,
    onFieldMode: (Boolean) -> Unit,
    onLogout: () -> Unit,
    onOpenRules: () -> Unit,
    onOpenAccount: () -> Unit,
) {
    val me = state.me ?: return

    Surface(color = MaterialTheme.colorScheme.surface, tonalElevation = 3.dp) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 12.dp, vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Box(
                Modifier
                    .size(10.dp)
                    .clip(RoundedCornerShape(5.dp))
                    .background(roleColor)
            )
            Spacer(Modifier.width(8.dp))

            Column(Modifier.weight(1f)) {
                Text(
                    me.display.ifBlank { me.callsign },
                    style = MaterialTheme.typography.titleMedium,
                    fontWeight = FontWeight.SemiBold,
                )
                Text(
                    when (me.role) {
                        "misterx" -> "Zielperson"
                        "hq" -> "Einsatzzentrale"
                        else -> "Fahndungsteam"
                    },
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }

            // Punktestand aus der Lage, nicht aus dem Zugangsdatensatz.
            //
            // Beides steht in der App, und nur eines davon ist aktuell: Die
            // Lage kommt über den Strom, sobald sich etwas ändert; der
            // Zugangsdatensatz erst beim nächsten vollen Umlauf, also bis zu
            // einer Minute später. Auf dem Prüfgerät buchte die Zentrale
            // zweiundvierzig Punkte, und die Kopfleiste zeigte weiter null.
            val punkte = state.live?.self?.points ?: me.points
            val fp = state.live?.self?.fp ?: me.fp

            // Der Stand ist der Knopf zu seiner eigenen Begründung: Gefragt
            // wird, während man auf die Zahl sieht, nicht im Menü.
            Column(
                horizontalAlignment = Alignment.End,
                modifier = Modifier
                    .clip(RoundedCornerShape(6.dp))
                    .clickable(onClick = onOpenAccount)
                    .padding(horizontal = 6.dp, vertical = 2.dp),
            ) {
                Text("$punkte Pkt", style = MonoStyle.merge(MaterialTheme.typography.bodyMedium))
                Text(
                    "$fp FP",
                    style = MonoStyle.merge(MaterialTheme.typography.bodySmall),
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }

            // Das Schloss zeigt, ob zusätzlich verschlüsselt wird. Die vier
            // Zeichen daneben sind der Anfang des Kennzeichens: Wer wissen
            // will, ob sein Gerät wirklich mit diesem Server spricht, hält sie
            // neben den Bildschirm der Zentrale.
            state.fingerprint?.let { kennzeichen ->
                Spacer(Modifier.width(6.dp))
                Text(
                    "🔒 " + kennzeichen.take(4),
                    style = MonoStyle.merge(MaterialTheme.typography.bodySmall),
                    color = PartyColors.ok,
                )
            }

            Spacer(Modifier.width(4.dp))
            OverflowMenu(state.fieldMode, onFieldMode, onLogout, onOpenRules)
        }
    }
}

/**
 * Das Menü rechts oben.
 *
 * Vorher standen hier drei Knöpfe nebeneinander; auf einem 720 Pixel breiten
 * Gerät blieb vom letzten nur "Ab" übrig. Was selten gebraucht wird, gehört
 * deshalb hinter die drei Punkte — der Platz in der Kopfleiste gehört dem
 * Rufzeichen und dem Punktestand.
 */
@Composable
private fun OverflowMenu(
    fieldMode: Boolean,
    onFieldMode: (Boolean) -> Unit,
    onLogout: () -> Unit,
    onOpenRules: () -> Unit,
) {
    var open by remember { mutableStateOf(false) }

    Box {
        IconButton(onClick = { open = true }) {
            Icon(Icons.Default.MoreVert, contentDescription = "Mehr")
        }
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            DropdownMenuItem(
                text = { Text("Regeln nachschlagen") },
                onClick = { open = false; onOpenRules() },
            )
            DropdownMenuItem(
                text = { Text(if (fieldMode) "Feldmodus aus" else "Feldmodus an") },
                onClick = { open = false; onFieldMode(!fieldMode) },
            )
            HorizontalDivider()
            DropdownMenuItem(
                text = { Text("Abmelden") },
                onClick = { open = false; onLogout() },
            )
        }
    }
}

/**
 * Der wichtigste Bedienteil: Countdown und Melde-Knopf.
 *
 * Der Countdown läuft im Gerät weiter, auch zwischen zwei Serverabfragen –
 * sonst stünde eine Minute lang dieselbe Zahl da, während die Frist abläuft.
 */
@Composable
private fun PingBar(
    state: UiState,
    onPing: () -> Unit,
    onTransit: () -> Unit,
    onTracking: (Boolean) -> Unit,
) {
    val due = state.live?.self?.dueInSec ?: 0
    var seconds by remember(state.live?.now) { mutableIntStateOf(due) }

    LaunchedEffect(state.live?.now) {
        while (true) {
            delay(1000)
            seconds -= 1
        }
    }

    val urgent = seconds < 120
    val overdue = seconds < 0

    val tint = when {
        overdue -> PartyColors.critical
        urgent -> PartyColors.warn
        else -> MaterialTheme.colorScheme.primary
    }

    Surface(
        modifier = Modifier.fillMaxWidth(),
        color = if (overdue) PartyColors.critical.copy(alpha = 0.12f)
        else MaterialTheme.colorScheme.surfaceVariant,
    ) {
        Column(Modifier.fillMaxWidth().padding(12.dp)) {
            Row(
                Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Button(
                    onClick = onPing,
                    enabled = !state.reporting,
                    modifier = Modifier.weight(1f).height(52.dp),
                ) {
                    Text(if (state.reporting) "Wird gemeldet …" else "Standort melden")
                }

                Spacer(Modifier.width(14.dp))

                // Der Countdown rechtsbündig: So steht die Zahl immer an
                // derselben Stelle, egal wie breit sie gerade ist.
                Column(horizontalAlignment = Alignment.End) {
                    Text(
                        if (overdue) "Überfällig seit" else "Nächste Meldung in",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Text(
                        formatClock(seconds),
                        style = MonoStyle.merge(MaterialTheme.typography.headlineSmall),
                        color = tint,
                    )
                }
            }

            Spacer(Modifier.height(8.dp))

            Row(
                Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                FilterChip(
                    selected = state.live?.self?.inTransit == true,
                    onClick = onTransit,
                    label = { Text("Transit") },
                )
                FilterChip(
                    selected = state.tracking,
                    onClick = { onTracking(!state.tracking) },
                    label = { Text(if (state.tracking) "Erfassung an" else "Erfassung aus") },
                )
                if (state.buffered > 0) {
                    AssistChip(
                        onClick = {},
                        label = { Text("${state.buffered} gepuffert") },
                    )
                }
            }

            state.live?.self?.lockouts?.firstOrNull()?.let { lock ->
                Spacer(Modifier.height(8.dp))
                Text(
                    "Sperre: ${lock.reason} — noch ${formatClock(lock.leftSec)}",
                    style = MaterialTheme.typography.bodySmall,
                    color = PartyColors.warn,
                )
            }
        }
    }
}

@Composable
private fun SectionLabel(text: String) {
    Text(
        text.uppercase(),
        style = MaterialTheme.typography.labelMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

@Composable
private fun IntelCard(item: IntelItem) {
    val accent = when (item.category) {
        "green" -> PartyColors.intelGreen
        "yellow" -> PartyColors.intelYellow
        else -> PartyColors.intelRed
    }

    // Die Frische steuert, wie laut ein Hinweis auftritt: Ein kalter tritt
    // zurück, ein heißer muss auffallen.
    val alpha = when (item.freshness) {
        "hot" -> 1f
        "warm" -> 0.85f
        else -> 0.6f
    }

    Card(Modifier.fillMaxWidth()) {
        Row(Modifier.padding(12.dp)) {
            Box(
                Modifier
                    .width(4.dp)
                    .height(38.dp)
                    .clip(RoundedCornerShape(2.dp))
                    .background(accent.copy(alpha = alpha))
            )
            Spacer(Modifier.width(10.dp))
            Column {
                Text(
                    when (item.freshness) {
                        "hot" -> "HEISS"
                        "warm" -> "WARM"
                        else -> "KALT"
                    } + " · " + formatAge(rememberAge(item.occurredAt, item.ageSec)),
                    style = MaterialTheme.typography.labelMedium,
                    color = accent.copy(alpha = alpha),
                )
                Text(item.text, style = MaterialTheme.typography.bodyMedium)
            }
        }
    }
}

@Composable
private fun TeamRow(pos: LivePosition) {
    val color = runCatching { Color(android.graphics.Color.parseColor(pos.color)) }
        .getOrElse { PartyColors.forRole(pos.role) }

    Row(
        Modifier.fillMaxWidth().padding(vertical = 2.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(
            Modifier
                .size(9.dp)
                .clip(RoundedCornerShape(5.dp))
                .background(color)
        )
        Spacer(Modifier.width(10.dp))
        Text(
            pos.display.ifBlank { pos.callsign },
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.weight(1f),
        )
        if (pos.blurM > 0) {
            Text(
                "±${pos.blurM.toInt()} m",
                style = MonoStyle.merge(MaterialTheme.typography.bodySmall),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Spacer(Modifier.width(8.dp))
        }
        val age = rememberAge(pos.capturedAt, pos.ageSec)
        Text(
            formatAge(age),
            style = MonoStyle.merge(MaterialTheme.typography.bodySmall),
            // Veraltet wird jetzt ebenfalls lokal beurteilt: Der Wert vom
            // Server wäre zwischen zwei Sendungen eingefroren.
            color = if (age > 900) PartyColors.warn else MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun PauseBanner(pause: de.operationx.app.data.PauseInfo?) {
    val bis = pause?.until.orEmpty().let { if (it.isBlank()) "" else uhrzeitLokal(it) }

    Surface(color = PartyColors.hq.copy(alpha = 0.18f)) {
        Column(Modifier.padding(12.dp)) {
            Text(
                "Pause" + (pause?.reason.orEmpty().takeIf { it.isNotBlank() }?.let { " — $it" } ?: ""),
                style = MaterialTheme.typography.titleMedium,
                color = PartyColors.hq,
            )
            Text(
                (if (bis.isNotBlank()) "Weiter gegen $bis Uhr. " else "") +
                    "Es läuft keine Frist, und euer Standort wird nicht aufgezeichnet.",
                style = MaterialTheme.typography.bodySmall,
            )
        }
    }
}

@Composable
private fun StartBanner(start: de.operationx.app.data.StartPoint) {
    Surface(color = PartyColors.ok.copy(alpha = 0.14f)) {
        Column(Modifier.padding(12.dp)) {
            Text(
                "Euer Startpunkt: #${start.number.toString().padStart(2, '0')} ${start.name}",
                style = MaterialTheme.typography.titleMedium,
                color = PartyColors.ok,
            )
            Text(
                "Dort beginnt euer Spiel, sobald die Zentrale startet.",
                style = MaterialTheme.typography.bodySmall,
            )
        }
    }
}

@Composable
private fun FinaleBanner(radiusM: Double, sector: String) {
    Surface(color = PartyColors.hq.copy(alpha = 0.15f)) {
        Text(
            "Finale. Der Suchbereich liegt offen und zieht sich zusammen — " +
                "aktuell ${radiusM.toInt()} m" + if (sector.isNotBlank()) " um $sector." else ".",
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.padding(12.dp),
        )
    }
}

@Composable
private fun OutcomeBanner(winner: String, reason: String) {
    val color = when (winner) {
        "misterx" -> PartyColors.misterX
        "detectives" -> PartyColors.detective
        else -> PartyColors.hq
    }

    Surface(color = color.copy(alpha = 0.16f)) {
        Column(Modifier.padding(12.dp)) {
            Text(
                when (winner) {
                    "misterx" -> "Die Zielperson hat gewonnen."
                    "detectives" -> "Die Fahndung hat gewonnen."
                    else -> "Unentschieden."
                },
                style = MaterialTheme.typography.titleMedium,
                color = color,
            )
            Text(reason, style = MaterialTheme.typography.bodySmall)
        }
    }
}

private fun formatClock(seconds: Int): String {
    val s = kotlin.math.abs(seconds)
    val sign = if (seconds < 0) "−" else ""
    return "$sign${s / 60}:${(s % 60).toString().padStart(2, '0')}"
}

private fun formatAge(seconds: Int): String = when {
    seconds < 60 -> "jetzt"
    seconds < 3600 -> "vor ${seconds / 60} min"
    else -> "vor ${seconds / 3600} h"
}

/**
 * Das Alter einer Meldung, im Gerät weitergezählt.
 *
 * Seit die Lage aus dem Strom kommt, treffen Aktualisierungen nur noch ein,
 * wenn sich wirklich etwas geändert hat – und das Alter ist genau das, was
 * sich auch ohne Änderung ändert. Vom Server übernommen bliebe "vor 2 min"
 * minutenlang stehen, während die Meldung längst zehn Minuten alt ist. Auf
 * einer Fahndungskarte ist das kein Schönheitsfehler.
 */
@Composable
private fun rememberAge(capturedAt: String, fallbackSec: Int): Int {
    val captured = remember(capturedAt) { parseIso(capturedAt) }
    var age by remember(capturedAt) {
        mutableIntStateOf(
            if (captured > 0) ((System.currentTimeMillis() - captured) / 1000).toInt()
            else fallbackSec
        )
    }

    LaunchedEffect(capturedAt) {
        while (captured > 0) {
            age = ((System.currentTimeMillis() - captured) / 1000).toInt().coerceAtLeast(0)
            delay(1000)
        }
    }
    return age
}
