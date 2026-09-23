package de.operationx.app

import com.google.zxing.BarcodeFormat
import com.google.zxing.EncodeHintType
import com.google.zxing.qrcode.QRCodeWriter
import de.operationx.app.ui.qrAusHelligkeit
import de.operationx.app.ui.qrLeser
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * Der QR-Beitritt, ohne Kamera.
 *
 * Geprüft wird nicht ZXing, sondern die Art, wie diese App es benutzt: die
 * Einstellungen des Lesers, das Zurücksetzen zwischen zwei Bildern und die
 * Zeilenlänge im Speicher. Genau diese drei sind es, die im Fehlerfall nichts
 * melden – die Kamera läuft, das Bild ist da, und es passiert einfach nie
 * etwas.
 */
class QrLeserTest {

    /** Erzeugt dieselbe Art Helligkeitsbild, wie CameraX es liefert. */
    private fun helligkeitsbild(
        text: String,
        seite: Int = 400,
        zeilenlaenge: Int = 448,   // absichtlich größer als das Bild breit ist
    ): ByteArray {
        val matrix = QRCodeWriter().encode(
            text, BarcodeFormat.QR_CODE, seite, seite,
            mapOf(EncodeHintType.MARGIN to 2),
        )

        // 0 ist schwarz, -1 (0xFF) ist weiß – wie bei einer echten Aufnahme.
        val daten = ByteArray(zeilenlaenge * seite) { -1 }
        for (y in 0 until seite) {
            for (x in 0 until seite) {
                if (matrix.get(x, y)) daten[y * zeilenlaenge + x] = 0
            }
        }
        return daten
    }

    @Test
    fun `liest die Beitrittsadresse aus einem Bild`() {
        val adresse = "https://operation-x.example.org"
        val bild = helligkeitsbild(adresse)

        val gelesen = qrAusHelligkeit(qrLeser(), bild, 448, 400, 400)

        assertEquals(adresse, gelesen)
    }

    @Test
    fun `findet nacheinander mehrere Bilder`() {
        // Ohne reset() im Leser bliebe der Zustand des ersten Bildes hängen –
        // der zweite Code würde nie gefunden.
        val leser = qrLeser()

        val erster = qrAusHelligkeit(leser, helligkeitsbild("http://192.168.1.5:8090"), 448, 400, 400)
        val zweiter = qrAusHelligkeit(leser, helligkeitsbild("http://192.168.1.9:8090"), 448, 400, 400)

        assertEquals("http://192.168.1.5:8090", erster)
        assertEquals("http://192.168.1.9:8090", zweiter)
    }

    @Test
    fun `ein Bild ohne Code liefert nichts und wirft nicht`() {
        val leer = ByteArray(448 * 400) { -1 }

        assertNull(qrAusHelligkeit(qrLeser(), leer, 448, 400, 400))
    }
}
