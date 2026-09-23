package de.operationx.app.data

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Welche Adresse darf unverschlüsselt angesprochen werden?
 *
 * Diese Frage hat zwei Seiten, die beide im Prüfdurchgang aufgefallen sind.
 * Fällt sie zu streng aus, ist ein Server im Heimnetz für die App unerreichbar
 * – genau das war der Zustand vorher. Fällt sie zu locker aus, geht das
 * Zugangstoken eines Teams offen durchs Mobilfunknetz.
 */
class AdressenTest {

    @Test
    fun eigenesNetzWirdErkannt() {
        val eigene = listOf(
            "192.168.86.28", "192.168.0.1", "10.0.0.5", "172.16.4.4", "172.31.255.1",
            "127.0.0.1", "localhost", "169.254.10.2", "testdesktop", "spielserver.local",
        )
        for (host in eigene) {
            assertTrue("$host müsste als eigenes Netz gelten", Adressen.istImEigenenNetz(host))
        }
    }

    @Test
    fun fremdeAdressenSindKeinEigenesNetz() {
        val fremde = listOf(
            "example.trycloudflare.com", "8.8.8.8", "172.32.0.1", "172.15.0.1",
            "193.99.144.80", "operation-x.de",
        )
        for (host in fremde) {
            assertFalse("$host dürfte nicht als eigenes Netz gelten", Adressen.istImEigenenNetz(host))
        }
    }

    @Test
    fun hostWirdAusDerAdresseGeloest() {
        assertEquals("192.168.86.28", Adressen.host("http://192.168.86.28:8090"))
        assertEquals("beispiel.trycloudflare.com", Adressen.host("https://beispiel.trycloudflare.com/"))
        assertEquals("testdesktop", Adressen.host("testdesktop:8090"))
    }

    /**
     * Der eigentliche Fehler: Die App setzte vor jede Eingabe ohne Protokoll
     * ein "https://". Ein Server im Heimnetz spricht aber kein HTTPS, und die
     * Meldung lautete "Kein Kontakt" – ohne jeden Hinweis auf die Ursache.
     */
    @Test
    fun imEigenenNetzWirdZuerstUnverschluesseltVersucht() {
        assertEquals(
            listOf("http://192.168.86.28:8090", "https://192.168.86.28:8090"),
            Adressen.kandidaten("192.168.86.28:8090"),
        )
    }

    @Test
    fun imInternetNurVerschluesselt() {
        assertEquals(
            listOf("https://beispiel.trycloudflare.com"),
            Adressen.kandidaten("beispiel.trycloudflare.com"),
        )
    }

    @Test
    fun eingetipptesProtokollBleibtStehen() {
        assertEquals(listOf("http://testdesktop:8090"), Adressen.kandidaten("http://testdesktop:8090"))
        assertEquals(listOf("https://beispiel.de"), Adressen.kandidaten("https://beispiel.de/"))
    }

    @Test
    fun unverschluesseltInsInternetIstVerboten() {
        assertFalse(Adressen.erlaubt("http://beispiel.trycloudflare.com"))
        assertFalse(Adressen.erlaubt("http://8.8.8.8"))
        assertTrue(Adressen.erlaubt("http://192.168.86.28:8090"))
        assertTrue(Adressen.erlaubt("https://beispiel.trycloudflare.com"))
    }
}
