package de.operationx.app.ui

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import de.operationx.app.data.RuleGroup
import de.operationx.app.ui.theme.MonoStyle

/**
 * Der Regelnachschlag.
 *
 * Die häufigste Frage draußen ist, was etwas kostet. Sie kommt im Gehen, mit
 * einer Hand am Riemen, und sie soll niemanden zwingen, den Funk zu belegen
 * oder in der Einweisung zurückzublättern.
 *
 * Deshalb eine Suche statt einer Gliederung: Wer wissen will, was die
 * Nebelkerze kostet, tippt "nebel" und ist fertig. Die Gruppen darunter sind
 * für die, die stöbern wollen.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun RuleSheet(groups: List<RuleGroup>, onClose: () -> Unit) {
    var query by remember { mutableStateOf("") }

    val filtered = remember(groups, query) {
        val q = query.trim().lowercase()
        if (q.isEmpty()) groups
        else groups.mapNotNull { g ->
            val hits = g.rules.filter {
                it.name.lowercase().contains(q) || it.what.lowercase().contains(q)
            }
            if (hits.isEmpty()) null else g.copy(rules = hits)
        }
    }

    ModalBottomSheet(onDismissRequest = onClose) {
        Column(
            Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp)
                .padding(bottom = 16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Text("Was gilt", style = MaterialTheme.typography.headlineSmall)

            OutlinedTextField(
                value = query,
                onValueChange = { query = it },
                label = { Text("Suchen") },
                placeholder = { Text("Nebelkerze, Sperre, Meldung …") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )

            if (groups.isEmpty()) {
                Text(
                    "Die Regelwerte ließen sich nicht laden.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            } else if (filtered.isEmpty()) {
                Text(
                    "Nichts gefunden. Andere Wörter versuchen.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }

            LazyColumn(
                modifier = Modifier.heightIn(max = 460.dp),
                verticalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                filtered.forEach { group ->
                    item(key = "kopf-${group.group}") {
                        Text(
                            group.group.uppercase(),
                            style = MaterialTheme.typography.labelMedium,
                            color = MaterialTheme.colorScheme.primary,
                            modifier = Modifier.padding(top = 8.dp, bottom = 2.dp),
                        )
                    }
                    items(group.rules, key = { it.key }) { rule ->
                        Surface(
                            color = MaterialTheme.colorScheme.surfaceVariant,
                            shape = MaterialTheme.shapes.small,
                            modifier = Modifier.fillMaxWidth(),
                        ) {
                            Column(Modifier.padding(10.dp)) {
                                Row(
                                    Modifier.fillMaxWidth(),
                                    horizontalArrangement = Arrangement.SpaceBetween,
                                    verticalAlignment = Alignment.CenterVertically,
                                ) {
                                    Text(
                                        rule.name,
                                        style = MaterialTheme.typography.bodyMedium,
                                        fontWeight = FontWeight.Medium,
                                        modifier = Modifier.weight(1f),
                                    )
                                    Text(
                                        "${rule.value} ${unitLabel(rule.unit)}".trim(),
                                        style = MonoStyle.merge(MaterialTheme.typography.bodyMedium),
                                    )
                                }
                                Text(
                                    rule.what,
                                    style = MaterialTheme.typography.bodySmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}

private fun unitLabel(unit: String) = when (unit) {
    "min" -> "Min."
    "sek" -> "Sek."
    "m" -> "m"
    "punkte" -> "Punkte"
    "fp" -> "FP"
    else -> ""
}
