<script>
  /*
   * Einrichtung des Spielfelds.
   *
   * Drei Schritte: Stadt suchen, Verwaltungsebene wählen, Sektoren anklicken.
   * Welche Ebene brauchbare Sektoren ergibt, unterscheidet sich je Stadt –
   * deshalb kommen alle mit Anzahl und typischer Größe zur Auswahl, statt eine
   * zu raten. In Dresden etwa sind die zehn Ortsamtsbereiche mit 20 bis 160 km²
   * unbrauchbar, erst die Ebene darunter trägt die spielbaren Stadtteile.
   */
  import { api } from '../lib/session.svelte.js'
  import Map from '../components/Map.svelte'

  let { onSaved = null } = $props()

  let query = $state('Dresden')
  let places = $state([])
  let city = $state(null)

  let levels = $state([])
  let activeLevel = $state(null)
  let selected = $state(new Set())
  let filter = $state('')

  let busy = $state(null)
  let error = $state(null)

  /*
   * Fertige Entwürfe – aus dem Programm oder aus einer KI.
   *
   * Für einige Städte liegen Sektoren und Hotspots schon bei, aus denselben
   * OpenStreetMap-Daten, nur vorher geholt. Für alle anderen gibt es den
   * Umweg über eine Sprachmaschine: Der Text dafür steht unten zum Kopieren,
   * ihre Antwort kommt hier wieder herein.
   *
   * Beides endet an derselben Stelle: einem Entwurf mit einem Haken vor jedem
   * Eintrag. Nichts davon wird gespeichert, bevor jemand ihn durchgesehen hat
   * – weder das Paket, das die Stadt nur aus Daten kennt, noch die Maschine,
   * die nie dort war.
   */
  let sets = $state([])
  let setNote = $state(null)
  let saved = $state(null)
  let mapRef = $state(null)

  // Der offene Entwurf.
  let pack = $state(null)
  let onSectors = $state(new Set())
  let onSpots = $state(new Set())
  let onPuzzles = $state(new Set())

  // Der Text für die KI und das, was zurückkommt.
  let kiOpen = $state(false)
  let kiText = $state('')
  let promptText = $state('')
  let promptBox = $state(null)
  let kopierHinweis = $state(null)

  /** Nominatim liefert "Dresden, Sachsen, Deutschland" – gemeint ist das erste Wort. */
  const stadtName = $derived(((city?.name ?? query) || '').split(',')[0].trim())

  const packDabei = $derived(
    sets.find((s) => s.city.toLowerCase() === stadtName.toLowerCase()) ?? null,
  )

  function umschalten(menge, i) {
    const next = new Set(menge)
    next.has(i) ? next.delete(i) : next.add(i)
    return next
  }

  /*
   * Selbst zeichnen.
   *
   * Die echten Stadtteilgrenzen sind der Normalfall und bleiben es – sie sind
   * genauer, als jemand mit dem Finger hinbekommt, und jeder kennt ihre Namen.
   * Aber sie passen nicht überall: Manche Städte haben keine brauchbare Ebene,
   * manche Stadtteile sind für ein Spiel zu groß, und mancher Spielplan will
   * einen Sektor, den die Verwaltung so nicht kennt – "innerhalb des Rings",
   * "alles nördlich der Bahn".
   *
   * Deshalb ein zweiter Weg neben dem ersten, nicht statt seiner: Beides lässt
   * sich mischen, und gespeichert wird gemeinsam.
   */
  let drawing = $state(false)
  let corners = $state([])
  let drawn = $state([])
  let drawName = $state('')

  const drawPreview = $derived(
    corners.length >= 3
      ? [{
          id: '__entwurf',
          name: drawName || 'Entwurf',
          code: '',
          color: '#d9a441',
          geometry: ringToPolygon(corners),
        }]
      : [],
  )

  /** Aus Ecken ein geschlossenes GeoJSON-Polygon machen. */
  function ringToPolygon(points) {
    const ring = points.map((p) => [p.lng, p.lat])
    ring.push(ring[0])
    return { type: 'Polygon', coordinates: [ring] }
  }

  function startDrawing() {
    drawing = true
    corners = []
    drawName = ''
  }

  function cancelDrawing() {
    drawing = false
    corners = []
    drawName = ''
  }

  function addCorner(point) {
    if (!drawing) return
    corners = [...corners, point]
  }

  function undoCorner() {
    corners = corners.slice(0, -1)
  }

  function closeShape() {
    if (corners.length < 3) return
    const name = drawName.trim() || `Eigener Sektor ${drawn.length + 1}`
    drawn = [...drawn, { name, geometry: ringToPolygon(corners) }]
    corners = []
    drawName = ''
    drawing = false
  }

  function removeDrawn(index) {
    drawn = drawn.filter((_, i) => i !== index)
  }

  /** Grobe Flächenangabe, damit man merkt, wenn ein Sektor zu groß gerät. */
  function areaKm2(polygon) {
    const ring = polygon.coordinates[0]
    let sum = 0
    for (let i = 0; i < ring.length - 1; i++) {
      const [x1, y1] = ring[i]
      const [x2, y2] = ring[i + 1]
      sum += x1 * y2 - x2 * y1
    }
    // Von Grad in Quadratkilometer, an der Breite dieses Rings.
    const lat = (ring.reduce((a, p) => a + p[1], 0) / ring.length) * (Math.PI / 180)
    const kmPerDegLat = 111.32
    const kmPerDegLng = 111.32 * Math.cos(lat)
    return Math.abs(sum / 2) * kmPerDegLat * kmPerDegLng
  }

  const districts = $derived(
    levels.find((l) => l.level === activeLevel)?.districts ?? [],
  )

  const visible = $derived(
    filter.trim()
      ? districts.filter((d) => d.name.toLowerCase().includes(filter.trim().toLowerCase()))
      : districts,
  )

  const chosen = $derived(districts.filter((d) => selected.has(String(d.osmId))))

  const totalArea = $derived(chosen.reduce((sum, d) => sum + d.areaKm2, 0))

  async function loadSets() {
    try {
      sets = (await api.get('/api/opx/setup/citysets')).sets ?? []
    } catch {
      /* ohne Pakete geht die Einrichtung von Hand weiter */
    }
  }

  $effect(() => {
    loadSets()
  })

  /** Ein mitgeliefertes Paket öffnen – als Entwurf, nicht als Tatsache. */
  async function openSet(set) {
    busy = `Öffne ${set.city} …`
    error = null
    try {
      zeigeEntwurf(await api.get(`/api/opx/setup/citysets/${set.slug}`), 'Stadtpaket')
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  function zeigeEntwurf(p, herkunft) {
    pack = { ...p, herkunft }
    onSectors = new Set((p.sectors ?? []).map((_, i) => i))
    onSpots = new Set((p.hotspots ?? []).map((_, i) => i))
    onPuzzles = new Set((p.puzzles ?? []).map((_, i) => i))
    setNote = null
    saved = null
    kiOpen = false
    mapRef?.fitBounds(entwurfsRahmen(p))
  }

  /**
   * Alle Eckpunkte einer Geometrie, gleich wie tief sie verschachtelt ist.
   *
   * Ein Polygon liegt zwei Ebenen tief, ein MultiPolygon drei – und was eine
   * KI liefert, weiß man vorher nicht. Ohne diese Unterscheidung käme aus dem
   * einen Fall eine Zahl und aus dem anderen ein Array, und die Karte spränge
   * ins Nichts.
   */
  function ecken(koordinaten) {
    if (!Array.isArray(koordinaten)) return []
    if (typeof koordinaten[0] === 'number' && typeof koordinaten[1] === 'number') {
      return [koordinaten]
    }
    return koordinaten.flatMap(ecken)
  }

  /** Umschließendes Rechteck über alles, was der Entwurf enthält. */
  function entwurfsRahmen(p) {
    const punkte = [
      ...(p.hotspots ?? []).map((h) => [h.lng, h.lat]),
      ...(p.sectors ?? []).flatMap((s) => ecken(s.geometry?.coordinates)),
    ].filter((c) => Number.isFinite(c[0]) && Number.isFinite(c[1]))

    if (punkte.length === 0) return null
    return {
      west: Math.min(...punkte.map((c) => c[0])),
      east: Math.max(...punkte.map((c) => c[0])),
      south: Math.min(...punkte.map((c) => c[1])),
      north: Math.max(...punkte.map((c) => c[1])),
    }
  }

  function verwerfen() {
    pack = null
    kiText = ''
  }

  /*
   * Übernehmen.
   *
   * Die Reihenfolge ist nicht beliebig: Erst die Sektoren, dann die Hotspots –
   * der Server ordnet jeden Punkt beim Speichern dem Sektor zu, in dem er
   * liegt, und kann das nur, wenn die Grenzen schon stehen.
   */
  async function entwurfUebernehmen() {
    const sektoren = (pack.sectors ?? []).filter((_, i) => onSectors.has(i))
    const spots = (pack.hotspots ?? []).filter((_, i) => onSpots.has(i))
    const raetsel = (pack.puzzles ?? []).filter((_, i) => onPuzzles.has(i))
    if (sektoren.length === 0 && spots.length === 0 && raetsel.length === 0) return

    const ersetzt = [
      sektoren.length > 0 ? 'Sektoren' : null,
      spots.length > 0 ? 'Hotspots' : null,
    ].filter(Boolean)

    const frage =
      `${sektoren.length} Sektoren, ${spots.length} Hotspots und ${raetsel.length} Rätsel übernehmen?\n\n` +
      (ersetzt.length > 0
        ? `Vorhandene ${ersetzt.join(' und ')} dieses Spiels werden dabei ersetzt.`
        : 'Vorhandene Rätsel bleiben; die neuen kommen dazu.')
    if (!confirm(frage)) return

    busy = 'Übernehme den Entwurf …'
    error = null

    try {
      let res = null
      if (sektoren.length > 0) {
        res = await api.post('/api/opx/setup/sectors', {
          replace: true,
          sectors: sektoren.map((s) => ({ name: s.name, geometry: s.geometry })),
        })
        saved = res
      }
      if (spots.length > 0) {
        await api.post('/api/opx/setup/hotspots', {
          replace: true,
          hotspots: spots.map((h) => ({
            name: h.name,
            lat: h.lat,
            lng: h.lng,
            kind: h.kind ?? 'other',
            notes: h.notes ?? '',
          })),
        })
      }
      for (const r of raetsel) {
        await api.post('/api/opx/setup/puzzles', { ...r, unlocked: false })
      }

      setNote =
        `${sektoren.length} Sektoren, ${spots.length} Hotspots und ${raetsel.length} Rätsel übernommen. ` +
        'Seht sie euch auf der Karte an, bevor ihr losspielt — besonders die Hotspots.'
      pack = null
      kiText = ''
      // Zweites Argument: hierbleiben. Der Hinweis über den Hotspots ist der
      // wichtigste Satz des ganzen Vorgangs – er darf nicht mit der Seite
      // verschwinden, die ihn anzeigt.
      if (res) onSaved?.(res, true)
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  /*
   * Der Text für die KI.
   *
   * Er kommt vom Server und nicht von hier: Dort steht auch die Auswertung der
   * Antwort, und ein Format, das an zwei Stellen beschrieben ist, driftet
   * auseinander.
   */
  async function kiOeffnen() {
    kiOpen = !kiOpen
    if (!kiOpen || promptText) return
    try {
      const res = await api.get(
        `/api/opx/setup/plan?city=${encodeURIComponent(stadtName || '')}`,
      )
      promptText = res.prompt
    } catch (err) {
      error = err.message
    }
  }

  /*
   * Kopieren ohne Zwischenablage-Rechte.
   *
   * Der Server läuft im Heimnetz über eine einfache Verbindung, und der
   * Browser gibt navigator.clipboard nur auf gesicherten Seiten heraus. Dann
   * bleibt der alte Weg über die Textmarkierung – und wenn auch der abgelehnt
   * wird, ist der Text wenigstens markiert, und es steht da, was zu tun ist.
   * Ein Knopf, der nichts tut und nichts sagt, ist schlimmer als keiner.
   */
  async function promptKopieren() {
    try {
      await navigator.clipboard.writeText(promptText)
      kopierHinweis = 'Kopiert.'
    } catch {
      promptBox?.focus()
      promptBox?.select()
      kopierHinweis = document.execCommand?.('copy')
        ? 'Kopiert.'
        : 'Der Text ist markiert — jetzt mit Strg+C kopieren.'
    }
    setTimeout(() => (kopierHinweis = null), 8000)
  }

  async function kiAuswerten() {
    busy = 'Lese die Antwort …'
    error = null
    try {
      zeigeEntwurf(await api.post('/api/opx/setup/plan', { text: kiText }), 'KI-Entwurf')
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  async function searchCity() {
    if (query.trim().length < 2) return
    busy = 'Suche die Stadt …'
    error = null
    places = []

    try {
      const res = await api.get(`/api/opx/setup/cities?q=${encodeURIComponent(query.trim())}`)
      places = res.places ?? []
      if (places.length === 0) error = 'Kein Ort mit Grenzen gefunden. Anderen Namen versuchen.'
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  async function pickCity(place) {
    city = place
    places = []
    levels = []
    activeLevel = null
    selected = new Set()
    error = null
    busy = 'Frage die Ortsteilgrenzen ab. Das dauert einen Moment …'

    mapRef?.fitBounds(place.bounds)

    try {
      const res = await api.post('/api/opx/setup/districts', {
        osmId: place.osmId,
        levels: [9, 10, 11],
      })
      levels = res.levels ?? []
      // Die Ebene vorschlagen, deren Gebiete am ehesten Spielgröße haben.
      activeLevel = bestLevel(levels)
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  /**
   * Ein Sektor, den man zu Fuß durchqueren kann, liegt bei ein bis drei
   * Quadratkilometern. Gewählt wird die Ebene, deren typische Größe dem am
   * nächsten kommt.
   */
  function bestLevel(list) {
    if (list.length === 0) return null
    const ideal = 2
    return list.reduce((best, l) =>
      Math.abs(l.medianKm2 - ideal) < Math.abs(best.medianKm2 - ideal) ? l : best,
    ).level
  }

  function toggle(osmId) {
    const key = String(osmId)
    const next = new Set(selected)
    next.has(key) ? next.delete(key) : next.add(key)
    selected = next
  }

  function selectAll() {
    selected = new Set(visible.map((d) => String(d.osmId)))
  }

  function selectNone() {
    selected = new Set()
  }

  async function save() {
    if (chosen.length === 0 && drawn.length === 0) return
    busy = 'Speichere die Sektoren …'
    error = null

    try {
      const res = await api.post('/api/opx/setup/sectors', {
        replace: true,
        sectors: [
          ...chosen.map((d) => ({ name: d.name, geometry: d.geometry })),
          ...drawn.map((d) => ({ name: d.name, geometry: d.geometry })),
        ],
      })
      saved = res
      onSaved?.(res)
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  /*
   * Kartenvorschau.
   *
   * Liegt ein Entwurf offen, zeigt die Karte ihn und sonst nichts: Sektoren
   * abgewählter Einträge bleiben sichtbar, aber blass, damit man sie mit einem
   * Klick zurückholen kann. Hotspots verschwinden beim Abwählen ganz – die
   * Karte zeigt damit genau das, was beim Übernehmen entsteht.
   */
  const preview = $derived(
    pack
      ? (pack.sectors ?? []).map((s, i) => ({
          id: String(i),
          name: s.name,
          code: '',
          color: '#d9a441',
          geometry: s.geometry,
        }))
      : [
          ...districts.map((d) => ({
            id: String(d.osmId),
            name: d.name,
            code: '',
            color: '#4f8ea3',
            geometry: d.geometry,
          })),
          ...drawn.map((d, i) => ({
            id: `__gezeichnet-${i}`,
            name: d.name,
            code: '',
            color: '#d9a441',
            geometry: d.geometry,
          })),
          ...drawPreview,
        ],
  )

  // Die Hotspots des Entwurfs, durchnummeriert wie später im Spiel.
  const packSpots = $derived(
    (pack?.hotspots ?? [])
      .map((h, i) => ({ ...h, i }))
      .filter((h) => onSpots.has(h.i))
      .map((h, n) => ({ id: String(h.i), number: n + 1, name: h.name, lat: h.lat, lng: h.lng })),
  )

  const markiert = $derived(pack ? [...onSectors].map(String) : [...selected])
</script>

<div class="setup">
  <div class="panel-col">
    {#if !pack}
      <section class="step packs">
        <header>
          <span class="num mono">00</span>
          <h3>Fertiger Plan</h3>
        </header>

        {#if packDabei}
          <div class="verfuegbar">
            <p>
              <strong>Stadtpaket verfügbar: {packDabei.city}</strong><br />
              {packDabei.sectors} Sektoren und {packDabei.hotspots} Hotspots sind
              schon dabei — ohne Internet, in Sekunden eingerichtet.
            </p>
            <button class="primary small" onclick={() => openSet(packDabei)} disabled={!!busy}>
              Paket ansehen
            </button>
          </div>
        {/if}

        {#if sets.length > 0}
          <p class="dim">
            {packDabei ? 'Weitere Städte:' : 'Für diese Städte ist alles vorbereitet:'}
          </p>
          <div class="packlist">
            {#each sets as set (set.slug)}
              <button
                class="pack"
                class:dabei={set.slug === packDabei?.slug}
                onclick={() => openSet(set)}
                disabled={!!busy}
              >
                <span class="pack-city">{set.city}</span>
                <span class="pack-meta mono">{set.sectors} Sektoren · {set.hotspots} Hotspots</span>
              </button>
            {/each}
          </div>
        {/if}

        <button class="ghost" onclick={kiOeffnen}>
          {kiOpen ? 'KI-Auftrag zuklappen' : 'Eure Stadt ist nicht dabei? Von einer KI planen lassen'}
        </button>

        {#if kiOpen}
          <div class="ki">
            <p class="hint">
              Diesen Auftrag in eine KI einfügen — eine, die im Netz nachsehen
              kann, liefert bessere Koordinaten. Ihre Antwort kommt unten wieder
              herein und landet als Entwurf auf der Karte, nicht im Spiel.
            </p>

            <textarea
              class="mono prompt"
              bind:this={promptBox}
              readonly
              rows="5"
              value={promptText}
            ></textarea>

            <button class="ghost small" onclick={promptKopieren} disabled={!promptText}>
              Auftrag kopieren
            </button>

            {#if kopierHinweis}<p class="note">{kopierHinweis}</p>{/if}

            <textarea
              bind:value={kiText}
              rows="4"
              placeholder="Antwort der KI hier einfügen …"
            ></textarea>

            <button
              class="primary small"
              onclick={kiAuswerten}
              disabled={!kiText.trim() || !!busy}
            >
              Antwort einlesen
            </button>
          </div>
        {/if}

        {#if setNote}<p class="note">{setNote}</p>{/if}
      </section>
    {/if}

    {#if pack}
      <section class="step grow">
        <header>
          <span class="num mono">00</span>
          <h3>{pack.herkunft}{pack.city ? `: ${pack.city}` : ''}</h3>
          <span class="count mono">{onSectors.size + onSpots.size + onPuzzles.size} gewählt</span>
        </header>

        <p class="hint">
          Haken wegnehmen, was nicht passt — auf der Karte lässt sich beides auch
          anklicken. Gespeichert ist noch nichts.
        </p>

        {#if pack.warnings?.length}
          <ul class="warnungen">
            {#each pack.warnings as w, i (i)}<li>{w}</li>{/each}
          </ul>
        {/if}

        {#if pack.sectors?.length}
          <h4>Sektoren <span class="mono dim">{onSectors.size} von {pack.sectors.length}</span></h4>
          <ul class="districts">
            {#each pack.sectors as s, i (i)}
              <li>
                <label class="district" class:on={onSectors.has(i)}>
                  <input
                    type="checkbox"
                    checked={onSectors.has(i)}
                    onchange={() => (onSectors = umschalten(onSectors, i))}
                  />
                  <span class="d-name">{s.name}</span>
                  {#if s.areaKm2}<span class="d-area mono">{s.areaKm2.toFixed(1)} km²</span>{/if}
                </label>
              </li>
            {/each}
          </ul>
        {/if}

        {#if pack.hotspots?.length}
          <h4>Hotspots <span class="mono dim">{onSpots.size} von {pack.hotspots.length}</span></h4>
          <ul class="districts">
            {#each pack.hotspots as h, i (i)}
              <li>
                <label class="district" class:on={onSpots.has(i)}>
                  <input
                    type="checkbox"
                    checked={onSpots.has(i)}
                    onchange={() => (onSpots = umschalten(onSpots, i))}
                  />
                  <span class="d-name">
                    {h.name}
                    {#if h.notes}<span class="aufgabe">{h.notes}</span>{/if}
                  </span>
                </label>
              </li>
            {/each}
          </ul>
        {/if}

        {#if pack.puzzles?.length}
          <h4>Rätsel <span class="mono dim">{onPuzzles.size} von {pack.puzzles.length}</span></h4>
          <ul class="districts">
            {#each pack.puzzles as r, i (i)}
              <li>
                <label class="district" class:on={onPuzzles.has(i)}>
                  <input
                    type="checkbox"
                    checked={onPuzzles.has(i)}
                    onchange={() => (onPuzzles = umschalten(onPuzzles, i))}
                  />
                  <span class="d-name">
                    {r.title}
                    <span class="aufgabe">Typ {r.type} · Lösung: {r.answer}</span>
                  </span>
                </label>
              </li>
            {/each}
          </ul>
        {/if}

        <div class="save-row">
          <button class="ghost small" onclick={verwerfen}>Verwerfen</button>
          <button class="primary" onclick={entwurfUebernehmen} disabled={!!busy}>
            Übernehmen
          </button>
        </div>
      </section>
    {/if}

    <!-- Solange ein Entwurf offen ist, gehört die Karte ihm. -->
    {#if !pack}
      <!-- Schritt 1 -->
      <section class="step">
        <header>
          <span class="num mono">01</span>
          <h3>Stadt</h3>
        </header>

        {#if city}
          <div class="chosen-city">
            <span class="name">{city.name}</span>
            <button class="ghost small" onclick={() => { city = null; levels = []; selected = new Set() }}>
              Ändern
            </button>
          </div>
        {:else}
          <div class="row">
            <input
              bind:value={query}
              placeholder="Dresden"
              onkeydown={(e) => e.key === 'Enter' && searchCity()}
            />
            <button onclick={searchCity} disabled={!!busy}>Suchen</button>
          </div>

          {#if places.length}
            <ul class="hits">
              {#each places as p (p.osmId)}
                <li>
                  <button class="hit" onclick={() => pickCity(p)}>
                    <span class="hit-name">{p.name}</span>
                    <span class="hit-id mono">R{p.osmId}</span>
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        {/if}
      </section>

      <!-- Schritt 2 -->
      {#if levels.length}
        <section class="step">
          <header>
            <span class="num mono">02</span>
            <h3>Ebene</h3>
          </header>
          <p class="note">
            Ein Sektor sollte zu Fuß zu durchqueren sein – ein bis drei Quadratkilometer.
          </p>

          <div class="levels">
            {#each levels as l (l.level)}
              <button
                class="level"
                class:active={l.level === activeLevel}
                onclick={() => { activeLevel = l.level; selected = new Set() }}
              >
                <span class="lv mono">Ebene {l.level}</span>
                <span class="lc">{l.count} Gebiete</span>
                <span class="la mono">⌀ {l.medianKm2.toFixed(1)} km²</span>
              </button>
            {/each}
          </div>
        </section>
      {/if}

      <!-- Schritt 3 -->
      {#if districts.length}
        <section class="step grow">
          <header>
            <span class="num mono">03</span>
            <h3>Sektoren</h3>
            <span class="count mono">{chosen.length} gewählt</span>
          </header>

          <div class="row">
            <input bind:value={filter} placeholder="Filtern …" />
            <button class="ghost small" onclick={selectAll}>Alle</button>
            <button class="ghost small" onclick={selectNone}>Keine</button>
          </div>

          <ul class="districts">
            {#each visible as d (d.osmId)}
              <li>
                <label class="district" class:on={selected.has(String(d.osmId))}>
                  <input
                    type="checkbox"
                    checked={selected.has(String(d.osmId))}
                    onchange={() => toggle(d.osmId)}
                  />
                  <span class="d-name">{d.name}</span>
                  <span class="d-area mono">{d.areaKm2.toFixed(1)} km²</span>
                </label>
              </li>
            {/each}
          </ul>

          <div class="save-row">
            <span class="total mono">
              {chosen.length + drawn.length > 0
                ? `${(totalArea + drawn.reduce((a, d) => a + areaKm2(d.geometry), 0)).toFixed(1)} km² Spielfläche`
                : 'Nichts gewählt'}
            </span>
            <button
              class="primary"
              onclick={save}
              disabled={(chosen.length === 0 && drawn.length === 0) || !!busy}
            >
              Sektoren übernehmen
            </button>
          </div>
        </section>
      {/if}

      <!-- Schritt 3b: selbst zeichnen -->
      {#if city}
        <section class="step">
          <header>
            <span class="num mono">3b</span>
            <h3>Selbst zeichnen</h3>
          </header>

          <p class="hint">
            Wenn die Stadtteilgrenzen nicht passen: Ecken auf die Karte klicken,
            Fläche schließen. Lässt sich mit gewählten Stadtteilen mischen.
          </p>

          {#if !drawing}
            <button class="ghost" onclick={startDrawing}>Fläche zeichnen</button>
          {:else}
            <div class="draw-box">
              <p class="draw-state mono">
                {corners.length === 0
                  ? 'Erste Ecke auf die Karte klicken'
                  : `${corners.length} ${corners.length === 1 ? 'Ecke' : 'Ecken'}${corners.length < 3 ? ' — mindestens drei' : ''}`}
              </p>

              <input bind:value={drawName} placeholder="Name, z. B. Innenstadtring" />

              <div class="draw-row">
                <button class="ghost small" onclick={undoCorner} disabled={corners.length === 0}>
                  Ecke zurück
                </button>
                <button class="ghost small" onclick={cancelDrawing}>Abbrechen</button>
                <button class="primary small" onclick={closeShape} disabled={corners.length < 3}>
                  Fläche schließen
                </button>
              </div>
            </div>
          {/if}

          {#if drawn.length > 0}
            <ul class="drawn">
              {#each drawn as d, i (d.name + i)}
                <li>
                  <span class="d-name">{d.name}</span>
                  <span class="d-area mono">{areaKm2(d.geometry).toFixed(1)} km²</span>
                  <button class="ghost small" onclick={() => removeDrawn(i)}>Entfernen</button>
                </li>
              {/each}
            </ul>
          {/if}
        </section>
      {/if}
    {/if}

    {#if busy}
      <p class="status mono">{busy}</p>
    {/if}
    {#if error}
      <p class="status error" role="alert">{error}</p>
    {/if}
    {#if saved}
      <p class="status ok mono">
        {saved.saved} Sektoren gespeichert: {saved.sectors.map((s) => s.code).join(' ')}
      </p>
    {/if}
  </div>

  <div class="map-col">
    <Map
      bind:this={mapRef}
      sectors={preview}
      hotspots={packSpots}
      selected={markiert}
      pins={corners}
      onSectorClick={(p) =>
        pack ? (onSectors = umschalten(onSectors, Number(p.id))) : !drawing && toggle(p.id)}
      onHotspotClick={(p) => pack && (onSpots = umschalten(onSpots, Number(p.id)))}
      onMapClick={addCorner}
    />
  </div>
</div>

<style>
  .packlist {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
  }

  .pack {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.1rem;
    padding: 0.45rem 0.7rem;
    background: var(--sheet);
    border: 1px solid var(--rule-hi);
    cursor: pointer;
    text-align: left;
  }

  .pack:hover:not(:disabled) {
    border-color: var(--hq);
  }

  .pack-city {
    font-family: var(--display);
    letter-spacing: 0.04em;
  }

  .pack-meta {
    font-size: var(--fs-xs);
    color: var(--muted);
  }

  /* Das Paket zur gesuchten Stadt – die Antwort auf die Frage, die gerade
     gestellt wurde, und deshalb hervorgehoben. */
  .verfuegbar {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    background: var(--sheet-2);
    border: 1px solid var(--rule);
    border-left: 2px solid var(--hq);
    padding: 0.6rem 0.7rem;
  }

  .verfuegbar p {
    margin: 0;
    font-size: var(--fs-sm);
  }

  .pack.dabei {
    border-color: var(--hq);
  }

  .ki {
    display: flex;
    flex-direction: column;
    gap: 0.45rem;
    background: var(--sheet-2);
    border: 1px solid var(--rule);
    padding: 0.6rem 0.7rem;
  }

  .prompt {
    font-size: var(--fs-xs);
    line-height: 1.45;
    color: var(--muted);
  }

  .warnungen {
    list-style: none;
    margin: 0;
    padding: 0.5rem 0.7rem;
    background: var(--sheet-2);
    border-left: 2px solid var(--warn);
    font-size: var(--fs-xs);
    color: var(--warn);
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  /* Die Aufgabe steht unter dem Ort, nicht daneben: Sie ist ein ganzer Satz
     und würde jede Zeile sprengen. */
  .aufgabe {
    display: block;
    font-size: var(--fs-xs);
    color: var(--muted);
    line-height: 1.35;
  }

  h4 {
    margin: 0.5rem 0 0;
    font-family: var(--display);
    font-size: var(--fs-sm);
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  h4 .dim {
    text-transform: none;
    letter-spacing: 0;
  }

  .setup {
    display: grid;
    grid-template-columns: minmax(19rem, 24rem) 1fr;
    gap: 1rem;
    flex: 1;
    min-height: 0;
  }

  .panel-col {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    min-height: 0;
    overflow-y: auto;
  }

  .draw-box {
    display: flex;
    flex-direction: column;
    gap: 0.45rem;
    background: var(--sheet-2);
    border: 1px solid var(--rule);
    border-left: 2px solid var(--hq);
    padding: 0.6rem 0.7rem;
  }

  .draw-state {
    font-size: var(--fs-xs);
    color: var(--hq);
  }

  .draw-row {
    display: flex;
    gap: 0.4rem;
  }

  .draw-row button {
    flex: 1;
  }

  .drawn {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .drawn li {
    display: grid;
    grid-template-columns: 1fr auto auto;
    gap: 0.5rem;
    align-items: center;
    background: var(--sheet-2);
    border-left: 2px solid var(--hq);
    padding: 0.3rem 0.5rem;
    font-size: var(--fs-sm);
  }

  button.small {
    min-height: 2rem;
    padding: 0 0.6rem;
    font-size: var(--fs-xs);
  }

  .map-col {
    min-height: 24rem;
  }

  .step {
    background: var(--sheet);
    border: 1px solid var(--rule);
    padding: 0.85rem 0.9rem;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .step.grow {
    flex: 1;
    min-height: 0;
  }

  .step header {
    display: flex;
    align-items: baseline;
    gap: 0.6rem;
    border-bottom: 1px solid var(--rule);
    padding-bottom: 0.5rem;
  }

  .num {
    font-size: var(--fs-xs);
    color: var(--hq);
  }

  h3 {
    font-size: var(--fs-base);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }

  .count {
    margin-left: auto;
    font-size: var(--fs-xs);
    color: var(--muted);
  }

  .note {
    font-size: var(--fs-xs);
    color: var(--muted);
    line-height: 1.45;
  }

  .row {
    display: flex;
    gap: 0.4rem;
  }

  .row input {
    flex: 1;
    min-width: 0;
  }

  button.small {
    min-height: 2.2rem;
    padding: 0 0.6rem;
    font-size: var(--fs-xs);
  }

  .row button:not(.small) {
    min-height: 2.6rem;
    padding: 0 0.9rem;
    white-space: nowrap;
  }

  .chosen-city {
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }

  .chosen-city .name {
    font-family: var(--display);
    font-size: var(--fs-lg);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-hi);
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .hits,
  .districts {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .districts {
    overflow-y: auto;
    flex: 1;
    min-height: 6rem;
  }

  .hit {
    width: 100%;
    display: flex;
    align-items: baseline;
    gap: 0.6rem;
    text-align: left;
    min-height: 2.4rem;
    background: var(--sheet-2);
    border: 1px solid var(--rule);
    text-transform: none;
    letter-spacing: 0;
    font-family: var(--body);
  }

  .hit-name {
    flex: 1;
    color: var(--text-hi);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .hit-id {
    font-size: var(--fs-xs);
    color: var(--faint);
  }

  .district {
    display: flex;
    align-items: center;
    gap: 0.55rem;
    padding: 0.4rem 0.5rem;
    background: var(--sheet-2);
    border-left: 2px solid transparent;
    cursor: pointer;
    font-size: var(--fs-sm);
  }

  .district:hover {
    background: var(--sheet-3);
  }

  .district.on {
    border-left-color: var(--hq);
    background: var(--hq-dim);
  }

  .district input {
    width: auto;
    min-height: auto;
    accent-color: var(--hq);
    flex: none;
  }

  .d-name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text);
  }

  .d-area {
    font-size: var(--fs-xs);
    color: var(--muted);
  }

  .levels {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .level {
    display: grid;
    grid-template-columns: auto 1fr auto;
    gap: 0.7rem;
    align-items: baseline;
    text-align: left;
    min-height: 2.5rem;
    background: var(--sheet-2);
    font-family: var(--body);
    text-transform: none;
    letter-spacing: 0;
  }

  .level.active {
    border-color: var(--hq);
    background: var(--hq-dim);
  }

  .lv {
    font-size: var(--fs-xs);
    color: var(--hq);
  }

  .lc {
    color: var(--text);
    font-size: var(--fs-sm);
  }

  .la {
    font-size: var(--fs-xs);
    color: var(--muted);
  }

  .save-row {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    border-top: 1px solid var(--rule);
    padding-top: 0.6rem;
  }

  .total {
    font-size: var(--fs-xs);
    color: var(--muted);
    flex: 1;
  }

  .status {
    font-size: var(--fs-sm);
    color: var(--muted);
    padding-left: 0.6rem;
    border-left: 2px solid var(--rule-hi);
  }

  .status.error {
    color: var(--x);
    border-left-color: var(--x);
  }

  .status.ok {
    color: var(--ok);
    border-left-color: var(--ok);
  }

  @media (max-width: 56rem) {
    .setup {
      grid-template-columns: 1fr;
    }

    .panel-col {
      overflow: visible;
    }

    .map-col {
      min-height: 20rem;
    }
  }
</style>
