<script>
  /*
   * Rätselpult der Einsatzzentrale.
   *
   * Freischalten, Lösung nachlesen, Versuche mitlesen. Der letzte Punkt ist im
   * Spiel der wichtigste: Wenn drei Teams dieselbe falsche Antwort einreichen,
   * ist nicht die Gruppe zu langsam, sondern die Frage schlecht gestellt – und
   * dann sollte die Zentrale nachhelfen können, bevor die Stimmung kippt.
   */
  import { api } from '../lib/session.svelte.js'
  import { stream } from '../lib/stream.svelte.js'

  let puzzles = $state([])
  let attempts = $state([])
  let error = $state(null)
  let busy = $state(null)
  let reveal = $state({})

  // Der Schreibplatz. Ein offenes Formular statt eines eigenen Bildschirms:
  // Rätsel schreibt man selten am Stück, sondern eines, wenn einem eines
  // einfällt — und dann will man die schon vorhandenen daneben sehen.
  let meta = $state(null)
  let editing = $state(null)
  let form = $state(blank())

  function blank() {
    return {
      code: '', type: 'A', title: '', question: '',
      answer: '', hint: '', category: 'yellow', points: 0,
      order: 0, unlocked: false,
    }
  }

  function startNew() {
    editing = 'neu'
    form = blank()
    form.order = puzzles.length + 1
  }

  function startEdit(p) {
    editing = p.id
    form = {
      code: p.code ?? '', type: p.type ?? 'A', title: p.title ?? '',
      question: p.question ?? '', answer: p.answer ?? '', hint: p.hint ?? '',
      category: p.intelCategory ?? 'yellow', points: p.points ?? 0,
      order: p.order ?? 0, unlocked: !!p.unlocked,
    }
  }

  async function save() {
    if (!form.title.trim() || !form.question.trim() || !form.answer.trim()) return
    busy = 'save'
    try {
      if (editing === 'neu') {
        await api.post('/api/opx/setup/puzzles', form)
      } else {
        await api.patch(`/api/opx/setup/puzzles/${editing}`, form)
      }
      editing = null
      await load()
      error = null
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  async function remove(p) {
    busy = p.id
    try {
      await api.del(`/api/opx/setup/puzzles/${p.id}`)
      await load()
      error = null
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  async function addExamples() {
    busy = 'examples'
    try {
      await api.post('/api/opx/setup/puzzles/examples', {})
      await load()
      error = null
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  const typeHint = $derived(
    meta?.types?.find((t) => t.type === form.type)?.hint ?? ''
  )

  async function load() {
    try {
      const res = await api.get('/api/opx/hq/puzzles')
      puzzles = res.puzzles ?? []
      attempts = res.attempts ?? []
      if (!meta) meta = await api.get('/api/opx/setup/puzzle-meta')
      error = null
    } catch (err) {
      if (err.status !== 404) error = err.message
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

  async function toggle(p) {
    busy = p.id
    try {
      await api.post(`/api/opx/hq/puzzles/${p.id}/unlock`, { unlocked: !p.unlocked })
      await load()
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  const typeNames = {
    A: 'Logik',
    B: 'Foto',
    C: 'Zahlen',
    D: 'Geometrie',
    E: 'Vor Ort',
    F: 'Kombination',
  }

  const catNames = { green: '🟢 sicher', yellow: '🟡 wahrscheinlich', red: '🔴 Indiz' }

  function triesFor(id) {
    return attempts.filter((a) => a.puzzle === id)
  }

  function time(iso) {
    return new Date(iso).toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit' })
  }
</script>

<div class="bar">
  <button class="primary" onclick={startNew} disabled={editing === 'neu'}>
    Rätsel schreiben
  </button>
  {#if puzzles.length === 0}
    <button class="ghost" onclick={addExamples} disabled={busy === 'examples'}>
      Beispielsatz für den Anfang
    </button>
  {/if}
  <span class="dim mono">{puzzles.length} angelegt</span>
</div>

{#if error}<p class="err">{error}</p>{/if}

{#if editing}
  <section class="editor">
    <header>
      <h3>{editing === 'neu' ? 'Neues Rätsel' : 'Rätsel ändern'}</h3>
      <button class="ghost small" onclick={() => (editing = null)}>Abbrechen</button>
    </header>

    <div class="grid">
      <label>
        <span class="label">Art</span>
        <select bind:value={form.type}>
          {#each meta?.types ?? [] as t}
            <option value={t.type}>{t.type} · {t.name}</option>
          {/each}
        </select>
      </label>

      <label>
        <span class="label">Kürzel</span>
        <input bind:value={form.code} placeholder="wird vergeben" />
      </label>

      <label>
        <span class="label">Punkte</span>
        <input type="number" bind:value={form.points} placeholder={meta?.defaultPoints ?? 15} />
      </label>

      <label>
        <span class="label">Reihenfolge</span>
        <input type="number" bind:value={form.order} />
      </label>
    </div>

    {#if typeHint}<p class="dim">{typeHint}</p>{/if}

    <label>
      <span class="label">Titel</span>
      <input bind:value={form.title} placeholder="Ausschlussverfahren" />
    </label>

    <label>
      <span class="label">Frage</span>
      <textarea bind:value={form.question} rows="4"
        placeholder="Was die Teams zu lesen bekommen."></textarea>
    </label>

    <label>
      <span class="label">Lösung</span>
      <input bind:value={form.answer}
        placeholder="Goldener Reiter|Der Goldene Reiter" />
    </label>
    <p class="dim">
      Mehrere zulässige Schreibweisen mit senkrechtem Strich trennen. Groß- und
      Kleinschreibung sowie Umlaute werden ohnehin großzügig behandelt.
    </p>

    <label>
      <span class="label">Tipp, falls es klemmt</span>
      <input bind:value={form.hint} placeholder="Neustädter Seite, an der Augustusbrücke." />
    </label>

    <label>
      <span class="label">Qualität des Hinweises, den das Lösen erzeugt</span>
      <select bind:value={form.category}>
        {#each meta?.categories ?? [] as c}
          <option value={c.key}>{c.name} — {c.hint}</option>
        {/each}
      </select>
    </label>

    <label class="check">
      <input type="checkbox" bind:checked={form.unlocked} />
      <span>Sofort freischalten</span>
    </label>

    <button class="primary" onclick={save}
      disabled={busy === 'save' || !form.title.trim() || !form.question.trim() || !form.answer.trim()}>
      {busy === 'save' ? 'Speichere …' : 'Speichern'}
    </button>
  </section>
{/if}

<div class="list">
  {#each puzzles as p (p.id)}
    <article class="card" class:locked={!p.unlocked} class:solved={p.solved}>
      <header>
        <span class="code mono">{p.code}</span>
        <span class="type">{typeNames[p.type] ?? p.type}</span>
        <span class="title">{p.title}</span>
        <span class="cat mono">{catNames[p.intelCategory] ?? ''}</span>
      </header>

      <p class="question">{p.question}</p>

      <div class="row">
        <button
          class:primary={!p.unlocked}
          class:ghost={p.unlocked}
          onclick={() => toggle(p)}
          disabled={busy === p.id || p.solved}
        >
          {p.solved ? 'gelöst' : p.unlocked ? 'Sperren' : 'Freischalten'}
        </button>

        <button class="ghost" onclick={() => (reveal = { ...reveal, [p.id]: !reveal[p.id] })}>
          {reveal[p.id] ? 'Lösung verbergen' : 'Lösung zeigen'}
        </button>

        <span class="pts mono">{p.points} Pkt</span>
      </div>

      <div class="row">
        <button class="ghost small" onclick={() => startEdit(p)}>Ändern</button>
        {#if !p.attempts}
          <button class="ghost small danger" onclick={() => remove(p)} disabled={busy === p.id}>
            Löschen
          </button>
        {/if}
      </div>

      {#if reveal[p.id]}
        <p class="answer mono">{p.answer}</p>
      {/if}

      {#if triesFor(p.id).length > 0}
        <ul class="tries">
          {#each triesFor(p.id).slice(0, 5) as a}
            <li class:right={a.correct}>
              <span class="mono">{time(a.at)}</span>
              <span>{a.callsign}</span>
              <span class="given">„{a.answer}“</span>
              <span class="mark">{a.correct ? '✓' : '✗'}</span>
            </li>
          {/each}
        </ul>
      {/if}
    </article>
  {/each}
</div>

{#if puzzles.length === 0 && !editing}
  <p class="dim">
    Noch keine Rätsel angelegt. Ohne sie haben die Fahndungsteams nichts zu tun —
    entweder selbst schreiben oder den Beispielsatz nehmen und anpassen.
  </p>
{/if}

<style>
  .bar {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.7rem;
  }

  .bar .dim {
    margin-left: auto;
    font-size: var(--fs-xs);
  }

  .editor {
    background: var(--sheet);
    border: 1px solid var(--rule-hi);
    border-top: 2px solid var(--hq);
    padding: 0.9rem 1rem 1rem;
    margin-bottom: 0.8rem;
    display: flex;
    flex-direction: column;
    gap: 0.55rem;
  }

  .editor header {
    display: flex;
    align-items: baseline;
    gap: 0.7rem;
    border-bottom: 1px solid var(--rule);
    padding-bottom: 0.45rem;
  }

  .editor h3 {
    flex: 1;
    font-size: var(--fs-base);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }

  .editor label {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  .editor label.check {
    flex-direction: row;
    align-items: center;
    gap: 0.5rem;
  }

  .editor .grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(8rem, 1fr));
    gap: 0.5rem;
  }

  .editor textarea {
    font-family: var(--body);
    resize: vertical;
  }

  button.danger {
    color: var(--x);
    border-color: var(--x);
  }

  .list {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(20rem, 1fr));
    gap: 0.7rem;
    overflow-y: auto;
  }

  .card {
    background: var(--sheet);
    border: 1px solid var(--rule);
    border-top: 2px solid var(--hq);
    padding: 0.75rem 0.85rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .card.locked {
    border-top-color: var(--rule-hi);
    opacity: 0.75;
  }

  .card.solved {
    border-top-color: var(--ok);
  }

  header {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0.3rem 0.6rem;
  }

  .code {
    font-size: var(--fs-xs);
    color: var(--hq);
  }

  .type {
    font-family: var(--display);
    text-transform: uppercase;
    letter-spacing: 0.1em;
    font-size: var(--fs-xs);
    color: var(--muted);
  }

  .title {
    flex: 1;
    min-width: 0;
    color: var(--text-hi);
    font-size: var(--fs-sm);
  }

  .cat,
  .pts {
    font-size: var(--fs-xs);
    color: var(--faint);
  }

  .question {
    font-size: var(--fs-xs);
    color: var(--muted);
    line-height: 1.5;
  }

  .row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }

  .row button {
    min-height: 2.1rem;
    padding: 0 0.7rem;
    font-size: var(--fs-xs);
  }

  .pts {
    margin-left: auto;
  }

  .answer {
    font-size: var(--fs-sm);
    color: var(--ok);
    background: var(--sheet-2);
    border-left: 2px solid var(--ok);
    padding: 0.35rem 0.6rem;
  }

  .tries {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 1px;
    border-top: 1px solid var(--rule);
    padding-top: 0.4rem;
  }

  .tries li {
    display: grid;
    grid-template-columns: 3.2rem 5.5rem 1fr auto;
    gap: 0.5rem;
    font-size: var(--fs-xs);
    color: var(--muted);
    align-items: baseline;
  }

  .given {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .mark {
    color: var(--x);
  }

  .tries li.right .mark {
    color: var(--ok);
  }

  .dim,
  .err {
    font-size: var(--fs-sm);
    color: var(--muted);
    border-left: 2px solid var(--rule-hi);
    padding-left: 0.7rem;
    line-height: 1.5;
  }

  .err {
    color: var(--x);
    border-left-color: var(--x);
  }
</style>
