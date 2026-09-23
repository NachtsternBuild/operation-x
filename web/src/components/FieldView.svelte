<script>
  /*
   * Die Feldansicht für Mister X und die Fahndungsteams.
   *
   * Beide Rollen brauchen dasselbe Gerüst – Karte, Standortleiste, Lage –
   * und bekommen vom Server verschiedene Wahrheiten geliefert. Was ein Team
   * sehen darf, entscheidet /api/opx/live, nicht diese Komponente.
   */
  import { stream } from '../lib/stream.svelte.js'
  import { useClock, ageLabel } from '../lib/clock.svelte.js'
  import { api, refresh as refreshSession } from '../lib/session.svelte.js'
  import { tracker } from '../lib/tracker.svelte.js'
  import Map from './Map.svelte'
  import PingBar from './PingBar.svelte'
  import MissionBook from './MissionBook.svelte'
  import IntelBoard from './IntelBoard.svelte'
  import ArrestForm from './ArrestForm.svelte'
  import JokerDeck from './JokerDeck.svelte'
  import Radio from './Radio.svelte'

  let { accent = 'det', title, intro, modules = [], missionBook = false, intelBoard = false, arrestForm = false, jokerDeck = false } = $props()

  let field = $state(null)
  let live = $state(null)
  let error = $state(null)
  let firstLoad = $state(true)

  async function loadField() {
    try {
      field = await api.get('/api/opx/map')
    } catch (err) {
      if (err.status !== 404) error = err.message
    }
  }

  async function loadLive() {
    try {
      live = await api.get('/api/opx/live')
      error = null
    } catch (err) {
      error = err.message
    } finally {
      firstLoad = false
    }
  }

  $effect(() => {
    if (!field) loadField()
  })

  // Die Lage kommt aus dem Strom. Vorher lief hier alle zehn Sekunden eine
  // Abfrage — sechs Stunden lang, über Mobilfunk, meist ohne Neuigkeit.
  // Das Alter der Meldungen läuft lokal weiter, sonst friert es zwischen
  // zwei Sendungen ein.
  $effect(() => useClock())

  $effect(() => {
    void stream.revision
    if (stream.live) live = stream.live
    else loadLive()
  })

  async function refreshAll() {
    await Promise.all([loadLive(), refreshSession().catch(() => {})])
  }

  const others = $derived(live?.positions ?? [])
  const status = $derived(live?.status ?? null)
</script>

<PingBar {live} onRefresh={refreshAll} />

