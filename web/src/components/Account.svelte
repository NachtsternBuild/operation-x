<script>
  /*
   * Das Punktekonto.
   *
   * "Warum habe ich minus fünfzehn?" ist die häufigste Frage am Spieltag, und
   * sie hatte bis hierher keine Antwort: Die Kopfleiste zeigte eine Zahl, das
   * Kontobuch lag in der Datenbank, und es gab keinen Weg dorthin. Gefragt
   * wurde deshalb über Funk — bei der Zentrale, die dann in ihrer Zeitleiste
   * suchen musste, während draußen jemand wartet.
   *
   * Die Zahl in der Kopfleiste ist jetzt der Knopf dorthin. Das ist die
   * kürzeste Verbindung zwischen der Frage und ihrer Antwort.
   */
  import { api } from '../lib/session.svelte.js'

  let { onClose = null } = $props()

  let data = $state(null)
  let loading = $state(true)
  let error = $state(null)

  async function load() {
    try {
      data = await api.get('/api/opx/ledger')
      error = null
    } catch (err) {
      error = err.message
    } finally {
      loading = false
    }
  }

  $effect(() => {
    load()
  })

  function time(iso) {
    if (!iso) return ''
    const d = new Date(iso)
    return d.toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit' })
  }

  function sign(n) {
    return n > 0 ? `+${n}` : `${n}`
  }
</script>

<div
  class="veil"
  role="button"
  tabindex="-1"
  onclick={(e) => e.target === e.currentTarget && onClose?.()}
  onkeydown={(e) => e.key === 'Escape' && onClose?.()}
>
  <section class="sheet">
    <header>
      <div class="head">
        <span class="label">Punktekonto</span>
        <h2>Woher der Stand kommt</h2>
      </div>
      <button class="ghost small" onclick={() => onClose?.()}>Schließen</button>
    </header>

    {#if loading}
      <p class="dim">Wird geladen …</p>
    {:else if error}
      <p class="err">{error}</p>
    {:else if !data || data.entries.length === 0}
      <p class="dim">
        Noch keine Buchung. Punkte gibt es für erreichte Zwischenziele und
        gelöste Rätsel, Abzüge für verpasste Fristen.
      </p>
    {:else}
      <div class="stand">
        <div>
          <span class="label">Punkte</span>
          <span class="value mono">{data.points}</span>
        </div>
        <div>
          <span class="label">Fluchtpunkte</span>
          <span class="value mono">{data.fp}</span>
        </div>
      </div>

      <ul class="scroll">
        {#each data.entries as e (e.id)}
          <li>
            <span class="mono t">{time(e.occurredAt)}</span>
            <span class="why">{e.reason || 'Ohne Angabe'}</span>
            <span class="delta mono">
              {#if e.deltaPoints}
                <b class:neg={e.deltaPoints < 0}>{sign(e.deltaPoints)} Pkt</b>
              {/if}
              {#if e.deltaFp}
                <b class:neg={e.deltaFp < 0}>{sign(e.deltaFp)} FP</b>
              {/if}
            </span>
            <!--
              Der Stand nach der Buchung, nicht nur die Änderung: Erst damit
              lässt sich der Weg von oben nach unten nachvollziehen, ohne im
              Kopf zu rechnen.
            -->
            <span class="after mono">{e.points} Pkt · {e.fp} FP</span>
          </li>
        {/each}
      </ul>
    {/if}
  </section>
</div>

<style>
  .veil {
    position: fixed;
    inset: 0;
    z-index: 50;
    display: grid;
    place-items: center;
    padding: 1rem;
    background: rgba(5, 9, 11, 0.9);
    border: none;
  }

  .sheet {
    width: min(100%, 42rem);
    max-height: min(100%, 42rem);
    background: var(--sheet);
    border: 1px solid var(--rule-hi);
    border-top: 3px solid var(--hq);
    padding: 1rem 1.1rem 1.1rem;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    min-height: 0;
  }

  header {
    display: flex;
    align-items: flex-start;
    gap: 0.8rem;
    border-bottom: 1px solid var(--rule);
    padding-bottom: 0.5rem;
  }

  .head {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
  }

  h2 {
    font-size: var(--fs-lg);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .stand {
    display: flex;
    gap: 1.6rem;
    border-bottom: 1px solid var(--rule);
    padding-bottom: 0.5rem;
  }

  .stand .value {
    font-size: var(--fs-lg);
    display: block;
  }

  .scroll {
    overflow-y: auto;
    min-height: 0;
    display: flex;
    flex-direction: column;
  }

  li {
    display: grid;
    grid-template-columns: 3.2rem 1fr auto;
    grid-template-areas: 't why delta' '. after after';
    gap: 0.1rem 0.6rem;
    padding: 0.45rem 0;
    border-bottom: 1px solid var(--rule);
    align-items: baseline;
  }

  .t {
    grid-area: t;
    color: var(--muted);
  }

  .why {
    grid-area: why;
  }

  .delta {
    grid-area: delta;
    white-space: nowrap;
  }

  .delta b {
    color: var(--ok);
    margin-left: 0.5rem;
  }

  .delta b.neg {
    color: var(--crit);
  }

  .after {
    grid-area: after;
    color: var(--muted);
    font-size: var(--fs-sm);
  }
</style>
