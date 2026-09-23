#!/usr/bin/env bash
#
# Baut Operation X für Windows, macOS und Linux.
#
# Ergebnis ist je System ein Ordner mit genau zwei Dingen darin: der
# Serverdatei und einem Startskript. Kein Installer, keine Abhängigkeiten —
# Ordner löschen genügt, um alles zu entfernen.
set -euo pipefail

cd "$(dirname "$0")/.."
VERSION="${1:-$(git describe --tags --always 2>/dev/null || echo dev)}"
OUT="dist"

echo "Baue Oberfläche …"
(cd web && npm run build >/dev/null)

rm -rf "$OUT"
mkdir -p "$OUT"

# CGO bleibt aus: Sonst bräuchte jedes Zielsystem seinen eigenen C-Compiler,
# und der Sinn einer einzelnen Datei wäre dahin. PocketBase nutzt eine reine
# Go-Fassung von SQLite, das geht auf.
export CGO_ENABLED=0

build() {
  local goos="$1" goarch="$2" name="$3" ext="${4:-}"
  local dir="$OUT/$name"

  echo "  $name"
  mkdir -p "$dir"
  GOOS="$goos" GOARCH="$goarch" go build \
    -trimpath \
    -ldflags "-s -w -X main.version=$VERSION" \
    -o "$dir/operationx$ext" ./cmd/operationx
}

build linux   amd64 "operation-x-linux-x64"
build linux   arm64 "operation-x-linux-arm64"
build windows amd64 "operation-x-windows-x64" ".exe"
build darwin  amd64 "operation-x-macos-intel"
build darwin  arm64 "operation-x-macos-apple"

# Startskripte: ein Doppelklick soll reichen.
for dir in "$OUT"/operation-x-linux-* "$OUT"/operation-x-macos-*; do
  cat > "$dir/START.command" <<'SH'
#!/usr/bin/env bash
# Startet den Spielserver und öffnet die Oberfläche im Browser.
cd "$(dirname "$0")"
./operationx serve --http=127.0.0.1:8090 &
SERVER=$!

