package de.operationx.app.ui.theme

import androidx.compose.material3.Typography
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp

/**
 * Schrift.
 *
 * Material You bringt die Systemschrift mit, und die ist auf jedem Gerät
 * vertraut – deshalb wird sie nicht ersetzt. Zählbares steht in Monospace,
 * damit Countdown und Punktestände nicht bei jeder Ziffer zappeln.
 *
 * Im Feldmodus wächst alles um rund ein Sechstel: Bei Sonne auf der Straße ist
 * das der Unterschied zwischen lesbar und geraten.
 */
fun opxTypography(fieldMode: Boolean): Typography {
    val f = if (fieldMode) 1.18f else 1f
    val base = Typography()

    return base.copy(
        displaySmall = base.displaySmall.copy(fontSize = 32.sp * f),
        headlineMedium = base.headlineMedium.copy(fontSize = 26.sp * f),
        headlineSmall = base.headlineSmall.copy(fontSize = 22.sp * f),
        titleLarge = base.titleLarge.copy(fontSize = 21.sp * f),
        titleMedium = base.titleMedium.copy(fontSize = 17.sp * f),
        bodyLarge = base.bodyLarge.copy(fontSize = 16.sp * f),
        bodyMedium = base.bodyMedium.copy(fontSize = 14.sp * f),
        bodySmall = base.bodySmall.copy(fontSize = 12.sp * f),
        labelLarge = base.labelLarge.copy(fontSize = 15.sp * f),
        labelMedium = base.labelMedium.copy(fontSize = 13.sp * f),
    )
}

/** Für alles Zählbare: Countdown, Punkte, Koordinaten. */
val MonoStyle = TextStyle(
    fontFamily = FontFamily.Monospace,
    fontWeight = FontWeight.Medium,
)
