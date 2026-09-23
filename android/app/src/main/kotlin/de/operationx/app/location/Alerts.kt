package de.operationx.app.location

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.VibrationEffect
import android.os.Vibrator
import android.os.VibratorManager
import androidx.core.app.NotificationCompat
import de.operationx.app.MainActivity
import de.operationx.app.data.LiveState

/**
 * Die Alarme.
 *
 * Ohne sie ist die App eine Webseite mit Extraschritten: Wer sein Telefon in
 * die Tasche steckt, verpasst die Meldefrist und zahlt dafür, obwohl er sich
 * völlig richtig verhalten hat. Genau dieser Fall war der Grund, überhaupt eine
 * eigene App zu bauen.
 *
 * Drei Kanäle, weil Android sie getrennt stummschalten lassen soll und weil sie
 * verschiedene Dringlichkeit haben:
 *
 *   - Der Dienst selbst, leise und dauerhaft. Der ist schon da.
 *   - Fristen: laut genug, um in der Hosentasche aufzufallen.
 *   - Sichtkontakt: der eine Moment, der laut sein darf.
 *
 * Ausgelöst wird aus dem Standortdienst heraus, nicht aus der Oberfläche. Die
 * Oberfläche ist genau dann tot, wenn die Alarme gebraucht werden.
 */
object Alerts {

    const val CHANNEL_DEADLINE = "opx.deadline"
    const val CHANNEL_ALARM = "opx.alarm"

    private const val ID_DEADLINE = 4712
    private const val ID_ALARM = 4713
    private const val ID_GAME = 4714

    fun createChannels(context: Context) {
        val manager = context.getSystemService(NotificationManager::class.java)

        manager.createNotificationChannel(
            NotificationChannel(
                CHANNEL_DEADLINE,
                "Fristen",
                NotificationManager.IMPORTANCE_HIGH,
            ).apply {
                description = "Erinnerung an die Pflichtmeldung und an Missionsfristen."
                enableVibration(true)
            }
        )

        manager.createNotificationChannel(
            NotificationChannel(
                CHANNEL_ALARM,
                "Alarm",
                NotificationManager.IMPORTANCE_HIGH,
            ).apply {
                description = "Sichtkontakt, Zugriff, Spielende."
                enableVibration(true)
                vibrationPattern = longArrayOf(0, 400, 200, 400)
            }
        )
    }

    fun deadline(context: Context, title: String, text: String) =
        post(context, ID_DEADLINE, CHANNEL_DEADLINE, title, text, urgent = false)

    fun alarm(context: Context, title: String, text: String) {
        post(context, ID_ALARM, CHANNEL_ALARM, title, text, urgent = true)
        buzz(context)
    }

    fun game(context: Context, title: String, text: String) =
        post(context, ID_GAME, CHANNEL_ALARM, title, text, urgent = false)

    private fun post(
        context: Context,
        id: Int,
        channel: String,
        title: String,
        text: String,
        urgent: Boolean,
    ) {
        val open = PendingIntent.getActivity(
            context, 0,
            Intent(context, MainActivity::class.java)
                .addFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP),
            PendingIntent.FLAG_IMMUTABLE,
        )

        val notification: Notification = NotificationCompat.Builder(context, channel)
            .setContentTitle(title)
            .setContentText(text)
            .setStyle(NotificationCompat.BigTextStyle().bigText(text))
            .setSmallIcon(android.R.drawable.ic_dialog_alert)
            .setContentIntent(open)
            .setAutoCancel(true)
            .setPriority(
                if (urgent) NotificationCompat.PRIORITY_MAX else NotificationCompat.PRIORITY_HIGH
            )
            .setCategory(
                if (urgent) NotificationCompat.CATEGORY_ALARM else NotificationCompat.CATEGORY_REMINDER
            )
            .build()

        runCatching {
            context.getSystemService(NotificationManager::class.java).notify(id, notification)
        }
    }

    /**
     * Vibration zusätzlich zum Ton.
     *
     * In der Hosentasche, auf der Straße, neben einer Hauptverkehrsstraße ist
     * der Ton wertlos – das Rütteln ist der eigentliche Kanal.
     */
    private fun buzz(context: Context) {
        val vibrator = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            context.getSystemService(VibratorManager::class.java)?.defaultVibrator
        } else {
            @Suppress("DEPRECATION")
            context.getSystemService(Context.VIBRATOR_SERVICE) as? Vibrator
        } ?: return

        runCatching {
            vibrator.vibrate(
                VibrationEffect.createWaveform(longArrayOf(0, 400, 200, 400), -1)
            )
        }
    }
}

/**
 * Entscheidet, welche Alarme aus einem Spielzustand folgen.
 *
 * Als eigene Klasse mit Gedächtnis, nicht als Funktion: Ohne das Gedächtnis
 * käme bei jeder Serverabfrage dieselbe Warnung erneut – aus einer Erinnerung
 * würde ein Dauerklingeln, und jemand schaltet die Benachrichtigungen ab. Dann
 * ist die eine Warnung, auf die es ankommt, auch weg.
 */
