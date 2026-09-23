package de.operationx.app.data

import android.content.Context
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import java.io.File

/**
 * Zwischenspeicher für Standortmeldungen.
 *
 * Der wichtigste Teil der App im Feld: In der Bahn, im Hinterhof, im Funkloch
 * kommt keine Meldung durch. Sie wird deshalb hier abgelegt und beim nächsten
 * Netzkontakt nachgereicht – mit ihrem ursprünglichen Zeitstempel, denn der
 * Server bewertet die Frist nach dem Erfassungszeitpunkt. Ohne diesen Puffer
 * bekäme jemand eine Strafe dafür, dass die Straßenbahn keinen Empfang hat.
 */
class PositionBuffer(context: Context) {

    private val file = File(context.filesDir, "positions.json")
    private val json = Json { ignoreUnknownKeys = true }

    companion object {
        /** Mehr als das speichert kein Gerät sinnvoll zwischen. */
        private const val MAX_ENTRIES = 500
        private val lock = Mutex()
    }

    suspend fun add(report: PositionReport) = withContext(Dispatchers.IO) {
        lock.withLock {
            val list = readUnlocked().toMutableList()
            list.add(report)
            writeUnlocked(list.takeLast(MAX_ENTRIES))
        }
    }

    suspend fun peek(): List<PositionReport> = withContext(Dispatchers.IO) {
        lock.withLock { readUnlocked() }
    }

    /**
     * Entfernt die übertragenen Meldungen.
     *
     * Bewusst erst nach erfolgreicher Übertragung und nur so viele, wie
     * tatsächlich angekommen sind: Bricht die Verbindung mittendrin ab, bleibt
     * der Rest erhalten.
     */
    suspend fun drop(count: Int) = withContext(Dispatchers.IO) {
        lock.withLock {
            val list = readUnlocked()
            writeUnlocked(if (count >= list.size) emptyList() else list.drop(count))
        }
    }

    suspend fun size(): Int = peek().size

    suspend fun clear() = withContext(Dispatchers.IO) {
        lock.withLock { writeUnlocked(emptyList()) }
    }

    private fun readUnlocked(): List<PositionReport> {
        if (!file.exists()) return emptyList()
        return runCatching {
            json.decodeFromString<List<PositionReport>>(file.readText())
        }.getOrElse { emptyList() }
    }

    private fun writeUnlocked(list: List<PositionReport>) {
        runCatching { file.writeText(json.encodeToString(list)) }
    }
}
