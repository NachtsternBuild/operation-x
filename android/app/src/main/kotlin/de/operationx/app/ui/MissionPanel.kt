package de.operationx.app.ui

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.foundation.background
import de.operationx.app.data.DelayResult
import de.operationx.app.data.MissionOption
import de.operationx.app.data.MissionState
import de.operationx.app.ui.theme.MonoStyle
import de.operationx.app.ui.theme.PartyColors
import kotlinx.coroutines.delay

/**
 * Das Missionsbuch.
 *
 * Drei Zustände: Varianten stehen zur Wahl, eine Mission läuft, oder es gibt
 * nichts zu tun. Der Nachweis-Knopf wird erst bedienbar, wenn das Gerät nah
 * genug ist – ein ausgegrauter Knopf ohne Begründung wäre im Feld nur ärgerlich,
 * deshalb steht der fehlende Abstand daneben.
 */
@Composable
fun MissionPanel(
    mission: MissionState?,
    delay: DelayResult?,
    busy: Boolean,
    onChoose: (String) -> Unit,
    onPasscode: (String) -> Unit,
    onPhoto: () -> Unit,
    onDelay: (String) -> Unit,
) {
    if (mission == null) {
        Text(
            "Missionsbuch wird geladen …",
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(16.dp),
        )
        return
    }

    Column(
        Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(12.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        when (mission.status) {
            "proposed" -> {
                Text(
                    "Zwischenziel ${mission.seq} von ${mission.planned}",
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Text("Route wählen", style = MaterialTheme.typography.headlineSmall)
                mission.options.forEach { OptionCard(it) { onChoose(it.id) } }
            }

            "active" -> ActiveMission(mission, delay, busy, onPasscode, onPhoto, onDelay)

            else -> Text(
                mission.message.ifBlank { "Zurzeit ist kein Zwischenziel offen." },
                style = MaterialTheme.typography.bodyMedium,
            )
        }
    }
}

@Composable
private fun OptionCard(option: MissionOption, onPick: () -> Unit) {
    val accent = when (option.kind) {
        "safe" -> PartyColors.ok
        "fast" -> PartyColors.warn
        else -> PartyColors.hq
    }

    Card(onClick = onPick, modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(option.label, style = MaterialTheme.typography.titleMedium, color = accent)
            Text(
                "#${option.number.toString().padStart(2, '0')} ${option.name}",
                style = MaterialTheme.typography.bodyLarge,
                fontWeight = FontWeight.Medium,
            )
            Text(
                option.description,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (option.task.isNotBlank()) {
                Text(
                    "Vor Ort: ${option.task}",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Text(
                "${option.distanceM.toInt()} m · ${option.timeLimitMin} min · " +
                    "${option.rewardPoints} Pkt" +
                    if (option.rewardFp > 0) " + ${option.rewardFp} FP" else "",
                style = MonoStyle.merge(MaterialTheme.typography.bodySmall),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun ActiveMission(
    mission: MissionState,
    delay: DelayResult?,
    busy: Boolean,
    onPasscode: (String) -> Unit,
    onPhoto: () -> Unit,
    onDelay: (String) -> Unit,
) {
    var delayOpen by remember { mutableStateOf(false) }
    var delayReason by remember { mutableStateOf("") }
    var code by remember { mutableStateOf("") }
    var left by remember(mission.deadlineAt) { mutableIntStateOf(mission.leftSec) }

    LaunchedEffect(mission.deadlineAt) {
        while (true) {
            delay(1000)
            left -= 1
        }
    }

    val target = mission.target
    val urgent = left < 300

    Text(
        "Zwischenziel ${mission.seq} von ${mission.planned}",
        style = MaterialTheme.typography.labelMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    Text(
        "#${target?.number.toString().padStart(2, '0')} ${target?.name.orEmpty()}",
        style = MaterialTheme.typography.headlineSmall,
    )

    // Die Aufgabe steht über der Uhr: Sie ist der Grund, dort zu sein.
    val aufgabe = target?.task.orEmpty()
    if (aufgabe.isNotBlank()) {
        Card(Modifier.fillMaxWidth()) {
            Text(
                aufgabe,
                Modifier.padding(14.dp),
                style = MaterialTheme.typography.bodyMedium,
            )
        }
    }

    Row(horizontalArrangement = Arrangement.spacedBy(24.dp)) {
        Stat("Rest", formatMissionClock(left), if (urgent) PartyColors.critical else null)
        Stat("Abstand", "${mission.distanceM.toInt()} m", null)
        Stat("Nachweis ab", "${mission.rangeM.toInt()} m", null)
    }

    if (mission.inRange) {
        OutlinedTextField(
            value = code,
            onValueChange = { code = it.uppercase() },
            label = { Text("Code vor Ort") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        Button(
            onClick = { onPasscode(code); code = "" },
            enabled = code.isNotBlank(),
            modifier = Modifier.fillMaxWidth().height(50.dp),
        ) {
            Text("Ankunft bestätigen")
        }

        // Der zweite Weg. Er ist der wichtigere, wenn der Zettel fehlt – und
        // Zettel im öffentlichen Raum verschwinden. Ohne ihn müsste jemand
        // mitten im Spiel in den Browser wechseln.
        OutlinedButton(
            onClick = onPhoto,
            modifier = Modifier.fillMaxWidth().height(50.dp),
        ) {
            Text("Stattdessen Foto aufnehmen")
        }
        Text(
            "Kein Code am Punkt? Ein Foto tut es auch. Die Zentrale sieht es an " +
                "und entscheidet – solange sie prüft, läuft eure Frist nicht weiter.",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    } else {
        Text(
            "Noch ${(mission.distanceM - mission.rangeM).toInt().coerceAtLeast(0)} m. " +
                "Der Nachweis lässt sich erst am Ziel führen.",
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }

    /*
     * Verzögerung melden.
     *
     * Die Frist misst, ob jemand rechtzeitig losgelaufen ist. Sie misst etwas
     * anderes, sobald der Auftrag am Ziel selbst Zeit kostet – zwei Tüten
     * Gummibärchen dauern eine Minute oder zwölf, je nachdem, wie viele Leute
     * vor der Kasse stehen. Wer dort ansteht, hat alles richtig gemacht.
     */
    delay?.let { StateNote(good = it.grantedMin > 0, text = it.message) }

    if (delayOpen) {
        OutlinedTextField(
            value = delayReason,
            onValueChange = { delayReason = it.take(120) },
            label = { Text("Was hält euch auf?") },
            placeholder = { Text("z. B. Schlange an der Kasse") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedButton(
                onClick = { delayOpen = false },
                modifier = Modifier.weight(1f).height(48.dp),
            ) { Text("Abbrechen") }
            Button(
                onClick = { onDelay(delayReason); delayReason = ""; delayOpen = false },
                enabled = !busy,
                modifier = Modifier.weight(1f).height(48.dp),
            ) { Text("Melden") }
        }
    } else {
        OutlinedButton(
            onClick = { delayOpen = true },
            enabled = !busy,
            modifier = Modifier.fillMaxWidth().height(48.dp),
        ) {
            val left = mission.grace?.leftMin ?: 0
            Text(
                if (left > 0) "Ich brauche länger (noch $left Min. Kulanz)"
                else "Ich brauche länger — Zentrale fragen"
            )
        }
    }
}

/**
 * Zustandsmeldung mit Randstreifen statt Farbfläche.
 *
 * Dieselbe Regel wie überall: Zustandsfarben tragen Information und stehen
 * deshalb außerhalb des dynamischen Farbschemas.
 */
@Composable
private fun StateNote(good: Boolean, text: String) {
    Row(
        Modifier
            .fillMaxWidth()
            .background(MaterialTheme.colorScheme.surfaceVariant, MaterialTheme.shapes.small),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(
            Modifier
                .width(3.dp)
                .heightIn(min = 42.dp)
                .background(if (good) PartyColors.ok else PartyColors.warn)
        )
        Text(
            text,
            Modifier.padding(10.dp),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun Stat(label: String, value: String, tint: androidx.compose.ui.graphics.Color?) {
    Column {
        Text(
            label,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Text(
            value,
            style = MonoStyle.merge(MaterialTheme.typography.titleLarge),
            color = tint ?: MaterialTheme.colorScheme.onSurface,
        )
    }
}

private fun formatMissionClock(seconds: Int): String {
    val s = seconds.coerceAtLeast(0)
    return "${s / 60}:${(s % 60).toString().padStart(2, '0')}"
}
