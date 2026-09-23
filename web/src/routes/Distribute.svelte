<script>
  /*
   * Verteilung.
   *
   * Der Weg, auf dem alle ins Spiel kommen: Tunnel starten, QR-Code zeigen,
   * fertig. Ohne Tunnel ist der Server nur im eigenen Netz erreichbar – und
   * Browser geben über eine unverschlüsselte Adresse gar keine Standortdaten
   * heraus, womit das Spiel nicht funktioniert. Deshalb steht dieser Zustand
   * hier deutlich da und nicht im Kleingedruckten.
   */
  import { api } from '../lib/session.svelte.js'

  let tunnel = $state({ running: false })
  let schluessel = $state(null)
  let join = $state(null)
  let busy = $state(false)
  let error = $state(null)

  async function load() {
    try {
      const [t, j] = await Promise.all([
        api.get('/api/opx/hq/tunnel'),
        api.get('/api/opx/join'),
      ])
      tunnel = t
      try {
        schluessel = await api.get('/api/opx/key')
      } catch {
        /* ohne Kennzeichen läuft das Spiel trotzdem */
      }
      join = j
      error = null
    } catch (err) {
      error = err.message
    }
  }

  $effect(() => {
    load()
    const timer = setInterval(load, 4000)
    return () => clearInterval(timer)
  })

  async function toggle() {
    busy = true
    error = null
    try {
      await api.post(`/api/opx/hq/tunnel/${tunnel.running ? 'stop' : 'start'}`, {})
      await load()
    } catch (err) {
      error = err.message
    } finally {
      busy = false
    }
  }

  async function copyLink() {
    try {
      await navigator.clipboard.writeText(join.joinUrl)
    } catch {
      /* ohne Zwischenablage bleibt der Link zum Abtippen stehen */
    }
  }
</script>

<div class="cols">
  <section class="box">
    <header><h3>Erreichbarkeit</h3></header>

    {#if tunnel.running && tunnel.url}
      <p class="state ok">Der Server ist von unterwegs erreichbar.</p>
      <p class="url mono">{tunnel.url}</p>
      <div class="row">
        <button class="ghost" onclick={copyLink}>Link kopieren</button>
        <button class="ghost" onclick={toggle} disabled={busy}>Tunnel beenden</button>
      </div>
    {:else if tunnel.running}
      <p class="state">{tunnel.message || 'Tunnel wird aufgebaut …'}</p>
    {:else}
      <p class="warn">
        Ohne Tunnel ist der Server nur im eigenen Netz erreichbar. Browser geben
        über eine unverschlüsselte Adresse keine Standortdaten heraus — damit
        lässt sich nicht spielen.
      </p>
      {#if tunnel.binary}
        <button class="primary" onclick={toggle} disabled={busy}>
          {busy ? 'Starte …' : 'Tunnel starten'}
        </button>
      {:else}
        <p class="hint">
          Dafür wird <span class="mono">cloudflared</span> gebraucht. Es stammt von
          Cloudflare, kostet nichts und braucht kein Konto:
        </p>
        <p class="cmd mono">{tunnel.installHint}</p>
        <button class="ghost" onclick={load}>Erneut suchen</button>
      {/if}
    {/if}

    {#if error}<p class="err">{error}</p>{/if}

    <!--
      Das Kennzeichen des Servers.

      Der Tunnel endet nicht hier, sondern bei seinem Betreiber; dort liegt der
      Verkehr im Klartext. Deshalb verschlüsseln die Geräte ein zweites Mal —
      und weil auch der Schlüssel durch den Tunnel käme, gibt es diesen
      Vergleich: Wer die Zeichen auf seinem Gerät neben die hier hält und
      dieselben sieht, redet wirklich mit diesem Server.
    -->
    {#if schluessel?.available}
      <p class="hint">
        Kennzeichen dieses Servers — es steht auch auf jedem Gerät oben in der
        Kopfleiste. Stimmen die ersten Zeichen überein, sitzt niemand dazwischen:
      </p>
      <p class="fingerprint mono">{schluessel.fingerprint}</p>
    {/if}
  </section>

  <section class="box">
    <header><h3>Beitritt</h3></header>
    {#if join?.joinUrl}
      <p class="hint">
        Diesen Code scannen die Mitspieler. Er führt auf die Anmeldung — im
        Browser oder in der App.
      </p>
      <div class="qr">
        <img src="/api/opx/qr?url={encodeURIComponent(join.joinUrl)}" alt="QR-Code für den Beitritt" />
      </div>
      <p class="url mono small">{join.joinUrl}</p>

      {#if join.hasApp}
        <p class="hint">
          Die Android-App wird mit ausgeliefert. Wer den Code auf einem
          Android-Gerät scannt, bekommt sie auf der Anmeldeseite angeboten.
        </p>
        <a class="dl mono" href="/operation-x.apk" download>{join.appUrl}</a>
      {:else}
        <p class="hint dim">
          Für den App-Download die Datei <span class="mono">operation-x.apk</span> neben
          die Serverdatei legen.
        </p>
      {/if}
    {:else}
      <p class="hint">Adresse steht noch nicht fest.</p>
    {/if}
  </section>
</div>

<style>
  .fingerprint {
    font-size: var(--fs-lg);
    letter-spacing: 0.08em;
    color: var(--ok);
    background: var(--sheet);
    border: 1px solid var(--rule-hi);
    padding: 0.5rem 0.7rem;
    text-align: center;
  }

  .cols {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(19rem, 1fr));
    gap: 1rem;
    align-items: start;
    overflow-y: auto;
  }

  .box {
    background: var(--sheet);
    border: 1px solid var(--rule);
    border-top: 2px solid var(--hq);
    padding: 0.9rem;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  header {
    border-bottom: 1px solid var(--rule);
    padding-bottom: 0.45rem;
  }

  h3 {
    font-size: var(--fs-base);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }

  .state,
  .hint,
  .warn,
  .err {
    font-size: var(--fs-sm);
    line-height: 1.5;
    padding-left: 0.7rem;
    border-left: 2px solid var(--rule-hi);
  }

  .state.ok { color: var(--ok); border-left-color: var(--ok); }
  .warn { color: var(--warn); border-left-color: var(--warn); }
  .err { color: var(--x); border-left-color: var(--x); }
  .hint { color: var(--muted); }
  .hint.dim { color: var(--faint); }

  .url {
    font-size: var(--fs-base);
    color: var(--text-hi);
    overflow-wrap: anywhere;
  }

  .url.small { font-size: var(--fs-xs); color: var(--muted); }

  .cmd {
    font-size: var(--fs-sm);
    background: var(--sheet-2);
    border: 1px solid var(--rule);
    padding: 0.5rem 0.6rem;
    overflow-x: auto;
    color: var(--text-hi);
  }

  .row { display: flex; gap: 0.4rem; }

  .dl {
    font-size: var(--fs-xs);
    color: var(--hq);
    overflow-wrap: anywhere;
  }

  .qr {
    background: #fff;
    padding: 0.8rem;
    display: grid;
    place-items: center;
  }

  .qr img { width: 100%; max-width: 15rem; display: block; }
</style>
