package de.operationx.app.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.ui.unit.dp
import de.operationx.app.data.ArrestResult
import de.operationx.app.data.SightingResult
import de.operationx.app.location.isoNow
import de.operationx.app.ui.theme.PartyColors
import java.util.Calendar

/**
 * Sichtkontakt und Zugriff.
 *
 * Der gefährlichste Knopf des Spiels sitzt hier. Stufe 1 und 2 kosten bei einem
 * Fehlschlag nichts – sie sind das Werkzeug, mit dem sich eine Vermutung
 * gefahrlos prüfen lässt. Stufe 3 entscheidet die Partie oder kostet Sperre,
 * Fluchtpunkt und fünfzehn Punkte.
 *
 * Deshalb ist die dritte Stufe hier nicht einfach die dritte Auswahl: Sie
 * verlangt alle drei Angaben, nennt ihren Preis daneben, und der Knopf trägt
 * die Warnfarbe. Auf einem Telefon, das jemand im Gehen bedient, ist das der
 * einzige Schutz vor einem versehentlichen Tippen.
 */
/**
 * Die Zahlen, die im Zugriffsfenster genannt werden.
 *
 * Sie kommen aus dem Regelpult der Zentrale und nicht aus dem Quelltext: Wer
 * hier abwägt, ob eine Vermutung fünfzehn oder dreißig Punkte wert ist, muss
 * den Preis lesen, der heute gilt.
 */
data class ZugriffsRegeln(
    val abzug: Int = -15,
    val sperreMin: Int = 10,
    val nachweisM: Int = 150,
    val verweilSek: Int = 20,
)

