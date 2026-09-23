import Foundation

/// Fehler, die man einem Menschen zeigen kann.
struct ApiFehler: LocalizedError {
    let text: String
    var status: Int = 0
    var errorDescription: String? { text }
}

/// Der Zugang zum Spielserver.
///
/// Eine Klasse für alles, was über das Netz geht – dieselbe Schnittstelle wie
/// in der Android-App, dieselben Pfade, dieselben Antworten. Wer hier etwas
/// hinzufügt, hat es dort auch schon.
///
/// Zwei Dinge macht sie nebenbei und ungefragt: Sie legt die zweite
/// Verschlüsselung an, sobald der Server einen Schlüssel anbietet, und sie
/// gibt Fehler auf Deutsch zurück. "Kein Kontakt zum Einsatzserver" ist am
/// Spieltag die häufigste Meldung überhaupt, und sie muss verständlich sein:
/// Wer zwischen zwei Häuserzeilen steht, hat nichts falsch gemacht.
@MainActor
final class Api {

    static let shared = Api()

    private let sitzung: URLSession
    private let dekoder = JSONDecoder()

    private var funk: Funk?
    /// Zu welchem Server der Funk aufgebaut wurde.
    private var funkFuer = ""
    /// Wann zuletzt versucht wurde, den Schlüssel zu holen.
    private var letzterVersuch = Date.distantPast

    var fingerprint: String { funk?.fingerprint ?? "" }

    private init() {
        let k = URLSessionConfiguration.default
        // Unterwegs ist die Verbindung oft schlecht, aber selten tot. Lieber
        // warten als abbrechen – der Puffer schickt sonst gleich noch einmal.
        k.timeoutIntervalForRequest = 20
        k.timeoutIntervalForResource = 60
        k.waitsForConnectivity = false
        sitzung = URLSession(configuration: k)
    }

    // MARK: - Verschlüsselung aufbauen

    /// Holt den öffentlichen Schlüssel des Servers und baut den Funk auf.
    ///
    /// Scheitert das, läuft alles weiter – dann eben nur mit der
    /// Verschlüsselung des Tunnels. Ein Spiel darf daran nicht scheitern.
    /// Mit `neu` wird der gespeicherte Schlüssel weggeworfen und frisch
    /// ausgehandelt. Das ist der Fall, in dem jemand ausdrücklich fragt, wer
    /// da eigentlich antwortet — und dann wäre das Kennzeichen von vorhin die
    /// falsche Auskunft.
    @discardableResult
    func funkAufbauen(_ basis: String, neu: Bool = false) async -> Funk? {
        if !neu, let f = funk, funkFuer == basis { return f }

        // Anderer Server, anderer Schlüssel: Ein Sitzungsschlüssel gilt immer
        // nur für das Paar, aus dem er entstanden ist.
        if neu || funkFuer != basis {
            funk = nil
            letzterVersuch = .distantPast
        }
        // Ein Server ohne Verschlüsselung würde sonst bei jeder Anfrage noch
        // einmal gefragt. Alle halbe Minute reicht.
        guard Date().timeIntervalSince(letzterVersuch) > 30 else { return nil }
        funkFuer = basis
        letzterVersuch = Date()

        guard let url = URL(string: basis + "/api/opx/key") else { return nil }
        guard let (daten, _) = try? await sitzung.data(from: url),
              let schluessel = try? dekoder.decode(ServerKey.self, from: daten),
              schluessel.available == true,
              let oeffentlich = schluessel.publicKey, !oeffentlich.isEmpty
        else { return nil }

        funk = Funk.aufbauen(serverPublicKey: oeffentlich,
                             fingerprint: schluessel.fingerprint ?? "")
        return funk
    }

    func funkZuruecksetzen() {
        funk = nil
        funkFuer = ""
        letzterVersuch = .distantPast
    }

    /// Das Kennzeichen eines Servers, ohne sich anzumelden.
    func kennzeichenVon(_ basis: String) async -> String? {
        await funkAufbauen(basis, neu: true)?.fingerprint
    }

    // MARK: - Ohne Anmeldung

