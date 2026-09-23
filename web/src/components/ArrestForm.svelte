<script>
  /*
   * Sichtkontakt und Zugriffsformular.
   *
   * Der gefährlichste Knopf im ganzen Spiel sitzt hier: Stufe 3 entscheidet die
   * Partie – oder kostet das Team eine Sperre, einen Fluchtpunkt und fünfzehn
   * Punkte. Deshalb wird sie erst freigegeben, wenn alle drei Angaben stehen,
   * und die Folgen stehen daneben, nicht im Kleingedruckten.
   */
  import { api } from '../lib/session.svelte.js'

  let { hotspots = [], onChanged = null } = $props()

  let sighting = $state(null)
  let sightBusy = $state(false)

  let level = $state(1)
  let claimedHotspot = $state('')
  let claimedTime = $state('')
  let claimedTarget = $state('')
  let result = $state(null)
  let busy = $state(false)
  let open = $state(false)

  async function reportSighting() {
    sightBusy = true
    try {
      sighting = await api.post('/api/opx/sighting', {})
      onChanged?.()
    } catch (err) {
      sighting = { confirmed: false, message: err.message }
    } finally {
      sightBusy = false
    }
  }

  async function submit() {
    busy = true
    result = null

    try {
      const payload = {
        level,
        claimedHotspot: Number(claimedHotspot),
      }
      if (level >= 2 && claimedTime) {
        // Die Eingabe ist eine Uhrzeit von heute.
        const [h, m] = claimedTime.split(':').map(Number)
        const d = new Date()
        d.setHours(h, m, 0, 0)
        payload.claimedTime = d.toISOString()
      }
      if (level >= 3) payload.claimedTarget = Number(claimedTarget)

      result = await api.post('/api/opx/arrest', payload)
      onChanged?.()
    } catch (err) {
      result = { correct: false, message: err.message }
    } finally {
      busy = false
    }
  }

  const ready = $derived(
    claimedHotspot !== '' &&
      (level < 2 || claimedTime !== '') &&
      (level < 3 || claimedTarget !== ''),
  )

  const levelInfo = {
    1: { name: 'Lokalisierung', gain: '+5 Punkte', risk: 'Kein Risiko.' },
    2: { name: 'Rekonstruktion', gain: '+10 Punkte', risk: 'Kein Risiko.' },
    3: {
      name: 'Vollständiger Zugriff',
      gain: 'Sofortiger Gesamtsieg',
      risk: 'Bei einem Fehler: 10 Minuten Sperre, −1 FP, −15 Punkte.',
    },
  }
</script>

