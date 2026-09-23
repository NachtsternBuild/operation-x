# Operation X

Spielleitzentrale für *Operation X* – ein reales Verfolgungsspiel im öffentlichen Raum
nach dem Regelwerk v3.2. Eine einzige ausführbare Datei enthält Datenbank, Webserver,
Spiel-Engine und die komplette Weboberfläche.

Drei Dokumente daneben:

* [docs/struktur.html](docs/struktur.html) — **wo was steht**: Teile,
  Verzeichnisse, Datenmodell, der Weg einer Meldung durch das System
* [docs/regelwerk.html](docs/regelwerk.html) — **was das Spiel tut**: alle
  Regeln mit allen Werten
* [DATENSCHUTZ.md](DATENSCHUTZ.md) — **was mit den Daten geschieht**, und was
  du als Betreiber tun musst. Wer das Spiel mit Jugendlichen spielt, liest das
  vorher.

MIT-Lizenz ([LICENSE](LICENSE)). Woher die eingebundenen Teile stammen und unter
welchen Lizenzen sie stehen, steht in [HERKUNFT.md](HERKUNFT.md).

## Schnellstart

```bash
git clone <adresse> && cd operation-x
./bauen.sh
./operationx serve         # danach http://127.0.0.1:8090 öffnen
```

`./bauen.sh` holt, was fehlt — Go, Node, JDK 21, Gradle, Android-SDK —, baut
Oberfläche, Server und App und lässt alle Tests laufen. Ohne
Administratorrechte: Alles landet in `~/.local/opt` und `~/android-sdk`, ein
zweiter Lauf lädt nichts mehr. Wer die App nicht braucht, nimmt
`--ohne-android` und spart rund ein Gigabyte.

