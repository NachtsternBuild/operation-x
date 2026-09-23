package de.operationx.app.ui

import android.Manifest
import android.content.pm.PackageManager
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.ImageProxy
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.view.PreviewView
import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.content.ContextCompat
import androidx.lifecycle.compose.LocalLifecycleOwner
import com.google.zxing.BinaryBitmap
import com.google.zxing.DecodeHintType
import com.google.zxing.MultiFormatReader
import com.google.zxing.PlanarYUVLuminanceSource
import com.google.zxing.BarcodeFormat
import com.google.zxing.common.HybridBinarizer
import java.util.concurrent.Executors

/**
 * QR-Beitritt.
 *
 * Der schnellste Weg ins Spiel: Die Spielleitung zeigt den Code, alle scannen
 * ihn. Erkannt wird alles, was nach einer Adresse aussieht – ob mit oder ohne
 * Protokoll davor, denn niemand tippt am Spieltag "https://" ab.
 */
@Composable
fun ScannerScreen(onFound: (String) -> Unit, onCancel: () -> Unit) {
    val context = LocalContext.current
    var hasPermission by remember {
        mutableStateOf(
            ContextCompat.checkSelfPermission(context, Manifest.permission.CAMERA)
                == PackageManager.PERMISSION_GRANTED
        )
    }

    val launcher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestPermission()
    ) { granted -> hasPermission = granted }

    LaunchedEffect(Unit) {
        if (!hasPermission) launcher.launch(Manifest.permission.CAMERA)
    }

    Box(Modifier.fillMaxSize()) {
        if (hasPermission) {
            CameraPreview(onFound)
        } else {
            Column(
                Modifier.fillMaxSize().padding(24.dp),
                verticalArrangement = Arrangement.Center,
            ) {
                Text("Kamera nicht freigegeben", style = MaterialTheme.typography.titleLarge)
                Spacer(Modifier.height(8.dp))
                Text(
                    "Ohne Kamera lässt sich kein QR-Code lesen. Die Adresse kann " +
                        "stattdessen von Hand eingegeben werden.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }

        Surface(
            modifier = Modifier.align(Alignment.BottomCenter).fillMaxWidth(),
            color = MaterialTheme.colorScheme.surface.copy(alpha = 0.9f),
        ) {
            Column(Modifier.padding(16.dp)) {
                Text(
                    "QR-Code der Spielleitung scannen",
                    style = MaterialTheme.typography.titleMedium,
                )
                Spacer(Modifier.height(8.dp))
                OutlinedButton(onClick = onCancel, modifier = Modifier.fillMaxWidth()) {
                    Text("Abbrechen und eintippen")
                }
            }
        }
    }
}

@Composable
private fun CameraPreview(onFound: (String) -> Unit) {
    val context = LocalContext.current
    val lifecycleOwner = LocalLifecycleOwner.current
    val executor = remember { Executors.newSingleThreadExecutor() }
    // ZXing statt ML Kit: ZXing steht unter Apache 2.0 und liegt vollständig
    // in der App. ML Kit ist ein geschlossenes Google-Paket – es funktioniert,
    // ließe sich aber nicht mitveröffentlichen, und auf Geräten ohne
    // Google-Dienste ist weniger Fremdes die sicherere Wahl.
    val leser = remember { qrLeser() }

    // Nur der erste Treffer zählt: Die Analyse läuft mehrmals je Sekunde, und
    // ohne diese Sperre würde derselbe Code ein Dutzend Mal gemeldet.
    var handled by remember { mutableStateOf(false) }

    DisposableEffect(Unit) {
        onDispose { executor.shutdown() }
    }

    AndroidView(
        modifier = Modifier.fillMaxSize(),
        factory = { ctx ->
            val previewView = PreviewView(ctx)
            val providerFuture = ProcessCameraProvider.getInstance(ctx)

            providerFuture.addListener({
                val provider = providerFuture.get()

                val preview = Preview.Builder().build().apply {
                    surfaceProvider = previewView.surfaceProvider
                }

                val analysis = ImageAnalysis.Builder()
                    .setBackpressureStrategy(ImageAnalysis.STRATEGY_KEEP_ONLY_LATEST)
                    .build()

                analysis.setAnalyzer(executor) { proxy ->
                    if (handled) {
                        proxy.close()
                        return@setAnalyzer
                    }
                    processFrame(proxy, leser) { value ->
                        if (!handled) {
                            handled = true
                            onFound(value)
                        }
                    }
                }

                runCatching {
                    provider.unbindAll()
                    provider.bindToLifecycle(
                        lifecycleOwner,
                        CameraSelector.DEFAULT_BACK_CAMERA,
                        preview,
                        analysis,
                    )
                }
            }, ContextCompat.getMainExecutor(ctx))

            previewView
        },
    )
}

/**
 * Ein Kamerabild nach einem QR-Code absuchen.
 *
 * CameraX liefert YUV; die Helligkeitswerte stehen vollständig in der ersten
 * Ebene, und mehr braucht ZXing nicht.
 */
private fun processFrame(
    proxy: ImageProxy,
    leser: MultiFormatReader,
    onValue: (String) -> Unit,
) {
    try {
        val ebene = proxy.planes.firstOrNull() ?: return
        val puffer = ebene.buffer
        val daten = ByteArray(puffer.remaining())
        puffer.get(daten)

        qrAusHelligkeit(leser, daten, ebene.rowStride, proxy.width, proxy.height)
            ?.let(onValue)
    } finally {
        proxy.close()
    }
}

/**
 * Der eigentliche Lesevorgang – ohne Android-Typen, damit er prüfbar ist.
 *
 * Zwei Feinheiten stecken darin, und beide sind lautlos, wenn man sie falsch
 * macht: Die Zeilenlänge im Speicher (rowStride) ist oft größer als das Bild
 * breit ist – wer stattdessen die Breite nimmt, bekommt ein schräg verzerrtes
 * Bild und findet nie etwas. Und der Leser behält ohne reset() den Zustand des
 * letzten Bildes und findet im nächsten nichts mehr.
 *
 * Die Drehung des Geräts ist hier gleichgültig: Ein QR-Code wird über seine
 * drei Ecken erkannt, in jeder Lage.
 */
internal fun qrAusHelligkeit(
    leser: MultiFormatReader,
    helligkeit: ByteArray,
    rowStride: Int,
    breite: Int,
    hoehe: Int,
): String? {
    val quelle = PlanarYUVLuminanceSource(
        helligkeit,
        rowStride,
        hoehe,
        0, 0,
        breite.coerceAtMost(rowStride),
        hoehe,
        false,
    )

    val treffer = runCatching {
        leser.decodeWithState(BinaryBitmap(HybridBinarizer(quelle)))
    }.getOrNull()

    leser.reset()
    return treffer?.text
}

/** Ein Leser mit den Einstellungen, die für den Beitritts-QR-Code gelten. */
internal fun qrLeser(): MultiFormatReader = MultiFormatReader().apply {
    setHints(
        mapOf(
            DecodeHintType.POSSIBLE_FORMATS to listOf(BarcodeFormat.QR_CODE),
            DecodeHintType.TRY_HARDER to true,
        )
    )
}
