<script>
  /*
   * Die Zeitleiste der Einsatzzentrale.
   *
   * Jede Buchung mit Zeitstempel und Begründung. Das beantwortet am Spieltag
   * die häufigste Frage überhaupt – „warum habe ich minus fünfzehn?“ – ohne
   * dass jemand diskutieren muss.
   */
  import { api } from '../lib/session.svelte.js'
  import { stream } from '../lib/stream.svelte.js'

  let events = $state([])
  let error = $state(null)
  let filter = $state('alle')
  let status = $state(null)
  let busy = $state(false)

  async function load() {
    try {
      const res = await api.get('/api/opx/hq/events')
      events = res.events ?? []
      error = null
    } catch (err) {
      error = err.message
    }
  }

  async function loadStatus() {
    try {
      const s = await api.get('/api/opx/status')
      status = s.status
    } catch {
      /* nicht kritisch */
    }
  }

  $effect(() => {
    // Nachladen, wenn der Server eine Änderung meldet. Der Takt daneben ist
    // nur die Rückfallebene für den Fall, dass der Strom nicht steht.
    void stream.revision
    load()
    loadStatus()
    const t = setInterval(load, 60_000)
    return () => clearInterval(t)
  })

  // Die Pause bekommt eine Ansage mit.
  //
  // "Angehalten" allein ist für die, die draußen stehen, von einer Störung
  // nicht zu unterscheiden – und genau dann fangen sie an, auf ihren Geräten
  // herumzudrücken. "Mittagessen, 60 Minuten" beantwortet beides auf einmal.
  let pauseReason = $state('')
  let pauseMinutes = $state(60)
  let pauseOpen = $state(false)

  async function setStatus(next, extra = {}) {
    busy = true
    try {
      await api.post('/api/opx/hq/status', { status: next, ...extra })
      await loadStatus()
      await load()
    } catch (err) {
      error = err.message
    } finally {
      busy = false
    }
  }

  const labels = {
    'ping.warning': 'Meldung überfällig',
    'ping.late': 'Meldung erneut überfällig',
    'ping.lockout': 'Standortsperre',
    'position.received': 'Standort gemeldet',
    'position.implausible': 'Positionssprung',
    'position.mocked': 'Simulierter Standort',
    'transit.start': 'Transit begonnen',
    'transit.end': 'Transit beendet',
    'game.running': 'Spiel gestartet',
    'game.paused': 'Spiel angehalten',
    'game.finished': 'Spiel beendet',
    'game.ready': 'Spiel bereit',
    'game.setup': 'Zurück in die Vorbereitung',
  }

  const shown = $derived(
    filter === 'alle'
      ? events
      : filter === 'strafen'
        ? events.filter((e) => e.deltaPoints !== 0 || e.deltaFp !== 0)
        : events.filter((e) => e.type.startsWith(filter)),
  )

  function time(iso) {
    const d = new Date(iso)
    return d.toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  }

  function severity(e) {
    if (e.type === 'ping.lockout' || e.type === 'position.mocked') return 'krit'
    if (e.deltaPoints < 0 || e.deltaFp < 0) return 'warn'
    if (e.deltaPoints > 0 || e.deltaFp > 0) return 'gut'
    return 'neutral'
  }
</script>