{#if live?.outcome}
  <p class="outcome" data-winner={live.outcome.winner}>
    <b>
      {live.outcome.winner === 'misterx'
        ? 'Die Zielperson hat gewonnen.'
        : live.outcome.winner === 'detectives'
          ? 'Die Fahndung hat gewonnen.'
          : 'Unentschieden.'}
    </b>
    {live.outcome.reason}
  </p>
{:else if live?.finale?.active}
  <p class="finale">
    <b>Finale.</b> Der Suchbereich ist offengelegt und zieht sich alle paar Minuten
    enger zusammen – aktuell {Math.round(live.finale.radiusM)} m{live.finale.sector
      ? ` um Sektor ${live.finale.sector}`
      : ''}. Täuschungsmanöver wirken jetzt nicht mehr.
  </p>
{/if}

{#if status && status !== 'running'}
  <p class="notice">
    {#if status === 'paused'}
      Das Spiel ist angehalten. Fristen werden in dieser Zeit nicht geahndet.
    {:else if status === 'finished'}
      Das Spiel ist beendet.
    {:else}
      Das Spiel hat noch nicht begonnen.
    {/if}
  </p>
{/if}

<div class="field">
  <div class="map-area">
    <Map
      sectors={field?.sectors ?? []}
      hotspots={field?.hotspots ?? []}
      area={field?.area ?? null}
      teams={others}
      finale={live?.finale ?? null}
    />
  </div>

  <aside class="side">
    {#if missionBook}
      <MissionBook onChanged={refreshAll} />
    {/if}

    {#if intelBoard}
      <IntelBoard onSolved={refreshAll} />
    {/if}

    {#if jokerDeck}
      <JokerDeck
        {accent}
        sectors={field?.sectors ?? []}
        hotspots={field?.hotspots ?? []}
        onUsed={refreshAll}
      />
    {/if}

    {#if arrestForm}
      <ArrestForm hotspots={field?.hotspots ?? []} onChanged={refreshAll} />
    {/if}

    <section class="block" data-accent={accent}>
      <h2>{title}</h2>
      <p class="intro">{intro}</p>
    </section>

    <section class="block">
      <span class="label">Lage</span>
      {#if firstLoad}
        <p class="dim mono">Lade …</p>
      {:else if others.length === 0}
        <p class="dim">Keine Positionen sichtbar.</p>
      {:else}
        <ul class="teams">
          {#each others as t (t.team)}
            <li>
              <span class="dot" style="background:{t.color}"></span>
              <span class="nm">{t.display || t.callsign}</span>
              <span class="age mono" class:stale={t.stale}>
                {ageLabel(t.capturedAt)}
              </span>
              {#if t.blurM > 0}
                <span class="blur mono">±{t.blurM} m</span>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    </section>

    {#if error}
      <p class="notice error">{error}</p>
    {/if}

    <Radio compact />

    {#if modules.length > 0}
      <section class="block">
        <span class="label">Kommt noch</span>
        <ul class="mods">
          {#each modules as m (m.name)}
            <li><b>{m.name}</b> <span class="mono">Abschnitt {m.phase}</span></li>
          {/each}
        </ul>
      </section>
    {/if}
  </aside>
</div>

<style>
  .field {
    display: grid;
    grid-template-columns: 1fr minmax(14rem, 18rem);
    gap: 1rem;
    flex: 1;
    min-height: 0;
  }

  .map-area {
    min-height: 22rem;
  }

  .side {
    display: flex;
    flex-direction: column;
    gap: 0.7rem;
    overflow-y: auto;
  }

  .block {
    background: var(--sheet);
    border: 1px solid var(--rule);
    border-top: 2px solid var(--rule);
    padding: 0.8rem 0.9rem;
    display: flex;
    flex-direction: column;
    gap: 0.45rem;
  }

  .block[data-accent='x'] { border-top-color: var(--x); }
  .block[data-accent='det'] { border-top-color: var(--det); }

  h2 {
    font-size: var(--fs-lg);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .intro,
  .dim {
    font-size: var(--fs-xs);
    color: var(--muted);
    line-height: 1.5;
  }

  .teams,
  .mods {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .teams li {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: var(--fs-sm);
  }

  .dot {
    width: 0.6rem;
    height: 0.6rem;
    border-radius: 50%;
    flex: none;
  }

  .nm {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .age {
    font-size: var(--fs-xs);
    color: var(--muted);
  }

  .age.stale {
    color: var(--warn);
  }

  .blur {
    font-size: var(--fs-xs);
    color: var(--faint);
  }

  .mods li {
    font-size: var(--fs-xs);
    color: var(--muted);
    display: flex;
    justify-content: space-between;
    gap: 0.6rem;
  }

  .mods b {
    color: var(--text);
    font-weight: 500;
  }

  .notice {
    font-size: var(--fs-sm);
    color: var(--warn);
    border-left: 2px solid var(--warn);
    padding-left: 0.7rem;
  }

  .notice.error {
    color: var(--x);
    border-left-color: var(--x);
  }

  .finale,
  .outcome {
    font-size: var(--fs-sm);
    line-height: 1.5;
    padding: 0.55rem 0.8rem;
    border-left: 3px solid var(--hq);
    background: var(--hq-dim);
    color: var(--text);
  }

  .finale b,
  .outcome b {
    color: var(--hq);
  }

  .outcome[data-winner='misterx'] {
    border-left-color: var(--x);
    background: var(--x-dim);
  }

  .outcome[data-winner='misterx'] b {
    color: var(--x);
  }

  .outcome[data-winner='detectives'] {
    border-left-color: var(--det);
    background: var(--det-dim);
  }

  .outcome[data-winner='detectives'] b {
    color: var(--det);
  }

  @media (max-width: 56rem) {
    .field {
      grid-template-columns: 1fr;
    }

    .map-area {
      min-height: 18rem;
    }
  }
</style>
