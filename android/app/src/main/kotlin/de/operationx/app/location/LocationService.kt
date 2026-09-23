package de.operationx.app.location

import android.Manifest
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.location.Location
import android.location.LocationListener
import android.location.LocationManager
import android.os.Build
import android.os.Bundle
import android.os.IBinder
import androidx.core.app.NotificationCompat
import androidx.core.content.ContextCompat
import androidx.core.location.LocationManagerCompat
import androidx.core.os.CancellationSignal
import androidx.lifecycle.LifecycleService
import androidx.lifecycle.lifecycleScope
import de.operationx.app.MainActivity
import de.operationx.app.R
import de.operationx.app.data.Api
import de.operationx.app.data.LiveStream
import de.operationx.app.data.PositionBuffer
import de.operationx.app.data.PositionReport
import de.operationx.app.data.Session
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withTimeoutOrNull
import kotlin.coroutines.resume
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.TimeZone

/**
 * Der Standortdienst.
 *
 * Das ist der Grund, warum es die App überhaupt gibt: Ein Browser-Tab hört auf
 * zu senden, sobald der Bildschirm aus ist – und das Regelwerk verlangt
 * Meldungen über Stunden. Ein Vordergrunddienst mit dauerhafter Benachrichtigung
 * ist die einzige von Android vorgesehene Methode für genau diesen Fall.
 *
 * Der Dienst erfasst, puffert und sendet. Fehlt das Netz, bleibt die Meldung
 * mit ihrem Erfassungszeitpunkt liegen und geht später mit.
 */
class LocationService : LifecycleService() {

    companion object {
        const val CHANNEL_ID = "opx.location"
        const val NOTIFICATION_ID = 4711

        const val ACTION_START = "de.operationx.app.START_TRACKING"
        const val ACTION_STOP = "de.operationx.app.STOP_TRACKING"
        const val ACTION_PING = "de.operationx.app.PING_NOW"

        /** Wie oft das Gerät eine Ortung versucht. */
        private const val FIX_INTERVAL_MS = 45_000L

        /**
         * Kein Mindestabstand.
         *
         * Vorher standen hier fünfzehn Meter, und das war der teuerste Fehler
         * der ganzen App: Android liefert eine Ortung erst, wenn BEIDE
         * Bedingungen erfüllt sind – die Zeit ist um UND das Gerät hat sich um
         * diese Strecke bewegt. Wer sich hinsetzt, bekommt also gar keine
         * Ortung mehr, meldet nichts, und kassiert nach der Meldefrist einen
         * Verstoß. Auf dem Prüfgerät waren das nach zwölf Minuten ohne jede
         * Bewegung drei Verstöße, eine Sperre und achtzehn Minuspunkte – für
         * ein Telefon, das ununterbrochen eine gültige Ortung hatte.
         *
         * Ein Fix alle fünfundvierzig Sekunden kostet Akku. Eine Strafe fürs
         * Stillsitzen kostet das Spiel.
         */
        private const val FIX_MIN_DISTANCE_M = 0f

        /**
         * Wie lange ein GPS-Fix das Netzwerk verdrängt.
         *
         * Beide Quellen laufen parallel, und die Netzwerkortung liegt in der
         * Stadt gern ein paar hundert Meter daneben. Kommt sie zwischen zwei
         * GPS-Fixes, springt die eigene Nadel über den halben Stadtteil – im
         * Prüflauf einmal um 2,7 Kilometer. Solange GPS liefert, hat das
         * Netzwerk deshalb nichts beizutragen.
         */
        private const val GPS_HOLD_MS = 150_000L

        /** Wie oft der Puffer zum Server geht. */
        private const val SEND_INTERVAL_MS = 60_000L

        /**
         * Rückfallebene, falls der Lagestrom nicht steht.
         *
         * Normalerweise kommen Alarme über die offene Verbindung, also in dem
         * Moment, in dem sie entstehen. Bricht sie weg – altes Netz, störrische
         * Zwischenstelle –, darf ein Sichtkontakt trotzdem nicht untergehen.
         */
        private const val ALERT_FALLBACK_MS = 60_000L

        fun start(context: Context) {
            val intent = Intent(context, LocationService::class.java).setAction(ACTION_START)
            ContextCompat.startForegroundService(context, intent)
        }

        fun stop(context: Context) {
            context.startService(Intent(context, LocationService::class.java).setAction(ACTION_STOP))
        }

        /**
         * Eine einzelne Meldung, sofort.
         *
         * Das ist der Knopf "Jetzt melden". Er lief vorher ins Leere: Er hat
         * die Lage vom Server neu geholt und sonst nichts – der Name versprach
         * eine Meldung, gemeldet wurde nichts. Wer ihn vor Ablauf der Frist
         * gedrückt hat, kassierte den Verstoß trotzdem.
         */
        fun pingNow(context: Context) {
            val intent = Intent(context, LocationService::class.java).setAction(ACTION_PING)
            ContextCompat.startForegroundService(context, intent)
        }
    }

