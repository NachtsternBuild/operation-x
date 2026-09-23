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

print("Prüfstand Lagefunk — \(basis)")

// 1. Der Schlüssel des Servers.
guard let (_, keyRoh) = await hole("/api/opx/key", funk: nil),
      let schluesselInfo = try? JSONSerialization.jsonObject(with: keyRoh) as? [String: Any],
      let serverKey = schluesselInfo["publicKey"] as? String,
      let kennzeichen = schluesselInfo["fingerprint"] as? String else {
    print("  FEHL Server nicht erreichbar oder ohne Verschlüsselung")
    exit(1)
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
