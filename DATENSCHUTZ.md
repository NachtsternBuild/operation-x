# Datenschutz

Operation X verarbeitet die Standorte von Menschen, oft von Jugendlichen, über
mehrere Stunden. Das ist der Kern des Spiels und zugleich der Grund, warum
dieses Dokument existiert.

**Die wichtigste Zeile zuerst: Verantwortlich ist, wer den Server betreibt —
nicht das Projekt.** Die Software läuft auf deinem Rechner, die Daten liegen
in deinem `pb_data`, und niemand sonst bekommt sie zu sehen. Damit bist du im
Sinne der DSGVO der Verantwortliche (Art. 4 Nr. 7). Was daraus folgt, steht
hier.

Dieses Dokument ist keine Rechtsberatung. Es ist der Versuch, alles
zusammenzutragen, was ein Betreiber wissen muss, damit ein Spieltag sauber
abläuft.

---

## 1. Was verarbeitet wird

| Was | Woher | Personenbezug |
|---|---|---|
| Standort (Breite, Länge, Genauigkeit, Tempo, Zeitpunkt) | Das Telefon, etwa alle zehn Minuten | **Ja**, und der heikelste Teil |
| Rufzeichen, Anzeigename, Rolle, Farbe | Bei der Einrichtung vergeben | Je nachdem, was ihr eintragt |
| Kennwort | Erzeugt, gespeichert als Prüfsumme | Nicht auslesbar |
| Funksprüche | Die Teams selbst | Freier Text, also alles möglich |
| Beweisfotos | Die Zielperson am Hotspot | **Ja**, auch von Unbeteiligten im Bild |
| Punkte, Buchungen, Ereignisse | Das Spiel selbst | Verhalten einer Person über den Tag |
| Sichtkontakte, Sperren, Zugriffe | Das Spiel selbst | Mit Ort und Zeit |

Die vollständige Liste aller Tabellen steht in
[docs/struktur.html](docs/struktur.html), Abschnitt 04.

**Nicht verarbeitet wird:** keine E-Mail-Adressen der Spieler (Teams haben
keine), keine Telefonnummern, keine Klarnamen — es sei denn, ihr tragt sie als
Anzeigenamen ein. **Tut das nicht.** „Team Alpha“ genügt dem Spiel vollauf.

---

## 2. Wozu, und auf welcher Grundlage

Zweck ist die Durchführung des Spiels: Standorte, damit die Fahndung etwas zu
fahnden hat; Fristen, damit niemand sich versteckt; Punkte, damit es einen
Ausgang gibt.

Rechtsgrundlage ist in aller Regel die **Einwilligung** (Art. 6 Abs. 1 lit. a
DSGVO). Sie muss:

* **freiwillig** sein — wer nicht möchte, spielt nicht mit, und das darf keine
  Nachteile haben,
* **informiert** sein — vorher muss klar sein, was erhoben wird und wie lange
  es bleibt,
* **widerrufbar** sein — jederzeit, für die Zukunft, ohne Begründung.

Eine Vorlage zum Ausdrucken steht unten in Abschnitt 9.

### Minderjährige

Bei Kindern und Jugendlichen unter 16 Jahren braucht es die Einwilligung der
Erziehungsberechtigten (Art. 8 DSGVO). Für eine Jugendgruppe oder eine
Klassenfahrt heißt das: **Der Zettel geht vorher nach Hause.** Das ist kein
Formalismus — es geht um Bewegungsprofile von Kindern.

Wer zwischen 16 und 18 ist, kann selbst einwilligen; ob das im konkreten Fall
reicht, entscheidet die Einrichtung, die den Ausflug verantwortet.

---

## 3. Wie lange die Daten bleiben

Das Programm löscht von selbst, in zwei Stufen:

| Stufe | Was | Voreinstellung | Wo einstellbar |
|---|---|---|---|
| Bewegungsspur | Positionen, Meldefristen, Sichtkontakte, Beweisfotos samt Aufnahmeort, Orte von Sperren | **24 Stunden nach Spielende** | Zentrale → Regelpult → *Standortdaten* |
| Ganzes Spiel | Alles übrige: Teams, Rätsel, Hinweise, Buchungen | bleibt, bis du es löschst | `pb_data/` löschen — damit ist der Server wieder leer |

Nach der ersten Stufe steht kein Ort mehr in der Datenbank — was bleibt, ist
das Protokoll ohne Koordinaten: die Antwort auf „warum habe ich minus 15?“,
ohne die Antwort auf „wo warst du um 15:40?“.

**Auf den Geräten** verfallen Zugang und zwischengespeicherte Meldungen
ebenfalls (24 Stunden bzw. mit dem Spielende). In den Einstellungen der
Android-App gibt es zusätzlich *Alles löschen*.

Willst du früher löschen, geht das jederzeit: **Zentrale → Regelpult →
Standortdaten → Jetzt löschen.** Danach steht kein Ort mehr in der Datenbank.
Dort lässt sich auch die Frist ändern — `0` bedeutet „sofort, sobald das Spiel
beendet ist“.

