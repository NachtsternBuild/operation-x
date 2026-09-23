#!/usr/bin/env bash
#
# Operation X — von "git clone" zu einem laufenden Spiel, in einem Befehl.
#
#     ./bauen.sh                 alles: Werkzeuge, Oberfläche, Server, App, Tests
#     ./bauen.sh --ohne-android  ohne die App (spart rund ein Gigabyte Android-SDK)
#     ./bauen.sh --nur-server    nur Oberfläche und Server
#     ./bauen.sh --ohne-tests    bauen, ohne zu prüfen
#     ./bauen.sh --hilfe         diese Übersicht
#
# Was fehlt, wird geholt — nach $HOME/.local/opt und $HOME/android-sdk, ohne
# Administratorrechte und ohne am System zu drehen. Was schon da ist, wird
# benutzt. Ein zweiter Lauf lädt nichts mehr.
#
# Läuft auf Linux und macOS. Unter Windows im WSL (Ubuntu) — die Android-App
# braucht dort zusätzlich die USB-Weiterleitung, siehe README.
set -euo pipefail
cd "$(dirname "$0")"

# --- Was wir brauchen ------------------------------------------------------
#
# Feste Fassungen, damit auf jedem Rechner dasselbe herauskommt. Wer neuere
# hat, behält sie: Geprüft wird auf "mindestens".

GO_MIN="1.27"
GO_FASSUNG="1.27.0"
NODE_MIN="20"
NODE_FASSUNG="22.22.1"
JDK_FASSUNG="21"            # nicht neuer – das Android-Plugin kommt damit noch nicht zurecht
GRADLE_FASSUNG="8.13"
ANDROID_PLATTFORM="35"
ANDROID_WERKZEUGE="11076708" # Fassung der Android-Kommandozeilenwerkzeuge

WERKZEUGE="${OPX_WERKZEUGE:-$HOME/.local/opt}"
ANDROID_SDK="${ANDROID_HOME:-$HOME/android-sdk}"

MIT_ANDROID=1
MIT_TESTS=1

for arg in "$@"; do
  case "$arg" in
    --ohne-android) MIT_ANDROID=0 ;;
    --nur-server)   MIT_ANDROID=0 ;;
    --ohne-tests)   MIT_TESTS=0 ;;
    --hilfe|-h|--help)
      sed -n '3,16p' "$0" | sed 's/^# \{0,1\}//'
      exit 0 ;;
    *) echo "Unbekannter Schalter: $arg (siehe --hilfe)" >&2; exit 1 ;;
  esac
done

# --- Ausgabe ---------------------------------------------------------------

if [ -t 1 ]; then
  BLASS=$'\033[2m'; FETT=$'\033[1m'; GRUEN=$'\033[32m'; ROT=$'\033[31m'; AUS=$'\033[0m'
else
  BLASS=""; FETT=""; GRUEN=""; ROT=""; AUS=""
fi

schritt() { echo; echo "${FETT}$*${AUS}"; }
sagen()   { echo "  $*"; }
leise()   { echo "  ${BLASS}$*${AUS}"; }
gut()     { echo "  ${GRUEN}✓${AUS} $*"; }
fehler()  { echo "  ${ROT}✗${AUS} $*" >&2; }

abbruch() {
  echo >&2
  fehler "$1"
  echo >&2
  exit 1
}

# --- Kleine Helfer ---------------------------------------------------------

# Vergleicht Fassungen: "mindestens 1.27" ist wahr für 1.27.0 und 1.28.
mindestens() {
  [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -1)" = "$2" ]
}

hat() { command -v "$1" >/dev/null 2>&1; }

# Führt einen Befehl aus und schweigt dazu — es sei denn, er scheitert.
#
# Bauwerkzeuge reden viel: Vite warnt über ungenutzte CSS-Regeln, Gradle
# zählt Aufgaben, npm meldet Finanzierungsbitten. Wer dieses Skript zum ersten
# Mal laufen lässt, will davon nichts sehen, sondern wissen, ob es geklappt
# hat. Geht etwas schief, steht alles da.
PROTOKOLL="$(mktemp -t opx-bauen.XXXXXX)"
trap 'rm -f "$PROTOKOLL"' EXIT

