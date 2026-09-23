package de.operationx.app.ui

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import de.operationx.app.UiState
import de.operationx.app.ui.theme.PartyColors

/** Anmeldung mit Rufzeichen – Teams haben keine E-Mail-Adressen. */
@Composable
fun LoginScreen(
    state: UiState,
    onLogin: (String, String) -> Unit,
    onBack: () -> Unit,
) {
    var callsign by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            // Die Tastatur schiebt den Inhalt hoch, statt ihn zu verdecken.
            .imePadding()
            .padding(24.dp),
        verticalArrangement = Arrangement.Center,
    ) {
        Text("Anmelden", style = MaterialTheme.typography.headlineMedium)

        if (state.serverName.isNotBlank()) {
            Spacer(Modifier.height(4.dp))
            Text(
                state.serverName,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }

        // Das Kennzeichen steht hier und nicht erst im Spiel: Vergleichen
        // lässt es sich nur, bevor jemand sein Kennwort eintippt.
        state.fingerprint?.let { kennzeichen ->
            Spacer(Modifier.height(8.dp))
            Text(
                "\uD83D\uDD12 $kennzeichen",
                style = MaterialTheme.typography.bodyMedium,
                color = PartyColors.ok,
            )
            Text(
                "Dieselben Zeichen wie auf der Teamkarte? Dann sitzt niemand dazwischen.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }

        state.kennzeichenWarnung?.let { warnung ->
            Spacer(Modifier.height(12.dp))
            Card(
                colors = CardDefaults.cardColors(
                    containerColor = MaterialTheme.colorScheme.errorContainer,
                )
            ) {
                Text(
                    warnung,
                    Modifier.padding(14.dp),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onErrorContainer,
                )
            }
        }

        Spacer(Modifier.height(24.dp))

        OutlinedTextField(
            value = callsign,
            onValueChange = { callsign = it },
            label = { Text("Rufzeichen") },
            placeholder = { Text("Team_Alpha") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )

        Spacer(Modifier.height(12.dp))

        OutlinedTextField(
            value = password,
            onValueChange = { password = it },
            label = { Text("Kennwort") },
            singleLine = true,
            visualTransformation = PasswordVisualTransformation(),
            // Als Kennwortfeld ausgewiesen, damit die Tastatur weder
            // verbessert noch vorschlägt: Die Kennwörter der Teamkarten sind
            // deutsche Wörter, und eine Autokorrektur, die aus "schatten"
            // etwas anderes macht, sperrt jemanden am Spieltag aus.
            keyboardOptions = KeyboardOptions(
                keyboardType = KeyboardType.Password,
                imeAction = ImeAction.Go,
            ),
            modifier = Modifier.fillMaxWidth(),
        )

        Spacer(Modifier.height(20.dp))

        Button(
            onClick = { onLogin(callsign, password) },
            enabled = !state.busy && callsign.isNotBlank() && password.isNotBlank(),
            modifier = Modifier
                .fillMaxWidth()
                .height(52.dp),
        ) {
            Text(if (state.busy) "Prüfe Zugang …" else "Anmelden")
        }

        state.error?.let {
            Spacer(Modifier.height(16.dp))
            Text(it, color = MaterialTheme.colorScheme.error)
        }

        Spacer(Modifier.height(12.dp))
        TextButton(onClick = onBack) { Text("Anderen Server wählen") }
    }
}