    func status(_ basis: String) async throws -> ServerStatus {
        try await holen(basis: basis, methode: "GET", pfad: "/api/opx/status", token: nil)
    }

    func login(basis: String, callsign: String, password: String) async throws -> (String, Me) {
        struct Anfrage: Encodable { let identity: String; let password: String }

        let auth: AuthResponse = try await holen(
            basis: basis, methode: "POST",
            pfad: "/api/collections/teams/auth-with-password",
            token: nil,
            rumpf: try JSONEncoder().encode(Anfrage(identity: callsign, password: password))
        )

        guard let token = auth.token, !token.isEmpty else {
            throw ApiFehler(text: "Rufzeichen oder Kennwort stimmen nicht.", status: 400)
        }

        let me: Me = try await holen(basis: basis, methode: "GET",
                                     pfad: "/api/opx/me", token: token)
        return (token, me)
    }

    // MARK: - Mit Anmeldung

    func me(_ s: Zugang) async throws -> Me { try await get(s, "/api/opx/me") }
    func live(_ s: Zugang) async throws -> LiveState { try await get(s, "/api/opx/live") }
    func map(_ s: Zugang) async throws -> FieldMap { try await get(s, "/api/opx/map") }
    func intel(_ s: Zugang) async throws -> IntelList { try await get(s, "/api/opx/intel") }
    func puzzles(_ s: Zugang) async throws -> PuzzleList { try await get(s, "/api/opx/puzzles") }
    func jokers(_ s: Zugang) async throws -> JokerList { try await get(s, "/api/opx/jokers") }
    func radio(_ s: Zugang) async throws -> RadioList { try await get(s, "/api/opx/radio") }
    func mission(_ s: Zugang) async throws -> MissionState { try await get(s, "/api/opx/mission") }
    func rules(_ s: Zugang) async throws -> RuleBook { try await get(s, "/api/opx/rules") }
    func ledger(_ s: Zugang) async throws -> LedgerBook { try await get(s, "/api/opx/ledger") }

    func sendPositions(_ s: Zugang, _ berichte: [PositionReport]) async throws -> PositionAck {
        struct Anfrage: Encodable { let positions: [PositionReport] }
        return try await post(s, "/api/opx/position", Anfrage(positions: berichte))
    }

    func setTransit(_ s: Zugang, aktiv: Bool) async throws {
        struct Anfrage: Encodable { let active: Bool }
        let _: LeereAntwort = try await post(s, "/api/opx/transit", Anfrage(active: aktiv))
    }

    func solvePuzzle(_ s: Zugang, id: String, answer: String) async throws -> SolveResult {
        struct Anfrage: Encodable { let answer: String }
        return try await post(s, "/api/opx/puzzles/\(id)/solve", Anfrage(answer: answer))
    }

    func chooseOption(_ s: Zugang, optionId: String) async throws -> MissionState {
        struct Anfrage: Encodable { let optionId: String }
        return try await post(s, "/api/opx/mission/choose", Anfrage(optionId: optionId))
    }

    func submitPasscode(_ s: Zugang, code: String) async throws {
        struct Anfrage: Encodable { let passcode: String }
        let _: LeereAntwort = try await post(s, "/api/opx/mission/evidence",
                                             Anfrage(passcode: code))
    }

    func useJoker(_ s: Zugang, kind: String,
                  sectorId: String? = nil, hotspotId: String? = nil,
                  puzzleId: String? = nil) async throws -> JokerErgebnis {
        struct Anfrage: Encodable {
            let kind: String
            let sectorId: String?
            let hotspotId: String?
            let puzzleId: String?
        }
        return try await post(s, "/api/opx/jokers",
                              Anfrage(kind: kind, sectorId: sectorId,
                                      hotspotId: hotspotId, puzzleId: puzzleId))
    }

    func postRadio(_ s: Zugang, text: String) async throws {
        struct Anfrage: Encodable { let text: String }
        let _: LeereAntwort = try await post(s, "/api/opx/radio", Anfrage(text: text))
    }

    func reportSighting(_ s: Zugang) async throws -> SightingResult {
        try await post(s, "/api/opx/sighting", LeereAnfrage())
    }

