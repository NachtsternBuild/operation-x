<script>
  /*
   * Die Einweisung.
   *
   * Fünf bis sieben Schritte, die die Oberfläche erklären, und am Ende eine
   * Handlungsprobe: einmal wirklich den Standort melden. Erst danach gilt ein
   * Team als bereit, und die Zentrale sieht in ihrer Liste, wer noch fehlt.
   *
   * Der Grund ist einfach: Ein Team, das die Oberfläche zum ersten Mal auf der
   * Straße sieht, verliert die erste halbe Stunde – und die fehlt dann am Ende.
   */
  import { api, session } from '../lib/session.svelte.js'
  import { tracker, sendNow } from '../lib/tracker.svelte.js'

  let { onDone = null } = $props()

  let step = $state(0)
  let probeDone = $state(false)
  let probeBusy = $state(false)
  let probeError = $state(null)

  const role = $derived(session.team?.role ?? 'detective')

  const scripts = {
    detective: [
      {
        title: 'Ihr seid die Fahndung',
        body: 'Mehrere Teams arbeiten zusammen gegen eine Zielperson. Ihr seht euch gegenseitig immer genau – von der Zielperson seht ihr nur, was ihr euch erarbeitet.',
      },
      {
        title: 'Standort melden, alle zehn Minuten',
        body: 'Ganz oben läuft ein Countdown. Läuft er ab, gibt es beim ersten Mal eine Verwarnung, beim zweiten zehn Minuspunkte, ab dem dritten zusätzlich fünf Minuten Sperre. In Bahn oder Bus schaltet ihr auf Transit – dann sind es dreizehn Minuten.',
      },
      {
        title: 'Rätsel bringen Hinweise',
        body: 'Jedes gelöste Rätsel erzeugt einen Hinweis aus der Lage in genau diesem Moment. Wer schnell löst, bekommt eine heiße Spur; wer lange braucht, eine kalte. Achtet auf die Markierung: Heiß ist frisch, kalt ist Geschichte.',
      },
      {
        title: 'Nicht alles stimmt',
        body: 'Die Zielperson kann falsche Hinweise einspeisen. Sie sehen genauso aus wie echte. Zwei Hinweise, die sich widersprechen, sind also kein Fehler – sondern eine Information.',
      },
      {
        title: 'Einsatzmittel kosten Punkte',
        body: 'Mit Fahndungspunkten könnt ihr orten, Sperrzonen legen, Wanzen auslegen oder einen Sektor scannen. Alles kostet, und eine Ortung kann ins Leere laufen, wenn die Zielperson gegensteuert.',
      },
      {
        title: 'Der Zugriff, drei Stufen',
        body: 'Stufe 1 und 2 kosten bei einem Fehlschlag nichts – nutzt sie, um eine Vermutung zu prüfen. Stufe 3 entscheidet das Spiel oder kostet zehn Minuten Sperre, einen Punkt und fünfzehn Minuspunkte. Überlegt zweimal.',
      },
    ],
    misterx: [
      {
        title: 'Ihr seid die Zielperson',
        body: 'Ihr bewegt euch unerkannt durch die Stadt, arbeitet Zwischenziele ab und wollt am Ende das geheime Fluchtziel erreichen. Die Fahndung erscheint als unscharfe Kreise – rund 200 Meter Unschärfe.',
      },
      {
        title: 'Standort melden, alle zehn Minuten',
        body: 'Auch ihr meldet euch regelmäßig, sonst gibt es Abzüge. Das ist kein Nachteil, sondern der Preis dafür, dass die Fahndung überhaupt eine Chance hat. In Bahn oder Bus schaltet ihr auf Transit.',
      },
      {
        title: 'Drei Wege zu jedem Ziel',
        body: 'An jedem Zwischenziel stehen drei Varianten zur Wahl: unauffällig mit viel Zeit, schnell mit knapper Frist, oder ein Bonusauftrag mit einem zusätzlichen Fluchtpunkt. Das ist eine echte Entscheidung – die schnelle Route führt manchmal direkt an der Fahndung vorbei.',
      },
      {
        title: 'Ankunft nachweisen',
        body: 'Am Ziel gebt ihr den Vor-Ort-Code ein oder ladet ein Foto hoch. Näher als 150 Meter müsst ihr dafür sein. Ein Foto prüft die Zentrale – solange sie prüft, läuft eure Frist nicht weiter.',
      },
      {
        title: 'Fluchtpunkte sind eure Waffe',
        body: 'Falsche Fährte, Nebelkerze, Phantom, U-Bahn-Geist: Damit stört ihr die Fahndung. Die Nebelkerze macht jede Ortung fünfzehn Minuten lang wirkungslos – auch eine, die schon läuft.',
      },
      {
        title: 'Im Finale wird es eng',
        body: 'Habt ihr alle Zwischenziele geschafft, wird der Suchbereich offengelegt und zieht sich alle paar Minuten enger. Täuschungsmanöver wirken dann nicht mehr. Ab da hilft nur noch Tempo.',
      },
    ],
    hq: [
      {
        title: 'Ihr leitet das Spiel',
        body: 'Die Zentrale sieht alles: echte Positionen, jede Buchung, welcher Hinweis gefälscht ist. Und sie kann eingreifen, wenn die Realität dazwischenkommt.',
      },
      {
        title: 'Vorher: Sektoren und Hotspots',
        body: 'Unter „Sektoren“ holt ihr die echten Stadtteilgrenzen und wählt aus, welche mitspielen. Unter „Hotspots“ setzt ihr die Punkte – aus Vorschlägen oder per Klick auf die Karte. Nummern und Vor-Ort-Codes vergibt der Server.',
      },
      {
        title: 'Drucken nicht vergessen',
        body: 'Unter „Druck“ liegen drei Blätter: die Fahndungskarte für die Teams, die Codeliste für euch und die Zettel zum Anbringen. Ohne die Zettel gibt es keine Vor-Ort-Codes.',
      },
      {
        title: 'Während des Spiels',
        body: 'Im Rätselpult schaltet ihr Aufgaben frei und lest Lösungsversuche mit. Unter „Beweise“ prüft ihr eingereichte Fotos – zügig, denn solange ihr nicht entscheidet, wartet jemand am Ziel.',
      },
      {
        title: 'Wenn etwas schiefgeht',
        body: 'Im Regelpult hebt ihr Sperren auf, korrigiert Punkte und tragt Teams nach. Jede Korrektur braucht eine Begründung und steht danach im Protokoll – das erspart Diskussionen.',
      },
      {
        title: 'Übt vorher',
        body: 'Im Regelpult könnt ihr eine Trockenübung starten: simulierte Spieler laufen durch das Spiel, während ihr zuseht. In einer Viertelstunde seht ihr alles, was an einem Spieltag passiert.',
      },
    ],
  }

  // Dieselbe Seite für alle Rollen, und zwar bevor die Erfassung anläuft.
  // Wer Standortdaten von Leuten verarbeitet, sagt ihnen vorher, was erhoben
  // wird und wie lange es bleibt. Steht wortgleich in beiden Apps.
  const datenschutz = {
    title: 'Was dieses Spiel über euch speichert',
    body: 'Euren Standort, solange die Erfassung läuft — im Spiel etwa alle zehn Minuten einer. Dazu Punkte, Buchungen und was ihr in den Funk schreibt. Alles hängt an diesem einen Spiel und liegt auf dem Rechner der Spielleitung, nicht bei uns; wir bekommen nichts davon zu sehen. Nach dem Spiel wird die Bewegungsspur automatisch gelöscht — ab Werk 24 Stunden nach Spielende. Was bleibt, ist der Punktestand ohne Koordinaten. Wer damit nicht einverstanden ist, spielt nicht mit; das ist in Ordnung und hat keine Folgen.',
  }

  const steps = $derived([...(scripts[role] ?? scripts.detective), datenschutz])
  const isLast = $derived(step >= steps.length)

  async function probe() {
    probeBusy = true
    probeError = null

    try {
      const res = await sendNow()
      if (res) {
        probeDone = true
      } else {
        probeError = tracker.error ?? 'Der Standort ließ sich nicht ermitteln.'
      }
    } catch (err) {
      probeError = err.message
    } finally {
      probeBusy = false
    }
  }

  async function finish() {
    try {
      await api.post('/api/opx/onboarded', {})
    } catch {
      /* die Einweisung ist trotzdem gelaufen */
    }
    onDone?.()
  }
