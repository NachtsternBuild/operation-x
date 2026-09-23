import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { INFO, ableiten, b64, beiwerk, salz, schluessel, unb64 } from './funk-kern.js'

/*
 * Rechnet den gemeinsamen Prüfvektor nach.
 *
 * Dieselbe Datei prüfen der Server (internal/crypt/vektor_test.go) und die
 * Android-App (FunkVektorTest.kt). Ergibt eine der drei Sprachen etwas
 * anderes, reden Server und Gerät aneinander vorbei — lautlos, weil eine
 * misslungene Entschlüsselung aussieht wie eine leere Antwort.
 */

const vektor = JSON.parse(readFileSync(new URL('../../../testdaten/lagefunk.json', import.meta.url)))

const hex = (text) => Uint8Array.from(text.match(/../g).map((b) => parseInt(b, 16)))
const hexVon = (roh) =>
  [...new Uint8Array(roh)].map((b) => b.toString(16).padStart(2, '0')).join('')

const dec = new TextDecoder()
const enc = new TextEncoder()

describe('Lagefunk-Kern', () => {
  it('leitet denselben Schlüssel ab wie der Server', async () => {
    const roh = await ableiten(
      hex(vektor.gemeinsamesGeheimnisHex),
      vektor.clientPublicKey,
      vektor.serverPublicKey,
    )

    expect(hexVon(roh)).toBe(vektor.schluesselHex)
  })

  it('benutzt dieselbe Kennung für HKDF', () => {
    expect(INFO).toBe(vektor.info)
  })

  it('baut das Salz aus beiden Schlüsseln, in dieser Reihenfolge', () => {
    expect(dec.decode(salz('AAA', 'BBB'))).toBe('AAA|BBB')
  })

  it('öffnet eine Nachricht des Servers', async () => {
    const key = await schluessel(hex(vektor.schluesselHex))
    const klar = await crypto.subtle.decrypt(
      {
        name: 'AES-GCM',
        iv: hex(vektor.nonceHex),
        additionalData: enc.encode(vektor.beiwerk),
      },
      key,
      hex(vektor.chiffreHex),
    )

    expect(dec.decode(klar)).toBe(vektor.klartext)
  })

  it('lässt mit falschem Beiwerk nichts aufgehen', async () => {
    const key = await schluessel(hex(vektor.schluesselHex))

    await expect(
      crypto.subtle.decrypt(
        {
          name: 'AES-GCM',
          iv: hex(vektor.nonceHex),
          additionalData: enc.encode('GET /woanders 7'),
        },
        key,
        hex(vektor.chiffreHex),
      ),
    ).rejects.toThrow()
  })

  it('baut das Beiwerk in der Form, die der Server erwartet', () => {
    expect(dec.decode(beiwerk('POST', '/api/opx/position?seit=3', 7))).toBe(vektor.beiwerk)
  })

  it('kodiert base64url ohne Auffüllzeichen', () => {
    const roh = new Uint8Array([0, 1, 2, 3, 4])
    const text = b64(roh)

    expect(text).not.toMatch(/[=+/]/)
    expect([...unb64(text)]).toEqual([...roh])
  })

  it('liest base64url mit und ohne Auffüllzeichen', () => {
    // Der Server schickt ohne, andere Quellen manchmal mit.
    expect([...unb64('AQID')]).toEqual([1, 2, 3])
    expect([...unb64('-_8')]).toEqual([251, 255])
  })
})