    private lateinit var session: Session
    private lateinit var buffer: PositionBuffer
    private lateinit var api: Api

    private val alerts = AlertDecider()
    private lateinit var stream: LiveStream
    private var locationManager: LocationManager? = null
    private var pending = 0
    private var lastError: String? = null

    /**
     * Läuft die Erfassung schon?
     *
     * Ohne diese Sperre legte jeder Aufruf von startTracking() eine weitere
     * Garnitur Schleifen an: noch ein Sendeumlauf, noch ein Lagestrom, noch
     * eine Anmeldung beim Ortungsdienst. Und weil zwei Sendeumläufe denselben
     * Puffer lesen und danach unabhängig voneinander "so viele wie gerade
     * gesendet" wegwerfen, hätten sie einander Meldungen gelöscht, die nie
     * angekommen sind. Ausgelöst wird das von harmlosen Dingen: zweimal auf
     * den Schalter tippen, oder Android startet den Dienst nach Speichermangel
     * neu.
     */
    private var running = false

    /** Wann zuletzt ein GPS-Fix kam – siehe GPS_HOLD_MS. */
    private var lastGpsAt = 0L

    /** Wann zuletzt etwas über den Lagestrom hereinkam. */
    private var lastStreamAt = 0L

    /**
     * Läuft gerade eine Pause?
     *
     * Während der Pause wird nicht aufgezeichnet – nicht gepuffert, nicht
     * gesendet, nicht einmal geortet. Die Pause ist für das Mittagessen da und
     * für den Gang aufs Klo; einen Standortverlauf davon anzulegen wäre für
     * das Spiel nutzlos und für die Beteiligten übergriffig.
     *
     * Die Auskunft kommt aus dem Lagestrom und wird von der Antwort auf eine
     * Übertragung bestätigt: Steht der Strom gerade nicht, korrigiert der
     * Server beim nächsten Sendeversuch.
     */
    @Volatile
    private var paused = false

    private val listener = object : LocationListener {
        override fun onLocationChanged(location: Location) {
            onFix(location)
        }

        // Die veralteten Rückrufe müssen auf älteren Geräten trotzdem da sein.
        override fun onStatusChanged(provider: String?, status: Int, extras: Bundle?) {}
        override fun onProviderEnabled(provider: String) {}
        override fun onProviderDisabled(provider: String) {
            lastError = "Ortung ist ausgeschaltet."
            updateNotification()
        }
    }

    override fun onCreate() {
        super.onCreate()
        session = Session(applicationContext)
        buffer = PositionBuffer(applicationContext)
        api = Api(session)
        stream = LiveStream(session, api)
        createChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        super.onStartCommand(intent, flags, startId)

        when (intent?.action) {
            ACTION_STOP -> {
                stopTracking()
                stopSelf()
                return START_NOT_STICKY
            }

            ACTION_PING -> {
                // Die Benachrichtigung muss binnen Sekunden stehen, sonst
                // beendet Android den Dienst mit einer Ausnahme.
                startForeground(NOTIFICATION_ID, buildNotification())
                lifecycleScope.launch { pingNow() }
            }

            else -> startTracking()
        }

        // START_STICKY: Räumt Android den Dienst bei Speichermangel ab, startet
        // er von selbst wieder. Ein Spiel dauert Stunden.
        return START_STICKY
    }

