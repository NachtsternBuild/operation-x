/*
 * Anmeldung und Serverzugriff.
 *
 * Zugangsdaten liegen mit einer Verfallszeit im Browser: Nach Ablauf löschen
 * sie sich beim nächsten Start von selbst. Das ist dieselbe Zusage, die die
 * Android-App später für ihren lokalen Speicher gibt – Standortdaten und
 * Zugänge überleben das Spiel nicht.
 */

import {
  ensureFunk,
  funkAktiv,
  funkB64,
  funkSchluessel,
  sealRequest,
  openResponse,
} from './funk.svelte.js'

const STORE_KEY = 'opx.session'

/** Wie lange eine Anmeldung gültig bleibt, wenn der Server nichts anderes sagt. */
const DEFAULT_TTL_HOURS = 24

/*
 * Speicher, Anfrage, Verfallszeit.
 *
 * Nach außen gegeben, damit ein aufbauendes Programm eigene Arten von Zugang
 * anmelden kann, ohne dieses Modul zu kopieren. Der gespeicherte Eintrag ist
 * ein beliebiges Objekt – wer zusätzliche Felder hineinlegt, bekommt sie beim
 * Laden zurück.
 */
export function loadStored() {
  try {
    const raw = localStorage.getItem(STORE_KEY)
    if (!raw) return null

    const data = JSON.parse(raw)
    if (!data?.token || !data?.expiresAt) return null

    if (Date.now() > data.expiresAt) {
      localStorage.removeItem(STORE_KEY)
      return null
    }
    return data
  } catch {
    // Privater Modus, gesperrter Speicher, beschädigter Eintrag – in allen
    // Fällen gilt: nicht angemeldet.
    return null
  }
}

export function store(data) {
  try {
    localStorage.setItem(STORE_KEY, JSON.stringify(data))
  } catch {
    // Ohne Speicher funktioniert die Sitzung trotzdem, sie überlebt nur
    // kein Neuladen der Seite.
  }
}

export function clearStored() {
  try {
    localStorage.removeItem(STORE_KEY)
  } catch {
    /* nichts zu tun */
  }
}

const stored = loadStored()

/** Beobachtbarer Sitzungszustand. */
export const session = $state({
  token: stored?.token ?? null,
  team: stored?.team ?? null,
  game: stored?.game ?? null,

  /*
   * Wofür der Zugangsschlüssel gerade gilt. Hier immer "team"; ein
   * aufbauendes Programm kennt weitere Arten von Zugang.
   *
   * Dass es dieses Feld gibt und nicht einfach "ist session.team gesetzt",
   * hat einen Grund, der einmal teuer war: refresh() schreibt session.team.
   * Wer das in einem Effekt liest, löst ihn damit erneut aus – eine Schleife,
   * die den Server mit Anfragen überzieht. Diese Angabe wird nur beim An- und
   * Abmelden gesetzt und ist deshalb sicher zu lesen.
   */
  // Der Fallback gilt Sitzungen, die noch aus der Zeit vor diesem Feld im
  // Browser liegen: Ohne ihn liefe eine laufende Anmeldung nach dem Update
  // weiter, aber ohne Lagestrom – die Oberfläche sähe eingefroren aus.
  kind: stored?.kind ?? (stored?.team ? 'team' : null),

  loading: false,
  error: null,
})

/** Rolle des angemeldeten Teams, oder null. */
export function role() {
  return session.team?.role ?? null
}

