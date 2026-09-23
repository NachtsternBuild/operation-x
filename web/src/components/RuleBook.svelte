<script>
  /*
   * Der Regelnachschlag.
   *
   * Die häufigste Frage am Spieltag ist, was etwas kostet — und sie kommt
   * unterwegs, mit einer Hand am Lenker. Deshalb liegt das Verzeichnis auf
   * jedem Bildschirm hinter einem Knopf und nicht in einem eigenen Menüpunkt,
   * und deshalb hat es eine Suche statt einer Gliederung zum Durchblättern.
   *
   * Dieselbe Ansicht dient der Zentrale zum Stellen der Werte. Das ist kein
   * Sparzwang, sondern das Richtige: Wer einen Wert ändert, soll denselben
   * Satz lesen, den die Spieler lesen — sonst ändert er etwas anderes, als er
   * glaubt.
   */
  import { api, session } from '../lib/session.svelte.js'

  let { onClose = null } = $props()

  let groups = $state([])
  let editable = $state(false)
  let loading = $state(true)
  let error = $state(null)
  let query = $state('')

  // Geänderte Werte sammeln sich, bis gespeichert wird: Wer den Meldeabstand
  // verlängert, will meist auch die Strafe anfassen.
  let draft = $state({})
  let saving = $state(false)
  let note = $state(null)

  async function load() {
    try {
      const res = await api.get('/api/opx/rules')
      groups = res.groups ?? []
      editable = !!res.editable
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

  const units = {
    min: 'Min.',
    sek: 'Sek.',
    m: 'm',
    punkte: 'Punkte',
    fp: 'FP',
    anzahl: '',
  }

  const sideNames = { misterx: 'Zielperson', detective: 'Fahndung' }

  const filtered = $derived.by(() => {
    const q = query.trim().toLowerCase()
    if (!q) return groups
    return groups
      .map((g) => ({
        ...g,
        rules: g.rules.filter(
          (r) =>
            r.name.toLowerCase().includes(q) ||
            r.what.toLowerCase().includes(q) ||
            r.key.toLowerCase().includes(q),
        ),
      }))
      .filter((g) => g.rules.length > 0)
  })

  const dirty = $derived(Object.keys(draft).length > 0)

  function edit(rule, value) {
    const n = Number(value)
    if (!Number.isFinite(n)) return
    if (n === rule.value) {
      const { [rule.key]: _drop, ...rest } = draft
      draft = rest
    } else {
      draft = { ...draft, [rule.key]: n }
    }
  }

  async function save() {
    saving = true
    error = null
    try {
      const res = await api.post('/api/opx/setup/rules', { values: draft })
      note = `${res.changed} Wert${res.changed === 1 ? '' : 'e'} geändert.`
      draft = {}
      await load()
    } catch (err) {
      error = err.message
    } finally {
      saving = false
    }
  }

  /** Anzeigewert: der Entwurf, sonst der gespeicherte Stand. */
  function shown(rule) {
    return rule.key in draft ? draft[rule.key] : rule.value
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
        <span class="label">Regelnachschlag</span>
        <h2>{editable ? 'Regelwerte' : 'Was gilt'}</h2>
      </div>
      <button class="ghost small" onclick={() => onClose?.()}>Schließen</button>
    </header>

    <input
      class="search"
      bind:value={query}
      placeholder="Suchen — „Nebelkerze“, „Sperre“, „Meldung“ …"
      autocomplete="off"
    />

    {#if loading}
      <p class="dim">Wird geladen …</p>
    {:else if error}
      <p class="err">{error}</p>
    {:else}
      <div class="scroll">
        {#each filtered as g (g.group)}
          <section class="group">
            <h3>{g.group}</h3>
            <ul>
              {#each g.rules as r (r.key)}
                <li class:changed={r.key in draft}>
                  <span class="name">
                    {r.name}
                    {#if r.side && session.team?.role === 'hq'}
                      <span class="side" data-side={r.side}>{sideNames[r.side]}</span>
                    {/if}
                  </span>

                  {#if editable}
                    <span class="field">
                      <input
                        type="number"
                        value={shown(r)}
                        min={r.min}
                        max={r.max}
                        onchange={(e) => edit(r, e.currentTarget.value)}
                      />
                      <span class="unit mono">{units[r.unit] ?? ''}</span>
                    </span>
                  {:else}
                    <span class="value mono">
                      {r.value}<span class="unit"> {units[r.unit] ?? ''}</span>
                    </span>
                  {/if}

                  <span class="what">{r.what}</span>
                </li>
              {/each}
            </ul>
          </section>
        {/each}

        {#if filtered.length === 0}
          <p class="dim">Nichts gefunden. Andere Wörter versuchen.</p>
        {/if}
      </div>

      {#if editable}
        <footer>
          {#if note}<span class="ok">{note}</span>{/if}
          <button class="primary" onclick={save} disabled={!dirty || saving}>
            {saving ? 'Speichere …' : dirty ? `${Object.keys(draft).length} übernehmen` : 'Nichts geändert'}
          </button>
        </footer>
      {/if}
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
    width: min(100%, 46rem);
    max-height: min(100%, 46rem);
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

  .search {
    flex: none;
  }

  .scroll {
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 0.9rem;
    min-height: 0;
  }

  h3 {
    font-family: var(--display);
    font-size: var(--fs-sm);
    text-transform: uppercase;
    letter-spacing: 0.12em;
    color: var(--hq);
    border-bottom: 1px solid var(--rule);
    padding-bottom: 0.25rem;
    margin-bottom: 0.4rem;
  }

  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }

  li {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 0.1rem 0.8rem;
    background: var(--sheet-2);
    padding: 0.4rem 0.55rem;
    align-items: center;
  }

  li.changed {
    outline: 1px solid var(--hq);
  }

  .name {
    color: var(--text-hi);
    font-size: var(--fs-sm);
    display: flex;
    align-items: baseline;
    gap: 0.4rem;
  }

  .side {
    font-family: var(--mono);
    font-size: var(--fs-xs);
    color: var(--faint);
  }

  .side[data-side='misterx'] {
    color: var(--x);
  }

  .side[data-side='detective'] {
    color: var(--det);
  }

  .value {
    color: var(--text-hi);
    font-size: var(--fs-base);
    white-space: nowrap;
  }

  .unit {
    color: var(--muted);
    font-size: var(--fs-xs);
  }

  .field {
    display: flex;
    align-items: center;
    gap: 0.3rem;
  }

  .field input {
    width: 5.5rem;
    text-align: right;
    min-height: 2.1rem;
  }

  .what {
    grid-column: 1 / -1;
    font-size: var(--fs-xs);
    color: var(--muted);
    line-height: 1.4;
  }

  footer {
    display: flex;
    align-items: center;
    gap: 0.7rem;
    border-top: 1px solid var(--rule);
    padding-top: 0.6rem;
  }

  footer .ok {
    flex: 1;
    font-size: var(--fs-sm);
    color: var(--ok);
  }

  .dim {
    font-size: var(--fs-sm);
    color: var(--muted);
  }

  .err {
    font-size: var(--fs-sm);
    color: var(--x);
    border-left: 2px solid var(--x);
    padding-left: 0.6rem;
  }
</style>
