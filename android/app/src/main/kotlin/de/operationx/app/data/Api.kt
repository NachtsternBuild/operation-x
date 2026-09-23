package de.operationx.app.data

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.MultipartBody
import okhttp3.RequestBody.Companion.asRequestBody
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.io.File
import java.util.concurrent.TimeUnit

/** Fehler mit der Meldung, die der Server geschickt hat. */
class ApiException(val status: Int, message: String) : Exception(message)

/**
 * Zugriff auf den Spielserver.
 *
 * Die App spricht dieselbe Schnittstelle wie die Weboberfläche und erbt von ihr
 * nichts anderes. Deshalb ist /api/opx die einzige gemeinsame Grundlage – was
 * hier nicht abgebildet ist, kann die App nicht.
 */
class Api(private val session: Session) {

    private val json = Json {
        ignoreUnknownKeys = true
        encodeDefaults = true
    }

    private val client = OkHttpClient.Builder()
        // Unterwegs ist die Verbindung oft schlecht, aber selten tot. Lieber
        // etwas warten als sofort aufgeben und den Puffer volllaufen lassen.
        .connectTimeout(15, TimeUnit.SECONDS)
        .readTimeout(30, TimeUnit.SECONDS)
        .retryOnConnectionFailure(true)
        .build()

    private val jsonType = "application/json; charset=utf-8".toMediaType()
    private val binaryType = "application/octet-stream".toMediaType()

    /**
     * Die zweite Verschlüsselung.
     *
     * Sie wird beim ersten Zugriff aufgebaut und danach behalten. Klappt sie
     * nicht – alter Server, unerwartete Antwort –, läuft alles wie bisher:
     * über die Verschlüsselung der Verbindung allein. Ein Spiel darf nicht
     * daran scheitern, dass eine zusätzliche Schicht fehlt.
     */
    @Volatile
    private var funk: Funk? = null

    @Volatile
    /** Zu welchem Server der Sitzungsschlüssel gehört. */
    private var funkBasis = ""
    /** Wann zuletzt vergeblich nach einem Schlüssel gefragt wurde. */
    private var letzterFunkversuch = 0L

    private val funkSperre = Any()

    /** Das Kennzeichen des Servers, sobald es vorliegt – zum Vergleichen. */
    val fingerprint: String?
        get() = funk?.fingerprint

    val verschluesselt: Boolean
        get() = funk != null

    /**
     * Baut den Sitzungsschlüssel zu genau diesem Server auf.
     *
     * Zwei Dinge sind hier wichtiger, als sie aussehen.
     *
     * **Der Schlüssel gehört zu einer Adresse.** Jeder Server hat einen
     * eigenen; wer nächste Woche auf einem anderen spielt, darf nicht mit dem
     * Schlüssel von heute reden. Vorher merkte sich die App nur, *dass* sie
     * einen hat – nach einem Serverwechsel wäre das der falsche gewesen.
     *
     * **Eine Nachfrage wird beantwortet.** Mit `neu` wird der gespeicherte
     * Schlüssel weggeworfen und frisch ausgehandelt. Sonst gäbe die App auf
     * der Anmeldeseite das Kennzeichen von vorhin aus – und genau dann, wenn
     * hinter derselben Adresse ein anderer Server steht, wäre das das
     * falsche.
     *
     * **Ein Fehlversuch ist kein Urteil.** Vorher hieß "einmal nicht
     * erreicht" für den Rest des Programmlaufs "dieser Server kann das
     * nicht", und das Spiel lief unverschlüsselt weiter – sichtbar nur daran,
     * dass oben das Schloss fehlt. Jetzt wird es nach einer halben Minute
     * wieder versucht.
     */
    private suspend fun funkAufbauen(base: String, neu: Boolean = false): Funk? {
        synchronized(funkSperre) {
            if (!neu && funk != null && funkBasis == base) return funk

            if (neu || funkBasis != base) {
                funk = null
                letzterFunkversuch = 0L
            }
            if (System.currentTimeMillis() - letzterFunkversuch < 30_000) return null
        }

        val info = runCatching {
            request<ServerKey>("GET", "$base/api/opx/key", null, null)
        }.getOrNull()

        synchronized(funkSperre) {
            funkBasis = base
            letzterFunkversuch = System.currentTimeMillis()
            if (info != null && info.available) {
                funk = Funk.aufbauen(info.publicKey, info.fingerprint)
            }
            return funk
        }
    }

