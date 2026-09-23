<script>
  import { aufsatz } from '../lib/aufsatz.svelte.js'
  import { session, logout, } from '../lib/session.svelte.js'
  import { stream } from '../lib/stream.svelte.js'
  import RuleBook from './RuleBook.svelte'
  import Account from './Account.svelte'
  import { funk } from '../lib/funk.svelte.js'

  let { children } = $props()

  // Der Regelnachschlag gehört in die Kopfleiste, nicht in ein Menü: Die Frage
  // „was kostet das?“ kommt unterwegs, und dann zählt jeder Handgriff.
  let rulesOpen = $state(false)

  // Der Punktestand ist der Knopf zu seiner eigenen Begründung. Ein eigener
  // Menüpunkt wäre die falsche Antwort: Gefragt wird, während man auf die Zahl
  // sieht.
  let accountOpen = $state(false)

  const roleLabels = {
    hq: 'Einsatzzentrale',
    misterx: 'Zielperson',
    detective: 'Fahndungsteam',
  }

  const roleClass = {
    hq: 'hq',
    misterx: 'x',
    detective: 'det',
  }

  let field = $state(document.documentElement.dataset.field === 'on')

  function toggleField() {
    field = !field
    document.documentElement.dataset.field = field ? 'on' : 'off'
    try {
      localStorage.setItem('opx.field', field ? 'on' : 'off')
    } catch {
      /* ohne Speicher gilt die Einstellung nur für diese Sitzung */
    }
  }

  const team = $derived(session.team)

  /*
   * Punkte und Fluchtpunkte kommen aus dem Lagestrom, nicht aus dem
   * Anmeldedatensatz.
   *
   * Der wurde beim Anmelden einmal geholt und danach von niemandem mehr
   * angefasst — die Zahl in der Kopfleiste stand also den ganzen Spieltag auf
   * ihrem Anfangswert. Wer nach zwei Stunden hinsah, las den Stand von vor
   * zwei Stunden, und wer sein Konto öffnete, fand darin eine andere Zahl.
   */
  const punkte = $derived(stream.live?.self?.points ?? team?.points ?? 0)
  const fluchtpunkte = $derived(stream.live?.self?.fp ?? team?.fp ?? 0)
</script>

<header class="bar">
  <div class="ident">
    <span class="tag {roleClass[team?.role] ?? ''}">{roleLabels[team?.role] ?? 'Unbekannt'}</span>
    <span class="callsign">{team?.display || team?.callsign}</span>
  </div>

  <div class="stats">
    {#if team?.role !== 'hq'}
      <button
        class="stat account"
        onclick={() => (accountOpen = true)}
        title="Jede Buchung mit Begründung nachlesen"
      >
        <span class="label">FP</span>
        <span class="value mono">{fluchtpunkte}</span>
      </button>
      <button
        class="stat account"
        onclick={() => (accountOpen = true)}
        title="Jede Buchung mit Begründung nachlesen"
      >
        <span class="label">Punkte</span>
        <span class="value mono">{punkte}</span>
      </button>
    {/if}
    {#if session.game}
      <div class="stat wide">
        <span class="label">Einsatz</span>
        <span class="value mono">{session.game.name}</span>
      </div>
    {/if}
  </div>

  <div class="actions">
    <button
      class="ghost"
      onclick={toggleField}
      aria-pressed={field}
      title="Größere Schrift und mehr Kontrast für draußen"
    >
      Feld {field ? 'an' : 'aus'}
    </button>
    <!--
      Der Zustand der zweiten Verschlüsselung, mit Fingerabdruck.
      Wer wissen will, ob sein Gerät wirklich mit diesem Server spricht und
      nicht mit jemandem dazwischen, vergleicht diese Zeichen mit denen auf dem
      Bildschirm der Spielleitung. Das ist der einzige Weg, der ohne
      Zertifikate auskommt.
    -->
    <span
      class="funk"
      class:on={funk.ready}
      title={funk.ready
        ? `Zusätzlich verschlüsselt. Kennzeichen: ${funk.fingerprint}`
        : funk.reason ?? 'Nur die Verschlüsselung der Verbindung.'}
    >
      {funk.ready ? '🔒' : '🔓'}
      {#if funk.ready}<span class="fp mono">{funk.fingerprint?.slice(0, 9)}</span>{/if}
    </span>

    <button class="ghost" onclick={() => (rulesOpen = true)} title="Preise, Fristen und Strafen nachschlagen">
      Regeln
    </button>
    {#if aufsatz.kopfzeile}
      {@const Kopfzeile = aufsatz.kopfzeile}
      <Kopfzeile />
    {/if}
    <button class="ghost" onclick={logout}>Abmelden</button>
  </div>
</header>

<main>
  {@render children()}
</main>

{#if rulesOpen}
  <RuleBook onClose={() => (rulesOpen = false)} />
{/if}

{#if accountOpen}
  <Account onClose={() => (accountOpen = false)} />
{/if}

<style>
  .funk {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    font-size: var(--fs-sm);
    color: var(--muted);
    cursor: help;
  }

  .funk.on {
    color: var(--ok);
  }

  .funk .fp {
    font-size: var(--fs-xs);
    letter-spacing: 0.02em;
  }

  /* Der Kontoknopf soll aussehen wie die Anzeige, die er ersetzt: Er ist eine
     Zahl, kein Schalter. Nur beim Zeigen darauf gibt er sich zu erkennen. */
  .account {
    background: none;
    border: 1px solid transparent;
    padding: 0.1rem 0.35rem;
    margin: -0.1rem -0.35rem;
    font: inherit;
    color: inherit;
    text-align: left;
    cursor: pointer;
  }

  .account:hover,
  .account:focus-visible {
    border-color: var(--rule-hi);
  }

  .bar {
    position: relative;
    z-index: 2;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.6rem 1.25rem;
    padding: 0.6rem 1rem;
    background: var(--sheet);
    border-bottom: 1px solid var(--rule-hi);
  }

  .ident {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    min-width: 0;
  }

  .callsign {
    font-family: var(--display);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    font-weight: 600;
    font-size: var(--fs-lg);
    color: var(--text-hi);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .stats {
    display: flex;
    align-items: center;
    gap: 1.25rem;
    margin-left: auto;
    min-width: 0;
  }

  .stat {
    display: flex;
    flex-direction: column;
    line-height: 1.2;
    min-width: 0;
  }

  .stat .value {
    font-size: var(--fs-base);
    color: var(--text-hi);
  }

  .stat.wide .value {
    font-size: var(--fs-sm);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 14rem;
  }

  .actions {
    display: flex;
    gap: 0.5rem;
  }

  .actions button {
    min-height: 2.1rem;
    padding: 0 0.7rem;
    font-size: var(--fs-xs);
  }

  main {
    flex: 1;
    position: relative;
    z-index: 1;
    padding: 1rem;
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  /* Auf dem Telefon rücken Kennung und Werte übereinander, damit die
     Bedienknöpfe erreichbar bleiben. */
  @media (max-width: 40rem) {
    .stats {
      order: 3;
      width: 100%;
      margin-left: 0;
      gap: 1rem;
    }

    .stat.wide {
      display: none;
    }

    .actions {
      margin-left: auto;
    }
  }
</style>
