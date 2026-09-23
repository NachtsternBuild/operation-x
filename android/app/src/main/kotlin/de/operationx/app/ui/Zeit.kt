package de.operationx.app.ui

import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.TimeZone

/**
 * Zeitangaben vom Server.
 *
 * Der Server spricht durchgehend UTC – eine Uhrzeit ohne Ortsbezug, damit
 * Server, Weboberfläche und App über denselben Zeitpunkt reden. Auf dem
 * Bildschirm hat das nichts zu suchen: Im Punktekonto stand "11:47" für eine
 * Buchung, die um 13:47 passiert ist, und wer sie einer Erinnerung zuordnen
 * will, findet in seinem Tag nichts, was um elf Uhr war.
 */

/** Wandelt einen Zeitstempel des Servers in Millisekunden um; 0 bei Unsinn. */
fun parseIso(value: String): Long {
    if (value.isBlank()) return 0

    // Der Server schickt "2026-09-12T11:47:25Z". Die Bruchteile sind nicht
    // vorgesehen, aber ein zusätzliches Muster kostet nichts und fängt eine
    // spätere Änderung ab, statt die Anzeige einfrieren zu lassen.
    for (muster in listOf("yyyy-MM-dd'T'HH:mm:ss'Z'", "yyyy-MM-dd'T'HH:mm:ss.SSS'Z'")) {
        val parsed = runCatching {
            SimpleDateFormat(muster, Locale.US).apply {
                timeZone = TimeZone.getTimeZone("UTC")
                isLenient = false
            }.parse(value)?.time
        }.getOrNull()

        if (parsed != null && parsed > 0) return parsed
    }
    return 0
}

/** Die Uhrzeit, wie sie auf der Uhr des Geräts steht. */
fun uhrzeitLokal(iso: String): String {
    val millis = parseIso(iso)
    if (millis == 0L) return ""

    return SimpleDateFormat("HH:mm", Locale.GERMANY)
        .format(Date(millis))
}