@Composable
fun ArrestPanel(
    hotspotNumbers: List<Int>,
    sighting: SightingResult?,
    arrest: ArrestResult?,
    busy: Boolean,
    regeln: ZugriffsRegeln,
    onSighting: () -> Unit,
    onArrest: (Int, Int, String?, Int?) -> Unit,
) {
    var level by remember { mutableIntStateOf(1) }
    var hotspot by remember { mutableStateOf("") }
    var hour by remember { mutableStateOf("") }
    var minute by remember { mutableStateOf("") }
    var target by remember { mutableStateOf("") }
    var confirmOpen by remember { mutableStateOf(false) }

    val hotspotOk = hotspot.toIntOrNull() != null
    val timeOk = hour.toIntOrNull() in 0..23 && minute.toIntOrNull() in 0..59
    val targetOk = target.toIntOrNull() != null

    val ready = when (level) {
        1 -> hotspotOk
        2 -> hotspotOk && timeOk
        else -> hotspotOk && timeOk && targetOk
    }

    fun timeIso(): String? {
        if (!timeOk) return null

        val cal = Calendar.getInstance()
        cal.set(Calendar.HOUR_OF_DAY, hour.toInt())
        cal.set(Calendar.MINUTE, minute.toInt())
        cal.set(Calendar.SECOND, 0)
        cal.set(Calendar.MILLISECOND, 0)

        // Über Mitternacht hinweg gehört die Uhrzeit zum Vortag.
        //
        // Eine Feier beginnt um acht und läuft bis eins. Wer um 00:10 einen
        // Zugriff auf "23:50" stützt, meint den Abend davor – gerechnet wurde
        // aber der heutige 23:50, also ein Zeitpunkt, der noch gar nicht
        // eingetreten ist. Der Zugriff konnte damit nur scheitern, und niemand
        // hätte erkannt, warum.
        if (cal.timeInMillis > System.currentTimeMillis() + 60_000) {
            cal.add(Calendar.DAY_OF_MONTH, -1)
        }

        return isoNow(cal.timeInMillis)
    }

    fun fire() {
        onArrest(
            level,
            hotspot.toInt(),
            if (level >= 2) timeIso() else null,
            if (level >= 3) target.toIntOrNull() else null,
        )
    }

    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Button(
            onClick = onSighting,
            enabled = !busy,
            modifier = Modifier.fillMaxWidth().height(52.dp),
            colors = ButtonDefaults.buttonColors(containerColor = PartyColors.detective),
        ) {
            Text("Sichtkontakt melden")
        }
        Text(
            "Der Kontakt gilt erst nach ${regeln.verweilSek} Sekunden in Sichtweite. " +
                "Die Zielperson wird sofort gewarnt – auch wenn es nichts wird.",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )

        sighting?.let { ResultCard(it.confirmed, it.message.ifBlank { "Gemeldet." }) }

        HorizontalDivider(Modifier.padding(vertical = 4.dp))

        Text("Zugriff", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)

        SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
            listOf("Ort", "Ort + Zeit", "Alles").forEachIndexed { index, label ->
                SegmentedButton(
                    selected = level == index + 1,
                    onClick = { level = index + 1 },
                    shape = SegmentedButtonDefaults.itemShape(index, 3),
                ) { Text(label) }
            }
        }

        Text(
            when (level) {
                1 -> "Lokalisierung: Ist die Zielperson jetzt hier? " +
                    "Ein Fehlschlag kostet nichts."
                2 -> "Rekonstruktion: War sie außerdem zur genannten Zeit hier? " +
                    "Ein Fehlschlag kostet nichts."
                else -> "Vollständiger Zugriff: Ort, Zeit und Fluchtziel zusammen. " +
                    "Stimmt alles, ist das Spiel gewonnen. Stimmt es nicht, kostet es " +
                    "${regeln.sperreMin} Minuten Sperre, einen Fluchtpunkt und " +
                    "${-regeln.abzug} Punkte."
            },
            style = MaterialTheme.typography.bodySmall,
            color = if (level >= 3) MaterialTheme.colorScheme.error
            else MaterialTheme.colorScheme.onSurfaceVariant,
        )

        OutlinedTextField(
            value = hotspot,
            onValueChange = { hotspot = it.filter(Char::isDigit).take(3) },
            label = { Text("Nummer des Punkts, an dem ihr steht") },
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        // Das ist die Bedingung, an der die meisten Versuche scheitern werden,
        // und sie steht nirgends sonst: Ein Zugriff ist eine Handlung im Feld.
        Text(
            "Ihr müsst an diesem Punkt stehen – höchstens ${regeln.nachweisM} Meter entfernt.",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )

        if (level >= 2) {
            Row(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                OutlinedTextField(
                    value = hour,
                    onValueChange = { hour = it.filter(Char::isDigit).take(2) },
                    label = { Text("Stunde") },
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    singleLine = true,
                    modifier = Modifier.weight(1f),
                )
                OutlinedTextField(
                    value = minute,
                    onValueChange = { minute = it.filter(Char::isDigit).take(2) },
                    label = { Text("Minute") },
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    singleLine = true,
                    modifier = Modifier.weight(1f),
                )
            }
        }

        if (level >= 3) {
            OutlinedTextField(
                value = target,
                onValueChange = { target = it.filter(Char::isDigit).take(3) },
                label = { Text("Fluchtziel: Hotspot-Nummer") },
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
        }

        Button(
            onClick = { if (level >= 3) confirmOpen = true else fire() },
            enabled = ready && !busy,
            modifier = Modifier.fillMaxWidth().height(52.dp),
            colors = if (level >= 3) {
                ButtonDefaults.buttonColors(containerColor = MaterialTheme.colorScheme.error)
            } else {
                ButtonDefaults.buttonColors()
            },
        ) {
            Text(if (level >= 3) "Zugriff Stufe 3" else "Prüfen")
        }

        arrest?.let {
            ResultCard(
                it.correct,
                buildString {
                    append(it.message.ifBlank { if (it.correct) "Trifft zu." else "Trifft nicht zu." })
                    if (it.points != 0) append("  (${if (it.points > 0) "+" else ""}${it.points} Punkte)")
                },
            )
        }

        Spacer(Modifier.height(8.dp))
    }

    // Die dritte Stufe bekommt eine Rückfrage. Nicht aus Vorsicht um der
    // Vorsicht willen, sondern weil ein Fehltipper hier das Spiel kostet.
    if (confirmOpen) {
        AlertDialog(
            onDismissRequest = { confirmOpen = false },
            title = { Text("Zugriff wirklich auslösen?") },
            text = {
                Text(
                    "Punkt $hotspot, $hour:$minute Uhr, Fluchtziel $target.\n\n" +
                        "Stimmt das nicht, folgen ${regeln.sperreMin} Minuten Sperre, " +
                        "ein Punkt Abzug beim Guthaben und ${-regeln.abzug} Minuspunkte.",
                )
            },
            confirmButton = {
                TextButton(onClick = { confirmOpen = false; fire() }) {
                    Text("Zugriff", color = MaterialTheme.colorScheme.error)
                }
            },
            dismissButton = {
                TextButton(onClick = { confirmOpen = false }) { Text("Abbrechen") }
            },
        )
    }
}

/**
 * Ergebnis mit Randstreifen statt Farbfläche.
 *
 * "Trifft zu" und "trifft nicht zu" müssen sich auf einen Blick unterscheiden,
 * und zwar unabhängig davon, welches Hintergrundbild jemand eingestellt hat.
 * Das dynamische Farbschema kann primaryContainer alarmrot ausfallen lassen –
 * dann läse sich ein Erfolg wie ein Fehler. Die Zustandsfarben des Regelwerks
 * liegen deshalb außerhalb des Schemas.
 */
@Composable
private fun ResultCard(good: Boolean, text: String) {
    val accent = if (good) PartyColors.ok else PartyColors.critical
    Row(
        Modifier
            .fillMaxWidth()
            .background(MaterialTheme.colorScheme.surfaceVariant, MaterialTheme.shapes.small),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(
            Modifier
                .width(3.dp)
                .heightIn(min = 46.dp)
                .background(accent)
        )
        Text(
            text,
            modifier = Modifier.padding(12.dp),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}
