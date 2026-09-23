package de.operationx.app.data

import android.content.Context
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.longPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.serialization.Serializable
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.json.Json

private val Context.dataStore by preferencesDataStore("opx")

/**
 * Serverprofil und Anmeldung auf dem Gerät.
 *
 * Alles hier hat eine Verfallszeit. Nach Spielende plus Aufbewahrungsfrist
 * verschwinden Zugangsdaten und Standortpuffer von selbst – das ist die Zusage
 * aus dem Konzept, und sie soll nicht davon abhängen, dass jemand daran denkt.
 */
class Session(private val context: Context) {

    private object Keys {
        val serverUrl = stringPreferencesKey("server_url")
        val token = stringPreferencesKey("token")
        val callsign = stringPreferencesKey("callsign")
        val role = stringPreferencesKey("role")
        val display = stringPreferencesKey("display")
        val expiresAt = longPreferencesKey("expires_at")
        val fieldMode = booleanPreferencesKey("field_mode")
        val tracking = booleanPreferencesKey("tracking")
        val server = stringPreferencesKey("bekannte_server")
    }

    val serverUrl = context.dataStore.data.map { it[Keys.serverUrl] ?: "" }
    val fieldMode = context.dataStore.data.map { it[Keys.fieldMode] ?: false }
    val trackingWanted = context.dataStore.data.map { it[Keys.tracking] ?: false }

    suspend fun currentServer(): String = serverUrl.first()

    /** Liefert das Zugangstoken – oder null, wenn es abgelaufen ist. */
    suspend fun token(): String? {
        val prefs = context.dataStore.data.first()
        val token = prefs[Keys.token] ?: return null
        val expires = prefs[Keys.expiresAt] ?: 0L

        if (expires in 1..<System.currentTimeMillis()) {
            clearCredentials()
            return null
        }
        return token.ifBlank { null }
    }

    suspend fun profile(): Triple<String, String, String> {
        val prefs = context.dataStore.data.first()
        return Triple(
            prefs[Keys.callsign] ?: "",
            prefs[Keys.display] ?: "",
            prefs[Keys.role] ?: "",
        )
    }

    suspend fun setServer(url: String) {
        context.dataStore.edit { it[Keys.serverUrl] = url.trimEnd('/') }
    }

    /**
     * Speichert die Anmeldung mit Verfallszeit.
     *
     * [validHours] rechnet Spieldauer plus Aufbewahrungsfrist zusammen; nach
     * Ablauf löscht sich der Zugang beim nächsten Start.
     */
    suspend fun setCredentials(token: String, me: Me, validHours: Long = 30) {
        context.dataStore.edit {
            it[Keys.token] = token
            it[Keys.callsign] = me.callsign
            it[Keys.display] = me.display
            it[Keys.role] = me.role
            it[Keys.expiresAt] = System.currentTimeMillis() + validHours * 3_600_000
        }
    }

    suspend fun clearCredentials() {
        context.dataStore.edit {
            it.remove(Keys.token)
            it.remove(Keys.callsign)
            it.remove(Keys.display)
            it.remove(Keys.role)
            it.remove(Keys.expiresAt)
        }
    }

    /**
     * Zurück zur Serverauswahl, ohne das Gedächtnis zu leeren.
     *
     * Der Unterschied zu wipe() ist die Merkliste der Server: Sie ist der
     * einzige Grund, warum die App merkt, wenn ein Server unter derselben
     * Adresse plötzlich ein anderes Kennzeichen hat. Sie ausgerechnet beim
     * Serverwechsel zu löschen hiesse, die Warnung genau dann wegzuwerfen,
     * wenn sie gebraucht wird – und die Liste wäre auf der Beitrittsseite
     * immer leer.
     */
    suspend fun serverWechseln() {
        clearCredentials()
        context.dataStore.edit { it.remove(Keys.serverUrl) }
        PositionBuffer(context).clear()
    }

    /** Löscht alles, was zum Spiel gehört – der „Jetzt alles löschen“-Knopf. */
    suspend fun wipe() {
        context.dataStore.edit { it.clear() }
        PositionBuffer(context).clear()
    }

    suspend fun setFieldMode(on: Boolean) {
        context.dataStore.edit { it[Keys.fieldMode] = on }
    }

    suspend fun setTrackingWanted(on: Boolean) {
        context.dataStore.edit { it[Keys.tracking] = on }
    }

    // --- Server, auf denen schon gespielt wurde ---------------------------

    /**
     * Wer mit dieser App auf einem Server gespielt hat, spielt nächstes Mal
     * vielleicht auf einem anderen: andere Gruppe, anderer Laptop, anderer
     * Schlüssel. Deshalb merkt sich die App das Kennzeichen zu jeder Adresse.
     *
     * Der Nutzen ist nicht die Bequemlichkeit, sondern die Warnung: Meldet
     * sich unter derselben Adresse plötzlich ein anderes Kennzeichen, ist
     * entweder der Server neu aufgesetzt worden – oder es ist nicht mehr
     * derselbe. Beides gehört gesagt, bevor jemand sein Kennwort eintippt.
     */
    suspend fun bekannteServer(): List<BekannterServer> {
        val roh = context.dataStore.data.first()[Keys.server] ?: return emptyList()
        return runCatching { nachsichtig.decodeFromString(ListSerializer(BekannterServer.serializer()), roh) }
            .getOrDefault(emptyList())
            .sortedByDescending { it.zuletzt }
    }

    suspend fun kennzeichenVon(adresse: String): String? =
        bekannteServer().firstOrNull { it.adresse == adresse.trimEnd('/') }
            ?.kennzeichen
            ?.ifBlank { null }

    /** Hält Adresse und Kennzeichen fest – oder zieht das Kennzeichen nach. */
    suspend fun merkeServer(adresse: String, kennzeichen: String?) {
        val sauber = adresse.trimEnd('/')
        val liste = bekannteServer().filter { it.adresse != sauber }

        val neu = BekannterServer(
            adresse = sauber,
            kennzeichen = kennzeichen.orEmpty(),
            zuletzt = System.currentTimeMillis(),
        )

        // Acht reichen: Es ist eine Liste zum Wiederfinden, kein Archiv.
        val gekuerzt = (listOf(neu) + liste).take(8)
        context.dataStore.edit { it[Keys.server] = nachsichtig.encodeToString(ListSerializer(BekannterServer.serializer()), gekuerzt) }
    }

    suspend fun vergissServer(adresse: String) {
        val sauber = adresse.trimEnd('/')
        val liste = bekannteServer().filter { it.adresse != sauber }
        context.dataStore.edit { it[Keys.server] = nachsichtig.encodeToString(ListSerializer(BekannterServer.serializer()), liste) }
    }

    private val nachsichtig = Json { ignoreUnknownKeys = true }
}

/** Ein Server, auf dem schon gespielt wurde. */
@Serializable
data class BekannterServer(
    val adresse: String,
    val kennzeichen: String = "",
    val zuletzt: Long = 0,
)
