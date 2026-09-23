package de.operationx.app.ui

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import de.operationx.app.data.Hotspot
import de.operationx.app.data.Joker
import de.operationx.app.data.Sector
import de.operationx.app.ui.theme.MonoStyle
import de.operationx.app.ui.theme.PartyColors

/**
 * Die Einsatzmittel.
 *
 * Jede Karte nennt Preis, Wirkung und – wenn sie nicht geht – den Grund dafür.
 * Sperrzone, Wanze und Scan brauchen ein Ziel; das wird erst abgefragt, wenn
 * die Karte angetippt wurde, damit die Liste nicht von Auswahlfeldern zerfasert.
 */
@Composable
fun JokerPanel(
    jokers: List<Joker>,
    fp: Int,
    sectors: List<Sector>,
    hotspots: List<Hotspot>,
    onUse: (String, String?, String?) -> Unit,
) {
    var picking by remember { mutableStateOf<String?>(null) }

    Column(
        Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(12.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text("Einsatzmittel", style = MaterialTheme.typography.titleLarge, modifier = Modifier.weight(1f))
            Text(
                "$fp FP",
                style = MonoStyle.merge(MaterialTheme.typography.titleMedium),
                color = PartyColors.hq,
            )
        }

        if (jokers.isEmpty()) {
            Text(
                "Noch keine Einsatzmittel verfügbar.",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }

        jokers.forEach { joker ->
            val needsTarget = joker.kind in setOf("lockdown", "bug", "scan")

            Card(
                onClick = {
                    if (!joker.available) return@Card
                    if (needsTarget) {
                        picking = if (picking == joker.kind) null else joker.kind
                    } else {
                        onUse(joker.kind, null, null)
                    }
                },
                enabled = joker.available,
                modifier = Modifier.fillMaxWidth(),
            ) {
                Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(
                            joker.name,
                            style = MaterialTheme.typography.titleMedium,
                            modifier = Modifier.weight(1f),
                        )
                        Text(
                            "${joker.cost} FP",
                            style = MonoStyle.merge(MaterialTheme.typography.bodyMedium),
                            color = PartyColors.hq,
                        )
                    }

                    Text(
                        joker.description,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )

                    when {
                        joker.activeUntil.isNotBlank() -> Text(
                            "läuft gerade",
                            style = MaterialTheme.typography.bodySmall,
                            color = PartyColors.ok,
                        )
                        !joker.available -> Text(
                            joker.reason,
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.error,
                        )
                        joker.maxUses > 0 -> Text(
                            "noch ${joker.maxUses - joker.used} von ${joker.maxUses}",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }

                    if (picking == joker.kind) {
                        Spacer(Modifier.height(4.dp))
                        if (joker.kind == "bug") {
                            hotspots.take(30).forEach { h ->
                                TextButton(onClick = {
                                    picking = null
                                    onUse(joker.kind, null, h.id)
                                }) {
                                    Text("#${h.number.toString().padStart(2, '0')} ${h.name}")
                                }
                            }
                        } else {
                            sectors.forEach { s ->
                                TextButton(onClick = {
                                    picking = null
                                    onUse(joker.kind, s.id, null)
                                }) {
                                    Text("${s.code} ${s.name}")
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}
