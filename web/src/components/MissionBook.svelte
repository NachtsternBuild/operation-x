<script>
  /*
   * Das Missionsbuch von Mister X.
   *
   * Drei Zustände: Es liegen Varianten zur Wahl, eine Mission läuft, oder es
   * gibt nichts zu tun. Der wichtigste Moment ist der Nachweis am Ziel –
   * deshalb wird der Knopf dafür groß und erst dann bedienbar, wenn das Gerät
   * wirklich nah genug ist.
   */
  import { api } from '../lib/session.svelte.js'
  import { stream } from '../lib/stream.svelte.js'
  import { sendNow } from '../lib/tracker.svelte.js'

  let { onChanged = null } = $props()

  let mission = $state(null)
  let error = $state(null)
  let busy = $state(null)
  let passcode = $state('')
  let now = $state(Date.now())
  let photoInput

  $effect(() => {
    const t = setInterval(() => (now = Date.now()), 1000)
    return () => clearInterval(t)
  })

  async function load() {
    try {
      mission = await api.get('/api/opx/mission')
      error = null
    } catch (err) {
      error = err.message
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

  const deadline = $derived(mission?.deadlineAt ? Date.parse(mission.deadlineAt) : null)
  const leftSec = $derived(deadline ? Math.round((deadline - now) / 1000) : null)

  function clock(sec) {
    if (sec === null) return '—:—'
    const s = Math.max(0, sec)
    return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
  }

  async function choose(optionId) {
    busy = 'Wähle …'
    try {
      mission = await api.post('/api/opx/mission/choose', { optionId })
      error = null
      onChanged?.()
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  async function submitCode() {
    if (!passcode.trim()) return
    busy = 'Prüfe Code …'
    error = null
    try {
      const res = await api.post('/api/opx/mission/evidence', { passcode })
      passcode = ''
      await load()
      onChanged?.()
      if (res.accepted) error = null
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  async function submitPhoto(event) {
    const file = event.target.files?.[0]
    if (!file) return

    busy = 'Lade Foto hoch …'
    error = null
    try {
      const body = new FormData()
      body.append('photo', file)

      await api.upload('/api/opx/mission/evidence', body)
      await load()
      onChanged?.()
    } catch (err) {
      error = err.message
    } finally {
      busy = null
      if (photoInput) photoInput.value = ''
    }
  }

  /*
   * Verzögerung melden.
   *
   * Für den Fall, dass der Auftrag am Ziel selbst Zeit kostet — die Schlange
   * an der Kasse, der Laden, der die Nadel um zweihundert Meter verfehlt.
   * Steht man am Ziel, kommt die Zeit sofort dazu; sonst geht die Meldung an
   * die Zentrale.
   */
  let delayReason = $state('')
  let delayResult = $state(null)
  let delayOpen = $state(false)

  async function reportDelay() {
    busy = 'Melde Verzögerung …'
    error = null
    try {
      delayResult = await api.post('/api/opx/mission/delay', { reason: delayReason.trim() })
      delayReason = ''
      delayOpen = false
      await load()
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  async function abort() {
    busy = 'Klinke aus …'
    error = null
    try {
      await api.post('/api/opx/mission/abort', {})
      await load()
      onChanged?.()
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  /** Standort melden, damit die Abstandsprüfung frische Daten hat. */
  async function refreshPosition() {
    busy = 'Melde Standort …'
    try {
      await sendNow()
      await load()
    } finally {
      busy = null
    }
  }
</script>

<section class="book">
  {#if !mission}
    <p class="dim mono">Missionsbuch wird geladen …</p>
  {:else if mission.status === 'none'}
    <p class="dim">{mission.message}</p>
  {:else if mission.status === 'proposed'}
    <header class="head">
      <span class="label">Zwischenziel {mission.seq} von {mission.planned}</span>
      <h3>Route wählen</h3>
    </header>

    <div class="options">
      {#each mission.options as o (o.id)}
        <button class="option" data-kind={o.kind} onclick={() => choose(o.id)} disabled={!!busy}>
          <span class="o-label">{o.label}</span>
          <span class="o-target">#{String(o.number).padStart(2, '0')} {o.name}</span>
          <span class="o-desc">{o.description}</span>
          {#if o.task}<span class="o-task">Vor Ort: {o.task}</span>{/if}
          <span class="o-meta mono">
            {Math.round(o.distanceM)} m · {o.timeLimitMin} min · {o.rewardPoints} Pkt{o.rewardFp
              ? ` + ${o.rewardFp} FP`
              : ''}
          </span>
        </button>
      {/each}
    </div>
  {:else if mission.status === 'active'}
    <header class="head">
      <span class="label">Zwischenziel {mission.seq} von {mission.planned}</span>
      <h3>#{String(mission.target?.number ?? 0).padStart(2, '0')} {mission.target?.name}</h3>
    </header>

    {#if mission.target?.task}
      <p class="task">{mission.target.task}</p>
    {/if}

    <div class="status-row" data-urgent={leftSec !== null && leftSec < 300}>
      <div class="stat">
        <span class="label">Rest</span>
        <span class="big mono">{clock(leftSec)}</span>
      </div>
      <div class="stat">
        <span class="label">Abstand</span>
        <span class="big mono">{Math.round(mission.distanceM)} m</span>
      </div>
      <div class="stat">
        <span class="label">Nachweis ab</span>
        <span class="big mono">{Math.round(mission.rangeM)} m</span>
      </div>
    </div>

    {#if mission.evidence?.status === 'pending'}
      <p class="pending">Foto eingereicht. Die Einsatzzentrale prüft es.</p>
    {:else if mission.evidence?.status === 'rejected'}
      <p class="rejected">
        Nachweis abgelehnt{mission.evidence.note ? `: ${mission.evidence.note}` : ''}. Noch einmal versuchen.
      </p>
    {/if}

    {#if mission.inRange}
      <div class="proof">
        <div class="code-row">
          <input
            bind:value={passcode}
            placeholder="Code vor Ort"
            class="code"
            onkeydown={(e) => e.key === 'Enter' && submitCode()}
          />
          <button class="primary" onclick={submitCode} disabled={!!busy || !passcode.trim()}>
            Bestätigen
          </button>
        </div>

        <label class="photo">
          <input
            bind:this={photoInput}
            type="file"
            accept="image/*"
            capture="environment"
            onchange={submitPhoto}
          />
          <span>Stattdessen Foto aufnehmen</span>
        </label>
      </div>
    {:else}
      <div class="far">
        <p class="dim">
          Noch {Math.max(0, Math.round(mission.distanceM - mission.rangeM))} m. Der Nachweis
          lässt sich erst am Ziel führen.
        </p>
        <button class="ghost" onclick={refreshPosition} disabled={!!busy}>Standort auffrischen</button>
      </div>
    {/if}

    <!-- Kulanzzeit: keine Strafe, kein Vorteil — eine Korrektur an der Messung. -->
    {#if delayResult}
      <p class="delay-msg" class:ok={delayResult.grantedMin > 0}>
        {delayResult.message}
      </p>
    {/if}

    {#if delayOpen}
      <div class="delay-box">
        <input
          bind:value={delayReason}
          placeholder="Was hält euch auf? z. B. Schlange an der Kasse"
          onkeydown={(e) => e.key === 'Enter' && reportDelay()}
        />
        <div class="delay-row">
          <button class="ghost small" onclick={() => (delayOpen = false)}>Abbrechen</button>
          <button class="primary small" onclick={reportDelay} disabled={!!busy}>
            Melden
          </button>
        </div>
      </div>
    {:else}
      <button
        class="ghost"
        onclick={() => (delayOpen = true)}
        disabled={!!busy}
        title="Wenn der Auftrag am Ziel länger dauert, als die Frist hergibt"
      >
        Ich brauche länger{mission.grace && mission.grace.leftMin > 0
          ? ` (noch ${mission.grace.leftMin} Min. Kulanz)`
          : ' — Zentrale fragen'}
      </button>
    {/if}

    <button class="ghost abort" onclick={abort} disabled={!!busy}>
      Ausklinken und neue Route (2 FP)
    </button>
  {/if}

  {#if busy}<p class="dim mono">{busy}</p>{/if}
  {#if error}<p class="err" role="alert">{error}</p>{/if}
</section>

<style>
  .book {
    background: var(--sheet);
    border: 1px solid var(--rule);
    border-top: 2px solid var(--x);
    padding: 0.85rem 0.9rem;
    display: flex;
    flex-direction: column;
    gap: 0.7rem;
  }

  .head {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    border-bottom: 1px solid var(--rule);
    padding-bottom: 0.5rem;
  }

  h3 {
    font-size: var(--fs-lg);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .options {
    display: flex;
    flex-direction: column;
    gap: 0.45rem;
  }

  .option {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    align-items: flex-start;
    text-align: left;
    padding: 0.6rem 0.7rem;
    min-height: auto;
    background: var(--sheet-2);
    border-left: 3px solid var(--rule-hi);
    font-family: var(--body);
    text-transform: none;
    letter-spacing: 0;
  }

  .option[data-kind='safe'] { border-left-color: var(--ok); }
  .option[data-kind='fast'] { border-left-color: var(--warn); }
  .option[data-kind='bonus'] { border-left-color: var(--hq); }

  /* Die Aufgabe, die die Spielleitung an diesen Ort geschrieben hat. Ohne sie
     ist ein Zwischenziel nur eine Koordinate. */
  .task {
    margin: 0;
    padding: 0.5rem 0.7rem;
    background: var(--sheet-2);
    border-left: 2px solid var(--x);
    font-size: var(--fs-sm);
    line-height: 1.45;
  }

  .o-task {
    font-size: var(--fs-xs);
    color: var(--muted);
    line-height: 1.35;
  }

  .o-label {
    font-family: var(--display);
    text-transform: uppercase;
    letter-spacing: 0.08em;
    font-size: var(--fs-sm);
    color: var(--text-hi);
  }

  .o-target {
    font-size: var(--fs-sm);
    color: var(--x);
  }

  .o-desc {
    font-size: var(--fs-xs);
    color: var(--muted);
    line-height: 1.4;
  }

  .o-meta {
    font-size: var(--fs-xs);
    color: var(--faint);
  }

  .status-row {
    display: flex;
    gap: 1.4rem;
    padding: 0.5rem 0.6rem;
    background: var(--sheet-2);
    border-left: 3px solid var(--rule-hi);
  }

  .status-row[data-urgent='true'] {
    border-left-color: var(--crit);
    background: var(--x-dim);
  }

  .stat {
    display: flex;
    flex-direction: column;
    line-height: 1.1;
  }

  .big {
    font-size: 1.25rem;
    color: var(--text-hi);
  }

  .status-row[data-urgent='true'] .big {
    color: var(--crit);
  }

  .proof {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .code-row {
    display: flex;
    gap: 0.4rem;
  }

  .code {
    flex: 1;
    min-width: 0;
    text-transform: uppercase;
    letter-spacing: 0.1em;
  }

  .photo {
    display: block;
    cursor: pointer;
  }

  .photo input {
    display: none;
  }

  .photo span {
    display: block;
    text-align: center;
    padding: 0.55rem;
    border: 1px dashed var(--rule-hi);
    color: var(--muted);
    font-size: var(--fs-sm);
  }

  .photo:hover span {
    border-color: var(--muted);
    color: var(--text);
  }

  .far {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .delay-box {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    background: var(--sheet-2);
    border-left: 2px solid var(--hq);
    padding: 0.6rem 0.7rem;
  }

  .delay-row {
    display: flex;
    gap: 0.4rem;
  }

  .delay-row button {
    flex: 1;
  }

  .delay-msg {
    font-size: var(--fs-sm);
    line-height: 1.45;
    color: var(--muted);
    border-left: 2px solid var(--rule-hi);
    padding-left: 0.6rem;
  }

  .delay-msg.ok {
    color: var(--ok);
    border-left-color: var(--ok);
  }

  button.small {
    min-height: 2rem;
    padding: 0 0.6rem;
    font-size: var(--fs-xs);
  }

  .abort {
    font-size: var(--fs-xs);
    min-height: 2.1rem;
    color: var(--faint);
  }

  .dim {
    font-size: var(--fs-xs);
    color: var(--muted);
    line-height: 1.5;
  }

  .err,
  .pending,
  .rejected {
    font-size: var(--fs-sm);
    padding-left: 0.6rem;
    border-left: 2px solid;
    line-height: 1.45;
  }

  .err { color: var(--x); border-left-color: var(--x); }
  .pending { color: var(--warn); border-left-color: var(--warn); }
  .rejected { color: var(--x); border-left-color: var(--x); }
</style>
