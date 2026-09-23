<script>
  /*
   * Hotspots setzen und verwalten.
   *
   * Drei Wege führen zu einem Punkt: aus den Vorschlägen übernehmen, auf die
   * Karte klicken, oder einen bestehenden verschieben. Nummern und Vor-Ort-Codes
   * vergibt der Server – Nummern müssen lückenlos sein, weil sie im
   * Zugriffsformular als Antwort eingetippt werden.
   */
  import { api } from '../lib/session.svelte.js'
  import Map from '../components/Map.svelte'

  let field = $state(null)
  let pois = $state([])
  let pins = $state([])
  let busy = $state(null)
  let error = $state(null)
  let note = $state(null)
  let editing = $state(null)
  let mapRef = $state(null)

  const sectorNames = $derived(
    Object.fromEntries((field?.sectors ?? []).map((s) => [s.id, `${s.code} ${s.name}`])),
  )

  async function load() {
    try {
      field = await api.get('/api/opx/map')
      error = null
    } catch (err) {
      if (err.status !== 404) error = err.message
    }
  }

  $effect(() => {
    if (!field) load()
  })

  async function loadPOIs() {
    busy = 'Suche markante Orte im Spielgebiet …'
    error = null
    try {
      const res = await api.get('/api/opx/setup/pois')
      pois = res.pois ?? []
      if (pois.length === 0) note = 'Keine weiteren Vorschläge gefunden.'
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  function addPin(p) {
    pins = [...pins, { name: p.name ?? '', lat: p.lat, lng: p.lng, kind: p.kind ?? 'other' }]
    if (p.name) pois = pois.filter((x) => x.name !== p.name)
  }

  function removePin(i) {
    pins = pins.filter((_, idx) => idx !== i)
  }

  async function savePins() {
    if (pins.length === 0) return
    busy = `Speichere ${pins.length} Hotspots …`
    error = null

    try {
      const res = await api.post('/api/opx/setup/hotspots', { hotspots: pins })
      note = `${res.saved} Hotspots angelegt.`
      pins = []
      field = null
      await load()
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  async function removeHotspot(h) {
    busy = `Entferne #${String(h.number).padStart(2, '0')} …`
    try {
      await api.del(`/api/opx/setup/hotspots/${h.id}`)
      field = null
      await load()
      note = 'Hotspot entfernt, Nummern nachgezogen.'
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  async function saveEdit() {
    if (!editing) return
    busy = 'Speichere Änderung …'
    try {
      await api.patch(`/api/opx/setup/hotspots/${editing.id}`, {
        name: editing.name,
        passcode: editing.passcode,
        kind: editing.kind,
        notes: editing.task,
      })
      editing = null
      field = null
      await load()
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  const kinds = [
    ['landmark', 'Wahrzeichen'],
    ['transit', 'Verkehr'],
    ['park', 'Park'],
    ['building', 'Gebäude'],
    ['other', 'Sonstiges'],
  ]
</script>

<div class="wrap">
  <div class="col">
    <section class="box">
      <header>
        <h3>Vorschläge</h3>
        <button class="ghost small" onclick={loadPOIs} disabled={!!busy}>Suchen</button>
      </header>
      <p class="hint">
        Markante Orte aus den Kartendaten, nach Bekanntheit sortiert. Bereits gesetzte
        Punkte sind ausgeblendet.
      </p>

      {#if pois.length}
        <ul class="list">
          {#each pois.slice(0, 40) as p (p.name)}
            <li>
              <button class="row-btn" onclick={() => addPin(p)}>
                <span class="nm">{p.name}</span>
                <span class="kd mono">{p.kind}</span>
                <span class="plus">+</span>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </section>

    {#if pins.length}
      <section class="box">
        <header>
          <h3>Vorgemerkt</h3>
          <span class="count mono">{pins.length}</span>
        </header>
        <ul class="list">
          {#each pins as p, i (i)}
            <li class="pin">
              <input
                class="pin-name"
                bind:value={pins[i].name}
                placeholder="Name des Punktes"
              />
              <button class="ghost small" onclick={() => removePin(i)} aria-label="Entfernen">
                ×
              </button>
            </li>
          {/each}
        </ul>
        <button class="primary" onclick={savePins} disabled={!!busy}>
          {pins.length} Hotspots übernehmen
        </button>
      </section>
    {/if}

    <section class="box grow">
      <header>
        <h3>Gesetzt</h3>
        <span class="count mono">{field?.hotspots?.length ?? 0}</span>
      </header>

      {#if field?.hotspots?.length}
        <ul class="list">
          {#each field.hotspots as h (h.id)}
            <li class="spot">
              {#if editing?.id === h.id}
                <div class="edit">
                  <input bind:value={editing.name} placeholder="Name" />
                  <div class="edit-row">
                    <input
                      class="code-input"
                      bind:value={editing.passcode}
                      placeholder="Code vor Ort"
                    />
                    <select bind:value={editing.kind}>
                      {#each kinds as [v, label]}
                        <option value={v}>{label}</option>
                      {/each}
                    </select>
                  </div>
                  <textarea
                    bind:value={editing.task}
                    rows="2"
                    placeholder="Aufgabe für die Zielperson vor Ort"
                  ></textarea>
                  <div class="edit-row">
                    <button class="primary small" onclick={saveEdit}>Sichern</button>
                    <button class="ghost small" onclick={() => (editing = null)}>Abbrechen</button>
                  </div>
                </div>
              {:else}
                <span class="num mono">{String(h.number).padStart(2, '0')}</span>
                <div class="spot-main">
                  <span class="nm">{h.name}</span>
                  <span class="meta mono">
                    {h.passcode ?? '—'} · {sectorNames[h.sector] ?? 'außerhalb'}
                  </span>
                  {#if h.task}<span class="task">{h.task}</span>{/if}
                </div>
                <button
                  class="ghost small"
                  onclick={() =>
                    (editing = {
                      id: h.id,
                      name: h.name,
                      passcode: h.passcode ?? '',
                      kind: h.kind ?? 'other',
                      task: h.task ?? '',
                    })}
                  aria-label="Bearbeiten"
                >
                  ✎
                </button>
                <button class="ghost small" onclick={() => removeHotspot(h)} aria-label="Löschen">
                  ×
                </button>
              {/if}
            </li>
          {/each}
        </ul>
      {:else}
        <p class="hint">
          Noch keine Hotspots. Vorschläge übernehmen oder auf die Karte klicken.
        </p>
      {/if}
    </section>

    {#if busy}<p class="status mono">{busy}</p>{/if}
    {#if error}<p class="status error" role="alert">{error}</p>{/if}
    {#if note && !busy && !error}<p class="status ok mono">{note}</p>{/if}
  </div>

  <div class="map-col">
    <Map
      bind:this={mapRef}
      sectors={field?.sectors ?? []}
      hotspots={field?.hotspots ?? []}
      area={field?.area ?? null}
      {pins}
      onMapClick={(c) => addPin({ ...c, name: '' })}
    />
    <p class="map-hint">Klick auf die Karte setzt einen eigenen Punkt.</p>
  </div>
</div>

<style>
  .wrap {
    display: grid;
    grid-template-columns: minmax(19rem, 25rem) 1fr;
    gap: 1rem;
    flex: 1;
    min-height: 0;
  }

  .col {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    min-height: 0;
    overflow-y: auto;
  }

  .map-col {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    min-height: 24rem;
  }

  .map-col :global(.map-wrap) {
    flex: 1;
  }

  .map-hint {
    font-size: var(--fs-xs);
    color: var(--faint);
  }

  .box {
    background: var(--sheet);
    border: 1px solid var(--rule);
    padding: 0.85rem 0.9rem;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .box.grow {
    flex: 1;
    min-height: 0;
  }

  .box header {
    display: flex;
    align-items: baseline;
    gap: 0.6rem;
    border-bottom: 1px solid var(--rule);
    padding-bottom: 0.5rem;
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

  .hint {
    font-size: var(--fs-xs);
    color: var(--muted);
    line-height: 1.45;
  }

  .list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
    overflow-y: auto;
    max-height: 22rem;
  }

  button.small {
    min-height: 2rem;
    padding: 0 0.55rem;
    font-size: var(--fs-xs);
    flex: none;
  }

  .row-btn {
    width: 100%;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    text-align: left;
    min-height: 2.2rem;
    padding: 0 0.5rem;
    background: var(--sheet-2);
    border: 1px solid transparent;
    font-family: var(--body);
    text-transform: none;
    letter-spacing: 0;
    font-size: var(--fs-sm);
  }

  .row-btn:hover {
    border-color: var(--rule-hi);
    background: var(--sheet-3);
  }

  .nm {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text);
  }

  .kd,
  .meta {
    font-size: var(--fs-xs);
    color: var(--faint);
  }

  /* Was die Zielperson hier tun soll. Sieht nur die Zentrale – die Fahndung
     bekommt diese Antwort aus der Karte nicht mitgeliefert. */
  .task {
    font-size: var(--fs-xs);
    color: var(--muted);
    line-height: 1.35;
  }

  .plus {
    color: var(--ok);
    font-size: 1.1rem;
    line-height: 1;
  }

  .pin,
  .spot {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.3rem 0.4rem;
    background: var(--sheet-2);
    font-size: var(--fs-sm);
  }

  .pin {
    border-left: 2px solid var(--ok);
  }

  .pin-name {
    flex: 1;
    min-height: 2rem;
    font-size: var(--fs-sm);
  }

  .spot {
    border-left: 2px solid var(--hq);
  }

  .num {
    font-size: var(--fs-xs);
    color: var(--hq);
    min-width: 1.5rem;
  }

  .spot-main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    line-height: 1.25;
  }

  .edit {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    padding: 0.2rem 0;
  }

  .edit input,
  .edit select {
    min-height: 2.1rem;
    font-size: var(--fs-sm);
  }

  .edit-row {
    display: flex;
    gap: 0.35rem;
  }

  .code-input {
    text-transform: uppercase;
  }

  .status {
    font-size: var(--fs-sm);
    color: var(--muted);
    border-left: 2px solid var(--rule-hi);
    padding-left: 0.6rem;
  }

  .status.error {
    color: var(--x);
    border-left-color: var(--x);
  }

  .status.ok {
    color: var(--ok);
    border-left-color: var(--ok);
  }

  @media (max-width: 56rem) {
    .wrap {
      grid-template-columns: 1fr;
    }

    .col {
      overflow: visible;
    }
  }
</style>