class AlertDecider {

    // Gemerkt wird der Fälligkeitszeitpunkt selbst, nicht der Moment der
    // Warnung: Er ist die Wahrheit, er steht in jeder Sendung gleich da, und
    // nach einer Meldung ist er ein anderer – damit ist die nächste Warnung
    // ohne weiteres Zutun wieder frei.
    private var warnedForDueAt: String? = null
    private var overdueForDueAt: String? = null
    private var lastGameStatus: String? = null

    // Bereits gemeldete Ereignisse. Begrenzt, weil der Dienst stundenlang
    // läuft und eine unbegrenzte Menge langsam vollliefe.
    private val announced = LinkedHashSet<String>()

    private fun remember(id: String): Boolean {
        if (!announced.add(id)) return false
        while (announced.size > 200) {
            announced.iterator().let { if (it.hasNext()) { it.next(); it.remove() } }
        }
        return true
    }

    /** Wie viele Sekunden vor Ablauf erinnert wird. */
    private val warnAheadSec = 120

    fun onLive(context: Context, live: LiveState) {
        deadline(context, live)
        events(context, live)
        gameState(context, live)
    }

    /**
     * Beim ersten Durchlauf wird nur gemerkt, nicht gemeldet.
     *
     * Sonst käme beim Start der App ein Schwall Benachrichtigungen für alles,
     * was in den letzten zehn Minuten passiert ist – und das Wichtigste ginge
     * darin unter.
     */
    private var primed = false

    private fun events(context: Context, live: LiveState) {
        if (!primed) {
            live.alerts.forEach { remember(it.id) }
            primed = true
            return
        }

        // Von alt nach neu, damit die jüngste Meldung oben liegt.
        live.alerts.asReversed().forEach { alert ->
            if (!remember(alert.id)) return@forEach
            if (alert.urgent) {
                Alerts.alarm(context, titleFor(alert.type), alert.reason)
            } else {
                Alerts.deadline(context, titleFor(alert.type), alert.reason)
            }
        }
    }

    private fun titleFor(type: String) = when (type) {
        "sighting.reported" -> "Sichtkontakt gemeldet"
        "ping.lockout" -> "Gesperrt"
        "mission.failed" -> "Frist abgelaufen"
        "arrest.partial" -> "Zugriff versucht"
        "camping" -> "Zu lange still"
        "finale.start" -> "Das Finale läuft"
        else -> "Operation X"
    }

    private fun deadline(context: Context, live: LiveState) {
        val dueAt = live.self.nextDueAt

        // Keine Frist, keine Warnung.
        //
        // Vor dem Spielstart hat noch kein Team eine Meldefrist. Der Server
        // schickt dafür eine leere Angabe und null Sekunden – und "null
        // Sekunden" las die App bisher als "gerade abgelaufen". Auf dem
        // Prüfgerät stand deshalb "Meldung überfällig – die Frist ist
        // abgelaufen" mit Alarmton und Rütteln auf dem Sperrbildschirm, in
        // einem Spiel, das noch gar nicht begonnen hatte.
        if (dueAt.isBlank()) {
            warnedForDueAt = null
            overdueForDueAt = null
            return
        }

        val due = live.self.dueInSec

        if (due in 1..warnAheadSec && dueAt != warnedForDueAt) {
            warnedForDueAt = dueAt
            Alerts.deadline(
                context,
                "Standort melden",
                "Noch ${due / 60}:${(due % 60).toString().padStart(2, '0')} Minuten bis zur " +
                    "Pflichtmeldung. Die App meldet automatisch, solange die Erfassung läuft.",
            )
        }

        if (due <= 0 && dueAt != overdueForDueAt) {
            overdueForDueAt = dueAt
            Alerts.alarm(
                context,
                "Meldung überfällig",
                "Die Frist ist abgelaufen. Jeder weitere Verstoß kostet Punkte, " +
                    "danach kommt eine Sperre dazu.",
            )
        }
    }

    private fun gameState(context: Context, live: LiveState) {
        val status = live.status.ifBlank { return }
        if (lastGameStatus == null) {
            lastGameStatus = status
            return
        }
        if (status == lastGameStatus) return
        lastGameStatus = status

        when (status) {
            "running" -> Alerts.game(context, "Das Spiel läuft", "Die Uhr tickt.")
            "paused" -> Alerts.game(
                context, "Spiel angehalten",
                "Die Zentrale hat unterbrochen. Fristen laufen nicht weiter.",
            )
            "finished" -> Alerts.alarm(
                context, "Spiel beendet",
                live.outcome?.reason?.ifBlank { "Das Spiel ist zu Ende." }
                    ?: "Das Spiel ist zu Ende.",
            )
        }
    }
}
