<script>
  import {
    login,
    loginHost,
    session,
    serverStatus,
    joinInfo,
    setupServer,
    setupInstance,
    createHostAccount,
  } from '../lib/session.svelte.js'
  import { aufsatz } from '../lib/aufsatz.svelte.js'

  let callsign = $state('')
  let password = $state('')
  let status = $state(null)
  let statusError = $state(false)
  let app = $state(null)

  // Ersteinrichtung: Ein frisch entpackter Server hat noch keinen Zugang. Ohne
  // diesen Weg stünde man vor einem Anmeldefeld, in das niemand etwas
  // eintragen könnte.
  let setupName = $state('Operation X')
  let setupCity = $state('Dresden')
  let setupHours = $state(4)
  let setupPassword = $state('')
  let setupBusy = $state(false)
  let setupError = $state(null)
  let setupDone = $state(null)

  const needsSetup = $derived(status?.setupNeeded === true)

  // Ein aufbauendes Programm kann hier einen eigenen Weg anbieten – eine
  // zweite Art sich anzumelden, oder eine zweite Art einzurichten. Solange es
  // nichts übernimmt, steht hier wie bisher die Anmeldung dieses Spiels.
  let zusatzUebernimmt = $state(false)


  // Auf einem Android-Gerät ist die App die bessere Wahl: Sie darf im
  // Hintergrund orten, der Browser nicht. Deshalb steht der Hinweis genau hier
  // — auf der Seite, auf der ein gescannter QR-Code landet.
  const onAndroid = /android/i.test(navigator.userAgent)

  // Zustand des Servers holen, damit auf der Anmeldeseite steht, wofür man
  // sich überhaupt anmeldet.
  $effect(() => {
    serverStatus()
      .then((s) => (status = s))
      .catch(() => (statusError = true))

    if (onAndroid) {
      joinInfo()
        .then((j) => (app = j.hasApp ? j : null))
        .catch(() => {
          /* ohne App-Hinweis geht es im Browser genauso weiter */
        })
    }
  })

  /** Sprechbares Kennwort – es wird am Spieltag vorgelesen und abgetippt. */
  function suggestPassword() {
    const words = ['anker', 'biber', 'distel', 'falke', 'hirsch', 'kranich', 'linde', 'otter', 'rabe', 'zeder']
    const pick = () => words[Math.floor(Math.random() * words.length)]
    setupPassword = `${pick()}-${pick()}-${10 + Math.floor(Math.random() * 89)}`
  }

  async function setup(event) {
    event.preventDefault()
    if (setupPassword.length < 8) return

    setupBusy = true
    setupError = null
    try {
      const res = await setupServer({
        name: setupName.trim(),
        city: setupCity.trim(),
        password: setupPassword,
        durationMin: Math.round(Number(setupHours) * 60),
      })
      setupDone = { callsign: res.callsign, password: setupPassword }
      callsign = res.callsign
      status = await serverStatus()
    } catch (err) {
      setupError = err.message
    } finally {
      setupBusy = false
    }
  }

  async function submit(event) {
    event.preventDefault()
    if (!callsign || !password) return

    try {
      await login(callsign.trim(), password)
    } catch {
      // Die Meldung steht bereits in session.error.
    }
  }

</script>

