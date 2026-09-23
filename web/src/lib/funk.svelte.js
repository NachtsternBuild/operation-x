/*
 * Der Lagefunk: eine zweite Verschlüsselung zwischen Gerät und Spielserver.
 *
 * Warum, wo doch alles über HTTPS läuft: Der Tunnel endet nicht auf dem
 * Spielserver, sondern bei seinem Betreiber. Dort wird entschlüsselt und neu
 * verschlüsselt – das ist der Sinn eines Tunnels und der Grund, warum er ohne
 * eigenes Zertifikat funktioniert. Es heißt aber auch: Wer ihn betreibt, sieht
 * den Klartext, und das wären hier die Standortdaten von zwanzig Leuten über
 * sechs Stunden.
 *
 * Also verschlüsselt dieses Gerät seine Anfragen ein zweites Mal, mit einem
 * Schlüssel, den nur es und der Spielserver kennen.
 *
 * Was das nicht leistet, und das gehört dazu: Der Spielserver sieht weiterhin
 * alles – er ist das Spiel. Und wer mitliest, sieht weiterhin, wann wie viel an
 * welche Adresse geht.
 *
 * Verfahren: ECDH auf P-256, daraus per HKDF ein Sitzungsschlüssel, damit
 * AES-256-GCM. Alles aus der Web-Crypto-Schnittstelle des Browsers; dasselbe
 * steht in Go und in Kotlin noch einmal, Zeichen für Zeichen gleich.
 */

import { b64, unb64, beiwerk, ableiten, schluessel } from './funk-kern.js'

/** Zustand für die Anzeige: Läuft die zweite Verschlüsselung? */
export const funk = $state({
  ready: false,
  fingerprint: null,
  /** Warum nicht, wenn nicht. */
  reason: null,
})

let session = null
let seq = 0

/**
 * Der laufende Aufbau.
 *
 * Jeder Aufrufer wartet auf dasselbe Versprechen, statt dass jeder eigene
 * Schlüssel aushandelt – und vor allem wartet er überhaupt: Ohne das schickte
 * die erste Anfrage unverschlüsselt los, während der Aufbau noch lief.
 */
let aufbau = null

const enc = new TextEncoder()

/**
 * Baut die Verschlüsselung auf.
 *
 * Ohne Web-Crypto geht es nicht – und das ist kein Versehen des Browsers,
 * sondern seine Regel: Verschlüsselung gibt es nur in einem sicheren Kontext,
 * also über HTTPS oder auf dem eigenen Rechner. Im Heimnetz über eine
 * IP-Adresse fehlt sie deshalb. Dort gibt es aber auch keinen Tunnel, gegen
 * den sie schützen müsste.
 */
export function ensureFunk() {
  if (!aufbau) aufbau = startFunk()
  return aufbau
}

async function startFunk() {
  if (session) return true

  if (!globalThis.crypto?.subtle) {
    funk.reason =
      'Der Browser gibt die Verschlüsselung nur über HTTPS frei. Im Heimnetz ' +
      'ist das unkritisch – dort liegt kein Tunnel dazwischen.'
    return false
  }

  try {
    const res = await fetch('/api/opx/key')
    const info = await res.json()
    if (!info.available) {
      funk.reason = 'Dieser Server bietet keine zusätzliche Verschlüsselung.'
      return false
    }

    const paar = await crypto.subtle.generateKey(
      { name: 'ECDH', namedCurve: 'P-256' },
      false,
      ['deriveBits'],
    )

    const eigenerPub = b64(await crypto.subtle.exportKey('raw', paar.publicKey))

    const serverPub = await crypto.subtle.importKey(
      'raw',
      unb64(info.publicKey),
      { name: 'ECDH', namedCurve: 'P-256' },
      false,
      [],
    )

    const shared = await crypto.subtle.deriveBits(
      { name: 'ECDH', public: serverPub },
      paar.privateKey,
      256,
    )

    // Dieselbe Ableitung wie auf dem Server: Salz ist das Schlüsselpaar beider
    // Seiten, damit kein zweites Gerät denselben Schlüssel bekommt.
    const rohKey = await ableiten(shared, eigenerPub, info.publicKey)
    const key = await schluessel(rohKey)

    session = { key, pub: eigenerPub }
    funk.ready = true
    funk.fingerprint = info.fingerprint
    funk.reason = null
    return true
  } catch (err) {
    funk.reason = `Verschlüsselung nicht aufgebaut: ${err.message}`
    return false
  }
}

/** Die Kopfzeilen und der verschlüsselte Rumpf für eine Anfrage. */
export async function sealRequest(method, path, bodyText) {
  if (!session) return null

  seq += 1
  const zusatz = beiwerk(method, path, seq)
  const headers = { 'X-Opx-Key': session.pub, 'X-Opx-Seq': String(seq) }

  let body = null
  if (bodyText != null) {
    const nonce = crypto.getRandomValues(new Uint8Array(12))
    const chiffre = await crypto.subtle.encrypt(
      { name: 'AES-GCM', iv: nonce, additionalData: zusatz },
      session.key,
      enc.encode(bodyText),
    )
    headers['X-Opx-Nonce'] = b64(nonce)
    headers['Content-Type'] = 'application/octet-stream'
    body = chiffre
  }

  return { headers, body, zusatz }
}

/** Entschlüsselt eine Antwort. Liefert den Text. */
export async function openResponse(res, zusatz) {
  const nonce = res.headers.get('X-Opx-Nonce')
  const roh = await res.arrayBuffer()

  // Ohne Zufallswert kam die Antwort im Klartext – so antwortet der Server auf
  // Fehler, damit ein Gerät die Begründung auch dann lesen kann, wenn es
  // gerade nicht entschlüsseln kann.
  if (!nonce) return new TextDecoder().decode(roh)

  const klar = await crypto.subtle.decrypt(
    { name: 'AES-GCM', iv: unb64(nonce), additionalData: zusatz },
    session.key,
    roh,
  )
  return new TextDecoder().decode(klar)
}

/** Entschlüsselt eine Sendung des Lagestroms. */
export async function openStreamEvent(text) {
  if (!session) return text

  const paket = unb64(text)
  const klar = await crypto.subtle.decrypt(
    {
      name: 'AES-GCM',
      iv: paket.slice(0, 12),
      additionalData: enc.encode('GET /api/opx/stream'),
    },
    session.key,
    paket.slice(12),
  )
  return new TextDecoder().decode(klar)
}

/** Der öffentliche Schlüssel dieses Geräts – für die Kopfzeile des Stroms. */
export function funkHeaders() {
  if (!session) return {}
  seq += 1
  return { 'X-Opx-Key': session.pub, 'X-Opx-Seq': String(seq) }
}

export function funkAktiv() {
  return session !== null
}

/** Der Sitzungsschlüssel – für den Sonderfall Dateiupload. */
export function funkSchluessel() {
  return session?.key ?? null
}

/** Base64 in der Form, die die Kopfzeilen erwarten. */
export const funkB64 = b64
