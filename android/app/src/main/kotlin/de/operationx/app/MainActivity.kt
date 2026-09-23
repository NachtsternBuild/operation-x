package de.operationx.app

import android.Manifest
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.provider.Settings
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import de.operationx.app.location.LocationService
import de.operationx.app.ui.FieldScreen
import de.operationx.app.ui.AccountSheet
import de.operationx.app.ui.RuleSheet
import de.operationx.app.ui.JoinScreen
import de.operationx.app.ui.LoginScreen
import de.operationx.app.ui.OnboardingScreen
import de.operationx.app.ui.ScannerScreen
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import de.operationx.app.ui.theme.OperationXTheme

class MainActivity : ComponentActivity() {

    private val model: AppState by viewModels()

    /**
     * Statusleiste und Navigationsleiste ausblenden.
     *
     * Auf einem 720 Pixel hohen Gerät kosten die beiden Leisten zusammen rund
     * ein Zehntel des Bildes – und was sie zeigen (Uhrzeit, Akku, Empfang),
     * steht bei einem Spiel, das stundenlang draußen läuft, ohnehin auf dem
     * Sperrbildschirm. Die Karte kann den Platz besser gebrauchen.
     *
     * BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE statt vollständigem Ausblenden:
     * Ein Wisch vom Rand holt sie zurück und lässt sie danach von selbst
     * wieder verschwinden. Wer auf den Akkustand sehen will, kommt heran, ohne
     * die App zu verlassen.
     */
    private fun hideSystemBars() {
        val controller = WindowCompat.getInsetsController(window, window.decorView)
        controller.systemBarsBehavior =
            WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
        controller.hide(WindowInsetsCompat.Type.systemBars())
    }

    /**
     * Nach jedem Zurückkehren erneut ausblenden.
     *
     * Android zeigt die Leisten wieder, sobald eine andere Anwendung
     * dazwischen war – etwa die Kamera für ein Beweisfoto oder ein
     * Berechtigungsdialog.
     */
    override fun onWindowFocusChanged(hasFocus: Boolean) {
        super.onWindowFocusChanged(hasFocus)
        if (hasFocus) hideSystemBars()
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        hideSystemBars()

        setContent {
            val state by model.ui.collectAsStateWithLifecycle()

            OperationXTheme(fieldMode = state.fieldMode) {
                Surface(
                    modifier = Modifier.fillMaxSize(),
                    color = MaterialTheme.colorScheme.background,
                ) {
                    AppRoot(state)
                }
            }
        }
    }

