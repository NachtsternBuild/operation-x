<script>
  /*
   * Beweisprüfung der Einsatzzentrale.
   *
   * Solange nichts entschieden ist, steht Mister X am Ziel und wartet, während
   * seine Frist läuft. Deshalb stehen wartende Einreichungen oben und die
   * beiden Knöpfe sind groß genug, um sie ohne Zielen zu treffen.
   */
  import { api } from '../lib/session.svelte.js'
  import { stream } from '../lib/stream.svelte.js'

  let items = $state([])
  let error = $state(null)
  let busy = $state(null)
  let note = $state({})
  let zoom = $state(null)

  // Geholte Bilder, je Einreichung. Sie liegen hinter der Anmeldung, deshalb
  // kann das <img> sie nicht selbst laden — wir holen sie und merken uns die
  // Objektadresse, bis die Seite verlassen wird.
  let bilder = $state({})

  async function bildHolen(item) {
    if (!item.photoUrl || bilder[item.id]) return
    try {
      bilder[item.id] = await api.bild(item.photoUrl)
    } catch {
      /* Ohne Bild bleibt die Einreichung trotzdem entscheidbar. */
    }
  }

  async function load() {
    try {
      const res = await api.get('/api/opx/hq/evidence')
      items = res.evidence ?? []
      error = null
      for (const item of items) bildHolen(item)
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
    return () => {
      clearInterval(t)
      for (const url of Object.values(bilder)) URL.revokeObjectURL(url)
    }
  })

  async function decide(item, accept) {
    busy = item.id
    try {
      await api.post(`/api/opx/hq/evidence/${item.id}`, {
        accept,
        note: note[item.id] ?? '',
      })
      await load()
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  function time(iso) {
    return new Date(iso).toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit' })
  }

  const pending = $derived(items.filter((i) => i.status === 'pending'))
  const decided = $derived(items.filter((i) => i.status !== 'pending'))
</script>

{#if error}
  <p class="err">{error}</p>
{/if}

<section class="group">
  <header>
    <h3>Wartet auf Prüfung</h3>
    <span class="count mono">{pending.length}</span>
  </header>

  {#if pending.length === 0}
    <p class="dim">Nichts offen.</p>
  {:else}
    <div class="cards">
      {#each pending as item (item.id)}
        <article class="card">
          <header class="c-head">
            <span class="who">{item.callsign}</span>
            <span class="where mono">#{String(item.hotspotNumber).padStart(2, '0')} {item.hotspotName}</span>
            <span class="when mono">{time(item.capturedAt)}</span>
          </header>

          {#if item.task}
            <p class="auftrag">{item.task}</p>
          {/if}

          {#if item.photoUrl && bilder[item.id]}
            <button class="shot" onclick={() => (zoom = bilder[item.id])}>
              <img src={bilder[item.id]} alt="Eingereichtes Beweisfoto von {item.callsign}" />
            </button>
          {:else if item.photoUrl}
            <p class="dim">Foto wird geladen …</p>
          {:else}
            <p class="dim">Kein Foto, nur Code: {item.passcodeEntered || '—'}</p>
          {/if}

          <p class="dist mono">
            {Math.round(item.distanceM)} m vom Punkt · Zwischenziel {item.seq}
          </p>

          <input
            placeholder="Bemerkung (bei Ablehnung hilfreich)"
            value={note[item.id] ?? ''}
            oninput={(e) => (note = { ...note, [item.id]: e.target.value })}
          />

          <div class="actions">
            <button class="ok" onclick={() => decide(item, true)} disabled={busy === item.id}>
              Annehmen
            </button>
            <button class="no" onclick={() => decide(item, false)} disabled={busy === item.id}>
              Ablehnen
            </button>
          </div>
        </article>
      {/each}
    </div>
  {/if}
</section>

{#if decided.length}
  <section class="group">
    <header>
      <h3>Entschieden</h3>
      <span class="count mono">{decided.length}</span>
    </header>
    <ul class="log">
      {#each decided as item (item.id)}
        <li data-status={item.status}>
          <span class="mono">{time(item.capturedAt)}</span>
          <span>{item.callsign}</span>
          <span class="mono">#{String(item.hotspotNumber).padStart(2, '0')} {item.hotspotName}</span>
          <span class="verdict">{item.status === 'accepted' ? 'angenommen' : 'abgelehnt'}</span>
        </li>
      {/each}
    </ul>
  </section>
{/if}

{#if zoom}
  <button class="lightbox" onclick={() => (zoom = null)} aria-label="Ansicht schließen">
    <img src={zoom} alt="Beweisfoto in voller Größe" />
  </button>
{/if}

<style>
  .group {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .group header {
    display: flex;
    align-items: baseline;
    gap: 0.6rem;
    border-bottom: 1px solid var(--rule);
    padding-bottom: 0.4rem;
  }

  h3 {
    font-size: var(--fs-base);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }

  .count {
    font-size: var(--fs-xs);
    color: var(--muted);
  }

  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(15rem, 1fr));
    gap: 0.6rem;
  }

  .card {
    background: var(--sheet);
    border: 1px solid var(--rule);
    border-top: 2px solid var(--warn);
    padding: 0.7rem 0.8rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  /* Was an diesem Ort verlangt war — die Messlatte für das Foto darunter. */
  .auftrag {
    margin: 0;
    font-size: var(--fs-sm);
    line-height: 1.4;
    color: var(--muted);
    border-left: 2px solid var(--rule-hi);
    padding-left: 0.6rem;
  }

  .c-head {
    display: flex;
    flex-wrap: wrap;
    gap: 0.2rem 0.6rem;
    align-items: baseline;
    border: none;
    padding: 0;
  }

  .who {
    font-family: var(--display);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--x);
  }

  .where,
  .when,
  .dist {
    font-size: var(--fs-xs);
    color: var(--muted);
  }

  .shot {
    padding: 0;
    border: 1px solid var(--rule);
    background: var(--ground);
    min-height: auto;
    overflow: hidden;
    cursor: zoom-in;
  }

  .shot img {
    display: block;
    width: 100%;
    height: 9rem;
    object-fit: cover;
  }

  .actions {
    display: flex;
    gap: 0.4rem;
  }

  .actions button {
    flex: 1;
    min-height: 2.4rem;
  }

  .ok {
    color: var(--ok);
    border-color: var(--ok);
    background: rgba(78, 201, 160, 0.12);
  }

  .no {
    color: var(--x);
    border-color: var(--x);
    background: var(--x-dim);
  }

  .log {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .log li {
    display: grid;
    grid-template-columns: 4rem 8rem 1fr auto;
    gap: 0.7rem;
    padding: 0.35rem 0.6rem;
    background: var(--sheet);
    border-left: 2px solid var(--rule-hi);
    font-size: var(--fs-sm);
  }

  .log li[data-status='accepted'] { border-left-color: var(--ok); }
  .log li[data-status='rejected'] { border-left-color: var(--x); }

  .verdict {
    font-size: var(--fs-xs);
    color: var(--muted);
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

  .lightbox {
    position: fixed;
    inset: 0;
    z-index: 50;
    display: grid;
    place-items: center;
    background: rgba(5, 9, 11, 0.94);
    border: none;
    padding: 2rem;
    cursor: zoom-out;
  }

  .lightbox img {
    max-width: 100%;
    max-height: 100%;
    object-fit: contain;
  }
</style>