<div class="screen">
  {#if zusatzUebernimmt}
    <!-- Der Aufsatz zeigt gerade seinen eigenen Weg. -->
  {:else if needsSetup}
    <form class="card" onsubmit={setup}>
      <div class="head">
        <span class="stamp">Ersteinrichtung</span>
        <h1>Operation X</h1>
        <p class="mission mono">Dieser Server ist noch leer.</p>
      </div>

      <p class="hint">
        Einmalig: Name und Stadt des Spiels festlegen und ein Kennwort für die
        Einsatzzentrale wählen. Alles Weitere — Sektoren, Hotspots, Rätsel,
        Teams — richtet ihr danach in der Oberfläche ein.
      </p>

      <div class="field">
        <label class="label" for="sname">Name des Spiels</label>
        <input id="sname" bind:value={setupName} placeholder="Operation X" />
      </div>

      <div class="field">
        <label class="label" for="scity">Stadt</label>
        <input id="scity" bind:value={setupCity} placeholder="Dresden" />
      </div>

      <div class="field">
        <label class="label" for="sdauer">Spieldauer in Stunden</label>
        <input id="sdauer" type="number" step="0.5" min="0.5" max="24" bind:value={setupHours} />
        <span class="sub">
          Danach endet das Spiel, und es gewinnt, wer vorne liegt. Die Uhr
          beginnt erst, wenn ihr startet — nicht jetzt.
        </span>
      </div>

      <div class="field">
        <label class="label" for="spass">Kennwort der Einsatzzentrale</label>
        <div class="row">
          <input id="spass" bind:value={setupPassword} placeholder="mind. 8 Zeichen" />
          <button type="button" class="ghost small" onclick={suggestPassword}>Vorschlag</button>
        </div>
      </div>

      {#if setupError}
        <p class="error mono" role="alert">{setupError}</p>
      {/if}

      <button class="primary" type="submit" disabled={setupBusy || setupPassword.length < 8}>
        {setupBusy ? 'Richte ein …' : 'Einrichten'}
      </button>

      <p class="hint">
        Notiert das Kennwort. Es lässt sich später nicht mehr auslesen, nur neu
        vergeben.
      </p>

    </form>
  {:else}
  <form class="card" onsubmit={submit}>
    <div class="head">
      <span class="stamp">Zugang beschränkt</span>
      <h1>Operation X</h1>
      {#if status?.ready}
        <p class="mission mono">
          {status.gameName} · {status.city}
        </p>
      {:else if statusError}
        <p class="mission mono offline">Kein Kontakt zum Einsatzserver</p>
      {:else}
        <p class="mission mono">Verbindung wird aufgebaut …</p>
      {/if}
    </div>

    <div class="field">
      <label class="label" for="callsign">Rufzeichen</label>
      <input
        id="callsign"
        bind:value={callsign}
        autocomplete="username"
        autocapitalize="none"
        autocorrect="off"
        spellcheck="false"
        placeholder="Team_Alpha"
      />
    </div>

    <div class="field">
      <label class="label" for="password">Kennwort</label>
      <input
        id="password"
        type="password"
        bind:value={password}
        autocomplete="current-password"
        placeholder="••••••••"
      />
    </div>

    {#if session.error}
      <p class="error mono" role="alert">{session.error}</p>
    {/if}

    <button class="primary" type="submit" disabled={session.loading || !callsign || !password}>
      {session.loading ? 'Prüfe Zugang …' : 'Anmelden'}
    </button>

    <p class="hint">
      Zugangsdaten stehen auf der Teamkarte. Sie verfallen nach Spielende
      automatisch.
    </p>

    {#if app}
      <a class="app" href="/operation-x.apk" download>
        <span class="app-title">Android-App installieren</span>
        <span class="app-sub">
          Empfohlen: Sie meldet den Standort auch mit gesperrtem Bildschirm
          weiter. Im Browser tut sie das nicht.
        </span>
      </a>
    {/if}

    {#if setupDone}
      <p class="done">
        Eingerichtet. Meldet euch als <span class="mono">{setupDone.callsign}</span>
        mit dem eben gewählten Kennwort an.
      </p>
    {/if}
  </form>
  {/if}

  {#if aufsatz.anmeldung}
    {@const Zusatz = aufsatz.anmeldung}
    <Zusatz
      {status}
      {needsSetup}
      bind:uebernimmt={zusatzUebernimmt}
      aufFrisch={async () => (status = await serverStatus())}
    />
  {/if}
</div>

<style>
  .screen {
    flex: 1;
    display: grid;
    place-items: center;
    padding: 1.5rem;
    position: relative;
    z-index: 1;
  }

  .card {
    width: min(100%, 24rem);
    display: flex;
    flex-direction: column;
    gap: 1rem;
    background: var(--sheet);
    border: 1px solid var(--rule);
    padding: 1.75rem 1.5rem;
  }

  .head {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    border-bottom: 1px solid var(--rule);
    padding-bottom: 1rem;
  }

  /* Ein Weg zur anderen Anmeldung – kein zweiter Hauptknopf. Wer hier landet,
     hat sich schon entschieden; die Ausnahme soll danebenstehen, nicht
     dazwischen. */
  .link {
    background: none;
    border: none;
    padding: 0;
    min-height: 0;
    font-size: var(--fs-xs);
    color: var(--muted);
    text-decoration: underline;
    text-underline-offset: 3px;
    cursor: pointer;
    align-self: flex-start;
  }

  .link:hover {
    color: var(--hq);
  }

  .stamp {
    align-self: flex-start;
    font-family: var(--display);
    text-transform: uppercase;
    letter-spacing: 0.18em;
    font-weight: 700;
    font-size: var(--fs-xs);
    color: var(--hq);
    border: 1px solid var(--hq);
    padding: 0.2rem 0.55rem 0.15rem;
  }

  h1 {
    font-size: var(--fs-2xl);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .mission {
    font-size: var(--fs-sm);
    color: var(--muted);
  }

  .mission.offline {
    color: var(--x);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .sub {
    font-size: var(--fs-xs);
    color: var(--faint);
    line-height: 1.45;
  }

  .error {
    font-size: var(--fs-sm);
    color: var(--x);
    border-left: 2px solid var(--x);
    padding-left: 0.6rem;
  }

  .hint {
    font-size: var(--fs-xs);
    color: var(--faint);
    line-height: 1.45;
  }

  .done {
    font-size: var(--fs-sm);
    color: var(--ok);
    border-left: 2px solid var(--ok);
    padding-left: 0.6rem;
    line-height: 1.45;
  }

  .row {
    display: flex;
    gap: 0.4rem;
  }

  .row input {
    min-width: 0;
  }

  button.small {
    min-height: 2rem;
    padding: 0 0.6rem;
    font-size: var(--fs-xs);
    flex: none;
  }

  .app {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    border: 1px solid var(--rule-hi);
    border-left: 2px solid var(--hq);
    padding: 0.6rem 0.7rem;
    text-decoration: none;
  }

  .app-title {
    font-family: var(--display);
    text-transform: uppercase;
    letter-spacing: 0.08em;
    font-size: var(--fs-sm);
    color: var(--hq);
  }

  .app-sub {
    font-size: var(--fs-xs);
    color: var(--muted);
    line-height: 1.45;
  }
</style>