Läuft auf Linux und macOS. Unter Windows im WSL (Ubuntu); für die App dort
zusätzlich [usbipd](https://learn.microsoft.com/windows/wsl/connect-usb) für
das Telefon am Kabel.

Von Hand geht es auch:

```bash
cd web && npm install && npm run build && cd ..
go build -o operationx ./cmd/operationx
```

Die erste Zeile ist keine Kür: Die Oberfläche wird in die Binary eingebettet.
Wer sie überspringt, bekommt einen Server, der läuft und eine Seite zeigt, auf
der steht, was fehlt. `web/dist` liegt bewusst nicht im Verlauf — es ist ein
Bauergebnis, kein Quelltext. Nur der Platzhalter `web/dist/.gitkeep` liegt
dort, weil die Go-Einbettung ein Verzeichnis braucht.

Beim ersten Start ist die Datenbank leer. Die Anmeldeseite zeigt dann die
Ersteinrichtung: Name und Stadt des Spiels, Kennwort für die Einsatzzentrale.
Danach steht ein Zugang mit dem Rufzeichen `HQ` bereit, und die Einrichtung
läuft vollständig in der Oberfläche weiter.

Ein Server, ein Spiel — das ist hier die ganze Geschichte. Wer mehrere Spiele
gleichzeitig führen will (eine Klassenfahrt mit drei Gruppen, oder ein Server
für fremde Gruppen), findet das in einem eigenen Projekt:
[operation-x-server](../operation-x-server). Es bindet dieses hier als
Submodul ein und legt Konten, Trennung und Fristen darüber — Regeländerungen
hier wirken dort mit. In diesem Verzeichnis kommt davon nichts vor.

Zum Entwickeln geht es schneller mit einem fertigen Testspiel:

```bash
./operationx seed          # Testspiel Dresden mit Teams und Hotspots anlegen
```

Der `seed`-Befehl gibt die Zugangsdaten aller Testteams aus.

## Entwicklung

Die Oberfläche liegt eingebettet in der Binary. Beim Arbeiten am Frontend läuft sie
getrennt mit Live-Reload, der Vite-Server leitet `/api` an den Spielserver weiter:

```bash
./operationx serve --http=127.0.0.1:8090    # Terminal 1
cd web && npm install && npm run dev        # Terminal 2, öffnet Port 5173
```

Vor jedem Go-Build muss die Oberfläche gebaut sein, sonst landet ein
veralteter Stand in der Binary — oder beim ersten Mal gar keiner.

```bash
cd web && npm run build && cd .. && go build -o operationx ./cmd/operationx
```

### Prüfen

```bash
./bauen.sh --ohne-android     # baut alles und prüft alles
go test ./pkg/...        # Server: Regeln, Sichtbarkeit, Spieltrennung, Löschen
cd web && npm test            # Oberfläche: der Kern der Verschlüsselung
cd android && ./bauen.sh      # App: prüft vor dem Bauen
```

Ein Teil der Tests läuft gegen eine echte Datenbank in einem
Wegwerfverzeichnis (`pkg/testhilfe`) — dort stehen die Zusagen, die sich
nicht im Rechenweg prüfen lassen: dass ein Spiel die Daten eines anderen nicht
sieht, dass eine gelöschte Bewegungsspur wirklich weg ist, dass keine Sammlung
von außen offen steht.

Die zweite Verschlüsselung steht in vier Sprachen. Damit sie nicht
auseinanderlaufen, rechnen alle vier denselben Prüfvektor nach
(`testdaten/lagefunk.json`): Go, Kotlin und JavaScript in ihren Tests, Swift
in `ios/Funkprobe/pruefen.sh` — das zusätzlich gegen einen laufenden Server
redet, wenn einer da ist. Wer an der Ableitung dreht, bricht gleichzeitig vier
Prüfungen, statt am Spieltag einen leeren Bildschirm zu sehen.

## Aufbau

```
cmd/operationx/     Einstiegspunkt, Serverstart, CLI-Befehle
pkg/schema/    Datenmodell – 20 Collections, beim Start idempotent angelegt
pkg/api/       Rollengefilterte Endpunkte unter /api/opx
pkg/game/      Regeln und Engine – Fristen, Strafen, Joker, Finale, Löschung
pkg/geo/       Geometrie, OpenStreetMap-Abfragen (Nominatim, Overpass)
pkg/tiles/     Kartenkacheln: einmal holen, allen Geräten ausliefern
pkg/citysets/  Fertige Stadtpakete, im Programm mitgeliefert
pkg/crypt/     Zweite Verschlüsselung zwischen Server und Geräten
pkg/sim/       Trockenübung – simulierte Spieler, je Spiel ein Lauf
pkg/hub/       Lagestrom: eine offene Verbindung je Gerät
pkg/seed/      Testspiel Dresden
pkg/tunnel/    Ausgehender Tunnel, macht den Server von unterwegs erreichbar
pkg/testhilfe/ Echter Server in einem Wegwerfverzeichnis, für Tests
tools/citysets/     Erzeugt ein Stadtpaket aus OpenStreetMap-Daten
web/                Weboberfläche (Svelte 5, Vite), per go:embed eingebettet
android/            Native App (Kotlin, Jetpack Compose, Material You)
ios/                Native App (Swift, SwiftUI) – Entwurf, Verschlüsselung geprüft
scripts/            Auslieferung – build-release.sh baut alle Zielsysteme
docs/               Struktur, Regelwerk (struktur.html, regelwerk.html)
testdaten/          Prüfvektor der Verschlüsselung, für alle Sprachen dieselbe
bauen.sh            Holt die Werkzeuge, baut alles, prüft alles
pb_data/            Laufzeitdaten – Datenbank, Uploads (nicht im Repository)
```

## Verteilung

`scripts/build-release.sh` baut fünf Auslieferungsordner — Linux x64 und arm64,
Windows x64, macOS Intel und Apple Silicon. Jeder enthält die Serverdatei, ein
Startskript zum Anklicken, eine deutsche `ANLEITUNG.txt`, das Regelwerk als
`REGELN.html` und `DATENSCHUTZ.md`. Liegt vorher eine gebaute Android-App
unter `android/app/build/outputs/apk/debug/`, kommt sie als `operation-x.apk`
mit in jeden Ordner.

Der Datenschutz liegt bewusst im Ordner und nicht nur im Repository: Wer nur
die fertige Datei herunterlädt, ist trotzdem derjenige, der die Standortdaten
verarbeitet.

```bash
./scripts/build-release.sh 1.0
```

Gebaut wird mit `CGO_ENABLED=0` — PocketBase bringt eine reine Go-Umsetzung von
SQLite mit, deshalb genügt Kreuzkompilieren ohne fremde Werkzeugkette.

Der Server liefert eine danebenliegende `operation-x.apk` unter `/operation-x.apk`
aus. Gesucht wird sie neben der Serverdatei, nicht im Arbeitsverzeichnis: Wer das
Programm anklickt statt es in einer Eingabeaufforderung zu starten, hat selten das
richtige Verzeichnis stehen. Wer den Beitritts-QR-Code auf einem Android-Gerät
scannt, bekommt die App auf der Anmeldeseite angeboten.

### Warum ein Tunnel nötig ist

Ein Laptop im heimischen WLAN ist aus dem Mobilfunknetz nicht erreichbar. Eine
Portfreigabe setzt Wissen voraus, das laut Anforderung niemand haben muss, und
scheitert bei vielen Anschlüssen ohnehin an CGNAT. Verschärfend kommt hinzu, dass
Browser Standortdaten nur in einem sicheren Kontext herausgeben — über
`http://192.168.x.x` liefert die Ortung schlicht nichts, und ohne Ortung ist das
Spiel hinfällig.

`pkg/tunnel` startet deshalb auf Knopfdruck einen Schnelltunnel von
Cloudflare. Er baut die Verbindung von innen nach außen auf, durch jeden Router,
und liefert eine öffentliche Adresse mit gültigem Zertifikat. Gebraucht wird dafür
`cloudflared`; heruntergeladen wird es **nicht** automatisch — ein Programm, das im
Hintergrund Binärdateien nachlädt, ist genau das, was man einem
Selbsthosting-Werkzeug nicht zutrauen möchte. Fehlt es, nennt die Oberfläche den
passenden Installationsbefehl.

## Zwei Grundsätze, die überall gelten

**Die Spielregeln laufen ausschließlich auf dem Server.** Jeder Timer, jede Strafzeit,
jede Punktvergabe und jede Unschärfe auf der Karte wird zentral berechnet. Die Clients
zeigen nur an. Anders lässt sich bei einem Verfolgungsspiel weder Mogeln ausschließen
noch verhindern, dass ein Handy mit schlechtem Empfang seinen Träger benachteiligt.

**Kein Client greift direkt auf die Datenbank zu.** Sämtliche PocketBase-Zugriffsregeln
sind gesperrt; alles läuft über die Endpunkte in `pkg/api`, die serverseitig
zuschneiden. Nur dort lässt sich durchsetzen, dass Mister X keine exakten
Detektivpositionen bekommt und Detektive nicht erfahren, welcher Hinweis gefälscht ist.
Wer eine neue Collection anlegt, lässt ihre Regeln gesperrt.

## Nützliche Befehle

```bash
./operationx puzzles                        # Beispielrätsel Typ A–F anlegen
./operationx districts Dresden              # Ortsteile einer Stadt anzeigen
./operationx districts Dresden --levels 11  # nur eine Verwaltungsebene
./operationx config                         # alle Regelwerte anzeigen
./operationx config ping_interval_min 1     # einen Regelwert ändern
./operationx superuser create du@example.de # Notzugang zur Datenbank, nur lokal

go run ./tools/citysets Leipzig             # ein Stadtpaket erzeugen
```

Welche Ebene brauchbare Sektoren ergibt, unterscheidet sich je Stadt. In Dresden sind
die zehn Ortsamtsbereiche auf Ebene 9 mit 20 bis 160 km² zu groß; erst Ebene 11 trägt
die 61 statistischen Stadtteile mit ein bis vier Quadratkilometern.

Das vollständige Regelwerk mit allen Werten und ihrer Anwendung steht in
[`docs/regelwerk.html`](docs/regelwerk.html) — eine einzelne Datei ohne Netz,
die auch in jedem Auslieferungsordner als `REGELN.html` landet. Die Zahlen
darin sind an den Code gebunden: `pkg/game/regelwerk_test.go` vergleicht
jede von ihnen mit der Voreinstellung, damit die Seite nicht still altert.

## Stand

**Abschnitt 0 (Fundament)** — fertig. Datenmodell, Anmeldung mit Rufzeichen,
Rollentrennung, Designsystem, Testdatensatz.

**Abschnitt 1 (Karte und Sektoren)** — Lagekarte, Stadtsuche, echte
Stadtteilgrenzen aus OpenStreetMap, Sektoren anlegen, Hotspots setzen und
verwalten (Vorschläge aus den Kartendaten oder Klick auf die Karte), automatische
Sektorzuordnung, Nummern und Vor-Ort-Codes vom Server, Sektoren auch von Hand
gezeichnet, druckbares Kartenblatt.

**Android** — wird nativ gebaut (Kotlin/Compose, Material You), nicht als
Web-Hülle. Die Weboberfläche bleibt als Ausweichlösung für iPhones und alle, die
nichts installieren wollen. Damit ist `/api/opx` die einzige gemeinsame Grundlage
beider Clients und muss ohne Kenntnis der Web-Oberfläche bedienbar sein.

**Abschnitt 2 (Standort und Ping-Engine)** — Positionsmeldungen mit Puffer für
Funklöcher, Fristen nach Regelwerk mit vollständiger Strafkaskade, Transitmodus,
Plausibilitätsprüfung, Ereignisprotokoll und Zeitleiste im HQ. Die Live-Karte
zeigt jeder Rolle eine andere Wahrheit.

**Abschnitt 3 (Missionen und Beweise)** — Missionsbuch mit dynamisch erzeugten
Zwischenzielen, je drei Varianten zur Wahl, Nachweis per Vor-Ort-Code oder Foto
mit Abstandsprüfung, Beweisprüfung im HQ, Ausklinken gegen Fluchtpunkte,
Missionsfristen mit Sperre und Zwangsmeldung. Mister X ist damit spielfähig.

**Abschnitt 4 (Rätsel und Hinweise)** — Rätseltypen A bis F, Hinweiserzeugung
aus der tatsächlichen Lage, Frischestufen, Hinweistafel für die Fahndung,
Rätselpult im HQ mit Freischaltung und mitlesbaren Versuchen. Damit haben die
Detektive etwas zu tun, und der Spielkern ist vollständig.

**Abschnitt 6 (Zugriff, Finale, Sieg)** — Sichtkontakt mit Verweildauer-Prüfung,
Zugriffsformular in drei Stufen, Finalmodus mit schrumpfendem Suchbereich,
Siegermittlung über alle drei Wege (Zugriff, Fluchtziel erreicht, Zeitablauf).
Das Spiel hat damit ein Ende.

**Abschnitt 5 (Fluchtpunkte und Joker)** — alle elf Einsatzmittel beider Seiten
mit Kosten, Laufzeiten und Wechselwirkungen. Die Nebelkerze neutralisiert die
Ortungswerkzeuge der Fahndung, gefälschte Hinweise sind von echten nicht zu
unterscheiden, Sperrzonen und Wanzen lösen selbsttätig aus.

**Abschnitt 7 (Funkkanal und Regelpult)** — Live-Textkanal mit Vertrauensstufen,
Sperren aufheben, Punkte korrigieren, Teams mitten im Spiel nachtragen. Jeder
Eingriff landet mit Begründung im Protokoll.

**Einweisung und Trockenübung** — geführte Einweisung je Rolle mit
Handlungsprobe, Bereitschaftsliste im HQ, und simulierte Spieler, die im
Zeitraffer durch dasselbe Spiel laufen wie echte Teilnehmer.

Die Trockenübung ist zugleich das Prüfwerkzeug: Statt für jede Regeländerung
sechs Stunden durch die Stadt zu laufen, spielt sie den Ablauf in einer
Viertelstunde durch. Zu starten im Regelpult, bei laufendem Spiel.

**Abschnitt 8 (Android-App)** — native App in Kotlin und Jetpack Compose mit
Material You: Anmeldung, Karte, Missionen, Rätsel, Joker, Funk, Codescanner. Ein
Vordergrunddienst meldet den Standort auch bei gesperrtem Bildschirm weiter und
puffert bei Funkloch. Die Zugangsdaten löschen sich nach Ablauf von selbst.

**Abschnitte 9 und 10 (Verteilung und Server-Einrichtung)** — Ersteinrichtung
beim ersten Start, Auslieferungsordner für fünf Zielsysteme, Tunnel auf
Knopfdruck, Beitritts-QR-Code, App-Auslieferung über denselben Server.

**Lagestrom statt Nachfragen** — die Clients halten eine Verbindung offen
(Server-Sent Events unter `/api/opx/stream`), und der Server schickt, wenn sich
etwas geändert hat. Der Sichtkontakt-Alarm erreicht ein Telefon mit gesperrtem
Bildschirm damit in unter einer Sekunde statt im Schnitt fünfzehn. Die
Weboberfläche kommt in dreißig Sekunden Spielzeit auf vier Anfragen statt auf
mehrere Dutzend.

**Nachgezogene Lücken** — Spieluhr und Spielende, Anti-Camping, automatische
Löschung der Standortdaten, Rätsel schreiben und Regelwerte stellen in der
Oberfläche, Regelnachschlag für alle Rollen, Sektoren selbst zeichnen,
Teamkarten mit Zugangsdaten und QR-Code. In der App: Zugriff, Benachrichtigungen
bei gesperrtem Bildschirm, Einweisung und Beweisfoto.

Die App lief dafür zum ersten Mal auf echter Hardware (Cat S52, Android 14,
GSI). Was dabei gefunden wurde, steht in den Fallstricken.

**Abschnitt 11 (Prüfdurchgang)** — ein Durchgang über Server, Weboberfläche und
App. Siebzehn Befunde allein in der App, drei davon kosteten still Punkte: Das
Telefon bestrafte, wer sich hinsetzt, „Standort melden“ meldete nichts, und der
Lagestrom hielt einen eingefrorenen Punktestand fest.

**Karte ohne Netz** — die Kacheln kommen über den Spielserver: Er holt jede
einmal von OpenStreetMap, legt sie ab und liefert sie an alle Geräte. Begrenzt
auf das Spielgebiet, damit daraus kein Abzug der Weltkarte wird.

**Zweite Verschlüsselung** — jedes Gerät verschlüsselt zusätzlich zum Tunnel
(P-256, HKDF, AES-256-GCM), weil der Tunnel bei seinem Betreiber endet und der
Verkehr dort im Klartext liegt. Zum Vergleichen gibt es ein Kennzeichen auf
jedem Gerät und auf der gedruckten Teamkarte.

**Vorbereitung ohne Handarbeit** — fertige Stadtpakete (Dresden liegt bei) und
ein Auftrag zum Kopieren, mit dem eine KI Sektoren, Hotspots, Aufgaben und
Rätsel für eine beliebige Stadt vorschlägt. Beides landet als Entwurf auf der
Karte, mit einem Haken vor jedem Eintrag.

**Startpunkte und Pause** — die Zentrale lost Startpunkte aus (jeder nur
einmal, Mindestabstand), und eine Pause mit Ansage hält das ganze Spiel an:
Während ihr wird kein Standort aufgezeichnet, Fristen wandern um die Dauer nach
hinten.

**Mehrere Spiele auf einem Server** — liegt seit der Trennung woanders, in
[operation-x-server](../operation-x-server). Dieses Programm führt genau ein
Spiel und weiß von keinem anderen; das andere bindet es ein und baut die Ebene
darüber. Wer nur einen Nachmittag ausrichtet, braucht es nicht.

**Abschnitt 12 (iOS-App)** — liegt als vollständiger Entwurf in `ios/`:
SwiftUI, MapKit mit den Kacheln des Spielservers, CryptoKit für die zweite
Verschlüsselung, Standort im Hintergrund, keine fremde Bibliothek. Geschrieben
auf einem Rechner ohne Xcode und ohne iPhone, aber nicht mehr blind: Alle
Dateien werden vom Swift-Übersetzer geprüft, Modelle und Sitzungsspeicher
vollständig typgeprüft — und die Verschlüsselung redet nachweislich mit einem
echten Server (`ios/Funkprobe/pruefen.sh`, neun Prüfungen). Ungeprüft bleibt
alles, was die Apple-SDK braucht: SwiftUI, MapKit, CoreLocation. Was beim
ersten Bauen auf einem Mac zuerst zu prüfen ist, steht in `ios/README.md` —
samt der Frage, ob es ganz ohne Mac geht (ja, mit xtool, aber es kostet).
Bis dahin ist der Browser der Weg für iPhones.

**Ein Client, viele Server** — der Sitzungsschlüssel hängt an einer Adresse,
nicht an „irgendeinem Server“. Jedes Gerät merkt sich das Kennzeichen der
Server, auf denen es gespielt hat, und warnt, wenn dieselbe Adresse plötzlich
ein anderes nennt. Am Gerät durchgespielt: angemeldet auf Server A, dann
Server B hinter dieselbe Adresse gehängt — die Warnung kam, die Anmeldung auf B
klappte danach.

**Datenschutz eingelöst statt zugesagt** — die Bewegungsspur nimmt jetzt auch
Beweisfotos, Sperrorte und Koordinaten aus Meldungen mit; im Regelpult gibt es
Knöpfe für „sofort löschen“ und „Zugang zurückziehen“; der Server schaltet
IP-Protokollierung ab und setzt eine Grenze gegen das Durchprobieren von
Kennwörtern. Für den Betreiber liegt [DATENSCHUTZ.md](DATENSCHUTZ.md)
daneben — mit Einwilligungsvorlage zum Ausdrucken.

**Ein Befehl nach dem Klonen** — `./bauen.sh` holt Go, Node, JDK, Gradle und
das Android-SDK, baut Oberfläche, Server und App und lässt alle Tests laufen.
Geprüft an einem frischen Klon mit leeren Werkzeugordnern.

**Tests in vier Sprachen** — Go, Kotlin, JavaScript und Swift. Der wichtigste
ist ein gemeinsamer Prüfvektor: Alle vier rechnen dieselben Zahlen der
Verschlüsselung nach, damit sie nicht wieder lautlos auseinanderlaufen.

Damit ist das Regelwerk v3.2 vollständig umgesetzt, das Spiel von Anfang bis Ende
durchspielbar und der Server ohne Fachkenntnisse in Betrieb zu nehmen.

## Sicherheit

**Im Repository liegt kein einziges Geheimnis.** Keine Schlüssel, keine
Kennwörter, keine Zugangsdaten — auch nicht in der Historie. Alles, was geheim
ist, entsteht zur Laufzeit:

* Der Schlüssel für die zweite Verschlüsselung wird beim ersten Start erzeugt
  und liegt als `pb_data/lagefunk.key` mit Rechten `0600` neben der Datenbank.
  `pb_data/` ist von der Versionsverwaltung ausgeschlossen.
* Kennwörter liegen als Prüfsumme in der Datenbank und lassen sich nicht
  auslesen — nur neu vergeben. Auch die des Testspiels werden gewürfelt und
  einmal ausgegeben; `operationx seed` hat keine festen Zugänge.
* Zugangstoken und Standortdaten stehen in keiner Protokollzeile.

**Was die zweite Verschlüsselung leistet — und was nicht.** Jedes Gerät
verschlüsselt zusätzlich zum Tunnel (P-256, HKDF-SHA256, AES-256-GCM), weil
ein Tunnel bei seinem Betreiber endet und der Verkehr dort im Klartext liegt.
Zum Vergleichen gibt es ein Kennzeichen auf jedem Gerät und auf der gedruckten
Teamkarte. Der Spielserver selbst sieht weiterhin alles — er ist das Spiel —,
und Verkehrsdaten bleiben sichtbar. Wer den Rechner hat, hat die Datenbank;
dagegen hilft keine Verschlüsselung, sondern nur, selbst zu betreiben.
Wer für andere betreibt, findet das ausführlich in
[operation-x-server](../operation-x-server).

**Standortdaten löschen sich von selbst.** Die Bewegungsspur verschwindet nach
dem Spiel (Voreinstellung 24 Stunden), beendete und liegengebliebene Spiele auf
Wunsch ganz. Auf den Geräten verfallen Zugang und Kartendaten ebenfalls. Im
Regelpult gibt es beides auch als Knopf: die Spur sofort löschen, und einen
einzelnen Zugang samt allem, was an ihm hängt, zurückziehen — der Weg für
einen Widerruf mitten im Spiel.

**Was der Server von sich aus abstellt.** PocketBase schreibt ab Werk zu jeder
Anfrage die IP-Adresse mit; Operation X schaltet das bei jedem Start ab — für
dieses Spiel wird sie nirgends gebraucht. Ebenso wird eine Grenze gegen das
Durchprobieren von Kennwörtern gesetzt (zwanzig Anmeldeversuche je Minute und
Adresse). Die allgemeine Datenbankschnittstelle von PocketBase
(`/api/collections/…`) ist für jedermann gesperrt, auch für angemeldete Teams:
Daten gibt es nur über die Endpunkte unter `/api/opx`, und die filtern nach
Rolle und Spiel. Die Datenbankverwaltung unter `/_/` antwortet nur dem eigenen
Rechner, auch wenn ein Tunnel läuft. Beweisfotos liegen hinter der Anmeldung
statt am offenen Dateiweg — sie entstehen im öffentlichen Raum und zeigen im
Zweifel Unbeteiligte.

**Fehler gefunden?** Bitte nicht als öffentliches Ticket, sondern per E-Mail an
die Adresse im Git-Verlauf. Es ist ein Hobbyprojekt ohne Team — eine Antwort
kann dauern, aber sie kommt.

## Vor dem Veröffentlichen

Zwei Stellen tragen die Adresse dieses Projekts und müssen auf die echte
zeigen, sobald es irgendwo liegt:

* `go.mod` — der Modulpfad
* `pkg/geo/nominatim.go` — `ProjektURL`, die Kennung gegenüber den
  Diensten von OpenStreetMap. Sie verlangen einen erreichbaren Absender.

## Fallstricke

**Datumsvergleiche in Filtern** brauchen `game.DBTime(t)`, nicht `time.RFC3339`.
PocketBase legt Datumsfelder als Zeichenketten der Form
`2006-01-02 15:04:05.000Z` ab, mit Leerzeichen. Vergleiche laufen als
Zeichenkettenvergleich, und weil das Leerzeichen kleiner ist als das `T` in
RFC3339, ist dann *jeder* gespeicherte Wert kleiner als der Filterwert —
`expires_at > jetzt` trifft nie zu, `deadline_at <= jetzt` immer. Für
JSON-Antworten an die Clients bleibt RFC3339 richtig.

**Kamera aus einer App mit CAMERA im Manifest.** Wer die Berechtigung
deklariert, muss sie auch halten, um `ACTION_IMAGE_CAPTURE` auszulösen –
sonst wirft Android eine `SecurityException` und die App stürzt ab, statt
höflich zu fragen. Betrifft jede App, die die Systemkamera ruft und nebenbei
einen eigenen Scanner hat.

**Zustandsfarben unter Material You.** `primaryContainer` kann unter einem
warmen dynamischen Schema alarmrot ausfallen. Für „das hat geklappt" ist es
deshalb unbrauchbar. Zustände tragen Information und kommen aus den festen
Farben des Regelwerks, als Randstreifen statt Fläche – dieselbe Regel wie in
der Weboberfläche.

**Nutzerbedingungen sind keine Serverfehler.** Ein fehlender erster Ping kam
als `HTTP 500` beim Client an, weil ein Handler jeden Fehler aus einer
Hilfsfunktion pauschal als `InternalServerError` weitergab. Der Text stimmte,
der Status log: Er sagt "hier ist etwas kaputt", obwohl der Spieler es selbst
beheben kann. Solche Fälle bekommen einen eigenen Fehlerwert und `400`.

**Texte nach Zeichen kürzen, nicht nach Bytes.** `text[:500]` schneidet Bytes.
Im Deutschen liegt an jeder dritten Stelle ein Umlaut, und trifft die Grenze
mitten hinein, entsteht ungültiges UTF-8. Dafür gibt es `kuerzen()`.

**`e.Auth` nie speichern.** Es ist die Kopie vom Beginn der Anfrage. Bucht die
Engine in der Zwischenzeit etwas — sie läuft alle fünf Sekunden —, schreibt ein
`Save` dieser Kopie den alten Punktestand zurück und macht die Buchung
stillschweigend rückgängig. Vor jeder Änderung frisch laden.

**Ein `$effect`, der schreibt, was er liest, läuft endlos.** In Svelte 5
genügt es, `session.team` im selben Effekt zu lesen, der es über `refresh()`
schreibt — das Ergebnis waren über hundert Anfragen je Sekunde an `/me`, jede
davon beantwortet, keine davon auffällig. Effekte hängen deshalb nur an dem,
was sie wirklich als Auslöser brauchen.

**Kennwort ändern beendet die eigene Sitzung.** PocketBase entwertet beim
Setzen eines Kennworts alle Token des Datensatzes. Wer wie die Teamkarten alle
Zugänge neu vergibt, wirft sich selbst hinaus und muss sich sofort wieder
anmelden.

**Die Voreinstellungen von PocketBase sind für einen Baukasten gedacht.** Drei
davon taugen für diesen Server nicht: Eine Beispielsammlung `users` mit offener
Registrierung wird angelegt (dieses Projekt benutzt sie nie — sie wird beim
Start entfernt), zu jeder Anfrage wird die IP-Adresse fünf Tage lang
mitgeschrieben, und eine Grenze gegen das Durchprobieren von Kennwörtern gibt
es ab Werk nicht. Alles drei setzt `pkg/schema/einstellungen.go` bei jedem
Start, und ein Test hält es fest.

**Dateifelder sind ohne `Protected` öffentlich.** PocketBase liefert die Datei
jedem aus, der ihre Adresse kennt — ohne Anmeldung. Die Adresse ist nicht zu
erraten, aber das ist keine Zugangsprüfung. Für Beweisfotos aus dem
öffentlichen Raum zu wenig; sie gehen deshalb über einen eigenen Endpunkt, der
Rolle und Spiel prüft.

**Ein Sitzungsschlüssel gehört zu einer Adresse.** Wer ihn nur daran festmacht,
*dass* einer ausgehandelt wurde, redet nach einem Serverwechsel mit dem
falschen — und weil eine misslungene Entschlüsselung wie eine leere Antwort
aussieht, merkt man es nicht. Dasselbe gilt andersherum: Schlägt das Öffnen
fehl, gehört der Schlüssel weggeworfen und neu ausgehandelt, sonst bleibt die
App bis zum Neustart stumm.

**`android.util.Base64` macht eine Datei untestbar.** In einem gewöhnlichen
JVM-Test ist sie nicht vorhanden. Ab Android 26 tut `java.util.Base64`
dasselbe — und damit lässt sich die Verschlüsselung der App auf dem Rechner
prüfen statt auf einem Telefon.
