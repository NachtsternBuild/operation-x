import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

// Redet mit einem laufenden Spielserver – verschlüsselt, mit der echten
// Funk.swift aus der iOS-App. Was hier durchgeht, geht auf dem iPhone auch
// durch: Es ist derselbe Quelltext, dieselben Regeln, derselbe Server.

let basis = CommandLine.arguments.count > 1 ? CommandLine.arguments[1]
                                            : "http://127.0.0.1:8090"
var fehler = 0

func pruefe(_ was: String, _ ok: Bool, _ dazu: String = "") {
    print(ok ? "  ok   \(was)" : "  FEHL \(was) \(dazu)")
    if !ok { fehler += 1 }
}

func hole(_ pfad: String, funk: Funk?) async -> (Int, Data)? {
    guard let url = URL(string: basis + pfad) else { return nil }
    var anfrage = URLRequest(url: url)
    anfrage.httpMethod = "GET"

    var nummer: Int64 = 0
    if let funk {
        nummer = funk.naechsteNummer()
        anfrage.setValue(funk.publicKey, forHTTPHeaderField: "X-Opx-Key")
        anfrage.setValue("\(nummer)", forHTTPHeaderField: "X-Opx-Seq")
    }

    guard let (roh, antwort) = try? await URLSession.shared.data(for: anfrage),
          let http = antwort as? HTTPURLResponse else { return nil }

    guard let funk, let nonceB64 = http.value(forHTTPHeaderField: "X-Opx-Nonce"),
          let nonce = Funk.entschluesselnB64(nonceB64) else { return (http.statusCode, roh) }

    let beiwerk = Funk.beiwerk(methode: "GET", pfad: pfad, nummer: nummer)
    guard let klar = funk.oeffnen(roh, nonce: nonce, beiwerk: beiwerk) else {
        return (http.statusCode, Data())
    }
    return (http.statusCode, klar)
}

// ---------------------------------------------------------------------------
// Teil 1: der gemeinsame Prüfvektor. Ohne Server, ohne Netz.
//
// Dieselben Zahlen rechnen Go (pkg/crypt/vektor_test.go), Kotlin
// (FunkVektorTest.kt) und die Weboberfläche (funk-kern.test.js) nach. Ergibt
// eine der vier Sprachen etwas anderes, reden Server und Gerät am Spieltag
// aneinander vorbei — und zwar lautlos, weil eine misslungene Entschlüsselung
// aussieht wie eine leere Antwort.
// ---------------------------------------------------------------------------

func hex(_ text: String) -> Data {
    var roh = Data()
    var i = text.startIndex
    while i < text.endIndex {
        let j = text.index(i, offsetBy: 2)
        roh.append(UInt8(text[i..<j], radix: 16) ?? 0)
        i = j
    }
    return roh
}

func hexVon(_ roh: Data) -> String {
    roh.map { String(format: "%02x", $0) }.joined()
}

print("Prüfvektor — testdaten/lagefunk.json")

