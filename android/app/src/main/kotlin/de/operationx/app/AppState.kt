package de.operationx.app

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import de.operationx.app.data.Adressen
import de.operationx.app.data.Api
import de.operationx.app.data.BekannterServer
import de.operationx.app.data.ApiException
import de.operationx.app.data.ArrestResult
import de.operationx.app.data.DelayResult
import de.operationx.app.data.FieldMap
import de.operationx.app.data.IntelItem
import de.operationx.app.data.Joker
import de.operationx.app.data.ServerStatus
import de.operationx.app.data.LedgerBook
import de.operationx.app.data.LiveState
import de.operationx.app.data.LiveStream
import de.operationx.app.data.Me
import de.operationx.app.data.MissionState
import de.operationx.app.data.Puzzle
import de.operationx.app.data.RadioMessage
import de.operationx.app.data.RuleGroup
import de.operationx.app.data.SightingResult
import de.operationx.app.data.PositionBuffer
import de.operationx.app.data.Session
import de.operationx.app.location.LocationService
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/** Wo die App gerade steht. */
enum class Stage { LOADING, JOIN, LOGIN, ONBOARDING, FIELD }

data class UiState(
    val stage: Stage = Stage.LOADING,
    val serverUrl: String = "",
    val serverName: String = "",
    val me: Me? = null,
    val live: LiveState? = null,
    val intel: List<IntelItem> = emptyList(),
    val field: FieldMap? = null,
    val mission: MissionState? = null,
    val jokers: List<Joker> = emptyList(),
    val puzzles: List<Puzzle> = emptyList(),
    val radio: List<RadioMessage> = emptyList(),
    val sighting: SightingResult? = null,
    val arrest: ArrestResult? = null,
    val delay: DelayResult? = null,
    val rules: List<RuleGroup> = emptyList(),
    /** Das eigene Punktekonto, sobald es geholt wurde. */
    val ledger: LedgerBook? = null,
    val buffered: Int = 0,
    val tracking: Boolean = false,
    /** Eine angeforderte Sofortmeldung läuft gerade. */
    val reporting: Boolean = false,
    val fieldMode: Boolean = false,
    val busy: Boolean = false,
    /** Ob die Lage gerade über den offenen Strom kommt. */
    val streaming: Boolean = false,
    /** Kennzeichen des Servers, wenn die zweite Verschlüsselung steht. */
    val fingerprint: String? = null,
    /**
     * Gesetzt, wenn sich unter einer bekannten Adresse ein anderes
     * Kennzeichen meldet als beim letzten Mal. Kein Fehler, aber eine Frage,
     * die vor dem Eintippen des Kennworts beantwortet gehört.
     */
    val kennzeichenWarnung: String? = null,
    /** Server, auf denen dieses Gerät schon gespielt hat. */
    val bekannteServer: List<BekannterServer> = emptyList(),
    val error: String? = null,
    val notice: String? = null,
)

class AppState(app: Application) : AndroidViewModel(app) {

    private val session = Session(app)
    private val buffer = PositionBuffer(app)
    val api = Api(session)
    private val liveStream = LiveStream(session, api)

    private val _ui = MutableStateFlow(UiState())
    val ui = _ui.asStateFlow()

    init {
        viewModelScope.launch { restore() }
        viewModelScope.launch { streamLoop() }
        viewModelScope.launch { slowLoop() }
    }