<section class="access">
  <button class="sight" onclick={reportSighting} disabled={sightBusy}>
    {sightBusy ? 'Melde …' : 'Sichtkontakt'}
  </button>

  {#if sighting}
    <p class="sight-msg" class:ok={sighting.confirmed}>
      {sighting.message}
      {#if sighting.distanceM != null && !sighting.confirmed}
        <span class="mono">({Math.round(sighting.distanceM)} m)</span>
      {/if}
    </p>
  {/if}

  <button class="toggle" onclick={() => (open = !open)}>
    Zugriffsformular {open ? 'schließen' : 'öffnen'}
  </button>

  {#if open}
    <div class="form">
      <div class="levels">
        {#each [1, 2, 3] as l}
          <button class="lv" class:on={level === l} class:danger={l === 3} onclick={() => (level = l)}>
            Stufe {l}
          </button>
        {/each}
      </div>

      <p class="lv-info" class:danger={level === 3}>
        <b>{levelInfo[level].name}</b> — {levelInfo[level].gain}. {levelInfo[level].risk}
      </p>

      <p class="standort">
        Der Zugriff erfolgt vor Ort: Ihr müsst an dem Punkt stehen, den ihr
        benennt — höchstens 150 Meter entfernt.
      </p>

      <label>
        <span class="label">Punkt-Nummer</span>
        <select bind:value={claimedHotspot}>
          <option value="">– wählen –</option>
          {#each hotspots as h (h.id)}
            <option value={h.number}>
              #{String(h.number).padStart(2, '0')} {h.name}
            </option>
          {/each}
        </select>
      </label>

      {#if level >= 2}
        <label>
          <span class="label">Uhrzeit</span>
          <input type="time" bind:value={claimedTime} />
        </label>
      {/if}

      {#if level >= 3}
        <label>
          <span class="label">Vermutetes Fluchtziel</span>
          <select bind:value={claimedTarget}>
            <option value="">– wählen –</option>
            {#each hotspots as h (h.id)}
              <option value={h.number}>
                #{String(h.number).padStart(2, '0')} {h.name}
              </option>
            {/each}
          </select>
        </label>
      {/if}

      <button
        class:primary={level < 3}
        class:danger={level === 3}
        onclick={submit}
        disabled={busy || !ready}
      >
        {level === 3 ? 'Zugriff — alles oder nichts' : 'Zugriff abschicken'}
      </button>

      {#if result}
        <div class="result" class:ok={result.correct}>
          <p>{result.message}</p>
          {#if result.detail}
            <ul class="detail mono">
              {#each Object.entries(result.detail) as [key, val]}
                <li class:no={!val}>{key}: {val ? 'stimmt' : 'stimmt nicht'}</li>
              {/each}
            </ul>
          {/if}
        </div>
      {/if}
    </div>
  {/if}
</section>

<style>
  .access {
    background: var(--sheet);
    border: 1px solid var(--rule);
    border-top: 2px solid var(--det);
    padding: 0.8rem 0.85rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .sight {
    background: var(--det-dim);
    border-color: var(--det);
    color: var(--det);
    min-height: 2.8rem;
  }

  .sight-msg {
    font-size: var(--fs-xs);
    color: var(--muted);
    border-left: 2px solid var(--rule-hi);
    padding-left: 0.6rem;
    line-height: 1.45;
  }

  .sight-msg.ok {
    color: var(--ok);
    border-left-color: var(--ok);
  }

  .toggle {
    background: transparent;
    border-color: var(--rule);
    color: var(--muted);
    min-height: 2.2rem;
    font-size: var(--fs-xs);
  }

  .form {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    border-top: 1px solid var(--rule);
    padding-top: 0.6rem;
  }

  .levels {
    display: flex;
    gap: 0.3rem;
  }

  .lv {
    flex: 1;
    min-height: 2.2rem;
    font-size: var(--fs-xs);
    background: transparent;
    border-color: var(--rule);
    color: var(--muted);
  }

  .lv.on {
    color: var(--det);
    border-color: var(--det);
    background: var(--det-dim);
  }

  .lv.on.danger {
    color: var(--x);
    border-color: var(--x);
    background: var(--x-dim);
  }

  .standort {
    font-size: var(--fs-xs);
    color: var(--muted);
    line-height: 1.45;
    border-left: 2px solid var(--rule-hi);
    padding-left: 0.6rem;
  }

  .lv-info {
    font-size: var(--fs-xs);
    color: var(--muted);
    line-height: 1.45;
    border-left: 2px solid var(--rule-hi);
    padding-left: 0.6rem;
  }

  .lv-info.danger {
    color: var(--x);
    border-left-color: var(--x);
  }

  label {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  button.danger {
    background: var(--x-dim);
    border-color: var(--x);
    color: var(--x);
  }

  .result {
    font-size: var(--fs-sm);
    color: var(--x);
    border-left: 2px solid var(--x);
    padding-left: 0.6rem;
    line-height: 1.45;
  }

  .result.ok {
    color: var(--ok);
    border-left-color: var(--ok);
  }

  .detail {
    list-style: none;
    margin: 0.3rem 0 0;
    padding: 0;
    font-size: var(--fs-xs);
    color: var(--muted);
  }

  .detail li.no {
    color: var(--x);
  }
</style>
