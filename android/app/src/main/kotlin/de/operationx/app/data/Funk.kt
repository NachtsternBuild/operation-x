package de.operationx.app.data

import java.math.BigInteger
import java.util.Base64
import java.security.AlgorithmParameters
import java.security.KeyFactory
import java.security.KeyPairGenerator
import java.security.SecureRandom
import java.security.interfaces.ECPublicKey
import java.security.spec.ECGenParameterSpec
import java.security.spec.ECParameterSpec
import java.security.spec.ECPoint
import java.security.spec.ECPublicKeySpec
import javax.crypto.Cipher
import javax.crypto.KeyAgreement
import javax.crypto.Mac
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

/**
 * Der Lagefunk: eine zweite Verschlüsselung zwischen Telefon und Spielserver.
 *
 * Warum, wo doch alles über HTTPS läuft: Der Tunnel endet nicht auf dem
 * Spielserver, sondern bei seinem Betreiber. Dort wird entschlüsselt und neu
 * verschlüsselt – das ist der Sinn eines Tunnels und der Grund, warum er ohne
 * eigenes Zertifikat funktioniert. Es heißt aber auch: Wer ihn betreibt, sieht
 * den Klartext. Bei diesem Spiel wäre das der Standortverlauf von zwanzig
 * Leuten über sechs Stunden, darunter Jugendliche, und gespielt wird dort, wo
 * sie wohnen.
 *
 * Also verschlüsselt dieses Telefon seine Anfragen ein zweites Mal, mit einem
 * Schlüssel, den nur es und der Spielserver kennen.
 *
 * Was das nicht leistet, und das gehört dazu: Der Spielserver sieht weiterhin
 * alles – er ist das Spiel. Und wer mitliest, sieht weiterhin, wann wie viel
 * an welche Adresse geht.
 *
 * Verfahren: ECDH auf P-256, daraus per HKDF ein Sitzungsschlüssel, damit
 * AES-256-GCM. Alles aus der Java-Kryptografie, die auf jedem Android liegt –
 * kein Zusatzpaket, das am Spieltag fehlen könnte. Dasselbe steht in Go und im
 * Browser noch einmal, Zeichen für Zeichen gleich.
 */
