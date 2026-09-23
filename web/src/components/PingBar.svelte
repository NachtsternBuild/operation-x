<script>
  /*
   * Die Standortleiste.
   *
   * Der wichtigste Bedienteil im Feld: Sie zeigt, wie lange bis zur nächsten
   * Pflichtmeldung bleibt, und bringt sie mit einem Griff auf den Weg. Deshalb
   * sitzt sie immer an derselben Stelle und wird laut, bevor es zu spät ist –
   * nicht erst danach.
   */
  import { tracker, start, stop, sendNow } from '../lib/tracker.svelte.js'
  import { api } from '../lib/session.svelte.js'

  let { live = null, onRefresh = null } = $props()

  let sending = $state(false)
  let now = $state(Date.now())

  // Eigener Sekundentakt: Der Countdown soll laufen, auch wenn zwischen zwei
  // Serverabfragen eine Minute liegt.
  $effect(() => {
    const t = setInterval(() => (now = Date.now()), 1000)
    return () => clearInterval(t)
  })

  const dueAt = $derived(live?.self?.nextDueAt ? Date.parse(live.self.nextDueAt) : null)
  const leftSec = $derived(dueAt ? Math.round((dueAt - now) / 1000) : null)

  const level = $derived.by(() => {
    if (leftSec === null) return 'unbekannt'
    if (leftSec < 0) return 'ueberfaellig'
    if (leftSec < 120) return 'knapp'
    return 'ruhig'
  })

  const lockout = $derived(live?.self?.lockouts?.[0] ?? null)
  const lockLeft = $derived(
    lockout ? Math.max(0, Math.round((Date.parse(lockout.expiresAt) - now) / 1000)) : 0,
  )

  function clock(sec) {
    if (sec === null) return '—:—'
    const s = Math.abs(sec)
    const m = Math.floor(s / 60)
    const r = s % 60
    return `${sec < 0 ? '−' : ''}${m}:${String(r).padStart(2, '0')}`
  }

  async function ping() {
    sending = true
    try {
      await sendNow()
      await onRefresh?.()
    } finally {
      sending = false
    }
  }

  async function toggleTransit() {
    const target = !live?.self?.inTransit
    await api.post('/api/opx/transit', { active: target })
    await onRefresh?.()
  }
</script>

<div class="bar" data-level={level}>
  <button class="ping" onclick={ping} disabled={sending}>
    {sending ? 'Sende …' : 'Standort melden'}
  </button>

  <div class="count">
    <span class="label">
      {#if level === 'ueberfaellig'}Überfällig seit{:else}Nächste Meldung in{/if}
    </span>
    <span class="clock mono">{clock(leftSec)}</span>
  </div>

  <div class="side">
    <button
      class="ghost"
      class:on={live?.self?.inTransit}
      onclick={toggleTransit}
      title="Im Transit steigt die Toleranz von 10 auf 13 Minuten"
    >
      Transit {live?.self?.inTransit ? 'an' : 'aus'}
    </button>

    <button
      class="ghost"
      class:on={tracker.state === 'an'}
      onclick={() => (tracker.state === 'an' ? stop() : start())}
    >
      Erfassung {tracker.state === 'an' ? 'an' : 'aus'}
    </button>
  </div>

  <div class="meta mono">
    {#if tracker.accuracy != null}<span>±{Math.round(tracker.accuracy)} m</span>{/if}
    {#if tracker.buffered > 0}<span class="buffered">{tracker.buffered} gepuffert</span>{/if}
    {#if live?.self?.violations > 0}
      <span class="viol">{live.self.violations} Verstöße</span>
    {/if}
  </div>
</div>

{#if tracker.error}
  <p class="hint error" role="alert">{tracker.error}</p>
{/if}

{#if lockout}
  <p class="hint lock" role="alert">
    <b>Standortsperre</b> — {lockout.reason}. Noch {clock(lockLeft)}.
  </p>
{/if}

<!--
  Die Pause gehört ganz nach oben, und sie muss sagen, wofür.
  "Angehalten" allein ist draußen von einer Störung nicht zu unterscheiden —
  und genau dann fängt jemand an, an seinem Gerät herumzudrücken, statt in
  Ruhe zu essen.
-->
{#if live?.status === 'paused'}
  <p class="hint pause">
    <b>Pause{live.pause?.reason ? ` — ${live.pause.reason}` : ''}</b>
    {#if live.pause?.until}
      · weiter gegen {new Date(live.pause.until).toLocaleTimeString('de-DE', {
        hour: '2-digit',
        minute: '2-digit',
      })}
    {/if}
    · Es läuft keine Frist, und euer Standort wird nicht aufgezeichnet.
  </p>
{/if}

<!--
  Der Startpunkt, solange das Spiel nicht läuft: Bis dahin ist er die einzige
  Auskunft, die jemand braucht.
-->
{#if live?.self?.start && live?.status !== 'running' && live?.status !== 'finished'}
  <p class="hint start">
    <b>Euer Startpunkt: #{String(live.self.start.number).padStart(2, '0')}
      {live.self.start.name}</b>
    · Dort beginnt euer Spiel, sobald die Zentrale startet.
  </p>
{/if}

<style>
  .hint.pause {
    border-left: 3px solid var(--hq);
    background: var(--hq-dim);
  }

  .hint.start {
    border-left: 3px solid var(--ok);
  }

  .bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.6rem 1rem;
    padding: 0.6rem 0.8rem;
    background: var(--sheet);
    border: 1px solid var(--rule);
    border-left: 3px solid var(--rule-hi);
  }

  .bar[data-level='knapp'] {
    border-left-color: var(--warn);
  }

  .bar[data-level='ueberfaellig'] {
    border-left-color: var(--crit);
    background: var(--x-dim);
  }

  .ping {
    background: var(--hq-dim);
    border-color: var(--hq);
    color: var(--hq);
    white-space: nowrap;
  }

  .bar[data-level='ueberfaellig'] .ping {
    background: var(--crit);
    border-color: var(--crit);
    color: #0b1215;
  }

  .count {
    display: flex;
    flex-direction: column;
    line-height: 1.1;
  }

  .clock {
    font-size: 1.45rem;
    color: var(--text-hi);
    font-variant-numeric: tabular-nums;
  }

  .bar[data-level='ueberfaellig'] .clock {
    color: var(--crit);
  }

  .bar[data-level='knapp'] .clock {
    color: var(--warn);
  }

  .side {
    display: flex;
    gap: 0.4rem;
    margin-left: auto;
  }

  .side button {
    min-height: 2.2rem;
    padding: 0 0.7rem;
    font-size: var(--fs-xs);
  }

  .side button.on {
    color: var(--ok);
    border-color: var(--ok);
  }

  .meta {
    display: flex;
    gap: 0.9rem;
    font-size: var(--fs-xs);
    color: var(--muted);
    width: 100%;
  }

  .buffered {
    color: var(--warn);
  }

  .viol {
    color: var(--x);
  }

  .hint {
    font-size: var(--fs-sm);
    padding-left: 0.7rem;
    border-left: 2px solid var(--rule-hi);
    line-height: 1.45;
  }

  .hint.error {
    color: var(--x);
    border-left-color: var(--x);
  }

  .hint.lock {
    color: var(--warn);
    border-left-color: var(--warn);
  }

  @media (max-width: 40rem) {
    .side {
      width: 100%;
      margin-left: 0;
    }

    .side button {
      flex: 1;
    }
  }
</style>