let vektorPfad = URL(fileURLWithPath: #filePath)
    .deletingLastPathComponent()   // Sources/Funkprobe
    .deletingLastPathComponent()   // Sources
    .deletingLastPathComponent()   // Funkprobe
    .deletingLastPathComponent()   // ios
    .deletingLastPathComponent()   // Wurzel des Projekts
    .appendingPathComponent("testdaten/lagefunk.json")

guard let vektorRoh = try? Data(contentsOf: vektorPfad),
      let vektor = try? JSONSerialization.jsonObject(with: vektorRoh) as? [String: String]
else {
    print("  FEHL Prüfvektor nicht lesbar: \(vektorPfad.path)")
    exit(1)
}

func feld(_ name: String) -> String { vektor[name] ?? "" }

// Die Ableitung: dieselben Bytes wie im Server?
let abgeleitet = Funk.ableiten(
    gemeinsam: hex(feld("gemeinsamesGeheimnisHex")),
    eigenB64: feld("clientPublicKey"),
    serverB64: feld("serverPublicKey")
)
let abgeleitetHex = abgeleitet.withUnsafeBytes { hexVon(Data($0)) }
pruefe("Schlüsselableitung (HKDF)", abgeleitetHex == feld("schluesselHex"),
       "\n       war: \(feld("schluesselHex"))\n       ist: \(abgeleitetHex)")

// Und die Nachricht aus dem Vektor muss aufgehen.
let ausVektor = Funk(schluessel: abgeleitet,
                     publicKey: feld("clientPublicKey"),
                     fingerprint: "PRUEF-VEKTOR")

if let klar = ausVektor.oeffnen(hex(feld("chiffreHex")),
                                nonce: hex(feld("nonceHex")),
                                beiwerk: Data(feld("beiwerk").utf8)) {
    pruefe("Nachricht des Servers geöffnet",
           String(data: klar, encoding: .utf8) == feld("klartext"))
} else {
    pruefe("Nachricht des Servers geöffnet", false, "sie ließ sich nicht öffnen")
}

pruefe("Falsches Beiwerk öffnet nicht",
       ausVektor.oeffnen(hex(feld("chiffreHex")),
                         nonce: hex(feld("nonceHex")),
                         beiwerk: Data("GET /woanders 7".utf8)) == nil)

pruefe("Beiwerk hat die Form des Servers",
       String(data: Funk.beiwerk(methode: "POST",
                                 pfad: "/api/opx/position?seit=3",
                                 nummer: 7), encoding: .utf8) == feld("beiwerk"))

// ---------------------------------------------------------------------------
// Teil 2: gegen einen laufenden Server. Nur, wenn einer antwortet.
// ---------------------------------------------------------------------------

print("")
print("Prüfstand Lagefunk — \(basis)")

// 1. Der Schlüssel des Servers.
guard let (_, keyRoh) = await hole("/api/opx/key", funk: nil),
      let schluesselInfo = try? JSONSerialization.jsonObject(with: keyRoh) as? [String: Any],
      let serverKey = schluesselInfo["publicKey"] as? String,
      let kennzeichen = schluesselInfo["fingerprint"] as? String else {
    // Kein Server, kein Beinbruch: Der Vektor oben ist der Teil, der ohne
    // Aufbau auskommt, und er ist gelaufen.
    print("  (kein Server erreichbar — der Teil mit dem Handschlag entfällt)")
    print(fehler == 0 ? "\nAlles durch, was ohne Server geht." : "\n\(fehler) Fehler.")
    exit(fehler == 0 ? 0 : 1)
}
print("  Kennzeichen des Servers: \(kennzeichen)")

// 2. Handschlag mit der echten Funk.swift.
guard let funk = Funk.aufbauen(serverPublicKey: serverKey, fingerprint: kennzeichen) else {
    print("  FEHL Handschlag gescheitert")
    exit(1)
}
pruefe("Handschlag (ECDH P-256 + HKDF)", true)

// 3. Eine verschlüsselte Anfrage, deren Antwort wieder aufgeht.
if let (code, klar) = await hole("/api/opx/status", funk: funk) {
    let text = String(data: klar, encoding: .utf8) ?? ""
    pruefe("Antwort entschlüsselt (\(code))", code == 200 && text.contains("{"), text.prefix(60).description)
} else {
    pruefe("Antwort entschlüsselt", false)
}

// 4. Mit Abfrageteil. Genau hier ist es im Browser schon einmal auseinander-
//    gelaufen: Wer nur den Pfad versiegelt, scheitert ab dem ersten "?".
if let (_, klar) = await hole("/api/opx/status?probe=1", funk: funk) {
    pruefe("Antwort mit Abfrageteil", !klar.isEmpty)
} else {
    pruefe("Antwort mit Abfrageteil", false)
}

// 5. Mehrere Nummern hintereinander – das Fenster gegen Wiedereinspielen
//    darf laufende Anfragen nicht abweisen.
var alleDurch = true
for _ in 0..<5 {
    if let (_, klar) = await hole("/api/opx/status", funk: funk), !klar.isEmpty { continue }
    alleDurch = false
}
pruefe("Fünf Anfragen nacheinander", alleDurch)

// 6. Ein zweiter Client bekommt einen eigenen Schlüssel.
if let zweiter = Funk.aufbauen(serverPublicKey: serverKey, fingerprint: kennzeichen) {
    pruefe("Zweiter Client, eigener Schlüssel", zweiter.publicKey != funk.publicKey)
    if let (_, klar) = await hole("/api/opx/status", funk: zweiter) {
        pruefe("Zweiter Client kommt durch", !klar.isEmpty)
    }
}

// 7. Der falsche Schlüssel muss sauber scheitern.
//
// Genau darauf verlässt sich die App: Schlägt das Öffnen fehl, wirft sie den
// Schlüssel weg und handelt neu aus. Käme statt "nichts" ein Absturz oder
// zufälliger Inhalt heraus, wäre dieser Weg nicht gangbar.
if let anderer = Funk.aufbauen(serverPublicKey: serverKey, fingerprint: kennzeichen) {
    let nummer = funk.naechsteNummer()
    let beiwerk = Funk.beiwerk(methode: "GET", pfad: "/api/opx/status", nummer: nummer)
    if let (chiffre, nonce) = funk.sealen(Data("Lagemeldung".utf8), beiwerk: beiwerk) {
        pruefe("Fremder Schlüssel öffnet nicht",
               anderer.oeffnen(chiffre, nonce: nonce, beiwerk: beiwerk) == nil)
        pruefe("Eigener Schlüssel öffnet",
               funk.oeffnen(chiffre, nonce: nonce, beiwerk: beiwerk)
                   .flatMap { String(data: $0, encoding: .utf8) } == "Lagemeldung")

        // Auch das Beiwerk muss stimmen: Wer die Nummer verdreht, kommt nicht
        // durch – das ist der Schutz gegen Wiedereinspielen.
        let falsches = Funk.beiwerk(methode: "GET", pfad: "/api/opx/status", nummer: nummer + 1)
        pruefe("Falsches Beiwerk öffnet nicht",
               funk.oeffnen(chiffre, nonce: nonce, beiwerk: falsches) == nil)
    }
}

print(fehler == 0 ? "\nAlles durch." : "\n\(fehler) Fehler.")
exit(fehler == 0 ? 0 : 1)