    /** Beim Start: Gibt es ein Serverprofil, gibt es eine gültige Anmeldung? */
    private suspend fun restore() {
        val server = session.currentServer()
        val fieldMode = session.fieldMode.first()
        val wantsTracking = session.trackingWanted.first()

        if (server.isBlank()) {
            _ui.value = _ui.value.copy(stage = Stage.JOIN, fieldMode = fieldMode)
            return
        }

        val token = session.token()
        if (token == null) {
            _ui.value = _ui.value.copy(
                stage = Stage.LOGIN, serverUrl = server, fieldMode = fieldMode,
            )
            checkServer(server)
            return
        }

        runCatching { api.me() }
            .onSuccess { me ->
                _ui.value = _ui.value.copy(
                    stage = stageFor(me),
                    serverUrl = server,
                    me = me,
                    fieldMode = fieldMode,
                    tracking = wantsTracking,
                )
                refresh()
            }
            .onFailure { err ->
                // Nur ein abgelehnter Zugang führt zurück zur Anmeldung.
                //
                // Vorher tat das jeder Fehlschlag, also auch der im Funkloch:
                // Wer die App im Keller, im Tunnel oder bei schlechtem Empfang
                // öffnete, war abgemeldet und musste das Kennwort von der
                // Teamkarte neu eintippen – ausgerechnet an einer Stelle ohne
                // Netz. Der Zugang bleibt deshalb liegen, bis der Server ihn
                // selbst zurückweist.
                val abgelehnt = err is ApiException && (err.status == 401 || err.status == 403)

                if (abgelehnt) {
                    session.clearCredentials()
                    _ui.value = _ui.value.copy(
                        stage = Stage.LOGIN, serverUrl = server, fieldMode = fieldMode,
                    )
                    // Ein zurückgewiesener Zugang ist der erste Hinweis darauf,
                    // dass hinter der Adresse etwas anderes steht als vorhin.
                    // Deshalb wird der Server hier neu geprüft – sonst tippt
                    // jemand sein Kennwort in einen Bildschirm ohne Kennzeichen.
                    checkServer(server)
                    return@onFailure
                }

                // Ohne Netz wird aus dem gespeicherten Profil weitergearbeitet:
                // Rufzeichen und Rolle stehen auf dem Gerät, alles andere holt
                // der nächste Umlauf nach, sobald wieder Empfang ist.
                val (callsign, display, role) = session.profile()
                _ui.value = _ui.value.copy(
                    stage = Stage.FIELD,
                    serverUrl = server,
                    me = Me(callsign = callsign, display = display, role = role, onboarded = true),
                    fieldMode = fieldMode,
                    tracking = wantsTracking,
                    error = "Kein Kontakt zum Server – die Anzeige füllt sich, sobald wieder Empfang ist.",
                )
            }
    }

    /**
     * Hört auf den Lagestrom.
     *
     * Die Lage kommt jetzt, wenn sie sich ändert, statt alle zehn Sekunden auf
     * Verdacht. Was nicht im Strom steckt – Rätsel, Hinweise, Funk, Missionen –
     * wird bei jeder Sendung nachgeholt: Wenn sich an der Lage etwas getan hat,
     * hat sich meist auch daneben etwas getan.
     */
    private suspend fun streamLoop() {
        liveStream.connect().collect { live: LiveState ->
            val vorher = _ui.value.live
            _ui.value = _ui.value.copy(live = live, streaming = true)

            // Nicht bei jeder Sendung alles nachladen: Bewegt sich nur ein
            // Punkt auf der Karte, hat sich an den Rätseln nichts geändert.
            if (vorher == null || live.status != vorher.status ||
                live.alerts.size != vorher.alerts.size ||
                live.self.fp != vorher.self.fp ||
                live.self.points != vorher.self.points
            ) {
                refreshSideData()
            }
        }
    }

    /**
     * Die Rückfallebene.
     *
     * Steht der Strom nicht – Funkloch, Zwischenstelle, alter Server –, darf
     * die Anzeige nicht einfrieren. Eine Minute ist selten genug, dass es am
     * Akku nicht auffällt, und oft genug, dass niemand ins Leere schaut.
     */
    private suspend fun slowLoop() {
        while (true) {
            delay(60_000)
            if (_ui.value.stage == Stage.FIELD) refresh()
        }
    }

    /** Lädt alles, was nicht im Lagestrom steckt. */
    private suspend fun refreshSideData() {
        val role = _ui.value.me?.role ?: return
        val intel = runCatching { api.intel().intel }.getOrElse { emptyList() }
        val jokers = runCatching { api.jokers().jokers }.getOrElse { emptyList() }
        val radio = runCatching { api.radio().messages }.getOrElse { emptyList() }
        val mission = if (role == "misterx") runCatching { api.mission() }.getOrNull()
        else _ui.value.mission
        val puzzles = if (role == "detective") {
            runCatching { api.puzzles().puzzles }.getOrElse { emptyList() }
        } else emptyList()

        _ui.value = _ui.value.copy(
            intel = intel, jokers = jokers, radio = radio,
            mission = mission, puzzles = puzzles,
            buffered = buffer.size(),
        )
    }

