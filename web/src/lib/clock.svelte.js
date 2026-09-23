/*
 * Eine Uhr für die ganze Anwendung.
 *
 * Seit die Lage aus dem Strom kommt, treffen Aktualisierungen nur noch ein,
 * wenn sich wirklich etwas geändert hat — und das Alter einer Meldung ist
 * genau das, was sich auch ohne Änderung ändert. Vom Server übernommen bliebe
 * "vor 2 min" minutenlang stehen, während die Meldung längst zehn Minuten alt
 * ist. Auf einer Fahndungskarte ist das keine Kleinigkeit.
 *
 * Deshalb eine Uhr im Gerät: Der Server liefert den Zeitpunkt, die Anzeige
 * rechnet das Alter selbst aus.
 */

export const clock = $state({ now: Date.now() })

let timer = null
let users = 0

/** Meldet Bedarf an und liefert den Abmelder. */
export function useClock() {
  users += 1
  if (!timer) timer = setInterval(() => (clock.now = Date.now()), 1000)

  return () => {
    users -= 1
    if (users <= 0 && timer) {
      clearInterval(timer)
      timer = null
    }
  }
}

/** Alter in Sekunden, aus einem ISO-Zeitpunkt. */
export function ageSec(iso) {
  if (!iso) return 0
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return 0
  return Math.max(0, Math.round((clock.now - t) / 1000))
}

/** "jetzt", "3 min", "1 h 12 min" */
export function ageLabel(iso) {
  const s = ageSec(iso)
  if (s < 60) return 'jetzt'
  const m = Math.floor(s / 60)
  if (m < 60) return `${m} min`
  return `${Math.floor(m / 60)} h ${m % 60} min`
}
