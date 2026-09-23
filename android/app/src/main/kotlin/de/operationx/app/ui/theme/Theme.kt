package de.operationx.app.ui.theme

import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext

/**
 * Die Farben der Parteien.
 *
 * Sie sind bewusst NICHT Teil des Material-Farbschemas und werden auch nicht
 * von der dynamischen Farbe berührt: Sie tragen Information. Wer sein
 * Hintergrundbild wechselt, darf nicht plötzlich die Fahndung in der Farbe der
 * Zielperson sehen.
 *
 * Material You bestimmt alles andere – Flächen, Schaltflächen, Formen. Das ist
 * die Arbeitsteilung: Das System liefert die Anmutung, das Spiel die Bedeutung.
 */
object PartyColors {
    val misterX = Color(0xFFFF5C47)
    val detective = Color(0xFF35C0D6)
    val hq = Color(0xFFD9A441)

    /** Zustände des Regelwerks, ebenfalls fest. */
    val ok = Color(0xFF4EC9A0)
    val warn = Color(0xFFE5A03D)
    val critical = Color(0xFFF2564F)

    /** Hinweisqualität. */
    val intelGreen = Color(0xFF4EC9A0)
    val intelYellow = Color(0xFFE5C04D)
    val intelRed = Color(0xFFF2564F)

    fun forRole(role: String): Color = when (role) {
        "misterx" -> misterX
        "hq" -> hq
        else -> detective
    }
}

/**
 * Feldmodus: Draußen in der Sonne gewinnt Lesbarkeit gegen Anmutung.
 * Größere Schrift, kräftigerer Kontrast, daumengroße Bedienelemente.
 */
val LocalFieldMode = staticCompositionLocalOf { false }

// Rückfallschema für Geräte ohne dynamische Farbe – die dunkle Lagekarte,
// die auch die Weboberfläche verwendet.
private val FallbackDark = darkColorScheme(
    primary = Color(0xFFD9A441),
    onPrimary = Color(0xFF0B1215),
    secondary = Color(0xFF35C0D6),
    background = Color(0xFF0B1215),
    onBackground = Color(0xFFCCD6DA),
    surface = Color(0xFF111A1E),
    onSurface = Color(0xFFCCD6DA),
    surfaceVariant = Color(0xFF16232A),
    onSurfaceVariant = Color(0xFF7A8D95),
    outline = Color(0xFF344B56),
    error = Color(0xFFFF5C47),
)

private val FallbackLight = lightColorScheme(
    primary = Color(0xFF8A6A1F),
    secondary = Color(0xFF186B7A),
    error = Color(0xFFB3261E),
)

@Composable
fun OperationXTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    fieldMode: Boolean = false,
    content: @Composable () -> Unit,
) {
    val context = LocalContext.current

    val colors = when {
        // Dynamische Farbe gibt es ab Android 12. Im Feldmodus bleibt es beim
        // festen dunklen Schema, weil dort Kontrast zählt und nicht Geschmack.
        fieldMode -> FallbackDark
        Build.VERSION.SDK_INT >= Build.VERSION_CODES.S && darkTheme -> dynamicDarkColorScheme(context)
        Build.VERSION.SDK_INT >= Build.VERSION_CODES.S -> dynamicLightColorScheme(context)
        darkTheme -> FallbackDark
        else -> FallbackLight
    }

    CompositionLocalProvider(LocalFieldMode provides fieldMode) {
        MaterialTheme(
            colorScheme = colors,
            typography = opxTypography(fieldMode),
            content = content,
        )
    }
}
