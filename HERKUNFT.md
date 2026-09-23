# Herkunft und Lizenzen

Operation X steht unter der MIT-Lizenz (siehe `LICENSE`). Was mitgeliefert,
eingebunden oder abgerufen wird, steht hier — mit seiner eigenen Lizenz.

Alles Aufgeführte ist quelloffen und mit MIT vereinbar. Eine Ausnahme gibt es
nicht; wo eine drohte, wurde sie ersetzt (siehe unten).

## Server (Go)

| Bestandteil | Lizenz | Wofür |
|---|---|---|
| [PocketBase](https://github.com/pocketbase/pocketbase) | MIT | Datenbank, Webserver, Anmeldung — als Bibliothek, nicht als fertiger Server |
| [cobra](https://github.com/spf13/cobra) | Apache-2.0 | Die Unterbefehle von `operationx` |
| [go-qrcode](https://github.com/skip2/go-qrcode) | MIT | Der Beitritts-QR-Code |

Die weiteren Einträge in `go.sum` sind mittelbare Abhängigkeiten von
PocketBase. Vollständige Liste mit Lizenzen:

```bash
go install github.com/google/go-licenses@latest
go-licenses report ./cmd/operationx
```

## Weboberfläche

| Bestandteil | Lizenz | Wofür |
|---|---|---|
| [Svelte](https://svelte.dev) | MIT | Die Oberfläche |
| [Vite](https://vitejs.dev) | MIT | Der Bauvorgang |
| [MapLibre GL JS](https://maplibre.org) | BSD-3-Clause | Die Karte im Browser |
| [Barlow](https://fonts.google.com/specimen/Barlow), Barlow Condensed | OFL-1.1 | Schrift |
| [IBM Plex Mono](https://github.com/IBM/plex) | OFL-1.1 | Schrift für Zahlen und Codes |

Die Schriften werden beim Bauen in `web/dist` eingebettet und damit
mitverteilt. Die OFL erlaubt das ausdrücklich, auch kommerziell, solange die
Schriften nicht einzeln verkauft werden.

## Android-App

| Bestandteil | Lizenz | Wofür |
|---|---|---|
| AndroidX, Jetpack Compose, CameraX | Apache-2.0 | Oberfläche, Kamera, Lebenszyklus |
| [OkHttp](https://square.github.io/okhttp/) | Apache-2.0 | Netzzugriff |
| [kotlinx.serialization](https://github.com/Kotlin/kotlinx.serialization) | Apache-2.0 | JSON |
| [MapLibre Native Android](https://maplibre.org) | BSD-2-Clause | Die Karte |
| [ZXing Core](https://github.com/zxing/zxing) | Apache-2.0 | QR-Codes lesen |

**Ersetzt:** Bis zur Veröffentlichung las den QR-Code *ML Kit* von Google. Es
funktioniert gut, ist aber ein geschlossenes Paket unter eigenen
Nutzungsbedingungen — in einer quelloffenen App der einzige Bestandteil
gewesen, den man nicht mitveröffentlichen könnte. ZXing tut dasselbe, steht
unter Apache-2.0 und macht die App neun Megabyte kleiner.

## iOS-App

Keine fremden Bibliotheken. SwiftUI, MapKit, CryptoKit, CoreLocation und
URLSession gehören zum System.

Nur zum Prüfen, nicht in der App: Der Prüfstand `ios/Funkprobe` bindet
[swift-crypto](https://github.com/apple/swift-crypto) (Apache-2.0) ein. Es
hat dieselbe Schnittstelle wie Apples CryptoKit und läuft auf Linux — damit
lässt sich die Verschlüsselung der iOS-App ohne Mac gegen einen echten Server
prüfen. In die App wandert davon nichts.

## Kartendaten

Die Karte stammt von **[OpenStreetMap](https://www.openstreetmap.org/copyright)**
und seinen Mitwirkenden, veröffentlicht unter der
[ODbL](https://opendatacommons.org/licenses/odbl/). Sie verlangt Namensnennung
— deshalb steht „© OpenStreetMap-Mitwirkende“ sichtbar auf jeder Karte dieses
Spiels: im Browser, in beiden Apps und auf dem gedruckten Kartenblatt.
Kartenbibliotheken legen diesen Hinweis gern hinter ein ⓘ; hier steht er auf
der Karte, weil eine Zeile, die niemand antippt, für eine Namensnennung zu
wenig ist.

Abgerufen werden drei Dienste, alle betrieben von der OpenStreetMap-Stiftung
und aus Spenden bezahlt:

* **Kacheln** über den Spielserver, der jede einmal holt und an alle Geräte
  weitergibt. Vorab geladen wird nichts — das untersagt die
  [Nutzungsrichtlinie](https://operations.osmfoundation.org/policies/tiles/)
  ausdrücklich, und sie hat recht damit.
* **Nominatim** für die Stadtsuche, mit Wartezeit zwischen den Anfragen.
* **Overpass** für Stadtteilgrenzen und Ortsvorschläge, sparsam und nur beim
  Einrichten.

Alle drei bekommen eine Kennung mit der Adresse dieses Projekts, damit sich
jemand melden kann, wenn sich das Programm schlecht benimmt. Sie steht in
`internal/geo/nominatim.go` unter `ProjektURL` — **beim Veröffentlichen auf die
echte Adresse ändern.**

Die mitgelieferten Stadtpakete in `internal/citysets/data` sind aus
OpenStreetMap-Daten erzeugt und stehen damit ebenfalls unter der ODbL.

## Der Tunnel

`cloudflared` wird **nicht** mitgeliefert und nicht automatisch
heruntergeladen. Wer den Tunnel benutzen will, installiert es selbst; die
Oberfläche nennt den Befehl. Es steht unter Apache-2.0, gehört aber Cloudflare
und ist ein Dienst, kein Bestandteil dieses Programms.
