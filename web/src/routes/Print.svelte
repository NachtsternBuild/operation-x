<script>
  /*
   * Druckunterlagen für den Spieltag.
   *
   * Drei Dinge, die auf Papier gehören:
   *   1. Das Kartenblatt – Sektorgrenzen und nummerierte Hotspots, wie sie im
   *      Dashboard erscheinen. Damit lässt sich am Tisch planen und unterwegs
   *      zeigen, wenn der Akku leer ist.
   *   2. Die Codeliste für die Spielleitung – welcher Code an welchem Punkt hängt.
   *   3. Die Zettel zum Ausschneiden und Anbringen.
   *
   * Gedruckt wird schwarz auf weiß. Die dunkle Bildschirmoberfläche wäre auf
   * Papier eine Tintenverschwendung und im Sonnenlicht schlechter lesbar als
   * ein normales Blatt.
   */
  import { api, login, session } from '../lib/session.svelte.js'
  import {
    makeProjector,
    boundsOf,
    geometryToPath,
    labelPoint,
    heightFor,
  } from '../lib/project.js'

  let field = $state(null)
  let error = $state(null)
  let sheet = $state('map') // map | codes | tags | teams

  /*
   * Die Teamkarten.
   *
   * Der Konzeptschritt "Zugänge erzeugen" endete bisher im Nichts: Kennwörter
   * lassen sich nicht auslesen, also musste man sie beim Anlegen mitschreiben
   * und später abtippen. Hier vergibt der Server sie auf Knopfdruck neu und
   * gibt sie genau einmal heraus — für dieses eine Blatt.
   */
  let cards = $state([])
  let kennzeichen = $state(null)
  let joinUrl = $state('')
  let issued = $state(false)
  let issuing = $state(false)

  async function loadCards() {
    try {
      const res = await api.get('/api/opx/setup/teamcards')
      cards = res.cards ?? []
      try {
        const k = await api.get('/api/opx/key')
        kennzeichen = k.available ? k.fingerprint : null
      } catch {
        /* ohne Kennzeichen bleibt die Karte sonst vollständig */
      }
      joinUrl = res.joinUrl ?? ''
    } catch (err) {
      error = err.message
    }
  }

  async function issueCredentials() {
    issuing = true
    try {
      const res = await api.post('/api/opx/setup/credentials', {})
      cards = res.cards ?? []
      joinUrl = res.joinUrl ?? ''
      issued = true
      error = null

      /*
       * Die eigene Sitzung retten.
       *
       * Die Zentrale vergibt auch ihr eigenes Kennwort neu, und PocketBase
       * macht damit jede bestehende Anmeldung ungültig — man stünde also
       * unmittelbar nach dem Knopfdruck vor dem Anmeldefenster, mit den frisch
       * erzeugten Kennwörtern nur noch auf einem Blatt, das man noch nicht
       * gedruckt hat. Deshalb hier gleich wieder anmelden.
       */
      const mine = cards.find((c) => c.callsign === session.team?.callsign)
      if (mine?.password) {
        await login(mine.callsign, mine.password)
      }
    } catch (err) {
      error = err.message
    } finally {
      issuing = false
    }
  }

  $effect(() => {
    if (sheet === 'teams' && cards.length === 0) loadCards()
  })

  const roleNames = {
    hq: 'Einsatzzentrale',
    misterx: 'Zielperson',
    detective: 'Fahndungsteam',
  }

  const roleRules = {
    hq: [
      'Ihr seht alles und greift ein, wenn die Realität dazwischenkommt.',
      'Beweisfotos zügig prüfen — solange ihr nicht entscheidet, wartet jemand am Ziel.',
      'Jede Korrektur braucht eine Begründung und steht danach im Protokoll.',
    ],
    misterx: [
      'Alle 10 Minuten Standort melden (13 im Transit).',
      'An jedem Ziel: Vor-Ort-Code eingeben oder Foto hochladen, näher als 150 m.',
      'Fluchtpunkte sind eure Waffe. Die Nebelkerze löscht auch eine laufende Ortung.',
      'Zwanzig Minuten ohne Bewegung und ohne Handlung kosten einen Fluchtpunkt.',
    ],
    detective: [
      'Alle 10 Minuten Standort melden (13 im Transit).',
      'Rätsel lösen bringt Hinweise. Schnell gelöst heißt heiße Spur.',
      'Nicht jeder Hinweis stimmt — die Zielperson kann welche fälschen.',
      'Zugriff nur vor Ort. Stufe 1 und 2 sind kostenlos, Stufe 3 entscheidet oder kostet.',
      'Fünf Minuten ohne Bewegung und ohne Handlung kosten einen Fahndungspunkt.',
    ],
  }

  const W = 760

  $effect(() => {
    if (!field) {
      api
        .get('/api/opx/map')
        .then((f) => (field = f))
        .catch((err) => (error = err.message))
    }
  })

  const sectors = $derived(field?.sectors ?? [])
  const hotspots = $derived(field?.hotspots ?? [])

  const bounds = $derived(
    boundsOf(
      sectors.map((s) => s.geometry).filter(Boolean),
      hotspots,
    ),
  )

  const H = $derived(bounds ? heightFor(bounds, W) : 500)
  const project = $derived(bounds ? makeProjector(bounds, W, H) : null)

  const shapes = $derived(
    !project
      ? []
      : sectors.map((s) => ({
          id: s.id,
          code: s.code,
          name: s.name,
          path: geometryToPath(s.geometry, project),
          label: labelPoint(s.geometry, project),
        })),
  )

  const marks = $derived(
    !project
      ? []
      : hotspots.map((h) => {
          const [x, y] = project(h.lat, h.lng)
          return { ...h, x, y, label: String(h.number).padStart(2, '0') }
        }),
  )

  const sectorName = $derived(
    Object.fromEntries(sectors.map((s) => [s.id, `${s.code} · ${s.name}`])),
  )

  // Maßstabsbalken: eine runde Distanz, die auf dem Blatt gut aussieht.
  const scaleBar = $derived.by(() => {
    if (!project || !bounds) return null
    const midLat = (bounds.north + bounds.south) / 2
    const [x0] = project(midLat, bounds.west)
    const [x1] = project(midLat, bounds.east)
    const metersPerPx =
      (haversine(midLat, bounds.west, midLat, bounds.east) || 1) / Math.max(x1 - x0, 1)

    for (const m of [200, 500, 1000, 2000, 5000]) {
      const px = m / metersPerPx
      if (px > 60 && px < 220) {
        return { px, label: m >= 1000 ? `${m / 1000} km` : `${m} m` }
      }
    }
    return null
  })

  function haversine(lat1, lng1, lat2, lng2) {
    const R = 6371008.8
    const toRad = (d) => (d * Math.PI) / 180
    const dLat = toRad(lat2 - lat1)
    const dLng = toRad(lng2 - lng1)
    const a =
      Math.sin(dLat / 2) ** 2 +
      Math.cos(toRad(lat1)) * Math.cos(toRad(lat2)) * Math.sin(dLng / 2) ** 2
    return 2 * R * Math.asin(Math.sqrt(a))
  }
