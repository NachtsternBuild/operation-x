<script>
  import { session, refresh } from './lib/session.svelte.js'
  import { startStream, stopStream } from './lib/stream.svelte.js'
  import Shell from './components/Shell.svelte'
  import Login from './routes/Login.svelte'
  import HQ from './routes/HQ.svelte'
  import MisterX from './routes/MisterX.svelte'
  import Detective from './routes/Detective.svelte'
  import Onboarding from './components/Onboarding.svelte'
  import { aufsatz } from './lib/aufsatz.svelte.js'

  // Feldmodus aus der letzten Sitzung wiederherstellen, bevor etwas gezeichnet wird.
  try {
    document.documentElement.dataset.field = localStorage.getItem('opx.field') ?? 'off'
  } catch {
    document.documentElement.dataset.field = 'off'
  }

  /*
   * Eine gespeicherte Anmeldung kann veraltet sein – etwa wenn das HQ das Team
   * entfernt hat. Einmal gegen den Server prüfen.
   *
   * Abhängig allein vom Zugangsschlüssel, nicht vom Team: refresh() schreibt
   * session.team, und wer das im selben Effekt liest, löst ihn damit erneut
   * aus. Das war eine Endlosschleife, die den Server mit Anfragen an /me
   * überzog — hundert je Sekunde, unbemerkt, weil jede einzelne beantwortet
   * wurde.
   */
  $effect(() => {
    if (!session.token || session.kind !== 'team') return
    refresh().catch(() => {
      /* refresh meldet bei 401 selbst ab */
    })
  })

  // Eine Verbindung für die ganze Anwendung, solange jemand angemeldet ist.
  // Ebenfalls nur am Zugangsschlüssel aufgehängt, aus demselben Grund.
  $effect(() => {
    // Der Lagestrom gehört einem Team. Ein Zugang, der keines ist, hat
    // nichts zu empfangen. Gelesen wird dafür session.kind und nicht
    // session.team: Letzteres schreibt refresh(), und das wäre wieder die
    // Schleife von oben.
    if (!session.token || session.kind !== 'team') return
    startStream()
    return () => stopStream()
  })

  const views = {
    hq: HQ,
    misterx: MisterX,
    detective: Detective,
  }

  const View = $derived(views[session.team?.role])

  // Die Einweisung erscheint einmal je Zugang. Wer sie übersprungen hat,
  // bekommt sie nicht erneut – am Spieltag wäre das nur im Weg.
  let showOnboarding = $state(false)
  $effect(() => {
    if (session.team?.onboarded === false) showOnboarding = true
  })
</script>

<div class="grid-bg" aria-hidden="true"></div>

{#if !session.token}
  <Login />
{:else if session.kind !== 'team' && aufsatz.hauptansicht}
  {@const Hauptansicht = aufsatz.hauptansicht}
  <Hauptansicht />
{:else if !session.team}
  <Login />
{:else if View}
  <Shell>
    <View />
  </Shell>
  {#if showOnboarding}
    <Onboarding onDone={() => { showOnboarding = false; refresh().catch(() => {}) }} />
  {/if}
{:else}
  <Shell>
    <p class="unknown">
      Diesem Zugang ist keine gültige Rolle zugeordnet. Das HQ muss die
      Teamdaten prüfen.
    </p>
  </Shell>
{/if}

<style>
  .unknown {
    max-width: 42ch;
    color: var(--x);
    border-left: 2px solid var(--x);
    padding-left: 0.8rem;
  }
</style>
