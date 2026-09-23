/*
 * Standorterfassung im Browser.
 *
 * Der Browser ist hier der Ausweichweg – die native Android-App macht das
 * gleiche zuverlässiger und auch bei gesperrtem Bildschirm. Was hier trotzdem
 * ernst genommen wird: Meldungen werden gepuffert, wenn das Netz fehlt, und
 * behalten dabei ihren Erfassungszeitpunkt. Der Server bewertet die Frist
 * danach, nicht nach dem Eingang.
 */

import { api } from './session.svelte.js'

const BUFFER_KEY = 'opx.positionBuffer'

/** Wie oft eine Meldung an den Server geht, wenn die Erfassung läuft. */
const SEND_INTERVAL_MS = 60_000

export const tracker = $state({
  /** aus | an | fehler */
  state: 'aus',
  error: null,
  lastFix: null,
  lastSentAt: null,
  buffered: 0,
  accuracy: null,
  supported: typeof navigator !== 'undefined' && 'geolocation' in navigator,
  secure: typeof window !== 'undefined' ? window.isSecureContext : true,
})

let watchId = null
let sendTimer = null

function loadBuffer() {
  try {
    return JSON.parse(localStorage.getItem(BUFFER_KEY) ?? '[]')
  } catch {
    return []
  }
}

function saveBuffer(list) {
  try {
    // Nach oben begrenzen: Ein Gerät, das stundenlang kein Netz hat, soll den
    // Speicher nicht vollschreiben.
    localStorage.setItem(BUFFER_KEY, JSON.stringify(list.slice(-200)))
  } catch {
    /* ohne Speicher geht der Puffer beim Neuladen verloren */
  }
  tracker.buffered = list.length
}

/** Startet die Erfassung. Fragt dabei die Berechtigung ab. */
export function start() {
  if (!tracker.supported) {
    tracker.state = 'fehler'
    tracker.error = 'Dieses Gerät liefert keine Standortdaten.'
    return
  }

  // Ohne sicheren Kontext rückt der Browser gar nichts heraus. Das trifft
  // genau den Fall „Server über die lokale Netzwerkadresse geöffnet“ und ist
  // der Grund für den Tunnel aus Abschnitt 10 des Konzepts.
  if (!tracker.secure) {
    tracker.state = 'fehler'
    tracker.error =
      'Standort nur über eine gesicherte Verbindung. Den Beitrittslink des HQ verwenden.'
    return
  }

  tracker.error = null
  tracker.state = 'an'

  watchId = navigator.geolocation.watchPosition(
    (pos) => {
      const fix = {
        lat: pos.coords.latitude,
        lng: pos.coords.longitude,
        accuracy: pos.coords.accuracy ?? 0,
        speed: pos.coords.speed ?? 0,
        heading: pos.coords.heading ?? 0,
        capturedAt: new Date(pos.timestamp).toISOString(),
      }

      tracker.lastFix = fix
      tracker.accuracy = fix.accuracy
      tracker.error = null

      const buffer = loadBuffer()
      buffer.push(fix)
      saveBuffer(buffer)
    },
    (err) => {
      tracker.state = 'fehler'
      tracker.error =
        err.code === err.PERMISSION_DENIED
          ? 'Standortfreigabe verweigert. Ohne sie lässt sich nicht mitspielen.'
          : 'Kein Standort verfügbar. Freien Himmel suchen.'
    },
    { enableHighAccuracy: true, maximumAge: 15_000, timeout: 30_000 },
  )

  sendTimer = setInterval(flush, SEND_INTERVAL_MS)
  // Einmal sofort, damit der Countdown gleich stimmt.
  setTimeout(flush, 2_000)
}

export function stop() {
  if (watchId !== null) navigator.geolocation.clearWatch(watchId)
  if (sendTimer) clearInterval(sendTimer)
  watchId = null
  sendTimer = null
  tracker.state = 'aus'
}

/**
 * Schickt den Puffer zum Server.
 *
 * Erst nach erfolgreicher Übertragung wird geleert – bricht die Verbindung ab,
 * bleiben die Meldungen erhalten und gehen beim nächsten Versuch mit.
 */
export async function flush() {
  const buffer = loadBuffer()
  if (buffer.length === 0) return null

  try {
    const res = await api.post('/api/opx/position', { positions: buffer })
    saveBuffer([])
    tracker.lastSentAt = new Date().toISOString()
    tracker.error = null
    return res
  } catch (err) {
    // Kein Netz: Puffer behalten und beim nächsten Mal erneut versuchen.
    tracker.error = 'Meldung wartet auf Netz.'
    return null
  }
}

/** Meldet den aktuellen Standort sofort, für den Ping-Knopf. */
export async function sendNow() {
  if (!tracker.supported || !tracker.secure) {
    start() // setzt die passende Fehlermeldung
    return null
  }

  return new Promise((resolve) => {
    navigator.geolocation.getCurrentPosition(
      async (pos) => {
        const buffer = loadBuffer()
        buffer.push({
          lat: pos.coords.latitude,
          lng: pos.coords.longitude,
          accuracy: pos.coords.accuracy ?? 0,
          speed: pos.coords.speed ?? 0,
          heading: pos.coords.heading ?? 0,
          capturedAt: new Date(pos.timestamp).toISOString(),
        })
        saveBuffer(buffer)
        tracker.accuracy = pos.coords.accuracy ?? 0
        resolve(await flush())
      },
      () => {
        tracker.error = 'Standort konnte nicht ermittelt werden.'
        resolve(null)
      },
      { enableHighAccuracy: true, timeout: 20_000 },
    )
  })
}

// Beim Laden sehen, was noch im Puffer liegt.
if (typeof localStorage !== 'undefined') {
  tracker.buffered = loadBuffer().length
}