ruhig() {
  if ! "$@" >"$PROTOKOLL" 2>&1; then
    echo >&2
    fehler "Gescheitert: $*"
    echo >&2
    sed 's/^/    /' "$PROTOKOLL" >&2
    exit 1
  fi
}

laden() {
  local url="$1" ziel="$2"
  # Der Fortschrittsbalken gehört auf einen Bildschirm. Läuft das Skript in
  # einer Protokolldatei oder in einer Pipeline, wären es nur tausend Rauten.
  local sichtbar=0
  [ -t 1 ] && sichtbar=1

  if hat curl; then
    if [ "$sichtbar" = "1" ]; then
      curl -fL --progress-bar "$url" -o "$ziel"
    else
      curl -fsSL "$url" -o "$ziel"
    fi
  elif hat wget; then
    if [ "$sichtbar" = "1" ]; then
      wget -q --show-progress "$url" -O "$ziel"
    else
      wget -q "$url" -O "$ziel"
    fi
  else
    abbruch "Weder curl noch wget vorhanden — eines davon wird gebraucht."
  fi
}

# Betriebssystem und Bauart, in der Schreibweise der jeweiligen Anbieter.
case "$(uname -s)" in
  Linux)  SYS="linux";  JDK_SYS="linux";  SDK_SYS="linux" ;;
  Darwin) SYS="darwin"; JDK_SYS="mac";    SDK_SYS="mac" ;;
  *) abbruch "Dieses Skript kennt nur Linux und macOS. Unter Windows: WSL." ;;
esac

case "$(uname -m)" in
  x86_64|amd64) BAUART="amd64"; NODE_BAUART="x64";   JDK_BAUART="x64" ;;
  arm64|aarch64) BAUART="arm64"; NODE_BAUART="arm64"; JDK_BAUART="aarch64" ;;
  *) abbruch "Unbekannte Bauart: $(uname -m)" ;;
esac

[ "$SYS" = "darwin" ] && NODE_SYS="darwin" || NODE_SYS="linux"

mkdir -p "$WERKZEUGE"

# --- Werkzeuge -------------------------------------------------------------

werkzeug_go() {
  if hat go && mindestens "$(go env GOVERSION 2>/dev/null | sed 's/^go//')" "$GO_MIN"; then
    gut "Go $(go env GOVERSION | sed 's/^go//') vorhanden"
    return
  fi
  if [ -x "$WERKZEUGE/go/bin/go" ]; then
    export PATH="$WERKZEUGE/go/bin:$PATH"
    gut "Go aus $WERKZEUGE/go"
    return
  fi

  sagen "Go $GO_FASSUNG wird geholt (rund 70 MB) …"
  local pak="$WERKZEUGE/go.tar.gz"
  laden "https://go.dev/dl/go${GO_FASSUNG}.${SYS}-${BAUART}.tar.gz" "$pak"
  tar -xzf "$pak" -C "$WERKZEUGE"
  rm -f "$pak"
  export PATH="$WERKZEUGE/go/bin:$PATH"
  gut "Go $GO_FASSUNG eingerichtet"
}

werkzeug_node() {
  if hat node && mindestens "$(node --version | sed 's/^v//')" "$NODE_MIN"; then
    gut "Node $(node --version) vorhanden"
    return
  fi
  local ordner="$WERKZEUGE/node-v${NODE_FASSUNG}-${NODE_SYS}-${NODE_BAUART}"
  if [ -x "$ordner/bin/node" ]; then
    export PATH="$ordner/bin:$PATH"
    gut "Node aus $ordner"
    return
  fi

  sagen "Node $NODE_FASSUNG wird geholt (rund 30 MB) …"
  local pak="$WERKZEUGE/node.tar.xz"
  laden "https://nodejs.org/dist/v${NODE_FASSUNG}/node-v${NODE_FASSUNG}-${NODE_SYS}-${NODE_BAUART}.tar.xz" "$pak"
  tar -xJf "$pak" -C "$WERKZEUGE"
  rm -f "$pak"
  export PATH="$ordner/bin:$PATH"
  gut "Node $NODE_FASSUNG eingerichtet"
}

