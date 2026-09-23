# Operation X — iOS

Native App in Swift mit SwiftUI, als Gegenstück zur Android-Fassung. Dieselbe
Schnittstelle unter `/api/opx`, dieselben Bildschirme, dieselben Texte.

## Zuerst das Wichtigste: was geprüft ist und was nicht

Dieser Quelltext entstand auf einem Linux-Rechner, ohne Xcode und ohne iPhone.
Was hier liegt, ist trotzdem kein blinder Entwurf mehr:

| | Stand |
|---|---|
| Alle 14 Dateien, Syntax | geprüft (`swiftc -parse`, Swift 6.4) |
| `Models.swift`, `Session.swift` | vollständig typgeprüft |
| `Funk.swift` — die Verschlüsselung | **gegen einen echten Server geprüft**, siehe unten |
| SwiftUI, MapKit, CoreLocation | ungeprüft — dafür braucht es die Apple-SDK |

Die Verschlüsselung war die gefährlichste Stelle, weil sie lautlos scheitert:
Passt das Salz oder das Beiwerk nicht zeichengenau zum Server, kommen leere
Seiten statt einer Fehlermeldung. Genau die ist jetzt nachgewiesen — mit der
echten Datei, gegen den echten Server (`Funkprobe/`).

Ungeprüft bleibt alles, was die Apple-SDK braucht. Rechnet dort mit
Tippfehlern und mit Feinheiten von SwiftUI, die sich erst im Übersetzer
zeigen. Deshalb halten sich alle Dateien an Bestandteile, die seit Jahren
stabil sind: SwiftUI, URLSession, CryptoKit, CoreLocation, MapKit. **Keine
einzige fremde Bibliothek** — es gibt nichts aufzulösen, nichts zu
versionieren, nichts, was am Spieltag fehlt.

## Die Verschlüsselung ohne Mac prüfen

```bash
cd ios/Funkprobe && ./pruefen.sh http://127.0.0.1:8090
```