    func arrest(_ s: Zugang, level: Int, hotspotNumber: Int,
                claimedTime: String?, claimedTarget: Int?) async throws -> ArrestResult {
        struct Anfrage: Encodable {
            let level: Int
            let hotspotNumber: Int
            let claimedTime: String?
            let claimedTarget: Int?
        }
        return try await post(s, "/api/opx/arrest",
                              Anfrage(level: level, hotspotNumber: hotspotNumber,
                                      claimedTime: claimedTime, claimedTarget: claimedTarget))
    }

    func reportDelay(_ s: Zugang, reason: String) async throws -> DelayResult {
        struct Anfrage: Encodable { let reason: String }
        return try await post(s, "/api/opx/mission/delay", Anfrage(reason: reason))
    }

    func markOnboarded(_ s: Zugang) async throws {
        let _: LeereAntwort = try await post(s, "/api/opx/onboarded", LeereAnfrage())
    }

    /// Lädt ein Beweisfoto hoch.
    ///
    /// Mehrteiliger Rumpf, von Hand gebaut: Die Trennmarke muss auch dann noch
    /// im Inhaltstyp stehen, wenn der ganze Rumpf verschlüsselt übertragen wird
    /// – deshalb reicht der Funk den ursprünglichen Typ in einer eigenen
    /// Kopfzeile mit.
    func uploadEvidence(_ s: Zugang, bild: Data, name: String = "beweis.jpg") async throws {
        let marke = "opx-\(UUID().uuidString)"
        var rumpf = Data()

        func anfuegen(_ text: String) { rumpf.append(Data(text.utf8)) }

        anfuegen("--\(marke)\r\n")
        anfuegen("Content-Disposition: form-data; name=\"photo\"; filename=\"\(name)\"\r\n")
        anfuegen("Content-Type: image/jpeg\r\n\r\n")
        rumpf.append(bild)
        anfuegen("\r\n--\(marke)--\r\n")

        let _: LeereAntwort = try await anfragen(
            basis: s.basis, methode: "POST", pfad: "/api/opx/mission/evidence",
            token: s.token, rumpf: rumpf,
            inhaltstyp: "multipart/form-data; boundary=\(marke)"
        )
    }

    // MARK: - Innenleben

    struct Zugang {
        let basis: String
        let token: String
    }

    struct LeereAnfrage: Encodable {}
    struct LeereAntwort: Decodable {}

    /// Die Antwort auf einen Jokereinsatz ist je Joker verschieden; gebraucht
    /// wird nur, was man vorlesen kann.
    struct JokerErgebnis: Decodable {
        var message: String?
        var text: String?
        var blocked: Bool?
        var inside: Bool?
        var fp: Int?
    }

    private func get<T: Decodable>(_ s: Zugang, _ pfad: String) async throws -> T {
        try await holen(basis: s.basis, methode: "GET", pfad: pfad, token: s.token)
    }

    private func post<T: Decodable, R: Encodable>(_ s: Zugang, _ pfad: String,
                                                  _ rumpf: R) async throws -> T {
        try await holen(basis: s.basis, methode: "POST", pfad: pfad, token: s.token,
                        rumpf: try JSONEncoder().encode(rumpf))
    }

    private func holen<T: Decodable>(basis: String, methode: String, pfad: String,
                                     token: String?, rumpf: Data? = nil) async throws -> T {
        try await anfragen(basis: basis, methode: methode, pfad: pfad, token: token,
                           rumpf: rumpf, inhaltstyp: "application/json")
    }

