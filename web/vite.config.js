import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { copyFileSync, mkdirSync } from 'node:fs'
import { createRequire } from 'node:module'
import { dirname, join } from 'node:path'

/**
 * Der Kartenarbeiter.
 *
 * MapLibre rechnet nicht im Hauptstrang: Geometrie wird in einem eigenen
 * Arbeiter zerlegt, und den lädt es zur Laufzeit als eigene Datei nach — unter
 * einem Namen, den es sich selbst zusammensetzt. Genau deshalb sieht der
 * Bündler die Datei nicht und legt sie nicht mit ab.
 *
 * Die Folge war eine Karte, die nur den Stadtplan zeigte: keine Sektoren,
 * keine Hotspots, keine Teams. Der Arbeiter wurde geladen, bekam aber die
 * Startseite zurück (so antwortet der Server auf alles Unbekannte), scheiterte
 * still, und jede Geometrie wartete von da an auf eine Antwort, die nie kam.
 *
 * Also die beiden Dateien nach dem Bauen danebenlegen — aus dem Paket selbst,
 * damit sie bei einer neuen Fassung nicht veralten.
 */
function maplibreArbeiter() {
  return {
    name: 'maplibre-arbeiter',
    closeBundle() {
      const require = createRequire(import.meta.url)
      const dir = dirname(require.resolve('maplibre-gl/dist/maplibre-gl.mjs'))

      mkdirSync('dist/assets', { recursive: true })
      for (const datei of ['maplibre-gl-worker.mjs', 'maplibre-gl-shared.mjs']) {
        copyFileSync(join(dir, datei), join('dist/assets', datei))
      }

      // Der Platzhalter, den die Go-Einbettung braucht. Vite raeumt dist vor
      // jedem Bauen leer; ohne diese Zeile waere er nach dem ersten Bauen weg
      // und "go build" schluege in einem frischen Klon fehl.
      copyFileSync(new URL('./dist-platzhalter.txt', import.meta.url), 'dist/.gitkeep')
    },
  }
}

export default defineConfig({
  plugins: [svelte(), maplibreArbeiter()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // Die Oberfläche landet eingebettet in der Go-Binary. Kleine Bundles
    // zahlen sich am Stadtrand im Mobilfunk aus.
    target: 'es2020',
  },
  server: {
    port: 5173,
    // Im Entwicklungsbetrieb läuft der Spielserver getrennt auf 8090.
    proxy: {
      '/api': 'http://127.0.0.1:8090',
    },
  },
})
