package de.operationx.app.data

/**
 * Serveradressen.
 *
 * Zwei Dinge, die im Prüflauf auf dem Gerät aufgefallen sind, hängen hier
 * zusammen und werden deshalb an einer Stelle geregelt.
 *
 * Erstens: Wer "192.168.86.28:8090" eintippte, bekam von der App ein
 * "https://" davorgesetzt und danach die Meldung "Kein Kontakt". Ein Server im
 * Heimnetz spricht aber kein HTTPS – die Verschlüsselung besorgt im Spiel der
 * Tunnel, und den gibt es im Heimnetz nicht. Die App muss also raten, und
 * raten heißt hier: beides versuchen, in der wahrscheinlicheren Reihenfolge.
 *
 * Zweitens: Android verbietet unverschlüsselte Verbindungen, solange sie nicht
 * ausdrücklich erlaubt sind. Die Erlaubnis ließ sich in der dafür vorgesehenen
 * Datei nicht sinnvoll formulieren – dort stehen Hostnamen, und ein ganzes
 * Heimnetz ist kein Hostname. "192.168.0.0" erlaubte genau diese eine Adresse
 * und keine der 254 dahinter. Die Erlaubnis steht deshalb jetzt allgemein in
 * der Datei, und die Einschränkung steht hier: unverschlüsselt nur ins eigene
 * Netz. Das ist strenger als vorher, weil es zum ersten Mal überhaupt greift.
 */
object Adressen {

    /**
     * Liegt diese Adresse im eigenen Netz?
     *
     * Nur für solche Adressen darf es ohne Verschlüsselung gehen: Sie
     * verlassen die Wohnung nicht. Alles andere – und das ist am Spieltag der
     * Tunnel – muss verschlüsselt sein, sonst läge das Zugangstoken jedes
     * Teams offen im Mobilfunknetz.
     */
    fun istImEigenenNetz(host: String): Boolean {
        val h = host.lowercase().trim().removeSurrounding("[", "]")

        if (h == "localhost" || h == "::1") return true
        if (h.endsWith(".local") || h.endsWith(".lan") || h.endsWith(".home.arpa")) return true

        // Ein Name ohne Punkt kann nur aus dem eigenen Netz kommen; im
        // Internet gibt es keine Namen ohne Punkt.
        if (!h.contains(".") && !h.contains(":")) return true

        val teile = h.split(".")
        if (teile.size != 4) return false
        val zahlen = teile.map { it.toIntOrNull() ?: return false }
        if (zahlen.any { it !in 0..255 }) return false

        return when {
            zahlen[0] == 127 -> true                          // Loopback
            zahlen[0] == 10 -> true                           // privat
            zahlen[0] == 192 && zahlen[1] == 168 -> true      // privat
            zahlen[0] == 172 && zahlen[1] in 16..31 -> true   // privat
            zahlen[0] == 169 && zahlen[1] == 254 -> true      // ohne DHCP vergeben
            else -> false
        }
    }

    /** Der Namensteil einer Adresse, ohne Protokoll, Port und Pfad. */
    fun host(url: String): String =
        url.substringAfter("://")
            .substringBefore('/')
            .substringBeforeLast(':')
            .ifBlank { url }

    /**
     * Was auszuprobieren ist, in dieser Reihenfolge.
     *
     * Steht ein Protokoll davor, gilt es. Sonst wird geraten – im eigenen Netz
     * zuerst unverschlüsselt, sonst zuerst verschlüsselt –, und die andere
     * Möglichkeit kommt als zweiter Versuch hinterher.
     */
    fun kandidaten(eingabe: String): List<String> {
        val roh = eingabe.trim().trimEnd('/')
        if (roh.isBlank()) return emptyList()

        if (roh.startsWith("http://") || roh.startsWith("https://")) {
            return listOf(roh)
        }

        return if (istImEigenenNetz(host(roh))) {
            listOf("http://$roh", "https://$roh")
        } else {
            listOf("https://$roh")
        }
    }

    /** Darf diese Adresse benutzt werden? */
    fun erlaubt(url: String): Boolean =
        url.startsWith("https://") ||
            (url.startsWith("http://") && istImEigenenNetz(host(url)))
}