export async function request(path, { method = 'GET', body, auth = true } = {}) {
  const headers = { 'Content-Type': 'application/json' }
  if (auth && session.token) headers['Authorization'] = session.token

  // Zweite Verschlüsselung, wenn sie steht: Der Rumpf geht dann verschlüsselt
  // hinaus und verschlüsselt zurück. Steht sie nicht, bleibt alles wie bisher –
  // ein Spiel darf nicht daran scheitern.
  const text = body ? JSON.stringify(body) : null

  // Auf den Aufbau warten, nicht daran vorbeilaufen: Sonst ginge ausgerechnet
  // die erste Anfrage – die Anmeldung mit dem Kennwort – im Klartext hinaus.
  await ensureFunk()
  const sealed = funkAktiv() ? await sealRequest(method, path, text) : null

  let res
  try {
    res = await fetch(path, {
      method,
      headers: sealed ? { ...headers, ...sealed.headers } : headers,
      body: sealed ? sealed.body : text || undefined,
    })
  } catch {
    /*
     * Kein Netz.
     *
     * Das ist am Spieltag die mit Abstand häufigste Störung, und ohne diesen
     * Zweig steht dort "Failed to fetch" — auf Englisch, und ohne zu sagen, was
     * zu tun ist. Wer zwischen zwei Häuserzeilen steht, soll erkennen, dass er
     * nichts falsch gemacht hat.
     */
    const err = new Error('Kein Kontakt zum Einsatzserver. Empfang prüfen.')
    err.status = 0
    throw err
  }

  let payload = null
  try {
    payload = sealed
      ? JSON.parse((await openResponse(res, sealed.zusatz)) || 'null')
      : await res.json()
  } catch {
    payload = null
  }

  if (!res.ok) {
    if (res.status === 401 && auth) logout()
    const err = new Error(payload?.message || `Server antwortete mit ${res.status}`)
    err.status = res.status
    err.data = payload
    throw err
  }

  return payload
}

export const api = {
  get: (path) => request(path),

  /**
   * Holt eine Datei und legt sie in den Ordner "Downloads".
   *
   * Der gewöhnliche Weg – ein Verweis zum Anklicken – geht hier nicht: Die
   * Sicherung liegt hinter der Anmeldung, und ein Verweis trägt kein
   * Zugangstoken. Also wird sie geholt, im Speicher gehalten und dem Browser
   * als Download untergeschoben.
   */
  async download(path, fallbackName) {
    const headers = {}
    if (session.token) headers['Authorization'] = session.token

    let res
    try {
      res = await fetch(path, { method: 'POST', headers })
    } catch {
      const err = new Error('Kein Kontakt zum Einsatzserver. Empfang prüfen.')
      err.status = 0
      throw err
    }

    if (!res.ok) {
      let message = `Server antwortete mit ${res.status}`
      try {
        const payload = await res.json()
        message = payload?.message || message
      } catch {
        /* keine Begründung dabei */
      }
      const err = new Error(message)
      err.status = res.status
      throw err
    }

    // Den Namen nennt der Server; er enthält Spielnamen und Zeitpunkt.
    const disposition = res.headers.get('Content-Disposition') ?? ''
    const match = disposition.match(/filename="?([^"]+)"?/)
    const name = match ? match[1] : fallbackName

    const blob = await res.blob()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = name
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)

    return { name, size: blob.size }
  },

  /**
   * Holt ein Bild und gibt eine Adresse zurück, die ein <img> versteht.
   *
   * Dasselbe Problem wie beim Download: Ein Verweis trägt kein Zugangstoken,
   * und seit die Beweisfotos hinter der Anmeldung liegen, reicht die Adresse
   * allein nicht mehr. Also holen, im Speicher halten, als Objektadresse
   * zurückgeben — und der Aufrufer gibt sie wieder frei.
   */
  async bild(path) {
    const headers = {}
    if (session.token) headers['Authorization'] = session.token

    const res = await fetch(path, { headers })
    if (!res.ok) throw new Error(`Bild nicht abrufbar (${res.status})`)
    return URL.createObjectURL(await res.blob())
  },

  post: (path, body) => request(path, { method: 'POST', body }),
  patch: (path, body) => request(path, { method: 'PATCH', body }),
  del: (path) => request(path, { method: 'DELETE' }),

  /**
   * Sendet Formulardaten mit Datei. Der Inhaltstyp wird bewusst nicht gesetzt –
   * den muss der Browser selbst bestimmen, weil er die Trennmarke der
   * mehrteiligen Nachricht enthält.
   */
  async upload(path, formData) {
    const headers = {}
    if (session.token) headers['Authorization'] = session.token

    await ensureFunk()

    let body = formData
    let zusatz = null

    if (funkAktiv()) {
      // Auch das Beweisfoto geht verschlüsselt hinaus: Ein Bild verrät den
      // Standort oft deutlicher als eine Koordinate.
      //
      // Den fertigen mehrteiligen Rumpf bekommt man nur über den Umweg eines
      // Request-Objekts – der Browser baut ihn selbst zusammen, mitsamt der
      // Trennmarke, und die muss im Inhaltstyp mitgehen, sonst findet der
      // Server die Teile nicht wieder.
      const roh = new Request(path, { method: 'POST', body: formData })
      const typ = roh.headers.get('Content-Type')
      const bytes = await roh.arrayBuffer()

      const sealed = await sealRequest('POST', path, null)
      const { headers: fh, zusatz: z } = sealed
      zusatz = z

      const nonce = crypto.getRandomValues(new Uint8Array(12))
      body = await crypto.subtle.encrypt(
        { name: 'AES-GCM', iv: nonce, additionalData: z },
        funkSchluessel(),
        bytes,
      )

      Object.assign(headers, fh, {
        'X-Opx-Nonce': funkB64(nonce),
        'X-Opx-Type': typ,
        'Content-Type': 'application/octet-stream',
      })
    }

    const res = await fetch(path, { method: 'POST', headers, body })

    let payload = null
    try {
      payload = zusatz
        ? JSON.parse((await openResponse(res, zusatz)) || 'null')
        : await res.json()
    } catch {
      payload = null
    }

    if (!res.ok) {
      if (res.status === 401) logout()
      const err = new Error(payload?.message || `Server antwortete mit ${res.status}`)
      err.status = res.status
      throw err
    }

    return payload
  },
}