    /**
     * Die Zeile unter der Ueberschrift auf der Anmeldeseite.
     *
     * Nennt der Server kein Spiel, steht hier ein neutraler Satz statt eines
     * einsamen Trennpunkts. Das kommt vor: Ein Server, der noch nicht
     * eingerichtet ist, nennt keines – und einer, der mehrere fuehrt, nennt
     * mit Absicht keines, weil die Namen niemanden etwas angehen, der noch
     * nichts vorgewiesen hat.
     */
    private fun serverNameVon(status: ServerStatus): String {
        val name = status.gameName.trim()
        val stadt = status.city.trim()
        return when {
            name.isNotEmpty() && stadt.isNotEmpty() -> "$name · $stadt"
            name.isNotEmpty() -> name
            else -> "Rufzeichen von der Teamkarte"
        }
    }

    fun checkServer(url: String) = viewModelScope.launch {
        // Anderer Server, anderer Schlüssel.
        api.funkZuruecksetzen()

        val kandidaten = Adressen.kandidaten(url)
        if (kandidaten.isEmpty()) return@launch

        _ui.value = _ui.value.copy(busy = true, error = null)

        for ((nummer, adresse) in kandidaten.withIndex()) {
            if (!Adressen.erlaubt(adresse)) {
                _ui.value = _ui.value.copy(
                    busy = false,
                    error = "Unverschlüsselt geht nur im eigenen Netz. " +
                        "Für einen Server im Internet muss die Adresse mit https:// beginnen.",
                )
                return@launch
            }

            val status = runCatching { api.status(adresse) }.getOrNull()

            if (status == null) {
                // Der letzte Versuch entscheidet über die Meldung; vorher wird
                // nur die andere Möglichkeit probiert.
                if (nummer == kandidaten.lastIndex) {
                    _ui.value = _ui.value.copy(
                        busy = false,
                        error = "Kein Kontakt zu ${Adressen.host(adresse)}. Adresse prüfen.",
                    )
                }
                continue
            }

            if (!status.ready) {
                _ui.value = _ui.value.copy(
                    busy = false,
                    error = "Auf diesem Server ist noch kein Spiel eingerichtet.",
                )
                return@launch
            }

            session.setServer(adresse)

            // Den Schlüssel gleich holen, nicht erst bei der ersten Anfrage:
            // Das Kennzeichen soll auf der Anmeldeseite stehen, damit es sich
            // mit der Teamkarte vergleichen lässt, bevor jemand sein Kennwort
            // eintippt.
            val kennzeichen = api.kennzeichenVon(adresse)
            val bekannt = session.kennzeichenVon(adresse)

            val warnung = when {
                kennzeichen == null || bekannt == null -> null
                kennzeichen == bekannt -> null
                else ->
                    "Achtung: Dieser Server meldet sich mit einem anderen " +
                        "Kennzeichen als beim letzten Mal ($kennzeichen statt " +
                        "$bekannt). Entweder wurde er neu aufgesetzt — oder " +
                        "es ist nicht mehr derselbe. Vergleicht die Zeichen mit " +
                        "der Teamkarte."
            }

            _ui.value = _ui.value.copy(
                busy = false,
                stage = Stage.LOGIN,
                serverUrl = adresse,
                fingerprint = kennzeichen,
                kennzeichenWarnung = warnung,
                bekannteServer = session.bekannteServer(),
                serverName = serverNameVon(status),
                error = null,
            )
            return@launch
        }
    }

    fun login(callsign: String, password: String) = viewModelScope.launch {
        _ui.value = _ui.value.copy(busy = true, error = null)

        val server = session.currentServer()
        runCatching { api.login(server, callsign.trim(), password) }
            .onSuccess { (token, me) ->
                session.setCredentials(token, me)
                // Erst jetzt gilt der Server als bekannt: Wer sich hier
                // anmeldet, hat die Zeichen entweder verglichen oder die
                // Warnung bewusst weggeklickt.
                session.merkeServer(server, api.fingerprint)
                liveStream.reset()
                // Die Serveradresse gehört ab hier in den Zustand: Die Karte
                // holt ihre Kacheln vom Spielserver und braucht dafür die
                // vollständige Adresse.
                _ui.value = _ui.value.copy(
                    busy = false, stage = stageFor(me), me = me, serverUrl = server,
                )
                refresh()
            }
            .onFailure { err ->
                val message = when {
                    err is ApiException && err.status == 400 -> "Rufzeichen oder Kennwort stimmen nicht."
                    err is ApiException -> err.message ?: "Anmeldung fehlgeschlagen."
                    else -> "Kein Kontakt zum Server."
                }
                _ui.value = _ui.value.copy(busy = false, error = message)
            }
    }