    /**
     * Holt das Kennzeichen eines Servers – und baut dabei den Schlüssel auf.
     *
     * Wird beim Verbinden aufgerufen, damit die Anmeldeseite es zeigen kann:
     * Vergleichen lässt es sich nur, bevor jemand sein Kennwort eintippt.
     */
    suspend fun kennzeichenVon(base: String): String? =
        funkAufbauen(base, neu = true)?.fingerprint

    /**
     * Die Verschlüsselung für den Lagestrom.
     *
     * Der Strom hat eine eigene Verbindung mit eigenen Zeitgrenzen, benutzt
     * aber denselben Sitzungsschlüssel: Zwei Schlüssel für dasselbe Gerät
     * wären zwei Dinge, die auseinanderlaufen können.
     */
    suspend fun funkFuerStrom(base: String): Funk? = funkAufbauen(base)

    /** Wirft den Sitzungsschlüssel weg – nach einem Serverwechsel. */
    fun funkZuruecksetzen() {
        synchronized(funkSperre) {
            funk = null
            funkBasis = ""
            letzterFunkversuch = 0L
        }
    }

    // --- Anmeldung ---

    suspend fun status(baseUrl: String): ServerStatus =
        request("GET", "$baseUrl/api/opx/status", null, null)

    suspend fun login(baseUrl: String, callsign: String, password: String): Pair<String, Me> {
        val body = JsonObject(
            mapOf(
                "identity" to JsonPrimitive(callsign),
                "password" to JsonPrimitive(password),
            )
        ).toString()

        val auth: AuthResponse = request(
            "POST", "$baseUrl/api/collections/teams/auth-with-password", body, null
        )

        val me: Me = request("GET", "$baseUrl/api/opx/me", null, auth.token)
        return auth.token to me
    }

    // --- Spiel ---

    suspend fun me(): Me = authed("GET", "/api/opx/me")
    suspend fun live(): LiveState = authed("GET", "/api/opx/live")
    suspend fun map(): FieldMap = authed("GET", "/api/opx/map")
    suspend fun intel(): IntelList = authed("GET", "/api/opx/intel")
    suspend fun puzzles(): PuzzleList = authed("GET", "/api/opx/puzzles")
    suspend fun jokers(): JokerList = authed("GET", "/api/opx/jokers")
    suspend fun radio(): RadioList = authed("GET", "/api/opx/radio")
    suspend fun mission(): MissionState = authed("GET", "/api/opx/mission")

    suspend fun sendPositions(reports: List<PositionReport>): PositionAck =
        authed("POST", "/api/opx/position", json.encodeToString(PositionBatch(reports)))

    suspend fun setTransit(active: Boolean): Unit =
        authedUnit("POST", "/api/opx/transit", """{"active":$active}""")

    suspend fun solvePuzzle(id: String, answer: String): SolveResult =
        authed("POST", "/api/opx/puzzles/$id/solve", jsonOf("answer" to answer))

    suspend fun chooseOption(optionId: String): MissionState =
        authed("POST", "/api/opx/mission/choose", jsonOf("optionId" to optionId))

    suspend fun submitPasscode(code: String): Unit =
        authedUnit("POST", "/api/opx/mission/evidence", jsonOf("passcode" to code))

