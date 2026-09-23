<script>
  import { api } from '../lib/session.svelte.js'
  import { stream } from '../lib/stream.svelte.js'
  import { useClock, ageLabel } from '../lib/clock.svelte.js'
  import Map from '../components/Map.svelte'
  import Setup from './Setup.svelte'
  import Hotspots from './Hotspots.svelte'
  import Print from './Print.svelte'
  import Timeline from './Timeline.svelte'
  import Evidence from './Evidence.svelte'
  import Puzzles from './Puzzles.svelte'
  import Control from './Control.svelte'
  import Distribute from './Distribute.svelte'

  let tab = $state('lage')

  let field = $state(null)
  let loadError = $state(null)
  let mapRef = $state(null)

  let live = $state(null)

  async function loadMap() {
    try {
      field = await api.get('/api/opx/map')
      loadError = null
    } catch (err) {
      loadError = err.status === 404 ? null : err.message
      field = null
    }
  }

  async function loadLive() {
    try {
      live = await api.get('/api/opx/live')
    } catch {
      /* ohne laufendes Spiel gibt es nichts zu zeigen */
    }
  }

  $effect(() => {
    if (tab === 'lage' && !field) loadMap()
  })

  // Die Lage kommt aus dem Strom, nicht aus einer Schleife: Der Server meldet
  // sich, wenn sich etwas bewegt hat.
  // Das Alter der Meldungen läuft lokal weiter, sonst friert es zwischen
  // zwei Sendungen ein.
  $effect(() => useClock())

  $effect(() => {
    void stream.revision
    if (tab !== 'lage') return
    if (stream.live) live = stream.live
    else loadLive()
  })

</script>

<nav class="tabs">
  <button class:on={tab === 'lage'} onclick={() => (tab = 'lage')}>Lagekarte</button>
  <button class:on={tab === 'setup'} onclick={() => (tab = 'setup')}>Sektoren</button>
  <button class:on={tab === 'spots'} onclick={() => (tab = 'spots')}>Hotspots</button>
  <button class:on={tab === 'raetsel'} onclick={() => (tab = 'raetsel')}>Rätsel</button>
  <button class:on={tab === 'beweis'} onclick={() => (tab = 'beweis')}>Beweise</button>
  <button class:on={tab === 'pult'} onclick={() => (tab = 'pult')}>Regelpult</button>
  <button class:on={tab === 'zeit'} onclick={() => (tab = 'zeit')}>Zeitleiste</button>
  <button class:on={tab === 'verteilen'} onclick={() => (tab = 'verteilen')}>Verteilen</button>
  <button class:on={tab === 'druck'} onclick={() => (tab = 'druck')}>Druck</button>
</nav>

{#if tab === 'lage'}
  <div class="lage">
    <div class="map-area">
      <Map
        bind:this={mapRef}
        sectors={field?.sectors ?? []}
        hotspots={field?.hotspots ?? []}
        area={field?.area ?? null}
        teams={live?.positions ?? []}
      />
    </div>

    <aside class="side">
      {#if loadError}
        <p class="status error">{loadError}</p>
      {:else if field}
        <div class="block">
          <span class="label">Spielfeld</span>
          <p class="big">{field.city || '—'}</p>
          <dl class="figures">
            <div><dt>Sektoren</dt><dd class="mono">{field.sectors.length}</dd></div>
            <div><dt>Hotspots</dt><dd class="mono">{field.hotspots.length}</dd></div>
          </dl>
        </div>

        {#if live?.positions?.length}
          <div class="block">
            <span class="label">Im Feld</span>
            <ul class="sector-list">
              {#each live.positions as p (p.team)}
                <li>
                  <span class="dot" style="background:{p.color}"></span>
                  <span class="code mono">{p.role === 'misterx' ? 'X' : 'D'}</span>
                  <span class="nm">{p.display || p.callsign}</span>
                  <span class="code mono">{ageLabel(p.capturedAt)}</span>
                </li>
              {/each}
            </ul>
          </div>
        {/if}

        {#if field.sectors.length}
          <div class="block">
            <span class="label">Sektoren</span>
            <ul class="sector-list">
              {#each field.sectors as s (s.id)}
                <li>
                  <span class="dot" style="background:{s.color}"></span>
                  <span class="code mono">{s.code}</span>
                  <span class="nm">{s.name}</span>
                </li>
              {/each}
            </ul>
          </div>
        {:else}
          <p class="status">
            Noch keine Sektoren. Unter <em>Einrichtung</em> lassen sie sich aus den
            echten Stadtteilgrenzen übernehmen.
          </p>
        {/if}
      {:else}
        <p class="status mono">Lade Spielfeld …</p>
      {/if}
    </aside>
  </div>
{:else if tab === 'setup'}
  <Setup onSaved={(_, bleiben) => { field = null; if (!bleiben) tab = 'lage' }} />
{:else if tab === 'spots'}
  <Hotspots />
{:else if tab === 'raetsel'}
  <Puzzles />
{:else if tab === 'beweis'}
  <Evidence />
{:else if tab === 'pult'}
  <Control />
{:else if tab === 'zeit'}
  <Timeline />
{:else if tab === 'verteilen'}
  <Distribute />
{:else}
  <Print />
{/if}

<style>
  .tabs {
    display: flex;
    gap: 0.4rem;
    border-bottom: 1px solid var(--rule);
    padding-bottom: 0.6rem;
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

  .lage {
    display: grid;
    grid-template-columns: 1fr minmax(15rem, 19rem);
    gap: 1rem;
    flex: 1;
    min-height: 0;
  }

  .map-area {
    min-height: 26rem;
  }

  .side {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    overflow-y: auto;
  }

  .block {
    background: var(--sheet);
    border: 1px solid var(--rule);
    padding: 0.85rem 0.9rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .big {
    font-family: var(--display);
    font-size: var(--fs-xl);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-hi);
    line-height: 1.1;
  }

  .figures {
    display: flex;
    gap: 1.5rem;
    margin: 0;
  }

  .figures div {
    display: flex;
    flex-direction: column;
  }

  .figures dt {
    font-family: var(--display);
    text-transform: uppercase;
    letter-spacing: 0.12em;
    font-size: var(--fs-xs);
    color: var(--muted);
  }

  .figures dd {
    margin: 0;
    font-size: var(--fs-lg);
    color: var(--text-hi);
  }

  .sector-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    max-height: 20rem;
    overflow-y: auto;
  }

  .sector-list li {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: var(--fs-sm);
  }

  .dot {
    width: 0.65rem;
    height: 0.65rem;
    border-radius: 50%;
    flex: none;
  }

  .code {
    font-size: var(--fs-xs);
    color: var(--hq);
    min-width: 1.4rem;
  }

  .nm {
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .status {
    font-size: var(--fs-sm);
    color: var(--muted);
    border-left: 2px solid var(--rule-hi);
    padding-left: 0.6rem;
    line-height: 1.5;
  }

  .status.error {
    color: var(--x);
    border-left-color: var(--x);
  }

  @media (max-width: 56rem) {
    .lage {
      grid-template-columns: 1fr;
    }

    .map-area {
      min-height: 20rem;
    }
  }
</style>
