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
import de.operationx.app.data.LedgerBook
import de.operationx.app.data.LedgerEntry
import de.operationx.app.ui.theme.MonoStyle
import de.operationx.app.ui.theme.PartyColors

/**
 * Das Punktekonto.
 *
 * "Warum habe ich minus fünfzehn?" ist die häufigste Frage am Spieltag. Die
 * Kopfleiste zeigte bisher nur eine Zahl; wer ihre Herkunft wissen wollte,
 * musste die Zentrale über Funk fragen, und die musste in ihrer Zeitleiste
 * suchen, während draußen jemand wartet.
 *
 * Deshalb ist die Zahl selbst der Knopf hierher, und deshalb steht neben jeder
 * Buchung der Stand danach: Die Frage lautet fast immer "seit wann", und die
 * beantwortet nur eine Spalte, die man von oben nach unten lesen kann.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AccountSheet(book: LedgerBook?, onClose: () -> Unit) {
    ModalBottomSheet(onDismissRequest = onClose) {
        Column(
            Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp)
                .padding(bottom = 16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Text("Punktekonto", style = MaterialTheme.typography.headlineSmall)

            if (book == null) {
                Text(
                    "Wird geladen …",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                return@Column
            }

            Row(horizontalArrangement = Arrangement.spacedBy(24.dp)) {
                Stand("Punkte", book.points.toString())
                Stand("Fluchtpunkte", book.fp.toString())
            }

            if (book.entries.isEmpty()) {
                Text(
                    "Noch keine Buchung. Punkte gibt es für erreichte Zwischenziele " +
                        "und gelöste Rätsel, Abzüge für verpasste Fristen.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                return@Column
            }

            HorizontalDivider()

            LazyColumn(
                modifier = Modifier.heightIn(max = 420.dp),
                verticalArrangement = Arrangement.spacedBy(2.dp),
            ) {
                items(book.entries, key = { it.id }) { BuchungsZeile(it) }
            }
        }
    }
}

@Composable
private fun Stand(label: String, value: String) {
    Column {
        Text(
            label,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Text(value, style = MonoStyle.merge(MaterialTheme.typography.titleLarge))
    }
}

@Composable
private fun BuchungsZeile(e: LedgerEntry) {
    Column(Modifier.fillMaxWidth().padding(vertical = 6.dp)) {
        Row(verticalAlignment = Alignment.Top) {
            Text(
                e.reason.ifBlank { "Ohne Angabe" },
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.weight(1f),
            )
            Spacer(Modifier.width(10.dp))
            Column(horizontalAlignment = Alignment.End) {
                if (e.deltaPoints != 0) Delta(e.deltaPoints, "Pkt")
                if (e.deltaFp != 0) Delta(e.deltaFp, "FP")
            }
        }
        Text(
            "${uhrzeitLokal(e.occurredAt)} · danach ${e.points} Pkt, ${e.fp} FP",
            style = MonoStyle.merge(MaterialTheme.typography.bodySmall),
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/**
 * Plus und Minus tragen hier Bedeutung und stehen deshalb außerhalb des
 * dynamischen Farbschemas – dieselbe Regel wie bei Zugriff und Nachweis.
 */
@Composable
private fun Delta(wert: Int, einheit: String) {
    Text(
        (if (wert > 0) "+$wert" else "$wert") + " " + einheit,
        style = MonoStyle.merge(MaterialTheme.typography.bodyMedium),
        fontWeight = FontWeight.Medium,
        color = if (wert < 0) PartyColors.critical else PartyColors.ok,
    )
}
