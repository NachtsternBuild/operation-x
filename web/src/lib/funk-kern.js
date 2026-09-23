/*
 * Der Kern der zweiten Verschlüsselung — ohne Zustand, ohne Netz, ohne Runen.
 *
 * Getrennt von funk.svelte.js, weil genau diese Rechenschritte mit drei
 * anderen Sprachen übereinstimmen müssen: Go (internal/crypt), Kotlin
 * (android/…/data/Funk.kt) und Swift (ios/…/Data/Funk.swift). Hier stehen sie
 * so, dass ein Test sie nachrechnen kann, ohne einen Browser, einen Server
 * oder einen Schlüsselaustausch zu brauchen.
 *
 * Der gemeinsame Prüfvektor liegt in testdaten/lagefunk.json.
 */

export const INFO = 'operation-x/lagefunk/1'

const enc = new TextEncoder()

/** base64url ohne Auffüllzeichen — so überträgt der Server Schlüssel und Zufallswerte. */
export function b64(bytes) {
  let s = ''
  for (const b of new Uint8Array(bytes)) s += String.fromCharCode(b)
  return btoa(s).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

export function unb64(text) {
  const s = atob(text.replace(/-/g, '+').replace(/_/g, '/'))
  const out = new Uint8Array(s.length)
  for (let i = 0; i < s.length; i++) out[i] = s.charCodeAt(i)
  return out
}

/**
 * Das Beiwerk, mit dem versiegelt wird: "METHODE /pfad?abfrage nummer".
 *
 * Diese Zeile ist schon einmal auseinandergelaufen — der Server nahm die
 * Adresse samt Abfrageteil, der Browser nur den Pfad. Jede Anfrage mit einem
 * Fragezeichen scheiterte, und zwar lautlos.
 */
export function beiwerk(methode, pfad, nummer) {
  return enc.encode(`${methode} ${pfad} ${nummer}`)
}

/** Das Salz von HKDF: beide öffentlichen Schlüssel, Reihenfolge fest. */
export function salz(eigenerPub, serverPub) {
  return enc.encode(`${eigenerPub}|${serverPub}`)
}

/**
 * Leitet den Sitzungsschlüssel aus dem gemeinsamen Geheimnis ab.
 *
 * Getrennt vom Schlüsselaustausch, damit ein Test hier einsteigen kann, ohne
 * ein Schlüsselpaar zu erzeugen.
 */
export async function ableiten(gemeinsamesGeheimnis, eigenerPub, serverPub) {
  const hkdfKey = await crypto.subtle.importKey('raw', gemeinsamesGeheimnis, 'HKDF', false, [
    'deriveBits',
  ])
  return crypto.subtle.deriveBits(
    {
      name: 'HKDF',
      hash: 'SHA-256',
      salt: salz(eigenerPub, serverPub),
      info: enc.encode(INFO),
    },
    hkdfKey,
    256,
  )
}

/** Macht aus rohen Schlüsselbytes einen Schlüssel für AES-GCM. */
export function schluessel(roh) {
  return crypto.subtle.importKey('raw', roh, 'AES-GCM', false, ['encrypt', 'decrypt'])
}