    private fun startTracking() {
        startForeground(NOTIFICATION_ID, buildNotification())

        if (ContextCompat.checkSelfPermission(this, Manifest.permission.ACCESS_FINE_LOCATION)
            != PackageManager.PERMISSION_GRANTED
        ) {
            lastError = "Standortfreigabe fehlt."
            updateNotification()
            return
        }

        if (running) return
        running = true

        val manager = getSystemService(Context.LOCATION_SERVICE) as LocationManager
        locationManager = manager

        runCatching {
            manager.requestLocationUpdates(
                LocationManager.GPS_PROVIDER,
                FIX_INTERVAL_MS,
                FIX_MIN_DISTANCE_M,
                listener,
            )
            // Das Netzwerk als zweite Quelle: In Innenhöfen und Bahnhöfen
            // kommt oft kein GPS durch, aber eine grobe Ortung ist besser als
            // eine verpasste Meldung.
            manager.requestLocationUpdates(
                LocationManager.NETWORK_PROVIDER,
                FIX_INTERVAL_MS,
                FIX_MIN_DISTANCE_M,
                listener,
            )
        }.onFailure { lastError = "Ortung nicht verfügbar." }

        lifecycleScope.launch {
            while (true) {
                flush()
                delay(SEND_INTERVAL_MS)
            }
        }

        // Der zweite Umlauf: Alarme über den offenen Lagestrom. Bewusst
        // getrennt vom Senden, damit ein hängender Sendeversuch die Warnungen
        // nicht aufhält.
        //
        // Das läuft hier und nicht in der Oberfläche, weil die Oberfläche genau
        // dann nicht läuft, wenn die Warnung gebraucht wird: Telefon in der
        // Tasche, Bildschirm aus.
        lifecycleScope.launch {
            stream.connect().collect { live ->
                lastStreamAt = System.currentTimeMillis()
                val vorher = paused
                paused = live.status == "paused"
                if (paused != vorher) updateNotification()
                alerts.onLive(this@LocationService, live)
            }
        }

        // Und die Rückfallebene, falls der Strom nicht steht.
        lifecycleScope.launch {
            while (true) {
                delay(ALERT_FALLBACK_MS)
                pollAlerts()
            }
        }
    }

    /** Holt die Lage einmalig – nur, wenn der Strom nicht durchkommt. */
    private suspend fun pollAlerts() {
        if (session.token() == null) return

        // Das stand vorher nur im Kommentar: Gefragt wurde jede Minute, auch
        // wenn der Strom einwandfrei lief. Damit war ein Teil der Ersparnis
        // wieder weg, für die es den Strom überhaupt gibt.
        if (System.currentTimeMillis() - lastStreamAt < ALERT_FALLBACK_MS) return

        runCatching { api.live() }
            .onSuccess { alerts.onLive(this, it) }
    }

    private fun stopTracking() {
        locationManager?.removeUpdates(listener)
        locationManager = null
        running = false

        // Den Lagestrom aktiv kappen.
        //
        // Das Beenden der Nebenläufigkeit allein genügt nicht: Der Strom hängt
        // in einem lesenden Zugriff auf eine offene Verbindung, und der bricht
        // nicht ab, nur weil niemand mehr zuhört. Die Verbindung bliebe dann
        // stehen, bis der Server sie nach einer Stunde selbst schließt – und
        // jedes Aus- und Einschalten der Erfassung legte eine weitere dazu.
        if (::stream.isInitialized) stream.reset()
    }

    /**
     * Eine einzelne Meldung auf Anforderung.
     *
     * Erst eine frische Ortung anfordern, sonst die letzte bekannte nehmen:
     * Eine Meldung von vor zwei Minuten ist immer noch eine Meldung, und die
     * Frist ist gleich um. Danach sofort senden statt auf den nächsten Umlauf
     * zu warten – wer drückt, will jetzt gemeldet haben.
     */
    private suspend fun pingNow() {
        val manager = locationManager
            ?: (getSystemService(Context.LOCATION_SERVICE) as? LocationManager)

        val fix = manager?.let { currentFix(it) ?: lastKnown(it) }

        if (fix != null) {
            buffer.add(reportOf(fix))
            pending = buffer.size()
            lastError = null
        } else {
            lastError = "Keine Ortung möglich – Freigabe und Standortdienst prüfen."
        }

        flush()

        // Wer die Erfassung nicht eingeschaltet hat, bekommt genau eine
        // Meldung und danach seine Ruhe zurück.
        if (!session.trackingWanted.first()) {
            stopSelf()
        }
    }

    /** Eine frische Ortung, mit Frist – blockiert nicht länger als nötig. */
    private suspend fun currentFix(manager: LocationManager): Location? {
        // Die Prüfung steht hier und nicht in einer Hilfsfunktion: So sieht
        // auch die statische Prüfung des Werkzeugkastens, dass die Freigabe
        // vorliegt, bevor die Ortung angefordert wird.
        if (ContextCompat.checkSelfPermission(this, Manifest.permission.ACCESS_FINE_LOCATION)
            != PackageManager.PERMISSION_GRANTED
        ) {
            return null
        }

        return withTimeoutOrNull(12_000) {
            suspendCancellableCoroutine { cont ->
                val signal = CancellationSignal()
                cont.invokeOnCancellation { runCatching { signal.cancel() } }

                val provider = if (manager.isProviderEnabled(LocationManager.GPS_PROVIDER)) {
                    LocationManager.GPS_PROVIDER
                } else {
                    LocationManager.NETWORK_PROVIDER
                }

                runCatching {
                    LocationManagerCompat.getCurrentLocation(
                        manager, provider, signal, ContextCompat.getMainExecutor(this@LocationService),
                    ) { location ->
                        if (cont.isActive) cont.resume(location)
                    }
                }.onFailure { if (cont.isActive) cont.resume(null) }
            }
        }
    }

