/*
 * Projektion für das gedruckte Kartenblatt.
 *
 * Auf Papier gibt es keine Kacheln – die Sektorgrenzen werden direkt als
 * Vektoren gezeichnet. Verwendet wird dieselbe Mercator-Projektion wie auf der
 * Bildschirmkarte, damit gedrucktes und digitales Blatt dieselbe Form zeigen
 * und sich Formen unterwegs wiedererkennen lassen.
 */

const toRad = (deg) => (deg * Math.PI) / 180

/*
 * Beide Achsen müssen dieselbe Einheit haben, sonst stimmt das Seitenverhältnis
 * nicht: Längengrad in Radiant, Breitengrad über die Mercator-Formel ebenfalls
 * in Radiant. Rechnet man x in Grad und y in Mercator-Radiant, unterscheiden
 * sich die Maßstäbe um rund das Achtzigfache und die Karte wird zu einem
 * flachen Band gestaucht.
 */

/** Mercator-X in Radiant. */
function mercatorX(lngDeg) {
  return toRad(lngDeg)
}

/** Mercator-Y in Radiant. */
function mercatorY(latDeg) {
  return Math.log(Math.tan(Math.PI / 4 + toRad(latDeg) / 2))
}

/**
 * Erzeugt eine Projektionsfunktion für eine Zeichenfläche.
 *
 * Das Seitenverhältnis der Bounding-Box wird beibehalten und der Inhalt
 * zentriert – sonst zöge die Karte längliche Sektoren in die Breite und
 * niemand erkennt seinen Stadtteil wieder.
 */
export function makeProjector(bounds, width, height, padding = 12) {
  const w = width - padding * 2
  const h = height - padding * 2

  const x0 = mercatorX(bounds.west)
  const x1 = mercatorX(bounds.east)
  const y0 = mercatorY(bounds.north)
  const y1 = mercatorY(bounds.south)

  const spanX = x1 - x0 || 1e-9
  const spanY = y1 - y0 || 1e-9

  // Gemeinsamer Maßstab für beide Achsen.
  const scale = Math.min(w / spanX, h / spanY)

  const offsetX = padding + (w - spanX * scale) / 2
  const offsetY = padding + (h - spanY * scale) / 2

  return function project(lat, lng) {
    return [
      offsetX + (mercatorX(lng) - x0) * scale,
      offsetY + (mercatorY(lat) - y0) * scale,
    ]
  }
}

/**
 * Höhe der Zeichenfläche, die zum Gebiet passt.
 *
 * Ein festes Seitenverhältnis ließe bei einer breiten Stadt die halbe Seite
 * leer. Begrenzt wird nach oben und unten, damit ein sehr schmales Spielgebiet
 * das Blatt nicht sprengt.
 */
export function heightFor(bounds, width, minRatio = 0.45, maxRatio = 1.15) {
  const spanX = mercatorX(bounds.east) - mercatorX(bounds.west)
  const spanY = mercatorY(bounds.south) - mercatorY(bounds.north)
  if (spanX <= 0) return Math.round(width * 0.7)

  const ratio = Math.min(Math.max(spanY / spanX, minRatio), maxRatio)
  return Math.round(width * ratio)
}

/** Bounding-Box über mehrere GeoJSON-Geometrien und Punkte. */
export function boundsOf(geometries = [], points = []) {
  const b = { west: 180, south: 90, east: -180, north: -90 }
  let found = false

  const consume = (lng, lat) => {
    if (!Number.isFinite(lat) || !Number.isFinite(lng)) return
    b.west = Math.min(b.west, lng)
    b.east = Math.max(b.east, lng)
    b.south = Math.min(b.south, lat)
    b.north = Math.max(b.north, lat)
    found = true
  }

  for (const raw of geometries) {
    const g = typeof raw === 'string' ? JSON.parse(raw) : raw
    if (!g?.coordinates) continue

    const rings =
      g.type === 'MultiPolygon' ? g.coordinates.flat() : g.coordinates
    for (const ring of rings) {
      for (const [lng, lat] of ring) consume(lng, lat)
    }
  }

  for (const p of points) consume(p.lng, p.lat)

  if (!found) return null

  // Etwas Luft, damit Beschriftungen am Rand nicht abgeschnitten werden.
  const padX = (b.east - b.west) * 0.03
  const padY = (b.north - b.south) * 0.03
  return {
    west: b.west - padX,
    east: b.east + padX,
    south: b.south - padY,
    north: b.north + padY,
  }
}

/** Wandelt eine GeoJSON-Fläche in einen SVG-Pfad. */
export function geometryToPath(raw, project) {
  const g = typeof raw === 'string' ? JSON.parse(raw) : raw
  if (!g?.coordinates) return ''

  const rings = g.type === 'MultiPolygon' ? g.coordinates.flat() : g.coordinates

  return rings
    .map((ring) => {
      const pts = ring.map(([lng, lat]) => {
        const [x, y] = project(lat, lng)
        return `${x.toFixed(1)},${y.toFixed(1)}`
      })
      return pts.length ? `M${pts.join('L')}Z` : ''
    })
    .filter(Boolean)
    .join(' ')
}

/** Schwerpunkt einer Fläche in Bildschirmkoordinaten – für die Beschriftung. */
export function labelPoint(raw, project) {
  const g = typeof raw === 'string' ? JSON.parse(raw) : raw
  if (!g?.coordinates) return null

  const ring = (g.type === 'MultiPolygon' ? g.coordinates.flat() : g.coordinates)[0]
  if (!ring?.length) return null

  let area = 0
  let cx = 0
  let cy = 0
  for (let i = 0; i < ring.length; i++) {
    const [x1, y1] = ring[i]
    const [x2, y2] = ring[(i + 1) % ring.length]
    const cross = x1 * y2 - x2 * y1
    area += cross
    cx += (x1 + x2) * cross
    cy += (y1 + y2) * cross
  }

  if (Math.abs(area) < 1e-12) {
    const [lng, lat] = ring[0]
    return project(lat, lng)
  }

  area *= 0.5
  return project(cy / (6 * area), cx / (6 * area))
}
