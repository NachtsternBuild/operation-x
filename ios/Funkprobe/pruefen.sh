#!/usr/bin/env bash
# Prüft die Verschlüsselung der iOS-App gegen einen laufenden Spielserver.
#
#     ./pruefen.sh                      # gegen http://127.0.0.1:8090
#     ./pruefen.sh http://192.168.1.5:8090
#
# Braucht Swift (swift.org/install), sonst nichts. Läuft auf Linux, macOS und
# überall, wo es eine Swift-Toolchain gibt – ein Mac ist dafür nicht nötig.
set -euo pipefail
cd "$(dirname "$0")"

# Die echte Funk.swift der App, nicht eine Kopie: Was hier durchgeht, geht
# auch auf dem iPhone durch.
cp ../OperationX/Data/Funk.swift Sources/Funkprobe/Funk.swift

swift build
.build/debug/Funkprobe "${1:-http://127.0.0.1:8090}"
