package de.operationx.app.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.unit.dp
import de.operationx.app.ui.theme.PartyColors

/**
 * Die Einweisung.
 *
 * Sie gab es bisher nur in der Weboberfläche; in der App stand der Aufruf da,
 * der ein Team als eingewiesen meldet, und wurde von keiner Stelle benutzt. In
 * der Bereitschaftsliste der Zentrale blieb damit jeder App-Nutzer dauerhaft
 * auf "offen" – und die Liste ist genau dafür da, vor dem Start zu sehen, wer
 * noch fehlt.
 *
 * Der Text ist derselbe wie im Browser, aber die Handlungsprobe ist eine
 * andere: Im Browser wird einmal der Standort gemeldet. Hier muss zusätzlich
 * die Erfassung im Hintergrund anlaufen, denn genau die ist der Grund, warum
 * jemand die App überhaupt installiert hat. Wer das hier nicht erlaubt, merkt
 * es sonst erst, wenn die erste Strafe gebucht ist.
 */
@Composable
fun OnboardingScreen(
    role: String,
    trackingOn: Boolean,
    onStartTracking: () -> Unit,
    onDone: () -> Unit,
) {
    val steps = remember(role) { scriptFor(role) + datenschutzSeite }
    var step by remember { mutableIntStateOf(0) }
    val atProbe = step >= steps.size

    Surface(Modifier.fillMaxSize()) {
        Column(
            Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(20.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp),
        ) {
            Spacer(Modifier.height(8.dp))

            Text(
                "EINWEISUNG",
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.primary,
            )
            Text(
                "${minOf(step + 1, steps.size)} von ${steps.size}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )

            // Fortschritt als Balkenreihe: Auf einem kleinen Bildschirm sagt
            // sie mehr als eine Zahl, ohne Platz zu kosten.
            Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                repeat(steps.size) { i ->
                    Box(
                        Modifier
                            .weight(1f)
                            .height(3.dp)
                            .clip(RoundedCornerShape(2.dp))
                            .background(
                                if (i <= step) MaterialTheme.colorScheme.primary
                                else MaterialTheme.colorScheme.surfaceVariant
                            )
                    )
                }
            }

            if (!atProbe) {
                Text(steps[step].first, style = MaterialTheme.typography.headlineSmall)
                Text(
                    steps[step].second,
                    style = MaterialTheme.typography.bodyLarge,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )

                Spacer(Modifier.weight(1f))

                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    if (step > 0) {
                        OutlinedButton(
                            onClick = { step-- },
                            modifier = Modifier.weight(1f).height(52.dp),
                        ) { Text("Zurück") }
                    }
                    Button(
                        onClick = { step++ },
                        modifier = Modifier.weight(1f).height(52.dp),
                    ) {
                        Text(if (step == steps.lastIndex) "Zur Probe" else "Weiter")
                    }
                }
            } else {
                Text("Handlungsprobe", style = MaterialTheme.typography.headlineSmall)

                if (role == "hq") {
                    Text(
                        "Damit seid ihr durch. Die Zentrale leitet ihr am besten " +
                            "am Rechner – dort ist Platz für Karte und Protokoll " +
                            "nebeneinander.",
                        style = MaterialTheme.typography.bodyLarge,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                } else {
                    Text(
                        "Einmal ernsthaft: Schaltet die Standorterfassung ein. " +
                            "Sie ist der Grund für diese App – sie meldet auch " +
                            "dann weiter, wenn der Bildschirm aus ist und das " +
                            "Telefon in der Tasche steckt.",
                        style = MaterialTheme.typography.bodyLarge,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )

                    if (trackingOn) {
                        StateNote(good = true, text = "Die Erfassung läuft. Ihr seid bereit.")
                    } else {
                        Button(
                            onClick = onStartTracking,
                            modifier = Modifier.fillMaxWidth().height(52.dp),
                        ) { Text("Erfassung einschalten") }
                        // Android fragt in zwei Schritten, und der zweite führt
                        // seit Android 11 zwingend über die Einstellungen. Wer
                        // das nicht weiß, hält den ersten für den einzigen und
                        // wundert sich später über die Strafen.
                        Text(
                            "Android fragt zweimal: zuerst „Bei Nutzung der App“ " +
                                "antippen. Danach kommt die Frage nach dem " +
                                "Hintergrund – dort führt der Weg über die " +
                                "Einstellungen zu „Immer zulassen“. Ohne den " +
                                "zweiten Schritt hört die Meldung auf, sobald der " +
                                "Bildschirm aus ist.",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }

                // Die Einweisung nennt konkrete Zahlen, weil abstrakte Regeln
                // niemand behält. Änderbar sind sie trotzdem alle: Die Zentrale
                // stellt sie im Regelpult ein. Wer das nicht weiß, hält die
                // Zahl von heute Morgen für die Wahrheit des Tages.
                Text(
                    "Die genannten Zahlen sind die Voreinstellung. Was in eurem Spiel " +
                        "gilt, steht jederzeit im Menü unter „Regeln“.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )

                Spacer(Modifier.height(8.dp))

                Row(
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    OutlinedButton(
                        onClick = { step-- },
                        modifier = Modifier.weight(1f).height(52.dp),
                    ) { Text("Zurück") }
                    Button(
                        onClick = onDone,
                        modifier = Modifier.weight(1f).height(52.dp),
                    ) { Text(if (trackingOn || role == "hq") "Los geht's" else "Trotzdem weiter") }
                }
            }

            Spacer(Modifier.height(16.dp))
        }
    }
}

/**
 * Zustandsmeldung mit Randstreifen statt Farbfläche.
 *
 * Die Zustandsfarben des Regelwerks stehen bewusst außerhalb des dynamischen
 * Farbschemas: Sie tragen Information. Als Fläche würden sie mit der
 * Parteifarbe konkurrieren, deshalb nur als Streifen am Rand — dieselbe Regel
 * wie in der Weboberfläche.
 */
@Composable
private fun StateNote(good: Boolean, text: String) {
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
                .height(46.dp)
                .background(accent)
        )
        Text(
            text,
            Modifier.padding(12.dp),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/**
 * Die Einweisungstexte.
 *
 * Wortgleich mit der Weboberfläche, wo es um dieselbe Sache geht – wer im
 * Browser eingewiesen wurde und dann die App installiert, soll nicht zwei
 * verschiedene Regeln gelesen haben. Wo die App etwas anderes kann, steht
 * hier etwas anderes.
 */
/**
 * Dieselbe Seite für jede Rolle, und zwar bevor die Erfassung anläuft.
 *
 * Wer Standortdaten von Leuten verarbeitet, sagt ihnen vorher, was erhoben
 * wird und wie lange es bleibt. Wortgleich in der Weboberfläche und in der
 * iOS-Fassung.
 */
private val datenschutzSeite = listOf(
    "Was dieses Spiel über euch speichert" to
        "Euren Standort, solange die Erfassung läuft — im Spiel etwa alle " +
        "zehn Minuten einer. Dazu Punkte, Buchungen und was ihr in den Funk " +
        "schreibt. Alles hängt an diesem einen Spiel und liegt auf dem " +
        "Rechner der Spielleitung, nicht bei uns; wir bekommen nichts davon " +
        "zu sehen. Nach dem Spiel wird die Bewegungsspur automatisch gelöscht " +
        "— ab Werk 24 Stunden nach Spielende. Was bleibt, ist der Punktestand " +
        "ohne Koordinaten. Wer damit nicht einverstanden ist, spielt nicht " +
        "mit; das ist in Ordnung und hat keine Folgen.",
)

private fun scriptFor(role: String): List<Pair<String, String>> = when (role) {
    "misterx" -> listOf(
        "Ihr seid die Zielperson" to
            "Ihr bewegt euch unerkannt durch die Stadt, arbeitet Zwischenziele ab " +
            "und wollt am Ende das geheime Fluchtziel erreichen. Die Fahndung " +
            "erscheint als unscharfe Kreise – rund 200 Meter Unschärfe.",
        "Standort melden, alle zehn Minuten" to
            "Auch ihr meldet euch regelmäßig, sonst gibt es Abzüge. Solange die " +
            "Erfassung läuft, macht die App das von selbst – auch mit gesperrtem " +
            "Bildschirm. In Bahn oder Bus schaltet ihr auf Transit, dann sind es " +
            "dreizehn Minuten.",
        "Drei Wege zu jedem Ziel" to
            "An jedem Zwischenziel stehen drei Varianten zur Wahl: unauffällig mit " +
            "viel Zeit, schnell mit knapper Frist, oder ein Bonusauftrag mit einem " +
            "zusätzlichen Fluchtpunkt. Die schnelle Route führt manchmal direkt an " +
            "der Fahndung vorbei.",
        "Ankunft nachweisen" to
            "Am Ziel gebt ihr den Vor-Ort-Code ein. Näher als 150 Meter müsst ihr " +
            "dafür sein.",
        "Fluchtpunkte sind eure Waffe" to
            "Falsche Fährte, Nebelkerze, Phantom, U-Bahn-Geist. Die Nebelkerze macht " +
            "jede Ortung fünfzehn Minuten lang wirkungslos – auch eine, die schon " +
            "läuft. Was was kostet, steht jederzeit im Menü unter „Regeln“.",
        "Stillstand kostet" to
            "Wer sich zwanzig Minuten weder bewegt noch etwas tut, verliert einen " +
            "Fluchtpunkt. Gezählt wird das nur außerhalb von Mission und Transit – " +
            "solange ein Zwischenziel läuft, misst schon die Frist, ob ihr " +
            "vorankommt. Verstecken ist erlaubt, Aussitzen nicht.",
    )

    "hq" -> listOf(
        "Ihr leitet das Spiel" to
            "Die Zentrale sieht alles: echte Positionen, jede Buchung, welcher " +
            "Hinweis gefälscht ist – und kann eingreifen, wenn die Realität " +
            "dazwischenkommt.",
        "Am Rechner, nicht am Telefon" to
            "Die App ist für das Feld gebaut. Zum Leiten öffnet ihr die " +
            "Weboberfläche am Rechner: Dort liegen Sektoren, Hotspots, Rätsel, " +
            "Druck und das Regelpult nebeneinander.",
        "Vorher einrichten" to
            "Sektoren aus echten Stadtteilgrenzen, Hotspots setzen, Rätsel " +
            "schreiben, drei Blätter drucken und die Zettel anbringen. Ohne die " +
            "Zettel gibt es keine Vor-Ort-Codes.",
    )

    else -> listOf(
        "Ihr seid die Fahndung" to
            "Mehrere Teams arbeiten zusammen gegen eine Zielperson. Ihr seht euch " +
            "gegenseitig immer genau – von der Zielperson seht ihr nur, was ihr " +
            "euch erarbeitet.",
        "Standort melden, alle zehn Minuten" to
            "Solange die Erfassung läuft, macht die App das von selbst – auch mit " +
            "gesperrtem Bildschirm. Läuft die Frist trotzdem ab, gibt es beim " +
            "ersten Mal eine Verwarnung, beim zweiten zehn Minuspunkte, ab dem " +
            "dritten zusätzlich fünf Minuten Sperre.",
        "Rätsel bringen Hinweise" to
            "Jedes gelöste Rätsel erzeugt einen Hinweis aus der Lage in genau " +
            "diesem Moment. Wer schnell löst, bekommt eine heiße Spur; wer lange " +
            "braucht, eine kalte.",
        "Nicht alles stimmt" to
            "Die Zielperson kann falsche Hinweise einspeisen. Sie sehen genauso aus " +
            "wie echte. Zwei Hinweise, die sich widersprechen, sind kein Fehler – " +
            "sondern eine Information.",
        "Der Zugriff, drei Stufen" to
            "Ihr müsst an dem Punkt stehen, den ihr benennt. Stufe 1 und 2 kosten " +
            "bei einem Fehlschlag nichts – nutzt sie, um eine Vermutung zu prüfen. " +
            "Stufe 3 entscheidet das Spiel oder kostet zehn Minuten Sperre, einen " +
            "Fluchtpunkt und fünfzehn Minuspunkte.",
        "Stillstand kostet" to
            "Wer fünf Minuten weder läuft noch etwas tut, verliert einen " +
            "Fahndungspunkt. An einem Ort auf die Zielperson zu warten, ist keine " +
            "Fahndung.",
    )
}
