package de.operationx.app.ui

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import de.operationx.app.UiState

/**
 * Beitritt.
 *
 * Am Spieltag scannt man den QR-Code der Spielleitung. Wer keinen scannen kann
 * oder will, tippt die Adresse ein – deshalb ist das Feld nicht versteckt,
 * sondern gleichrangig.
 */
@Composable
fun JoinScreen(state: UiState, onCheck: (String) -> Unit, onScan: () -> Unit) {
    var url by remember { mutableStateOf(state.serverUrl) }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            // Die Tastatur schiebt den Inhalt hoch, statt ihn zu verdecken.
            .imePadding()
            .padding(24.dp),
        verticalArrangement = Arrangement.Center,
    ) {
        Text("Operation X", style = MaterialTheme.typography.displaySmall)
        Spacer(Modifier.height(8.dp))
        Text(
            "Verbinde dich mit dem Einsatzserver. Die Adresse steht auf der Teamkarte, " +
                "oder du scannst den QR-Code der Spielleitung.",
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )

        Spacer(Modifier.height(28.dp))

        OutlinedTextField(
            value = url,
            onValueChange = { url = it },
            label = { Text("Serveradresse") },
            placeholder = { Text("beispiel.trycloudflare.com") },
            singleLine = true,
            keyboardOptions = KeyboardOptions(
                keyboardType = KeyboardType.Uri,
                imeAction = ImeAction.Go,
            ),
            modifier = Modifier.fillMaxWidth(),
        )

        Spacer(Modifier.height(16.dp))

        Button(
            onClick = { onCheck(url) },
            enabled = !state.busy && url.isNotBlank(),
            modifier = Modifier
                .fillMaxWidth()
                .height(52.dp),
        ) {
            Text(if (state.busy) "Verbinde …" else "Verbinden")
        }

        Spacer(Modifier.height(8.dp))

        OutlinedButton(
            onClick = onScan,
            modifier = Modifier
                .fillMaxWidth()
                .height(52.dp),
        ) {
            Text("QR-Code scannen")
        }

        state.error?.let {
            Spacer(Modifier.height(16.dp))
            Text(
                it,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.error,
            )
        }

        Spacer(Modifier.height(28.dp))
        Text(
            "Zugangsdaten und Kartendaten verfallen nach dem Spiel automatisch.",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}