    fun logout() = viewModelScope.launch {
        session.clearCredentials()
        liveStream.reset()
        _ui.value = UiState(stage = Stage.LOGIN, serverUrl = session.currentServer())
        checkServer(session.currentServer())
    }

    /** Löscht Zugang, Puffer und Serverprofil – der Knopf aus den Einstellungen. */
    fun wipe() = viewModelScope.launch {
        session.wipe()
        liveStream.reset()
        // Anderer Server, anderer Schlüssel.
        api.funkZuruecksetzen()
        _ui.value = UiState(stage = Stage.JOIN)
    }

    fun refresh() = viewModelScope.launch {
        runCatching {
            val live = api.live()
            val kennzeichen = api.fingerprint
            val me = runCatching { api.me() }.getOrNull()
            val role = me?.role ?: _ui.value.me?.role

            // Was geladen wird, hängt an der Rolle: Ein Fahndungsteam bekommt
            // auf /mission ohnehin nur eine Absage, und die Zielperson hat
            // keinen Zugang zu den Rätseln.
            val intel = runCatching { api.intel().intel }.getOrElse { emptyList() }
            val jokers = runCatching { api.jokers().jokers }.getOrElse { emptyList() }
            val radio = runCatching { api.radio().messages }.getOrElse { emptyList() }
            val field = _ui.value.field ?: runCatching { api.map() }.getOrNull()

            val mission = if (role == "misterx") {
                runCatching { api.mission() }.getOrNull()
            } else null

            val puzzles = if (role == "detective") {
                runCatching { api.puzzles().puzzles }.getOrElse { emptyList() }
            } else emptyList()

            // Die Antwort auf eine gemeldete Verzögerung gehört zu genau
            // einem Zwischenziel. Ohne das hier stünde "10 Minuten mehr" noch
            // beim übernächsten Auftrag auf dem Schirm.
            val delay = if (mission?.id != _ui.value.mission?.id) null else _ui.value.delay

            // Die Regelwerte gehören von Anfang an dazu: Auf ihnen stehen die
            // Preise, die im Zugriffsfenster genannt werden. Ohne sie stünden
            // dort feste Zahlen, die mit dem Regelpult der Zentrale nichts mehr
            // zu tun haben.
            if (_ui.value.rules.isEmpty()) loadRules()

            _ui.value = _ui.value.copy(
                live = live,
                intel = intel,
                field = field,
                mission = mission,
                delay = delay,
                jokers = jokers,
                puzzles = puzzles,
                radio = radio,
                me = me ?: _ui.value.me,
                buffered = buffer.size(),
                fingerprint = kennzeichen,
                error = null,
            )
        }.onFailure { err ->
            if (err is ApiException && err.status == 401) {
                session.clearCredentials()
                _ui.value = _ui.value.copy(stage = Stage.LOGIN)
                checkServer(session.currentServer())
            } else {
                _ui.value = _ui.value.copy(buffered = buffer.size())
            }
        }
    }

    /**
     * Jetzt melden.
     *
     * Der Knopf hieß immer "Standort melden" und hat die Lage vom Server
     * geholt – gemeldet hat er nichts. Wer ihn kurz vor Fristende gedrückt hat,
     * bekam trotzdem seinen Verstoß, und die App gab ihm keinen Hinweis darauf.
     *
     * Jetzt fordert er beim Standortdienst eine Ortung an, die sofort
     * übertragen wird. Bis die neue Frist zurückkommt, bleibt der Knopf
     * beschäftigt: Ortung, Funknetz und Antwort brauchen zusammen ein paar
     * Sekunden, und in dieser Zeit soll niemand ein zweites Mal drücken.
     */
    fun reportNow() = viewModelScope.launch {
        val vorher = _ui.value.live?.self?.nextDueAt
        _ui.value = _ui.value.copy(reporting = true)

        LocationService.pingNow(getApplication())

        for (versuch in 1..6) {
            delay(2_000)
            refresh().join()
            if (_ui.value.live?.self?.nextDueAt != vorher) break
        }

        _ui.value = _ui.value.copy(reporting = false)
    }