<div class="control">
  <div class="state">
    <span class="label">Spielzustand</span>
    <span class="value mono">{status ?? '—'}</span>
  </div>

  <div class="buttons">
    {#if status !== 'running'}
      <button class="primary" onclick={() => setStatus('running')} disabled={busy}>
        {status === 'paused' ? 'Fortsetzen' : 'Spiel starten'}
      </button>
    {:else}
      <button onclick={() => (pauseOpen = !pauseOpen)} disabled={busy}>Anhalten …</button>
    {/if}
    <button class="ghost" onclick={() => setStatus('finished')} disabled={busy || status === 'finished'}>
      Beenden
    </button>
  </div>
</div>

{#if pauseOpen && status === 'running'}
  <div class="pausebox">
    <p class="dim">
      Wofür wird angehalten? Die Ansage steht auf allen Geräten — und während
      der Pause wird kein Standort aufgezeichnet.
    </p>
    <div class="pauserow">
      <input
        bind:value={pauseReason}
        placeholder="z. B. Mittagessen, Anreise zum Startpunkt, kurze Pause"
      />
      <label class="mins">
        <span class="label">Minuten</span>
        <input type="number" min="0" max="480" bind:value={pauseMinutes} />
      </label>
      <button
        class="primary"
        disabled={busy}
        onclick={() => {
          pauseOpen = false
          setStatus('paused', { reason: pauseReason, minutes: Number(pauseMinutes) || 0 })
        }}
      >
        Anhalten
      </button>
    </div>
  </div>
{/if}

{#if status === 'paused'}
  <p class="pausehint">
    Das Spiel ist angehalten. Fristen werden nicht geahndet, Missionsfristen und
    Spielende wandern beim Fortsetzen um die Dauer der Pause nach hinten, und es
    wird kein Standort aufgezeichnet.
  </p>
{/if}

<div class="filters">
  {#each [['alle', 'Alles'], ['strafen', 'Nur Buchungen'], ['ping', 'Meldungen'], ['position', 'Standorte'], ['game', 'Spielsteuerung']] as [key, label]}
    <button class="chip" class:on={filter === key} onclick={() => (filter = key)}>{label}</button>
  {/each}
</div>

{#if error}
  <p class="err">{error}</p>
{:else if shown.length === 0}
  <p class="empty">Noch keine Ereignisse.</p>
{:else}
  <ul class="log">
    {#each shown as e (e.id)}
      <li data-sev={severity(e)}>
        <span class="t mono">{time(e.occurredAt)}</span>
        <span class="who">{e.callsign ?? 'System'}</span>
        <span class="what">
          {labels[e.type] ?? e.type}
          <!--
            Der Grund gehört daneben, nicht in eine Auskunft auf Nachfrage:
            "Warum habe ich minus fünfzehn?" ist die häufigste Frage am
            Spieltag, und die Zeitleiste ist der Ort, an dem sie beantwortet
            wird. Bis hierher zeigte sie nur den Ereignistyp.
          -->
          {#if e.reason}<span class="why">{e.reason}</span>{/if}
        </span>
        <span class="delta mono">
          {#if e.deltaPoints}<b class:neg={e.deltaPoints < 0}>{e.deltaPoints > 0 ? '+' : ''}{e.deltaPoints} Pkt</b>{/if}
          {#if e.deltaFp}<b class:neg={e.deltaFp < 0}>{e.deltaFp > 0 ? '+' : ''}{e.deltaFp} FP</b>{/if}
        </span>
      </li>
    {/each}
  </ul>
{/if}

<style>
  .why {
    display: block;
    color: var(--muted);
    font-size: 0.82rem;
    line-height: 1.3;
  }

  .pausebox {
    background: var(--sheet);
    border: 1px solid var(--rule);
    border-left: 3px solid var(--hq);
    padding: 0.7rem 0.9rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .pauserow {
    display: flex;
    gap: 0.6rem;
    align-items: flex-end;
    flex-wrap: wrap;
  }

  .pauserow input[type='text'],
  .pauserow > input:not([type]) {
    flex: 1 1 18rem;
  }

  .mins {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    width: 7rem;
  }

  .control {
    display: flex;
    align-items: center;
    gap: 1rem;
    background: var(--sheet);
    border: 1px solid var(--rule);
    padding: 0.7rem 0.9rem;
  }

  .state {
    display: flex;
    flex-direction: column;
    line-height: 1.2;
  }

  .state .value {
    color: var(--text-hi);
    font-size: var(--fs-base);
  }

  .buttons {
    display: flex;
    gap: 0.4rem;
    margin-left: auto;
  }

  .buttons button {
    min-height: 2.3rem;
    padding: 0 0.9rem;
  }

  .pausehint {
    font-size: var(--fs-sm);
    color: var(--warn);
    border-left: 2px solid var(--warn);
    padding-left: 0.7rem;
  }

  .filters {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
  }

  .chip {
    min-height: 2rem;
    padding: 0 0.7rem;
    font-size: var(--fs-xs);
    background: transparent;
    border-color: var(--rule);
    color: var(--muted);
  }

  .chip.on {
    color: var(--hq);
    border-color: var(--hq);
    background: var(--hq-dim);
  }

  .log {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 1px;
    overflow-y: auto;
    flex: 1;
    min-height: 0;
  }

  .log li {
    display: grid;
    grid-template-columns: 5rem 8rem 1fr auto;
    gap: 0.7rem;
    align-items: baseline;
    padding: 0.4rem 0.6rem;
    background: var(--sheet);
    border-left: 2px solid var(--rule-hi);
    font-size: var(--fs-sm);
  }

  .log li[data-sev='warn'] { border-left-color: var(--warn); }
  .log li[data-sev='krit'] { border-left-color: var(--crit); }
  .log li[data-sev='gut'] { border-left-color: var(--ok); }

  .t {
    font-size: var(--fs-xs);
    color: var(--muted);
  }

  .who {
    color: var(--text-hi);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .what {
    color: var(--text);
    min-width: 0;
  }

  .delta {
    display: flex;
    gap: 0.6rem;
    font-size: var(--fs-xs);
  }

  .delta b {
    color: var(--ok);
    font-weight: 500;
  }

  .delta b.neg {
    color: var(--x);
  }

  .err,
  .empty {
    font-size: var(--fs-sm);
    color: var(--muted);
    border-left: 2px solid var(--rule-hi);
    padding-left: 0.7rem;
  }

  .err {
    color: var(--x);
    border-left-color: var(--x);
  }

  @media (max-width: 46rem) {
    .log li {
      grid-template-columns: 4.5rem 1fr;
      grid-template-areas: 't who' 'what what' 'delta delta';
    }
  }
</style>