Das ganze Spiel verschwindet, indem du `pb_data/` löschst — danach ist der
Server wieder leer wie am ersten Tag.

---

## 4. Wer was sieht

| | Eigene Position | Fremde Position | Protokoll |
|---|---|---|---|
| Fahndungsteam | genau | andere Fahnder genau, Zielperson nur als Unschärfekreis und nur, wenn erarbeitet | eigenes Kontobuch |
| Zielperson | genau | Fahndung nur als Unschärfekreis | eigenes Kontobuch |
| Einsatzzentrale | — | **alles genau** | vollständig |

Hier fallen Spielleitung und Betreiber zusammen: Der Server läuft auf deinem
Rechner, und **wer den Rechner hat, hat die Datenbank.** Wer möchte, dass
niemand sonst mitlesen kann, betreibt selbst.

---

## 5. Was das Programm von sich aus tut

Datenschutz durch Voreinstellung (Art. 25 DSGVO) ist hier nicht angeflanscht,
sondern eingebaut:

* **Keine IP-Adressen im Protokoll.** PocketBase schreibt sie ab Werk zu jeder
  Anfrage mit; Operation X schaltet das bei jedem Start ab. Ebenso die
  Kennung des angemeldeten Kontos.
* **Die Bewegungsspur löscht sich selbst**, ohne dass jemand daran denken muss.
* **Die Geräte sprechen nur mit dem Spielserver.** Kartenkacheln holt der
  Server und gibt sie weiter — OpenStreetMap erfährt nicht, wer wo hinsieht.
  Kein Analysedienst, kein Absturzmelder, keine Werbekennung, keine
  Google-Dienste in der App.
* **Zweite Verschlüsselung** zwischen Gerät und Server, zusätzlich zum Tunnel
  (P-256, HKDF-SHA256, AES-256-GCM). Der Betreiber eines Tunnels sieht damit
  nur, *dass* jemand etwas schickt, nicht *was*.
* **Die allgemeine Datenbankschnittstelle ist für alle gesperrt.** Daten gibt
  es nur über Endpunkte, die nach Rolle und Spiel filtern.
* **Die Datenbankverwaltung ist auf den eigenen Rechner beschränkt** — auch
  wenn der Server über einen Tunnel öffentlich erreichbar ist.
* **Eine Grenze gegen das Durchprobieren von Kennwörtern**: zwanzig
  Anmeldeversuche je Minute und Adresse.
* **Beweisfotos liegen hinter der Anmeldung.** Sie entstehen im öffentlichen
  Raum und zeigen im Zweifel Leute, die von diesem Spiel nichts wissen.
  Abrufen kann sie nur die Einsatzzentrale ihres eigenen Spiels.
* **Widerruf und Löschung sind Knöpfe, keine Datenbankarbeit.** Das Regelpult
  der Zentrale hat beides: die Spur dieses Spiels sofort löschen, und einen
  einzelnen Zugang samt allem, was an ihm hängt, entfernen.
* **Die Spieler werden vor der ersten Messung informiert.** Die Einweisung
  hat eine Seite „Was dieses Spiel über euch speichert“, in allen drei
  Oberflächen wortgleich, und sie steht vor dem Einschalten der Ortung.

---

## 6. Empfänger außerhalb

Ohne Tunnel verlässt kein personenbezogenes Datum deinen Rechner. Sonst:

* **OpenStreetMap-Stiftung** — Kartenkacheln, Stadtsuche, Stadtteilgrenzen.
  Angefragt wird das **nur vom Server**, nicht von den Geräten, und nichts
  davon enthält Spielerdaten. Beim Einrichten sieht Nominatim den gesuchten
  Stadtnamen, mehr nicht.
* **Cloudflare (cloudflared-Tunnel)** — nur wenn du ihn einschaltest. Dann
  läuft der gesamte Verkehr über Server von Cloudflare, einem Unternehmen mit
  Sitz in den USA: eine Übermittlung in ein Drittland (Kapitel V DSGVO). Die
  Inhalte sind durch die zweite Verschlüsselung geschützt, die Verkehrsdaten
  — IP-Adressen der Spieler, Zeitpunkte, Datenmengen — nicht.

  **Wenn du das vermeiden willst:** Spielt im selben WLAN oder gib die
  Portfreigabe deines Routers frei. Der Tunnel ist Bequemlichkeit, keine
  Notwendigkeit. Nutzt du ihn, gehört er in die Einwilligung.

---

## 7. Rechte der Spieler, und wie du sie erfüllst

| Recht | Artikel | Wie du es einlöst |
|---|---|---|
| Auskunft | 15 | Zentrale → Zeitleiste und Kontobuch des Teams; für eine vollständige Kopie die Datenbankverwaltung unter `http://127.0.0.1:8090/_/` am eigenen Rechner öffnen und die Datensätze des Teams exportieren |
| Berichtigung | 16 | Anzeigename und Rolle in der Zentrale ändern |
| Löschung | 17 | **Regelpult → Zugang zurückziehen → Entfernen** löscht ein Team samt allem, was an ihm hängt. Für das ganze Spiel: *Standortdaten → Jetzt löschen*, oder `pb_data/` löschen |
| Widerruf der Einwilligung | 7 Abs. 3 | Derselbe Knopf: **Regelpult → Zugang zurückziehen.** Mit dem Zugang fallen Positionen, Meldungen, Buchungen und Funksprüche dieser Person |
| Beschwerde | 77 | Zuständig ist die Datenschutzaufsicht des Bundeslandes, in dem du sitzt |

