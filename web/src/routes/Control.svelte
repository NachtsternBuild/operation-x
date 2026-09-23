<script>
  /*
   * Das Regelpult.
   *
   * Alles, was die Spielleitung braucht, wenn die Realität dazwischenkommt:
   * Sperren aufheben, Punkte korrigieren, ein Team nachträglich anlegen. Jeder
   * Eingriff landet mit Begründung im Protokoll – am Spieltag klärt das die
   * Frage „wieso hat das Team plötzlich mehr Punkte?“, bevor sie aufkommt.
   */
  import { api, session, refresh } from '../lib/session.svelte.js'
  import { stream } from '../lib/stream.svelte.js'
  import Radio from '../components/Radio.svelte'

  let penalties = $state([])
  let teams = $state([])
  let readiness = $state([])
  let training = $state({ running: false, bots: [] })
  let error = $state(null)
  let note = $state(null)
  let busy = $state(null)

  // Korrektur
  let adjTeam = $state('')
  let adjPoints = $state(0)
  let adjFP = $state(0)
  let adjReason = $state('')

  // Missionsfrist
  let extendMin = $state(10)
  let extendReason = $state('')
  let delays = $state([])

  // Sicherung
  let backups = $state([])

  // Karte
  let kacheln = $state({ count: 0, bytes: 0, local: false })

  // Startpunkte
  let starts = $state([])

  // Neues Team
  let newCallsign = $state('')
  let newDisplay = $state('')
  let newPassword = $state('')
  let newRole = $state('detective')

  /*
   * Spieldauer.
   *
   * Der häufigste Satz eines Spieltags ist "wir hängen noch eine Stunde
   * dran". Läuft das Spiel schon, wandert das Spielende um die Differenz —
   * der Server rechnet es nicht neu, damit eine Pausenverschiebung nicht
   * stillschweigend verschwindet.
   */
  let dauerStunden = $state((session.game?.durationMin ?? 240) / 60)
  let dauerNote = $state(null)

  async function dauerSpeichern() {
    const minuten = Math.round(Number(dauerStunden) * 60)
    if (!(minuten >= 30 && minuten <= 1440)) return

    busy = 'dauer'
    error = null
    try {
      const res = await api.post('/api/opx/hq/duration', { minutes: minuten })
      dauerNote = res.endsAt
        ? `Spielende jetzt ${new Date(res.endsAt).toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit' })} Uhr.`
        : 'Gemerkt. Die Uhr beginnt beim Start.'
      await refresh()
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  /*
   * Datenschutz zum Anfassen.
   *
   * Die Bewegungsspur löscht sich von selbst — aber zwei Fälle warten nicht
   * auf eine Frist: Jemand zieht seine Einwilligung zurück, oder die
   * Spielleitung will nach dem Ausklang nicht darauf hoffen, dass der Laptop
   * morgen noch einmal läuft. Für beides gibt es hier einen Knopf, damit
   * niemand dafür in die Datenbankverwaltung muss.
   */
  let spurStunden = $state(24)
  let spurNote = $state(null)

  async function spurfristSpeichern() {
    const stunden = Math.max(0, Math.min(168, Math.round(Number(spurStunden))))
    busy = 'spur'
    error = null
    try {
      await api.post('/api/opx/hq/aufbewahrung', { hours: stunden })
      spurNote =
        stunden === 0
          ? 'Die Standortdaten fallen, sobald das Spiel beendet ist.'
          : `Die Standortdaten fallen ${stunden} Stunden nach Spielende.`
      await refresh()
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  async function spurJetztLoeschen() {
    if (
      !confirm(
        'Alle Standortdaten dieses Spiels jetzt löschen?\n\n' +
          'Positionen, Meldefristen, Sichtkontakte, Beweisfotos und die Orte ' +
          'von Sperren verschwinden. Punkte, Buchungen und der Ausgang ' +
          'bleiben. Das lässt sich nicht rückgängig machen.',
      )
    )
      return

    busy = 'spur'
    error = null
    try {
      await api.post('/api/opx/hq/spur-loeschen', {})
      spurNote = 'Gelöscht. In der Datenbank steht kein Ort mehr.'
      await load()
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  async function teamLoeschen(team) {
    const name = team.display || team.callsign
    if (
      !confirm(
        `${name} vollständig aus dem Spiel entfernen?\n\n` +
          'Mit dem Zugang verschwinden alle Positionen, Meldungen, Buchungen ' +
          'und Funksprüche dieses Teams. Das ist der Weg für einen Widerruf ' +
          'der Einwilligung — und er lässt sich nicht rückgängig machen.',
      )
    )
      return

    busy = 'team'
    error = null
    try {
      await api.del(`/api/opx/hq/teams/${team.id ?? team.team}`)
      await load()
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  async function load() {
    try {
      const [p, live, r, t] = await Promise.all([
        api.get('/api/opx/hq/penalties'),
        api.get('/api/opx/live'),
        api.get('/api/opx/hq/readiness'),
        api.get('/api/opx/hq/training'),
      ])
      penalties = p.penalties ?? []
      // Verzögerungsmeldungen der Zielperson aus dem Protokoll ziehen: Sie
      // sind der Grund, warum die Zentrale hier überhaupt hinsieht.
      try {
        const ev = await api.get('/api/opx/hq/events')
        delays = (ev.events ?? [])
          .filter((e) => e.type === 'mission.delay' || e.type === 'mission.grace')
          .slice(0, 6)
      } catch {
        /* das Protokoll ist hier nur Beiwerk */
      }
      teams = live.positions ?? []
      readiness = r.teams ?? []
      training = t
      try {
        backups = (await api.get('/api/opx/hq/backups')).backups ?? []
        kacheln = await api.get('/api/opx/hq/tiles')
        starts = (await api.get('/api/opx/hq/starts')).starts ?? []
      } catch {
        /* ohne Liste geht das Sichern trotzdem */
      }
      error = null
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

  async function lift(p) {
    busy = p.id
    try {
      await api.post(`/api/opx/hq/penalties/${p.id}/lift`, {})
      note = `Sperre für ${p.callsign} aufgehoben.`
      await load()
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  async function adjust() {
    if (!adjTeam || !adjReason.trim() || (!adjPoints && !adjFP)) return
    busy = 'adjust'
    try {
      const res = await api.post('/api/opx/hq/adjust', {
        teamId: adjTeam,
        points: Number(adjPoints),
        fp: Number(adjFP),
        reason: adjReason,
      })
      note = `Korrigiert. Neuer Stand: ${res.points} Punkte, ${res.fp} FP.`
      adjPoints = 0
      adjFP = 0
      adjReason = ''
      await load()
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  async function createTeam() {
    if (!newCallsign.trim() || newPassword.length < 8) return
    busy = 'team'
    try {
      const res = await api.post('/api/opx/hq/teams', {
        callsign: newCallsign.trim(),
        display: newDisplay.trim() || newCallsign.trim(),
        password: newPassword,
        role: newRole,
      })
      note = `Zugang ${res.callsign} angelegt. Kennwort: ${newPassword}`
      newCallsign = ''
      newDisplay = ''
      newPassword = ''
      newRole = 'detective'
      await load()
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  /** Sprechbares Kennwort, das sich am Telefon durchgeben lässt. */
  function suggestPassword() {
    const words = ['anker', 'biber', 'distel', 'falke', 'hirsch', 'kranich', 'linde', 'otter', 'rabe', 'zeder']
    const pick = () => words[Math.floor(Math.random() * words.length)]
    newPassword = `${pick()}-${pick()}-${10 + Math.floor(Math.random() * 89)}`
  }

  async function extendMission() {
    busy = 'extend'
    try {
      const res = await api.post('/api/opx/hq/mission/extend', {
        minutes: Number(extendMin),
        reason: extendReason.trim(),
      })
      note = `Frist um ${res.grantedMin} Minuten verlängert.`
      extendReason = ''
      await load()
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  async function training_(action) {
    busy = 'training'
    try {
      const res = await api.post(`/api/opx/hq/training/${action}`, { detectives: 2 })
      if (action === 'cleanup') note = `${res.removed} Übungsteams entfernt.`
      await load()
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  const roleLabel = { hq: 'Zentrale', misterx: 'Zielperson', detective: 'Fahndung' }

  /**
   * Sichern und herunterladen.
   *
   * Beides in einem Schritt, weil beides zusammengehört: Eine Sicherung, die
   * auf demselben Rechner liegen bleibt, hilft genau dann nicht, wenn man sie
   * braucht.
   */
  async function makeBackup() {
    busy = 'backup'
    note = null
    error = null
    try {
      const res = await api.download('/api/opx/hq/backup', 'operation-x.zip')
      note = `Gesichert: ${res.name} (${mb(res.size)}). Die Datei liegt in „Downloads“.`
      try {
        backups = (await api.get('/api/opx/hq/backups')).backups ?? []
      } catch {
        /* die Liste ist hier nur Beiwerk */
      }
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  /**
   * Startpunkte ziehen.
   *
   * Jeder Punkt nur einmal: Starten Zielperson und ein Fahndungsteam am selben
   * Ort, ist das Spiel in der ersten Minute entschieden, ohne dass jemand
   * etwas richtig oder falsch gemacht hätte.
   */
  async function drawStarts() {
    busy = 'starts'
    note = null
    error = null
    try {
      starts = (await api.post('/api/opx/hq/starts', {})).starts ?? []
      note = 'Startpunkte gezogen. Sie stehen jetzt auf den Geräten der Teams.'
    } catch (err) {
      error = err.message
    } finally {
      busy = null
    }
  }

  function meter(m) {
    if (!m) return ''
    return m >= 1000 ? `${(m / 1000).toFixed(1)} km` : `${Math.round(m)} m`
  }

  function mb(bytes) {
    if (!bytes) return '0 MB'
    const mb = bytes / (1024 * 1024)
    return mb < 1 ? `${Math.round(bytes / 1024)} kB` : `${mb.toFixed(1)} MB`
  }

  function time(iso) {
    if (!iso) return '—'
    return new Date(iso).toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit' })
  }

  function clock(sec) {
    const s = Math.max(0, sec)
    return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
  }
</script>

<div class="pult">
  <div class="col">
    <section class="box">
      <header>
        <h3>Trockenübung</h3>
        {#if training.running}<span class="live mono">läuft</span>{/if}
      </header>
      <p class="dim">
        Simulierte Spieler laufen durch dasselbe Spiel und unterliegen denselben
        Regeln. In einer Viertelstunde seht ihr alles, was an einem Spieltag
        passiert – ohne dass jemand vor die Tür muss.
      </p>
      <div class="row">
        {#if training.running}
          <button class="ghost" onclick={() => training_('stop')} disabled={busy === 'training'}>
            Anhalten
          </button>
        {:else}
          <button class="primary" onclick={() => training_('start')} disabled={busy === 'training'}>
            Starten
          </button>
        {/if}
        <button class="ghost" onclick={() => training_('cleanup')} disabled={busy === 'training'}>
          Übungsteams entfernen
        </button>
      </div>
      {#if training.bots?.length}
        <p class="dim mono">Beteiligt: {training.bots.join(', ')}</p>
      {/if}
    </section>

    <section class="box">
      <header><h3>Bereitschaft</h3></header>
      <p class="dim">Wer die Einweisung durchlaufen hat und ein Signal sendet.</p>
      <ul class="ready">
        {#each readiness as t (t.id)}
          <li>
            <span class="who">{t.display || t.callsign}</span>
            <span class="rl">{roleLabel[t.role] ?? t.role}{t.bot ? ' · Übung' : ''}</span>
            <span class="flag" class:on={t.onboarded}>{t.onboarded ? 'eingewiesen' : 'offen'}</span>
            <span class="flag" class:on={t.hasFix}>{t.hasFix ? 'Signal' : 'kein Signal'}</span>
          </li>
        {/each}
      </ul>
    </section>

    <section class="box">
      <header><h3>Laufende Sperren</h3><span class="mono">{penalties.length}</span></header>
      {#if penalties.length === 0}
        <p class="dim">Keine.</p>
      {:else}
        <ul class="locks">
          {#each penalties as p (p.id)}
            <li>
              <span class="who">{p.callsign}</span>
              <span class="why">{p.reason}</span>
              <span class="left mono">{clock(p.leftSec)}</span>
              <button class="ghost small" onclick={() => lift(p)} disabled={busy === p.id}>
                Aufheben
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </section>

    <section class="box">
      <header><h3>Missionsfrist</h3></header>
      <p class="dim">
        Wenn der Auftrag am Ziel länger dauert, als die Frist hergibt. Am Ziel
        gewährt der Server die Kulanzzeit von selbst — hier geht es um alles
        darüber hinaus: ausgefallene Bahn, Laden zweihundert Meter neben der
        Nadel, ein GPS, das zwischen Häusern verrutscht.
      </p>

      {#if delays.length > 0}
        <ul class="delays">
          {#each delays as d (d.id)}
            <li class:grace={d.type === 'mission.grace'}>
              <span class="mono">{time(d.occurredAt)}</span>
              <span class="why">{d.reason || d.payload?.grund || '—'}</span>
            </li>
          {/each}
        </ul>
      {:else}
        <p class="dim">Bisher keine Meldung.</p>
      {/if}

      <div class="row">
        <label>
          <span class="label">Minuten</span>
          <input type="number" bind:value={extendMin} min="1" max="120" />
        </label>
        <label style="flex:2">
          <span class="label">Begründung</span>
          <input bind:value={extendReason} placeholder="z. B. Bahn ausgefallen" />
        </label>
      </div>
      <button class="primary" onclick={extendMission} disabled={busy === 'extend'}>
        Frist verlängern
      </button>
    </section>

    <section class="box">
      <header><h3>Punkte korrigieren</h3></header>
      <p class="dim">
        Für den Fall, dass ein Handy ausfällt oder eine Regel vor Ort anders
        ausgelegt wird. Die Begründung steht später im Protokoll.
      </p>

      <select bind:value={adjTeam}>
        <option value="">Team wählen …</option>
        {#each teams as t (t.team)}
          <option value={t.team}>{t.display || t.callsign}</option>
        {/each}
      </select>

      <div class="row">
        <label>
          <span class="label">Punkte</span>
          <input type="number" bind:value={adjPoints} />
        </label>
        <label>
          <span class="label">Fluchtpunkte</span>
          <input type="number" bind:value={adjFP} />
        </label>
      </div>

      <input bind:value={adjReason} placeholder="Begründung" />
      <button
        class="primary"
        onclick={adjust}
        disabled={busy === 'adjust' || !adjTeam || !adjReason.trim() || (!adjPoints && !adjFP)}
      >
        Buchen
      </button>
    </section>

    <section class="box">
      <header><h3>Spieldauer</h3></header>
      <p class="dim">
        Geplant sind {(session.game?.durationMin ?? 240) / 60} Stunden.
        Läuft das Spiel schon, wandert das Spielende um die Änderung mit.
      </p>

      <div class="row">
        <input type="number" step="0.5" min="0.5" max="24" bind:value={dauerStunden} />
        <button class="primary small" onclick={dauerSpeichern} disabled={busy === 'dauer'}>
          Übernehmen
        </button>
      </div>
      {#if dauerNote}<p class="dim">{dauerNote}</p>{/if}
    </section>

    <section class="box">
      <header><h3>Standortdaten</h3></header>
      <p class="dim">
        Die Bewegungsspur dieses Spiels löscht sich nach dem Spielende von
        selbst: Positionen, Meldefristen, Sichtkontakte, Beweisfotos und die
        Orte von Sperren. Punkte und Protokoll bleiben — ohne Koordinaten.
      </p>

      <div class="row">
        <input type="number" step="1" min="0" max="168" bind:value={spurStunden} />
        <button class="primary small" onclick={spurfristSpeichern} disabled={busy === 'spur'}>
          Stunden nach Spielende
        </button>
      </div>
      <p class="dim">0 bedeutet: sofort, sobald das Spiel beendet ist.</p>

      <button class="ghost small" onclick={spurJetztLoeschen} disabled={busy === 'spur'}>
        Jetzt löschen
      </button>
      {#if spurNote}<p class="dim">{spurNote}</p>{/if}
    </section>

    <section class="box">
      <header><h3>Zugang zurückziehen</h3></header>
      <p class="dim">
        Wer seine Einwilligung widerruft, verschwindet vollständig: Zugang,
        Positionen, Meldungen, Buchungen, Funksprüche. Die Einsatzzentrale
        selbst lässt sich nicht löschen.
      </p>

      {#if readiness.filter((t) => t.role !== 'hq').length === 0}
        <p class="dim">Noch keine Zugänge außer der Zentrale.</p>
      {:else}
        <ul class="plain">
          {#each readiness.filter((t) => t.role !== 'hq') as t}
            <li class="row between">
              <span>{t.display || t.callsign}</span>
              <button class="ghost small" onclick={() => teamLoeschen(t)} disabled={busy === 'team'}>
                Entfernen
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </section>

    <section class="box">
      <header><h3>Zugang anlegen</h3></header>
      <p class="dim">
        Für die Zielperson, für ein Fahndungsteam, das später dazustößt, oder
        wenn ein Zugang neu vergeben werden muss.
      </p>

      <div class="rollen">
        <label class:on={newRole === 'detective'}>
          <input type="radio" bind:group={newRole} value="detective" />
          Fahndung
        </label>
        <label class:on={newRole === 'misterx'}>
          <input type="radio" bind:group={newRole} value="misterx" />
          Zielperson
        </label>
      </div>

      <input bind:value={newCallsign} placeholder="Rufzeichen, z. B. Team_Delta" />
      <input bind:value={newDisplay} placeholder="Anzeigename (optional)" />
      <div class="row">
        <input bind:value={newPassword} placeholder="Kennwort, mind. 8 Zeichen" />
        <button class="ghost small" onclick={suggestPassword}>Vorschlag</button>
      </div>
      <button
        class="primary"
        onclick={createTeam}
        disabled={busy === 'team' || !newCallsign.trim() || newPassword.length < 8}
      >
        Anlegen
      </button>
    </section>

    <section class="box">
      <header><h3>Sicherung</h3></header>
      <p class="dim">
        Der ganze Spieltag steckt in einer Datei: Sektoren, Hotspots, Rätsel,
        Zugangsdaten, jede Buchung. Diese Kopie landet im Ordner „Downloads“ —
        also nicht neben dem Original, denn dort nützt sie nichts.
      </p>

      {#if backups.length > 0}
        <p class="dim mono last">
          Zuletzt gesichert: {time(backups[0].createdAt)} ({mb(backups[0].sizeBytes)})
        </p>
      {:else}
        <p class="dim">Bisher keine Sicherung.</p>
      {/if}

      <button class="primary" onclick={makeBackup} disabled={busy === 'backup'}>
        {busy === 'backup' ? 'Wird gepackt …' : 'Jetzt sichern und herunterladen'}
      </button>
    </section>

    <section class="box">
      <header><h3>Startpunkte</h3></header>
      <p class="dim">
        Gezogen wird vor dem Start, jeder Punkt nur einmal. Danach ist die
        Anreise dran — dafür lässt sich das Spiel anhalten, dann läuft für
        niemanden eine Frist.
      </p>

      {#if starts.length > 0 && starts.some((s) => s.number)}
        <ul class="starts">
          {#each starts as s (s.team)}
            <li class:x={s.role === 'misterx'}>
              <span class="wer">{s.display || s.callsign}</span>
              {#if s.number}
                <span class="wo mono">#{String(s.number).padStart(2, '0')} {s.name}</span>
                <!--
                  Der Anreisestand: Solange die Anreise läuft, ist die Frage
                  nicht "wo sind alle", sondern "sind alle da".
                -->
                <span class="an" class:da={s.arrived}>
                  {#if !s.known}noch keine Ortung
                  {:else if s.arrived}ist da
                  {:else}noch {meter(s.distanceM)}
                  {/if}
                </span>
              {:else}
                <span class="wo dim">kein Startpunkt</span>
              {/if}
            </li>
          {/each}
        </ul>
      {:else}
        <p class="dim">Noch nicht gezogen.</p>
      {/if}

      <button class="primary" onclick={drawStarts} disabled={busy === 'starts'}>
        {busy === 'starts' ? 'Wird gezogen …' : starts.some((s) => s.number) ? 'Neu ziehen' : 'Startpunkte ziehen'}
      </button>
    </section>

    <section class="box">
      <header><h3>Karte im Speicher</h3></header>
      <p class="dim">
        Die Kacheln kommen über diesen Server. Er holt jede einmal von
        OpenStreetMap und behält sie: Das zweite Gerät bekommt sie von hier,
        und was einmal jemand angesehen hat, bleibt auch im Funkloch sichtbar.
      </p>

      <p class="mono last">
        {kacheln.count} Kacheln · {mb(kacheln.bytes)}
        {#if kacheln.local}· eigener Kachelsatz gefunden{/if}
      </p>

      <p class="dim">
        Vorab geladen wird nichts — das untersagen die Bedingungen von
        OpenStreetMap. Wer ganz ohne Netz spielen will, legt einen eigenen
        Kachelsatz als Ordner „kacheln“ neben das Programm (Aufbau z/x/y.png);
        der hat dann Vorrang.
      </p>
    </section>

    {#if note}<p class="note">{note}</p>{/if}
    {#if error}<p class="err">{error}</p>{/if}
  </div>

  <div class="col">
    <Radio />
  </div>
</div>

<style>
  .starts {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }

  .starts li {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 0.1rem 0.6rem;
    padding: 0.35rem 0 0.35rem 0.5rem;
    border-left: 2px solid var(--det);
  }

  .starts li.x {
    border-left-color: var(--x);
  }

  .starts .wo {
    grid-column: 1 / -1;
    font-size: var(--fs-sm);
  }

  .starts .an {
    color: var(--muted);
    font-size: var(--fs-sm);
  }

  .starts .an.da {
    color: var(--ok);
  }

  .pult {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(20rem, 1fr));
    gap: 1rem;
    align-items: start;
    overflow-y: auto;
  }

  .col {
    display: flex;
    flex-direction: column;
    gap: 0.7rem;
    min-width: 0;
  }

  .box {
    background: var(--sheet);
    border: 1px solid var(--rule);
    border-top: 2px solid var(--hq);
    padding: 0.8rem 0.85rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
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

  header .mono {
    font-size: var(--fs-xs);
    color: var(--muted);
  }

  .locks {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }

  .live {
    font-family: var(--display);
    text-transform: uppercase;
    letter-spacing: 0.1em;
    font-size: var(--fs-xs);
    color: var(--ok);
  }

  .ready {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }

  .ready li {
    display: grid;
    grid-template-columns: 1fr auto auto auto;
    gap: 0.6rem;
    align-items: baseline;
    background: var(--sheet-2);
    padding: 0.3rem 0.5rem;
    font-size: var(--fs-sm);
  }

  .rl {
    font-size: var(--fs-xs);
    color: var(--muted);
  }

  .flag {
    font-size: var(--fs-xs);
    color: var(--faint);
  }

  .flag.on {
    color: var(--ok);
  }

  .delays {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .delays li {
    display: grid;
    grid-template-columns: 3.4rem 1fr;
    gap: 0.5rem;
    align-items: baseline;
    background: var(--sheet-2);
    border-left: 2px solid var(--warn);
    padding: 0.3rem 0.5rem;
    font-size: var(--fs-xs);
  }

  .delays li.grace {
    border-left-color: var(--ok);
  }

  .delays .why {
    color: var(--text);
    line-height: 1.4;
  }

  .locks li {
    display: grid;
    grid-template-columns: 6rem 1fr auto auto;
    gap: 0.5rem;
    align-items: center;
    background: var(--sheet-2);
    border-left: 2px solid var(--warn);
    padding: 0.35rem 0.5rem;
    font-size: var(--fs-sm);
  }

  .who {
    color: var(--text-hi);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .why {
    font-size: var(--fs-xs);
    color: var(--muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .left {
    font-size: var(--fs-xs);
    color: var(--warn);
  }

  /* Die Rolle ist die erste Entscheidung dieses Formulars, nicht eine unter
     vielen: Eine Zielperson mehr anzulegen, wo ein Team gemeint war, merkt
     man erst am Spieltag. */
  .rollen {
    display: flex;
    gap: 0.4rem;
  }

  .rollen label {
    flex: 1;
    text-align: center;
    padding: 0.4rem 0.6rem;
    border: 1px solid var(--rule);
    background: var(--sheet-2);
    cursor: pointer;
    font-size: var(--fs-sm);
  }

  .rollen label.on {
    border-color: var(--hq);
    color: var(--hq);
  }

  .rollen input {
    display: none;
  }

  .row {
    display: flex;
    gap: 0.4rem;
  }

  .row.between {
    justify-content: space-between;
    align-items: center;
  }

  .plain {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }

  .plain li {
    background: var(--sheet-2);
    padding: 0.35rem 0.5rem;
    font-size: var(--fs-sm);
  }

  .row label {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
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

  .dim {
    font-size: var(--fs-xs);
    color: var(--muted);
    line-height: 1.45;
  }

  .note,
  .err {
    font-size: var(--fs-sm);
    border-left: 2px solid;
    padding-left: 0.6rem;
    line-height: 1.45;
  }

  .note {
    color: var(--ok);
    border-left-color: var(--ok);
  }

  .err {
    color: var(--x);
    border-left-color: var(--x);
  }
</style>