    fun setTracking(on: Boolean) = viewModelScope.launch {
        session.setTrackingWanted(on)
        _ui.value = _ui.value.copy(tracking = on)
    }

    fun setFieldMode(on: Boolean) = viewModelScope.launch {
        session.setFieldMode(on)
        _ui.value = _ui.value.copy(fieldMode = on)
    }

    fun toggleTransit() = viewModelScope.launch {
        val target = !(_ui.value.live?.self?.inTransit ?: false)
        runCatching { api.setTransit(target) }.onSuccess { refresh() }
    }

    fun chooseOption(optionId: String) = viewModelScope.launch {
        runCatching { api.chooseOption(optionId) }
            .onSuccess { refresh() }
            .onFailure { _ui.value = _ui.value.copy(error = it.message) }
    }

    fun submitPasscode(code: String) = viewModelScope.launch {
        runCatching { api.submitPasscode(code) }
            .onSuccess {
                _ui.value = _ui.value.copy(notice = "Zwischenziel bestätigt.")
                refresh()
            }
            .onFailure { _ui.value = _ui.value.copy(error = it.message) }
    }

    fun solvePuzzle(id: String, answer: String) = viewModelScope.launch {
        runCatching { api.solvePuzzle(id, answer) }
            .onSuccess { result ->
                _ui.value = _ui.value.copy(notice = result.message)
                refresh()
            }
            .onFailure { _ui.value = _ui.value.copy(error = it.message) }
    }

    fun useJoker(kind: String, sectorId: String? = null, hotspotId: String? = null) =
        viewModelScope.launch {
            runCatching { api.useJoker(kind, sectorId, hotspotId) }
                .onSuccess { result ->
                    val message = (result["message"] as? kotlinx.serialization.json.JsonPrimitive)?.content
                    _ui.value = _ui.value.copy(notice = message ?: "Eingesetzt.")
                    refresh()
                }
                .onFailure { _ui.value = _ui.value.copy(error = it.message) }
        }

    fun reportSighting() = viewModelScope.launch {
        _ui.value = _ui.value.copy(busy = true)
        runCatching { api.reportSighting() }
            .onSuccess { result ->
                _ui.value = _ui.value.copy(
                    sighting = result,
                    notice = result.message.ifBlank { "Gemeldet." },
                    busy = false,
                )
                refresh()
            }
            .onFailure { _ui.value = _ui.value.copy(error = it.message, busy = false) }
    }

    /**
     * Der Zugriff.
     *
     * Das Ergebnis bleibt im Zustand stehen, statt nur als kurze Meldung
     * aufzublitzen: Wer Stufe 3 ausgelöst hat, will nachlesen können, was
     * herauskam – und zwar auch dann, wenn er in dem Moment nicht hingesehen hat.
     */
    fun arrest(level: Int, hotspot: Int, timeIso: String?, target: Int?) =
        viewModelScope.launch {
            _ui.value = _ui.value.copy(busy = true, arrest = null)
            runCatching { api.arrest(level, hotspot, timeIso, target) }
                .onSuccess { result ->
                    _ui.value = _ui.value.copy(
                        arrest = result,
                        notice = result.message.ifBlank {
                            if (result.correct) "Trifft zu." else "Trifft nicht zu."
                        },
                        busy = false,
                    )
                    refresh()
                }
                .onFailure { _ui.value = _ui.value.copy(error = it.message, busy = false) }
        }