/** Zustand des Servers, auch ohne Anmeldung abrufbar. */
export function serverStatus() {
  return request('/api/opx/status', { auth: false })
}

/** Richtet Spiel und Einsatzzentrale ein. Geht nur beim allerersten Mal. */
export function setupServer(body) {
  return request('/api/opx/firstrun', { method: 'POST', auth: false, body })
}

/** Beitrittsinformationen: Adresse und ob die App bereitliegt. Ohne Anmeldung. */
export function joinInfo() {
  return request('/api/opx/join', { auth: false })
}

/**
 * Meldet ein Team mit Rufzeichen und Passwort an.
 * Die Gültigkeit richtet sich nach dem Spielende, mindestens aber DEFAULT_TTL_HOURS.
 */
export async function login(callsign, password) {
  session.loading = true
  session.error = null

  try {
    const auth = await request('/api/collections/teams/auth-with-password', {
      method: 'POST',
      auth: false,
      body: { identity: callsign, password },
    })

    session.token = auth.token
    session.kind = 'team'

    const me = await request('/api/opx/me')
    session.team = me
    session.game = me.game ?? null

    store({
      token: auth.token,
      kind: 'team',
      team: me,
      game: me.game ?? null,
      expiresAt: expiryFor(me.game),
    })

    return me
  } catch (err) {
    session.token = null
    session.team = null
    session.error = friendlyError(err)
    throw err
  } finally {
    session.loading = false
  }
}

/** Lädt den eigenen Zustand neu – etwa nachdem sich Punkte geändert haben. */
export async function refresh() {
  // Ein Zugang, der kein Team ist, hat kein /me – das gehört den Teams. Ohne
  // diese Zeile meldete die Prüfung ihn sofort wieder ab.
  if (!session.token || session.kind !== 'team') return null

  const me = await api.get('/api/opx/me')
  session.team = me
  session.game = me.game ?? null

  const current = loadStored()
  if (current) store({ ...current, team: me, game: me.game ?? null })

  return me
}

export function logout() {
  session.token = null
  session.kind = null
  session.team = null
  session.game = null
  clearStored()
}

export function expiryFor(game) {
  const fallback = Date.now() + DEFAULT_TTL_HOURS * 3600_000
  if (!game?.endsAt) return fallback

  const ends = Date.parse(game.endsAt.replace(' ', 'T'))
  if (Number.isNaN(ends)) return fallback

  // Spielende plus Aufbewahrungsfrist.
  return Math.max(fallback, ends + DEFAULT_TTL_HOURS * 3600_000)
}

function friendlyError(err) {
  if (err.status === 400 || err.status === 401) {
    return 'Rufzeichen oder Passwort stimmen nicht.'
  }
  if (err.message?.includes('fetch')) {
    return 'Kein Kontakt zum Server. Verbindung prüfen.'
  }
  return err.message || 'Unbekannter Fehler.'
}