werkzeug_jdk() {
  # Das Android-Plugin will genau 21 – ein neueres JDK lehnt es ab.
  if [ -n "${JAVA_HOME:-}" ] && [ -x "$JAVA_HOME/bin/javac" ] &&
     "$JAVA_HOME/bin/javac" -version 2>&1 | grep -q " ${JDK_FASSUNG}\."; then
    gut "JDK $JDK_FASSUNG aus JAVA_HOME"
    return
  fi

  local vorhandenes
  vorhandenes="$(ls -d "$WERKZEUGE"/jdk-${JDK_FASSUNG}* 2>/dev/null | head -1 || true)"
  if [ -n "$vorhandenes" ]; then
    export JAVA_HOME="$vorhandenes"
    [ -d "$JAVA_HOME/Contents/Home" ] && JAVA_HOME="$JAVA_HOME/Contents/Home"
    export PATH="$JAVA_HOME/bin:$PATH"
    gut "JDK aus $JAVA_HOME"
    return
  fi

  if hat javac && javac -version 2>&1 | grep -q " ${JDK_FASSUNG}\."; then
    export JAVA_HOME="$(dirname "$(dirname "$(readlink -f "$(command -v javac)")")")"
    gut "JDK $JDK_FASSUNG im System gefunden"
    return
  fi

  sagen "JDK $JDK_FASSUNG wird geholt (rund 190 MB) …"
  local pak="$WERKZEUGE/jdk.tar.gz"
  laden "https://api.adoptium.net/v3/binary/latest/${JDK_FASSUNG}/ga/${JDK_SYS}/${JDK_BAUART}/jdk/hotspot/normal/eclipse" "$pak"
  tar -xzf "$pak" -C "$WERKZEUGE"
  rm -f "$pak"
  export JAVA_HOME="$(ls -d "$WERKZEUGE"/jdk-${JDK_FASSUNG}* | head -1)"
  [ -d "$JAVA_HOME/Contents/Home" ] && export JAVA_HOME="$JAVA_HOME/Contents/Home"
  export PATH="$JAVA_HOME/bin:$PATH"
  gut "JDK eingerichtet: $JAVA_HOME"
}

werkzeug_gradle() {
  if hat gradle && mindestens "$(gradle --version 2>/dev/null | awk '/^Gradle/{print $2}')" "$GRADLE_FASSUNG"; then
    GRADLE="$(command -v gradle)"
    gut "Gradle $(gradle --version | awk '/^Gradle/{print $2}') vorhanden"
    return
  fi
  GRADLE="$WERKZEUGE/gradle-${GRADLE_FASSUNG}/bin/gradle"
  if [ -x "$GRADLE" ]; then
    gut "Gradle aus $WERKZEUGE/gradle-${GRADLE_FASSUNG}"
    return
  fi

  sagen "Gradle $GRADLE_FASSUNG wird geholt (rund 130 MB) …"
  hat unzip || abbruch "unzip fehlt — bitte nachinstallieren (apt install unzip)."
  local pak="$WERKZEUGE/gradle.zip"
  laden "https://services.gradle.org/distributions/gradle-${GRADLE_FASSUNG}-bin.zip" "$pak"
  unzip -q "$pak" -d "$WERKZEUGE"
  rm -f "$pak"
  gut "Gradle $GRADLE_FASSUNG eingerichtet"
}

