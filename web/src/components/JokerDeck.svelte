<script>
  /*
   * Die Jokerkarten.
   *
   * Beide Seiten sehen hier dasselbe Muster: Was kostet es, was bringt es, und
   * wieso geht es gerade nicht. Der letzte Punkt ist der wichtigste – ein
   * ausgegrauter Knopf ohne Begründung ist im Feld nur ärgerlich.
   */
  import { api } from '../lib/session.svelte.js'
  import { stream } from '../lib/stream.svelte.js'

  let { accent = 'det', sectors = [], hotspots = [], onUsed = null } = $props()

  let jokers = $state([])
  let fp = $state(0)
  let busy = $state(null)
  let feedback = $state(null)
  let picking = $state(null)
  let now = $state(Date.now())

  $effect(() => {
    const t = setInterval(() => (now = Date.now()), 1000)
    return () => clearInterval(t)
  })

  async function load() {
    try {
      const res = await api.get('/api/opx/jokers')
      jokers = res.jokers ?? []
      fp = res.fp ?? 0
    } catch {
      /* vor dem Spielstart gibt es nichts */
    }
  }

  $effect(() => {
    // Nachladen, wenn der Server eine Änderung meldet. Der Takt daneben ist
    // nur die Rückfallebene für den Fall, dass der Strom nicht steht.
    void stream.revision
    load()
    const t = setInterval(load, 60_000)
    return () => clearInterval(t)
  })

  // Sperrzone, Wanze und Freischaltung brauchen ein Ziel.
  function needsTarget(kind) {
    return kind === 'lockdown' || kind === 'scan' || kind === 'bug'
  }

  async function use(joker, extra = {}) {
    busy = joker.kind
    feedback = null
    picking = null

    try {
      const res = await api.post('/api/opx/jokers', { kind: joker.kind, ...extra })
      feedback = { ok: true, text: res.message, detail: res.text }
      await load()
      onUsed?.()
    } catch (err) {
      feedback = { ok: false, text: err.message }
    } finally {
      busy = null
    }
  }

  function activate(joker) {
    if (needsTarget(joker.kind)) {
      picking = picking === joker.kind ? null : joker.kind
      return
    }
    use(joker)
  }

  function remaining(iso) {
    const left = Math.max(0, Math.round((Date.parse(iso) - now) / 1000))
    return `${Math.floor(left / 60)}:${String(left % 60).padStart(2, '0')}`
  }
</script>

<section class="deck" data-accent={accent}>
  <header>
    <h3>Einsatzmittel</h3>
    <span class="fp mono">{fp} FP</span>
  </header>

  {#if jokers.length === 0}
    <p class="dim">Noch keine Einsatzmittel verfügbar.</p>
  {:else}
    <ul class="cards">
      {#each jokers as j (j.kind)}
        <li class:spent={!j.available} class:running={j.activeUntil}>
          <button class="card" onclick={() => activate(j)} disabled={!j.available || busy === j.kind}>
            <span class="top">
              <span class="name">{j.name}</span>
              <span class="cost mono">{j.cost} FP</span>
            </span>
            <span class="desc">{j.description}</span>
            <span class="foot mono">
              {#if j.activeUntil}
                <b class="live">läuft noch {remaining(j.activeUntil)}</b>
              {:else if !j.available}
                {j.reason}
              {:else if j.maxUses}
                noch {j.maxUses - j.used} von {j.maxUses}
              {/if}
            </span>
          </button>

          {#if picking === j.kind}
            <div class="picker">
              {#if j.kind === 'bug'}
                <select onchange={(e) => e.target.value && use(j, { hotspotId: e.target.value })}>
                  <option value="">Hotspot wählen …</option>
                  {#each hotspots as h (h.id)}
                    <option value={h.id}>#{String(h.number).padStart(2, '0')} {h.name}</option>
                  {/each}
                </select>
              {:else}
                <select onchange={(e) => e.target.value && use(j, { sectorId: e.target.value })}>
                  <option value="">Sektor wählen …</option>
                  {#each sectors as s (s.id)}
                    <option value={s.id}>{s.code} {s.name}</option>
                  {/each}
                </select>
              {/if}
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}

  {#if feedback}
    <p class="feedback" class:ok={feedback.ok} role="alert">
      {feedback.text}
      {#if feedback.detail}<br /><span class="mono">{feedback.detail}</span>{/if}
    </p>
  {/if}
</section>

<style>
  .deck {
    background: var(--sheet);
    border: 1px solid var(--rule);
    border-top: 2px solid var(--rule);
    padding: 0.8rem 0.85rem;
    display: flex;
    flex-direction: column;
    gap: 0.55rem;
  }

  .deck[data-accent='x'] { border-top-color: var(--x); }
  .deck[data-accent='det'] { border-top-color: var(--det); }

  header {
    display: flex;
    align-items: baseline;
    gap: 0.6rem;
    border-bottom: 1px solid var(--rule);
    padding-bottom: 0.45rem;
  }

  h3 {
    font-size: var(--fs-base);
    text-transform: uppercase;
    letter-spacing: 0.08em;
    flex: 1;
  }

  .fp {
    font-size: var(--fs-base);
    color: var(--hq);
  }

  .cards {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .card {
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 0.12rem;
    align-items: flex-start;
    text-align: left;
    padding: 0.5rem 0.6rem;
    min-height: auto;
    background: var(--sheet-2);
    border: 1px solid var(--rule);
    border-left: 3px solid var(--rule-hi);
    font-family: var(--body);
    text-transform: none;
    letter-spacing: 0;
  }

  li.spent .card {
    opacity: 0.5;
  }

  li.running .card {
    border-left-color: var(--ok);
  }

  .top {
    display: flex;
    width: 100%;
    align-items: baseline;
    gap: 0.6rem;
  }

  .name {
    flex: 1;
    font-family: var(--display);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    font-size: var(--fs-sm);
    color: var(--text-hi);
  }

  .cost {
    font-size: var(--fs-xs);
    color: var(--hq);
  }

  .desc {
    font-size: var(--fs-xs);
    color: var(--muted);
    line-height: 1.4;
  }

  .foot {
    font-size: var(--fs-xs);
    color: var(--faint);
  }

  .foot .live {
    color: var(--ok);
    font-weight: 500;
  }

  .picker {
    padding: 0.35rem 0.6rem 0.5rem;
    background: var(--sheet-2);
  }

  .dim {
    font-size: var(--fs-xs);
    color: var(--muted);
  }

  .feedback {
    font-size: var(--fs-sm);
    color: var(--x);
    border-left: 2px solid var(--x);
    padding-left: 0.6rem;
    line-height: 1.45;
  }

  .feedback.ok {
    color: var(--ok);
    border-left-color: var(--ok);
  }
</style>