Braucht nur eine Swift-Toolchain (<https://swift.org/install>) und einen
laufenden Spielserver. Der Prüfstand übersetzt die **echte**
`OperationX/Data/Funk.swift` — keine Kopie — und redet damit verschlüsselt mit
dem Server: Handschlag, Antwort öffnen, Anfrage mit Abfrageteil, mehrere
Nummern hintereinander, zwei Clients nebeneinander, und dass ein fremder
Schlüssel sauber scheitert statt Unsinn zu liefern.

Möglich macht das ein Ziel namens `CryptoKit`, das
[swift-crypto](https://github.com/apple/swift-crypto) durchreicht — dieselbe
Schnittstelle wie Apples CryptoKit, nur quelloffen und auf Linux lauffähig.
Die App selbst hängt davon nicht ab; das ist nur Prüfwerk.

## Ganz ohne Mac bauen — geht, kostet aber

Mit [xtool](https://github.com/xtool-org/xtool) lässt sich ein SwiftPM-Paket
auf Linux zu einer iOS-App bauen, signieren und auf ein angestecktes iPhone
spielen. Nötig sind dafür:

* Swift 6.4 und `usbmuxd`,
* **Xcode.xip von Apple** (rund 10 GB, Apple-ID und Lizenzzustimmung nötig) —
  daraus baut xtool die Darwin-SDK. Apples Lizenz erlaubt Xcode nur auf
  Apple-Geräten; wer das auf einem Linux-Rechner tut, sollte das wissen,
* eine Apple-ID zum Signieren (eine kostenlose genügt, sieben Tage Laufzeit),
* ein **echtes iPhone am Kabel** — einen Simulator gibt es auf Linux nicht,
* und den Umbau dieses Ordners von einem Xcode-Projekt zu einem
  SwiftPM-Paket mit `xtool.yml`.

Kurz: technisch möglich, aber der bequemere Weg bleibt eine Stunde an einem
geliehenen Mac. Für die Verschlüsselung braucht es keinen davon — dafür ist
`Funkprobe/` da.

## Bauen

```bash
brew install xcodegen          # einmalig
cd ios && xcodegen generate
open OperationX.xcodeproj
```

Ohne XcodeGen: in Xcode ein neues iOS-App-Projekt anlegen, den Ordner
`OperationX` hineinziehen und die Einträge aus `project.yml` unter `info` von
Hand in die Ziel-Einstellungen übernehmen. Fünf Minuten.

Zum Spielen braucht es ein Entwicklerkonto (ein kostenloses genügt) und ein
Kabel: Xcode signiert die App auf euren Namen und spielt sie auf das Gerät.
Eine so signierte App läuft sieben Tage, danach neu aufspielen.

## Aufbau

```
OperationX/
  OperationXApp.swift     Einstieg, drei Zustände: verbinden, anmelden, spielen
  AppState.swift          Der Zustand der ganzen App und alle Handlungen
  Data/
    Models.swift          Die Antworten des Servers, eins zu eins
    Api.swift             Netz, Fehler auf Deutsch, zweite Verschlüsselung
    Funk.swift            ECDH P-256, HKDF, AES-GCM — mit CryptoKit
    Session.swift         Speicher mit Verfallszeit, Puffer fürs Funkloch
  Location/
    LocationService.swift Standort im Hintergrund
  UI/
    Theme.swift           Die drei Parteifarben und geteilte Bausteine
    JoinLogin.swift       Beitritt, Anmeldung, Einweisung
    FieldView.swift       Kopf, Meldeleiste, Reiter, Lagekarte
    MapPanel.swift        MapKit mit den Kacheln des Spielservers
    MissionPanel.swift    Missionsbuch der Zielperson
    PuzzlePanel.swift     Rätsel, Sichtkontakt, Zugriff in drei Stufen
    Mittel.swift          Einsatzmittel, Punktekonto, Regeln, Funk
Funkprobe/                Prüfstand für die Verschlüsselung, läuft auf Linux
```

## Drei Entscheidungen, die von Android abweichen

**Die Karte kommt von MapKit, nicht von MapLibre.** Der Untergrund ist eine
`MKTileOverlay` auf `/api/opx/tiles/{z}/{x}/{y}` — dieselben Kacheln, die
Browser und Android-App bekommen, geholt vom Spielserver. Damit entfällt eine
fremde Kartenbibliothek samt ihren 40 MB, und das Kartenbild bleibt überall
dasselbe.

**Kein QR-Scanner.** Auf dem iPhone erkennt die Kamera-App QR-Codes von selbst
und öffnet die Beitrittsseite. Ein zweiter Scanner in der App wäre Arbeit für
eine Fähigkeit, die das Gerät schon hat. Die Adresse lässt sich außerdem
eintippen.

**Gefragt statt zugehört.** Die Android-App hält eine offene Verbindung zum
Server (SSE) und erfährt jede Änderung in unter einer Sekunde. Hier fragt die
App alle zehn Sekunden nach. Der Unterschied fällt genau an einer Stelle auf:
Der Sichtkontakt-Alarm erreicht die Zielperson später. Das ist der nächste
Schritt, wenn der Rest läuft — `URLSession.bytes(for:)` liest einen
Ereignisstrom, und jede Sendung ist einzeln verschlüsselt (siehe
`web/src/lib/funk.svelte.js`, Funktion `openStreamEvent`).

## Was beim ersten Übersetzen zu erwarten ist

In dieser Reihenfolge prüfen — oben steht, was am ehesten klemmt. Die
Verschlüsselung stand früher an erster Stelle; sie ist inzwischen geprüft
(siehe oben) und deshalb hier heraus.

1. **`LocationService`, der Hintergrund.** `allowsBackgroundLocationUpdates`
   wirft, wenn der Hintergrundmodus im Projekt fehlt. Und ohne die Erlaubnis
   *Immer* hört die Meldung auf, sobald der Bildschirm ausgeht — genau der
   Fehler, den diese App vermeiden soll. Im Feld prüfen, nicht am Schreibtisch.
2. **`MapPanel`.** `canReplaceMapContent` sorgt dafür, dass Apples Grundkarte
   verschwindet. Bleibt sie sichtbar, liegen zwei Karten übereinander.
3. **Die Zeitstempel.** Der Server erwartet RFC 3339 in UTC und bewertet
   Fristen nach dem Erfassungszeitpunkt. Eine Meldung mit Ortszeit wäre um
   Stunden daneben und würde als verspätet gelten.
4. **Die Modelle.** Alle Felder sind absichtlich optional: Swift benutzt
   Vorgabewerte beim Dekodieren nicht, und der Server lässt leere Felder weg.
   Ein nicht-optionales Feld würde eine ganze Antwort verwerfen.

## Was die App kann

Beitritt, Anmeldung, Standort im Hintergrund mit Puffer und Fake-GPS-Erkennung,
Lagekarte mit Sektoren, Hotspots, Teams und Unschärfekreisen, Missionsbuch mit
Routenwahl, Vor-Ort-Code, Beweisfoto und Verzögerungsmeldung, Rätsel,
Sichtkontakt, Zugriff in drei Stufen, Einsatzmittel, Funkkanal, Regelnachschlag,
Punktekonto, Einweisung, Pausenanzeige, Startpunkt — und die zweite
Verschlüsselung mit dem Kennzeichen des Servers in der Kopfleiste.

Nicht dabei: Benachrichtigungen bei gesperrtem Bildschirm (die Android-Fassung
weckt damit die Zielperson beim Sichtkontakt) und der Ereignisstrom. Beides
hängt am selben Punkt: Solange gefragt statt zugehört wird, gibt es nichts,
worauf eine Benachrichtigung reagieren könnte.
