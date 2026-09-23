package de.operationx.app.ui

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Der Server spricht UTC, der Bildschirm spricht Ortszeit.
 *
 * Im Punktekonto stand "11:47" für eine Buchung, die um 13:47 passiert ist –
 * die Uhrzeit des Servers, unverändert durchgereicht. Für ein Konto, das genau
 * dazu da ist, eine Buchung einer Erinnerung zuzuordnen, ist das wertlos.
 */
class ZeitTest {

    @Test
    fun zeitstempelWirdAlsUtcGelesen() {
        // 2026-09-12T11:47:25Z entspricht 1789213645000 ms seit 1970.
        assertEquals(1789213645000L, parseIso("2026-09-12T11:47:25Z"))
    }

    @Test
    fun bruchteileWerdenVerkraftet() {
        assertEquals(1789213645123L, parseIso("2026-09-12T11:47:25.123Z"))
    }

    @Test
    fun unsinnGibtNull() {
        assertEquals(0L, parseIso(""))
        assertEquals(0L, parseIso("morgen früh"))
        assertEquals(0L, parseIso("2026-09-12"))
    }

    /**
     * Die Anzeige folgt der Uhr des Geräts. Geprüft wird gegen dieselbe
     * Umrechnung, die auch das Telefon macht – eine feste Uhrzeit wäre nur in
     * einer Zeitzone richtig und würde den Test je nach Rechner brechen.
     */
    @Test
    fun anzeigeFolgtDerGeraeteUhr() {
        val iso = "2026-09-12T11:47:25Z"
        val erwartet = java.text.SimpleDateFormat("HH:mm", java.util.Locale.GERMANY)
            .format(java.util.Date(1789213645000L))

        assertEquals(erwartet, uhrzeitLokal(iso))
        assertEquals("", uhrzeitLokal("kein Zeitstempel"))
    }
}