Ein Widerruf während des Spiels beendet für diese Person die Teilnahme — ohne
Standort gibt es kein Spiel. Das ist zumutbar und muss vorher klar sein.

---

## 8. Wenn du für andere betreibst

Dieses Programm führt ein Spiel auf deinem Rechner: Du bist Spielleitung und
Verantwortlicher in einer Person. Wer einen Server stellt, auf dem *fremde*
Gruppen spielen, verarbeitet deren Daten im Auftrag — dann gilt Art. 28 DSGVO
mit allem, was dazugehört. Das steht im anderen Projekt:
[operation-x-server](../operation-x-server), Abschnitt „Auftragsverarbeitung".

## 9. Vorlage: Einwilligung zum Ausdrucken

> **Einwilligung zur Teilnahme an Operation X**
>
> *Spielleitung:* ……………………………………………………
> *Datum und Ort des Spiels:* ……………………………………………………
>
> Operation X ist ein Verfolgungsspiel in der Stadt. Während des Spiels
> übermittelt die App auf dem Telefon der Teilnehmerin oder des Teilnehmers
> etwa alle zehn Minuten den aktuellen Standort an einen Server, den die
> Spielleitung selbst betreibt. Dazu kommen Punkte, Spielereignisse und
> Nachrichten, die im Spielfunk geschrieben werden. Die Zielperson macht
> unterwegs Beweisfotos.
>
> **Wer verantwortlich ist:** die oben genannte Spielleitung. Der Server läuft
> auf ihrem Gerät. *Kontakt:* ……………………………………………………
>
> **Wozu:** ausschließlich zur Durchführung dieses Spiels.
>
> **Wie lange:** Die Standortdaten und die Beweisfotos werden spätestens
> ……… Stunden nach Spielende automatisch gelöscht (Voreinstellung: 24).
> Punktestand und Spielprotokoll ohne Ortsangaben bleiben
> ……………………………… erhalten.
>
> **Wer sie sieht:** die Spielleitung vollständig; die anderen Teams nur, was
> die Spielregeln vorsehen (die Zielperson erscheint als ungefährer Kreis).
>
> **Übermittlung nach außen:** ☐ nein — der Server ist nur im örtlichen Netz
> erreichbar · ☐ ja — der Zugang läuft über einen Tunnel des Anbieters
> Cloudflare (USA); dabei werden Verbindungsdaten dort verarbeitet. Die
> Inhalte sind zusätzlich verschlüsselt.
>
> **Freiwilligkeit:** Die Teilnahme ist freiwillig. Die Einwilligung kann
> jederzeit für die Zukunft widerrufen werden, ohne Angabe von Gründen und
> ohne Nachteile. Mit dem Widerruf endet die Teilnahme am Spiel, und bereits
> erhobene Standortdaten werden auf Wunsch sofort gelöscht.
>
> **Rechte:** Auskunft, Berichtigung, Löschung, Einschränkung, Widerspruch,
> Datenübertragbarkeit sowie das Recht auf Beschwerde bei der zuständigen
> Datenschutzaufsichtsbehörde.
>
> ☐ Ich bin einverstanden.
>
> Name: ………………………………………  Datum: …………………
>
> Unterschrift: ………………………………………
>
> *Bei Minderjährigen unter 16 Jahren zusätzlich:*
>
> Name der/des Erziehungsberechtigten: ………………………………………
>
> Unterschrift: ………………………………………

---

## 10. Checkliste vor dem Spieltag

- [ ] Einwilligungen eingesammelt — bei Minderjährigen von den Eltern
- [ ] Anzeigenamen geprüft: Teamnamen statt Klarnamen
- [ ] Entschieden, ob ein Tunnel läuft — und es in der Einwilligung vermerkt
- [ ] Löschfrist für die Bewegungsspur geprüft (Zentrale → Regelpult)
- [ ] Bei dauerhaftem Betrieb: `keep_days` und `idle_days` gesetzt
- [ ] Den Teams gesagt, dass Beweisfotos keine fremden Gesichter zeigen sollen
- [ ] Nach dem Spiel: Spiel beenden, damit die Frist überhaupt zu laufen beginnt
- [ ] Wer unterwegs aussteigt: *Regelpult → Zugang zurückziehen*

Der letzte Punkt ist der, der in der Praxis vergessen wird: Ein Spiel, das nie
beendet wurde, hat kein Spielende — und ohne Spielende beginnt keine Frist.
Deshalb gibt es die Frist für liegengebliebene Spiele. Einschalten muss man
sie trotzdem.