</script>

<div class="veil">
  <section class="sheet">
    <header>
      <span class="label">Einweisung</span>
      <span class="pos mono">{Math.min(step + 1, steps.length)} / {steps.length}</span>
    </header>

    {#if !isLast}
      <h2>{steps[step].title}</h2>
      <p class="body">{steps[step].body}</p>

      <div class="dots">
        {#each steps as _, i}
          <span class="dot" class:on={i <= step}></span>
        {/each}
      </div>

      <div class="actions">
        {#if step > 0}
          <button class="ghost" onclick={() => step--}>Zurück</button>
        {/if}
        <button class="primary" onclick={() => step++}>
          {step === steps.length - 1 ? 'Zur Probe' : 'Weiter'}
        </button>
      </div>
    {:else}
      <h2>Handlungsprobe</h2>
      <!--
        Die Einweisung nennt konkrete Zahlen, weil abstrakte Regeln niemand
        behält. Änderbar sind sie trotzdem alle — die Zentrale stellt sie im
        Regelpult. Wer das nicht weiß, hält die Zahl von heute Morgen für die
        Wahrheit des Tages. Derselbe Satz steht in der App.
      -->
      <p class="hint">
        Die genannten Zahlen sind die Voreinstellung. Was in eurem Spiel gilt,
        steht jederzeit oben unter „Regeln“.
      </p>
      {#if role === 'hq'}
        <p class="body">
          Damit seid ihr durch. Startet als Nächstes eine Trockenübung im
          Regelpult – dann kennt ihr jeden Handgriff, bevor es ernst wird.
        </p>
        <div class="actions">
          <button class="primary" onclick={finish}>Fertig</button>
        </div>
      {:else}
        <p class="body">
          Einmal ernsthaft: Meldet jetzt euren Standort. Wenn das klappt,
          funktioniert am Spieltag alles Übrige auch.
        </p>

        {#if probeDone}
          <p class="ok">Standort übermittelt. Ihr seid bereit.</p>
          <div class="actions">
            <button class="primary" onclick={finish}>Los geht's</button>
          </div>
        {:else}
          {#if probeError}
            <p class="err">{probeError}</p>
          {/if}
          <div class="actions">
            <button class="ghost" onclick={() => step--}>Zurück</button>
            <button class="primary" onclick={probe} disabled={probeBusy}>
              {probeBusy ? 'Ermittle …' : 'Standort melden'}
            </button>
          </div>
          <button class="skip" onclick={finish}>Überspringen</button>
        {/if}
      {/if}
    {/if}
  </section>
</div>

<style>
  .hint {
    color: var(--muted);
    font-size: var(--fs-sm);
    border-left: 2px solid var(--rule-hi);
    padding-left: 0.6rem;
  }

  .veil {
    position: fixed;
    inset: 0;
    z-index: 40;
    display: grid;
    place-items: center;
    padding: 1.2rem;
    background: rgba(5, 9, 11, 0.92);
  }

  .sheet {
    width: min(100%, 30rem);
    background: var(--sheet);
    border: 1px solid var(--rule-hi);
    border-top: 3px solid var(--hq);
    padding: 1.4rem 1.5rem;
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
  }

  header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    border-bottom: 1px solid var(--rule);
    padding-bottom: 0.5rem;
  }

  .pos {
    font-size: var(--fs-xs);
    color: var(--muted);
  }

  h2 {
    font-size: var(--fs-xl);
    text-transform: uppercase;
    letter-spacing: 0.03em;
  }

  .body {
    font-size: var(--fs-base);
    line-height: 1.6;
    color: var(--text);
  }

  .dots {
    display: flex;
    gap: 0.3rem;
  }

  .dot {
    height: 3px;
    flex: 1;
    background: var(--rule);
  }

  .dot.on {
    background: var(--hq);
  }

  .actions {
    display: flex;
    gap: 0.5rem;
    margin-top: 0.2rem;
  }

  .actions button {
    flex: 1;
    min-height: 2.7rem;
  }

  .skip {
    background: transparent;
    border: none;
    color: var(--faint);
    font-size: var(--fs-xs);
    min-height: 2rem;
    text-transform: none;
    letter-spacing: 0;
  }

  .ok,
  .err {
    font-size: var(--fs-sm);
    padding-left: 0.7rem;
    border-left: 2px solid;
    line-height: 1.45;
  }

  .ok {
    color: var(--ok);
    border-left-color: var(--ok);
  }

  .err {
    color: var(--x);
    border-left-color: var(--x);
  }
</style>