    /**
     * Beweisfoto hochladen.
     *
     * Der zweite Weg, eine Ankunft nachzuweisen – und der einzige, wenn der
     * Zettel mit dem Vor-Ort-Code abgerissen wurde. Bisher gab es ihn nur im
     * Browser, was bedeutete: Wer die App benutzt, muss für genau diesen Fall
     * hinüberwechseln.
     *
     * Die Zentrale sieht das Foto und entscheidet; solange sie prüft, läuft
     * die Frist nicht weiter.
     */
    suspend fun uploadEvidence(photo: File): Unit = withContext(Dispatchers.IO) {
        val base = session.currentServer()
        val token = session.token() ?: throw ApiException(401, "Nicht angemeldet.")

        val body = MultipartBody.Builder()
            .setType(MultipartBody.FORM)
            .addFormDataPart(
                "photo", photo.name,
                photo.asRequestBody("image/jpeg".toMediaType()),
            )
            .build()

        val builder = Request.Builder()
            .url(base + "/api/opx/mission/evidence")
            .header("Authorization", token)

        // Auch das Beweisfoto geht verschlüsselt hinaus. Es zeigt, wo jemand
        // steht – und ein Bild verrät den Ort oft deutlicher als eine
        // Koordinate.
        val funk = funkFuerStrom(base)
        if (funk != null) {
            val puffer = okio.Buffer()
            body.writeTo(puffer)

            val seq = funk.naechsteNummer()
            val zusatz = Funk.beiwerk("POST", "/api/opx/mission/evidence", seq)
            val (chiffre, nonce) = funk.sealen(puffer.readByteArray(), zusatz)

            builder.header("X-Opx-Key", funk.publicKey)
            builder.header("X-Opx-Seq", seq.toString())
            builder.header("X-Opx-Nonce", Funk.b64(nonce))
            builder.header("X-Opx-Type", body.contentType().toString())
            builder.post(chiffre.toRequestBody(binaryType))
        } else {
            builder.post(body)
        }

        client.newCall(builder.build()).execute().use { response ->
            if (!response.isSuccessful) {
                val text = response.body?.string().orEmpty()
                throw ApiException(response.code, extractMessage(text, response.code))
            }
        }
    }

    suspend fun useJoker(kind: String, sectorId: String? = null, hotspotId: String? = null): JsonObject {
        val fields = buildMap {
            put("kind", JsonPrimitive(kind))
            sectorId?.let { put("sectorId", JsonPrimitive(it)) }
            hotspotId?.let { put("hotspotId", JsonPrimitive(it)) }
        }
        return authed("POST", "/api/opx/jokers", JsonObject(fields).toString())
    }

    suspend fun postRadio(text: String): Unit =
        authedUnit("POST", "/api/opx/radio", jsonOf("text" to text))

    suspend fun reportSighting(): SightingResult = authed("POST", "/api/opx/sighting", "{}")

    /**
     * Der Zugriff.
     *
     * Der Zeitpunkt geht als vollständiger Zeitstempel hinaus, nicht als
     * Uhrzeit: Der Server rechnet damit, und eine Uhrzeit ohne Datum wäre am
     * Tageswechsel mehrdeutig.
     */
    suspend fun arrest(
        level: Int,
        hotspot: Int,
        timeIso: String? = null,
        target: Int? = null,
    ): ArrestResult {
        val fields = buildMap {
            put("level", JsonPrimitive(level))
            put("claimedHotspot", JsonPrimitive(hotspot))
            timeIso?.let { put("claimedTime", JsonPrimitive(it)) }
            target?.let { put("claimedTarget", JsonPrimitive(it)) }
        }
        return authed("POST", "/api/opx/arrest", JsonObject(fields).toString())
    }

    /**
     * Verzögerung melden.
     *
     * Für den Fall, dass der Auftrag am Ziel selbst Zeit kostet — die Schlange
     * an der Kasse, der Laden zweihundert Meter neben der Nadel. Steht die
     * Zielperson am Ziel, kommt die Zeit sofort dazu; sonst entscheidet die
     * Zentrale.
     */
    suspend fun reportDelay(reason: String): DelayResult =
        authed("POST", "/api/opx/mission/delay", jsonOf("reason" to reason))

    suspend fun rules(): RuleBook = authed("GET", "/api/opx/rules")

    /**
     * Das Punktekonto.
     *
     * Jede Buchung mit ihrer Begründung – die Antwort auf "warum habe ich
     * minus fünfzehn?", ohne dass jemand dafür den Funk belegen muss.
     */
    suspend fun ledger(): LedgerBook = authed("GET", "/api/opx/ledger")

    suspend fun markOnboarded(): Unit = authedUnit("POST", "/api/opx/onboarded", "{}")

    // --- Innereien ---

    private fun jsonOf(vararg pairs: Pair<String, String>): String =
        JsonObject(pairs.associate { it.first to JsonPrimitive(it.second) }).toString()

    private suspend inline fun <reified T> authed(method: String, path: String, body: String? = null): T {
        val base = session.currentServer()
        val token = session.token() ?: throw ApiException(401, "Nicht angemeldet.")
        return request(method, base + path, body, token)
    }