    /**
     * Beweisfoto hochladen und danach löschen.
     *
     * Gelöscht wird in jedem Fall, auch bei einem Fehlschlag: Die Datei liegt
     * im Zwischenspeicher der App, und ein Beweisfoto ist nichts, was nach dem
     * Spiel noch irgendwo herumliegen soll.
     */
    fun uploadEvidence(photo: java.io.File) = viewModelScope.launch {
        _ui.value = _ui.value.copy(busy = true)
        runCatching { api.uploadEvidence(photo) }
            .onSuccess {
                _ui.value = _ui.value.copy(
                    busy = false,
                    notice = "Foto eingereicht. Die Zentrale sieht es sich an.",
                )
                refresh()
            }
            .onFailure { _ui.value = _ui.value.copy(busy = false, error = it.message) }
        runCatching { photo.delete() }
    }

    /**
     * Verzögerung melden.
     *
     * Das Ergebnis bleibt stehen, statt kurz aufzublitzen: Wer wissen will, ob
     * die Zeit gewährt wurde, schaut oft erst hin, wenn die Hände wieder frei
     * sind.
     */
    fun reportDelay(reason: String) = viewModelScope.launch {
        _ui.value = _ui.value.copy(busy = true)
        runCatching { api.reportDelay(reason) }
            .onSuccess { result ->
                _ui.value = _ui.value.copy(
                    busy = false,
                    delay = result,
                    notice = result.message,
                )
                refresh()
            }
            .onFailure { _ui.value = _ui.value.copy(busy = false, error = it.message) }
    }

    /**
     * Das Punktekonto holen.
     *
     * Jedes Mal frisch, anders als das Regelverzeichnis: Es ändert sich
     * ständig, und wer es öffnet, tut das gerade wegen der letzten Buchung.
     */
    fun loadLedger() = viewModelScope.launch {
        runCatching { api.ledger() }
            .onSuccess { _ui.value = _ui.value.copy(ledger = it) }
            .onFailure { _ui.value = _ui.value.copy(error = it.message) }
    }

    /** Regelnachschlag. Einmal geholt, dann im Zustand behalten. */
    fun loadRules() = viewModelScope.launch {
        if (_ui.value.rules.isNotEmpty()) return@launch
        runCatching { api.rules() }
            .onSuccess { _ui.value = _ui.value.copy(rules = it.groups) }
            .onFailure { _ui.value = _ui.value.copy(error = it.message) }
    }

    fun postRadio(text: String) = viewModelScope.launch {
        runCatching { api.postRadio(text) }
            .onSuccess { refresh() }
            .onFailure { _ui.value = _ui.value.copy(error = it.message) }
    }

    /**
     * Die Einweisung abschließen.
     *
     * Der Serveraufruf darf scheitern, ohne dass jemand hängen bleibt: Die
     * Einweisung ist gelaufen, ob die Zentrale es nun in ihrer Liste sieht oder
     * nicht. Sie am Straßenrand zu wiederholen wäre die schlechtere Antwort.
     */
    fun finishOnboarding() = viewModelScope.launch {
        runCatching { api.markOnboarded() }
        _ui.value = _ui.value.copy(
            stage = Stage.FIELD,
            me = _ui.value.me?.copy(onboarded = true),
        )
        refresh()
    }

    /**
     * Wohin nach der Anmeldung.
     *
     * Die Einweisung erscheint einmal je Zugang. Wer sie durchlaufen oder
     * übersprungen hat, bekommt sie nicht erneut – am Spieltag wäre das nur im
     * Weg.
     */
    private fun stageFor(me: Me): Stage =
        if (me.onboarded) Stage.FIELD else Stage.ONBOARDING

    fun clearMessages() {
        _ui.value = _ui.value.copy(error = null, notice = null)
    }

    fun showNotice(text: String) {
        _ui.value = _ui.value.copy(notice = text)
    }
}

/**
 * Ein Regelwert aus dem Nachschlagewerk.
 *
 * Die Oberfläche nennt Preise und Abstände – "zehn Minuten Sperre", "höchstens
 * 150 Meter". Diese Zahlen standen fest im Text, obwohl die Zentrale sie im
 * Regelpult ändern kann. Nach einer Änderung log die App: Sie nannte einen
 * Preis, den es nicht mehr gab, und jemand entschied danach.
 *
 * [fallback] gilt, solange das Nachschlagewerk noch nicht geladen ist.
 */
fun UiState.regel(key: String, fallback: Int): Int =
    rules.asSequence()
        .flatMap { it.rules.asSequence() }
        .firstOrNull { it.key == key }
        ?.value
        ?: fallback