// Der Konstruktor ist modulintern statt privat: Der Prüfvektor in
// FunkVektorTest baut damit eine Verschlüsselung aus einem festen Schlüssel
// und rechnet nach, ob dieselben Bytes herauskommen wie im Server. Von außen
// bleibt weiterhin nur aufbauen() erreichbar.
class Funk internal constructor(
    private val key: SecretKeySpec,
    /** Der eigene öffentliche Schlüssel, wie er in die Kopfzeile geht. */
    val publicKey: String,
    /** Das Kennzeichen des Servers zum Vergleichen. */
    val fingerprint: String,
) {

    companion object {
        private const val INFO = "operation-x/lagefunk/1"
        private const val KURVE = "secp256r1"

        /**
         * Baut die Verschlüsselung zu einem Server auf.
         *
         * [serverPublicKey] kommt entweder vom Server selbst oder – besser –
         * aus dem QR-Code der Teamkarte: Der wird auf dem Rechner der
         * Spielleitung gedruckt und läuft damit nicht durch den Tunnel. Wer
         * ihn scannt, hat den Schlüssel aus erster Hand.
         */
        fun aufbauen(serverPublicKey: String, fingerprint: String): Funk? = runCatching {
            val params = AlgorithmParameters.getInstance("EC").run {
                init(ECGenParameterSpec(KURVE))
                getParameterSpec(ECParameterSpec::class.java)
            }

            val erzeuger = KeyPairGenerator.getInstance("EC").apply {
                initialize(ECGenParameterSpec(KURVE), SecureRandom())
            }
            val paar = erzeuger.generateKeyPair()

            val eigenerPub = punktBytes(paar.public as ECPublicKey)
            val eigenerPubText = b64(eigenerPub)

            val serverPunkt = punktLesen(unb64(serverPublicKey), params)
            val serverKey = KeyFactory.getInstance("EC")
                .generatePublic(ECPublicKeySpec(serverPunkt, params))

            val gemeinsam = KeyAgreement.getInstance("ECDH").run {
                init(paar.private)
                doPhase(serverKey, true)
                generateSecret()
            }

            // Dieselbe Ableitung wie auf dem Server: Das Salz sind beide
            // öffentlichen Schlüssel, damit kein zweites Gerät denselben
            // Sitzungsschlüssel bekommt.
            val salz = "$eigenerPubText|$serverPublicKey".toByteArray()
            val roh = hkdf(gemeinsam, salz, INFO.toByteArray(), 32)

            Funk(SecretKeySpec(roh, "AES"), eigenerPubText, fingerprint)
        }.getOrNull()

        // --- Umrechnungen zwischen Punkt und Bytes ---

        /** Unkomprimierte Punktdarstellung: 0x04 || X || Y, je 32 Byte. */
        private fun punktBytes(pub: ECPublicKey): ByteArray {
            val x = feld(pub.w.affineX)
            val y = feld(pub.w.affineY)
            return byteArrayOf(4) + x + y
        }

        private fun punktLesen(roh: ByteArray, params: ECParameterSpec): ECPoint {
            require(roh.size == 65 && roh[0] == 4.toByte()) {
                "unerwartete Form des öffentlichen Schlüssels"
            }
            val x = BigInteger(1, roh.copyOfRange(1, 33))
            val y = BigInteger(1, roh.copyOfRange(33, 65))
            require(params.order.signum() > 0)
            return ECPoint(x, y)
        }

        /**
         * Eine Koordinate auf genau 32 Byte bringen.
         *
         * BigInteger.toByteArray() liefert je nach Zahl 31, 32 oder 33 Bytes –
         * mit führender Null, wenn das oberste Bit gesetzt ist. Ungeprüft
         * übernommen ergibt das einen Schlüssel, den die Gegenseite nicht
         * versteht, und zwar nur bei jedem zweiten Verbindungsaufbau.
         */
        private fun feld(v: BigInteger): ByteArray {
            val roh = v.toByteArray()
            return when {
                roh.size == 32 -> roh
                roh.size > 32 -> roh.copyOfRange(roh.size - 32, roh.size)
                else -> ByteArray(32 - roh.size) + roh
            }
        }

        /** HKDF nach RFC 5869 mit SHA-256 – dieselben Schritte wie im Server. */
        internal fun hkdf(secret: ByteArray, salz: ByteArray, info: ByteArray, laenge: Int): ByteArray {
            val prk = Mac.getInstance("HmacSHA256").run {
                init(SecretKeySpec(salz, "HmacSHA256"))
                doFinal(secret)
            }

            var block = ByteArray(0)
            var out = ByteArray(0)
            var i = 1
            while (out.size < laenge) {
                val mac = Mac.getInstance("HmacSHA256")
                mac.init(SecretKeySpec(prk, "HmacSHA256"))
                mac.update(block)
                mac.update(info)
                mac.update(i.toByte())
                block = mac.doFinal()
                out += block
                i++
            }
            return out.copyOf(laenge)
        }

        /**
         * Das Beiwerk, mit dem versiegelt wird.
         *
         * "METHODE /pfad?abfrage nummer" – und zwar zeichengenau so, wie es
         * der Server baut (internal/api/secure.go, funkZusatz). Diese Zeile
         * ist schon einmal auseinandergelaufen: Der Server nahm die Adresse
         * samt Abfrageteil, der Browser nur den Pfad. Ergebnis war, dass jede
         * Anfrage mit einem "?" scheiterte – und zwar lautlos. Deshalb steht
         * sie hier als eigene Funktion und nicht mehr an jeder Aufrufstelle.
         */
        fun beiwerk(methode: String, pfad: String, nummer: Long): ByteArray =
            "$methode $pfad $nummer".toByteArray()

        /**
         * Der Pfad einer Adresse, mit Abfrageteil.
         *
         * Aus "http://192.168.1.5:8090/api/opx/live?seit=3" wird
         * "/api/opx/live?seit=3" – genau das, was auf der anderen Seite in
         * r.URL.RequestURI() steht.
         */
        fun pfadVon(url: String): String =
            url.substringAfter("://").substringAfter("/").let { "/$it" }

        fun b64(roh: ByteArray): String =
            Base64.getUrlEncoder().withoutPadding().encodeToString(roh)

        fun unb64(text: String): ByteArray =
            Base64.getUrlDecoder().decode(text)
    }

    /**
     * Die laufende Nummer gegen Wiedereinspielen.
     *
     * Der Server führt dazu ein Fenster: Nummern dürfen durcheinander
     * eintreffen, aber keine zweimal.
     */
    private var seq = 0L

    @Synchronized
    fun naechsteNummer(): Long = ++seq

    /** Verschlüsselt einen Rumpf. Liefert Chiffre und Zufallswert. */
    fun sealen(klartext: ByteArray, zusatz: ByteArray): Pair<ByteArray, ByteArray> {
        val nonce = ByteArray(12).also { SecureRandom().nextBytes(it) }

        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, key, GCMParameterSpec(128, nonce))
        cipher.updateAAD(zusatz)

        return cipher.doFinal(klartext) to nonce
    }

    /** Entschlüsselt eine Antwort. */
    fun oeffnen(chiffre: ByteArray, nonce: ByteArray, zusatz: ByteArray): ByteArray {
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(128, nonce))
        cipher.updateAAD(zusatz)
        return cipher.doFinal(chiffre)
    }

    /** Entschlüsselt eine Sendung des Lagestroms: Zufallswert und Text in einem. */
    fun oeffneStromSendung(text: String): String {
        val paket = unb64(text)
        val klar = oeffnen(
            paket.copyOfRange(12, paket.size),
            paket.copyOfRange(0, 12),
            "GET /api/opx/stream".toByteArray(),
        )
        return String(klar)
    }
}