    /// Eine Anfrage, verschlüsselt, wenn der Funk steht.
    private func anfragen<T: Decodable>(basis: String, methode: String, pfad: String,
                                        token: String?, rumpf: Data?,
                                        inhaltstyp: String) async throws -> T {
        guard let url = URL(string: basis + pfad) else {
            throw ApiFehler(text: "Die Serveradresse ist unbrauchbar.")
        }

        var anfrage = URLRequest(url: url)
        anfrage.httpMethod = methode
        if let token { anfrage.setValue(token, forHTTPHeaderField: "Authorization") }

        // Den Schlüssel selbst holt die App im Klartext – er ist öffentlich,
        // und ohne ihn gäbe es keine Verschlüsselung, mit der man ihn holen
        // könnte.
        let f: Funk? = pfad == "/api/opx/key" ? nil : await funkAufbauen(basis)

        var beiwerk = Data()

        if let f {
            let nummer = f.naechsteNummer()
            beiwerk = Funk.beiwerk(methode: methode, pfad: pfad, nummer: nummer)

            anfrage.setValue(f.publicKey, forHTTPHeaderField: "X-Opx-Key")
            anfrage.setValue(String(nummer), forHTTPHeaderField: "X-Opx-Seq")

            if let klartext = rumpf {
                guard let (chiffre, nonce) = f.sealen(klartext, beiwerk: beiwerk) else {
                    throw ApiFehler(text: "Die Nachricht ließ sich nicht verschlüsseln.")
                }
                anfrage.setValue(Funk.verschluesselnB64(nonce), forHTTPHeaderField: "X-Opx-Nonce")
                anfrage.setValue(inhaltstyp, forHTTPHeaderField: "X-Opx-Type")
                anfrage.setValue("application/octet-stream", forHTTPHeaderField: "Content-Type")
                anfrage.httpBody = chiffre
            }
        } else if let klartext = rumpf {
            anfrage.setValue(inhaltstyp, forHTTPHeaderField: "Content-Type")
            anfrage.httpBody = klartext
        }

        let daten: Data
        let antwort: URLResponse
        do {
            (daten, antwort) = try await sitzung.data(for: anfrage)
        } catch {
            // Am Spieltag die häufigste Störung überhaupt. Ohne diesen Zweig
            // stünde hier eine englische Systemmeldung, und wer zwischen zwei
            // Häuserzeilen steht, soll erkennen, dass er nichts falsch gemacht
            // hat.
            throw ApiFehler(text: "Kein Kontakt zum Einsatzserver. Empfang prüfen.", status: 0)
        }

        let http = antwort as? HTTPURLResponse
        let code = http?.statusCode ?? 0

        // Ausgehend entschlüsseln, wenn der Server versiegelt geantwortet hat.
        var klartext = daten
        if let f, let nonceText = http?.value(forHTTPHeaderField: "X-Opx-Nonce"),
           let nonce = Funk.entschluesselnB64(nonceText) {

            // Lässt sich die Antwort nicht öffnen, passt der Schlüssel nicht
            // mehr zu diesem Server – neu aufgesetzt, oder es ist nicht mehr
            // derselbe. Vorher lief hier das Chiffrat weiter in den Dekoder
            // und kam als unverständlicher Formatfehler heraus; und der
            // Schlüssel blieb, was ihn bis zum nächsten Start unbrauchbar
            // hielt.
            guard let geoeffnet = f.oeffnen(daten, nonce: nonce, beiwerk: beiwerk) else {
                funkZuruecksetzen()
                throw ApiFehler(
                    text: "Die Antwort des Servers ließ sich nicht entschlüsseln. "
                        + "Der Schlüssel wird neu ausgehandelt.",
                    status: code)
            }
            klartext = geoeffnet
        }

        guard (200..<300).contains(code) else {
            throw ApiFehler(text: meldung(aus: klartext, code: code), status: code)
        }

        if T.self == LeereAntwort.self, let leer = LeereAntwort() as? T {
            return leer
        }

        do {
            return try dekoder.decode(T.self, from: klartext)
        } catch {
            throw ApiFehler(text: "Die Antwort des Servers war nicht lesbar.", status: code)
        }
    }

    /// Fehler des Servers tragen ihren Text im Feld "message".
    private func meldung(aus daten: Data, code: Int) -> String {
        struct Fehler: Decodable { var message: String? }
        if let f = try? dekoder.decode(Fehler.self, from: daten),
           let text = f.message, !text.isEmpty {
            return text
        }
        if code == 401 { return "Die Anmeldung gilt nicht mehr." }
        return "Der Server antwortete mit \(code)."
    }
}
