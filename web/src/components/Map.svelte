<script>
  /*
   * Die Lagekarte.
   *
   * Zeigt Spielgebiet, Sektoren und Hotspots. MapLibre wird erst hier
   * nachgeladen – die Bibliothek ist groß, und die Anmeldeseite braucht sie
   * nicht.
   */
  import { onMount, onDestroy } from 'svelte'
  import { baseStyle } from '../lib/mapstyle.js'

  let {
    sectors = [],
    hotspots = [],
    area = null,
    highlight = null, // Kennung eines hervorzuhebenden Sektors
    selected = [], // Kennungen ausgewählter Sektoren
    pins = [], // vorgemerkte, noch nicht gespeicherte Punkte
    teams = [], // Live-Positionen aus /api/opx/live
    finale = null, // Suchbereich im Endspurt
    onHotspotClick = null,
    onSectorClick = null,
    onMapClick = null, // Klick auf freie Fläche, liefert {lat, lng}
    center = [13.7373, 51.0504], // Dresden
    zoom = 11.5,
  } = $props()

  let container
  let map = null
  let maplibre = null
  let resizeObserver = null
  let ready = $state(false)
  let failed = $state(null)

  onMount(async () => {
    try {
      maplibre = await import('maplibre-gl')
      // Das Stylesheet gehört zur Bibliothek und wird mit ihr geladen.
      await import('maplibre-gl/dist/maplibre-gl.css')

      map = new maplibre.Map({
        container,
        style: baseStyle(),
        center,
        zoom,
        attributionControl: { compact: true },
        // Kartendrehung kostet unterwegs nur Orientierung.
        pitchWithRotate: false,
        dragRotate: false,
        touchZoomRotate: { around: 'center' },
      })

      map.addControl(new maplibre.NavigationControl({ showCompass: false }), 'top-right')
      map.addControl(
        new maplibre.GeolocateControl({
          positionOptions: { enableHighAccuracy: true },
          trackUserLocation: true,
          showAccuracyCircle: true,
        }),
        'top-right',
      )

      map.on('load', () => {
        ready = true
        drawAll()
        // Hatte der Container beim Anlegen noch nicht seine endgültige Größe –
        // etwa weil der Reiter gerade erst eingeblendet wurde –, zieht dieser
        // Aufruf das Canvas nach.
        requestAnimationFrame(() => map?.resize())
      })

      // Die Karte lebt in Reitern, die ein- und ausgeblendet werden, und der
      // Feldmodus ändert die Schriftgröße. Ohne Beobachtung behält das Canvas
      // die Größe, die der Container beim Anlegen hatte – und die Karte klebt
      // als Briefmarke in der Ecke.
      resizeObserver = new ResizeObserver(() => map?.resize())
      resizeObserver.observe(container)
    } catch (err) {
      failed = 'Die Karte konnte nicht geladen werden.'
      console.error(err)
    }
  })

  onDestroy(() => {
    resizeObserver?.disconnect()
    resizeObserver = null
    map?.remove()
    map = null
  })

  // Neu zeichnen, sobald sich die Daten ändern.
  $effect(() => {
    // Zugriff, damit Svelte die Abhängigkeiten erkennt.
    void sectors
    void hotspots
    void area
    void highlight
    void selected
    void pins
    void teams
    void finale
    if (ready) drawAll()
  })

  function drawAll() {
    if (!map) return

    // Beim load-Ereignis ist der Stil noch nicht zwangsläufig fertig – bei
    // Rasterquellen meldet isStyleLoaded() dann false. Ohne diesen zweiten
    // Anlauf bliebe die Karte für immer leer, weil sich an den Daten nichts
    // mehr ändert und der Effekt nicht erneut anspringt.
    if (!map.isStyleLoaded()) {
      map.once('idle', drawAll)
      return
    }

    setSource('area-src', area ? feature(area) : empty())
    setSource('sectors-src', sectorCollection())
    setSource('hotspots-src', hotspotCollection())
    setSource('pins-src', pinCollection())
    setSource('blur-src', blurCollection())
    setSource('teams-src', teamCollection())
    setSource('finale-src', finaleCollection())

    ensureLayers()
  }

  function empty() {
    return { type: 'FeatureCollection', features: [] }
  }

  function feature(geometry, properties = {}) {
    return {
      type: 'FeatureCollection',
      features: [{ type: 'Feature', geometry, properties }],
    }
  }

  function sectorCollection() {
    const chosen = new Set(selected)

    return {
      type: 'FeatureCollection',
      features: sectors
        .filter((s) => s.geometry)
        .map((s) => {
          const key = s.id ?? s.name
          return {
            type: 'Feature',
            geometry: typeof s.geometry === 'string' ? JSON.parse(s.geometry) : s.geometry,
            properties: {
              id: key,
              code: s.code ?? '',
              name: s.name ?? '',
              color: s.color || '#4f8ea3',
              active: key === highlight || chosen.has(key) ? 1 : 0,
            },
          }
        }),
    }
  }

  function hotspotCollection() {
    return {
      type: 'FeatureCollection',
      features: hotspots.map((h) => ({
        type: 'Feature',
        geometry: { type: 'Point', coordinates: [h.lng, h.lat] },
        properties: {
          id: h.id,
          number: h.number,
          label: String(h.number ?? '').padStart(2, '0'),
          name: h.name ?? '',
        },
      })),
    }
  }

  // Vorgemerkte Punkte: noch nicht gespeichert, deshalb optisch abgesetzt.
  function pinCollection() {
    return {
      type: 'FeatureCollection',
      features: pins.map((p, i) => ({
        type: 'Feature',
        geometry: { type: 'Point', coordinates: [p.lng, p.lat] },
        properties: { index: i, name: p.name ?? '' },
      })),
    }
  }

  function teamCollection() {
    return {
      type: 'FeatureCollection',
      features: teams.map((t) => ({
        type: 'Feature',
        geometry: { type: 'Point', coordinates: [t.lng, t.lat] },
        properties: {
          team: t.team,
          label: t.display || t.callsign,
          color: t.color || '#35c0d6',
          blurred: t.blurM > 0 ? 1 : 0,
          stale: t.stale ? 1 : 0,
        },
      })),
    }
  }

  /*
   * Unschärfekreise als echte Flächen statt als Punktsymbole: Ein Kreis mit
   * fester Pixelgröße würde beim Zoomen seine Bedeutung verlieren – hier
   * stehen 200 Meter für 200 Meter, auf jeder Zoomstufe.
   */
  function blurCollection() {
    return {
      type: 'FeatureCollection',
      features: teams
        .filter((t) => t.blurM > 0)
        .map((t) => ({
          type: 'Feature',
          geometry: { type: 'Polygon', coordinates: [circleRing(t.lat, t.lng, t.blurM)] },
          properties: { color: t.color || '#35c0d6' },
        })),
    }
  }

  // Der Suchbereich des Finales. Er schrumpft im Spielverlauf – deshalb als
  // echte Fläche, damit die Verengung auf der Karte sichtbar wird.
  function finaleCollection() {
    if (!finale?.active) return empty()

    return {
      type: 'FeatureCollection',
      features: [
        {
          type: 'Feature',
          geometry: {
            type: 'Polygon',
            coordinates: [circleRing(finale.lat, finale.lng, finale.radiusM)],
          },
          properties: {},
        },
      ],
    }
  }

  function circleRing(lat, lng, radiusM, steps = 64) {
    const earth = 6371008.8
    const dLat = (radiusM / earth) * (180 / Math.PI)
    const dLng = dLat / Math.cos((lat * Math.PI) / 180)

    const ring = []
    for (let i = 0; i <= steps; i++) {
      const a = (i / steps) * 2 * Math.PI
      ring.push([lng + dLng * Math.cos(a), lat + dLat * Math.sin(a)])
    }
    return ring
  }

  function setSource(id, data) {
    const src = map.getSource(id)
    if (src) src.setData(data)
    else map.addSource(id, { type: 'geojson', data })
  }

  function ensureLayers() {
    if (map.getLayer('sector-fill')) return

    // Spielgebiet: nur ein Rahmen, damit klar ist, wo Schluss ist.
    map.addLayer({
      id: 'area-line',
      type: 'line',
      source: 'area-src',
      paint: {
        'line-color': '#d9a441',
        'line-width': 1.5,
        'line-dasharray': [3, 2],
        'line-opacity': 0.75,
      },
    })

    map.addLayer({
      id: 'sector-fill',
      type: 'fill',
      source: 'sectors-src',
      paint: {
        'fill-color': ['get', 'color'],
        'fill-opacity': ['case', ['==', ['get', 'active'], 1], 0.42, 0.16],
      },
    })

    map.addLayer({
      id: 'sector-line',
      type: 'line',
      source: 'sectors-src',
      paint: {
        'line-color': ['get', 'color'],
        'line-width': ['case', ['==', ['get', 'active'], 1], 2.5, 1.2],
        'line-opacity': 0.9,
      },
    })

    map.addLayer({
      id: 'sector-label',
      type: 'symbol',
      source: 'sectors-src',
      layout: {
        'text-field': ['get', 'code'],
        'text-size': 15,
        'text-letter-spacing': 0.12,
        'text-allow-overlap': false,
      },
      paint: {
        'text-color': '#eef4f6',
        'text-halo-color': '#0b1215',
        'text-halo-width': 1.6,
      },
    })

    // Hotspots als nummerierte Ringe – so stehen sie auch auf dem Kartenblatt.
    map.addLayer({
      id: 'hotspot-dot',
      type: 'circle',
      source: 'hotspots-src',
      paint: {
        'circle-radius': ['interpolate', ['linear'], ['zoom'], 10, 5, 15, 11],
        'circle-color': '#0b1215',
        'circle-stroke-color': '#d9a441',
        'circle-stroke-width': 1.6,
        'circle-opacity': 0.92,
      },
    })

    map.addLayer({
      id: 'hotspot-label',
      type: 'symbol',
      source: 'hotspots-src',
      minzoom: 12.5,
      layout: {
        'text-field': ['get', 'label'],
        'text-size': 10,
        'text-allow-overlap': true,
      },
      paint: { 'text-color': '#d9a441' },
    })

    // Vorgemerkte Punkte heben sich ab: gefüllt statt hohl, damit auf einen
    // Blick klar ist, was schon gespeichert ist und was noch nicht.
    map.addLayer({
      id: 'pin-dot',
      type: 'circle',
      source: 'pins-src',
      paint: {
        'circle-radius': ['interpolate', ['linear'], ['zoom'], 10, 5, 15, 10],
        'circle-color': '#4ec9a0',
        'circle-stroke-color': '#0b1215',
        'circle-stroke-width': 1.5,
      },
    })

    map.on('click', 'pin-dot', (e) => {
      const f = e.features?.[0]
      if (f && onHotspotClick) onHotspotClick({ ...f.properties, pin: true })
    })

    map.addLayer({
      id: 'finale-area',
      type: 'fill',
      source: 'finale-src',
      paint: { 'fill-color': '#d9a441', 'fill-opacity': 0.1 },
    })

    map.addLayer({
      id: 'finale-edge',
      type: 'line',
      source: 'finale-src',
      paint: { 'line-color': '#d9a441', 'line-width': 2, 'line-dasharray': [4, 2] },
    })

    // Teams liegen über allem anderen – sie sind der Grund, warum jemand
    // überhaupt auf die Karte schaut.
    map.addLayer({
      id: 'blur-area',
      type: 'fill',
      source: 'blur-src',
      paint: { 'fill-color': ['get', 'color'], 'fill-opacity': 0.14 },
    })

    map.addLayer({
      id: 'blur-edge',
      type: 'line',
      source: 'blur-src',
      paint: {
        'line-color': ['get', 'color'],
        'line-width': 1.2,
        'line-dasharray': [2, 2],
        'line-opacity': 0.7,
      },
    })

    map.addLayer({
      id: 'team-dot',
      type: 'circle',
      source: 'teams-src',
      paint: {
        // Eine unscharfe Position bekommt keinen harten Punkt: Der Kreis ist
        // die Aussage, nicht sein Mittelpunkt.
        'circle-radius': ['case', ['==', ['get', 'blurred'], 1], 4, 7],
        'circle-color': ['get', 'color'],
        'circle-opacity': ['case', ['==', ['get', 'stale'], 1], 0.45, 1],
        'circle-stroke-color': '#0b1215',
        'circle-stroke-width': 2,
      },
    })

    map.addLayer({
      id: 'team-label',
      type: 'symbol',
      source: 'teams-src',
      layout: {
        'text-field': ['get', 'label'],
        'text-size': 11,
        'text-offset': [0, 1.3],
        'text-anchor': 'top',
        'text-allow-overlap': false,
      },
      paint: {
        'text-color': ['get', 'color'],
        'text-halo-color': '#0b1215',
        'text-halo-width': 1.6,
      },
    })

    map.on('click', 'hotspot-dot', (e) => {
      const f = e.features?.[0]
      if (f && onHotspotClick) onHotspotClick(f.properties)
    })

    map.on('click', 'sector-fill', (e) => {
      const f = e.features?.[0]
      if (f && onSectorClick) onSectorClick(f.properties)
    })

    // Klick auf freie Fläche setzt einen neuen Punkt – aber nur, wenn nicht
    // gerade ein vorhandener Hotspot getroffen wurde.
    map.on('click', (e) => {
      if (!onMapClick) return
      const hits = map.queryRenderedFeatures(e.point, {
        layers: ['hotspot-dot', 'pin-dot'].filter((l) => map.getLayer(l)),
      })
      if (hits.length > 0) return
      onMapClick({ lat: e.lngLat.lat, lng: e.lngLat.lng })
    })

    for (const layer of ['hotspot-dot', 'sector-fill']) {
      map.on('mouseenter', layer, () => (map.getCanvas().style.cursor = 'pointer'))
      map.on('mouseleave', layer, () => (map.getCanvas().style.cursor = ''))
    }
  }

  /** Springt auf ein Rechteck – wird von der Setup-Ansicht aufgerufen. */
  export function fitBounds(bounds, padding = 40) {
    if (!map || !bounds) return
    map.fitBounds(
      [
        [bounds.west, bounds.south],
        [bounds.east, bounds.north],
      ],
      { padding, duration: 600 },
    )
  }
