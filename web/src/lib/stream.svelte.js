/*
 * Der Lagestrom.
 *
 * Vorher fragte jede Ansicht für sich nach: die Lagekarte alle sechs Sekunden,
 * das Protokoll alle acht, das Regelpult alle zehn, die Hinweistafel alle
 * zwanzig. Zusammen waren das an einem Spieltag Zehntausende Anfragen über
 * Mobilfunk — für ein Spiel, in dem vielleicht zweihundertmal wirklich etwas
 * passiert.
 *
 * Jetzt hält eine einzige Verbindung offen, und der Server schickt, wenn es
 * etwas zu schicken gibt. Die Lage kommt direkt mit; alles Übrige lädt seine
 * Ansicht nach, wenn `revision` sich ändert.
 *
 * Bewusst kein EventSource: Der kann keine Kopfzeilen mitgeben, und der einzige
 * Ausweg wäre der Zugangsschlüssel in der Adresse — wo er in jedem Protokoll
 * und jedem Zwischenspeicher landet. Ein von Hand gelesener fetch-Strom kostet
 * dreißig Zeilen und vermeidet das.
 */
import { session, logout } from './session.svelte.js'
import { ensureFunk, funkAktiv, funkHeaders, openStreamEvent } from './funk.svelte.js'

export const stream = $state({
  /** Die zuletzt empfangene Lage, oder null. */
  live: null,
  /** Zählt hoch, sobald etwas Neues kam. Ansichten hängen sich daran. */
  revision: 0,
  /** Ob die Verbindung gerade steht. */
  connected: false,
  /** Klartext, wenn sie es nicht tut. */
  error: null,
})

let controller = null
let retryDelay = 1000
let retryTimer = null
let running = false

/** Wie lange höchstens zwischen zwei Verbindungsversuchen gewartet wird. */
const MAX_RETRY_MS = 15_000

export function startStream() {
  if (running) return
  running = true
  connect()
}

export function stopStream() {
  running = false
  clearTimeout(retryTimer)
  controller?.abort()
  controller = null
  stream.connected = false
}

function scheduleRetry() {
  if (!running) return
  clearTimeout(retryTimer)
  retryTimer = setTimeout(connect, retryDelay)
  // Beim ersten Bruch sofort wieder versuchen, danach zurückhaltender: Wer im
  // Funkloch steht, soll den Akku nicht mit Verbindungsversuchen leeren.
  retryDelay = Math.min(retryDelay * 2, MAX_RETRY_MS)
}

async function connect() {
  if (!running || !session.token) return

  await ensureFunk()

  controller?.abort()
  controller = new AbortController()

  try {
    const res = await fetch('/api/opx/stream', {
      // Mit den Kopfzeilen der zweiten Verschlüsselung, wenn sie steht: Der
      // Strom trägt die Positionen aller sichtbaren Teams und ist damit der
      // heikelste Inhalt überhaupt.
      headers: { Authorization: session.token, ...funkHeaders() },
      signal: controller.signal,
    })

    if (res.status === 401) {
      logout()
      return
    }
    if (!res.ok || !res.body) {
      throw new Error(`Der Server antwortete mit ${res.status}.`)
    }

    stream.connected = true
    stream.error = null
    retryDelay = 1000

    await readEvents(res.body)
  } catch (err) {
    if (err?.name === 'AbortError') return
    stream.error = err?.message ?? 'Verbindung unterbrochen.'
  } finally {
    stream.connected = false
    scheduleRetry()
  }
}

/**
 * Liest den Strom Zeile für Zeile.
 *
 * Das Format ist einfach: Blöcke, getrennt durch eine Leerzeile, darin Zeilen
 * der Form `feld: wert`. Zeilen, die mit einem Doppelpunkt beginnen, sind
 * Kommentare — der Server nutzt sie als Lebenszeichen.
 */
async function readEvents(body) {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  for (;;) {
    const { value, done } = await reader.read()
    if (done) return

    buffer += decoder.decode(value, { stream: true })

    let split
    while ((split = buffer.indexOf('\n\n')) >= 0) {
      const block = buffer.slice(0, split)
      buffer = buffer.slice(split + 2)
      handleBlock(block)
    }
  }
}

function handleBlock(block) {
  let event = 'message'
  let data = ''

  for (const line of block.split('\n')) {
    if (line.startsWith(':')) continue
    if (line.startsWith('event:')) event = line.slice(6).trim()
    else if (line.startsWith('data:')) data += line.slice(5).trim()
  }

  if (event === 'bye') {
    // Der Server beendet die Verbindung planmäßig. Sofort neu verbinden, nicht
    // mit Verzögerung — hier ist nichts kaputt.
    retryDelay = 200
    controller?.abort()
    return
  }

  if (event !== 'live' || !data) return

  void zeigeLage(data)
}

/** Entschlüsselt bei Bedarf und übernimmt die Lage. */
async function zeigeLage(data) {
  try {
    const text = funkAktiv() ? await openStreamEvent(data) : data
    stream.live = JSON.parse(text)
    stream.revision += 1
  } catch {
    /* Ein unvollständiger Block wird verworfen; der nächste kommt gleich. */
  }
}
