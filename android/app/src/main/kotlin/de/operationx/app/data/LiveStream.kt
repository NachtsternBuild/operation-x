package de.operationx.app.data

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.flow.flowOn
import kotlinx.coroutines.isActive
import kotlinx.serialization.json.Json
import okhttp3.OkHttpClient
import okhttp3.Request
import java.io.BufferedReader
import java.util.concurrent.TimeUnit

/**
 * Der Lagestrom.
 *
 * Vorher fragte die App alle fünf Sekunden nach, ob es etwas Neues gibt. Über
 * sechs Stunden sind das mehr als viertausend Anfragen je Gerät – über
 * Mobilfunk, mit dem Funkgerät als größtem Stromverbraucher nach dem
 * Bildschirm, an einem Tag, an dem niemand eine Steckdose findet.
 *
 * Jetzt bleibt eine Verbindung offen, und der Server schickt, wenn es etwas zu
 * schicken gibt. Zwischen zwei Sendungen schweigt die Leitung bis auf ein
 * Lebenszeichen alle fünfzehn Sekunden.
 *
 * Eigener Client mit eigenen Zeitgrenzen: Der gewöhnliche hat eine Lesefrist
 * von wenigen Sekunden, und die wäre hier genau falsch – Schweigen ist der
 * Normalfall, nicht der Fehler.
 */
class LiveStream(private val session: Session, private val api: Api) {

    private val client = OkHttpClient.Builder()
        .connectTimeout(15, TimeUnit.SECONDS)
        // Kein Lesezeitlimit: Der Server schweigt planmäßig. Bemerkt wird ein
        // Abriss über das ausbleibende Lebenszeichen, nicht über eine Frist.
        .readTimeout(0, TimeUnit.MILLISECONDS)
        .retryOnConnectionFailure(true)
        .build()

    private val json = Json { ignoreUnknownKeys = true }

    /**
     * Die gerade offene Verbindung.
     *
     * Sie wird gebraucht, um den Strom bei einem Wechsel der Anmeldung zu
     * kappen: Ein Lesevorgang auf einer offenen Verbindung lässt sich nicht
     * einfach abbrechen, er hängt, bis etwas hereinkommt. Ohne dieses Kappen
     * liefe nach einer Abmeldung der alte Strom weiter – und wenn danach ein
     * anderes Team dasselbe Telefon benutzt, bekäme es die Lage seines
     * Vorgängers auf die Karte.
     */
    @Volatile
    private var current: okhttp3.Call? = null

    /** Trennt den Strom, damit er sich mit den neuen Zugangsdaten neu aufbaut. */
    fun reset() {
        runCatching { current?.cancel() }
        current = null
    }

    /**
     * Liefert jede eintreffende Lage.
     *
     * Bricht die Verbindung ab, wird sie neu aufgebaut – mit wachsender Pause,
     * damit ein Funkloch nicht zum Akkufresser wird. Der Fluss endet erst, wenn
     * der Sammler ihn beendet.
     */
    fun connect(): Flow<LiveState> = callbackFlow {
        var delayMs = 1_000L

        while (isActive) {
            val base = session.currentServer()
            val token = session.token()

            if (base.isBlank() || token == null) {
                delay(2_000)
                continue
            }

            try {
                // Derselbe Sitzungsschlüssel wie für die übrigen Anfragen. Der
                // Strom trägt die Positionen aller sichtbaren Teams und ist
                // damit der heikelste Inhalt überhaupt.
                val funk = api.funkFuerStrom(base)

                val builder = Request.Builder()
                    .url("$base/api/opx/stream")
                    .header("Authorization", token)
                    .header("Accept", "text/event-stream")
                    .get()

                if (funk != null) {
                    builder.header("X-Opx-Key", funk.publicKey)
                    builder.header("X-Opx-Seq", funk.naechsteNummer().toString())
                }

                val request = builder.build()

                val call = client.newCall(request)
                current = call
                call.execute().use { response ->
                    if (!response.isSuccessful) {
                        throw ApiException(response.code, "Strom abgelehnt (${response.code})")
                    }
                    delayMs = 1_000

                    val reader = response.body?.charStream()?.buffered()
                        ?: throw ApiException(0, "Leere Antwort")

                    readEvents(reader, funk) { state -> trySend(state) }
                }
            } catch (_: Throwable) {
                // Jede Störung führt zum Neuaufbau. Welche es war, hilft hier
                // niemandem – die Anzeige hängt am zuletzt bekannten Stand,
                // und der bleibt stehen.
            }

            if (!isActive) break
            delay(delayMs)
            delayMs = (delayMs * 2).coerceAtMost(15_000)
        }

        awaitClose { reset() }
    }.flowOn(Dispatchers.IO)

    /**
     * Liest das Format: Blöcke, getrennt durch eine Leerzeile, darin Zeilen der
     * Form `feld: wert`. Zeilen mit führendem Doppelpunkt sind Kommentare – der
     * Server nutzt sie als Lebenszeichen.
     */
    private inline fun readEvents(
        reader: BufferedReader,
        funk: Funk?,
        onLive: (LiveState) -> Unit,
    ) {
        var event = "message"
        val data = StringBuilder()

        while (true) {
            val line = reader.readLine() ?: return

            when {
                line.isEmpty() -> {
                    if (event == "live" && data.isNotEmpty()) {
                        runCatching {
                            val text = funk?.oeffneStromSendung(data.toString()) ?: data.toString()
                            json.decodeFromString<LiveState>(text)
                        }.onSuccess(onLive)
                    }
                    if (event == "bye") return
                    event = "message"
                    data.setLength(0)
                }
                line.startsWith(":") -> Unit
                line.startsWith("event:") -> event = line.removePrefix("event:").trim()
                line.startsWith("data:") -> data.append(line.removePrefix("data:").trim())
            }
        }
    }
}