</script>

<div class="map-wrap">
  <div bind:this={container} class="map"></div>

  {#if failed}
    <div class="overlay error">{failed}</div>
  {:else if !ready}
    <div class="overlay mono">Lagekarte wird aufgebaut …</div>
  {/if}
</div>

<style>
  .map-wrap {
    position: relative;
    width: 100%;
    height: 100%;
    min-height: 18rem;
    border: 1px solid var(--rule);
    background: var(--ground);
    overflow: hidden;
  }

  .map {
    position: absolute;
    inset: 0;
  }

  .overlay {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    background: var(--ground);
    color: var(--muted);
    font-size: var(--fs-sm);
    pointer-events: none;
  }

  .overlay.error {
    color: var(--x);
  }

  /* MapLibre bringt eigene helle Bedienelemente mit – die passen wir an. */
  .map-wrap :global(.maplibregl-ctrl-group) {
    background: var(--sheet);
    border: 1px solid var(--rule-hi);
    border-radius: var(--radius);
    box-shadow: none;
  }

  .map-wrap :global(.maplibregl-ctrl-group button) {
    min-height: 30px;
    background: transparent;
    border: none;
  }

  .map-wrap :global(.maplibregl-ctrl-group button + button) {
    border-top: 1px solid var(--rule);
  }

  .map-wrap :global(.maplibregl-ctrl-group button span) {
    filter: invert(1) brightness(1.6);
  }

  .map-wrap :global(.maplibregl-ctrl-attrib) {
    background: rgba(11, 18, 21, 0.82);
    color: var(--muted);
    font-size: 10px;
  }

  .map-wrap :global(.maplibregl-ctrl-attrib a) {
    color: var(--muted);
  }
</style>
