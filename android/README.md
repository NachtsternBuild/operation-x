# Operation X — Android

Native App in Kotlin mit Jetpack Compose und Material You. Die Weboberfläche
bleibt als Ausweichweg für iPhones und alle, die nichts installieren wollen;
beide sprechen dieselbe Schnittstelle unter `/api/opx`.

## Bauen

```bash
./bauen.sh              # prüft, baut und spielt auf ein angeschlossenes Gerät
./bauen.sh --nur-bauen  # nur bauen
```

Das Skript kennt die Pfade dieses Rechners, führt zuerst die Tests aus und
spielt das Ergebnis auf ein angeschlossenes Gerät, falls eines da ist. Von
Hand geht es auch:

```bash
export JAVA_HOME=~/.local/opt/jdk-21.0.12.1+1
~/.local/opt/gradle-8.13/bin/gradle :app:assembleDebug
```

Das APK liegt danach unter `app/build/outputs/apk/debug/app-debug.apk`.

Voraussetzungen: JDK 21 (nicht 25 — das Android-Gradle-Plugin kommt damit noch
nicht zurecht), Gradle 8.13, Android SDK mit Plattform 35 und Build-Tools 35.
Der SDK-Pfad steht in `local.properties`.

## Aufbau

```
data/       Schnittstelle zum Server, Modelle, Speicher mit Verfallszeit,
            Puffer für Funklöcher, zweite Verschlüsselung (Funk.kt)
location/   Vordergrunddienst für Standortmeldungen und Alarme
ui/         Beitritt, Anmeldung, Feldansicht mit ihren Reitern
ui/theme/   Material You mit festen Parteifarben
```

Die Karte gehört dem Feldbildschirm und nicht dem Reiter "Lage"
(`rememberKarte` in `ui/MapPanel.kt`). Lag sie im Reiter, wurde sie bei jedem
Wechsel neu gebaut: schwarze Sekunde, Kacheln noch einmal geholt, Ausschnitt
zurück auf Anfang.

## Zwei Grundsätze

**Material You bestimmt die Anmutung, das Spiel die Bedeutung.** Flächen,
Formen und Bedienelemente folgen der dynamischen Farbe des Geräts. Die drei
Parteifarben – Mister X rot, Fahndung cyan, Zentrale bernstein – und die
Zustandsfarben des Regelwerks bleiben fest, weil sie Information tragen. Wer
sein Hintergrundbild wechselt, darf nicht plötzlich die Fahndung in der Farbe
der Zielperson sehen.

**Meldungen gehen nie verloren.** In Bahn, Hinterhof und Funkloch kommt nichts
durch, deshalb liegt jede Standortmeldung zuerst im Puffer und wird beim
nächsten Netzkontakt nachgereicht – mit ihrem ursprünglichen Zeitstempel. Der
Server bewertet die Frist nach dem Erfassungszeitpunkt, nicht nach dem Eingang.

## Stand

Vollständig bedienbar: Beitritt per QR-Code oder eingetippter Adresse, Anmeldung
mit Rufzeichen, Vordergrunddienst mit Puffer und Fake-GPS-Erkennung, Lagekarte
mit Sektoren, Hotspots, Teams und Unschärfekreisen, Missionsbuch mit Routenwahl,
Vor-Ort-Code und Beweisfoto, Rätselmodul, Sichtkontakt, Zugriffsformular in drei
Stufen, Einsatzmittel, Funkkanal, Einweisung, Regelnachschlag, Punktekonto,
Benachrichtigungen bei gesperrtem Bildschirm, Feldmodus und Verfallszeit für
alle lokalen Daten.

Die Kacheln kommen über den Spielserver: Er holt jede einmal von
OpenStreetMap und liefert sie an alle Geräte. Ein eigenes Kartenpaket in der
App braucht es damit nicht mehr.

Der Verkehr ist zusätzlich zum Tunnel verschlüsselt. Das Kennzeichen des
Servers steht oben in der Leiste — dieselben Zeichen wie auf der gedruckten
Teamkarte bedeuten, dass niemand dazwischensitzt.

## Größe

Das Debug-APK liegt bei rund 49 MB — die Kartenbibliothek bringt native Teile
mit. Gebaut wird nur für arm64 und armv7; x86 gibt es nur in Emulatoren, und
der Download läuft am Spieltag oft über Mobilfunk. Ein Release-Build mit
aktiviertem Minify fällt nochmals deutlich kleiner aus.