</script>

<div class="controls no-print">
  <div class="tabs">
    <button class:on={sheet === 'map'} onclick={() => (sheet = 'map')}>Kartenblatt</button>
    <button class:on={sheet === 'codes'} onclick={() => (sheet = 'codes')}>Codeliste</button>
    <button class:on={sheet === 'tags'} onclick={() => (sheet = 'tags')}>Zettel</button>
    <button class:on={sheet === 'teams'} onclick={() => (sheet = 'teams')}>Teamkarten</button>
  </div>
  <button class="primary" onclick={() => window.print()}>Drucken</button>
</div>

{#if error}
  <p class="err no-print">{error}</p>
{:else if !field}
  <p class="wait no-print mono">Lade Spielfeld …</p>
{:else}
  <div class="paper">
    {#if sheet === 'map'}
      <!-- Blatt 1: Karte für alle Spieler. Ohne Codes. -->
      <header class="ph">
        <h1>Operation X — Fahndungskarte</h1>
        <span class="sub">{field.city} · {sectors.length} Sektoren · {hotspots.length} Hotspots</span>
      </header>

      {#if project}
        <svg viewBox="0 0 {W} {H}" class="chart" role="img" aria-label="Sektorkarte">
          {#each shapes as s (s.id)}
            <path d={s.path} class="sector" />
          {/each}

          {#each shapes as s (s.id)}
            {#if s.label}
              <text x={s.label[0]} y={s.label[1]} class="sector-code">{s.code}</text>
            {/if}
          {/each}

          {#each marks as m (m.id)}
            <circle cx={m.x} cy={m.y} r="9" class="spot" />
            <text x={m.x} y={m.y + 3.2} class="spot-num">{m.label}</text>
          {/each}

          {#if scaleBar}
            <g transform="translate(20, {H - 22})">
              <line x1="0" y1="0" x2={scaleBar.px} y2="0" class="scale" />
              <line x1="0" y1="-4" x2="0" y2="4" class="scale" />
              <line x1={scaleBar.px} y1="-4" x2={scaleBar.px} y2="4" class="scale" />
              <text x={scaleBar.px / 2} y="-7" class="scale-label">{scaleBar.label}</text>
            </g>
          {/if}
        </svg>
      {:else}
        <p class="empty">Noch keine Sektoren oder Hotspots angelegt.</p>
      {/if}

      <div class="legend">
        {#each sectors as s (s.id)}
          <span class="leg"><b>{s.code}</b> {s.name}</span>
        {/each}
      </div>

      <ol class="spotlist">
        {#each hotspots as h (h.id)}
          <li><b>{String(h.number).padStart(2, '0')}</b> {h.name}</li>
        {/each}
      </ol>

      <footer class="pf">
        Kartendaten © OpenStreetMap-Mitwirkende · Schematische Darstellung, kein Ersatz für einen Stadtplan
      </footer>
    {:else if sheet === 'codes'}
      <!-- Blatt 2: nur für die Spielleitung. -->
      <header class="ph">
        <h1>Operation X — Vor-Ort-Codes</h1>
        <span class="sub warn">Nur für die Einsatzzentrale. Nicht an Spieler geben.</span>
      </header>

      <table class="codes">
        <thead>
          <tr><th>Nr.</th><th>Ort</th><th>Sektor</th><th>Code</th><th>Angebracht</th></tr>
        </thead>
        <tbody>
          {#each hotspots as h (h.id)}
            <tr>
              <td class="num">{String(h.number).padStart(2, '0')}</td>
              <td>{h.name}</td>
              <td class="sec">{sectorName[h.sector] ?? '—'}</td>
              <td class="code">{h.passcode ?? '—'}</td>
              <td class="tick"></td>
            </tr>
          {/each}
        </tbody>
      </table>

      <p class="note">
        Der Code beweist, dass jemand wirklich am Punkt stand. Entweder als Zettel
        anbringen (Blatt „Zettel“) oder durch etwas ersetzen, das ohnehin dort steht –
        eine Jahreszahl am Portal, eine Hausnummer, eine Inschrift. Dann den Code hier
        entsprechend ändern.
      </p>
    {:else if sheet === 'tags'}
      <!-- Blatt 3: zum Ausschneiden. -->
      <header class="ph no-break">
        <h1>Operation X — Zettel zum Anbringen</h1>
        <span class="sub">Ausschneiden und am jeweiligen Punkt befestigen</span>
      </header>

      <div class="tags">
        {#each hotspots as h (h.id)}
          <div class="tag-card">
            <span class="tag-num">#{String(h.number).padStart(2, '0')}</span>
            <span class="tag-name">{h.name}</span>
            <span class="tag-code">{h.passcode ?? '—'}</span>
            <span class="tag-foot">Operation X · Code im Dashboard eingeben</span>
          </div>
        {/each}
      </div>
    {:else}
      <!-- Blatt 4: eine Karte je Team, zum Ausschneiden und Verteilen. -->
      <header class="ph no-break">
        <h1>Operation X — Teamkarten</h1>
        <span class="sub">Ausschneiden und je Team ausgeben</span>
      </header>

      {#if !issued}
        <div class="issue no-print">
          <p>
            Kennwörter lassen sich nicht auslesen — sie liegen nur als Prüfsumme
            in der Datenbank. Für die Karten vergibt der Server neue, sprechbare
            Kennwörter und zeigt sie <b>einmal</b>. Danach sind auch sie wieder
            unlesbar.
          </p>
          <p class="warn">
            Damit gelten die bisherigen Kennwörter nicht mehr. Also erst
            drücken, wenn ihr gleich druckt — nicht mitten im Spiel.
          </p>
          <button class="primary" onclick={issueCredentials} disabled={issuing}>
            {issuing ? 'Vergebe …' : `Kennwörter vergeben (${cards.length} Zugänge)`}
          </button>
        </div>
      {/if}

      <div class="cards">
        {#each cards as c (c.id)}
          <div class="team-card">
            <div class="tc-head">
              <span class="tc-role">{roleNames[c.role] ?? c.role}</span>
              <span class="tc-name">{c.display || c.callsign}</span>
            </div>

            <div class="tc-login">
              <div class="tc-field">
                <span class="tc-label">Rufzeichen</span>
                <span class="tc-value">{c.callsign}</span>
              </div>
              <div class="tc-field">
                <span class="tc-label">Kennwort</span>
                <span class="tc-value">{c.password || '________________'}</span>
              </div>
            </div>

            <div class="tc-body">
              <div class="tc-qr">
                {#if joinUrl}
                  <img src="/api/opx/qr?url={encodeURIComponent(joinUrl)}" alt="Beitritt" />
                  <span class="tc-url">{joinUrl}</span>
                {:else}
                  <span class="tc-url">Adresse steht noch nicht fest</span>
                {/if}
                <!--
                  Das Kennzeichen des Servers gehört auf die Karte, nicht nur
                  auf den Bildschirm: Es ist die einzige Angabe, die nicht durch
                  den Tunnel gelaufen ist — gedruckt wird auf dem Rechner der
                  Spielleitung. Wer die ersten Zeichen mit denen in seiner App
                  vergleicht, weiß, dass niemand dazwischensitzt.
                -->
                {#if kennzeichen}
                  <span class="tc-fp">🔒 {kennzeichen}</span>
                {/if}
              </div>

              <ul class="tc-rules">
                {#each roleRules[c.role] ?? [] as r}
                  <li>{r}</li>
                {/each}
              </ul>
            </div>

            <span class="tc-foot">
              Zugangsdaten verfallen nach Spielende. Standortdaten werden
              danach automatisch gelöscht.
            </span>
          </div>
        {/each}
      </div>

      {#if cards.length === 0}
        <p class="hint no-print">Keine Zugänge gefunden.</p>
      {/if}
    {/if}
  </div>
{/if}

<style>
  .tc-fp {
    display: block;
    font-family: var(--mono);
    font-size: 7pt;
    letter-spacing: 0.04em;
    margin-top: 0.15rem;
  }

  .issue {
    background: var(--sheet-2);
    border: 1px solid var(--rule);
    border-left: 2px solid var(--hq);
    padding: 0.8rem 0.9rem;
    margin-bottom: 1rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    font-size: var(--fs-sm);
    line-height: 1.5;
    color: var(--text);
    max-width: 46rem;
  }

  .issue .warn {
    color: var(--warn);
  }

  /* --- Teamkarten. Zwei je Zeile, keine über den Seitenrand hinweg. --- */
  .cards {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 8mm;
  }

  .team-card {
    border: 1px solid #000;
    padding: 5mm;
    display: flex;
    flex-direction: column;
    gap: 3mm;
    break-inside: avoid;
    page-break-inside: avoid;
    color: #000;
    background: #fff;
  }

  .tc-head {
    border-bottom: 1px solid #000;
    padding-bottom: 2mm;
    display: flex;
    flex-direction: column;
    gap: 0.5mm;
  }

  .tc-role {
    font-family: var(--display);
    text-transform: uppercase;
    letter-spacing: 0.14em;
    font-size: 8pt;
  }

  .tc-name {
    font-family: var(--display);
    font-size: 16pt;
    font-weight: 700;
    text-transform: uppercase;
  }

  .tc-login {
    display: flex;
    gap: 6mm;
  }

  .tc-field {
    display: flex;
    flex-direction: column;
    gap: 0.5mm;
  }

  .tc-label {
    font-size: 7pt;
    text-transform: uppercase;
    letter-spacing: 0.1em;
  }

  .tc-value {
    font-family: var(--mono);
    font-size: 12pt;
    font-weight: 500;
  }

  .tc-body {
    display: flex;
    gap: 4mm;
    align-items: flex-start;
  }

  .tc-qr {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 1mm;
    flex: none;
  }

  .tc-qr img {
    width: 26mm;
    height: 26mm;
  }

  .tc-url {
    font-family: var(--mono);
    font-size: 6pt;
    max-width: 28mm;
    overflow-wrap: anywhere;
    text-align: center;
  }

  .tc-rules {
    margin: 0;
    padding-left: 4mm;
    font-size: 8pt;
    line-height: 1.45;
    display: flex;
    flex-direction: column;
    gap: 1mm;
  }

  .tc-foot {
    border-top: 1px solid #000;
    padding-top: 1.5mm;
    font-size: 6.5pt;
    line-height: 1.35;
  }

  .controls {
    display: flex;
    gap: 0.6rem;
    align-items: center;
    border-bottom: 1px solid var(--rule);
    padding-bottom: 0.6rem;
  }

  .tabs {
    display: flex;
    gap: 0.4rem;
    flex: 1;
  }

  .tabs button {
    min-height: 2.3rem;
    padding: 0 0.9rem;
    background: transparent;
    border-color: var(--rule);
    color: var(--muted);
  }

  .tabs button.on {
    color: var(--hq);
    border-color: var(--hq);
    background: var(--hq-dim);
  }

  .err,
  .wait {
    color: var(--muted);
    border-left: 2px solid var(--rule-hi);
    padding-left: 0.6rem;
  }

  .err {
    color: var(--x);
    border-left-color: var(--x);
  }

  /*
   * Das Blatt ist auch am Bildschirm weiß: Was gedruckt wird, soll man vorher
   * so sehen, wie es aus dem Drucker kommt.
   */
  .paper {
    background: #fff;
    color: #14181a;
    padding: 1.6rem 1.8rem;
    max-width: 60rem;
    margin: 0 auto;
    font-family: var(--body);
    line-height: 1.45;
  }

  .ph {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    border-bottom: 1.5px solid #14181a;
    padding-bottom: 0.5rem;
    margin-bottom: 1rem;
  }

  .ph h1 {
    font-family: var(--display);
    font-size: 1.5rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: #14181a;
  }

  .sub {
    font-family: var(--mono);
    font-size: 0.75rem;
    color: #5b6a70;
  }

  .sub.warn {
    color: #a3341f;
    font-weight: 500;
  }

  .chart {
    width: 100%;
    height: auto;
    display: block;
    border: 1px solid #c8d0d3;
  }

  .chart .sector {
    fill: #eef2f3;
    stroke: #52646b;
    stroke-width: 1.1;
    stroke-linejoin: round;
  }

  .chart .sector-code {
    font-family: var(--display);
    font-size: 17px;
    font-weight: 700;
    fill: #7c8d94;
    text-anchor: middle;
    letter-spacing: 0.08em;
  }

  .chart .spot {
    fill: #fff;
    stroke: #14181a;
    stroke-width: 1.4;
  }

  /*
   * In der Altstadt liegen mehrere Hotspots dicht beieinander und die Kreise
   * überlappen. Ein weißer Rand hinter der Ziffer hält sie trotzdem lesbar –
   * die Punkte bleiben dabei an ihrer echten Position, denn nach dieser Karte
   * wird vor Ort navigiert.
   */
  .chart .spot-num {
    font-family: var(--mono);
    font-size: 9px;
    font-weight: 500;
    fill: #14181a;
    text-anchor: middle;
    stroke: #fff;
    stroke-width: 2.4px;
    paint-order: stroke fill;
  }

  .chart .scale {
    stroke: #14181a;
    stroke-width: 1.2;
  }

  .chart .scale-label {
    font-family: var(--mono);
    font-size: 9px;
    fill: #14181a;
    text-anchor: middle;
  }

  .legend {
    display: flex;
    flex-wrap: wrap;
    gap: 0.3rem 1.1rem;
    margin-top: 0.7rem;
    font-size: 0.82rem;
  }

  .leg b {
    font-family: var(--mono);
    color: #52646b;
  }

  .spotlist {
    columns: 3;
    column-gap: 1.6rem;
    list-style: none;
    padding: 0;
    margin: 0.9rem 0 0;
    font-size: 0.82rem;
  }

  .spotlist li {
    break-inside: avoid;
    padding: 0.05rem 0;
  }

  .spotlist b {
    font-family: var(--mono);
    color: #52646b;
    margin-right: 0.3rem;
  }

  .pf {
    margin-top: 1rem;
    padding-top: 0.5rem;
    border-top: 1px solid #c8d0d3;
    font-size: 0.7rem;
    color: #7c8d94;
  }

  table.codes {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.88rem;
  }

  table.codes th {
    text-align: left;
    font-family: var(--display);
    text-transform: uppercase;
    letter-spacing: 0.1em;
    font-size: 0.7rem;
    color: #5b6a70;
    border-bottom: 1.2px solid #14181a;
    padding: 0.3rem 0.4rem;
  }

  table.codes td {
    padding: 0.32rem 0.4rem;
    border-bottom: 1px solid #dde3e5;
  }

  table.codes .num,
  table.codes .code {
    font-family: var(--mono);
  }

  table.codes .code {
    font-weight: 500;
    letter-spacing: 0.06em;
  }

  table.codes .sec {
    color: #5b6a70;
    font-size: 0.8rem;
  }

  table.codes .tick {
    width: 4.5rem;
    border-bottom: 1px solid #dde3e5;
  }

  table.codes tbody tr td.tick::after {
    content: '';
    display: block;
    width: 0.85rem;
    height: 0.85rem;
    border: 1px solid #8b989d;
  }

  .note {
    margin-top: 0.9rem;
    font-size: 0.78rem;
    color: #5b6a70;
    max-width: 46rem;
  }

  .tags {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 0.5rem;
  }

  .tag-card {
    border: 1.5px dashed #8b989d;
    padding: 0.9rem 1rem;
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    break-inside: avoid;
    min-height: 7rem;
  }

  .tag-num {
    font-family: var(--mono);
    font-size: 0.78rem;
    color: #5b6a70;
  }

  .tag-name {
    font-family: var(--display);
    font-size: 1.05rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .tag-code {
    font-family: var(--mono);
    font-size: 2rem;
    font-weight: 500;
    letter-spacing: 0.14em;
    margin-top: auto;
  }

  .tag-foot {
    font-size: 0.62rem;
    color: #7c8d94;
  }

  .empty {
    color: #7c8d94;
    padding: 2rem 0;
  }

  @media print {
    :global(body) {
      background: #fff;
    }

    /* Alles außerhalb des Blattes verschwindet – Kopfleiste, Reiter, Knöpfe. */
    :global(.grid-bg),
    :global(header.bar),
    .no-print {
      display: none !important;
    }

    :global(main) {
      padding: 0;
    }

    .paper {
      max-width: none;
      padding: 0;
    }

    .no-break,
    .tag-card,
    .chart {
      break-inside: avoid;
    }

    @page {
      margin: 14mm;
    }
  }
</style>