    @Composable
    private fun AppRoot(state: UiState) {
        val context = LocalContext.current
        var showPermissionHint by remember { mutableStateOf(false) }
        var showRules by remember { mutableStateOf(false) }
        var showAccount by remember { mutableStateOf(false) }

        // Das Beweisfoto entsteht in der Kamera des Geräts, nicht in einer
        // eigenen Aufnahmeoberfläche: Auf einem älteren Telefon liefert die
        // Systemkamera das bessere Bild, und ein unscharfes Beweisfoto kostet
        // am Spieltag eine Rückfrage im Funk.
        var photoTarget by remember { mutableStateOf<java.io.File?>(null) }
        val takePhoto = rememberLauncherForActivityResult(
            ActivityResultContracts.TakePicture()
        ) { ok ->
            val file = photoTarget
            photoTarget = null
            if (ok && file != null) {
                model.uploadEvidence(file)
            } else {
                file?.delete()
            }
        }

        /**
         * Die Aufnahme anstoßen.
         *
         * Der Umweg über die Berechtigung ist nicht optional: Wer CAMERA im
         * Manifest deklariert – und das tut diese App für den QR-Scanner –,
         * darf ACTION_IMAGE_CAPTURE nur mit erteilter Berechtigung auslösen.
         * Sonst wirft Android eine SecurityException, und die App stürzt ab,
         * statt eine Frage zu stellen. Genau das ist beim ersten Versuch auf
         * dem Gerät passiert.
         */
        fun startPhoto() {
            val dir = java.io.File(cacheDir, "beweise").apply { mkdirs() }
            val file = java.io.File(dir, "beweis-${System.currentTimeMillis()}.jpg")
            photoTarget = file
            takePhoto.launch(
                androidx.core.content.FileProvider.getUriForFile(
                    this@MainActivity, "$packageName.fileprovider", file,
                )
            )
        }

        val askCamera = rememberLauncherForActivityResult(
            ActivityResultContracts.RequestPermission()
        ) { granted ->
            if (granted) {
                startPhoto()
            } else {
                model.showNotice(
                    "Ohne Kamerazugriff geht kein Beweisfoto. " +
                        "Der Vor-Ort-Code funktioniert weiterhin."
                )
            }
        }
        var scanning by remember { mutableStateOf(false) }

        // Die nötigen Berechtigungen werden gebündelt erfragt. Der Standort im
        // Hintergrund muss Android separat bestätigt bekommen und lässt sich
        // nicht mit abfragen – darauf weist der Hinweis unten hin.
        val permissionLauncher = rememberLauncherForActivityResult(
            ActivityResultContracts.RequestMultiplePermissions()
        ) { granted ->
            val fine = granted[Manifest.permission.ACCESS_FINE_LOCATION] == true
            if (fine) {
                model.setTracking(true)
                LocationService.start(context)
                showPermissionHint = Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q
            } else {
                model.setTracking(false)
                model.showNotice("Ohne Standortfreigabe lässt sich nicht mitspielen.")
            }
        }

        fun requestTracking(on: Boolean) {
            if (!on) {
                model.setTracking(false)
                LocationService.stop(context)
                return
            }

            val needed = buildList {
                add(Manifest.permission.ACCESS_FINE_LOCATION)
                add(Manifest.permission.ACCESS_COARSE_LOCATION)
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                    add(Manifest.permission.POST_NOTIFICATIONS)
                }
            }

            val missing = needed.any {
                ContextCompat.checkSelfPermission(context, it) != android.content.pm.PackageManager.PERMISSION_GRANTED
            }

            if (missing) {
                permissionLauncher.launch(needed.toTypedArray())
            } else {
                model.setTracking(true)
                LocationService.start(context)
            }
        }

        when (state.stage) {
            Stage.LOADING -> Box(Modifier.fillMaxSize(), contentAlignment = androidx.compose.ui.Alignment.Center) {
                CircularProgressIndicator()
            }

            Stage.JOIN -> if (scanning) {
                ScannerScreen(
                    onFound = { value ->
                        scanning = false
                        model.checkServer(value)
                    },
                    onCancel = { scanning = false },
                )
            } else {
                JoinScreen(
                    state = state,
                    onCheck = { model.checkServer(it) },
                    onScan = { scanning = true },
                )
            }

            Stage.LOGIN -> LoginScreen(
                state = state,
                onLogin = { c, p -> model.login(c, p) },
                onBack = { model.wipe() },
            )

            Stage.ONBOARDING -> OnboardingScreen(
                role = state.me?.role ?: "detective",
                trackingOn = state.tracking,
                onStartTracking = { requestTracking(true) },
                onDone = { model.finishOnboarding() },
            )

            Stage.FIELD -> FieldScreen(
                state = state,
                onPing = {
                    // Ohne Standortfreigabe wäre eine Meldung nicht möglich;
                    // dann ist die Frage danach die richtige Antwort auf den
                    // Knopf, nicht ein stilles Nichts.
                    val darf = ContextCompat.checkSelfPermission(
                        context, Manifest.permission.ACCESS_FINE_LOCATION,
                    ) == android.content.pm.PackageManager.PERMISSION_GRANTED

                    if (darf) model.reportNow() else requestTracking(true)
                },
                onTransit = { model.toggleTransit() },
                onTracking = ::requestTracking,
                onFieldMode = { model.setFieldMode(it) },
                onLogout = { model.logout() },
                onChooseOption = { model.chooseOption(it) },
                onPasscode = { model.submitPasscode(it) },
                onPhoto = {
                    val granted = androidx.core.content.ContextCompat.checkSelfPermission(
                        this@MainActivity, Manifest.permission.CAMERA,
                    ) == android.content.pm.PackageManager.PERMISSION_GRANTED

                    if (granted) startPhoto() else askCamera.launch(Manifest.permission.CAMERA)
                },
                onSolve = { id, answer -> model.solvePuzzle(id, answer) },
                onSighting = { model.reportSighting() },
                onArrest = { level, hotspot, time, target ->
                    model.arrest(level, hotspot, time, target)
                },
                onUseJoker = { kind, sector, hotspot -> model.useJoker(kind, sector, hotspot) },
                onSendRadio = { model.postRadio(it) },
                onDelay = { model.reportDelay(it) },
                onOpenRules = { model.loadRules(); showRules = true },
                onOpenAccount = { model.loadLedger(); showAccount = true },
            )
        }