werkzeug_android() {
  local sdkmanager="$ANDROID_SDK/cmdline-tools/latest/bin/sdkmanager"

  if [ ! -x "$sdkmanager" ]; then
    sagen "Android-Kommandozeilenwerkzeuge werden geholt (rund 130 MB) …"
    hat unzip || abbruch "unzip fehlt — bitte nachinstallieren (apt install unzip)."
    local pak="$WERKZEUGE/cmdline-tools.zip"
    laden "https://dl.google.com/android/repository/commandlinetools-${SDK_SYS}-${ANDROID_WERKZEUGE}_latest.zip" "$pak"
    mkdir -p "$ANDROID_SDK/cmdline-tools"
    rm -rf "$ANDROID_SDK/cmdline-tools/latest"
    unzip -q "$pak" -d "$ANDROID_SDK/cmdline-tools"
    mv "$ANDROID_SDK/cmdline-tools/cmdline-tools" "$ANDROID_SDK/cmdline-tools/latest"
    rm -f "$pak"
  fi

  export ANDROID_HOME="$ANDROID_SDK"
  export ANDROID_SDK_ROOT="$ANDROID_SDK"

  if [ ! -d "$ANDROID_SDK/platforms/android-${ANDROID_PLATTFORM}" ]; then
    sagen "Android-Plattform ${ANDROID_PLATTFORM} und Bauwerkzeuge (rund 600 MB) …"
    leise "Die Lizenzen von Google werden dabei bestätigt."
    yes | "$sdkmanager" --sdk_root="$ANDROID_SDK" --licenses >/dev/null 2>&1 || true
    "$sdkmanager" --sdk_root="$ANDROID_SDK" \
      "platform-tools" \
      "platforms;android-${ANDROID_PLATTFORM}" \
      "build-tools;${ANDROID_PLATTFORM}.0.0" >/dev/null
  fi

  # Gradle findet das SDK über diese Datei. Sie ist Rechnersache und liegt
  # deshalb nicht im Verlauf — also hier anlegen.
  if [ ! -f android/local.properties ]; then
    echo "sdk.dir=$ANDROID_SDK" > android/local.properties
    leise "android/local.properties angelegt"
  fi

  gut "Android-SDK in $ANDROID_SDK"
}

# --- Los -------------------------------------------------------------------

echo
echo "${FETT}Operation X wird gebaut${AUS}"
leise "Werkzeuge in $WERKZEUGE · nichts davon braucht Administratorrechte"

schritt "1/4  Werkzeuge"
werkzeug_go
werkzeug_node
if [ "$MIT_ANDROID" = "1" ]; then
  werkzeug_jdk
  werkzeug_gradle
  werkzeug_android
fi

schritt "2/4  Weboberfläche"
(
  cd web
  if [ -d node_modules ] && [ package-lock.json -ot node_modules ]; then
    leise "Abhängigkeiten sind aktuell"
  else
    sagen "Abhängigkeiten werden geholt …"
    ruhig npm install --no-audit --no-fund --loglevel=error
  fi
  sagen "Oberfläche wird gebaut …"
  ruhig npm run build
)
gut "web/dist gebaut"

schritt "3/4  Server"
sagen "Go-Abhängigkeiten …"
ruhig go mod download
sagen "Server wird gebaut …"
ruhig go build -o operationx ./cmd/operationx
gut "./operationx gebaut"

if [ "$MIT_ANDROID" = "1" ]; then
  schritt "4/4  Android-App"
  sagen "Das erste Mal dauert einige Minuten …"
  (cd android && export GRADLE="$GRADLE" JAVA_HOME="$JAVA_HOME" && ruhig ./bauen.sh --nur-bauen)
  gut "android/app/build/outputs/apk/debug/app-debug.apk gebaut"
else
  schritt "4/4  Android-App"
  leise "übersprungen"
fi

if [ "$MIT_TESTS" = "1" ]; then
  schritt "Prüfen"

  sagen "Server …"
  ruhig go test ./pkg/...
  gut "Go-Tests durch"

  sagen "Weboberfläche …"
  (cd web && ruhig npm test)
  gut "Web-Tests durch"

  if [ "$MIT_ANDROID" = "1" ]; then
    sagen "Android …"
    (cd android && ruhig "$GRADLE" -q testDebugUnitTest)
    gut "Android-Tests durch"
  fi
fi

# --- Wie es weitergeht -----------------------------------------------------

echo
echo "${FETT}Fertig.${AUS}"
echo
sagen "Server starten:      ./operationx serve"
sagen "Danach im Browser:   http://127.0.0.1:8090"
echo
sagen "Zum Ausprobieren ein fertiges Testspiel mit Zugängen:"
sagen "  ./operationx seed"
if [ "$MIT_ANDROID" = "1" ]; then
  echo
  sagen "App auf ein angestecktes Telefon spielen:"
  sagen "  cd android && ./bauen.sh"
fi
echo
if [ "$WERKZEUGE" != "${OPX_WERKZEUGE:-}" ] && [ -d "$WERKZEUGE/go" ]; then
  leise "Go liegt in $WERKZEUGE/go/bin und steht nur in diesem Skript im Pfad."
  leise "Dauerhaft: export PATH=\"$WERKZEUGE/go/bin:\$PATH\" in die Shell-Datei."
  echo
fi
