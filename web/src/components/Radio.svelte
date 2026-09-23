<script>
  /*
   * Der HQ-Funkkanal.
   *
   * Zwei Arten von Nachrichten laufen hier zusammen: Absprachen der
   * Fahndungsteams untereinander und Lagemeldungen der Zentrale. Nur letztere
   * tragen eine Vertrauensstufe – ein Team, das seine eigene Vermutung als
   * „bestätigt“ kennzeichnen könnte, würde die Abstufung wertlos machen.
   */
  import { api, session } from '../lib/session.svelte.js'
  import { stream } from '../lib/stream.svelte.js'

  let { compact = false } = $props()

  let messages = $state([])
  let text = $state('')
  let trust = $state('confirmed')
  let audience = $state('detectives')
  let busy = $state(false)
  let error = $state(null)
  let box

  const isHQ = $derived(session.team?.role === 'hq')
  const readOnly = $derived(session.team?.role === 'misterx')

  async function load() {
    try {
      const res = await api.get('/api/opx/radio')
      const wasAtBottom = !box || box.scrollHeight - box.scrollTop - box.clientHeight < 60
      messages = res.messages ?? []
      error = null

      // Nur nach unten scrollen, wenn der Leser ohnehin unten war – sonst
      // reißt es ihn beim Nachlesen aus dem Verlauf.
      if (wasAtBottom) {
        queueMicrotask(() => box && (box.scrollTop = box.scrollHeight))
      }
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

  async function send() {
    const value = text.trim()
    if (!value) return

    busy = true
    try {
      await api.post('/api/opx/radio', {
        text: value,
        ...(isHQ ? { trust, audience } : {}),
      })
      text = ''
      await load()
    } catch (err) {
      error = err.message
    } finally {
      busy = false
    }
  }

  const trustLabel = {
    confirmed: '🟢 Bestätigt',
    unconfirmed: '🟡 Unbestätigt',
    rumor: '🔴 Gerücht',
  }

  function time(iso) {
    return new Date(iso).toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit' })
  }
</script>

<section class="radio" class:compact>
  <header>
    <h3>Funkkanal</h3>
    <span class="count mono">{messages.length}</span>
  </header>

  <div class="log" bind:this={box}>
    {#if messages.length === 0}
      <p class="dim">Noch keine Meldungen.</p>
    {:else}
      {#each messages as m (m.id)}
        <article class="msg" class:mine={m.mine} class:system={!m.role} data-trust={m.trust}>
          <div class="head">
            <span class="who" style={m.color ? `color:${m.color}` : ''}>{m.author}</span>
            {#if m.trust}
              <span class="trust">{trustLabel[m.trust] ?? m.trust}</span>
            {/if}
            {#if m.audience === 'misterx'}
              <span class="to">an die Zielperson</span>
            {:else if m.audience === 'all'}
              <span class="to">an alle</span>
            {/if}
            <span class="at mono">{time(m.at)}</span>
          </div>
          <p class="text">{m.text}</p>
        </article>
      {/each}
    {/if}
  </div>

  {#if readOnly}
    <p class="dim">Nur mitlesen – der Kanal gehört der Fahndung.</p>
  {:else}
    {#if isHQ}
      <div class="controls">
        <select bind:value={trust}>
          <option value="confirmed">🟢 Bestätigt</option>
          <option value="unconfirmed">🟡 Unbestätigt</option>
          <option value="rumor">🔴 Gerücht</option>
        </select>
        <select bind:value={audience}>
          <option value="detectives">an die Fahndung</option>
          <option value="misterx">an die Zielperson</option>
          <option value="all">an alle</option>
        </select>
      </div>
    {/if}

    <div class="compose">
      <input
        bind:value={text}
        placeholder={isHQ ? 'Lagemeldung …' : 'Nachricht an die anderen Teams …'}
        onkeydown={(e) => e.key === 'Enter' && send()}
      />
      <button class="primary" onclick={send} disabled={busy || !text.trim()}>Senden</button>
    </div>
  {/if}

  {#if error}<p class="err">{error}</p>{/if}
</section>

<style>
  .radio {
    background: var(--sheet);
    border: 1px solid var(--rule);
    border-top: 2px solid var(--hq);
    padding: 0.8rem 0.85rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    min-height: 0;
  }

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

  .count {
    font-size: var(--fs-xs);
    color: var(--muted);
  }

  .log {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    overflow-y: auto;
    max-height: 18rem;
    min-height: 4rem;
  }

  .compact .log {
    max-height: 11rem;
  }

  .msg {
    background: var(--sheet-2);
    border-left: 2px solid var(--rule-hi);
    padding: 0.35rem 0.55rem;
  }

  .msg.mine {
    border-left-color: var(--det);
  }

  .msg.system {
    border-left-color: var(--hq);
    background: var(--hq-dim);
  }

  /* Die Vertrauensstufe steuert nur die Randfarbe, nie die Fläche: Sonst
     konkurriert sie mit der Parteifarbe um Aufmerksamkeit. */
  .msg[data-trust='unconfirmed'] { border-left-color: var(--warn); }
  .msg[data-trust='rumor'] { border-left-color: var(--x); }

  .head {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0.2rem 0.5rem;
    font-size: var(--fs-xs);
  }

  .who {
    font-family: var(--display);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-hi);
  }

  .trust,
  .to {
    color: var(--muted);
  }

  .at {
    margin-left: auto;
    color: var(--faint);
  }

  .text {
    font-size: var(--fs-sm);
    color: var(--text);
    line-height: 1.4;
    overflow-wrap: anywhere;
  }

  .controls {
    display: flex;
    gap: 0.35rem;
  }

  .controls select {
    flex: 1;
    min-height: 2.1rem;
    font-size: var(--fs-xs);
  }

  .compose {
    display: flex;
    gap: 0.4rem;
  }

  .compose input {
    flex: 1;
    min-width: 0;
    min-height: 2.4rem;
  }

  .compose button {
    min-height: 2.4rem;
    padding: 0 0.8rem;
  }

  .dim {
    font-size: var(--fs-xs);
    color: var(--muted);
  }

  .err {
    font-size: var(--fs-sm);
    color: var(--x);
    border-left: 2px solid var(--x);
    padding-left: 0.6rem;
  }
</style>
