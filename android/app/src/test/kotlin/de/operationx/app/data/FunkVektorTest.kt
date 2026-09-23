package de.operationx.app.data

import java.io.File
import javax.crypto.spec.SecretKeySpec
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test

/**
 * Rechnet den gemeinsamen Prüfvektor nach.
 *
 * Dieselbe Datei prüfen auch der Server (pkg/crypt/vektor_test.go), die
 * Weboberfläche (web/src/lib/funk-kern.test.js) und die iOS-Fassung
 * (ios/Funkprobe). Ergibt eine der vier Sprachen etwas anderes, reden Server
 * und Gerät am Spieltag aneinander vorbei — und zwar lautlos, weil eine
 * misslungene Entschlüsselung aussieht wie eine leere Antwort.
 */
class FunkVektorTest {

    private val vektor: Map<String, String> by lazy {
        // Gradle führt Tests im Modulverzeichnis aus: android/app.
        val datei = File("../../testdaten/lagefunk.json")
        assertTrue("Prüfvektor nicht gefunden: ${datei.absolutePath}", datei.exists())
        Json.parseToJsonElement(datei.readText())
            .let { it as kotlinx.serialization.json.JsonObject }
            .mapValues { (_, v) -> v.jsonPrimitive.content }
    }

    private fun feld(name: String): String = vektor.getValue(name)

    private fun hex(text: String): ByteArray =
        ByteArray(text.length / 2) { text.substring(it * 2, it * 2 + 2).toInt(16).toByte() }

    private fun hexVon(roh: ByteArray): String = roh.joinToString("") { "%02x".format(it) }

    private fun funkMitVektorschluessel(): Funk =
        Funk(SecretKeySpec(hex(feld("schluesselHex")), "AES"), feld("clientPublicKey"), "PRUEF")

    @Test
    fun ableitungErgibtDieselbenBytesWieDerServer() {
        val salz = (feld("clientPublicKey") + "|" + feld("serverPublicKey")).toByteArray()
        val abgeleitet = Funk.hkdf(hex(feld("gemeinsamesGeheimnisHex")), salz,
            feld("info").toByteArray(), 32)

        assertEquals(
            "HKDF weicht ab — Salz, Info oder Verfahren stimmen nicht mehr überein",
            feld("schluesselHex"),
            hexVon(abgeleitet),
        )
    }

    @Test
    fun nachrichtDesServersLaesstSichOeffnen() {
        val klar = funkMitVektorschluessel().oeffnen(
            hex(feld("chiffreHex")),
            hex(feld("nonceHex")),
            feld("beiwerk").toByteArray(),
        )
        assertEquals(feld("klartext"), String(klar))
    }

    @Test
    fun mitFalschemBeiwerkGehtNichtsAuf() {
        // Genau dieser Schutz verhindert, dass eine abgefangene Meldung auf
        // einen anderen Weg umgebogen wird: Der Text bliebe gültig, die
        // Prüfsumme nicht.
        try {
            funkMitVektorschluessel().oeffnen(
                hex(feld("chiffreHex")),
                hex(feld("nonceHex")),
                "GET /woanders 7".toByteArray(),
            )
            fail("Mit falschem Beiwerk ließ sich die Nachricht öffnen")
        } catch (erwartet: Exception) {
            // So soll es sein.
        }
    }

    @Test
    fun eigenesVersiegeltesGehtWiederAuf() {
        val funk = funkMitVektorschluessel()
        val beiwerk = Funk.beiwerk("POST", "/api/opx/position", funk.naechsteNummer())
        val (chiffre, nonce) = funk.sealen("Standort 51.05/13.74".toByteArray(), beiwerk)

        assertEquals("Standort 51.05/13.74", String(funk.oeffnen(chiffre, nonce, beiwerk)))
    }

    @Test
    fun beiwerkHatDieFormDieDerServerErwartet() {
        assertEquals(feld("beiwerk"),
            String(Funk.beiwerk("POST", "/api/opx/position?seit=3", 7)))
    }

    @Test
    fun pfadBehaeltSeinenAbfrageteil() {
        // Der Fehler, den es schon einmal gab: Ohne Abfrageteil scheiterte
        // jede Anfrage mit einem Fragezeichen — und damit die Stadtsuche.
        assertEquals("/api/opx/live?seit=3",
            Funk.pfadVon("http://192.168.1.5:8090/api/opx/live?seit=3"))
        assertEquals("/api/opx/status",
            Funk.pfadVon("https://spiel.example.org/api/opx/status"))
        assertEquals("/api/opx/tiles/14/8800/5400",
            Funk.pfadVon("http://127.0.0.1:8090/api/opx/tiles/14/8800/5400"))
    }

    @Test
    fun base64LaeuftOhneAuffuellzeichen() {
        val roh = ByteArray(5) { it.toByte() }
        val text = Funk.b64(roh)

        assertTrue("base64url darf kein '=' enthalten: $text", !text.contains("="))
        assertTrue("base64url darf kein '+' enthalten: $text", !text.contains("+"))
        assertTrue("base64url darf kein '/' enthalten: $text", !text.contains("/"))
        assertEquals(roh.toList(), Funk.unb64(text).toList())
    }
}