    /** Die zuletzt bekannte Ortung, egal von welcher Quelle. */
    private fun lastKnown(manager: LocationManager): Location? {
        if (ContextCompat.checkSelfPermission(this, Manifest.permission.ACCESS_FINE_LOCATION)
            != PackageManager.PERMISSION_GRANTED
        ) {
            return null
        }

        return listOf(LocationManager.GPS_PROVIDER, LocationManager.NETWORK_PROVIDER)
            .mapNotNull { runCatching { manager.getLastKnownLocation(it) }.getOrNull() }
            .maxByOrNull { it.time }
    }

    private fun onFix(location: Location) {
        if (paused) return

        val now = System.currentTimeMillis()

        if (location.provider == LocationManager.GPS_PROVIDER) {
            lastGpsAt = now
        } else if (now - lastGpsAt < GPS_HOLD_MS) {
            // GPS liefert gerade. Eine gleichzeitige Netzwerkortung wäre nur
            // ein Sprung auf der Karte, kein Erkenntnisgewinn.
            return
        }

        lifecycleScope.launch {
            buffer.add(reportOf(location))
            pending = buffer.size()
            lastError = null
            updateNotification()
        }
    }

    private fun reportOf(location: Location): PositionReport {
        // Simulierte Standorte werden gemeldet, nicht verschwiegen: Der Server
        // legt sie der Spielleitung vor, statt automatisch zu bestrafen.
        val mocked = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            location.isMock
        } else {
            @Suppress("DEPRECATION")
            location.isFromMockProvider
        }

        return PositionReport(
            lat = location.latitude,
            lng = location.longitude,
            accuracy = location.accuracy.toDouble(),
            speed = location.speed.toDouble(),
            heading = location.bearing.toDouble(),
            capturedAt = isoNow(location.time),
            mocked = mocked,
        )
    }

    /** Schickt den Puffer zum Server und leert nur, was angekommen ist. */
    private suspend fun flush() {
        val queued = buffer.peek()
        if (queued.isEmpty()) return

        runCatching { api.sendPositions(queued) }
            .onSuccess { ack ->
                // Der Server nimmt während der Pause nichts an. Die Meldungen
                // sind damit nicht verloren gegangen, sondern gar nicht erst
                // entstanden – der Puffer wird geleert, sonst liefe er über
                // eine Mittagspause hinweg voll.
                if (ack.paused) {
                    paused = true
                    buffer.clear()
                } else {
                    paused = false
                    buffer.drop(queued.size)
                }
                pending = buffer.size()
                lastError = null
            }
            .onFailure {
                lastError = "Wartet auf Netz."
            }

        updateNotification()
    }

    private fun createChannel() {
        val manager = getSystemService(NotificationManager::class.java)
        val channel = NotificationChannel(
            CHANNEL_ID,
            getString(R.string.location_channel),
            NotificationManager.IMPORTANCE_LOW,
        ).apply {
            description = getString(R.string.location_channel_desc)
            setShowBadge(false)
        }
        manager.createNotificationChannel(channel)

        // Die Alarmkanäle gehören daneben: getrennt stummschaltbar, mit
        // eigener Dringlichkeit.
        Alerts.createChannels(this)
    }

    private fun buildNotification(): Notification {
        val open = PendingIntent.getActivity(
            this, 0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE,
        )

        val text = when {
            paused -> "Pause – es wird kein Standort aufgezeichnet"
            lastError != null -> lastError
            pending == 1 -> "Eine Meldung wartet auf Übertragung"
            pending > 1 -> "$pending Meldungen warten auf Übertragung"
            else -> "Standort wird gemeldet"
        }

        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle("Operation X")
            .setContentText(text)
            .setSmallIcon(android.R.drawable.ic_menu_mylocation)
            .setContentIntent(open)
            .setOngoing(true)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .build()
    }

    private fun updateNotification() {
        getSystemService(NotificationManager::class.java)
            .notify(NOTIFICATION_ID, buildNotification())
    }

    override fun onBind(intent: Intent): IBinder? {
        super.onBind(intent)
        return null
    }

    override fun onDestroy() {
        stopTracking()
        super.onDestroy()
    }
}

/** Zeitstempel im Format, das der Server erwartet. */
fun isoNow(millis: Long = System.currentTimeMillis()): String {
    val format = SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss'Z'", Locale.US)
    format.timeZone = TimeZone.getTimeZone("UTC")
    return format.format(Date(millis))
}