# Kurz warten, dann die Oberfläche öffnen. Der Server sagt selbst, was zu tun
# ist – hier wird nichts wiederholt.
sleep 2
(command -v xdg-open >/dev/null && xdg-open http://127.0.0.1:8090) \
  || (command -v open >/dev/null && open http://127.0.0.1:8090) || true

wait $SERVER
SH
  chmod +x "$dir/START.command"
  cp "$dir/START.command" "$dir/start.sh"
done

for dir in "$OUT"/operation-x-windows-*; do
  cat > "$dir/START.bat" <<'BAT'
@echo off
rem Startet den Spielserver und oeffnet die Oberflaeche im Browser.
cd /d "%~dp0"

rem Der Browser wird mit Verzoegerung geoeffnet, nicht sofort: Vorher stand
rem hier ein "start" direkt vor dem Serverstart, und der Browser landete auf
rem einem Port, an dem noch niemand horchte. Der Nutzer sah als Erstes eine
rem Fehlerseite und musste neu laden.
start "" /min powershell -NoProfile -WindowStyle Hidden -Command "Start-Sleep 3; Start-Process 'http://127.0.0.1:8090'"

operationx.exe serve --http=127.0.0.1:8090
pause
BAT
done

# Kurzanleitung in jeden Ordner.
for dir in "$OUT"/*/; do
  cat > "$dir/ANLEITUNG.txt" <<'TXT'
Operation X — Spielserver
=========================

Alles, was gebraucht wird, liegt in diesem Ordner. Nichts wird installiert,
nichts wird woandershin geschrieben.


1. Starten
   Windows:      START.bat doppelklicken
   macOS/Linux:  START.command doppelklicken (oder ./start.sh im Terminal)

   Der Browser öffnet sich mit der Oberfläche. Beim ersten Start legt das
   Programm einen Ordner pb_data an — darin liegt die ganze Spielwelt.

   Das schwarze Fenster muss offen bleiben. Wird es geschlossen, ist der
   Server aus.


2. Beim allerersten Mal: einrichten
   Die Seite zeigt "Ersteinrichtung". Dort werden Name und Stadt des Spiels
   eingetragen und ein Kennwort für die Einsatzzentrale vergeben — auf
   "Vorschlag" klicken, wenn nichts einfällt.

   Dieses Kennwort aufschreiben. Es lässt sich später nicht auslesen, nur
   neu vergeben.

   Danach anmelden mit Rufzeichen  HQ  und diesem Kennwort.


3. Spiel vorbereiten
   Schnellster Weg: unter "Sektoren" steht ganz oben, ob für eure Stadt ein
   fertiges Paket dabei ist. Ist es das, sind Sektoren und Hotspots mit
   zwei Klicks gesetzt — mit einem Haken vor jedem Eintrag, damit ihr
   wegnehmen könnt, was nicht passt.

   Ist eure Stadt nicht dabei, gibt es dort einen Auftrag zum Kopieren:
   in eine KI einfügen (eine, die im Netz nachsehen kann), die Antwort
   zurückkopieren. Daraus wird derselbe Entwurf — samt Aufgaben für die
   Zielperson und Rätseln für die Fahndung. Er ist ein Vorschlag, kein
   Ergebnis: Die Maschine war nie in eurer Stadt. Seht jeden Punkt auf der
   Karte an, bevor ihr losspielt.

   Sonst von Hand, in dieser Reihenfolge:

     Sektoren  → Stadt suchen, Stadtteile holen, auswählen.
                 Passt keine Ebene, lässt sich auch selbst zeichnen.
     Hotspots  → Punkte setzen (Vorschläge oder Klick auf die Karte).
                 Je Punkt lässt sich eine Aufgabe hinterlegen — die liest
                 die Zielperson, wenn sie dieses Zwischenziel wählt.
     Rätsel    → schreiben, oder den Beispielsatz nehmen und anpassen
     Regelpult → Teams anlegen
     Druck     → vier Blätter:
                   Kartenblatt für alle Teams
                   Codeliste für euch
                   Zettel zum Anbringen
                   Teamkarten mit Zugangsdaten und QR-Code

   Ohne die angebrachten Zettel gibt es keine Vor-Ort-Codes.

   Die Teamkarten vergeben beim Erzeugen neue Kennwörter und zeigen sie
   einmal. Also erst drücken, wenn ihr gleich druckt.

   Regelwerte ändern: unter "Regeln" in der Kopfleiste. Dort schlagen auch
   die Spieler nach, was ein Joker kostet — mit den Werten eures Spiels.

   REGELN.html in diesem Ordner erklaert alle Regeln und wie sie angewendet
   werden. Sie braucht keinen laufenden Server, laesst sich auf dem Telefon
   lesen und sauber ausdrucken. Ein Ausdruck fuer die Zentrale lohnt sich.

   Zum Üben: im Regelpult die Trockenübung starten. Simulierte Spieler
   laufen durch dasselbe Spiel, während ihr zuseht.


4. Erreichbar machen
   Solange nur dieser Rechner den Server sieht, kann niemand mitspielen —
   und Browser geben über eine unverschlüsselte Adresse ohnehin keine
   Standortdaten heraus.

   Deshalb unter "Verteilen" den Tunnel starten. Dafür wird einmalig
   cloudflared gebraucht (von Cloudflare, kostenlos, kein Konto):

     Windows:  winget install --id Cloudflare.cloudflared
     macOS:    brew install cloudflared
     Linux:    von github.com/cloudflare/cloudflared/releases

   Danach in der Oberfläche auf "Erneut suchen" klicken.


5. Mitspieler holen
   Unter "Verteilen" steht ein QR-Code. Den scannen lassen — er führt auf
   die Anmeldeseite.

   Liegt operation-x.apk in diesem Ordner, wird die Android-App dort
   gleich mit angeboten. Sie ist die bessere Wahl: Sie meldet den Standort
   auch bei gesperrtem Bildschirm weiter, der Browser tut das nicht.

   Zum Installieren muss auf dem Handy einmal "Aus dieser Quelle
   installieren" erlaubt werden — Android fragt von selbst danach.


6. Sichern
   Im Regelpult unter "Sicherung" auf den Knopf drücken. Es landet eine
   Datei im Ordner "Downloads" — darin steckt der ganze Spieltag: Sektoren,
   Hotspots, Rätsel, Zugangsdaten, jede Buchung.

   Einmal nach dem Vorbereiten drücken ist die Minute wert. Wer die Zettel
   geschrieben und die Rätsel getippt hat, will das nach einem
   Festplattenfehler nicht noch einmal tun.

   Zurückspielen, falls doch etwas passiert:
     - Server beenden (schwarzes Fenster schließen)
     - Die ZIP-Datei entpacken
     - data.db und auxiliary.db daraus nach pb_data kopieren und die
       vorhandenen überschreiben
     - Server wieder starten


7. Karte ohne Netz
   Die Karte kommt über den Spielserver. Er holt jede Kachel einmal von
   OpenStreetMap und behält sie — das zweite Telefon bekommt sie von ihm, und
   was einmal jemand angesehen hat, bleibt auch im Funkloch sichtbar. Wie viel
   schon im Haus ist, steht im Regelpult.

   Für ein Spiel ganz ohne Internet: einen eigenen Kachelsatz als Ordner
   "kacheln" neben das Programm legen (Aufbau kacheln/z/x/y.png). Der wird
   dann immer zuerst genommen.


8. Zusätzliche Verschlüsselung
   Der Tunnel endet nicht auf diesem Rechner, sondern beim Betreiber des
   Tunnels — dort liegt der Verkehr im Klartext. Deshalb verschlüsseln die
   Geräte ein zweites Mal, mit einem Schlüssel, den nur sie und dieser Server
   kennen. Das läuft von selbst; zu tun ist nichts.

   Auf jedem Gerät steht oben ein Schloss mit vier Zeichen, unter "Verteilen"
   steht das vollständige Kennzeichen dieses Servers. Stimmen die ersten
   Zeichen überein, sitzt niemand dazwischen. Ein Blick darauf lohnt sich
   einmal zu Beginn.

   Ohne Schloss läuft das Spiel trotzdem — dann eben nur mit der
   Verschlüsselung der Verbindung. Im Heimnetz über eine IP-Adresse ist das
   der Normalfall: Dort gibt der Browser die Verschlüsselung nicht frei, und
   dort liegt auch kein Tunnel dazwischen.


Wenn etwas klemmt
-----------------
Seite bleibt leer            → Läuft das schwarze Fenster noch?
"Kein Kontakt zum Server"    → dasselbe
Tunnel startet nicht         → cloudflared fehlt, siehe Schritt 4
Kein Standort im Browser     → Es fehlt der Tunnel, siehe Schritt 4
Karte bleibt grau            → Der Server kommt nicht ins Internet. Was er
                                schon geholt hat, zeigt er trotzdem.
Kennwort der Zentrale weg    → pb_data löschen und neu einrichten;
                                das Spiel ist dann allerdings auch weg

Wieder entfernen: diesen Ordner löschen. Sonst bleibt nichts zurück.
TXT
done

# Das Regelwerk als eigene Seite in jeden Ordner: zum Nachschlagen am
# Spieltag, zum Ausdrucken vorher, und lesbar ohne laufenden Server.
#
# Und der Datenschutz daneben. Wer nur die fertige Datei herunterlaedt, sieht
# das Repository nie — waere er dort allein, bekaeme ihn ausgerechnet der
# nicht zu Gesicht, der ihn braucht: Verantwortlich ist, wer den Server
# betreibt. Die Einwilligungsvorlage zum Ausdrucken steht darin.
for dir in "$OUT"/*/; do
  cp docs/regelwerk.html "$dir/REGELN.html"
  cp DATENSCHUTZ.md "$dir/DATENSCHUTZ.md"
done

# Die Android-App mitliefern, wenn sie gebaut wurde. Der Server bietet sie
# dann unter /operation-x.apk zum Herunterladen an, und ein QR-Code genügt für
# alles.
APK="android/app/build/outputs/apk/debug/app-debug.apk"
if [ -f "$APK" ]; then
  echo "Lege die Android-App bei …"
  for dir in "$OUT"/*/; do
    cp "$APK" "$dir/operation-x.apk"
  done
else
  echo "Hinweis: Keine Android-App gefunden, es wird nur der Server ausgeliefert."
fi

echo
echo "Fertig in $OUT/:"
du -sh "$OUT"/*/ | sed 's|^|  |'
