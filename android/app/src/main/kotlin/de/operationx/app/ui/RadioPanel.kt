package de.operationx.app.ui

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import de.operationx.app.data.RadioMessage
import de.operationx.app.ui.theme.MonoStyle
import de.operationx.app.ui.theme.PartyColors

/**
 * Der Funkkanal.
 *
 * Nur die Zentrale vergibt Vertrauensstufen. Für die Zielperson ist der Kanal
 * schreibgeschützt – sie sieht ohnehin nur, was ausdrücklich an sie geht.
 */
@Composable
fun RadioPanel(
    messages: List<RadioMessage>,
    canWrite: Boolean,
    onSend: (String) -> Unit,
) {
    var text by remember { mutableStateOf("") }
    val listState = rememberLazyListState()

    LaunchedEffect(messages.size) {
        if (messages.isNotEmpty()) listState.animateScrollToItem(messages.size - 1)
    }

    Column(Modifier.fillMaxSize()) {
        LazyColumn(
            state = listState,
            modifier = Modifier.weight(1f).padding(horizontal = 12.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp),
            contentPadding = PaddingValues(vertical = 12.dp),
        ) {
            if (messages.isEmpty()) {
                item {
                    Text(
                        "Noch keine Meldungen.",
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }

            items(messages, key = { it.id }) { message ->
                val accent = when (message.trust) {
                    "confirmed" -> PartyColors.ok
                    "unconfirmed" -> PartyColors.warn
                    "rumor" -> PartyColors.critical
                    else -> MaterialTheme.colorScheme.outline
                }

                Card(Modifier.fillMaxWidth()) {
                    Column(Modifier.padding(10.dp)) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Text(
                                message.author,
                                style = MaterialTheme.typography.labelLarge,
                                color = if (message.mine) PartyColors.detective else MaterialTheme.colorScheme.onSurface,
                            )
                            if (message.trust.isNotBlank()) {
                                Spacer(Modifier.width(8.dp))
                                Text(
                                    when (message.trust) {
                                        "confirmed" -> "bestätigt"
                                        "unconfirmed" -> "unbestätigt"
                                        else -> "Gerücht"
                                    },
                                    style = MaterialTheme.typography.bodySmall,
                                    color = accent,
                                )
                            }
                            Spacer(Modifier.weight(1f))
                            Text(
                                shortTime(message.at),
                                style = MonoStyle.merge(MaterialTheme.typography.bodySmall),
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                        Text(message.text, style = MaterialTheme.typography.bodyMedium)
                    }
                }
            }
        }

        if (canWrite) {
            Row(
                Modifier.fillMaxWidth().padding(12.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                OutlinedTextField(
                    value = text,
                    onValueChange = { text = it },
                    placeholder = { Text("Nachricht") },
                    singleLine = true,
                    modifier = Modifier.weight(1f),
                )
                Spacer(Modifier.width(8.dp))
                Button(
                    onClick = { onSend(text); text = "" },
                    enabled = text.isNotBlank(),
                    modifier = Modifier.height(52.dp),
                ) {
                    Text("Senden")
                }
            }
        } else {
            Text(
                "Nur mitlesen – der Kanal gehört der Fahndung.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(12.dp),
            )
        }
    }
}

/** Aus "2026-09-09T14:30:12Z" wird "14:30". */
private fun shortTime(iso: String): String =
    runCatching { iso.substring(11, 16) }.getOrElse { "" }