        if (showRules) {
            RuleSheet(groups = state.rules, onClose = { showRules = false })
        }

        if (showAccount) {
            AccountSheet(book = state.ledger, onClose = { showAccount = false })
        }

        if (showPermissionHint) {
            BackgroundLocationDialog(
                onOpenSettings = {
                    showPermissionHint = false
                    startActivity(
                        Intent(
                            Settings.ACTION_APPLICATION_DETAILS_SETTINGS,
                            Uri.fromParts("package", packageName, null),
                        )
                    )
                },
                onDismiss = { showPermissionHint = false },
            )
        }

        state.notice?.let { text ->
            AlertDialog(
                onDismissRequest = { model.clearMessages() },
                confirmButton = {
                    TextButton(onClick = { model.clearMessages() }) { Text("Verstanden") }
                },
                text = { Text(text) },
            )
        }

        // Fehlermeldungen wurden im Feld bisher verschluckt.
        //
        // Sie landeten im Zustand, angezeigt wurden sie aber nur auf den
        // Anmeldeseiten. Wer im Spiel eine Nebelkerze ohne genug Fluchtpunkte
        // einsetzte oder eine Antwort abschickte, die der Server ablehnte,
        // sah: nichts. Die Begründung des Servers ist aber genau das, was in
        // dem Moment weiterhilft.
        if (state.notice == null) {
            state.error?.let { text ->
                AlertDialog(
                    onDismissRequest = { model.clearMessages() },
                    confirmButton = {
                        TextButton(onClick = { model.clearMessages() }) { Text("Verstanden") }
                    },
                    title = { Text("Das ging nicht") },
                    text = { Text(text) },
                )
            }
        }
    }
}

/**
 * Hinweis auf den Standort im Hintergrund.
 *
 * Android lässt diese Berechtigung nicht im normalen Dialog abfragen – sie muss
 * in den Einstellungen auf „Immer zulassen“ gestellt werden. Ohne sie hört die
 * Meldung auf, sobald der Bildschirm länger aus ist, und das kostet im Spiel
 * Punkte für etwas, das niemand falsch gemacht hat.
 */
@Composable
private fun BackgroundLocationDialog(onOpenSettings: () -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Standort auch im Hintergrund") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(
                    "Damit die Meldungen auch bei gesperrtem Bildschirm weiterlaufen, " +
                        "muss der Standortzugriff auf „Immer zulassen“ stehen."
                )
                Text(
                    "Sonst verpasst ihr Fristen, ohne etwas falsch gemacht zu haben.",
                    style = MaterialTheme.typography.bodySmall,
                )
            }
        },
        confirmButton = { TextButton(onClick = onOpenSettings) { Text("Einstellungen öffnen") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Später") } },
    )
}
