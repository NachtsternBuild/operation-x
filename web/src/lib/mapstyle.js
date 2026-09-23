/*
 * Kartenstil für Operation X.
 *
 * Die Karte ist der Untergrund, nicht der Inhalt: Sie muss dunkel und ruhig
 * bleiben, damit Sektorflächen, Hotspots und Teamsymbole darüber lesbar sind.
 * Statt eines CSS-Filters über der ganzen Karte – der auch die Overlays
 * entfärben würde – wird der Kartenlayer selbst abgedunkelt und entsättigt.
 *
 * Die Kacheln kommen vom eigenen Spielserver. Der holt sie einmal von
 * OpenStreetMap und legt sie ab: Das zweite Gerät bekommt sie aus dem Ordner,
 * und nach dem ersten Rundgang liegt das Spielgebiet vollständig dort. Acht
 * Telefone laden damit nicht achtmal dasselbe über Mobilfunk, und im Funkloch
 * bleibt sichtbar, was schon einmal jemand angesehen hat.
 *
 * Vorab heruntergeladen wird nichts – das untersagen die Bedingungen von
 * OpenStreetMap, und zwar zu Recht. Wer ganz ohne Netz spielen will, legt
 * einen eigenen Kachelsatz neben das Programm; der Server nimmt dann den.
 */

export const ATTRIBUTION =
  '© <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noreferrer">OpenStreetMap</a>-Mitwirkende'

export function baseStyle() {
  return {
    version: 8,
    sources: {
      osm: {
        type: 'raster',
        tiles: ['/api/opx/tiles/{z}/{x}/{y}'],
        tileSize: 256,
        maxzoom: 19,
        attribution: ATTRIBUTION,
      },
    },
    layers: [
      {
        // Grundfläche in der Farbe der Anwendung, damit beim Nachladen der
        // Kacheln kein weißes Aufblitzen entsteht.
        id: 'ground',
        type: 'background',
        paint: { 'background-color': '#0b1215' },
      },
      {
        id: 'osm',
        type: 'raster',
        source: 'osm',
        paint: {
          // Kräftig abgedunkelt und weitgehend entsättigt: Die Karte ist der
          // Untergrund, auf dem Sektorflächen und Teamsymbole lesbar bleiben
          // müssen. Straßenverläufe und Gewässer bleiben dabei erkennbar –
          // danach navigiert im Feld schließlich jemand.
          'raster-brightness-min': 0,
          'raster-brightness-max': 0.28,
          'raster-saturation': -0.75,
          'raster-contrast': 0.2,
          'raster-opacity': 0.85,
        },
      },
    ],
  }
}
