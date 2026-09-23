package de.operationx.app.ui

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import de.operationx.app.data.ArrestResult
import de.operationx.app.data.Puzzle
import de.operationx.app.data.SightingResult
import de.operationx.app.ui.theme.MonoStyle
import de.operationx.app.ui.theme.PartyColors

/**
 * Rätselmodul und Zugriff.
 *
 * Der gefährlichste Knopf im Spiel sitzt hier unten: Stufe 3 entscheidet die
 * Partie oder kostet Sperre, Fluchtpunkt und fünfzehn Punkte. Die Folgen stehen
 * daneben, nicht im Kleingedruckten.
 */
@Composable
fun PuzzlePanel(
    puzzles: List<Puzzle>,
    hotspotNumbers: List<Int>,
    sighting: SightingResult?,
    arrest: ArrestResult?,
    busy: Boolean,
    regeln: ZugriffsRegeln,
    onSolve: (String, String) -> Unit,
    onSighting: () -> Unit,
    onArrest: (Int, Int, String?, Int?) -> Unit,
) {
    var openId by remember { mutableStateOf<String?>(null) }
    var answer by remember { mutableStateOf("") }

    Column(
        Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(12.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        ArrestPanel(
            regeln = regeln,
            hotspotNumbers = hotspotNumbers,
            sighting = sighting,
            arrest = arrest,
            busy = busy,
            onSighting = onSighting,
            onArrest = onArrest,
        )

        HorizontalDivider(Modifier.padding(vertical = 4.dp))

        Text("Rätsel", style = MaterialTheme.typography.titleMedium)

        if (puzzles.isEmpty()) {
            Text(
                "Noch keine Rätsel freigeschaltet.",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }

        puzzles.forEach { puzzle ->
            Card(Modifier.fillMaxWidth()) {
                Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    Row(verticalAlignment = androidx.compose.ui.Alignment.CenterVertically) {
                        Text(
                            puzzle.code,
                            style = MonoStyle.merge(MaterialTheme.typography.bodySmall),
                            color = PartyColors.detective,
                        )
                        Spacer(Modifier.width(8.dp))
                        Text(
                            puzzle.title,
                            style = MaterialTheme.typography.titleMedium,
                            modifier = Modifier.weight(1f),
                        )
                        if (puzzle.solved) {
                            Text(
                                "gelöst",
                                style = MaterialTheme.typography.bodySmall,
                                color = PartyColors.ok,
                            )
                        } else {
                            Text(
                                "${puzzle.points} Pkt",
                                style = MonoStyle.merge(MaterialTheme.typography.bodySmall),
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }

                    if (!puzzle.solved) {
                        TextButton(onClick = {
                            openId = if (openId == puzzle.id) null else puzzle.id
                            answer = ""
                        }) {
                            Text(if (openId == puzzle.id) "Zuklappen" else "Aufgabe zeigen")
                        }

                        if (openId == puzzle.id) {
                            Text(puzzle.question, style = MaterialTheme.typography.bodyMedium)

                            if (puzzle.hint.isNotBlank()) {
                                Text(
                                    "Denkanstoß: ${puzzle.hint}",
                                    style = MaterialTheme.typography.bodySmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            }

                            OutlinedTextField(
                                value = answer,
                                onValueChange = { answer = it },
                                label = { Text("Antwort") },
                                singleLine = true,
                                modifier = Modifier.fillMaxWidth(),
                            )
                            Button(
                                onClick = { onSolve(puzzle.id, answer); answer = "" },
                                enabled = answer.isNotBlank(),
                                modifier = Modifier.fillMaxWidth(),
                            ) {
                                Text("Einreichen")
                            }
                        }
                    }
                }
            }
        }
    }
}
