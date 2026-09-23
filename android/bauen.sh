#!/usr/bin/env bash
# Baut die Android-App und spielt sie auf ein angeschlossenes Gerät.
#
# Sucht die Werkzeuge dort, wo sie üblicherweise liegen, und nimmt sonst, was
# im Pfad steht. Wer sie woanders hat, setzt JAVA_HOME, GRADLE oder ADB:
#
#     GRADLE=/opt/gradle/bin/gradle ./bauen.sh
#
# Voraussetzungen: JDK 21 (nicht neuer – das Android-Plugin kommt damit noch
# nicht zurecht), Gradle 8.13 und das Android-SDK mit Plattform 35.
set -euo pipefail
cd "$(dirname "$0")"

# Der erste Pfad, der existiert, gewinnt; sonst das, was im Pfad steht.
zuerst_vorhandenes() {
  for kandidat in "$@"; do
    [ -e "$kandidat" ] && { echo "$kandidat"; return; }
  done
  echo "$1"
}

export JAVA_HOME="${JAVA_HOME:-$(zuerst_vorhandenes \
  "$HOME/.local/opt/jdk-21.0.12.1+1" \
  /usr/lib/jvm/java-21-openjdk-amd64 \
  /usr/lib/jvm/java-21-openjdk \
  "${JAVA_HOME:-}")}"

GRADLE="${GRADLE:-$(command -v gradle || zuerst_vorhandenes \
  "$HOME/.local/opt/gradle-8.13/bin/gradle" \
  /opt/gradle/bin/gradle)}"

ADB="${ADB:-$(command -v adb || zuerst_vorhandenes \
  "$HOME/Android/Sdk/platform-tools/adb" \
  "$HOME/Library/Android/sdk/platform-tools/adb")}"

APK="app/build/outputs/apk/debug/app-debug.apk"

if [ ! -x "$GRADLE" ] && ! command -v "$GRADLE" >/dev/null 2>&1; then
  echo "Gradle nicht gefunden. Erwartet 8.13 – Pfad mit GRADLE=… angeben." >&2
  exit 1
fi

# Eigene Schalter vor Gradle abfangen: Gradle kennt sie nicht und bricht ab.
NUR_BAUEN=0
ARGS=()
for arg in "$@"; do
  if [ "$arg" = "--nur-bauen" ]; then NUR_BAUEN=1; else ARGS+=("$arg"); fi
done

# Erst prüfen, dann bauen: Die Tests laufen in Sekunden und decken die
# Stellen ab, die ohne Gerät prüfbar sind.
"$GRADLE" testDebugUnitTest assembleDebug --console=plain ${ARGS+"${ARGS[@]}"}

if [ "$NUR_BAUEN" = "1" ]; then
  echo "Gebaut: $APK"
  exit 0
fi

if "$ADB" get-state >/dev/null 2>&1; then
  echo "Spiele auf das Gerät …"
  "$ADB" install -r "$APK"
  echo "Fertig."
else
  echo "Kein Gerät angeschlossen – nur gebaut: $APK"
fi