    private suspend fun authedUnit(method: String, path: String, body: String?) {
        val base = session.currentServer()
        val token = session.token() ?: throw ApiException(401, "Nicht angemeldet.")
        requestRaw(method, base + path, body, token)
    }

    private suspend inline fun <reified T> request(
        method: String,
        url: String,
        body: String?,
        token: String?,
    ): T {
        val text = requestRaw(method, url, body, token)
        return json.decodeFromString(text)
    }

    suspend fun requestRaw(method: String, url: String, body: String?, token: String?): String =
        withContext(Dispatchers.IO) {
            // Mit Abfrageteil: Er steht im Beiwerk der Verschlüsselung und ist
            // damit genauso unveränderlich wie der Rumpf. Ohne ihn wäre der
            // Pfad geschützt und die Frage daran nicht.
            val pfad = Funk.pfadVon(url)

            // Den Schlüssel selbst holt die App im Klartext – er ist öffentlich,
            // und ohne ihn gäbe es keine Verschlüsselung, mit der man ihn holen
            // könnte.
            val f = if (pfad.substringBefore("?") == "/api/opx/key") null
            else funkAufbauen(url.substringBefore("/api/"))

            val builder = Request.Builder().url(url)
            token?.let { builder.header("Authorization", it) }

            var zusatz = ByteArray(0)

            if (f != null) {
                val seq = f.naechsteNummer()
                zusatz = Funk.beiwerk(method, pfad, seq)
                builder.header("X-Opx-Key", f.publicKey)
                builder.header("X-Opx-Seq", seq.toString())

                when (method) {
                    "GET" -> builder.get()
                    "POST" -> {
                        val (chiffre, nonce) = f.sealen((body ?: "{}").toByteArray(), zusatz)
                        builder.header("X-Opx-Nonce", Funk.b64(nonce))
                        builder.post(chiffre.toRequestBody(binaryType))
                    }
                    else -> throw IllegalArgumentException("Unbekannte Methode $method")
                }
            } else {
                when (method) {
                    "GET" -> builder.get()
                    "POST" -> builder.post((body ?: "{}").toRequestBody(jsonType))
                    else -> throw IllegalArgumentException("Unbekannte Methode $method")
                }
            }

            client.newCall(builder.build()).execute().use { response ->
                val nonce = response.header("X-Opx-Nonce")

                // Ohne Zufallswert kam die Antwort im Klartext – so antwortet
                // der Server auf Fehler, damit die Begründung auch dann lesbar
                // ist, wenn gerade nicht entschlüsselt werden kann.
                val text = if (f != null && nonce != null) {
                    val roh = response.body?.bytes() ?: ByteArray(0)
                    val klar = runCatching { f.oeffnen(roh, Funk.unb64(nonce), zusatz) }
                        .getOrNull()

                    // Lässt sich die Antwort nicht öffnen, passt der Schlüssel
                    // nicht mehr zu diesem Server – neu aufgesetzt, oder es ist
                    // nicht mehr derselbe. Ohne dieses Wegwerfen bliebe die App
                    // bis zum Neustart stumm: Sie redete weiter mit einem
                    // Schlüssel, den das Gegenüber nicht kennt.
                    if (klar == null) {
                        funkZuruecksetzen()
                        throw ApiException(
                            response.code,
                            "Die Antwort des Servers ließ sich nicht entschlüsseln. " +
                                "Der Schlüssel wird neu ausgehandelt.",
                        )
                    }
                    String(klar)
                } else {
                    response.body?.string().orEmpty()
                }

                if (!response.isSuccessful) {
                    throw ApiException(response.code, extractMessage(text, response.code))
                }
                text
            }
        }

    /** Holt die Klartextmeldung des Servers aus der Fehlerantwort. */
    private fun extractMessage(text: String, code: Int): String {
        val fallback = when (code) {
            401 -> "Anmeldung abgelaufen."
            403 -> "Dafür fehlt die Berechtigung."
            404 -> "Nicht gefunden."
            else -> "Server antwortete mit $code."
        }

        return runCatching {
            val obj = json.parseToJsonElement(text) as? JsonObject ?: return fallback
            (obj["message"] as? JsonPrimitive)?.content ?: fallback
        }.getOrElse { fallback }
    }
}
