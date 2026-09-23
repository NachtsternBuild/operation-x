<script>
  /*
   * Hinweistafel und Rätselmodul.
   *
   * Beide gehören zusammen: Ein gelöstes Rätsel erzeugt in dem Moment einen
   * Hinweis aus der tatsächlichen Lage. Wer schnell löst, bekommt einen
   * frischen; wer lange braucht, eine historische Spur. Deshalb steht die
   * Frische groß dran.
   */
  import { api } from '../lib/session.svelte.js'
  import { stream } from '../lib/stream.svelte.js'

  let { onSolved = null } = $props()

  let puzzles = $state([])
  let intel = $state([])
  let open = $state(null)
  let answer = $state('')
  let busy = $state(false)
  let feedback = $state(null)
  let tab = $state('intel')

  async function load() {
    try {
      const [p, i] = await Promise.all([
        api.get('/api/opx/puzzles'),
        api.get('/api/opx/intel'),
      ])
      puzzles = p.puzzles ?? []
      intel = i.intel ?? []
    } catch {
      /* beim Start gibt es noch nichts */
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

  async function solve(puzzle) {
    if (!answer.trim()) return
    busy = true
    feedback = null

    try {
      const res = await api.post(`/api/opx/puzzles/${puzzle.id}/solve`, { answer })
      feedback = { ok: res.correct, text: res.message, intel: res.intel }
      if (res.correct) {
        answer = ''
        open = null
        await load()
        onSolved?.()
      }
    } catch (err) {
      feedback = { ok: false, text: err.message }
    } finally {
      busy = false
    }
  }

  const symbols = { green: '🟢', yellow: '🟡', red: '🔴' }
  const freshLabel = { hot: 'Heiß', warm: 'Warm', cold: 'Kalt' }

  function age(sec) {
    if (sec < 60) return 'gerade eben'
    const m = Math.floor(sec / 60)
    return m < 60 ? `vor ${m} min` : `vor ${Math.floor(m / 60)} h ${m % 60} min`
  }

  const openPuzzles = $derived(puzzles.filter((p) => !p.solved))
  const solvedPuzzles = $derived(puzzles.filter((p) => p.solved))
</script>

<section class="board">
  <nav class="tabs">
    <button class:on={tab === 'intel'} onclick={() => (tab = 'intel')}>
      Hinweise <span class="mono">{intel.length}</span>
    </button>
    <button class:on={tab === 'puzzles'} onclick={() => (tab = 'puzzles')}>
      Rätsel <span class="mono">{openPuzzles.length}</span>
    </button>
  </nav>

  {#if tab === 'intel'}
    {#if intel.length === 0}
      <p class="dim">
        Noch keine Hinweise. Sie entstehen, wenn ein Rätsel gelöst wird – und
        zwar aus der Lage in genau diesem Moment.
      </p>
    {:else}
      <ul class="intel">
        {#each intel as i (i.id)}
          <li data-fresh={i.freshness} data-cat={i.category}>
            <div class="i-head">
              <span class="sym">{symbols[i.category] ?? '•'}</span>
              <span class="fresh">{freshLabel[i.freshness] ?? i.freshness}</span>
              <span class="when mono">{age(i.ageSec)}</span>
            </div>
            <p class="i-text">{i.text}</p>
          </li>
        {/each}
      </ul>
    {/if}
  {:else}
    {#if puzzles.length === 0}
      <p class="dim">Noch keine Rätsel freigeschaltet.</p>
    {:else}
      <ul class="puzzles">
        {#each openPuzzles as p (p.id)}
          <li>
            <button class="p-head" onclick={() => { open = open === p.id ? null : p.id; feedback = null }}>
              <span class="p-code mono">{p.code}</span>
              <span class="p-title">{p.title}</span>
              <span class="p-points mono">{p.points} Pkt</span>
            </button>

            {#if open === p.id}
              <div class="p-body">
                <p class="question">{p.question}</p>
                {#if p.imageUrl}
                  <img src={p.imageUrl} alt="Bild zum Rätsel {p.code}" />
                {/if}
                {#if p.hint}
                  <details>
                    <summary>Denkanstoß</summary>
                    <p class="hint">{p.hint}</p>
                  </details>
                {/if}

                <div class="answer-row">
                  <input
                    bind:value={answer}
                    placeholder="Antwort"
                    onkeydown={(e) => e.key === 'Enter' && solve(p)}
                  />
                  <button class="primary" onclick={() => solve(p)} disabled={busy || !answer.trim()}>
                    Einreichen
                  </button>
                </div>

                {#if p.attempts > 0}
                  <p class="tries mono">{p.attempts} Versuche bisher</p>
                {/if}
              </div>
            {/if}
          </li>
        {/each}

        {#each solvedPuzzles as p (p.id)}
          <li class="done">
            <div class="p-head static">
              <span class="p-code mono">{p.code}</span>
              <span class="p-title">{p.title}</span>
              <span class="p-solved">gelöst{p.solvedByMe ? ' (von uns)' : ''}</span>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  {/if}

  {#if feedback}
    <p class="feedback" class:ok={feedback.ok} role="alert">
      {feedback.text}
      {#if feedback.intel}<br /><b>{feedback.intel.text}</b>{/if}
    </p>
  {/if}
</section>

<style>
  .board {
    background: var(--sheet);
    border: 1px solid var(--rule);
    border-top: 2px solid var(--det);
    padding: 0.8rem 0.85rem;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    min-height: 0;
  }

  .tabs {
    display: flex;
    gap: 0.35rem;
    border-bottom: 1px solid var(--rule);
    padding-bottom: 0.5rem;
  }

  .tabs button {
    flex: 1;
    min-height: 2.1rem;
    padding: 0 0.5rem;
    background: transparent;
    border-color: var(--rule);
    color: var(--muted);
    font-size: var(--fs-xs);
  }

  .tabs button.on {
    color: var(--det);
    border-color: var(--det);
    background: var(--det-dim);
  }

  .intel,
  .puzzles {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    overflow-y: auto;
    max-height: 26rem;
  }

  .intel li {
    background: var(--sheet-2);
    border-left: 3px solid var(--rule-hi);
    padding: 0.5rem 0.6rem;
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  /* Die Frische steuert die Auffälligkeit: Ein heißer Hinweis muss sofort
     ins Auge fallen, ein kalter zurücktreten. */
  .intel li[data-fresh='hot'] { border-left-color: var(--crit); }
  .intel li[data-fresh='warm'] { border-left-color: var(--warn); }
  .intel li[data-fresh='cold'] { border-left-color: var(--rule-hi); opacity: 0.72; }

  .i-head {
    display: flex;
    align-items: baseline;
    gap: 0.5rem;
  }

  .sym {
    font-size: 0.8rem;
  }

  .fresh {
    font-family: var(--display);
    text-transform: uppercase;
    letter-spacing: 0.12em;
    font-size: var(--fs-xs);
    color: var(--muted);
  }

  .intel li[data-fresh='hot'] .fresh { color: var(--crit); }
  .intel li[data-fresh='warm'] .fresh { color: var(--warn); }

  .when {
    margin-left: auto;
    font-size: var(--fs-xs);
    color: var(--faint);
  }

  .i-text {
    font-size: var(--fs-sm);
    color: var(--text-hi);
    line-height: 1.45;
  }

  .puzzles li {
    background: var(--sheet-2);
    border-left: 3px solid var(--det);
  }

  .puzzles li.done {
    border-left-color: var(--ok);
    opacity: 0.65;
  }

  .p-head {
    width: 100%;
    display: flex;
    align-items: baseline;
    gap: 0.6rem;
    text-align: left;
    min-height: 2.3rem;
    padding: 0 0.6rem;
    background: transparent;
    border: none;
    font-family: var(--body);
    text-transform: none;
    letter-spacing: 0;
    font-size: var(--fs-sm);
  }

  .p-head.static {
    cursor: default;
  }

  .p-code {
    font-size: var(--fs-xs);
    color: var(--det);
  }

  .p-title {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-hi);
  }

  .p-points,
  .p-solved,
  .tries {
    font-size: var(--fs-xs);
    color: var(--muted);
  }

  .p-solved {
    color: var(--ok);
  }

  .p-body {
    padding: 0 0.6rem 0.6rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .question {
    font-size: var(--fs-sm);
    line-height: 1.5;
    color: var(--text);
  }

  .p-body img {
    width: 100%;
    border: 1px solid var(--rule);
  }

  details summary {
    font-size: var(--fs-xs);
    color: var(--muted);
    cursor: pointer;
  }

  .hint {
    font-size: var(--fs-xs);
    color: var(--muted);
    padding-top: 0.3rem;
    line-height: 1.45;
  }

  .answer-row {
    display: flex;
    gap: 0.4rem;
  }

  .answer-row input {
    flex: 1;
    min-width: 0;
  }

  .dim {
    font-size: var(--fs-xs);
    color: var(--muted);
    line-height: 1.5;
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
