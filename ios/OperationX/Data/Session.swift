import Foundation

/// Was das Telefon zwischen zwei Starts behält – und wie lange.
///
/// Die Zusage des Konzepts lautet: Zugangsdaten und Ortsbezogenes überleben
/// das Spiel nicht. Deshalb hat hier alles eine Verfallszeit, und abgelaufene
/// Einträge werden beim ersten Zugriff gelöscht, nicht erst beim Aufräumen.
///
/// Absichtlich UserDefaults und nicht der Schlüsselbund: Der Schlüsselbund
/// überlebt die Deinstallation der App, und genau das soll dieser Zugang
/// nicht. Er gilt einen Spieltag.
final class Session {

    static let shared = Session()

    private let d = UserDefaults.standard

    private enum Feld {
        static let server = "opx.server"
        static let token = "opx.token"
        static let verfall = "opx.verfall"
        static let onboarded = "opx.onboarded"
        static let puffer = "opx.puffer"
        static let bekannt = "opx.bekannt"
    }

    /// Wie lange eine Anmeldung gilt, wenn der Server nichts anderes sagt.
    private let stunden: TimeInterval = 24 * 3600

    private init() { abgelaufenesAufraeumen() }

    // MARK: - Server und Anmeldung

    var server: String? {
        get { abgelaufenesAufraeumen(); return d.string(forKey: Feld.server) }
        set { d.set(newValue, forKey: Feld.server) }
    }

    var token: String? {
        get { abgelaufenesAufraeumen(); return d.string(forKey: Feld.token) }
    }

    func anmelden(token: String, gueltigBis: Date?) {
        d.set(token, forKey: Feld.token)
        d.set((gueltigBis ?? Date().addingTimeInterval(stunden)).timeIntervalSince1970,
              forKey: Feld.verfall)
    }

    func abmelden() {
        d.removeObject(forKey: Feld.token)
        d.removeObject(forKey: Feld.verfall)
        d.removeObject(forKey: Feld.puffer)
    }

    /// Serverwechsel: Zugang und Puffer gehören zum alten Server.
    func serverWechseln() {
        abmelden()
        d.removeObject(forKey: Feld.server)
    }

    var onboarded: Bool {
        get { d.bool(forKey: Feld.onboarded) }
        set { d.set(newValue, forKey: Feld.onboarded) }
    }

    // MARK: - Server, mit denen dieses Gerät schon gesprochen hat

    /// Die Kennzeichen bekannter Server.
    ///
    /// Anders als der Zugang verfällt diese Liste nicht: Sie ist der Grund,
    /// warum die App merkt, wenn ein Server unter derselben Adresse plötzlich
    /// einen anderen Schlüssel hat. Gespeichert wird nur Adresse und
    /// Kennzeichen — nichts, was ein Spiel verrät.
    var bekannteServer: [BekannterServer] {
        guard let roh = d.data(forKey: Feld.bekannt),
              let liste = try? JSONDecoder().decode([BekannterServer].self, from: roh)
        else { return [] }
        return liste
    }

    func kennzeichenVon(_ adresse: String) -> String? {
        bekannteServer.first { $0.adresse == adresse }?.kennzeichen
    }

    func merkeServer(_ adresse: String, kennzeichen: String) {
        guard !kennzeichen.isEmpty else { return }
        var liste = bekannteServer.filter { $0.adresse != adresse }
        liste.insert(BekannterServer(adresse: adresse, kennzeichen: kennzeichen,
                                     zuletzt: Date().timeIntervalSince1970), at: 0)
        // Acht reichen. Wer mehr Server im Kopf hat als eine Klassenfahrt
        // Stationen, tippt die Adresse auch noch einmal ab.
        if liste.count > 8 { liste.removeLast(liste.count - 8) }
        if let roh = try? JSONEncoder().encode(liste) { d.set(roh, forKey: Feld.bekannt) }
    }

    /// Nach einem Serverwechsel, der begründet war: das alte Kennzeichen
    /// vergessen, damit die Warnung nicht ewig bleibt.
    func vergissServer(_ adresse: String) {
        let liste = bekannteServer.filter { $0.adresse != adresse }
        if let roh = try? JSONEncoder().encode(liste) { d.set(roh, forKey: Feld.bekannt) }
    }

    private func abgelaufenesAufraeumen() {
        let verfall = d.double(forKey: Feld.verfall)
        guard verfall > 0, Date().timeIntervalSince1970 > verfall else { return }
        abmelden()
    }

    // MARK: - Puffer für das Funkloch

    /// Meldungen, die noch nicht durchkamen.
    ///
    /// In Bahn, Hinterhof und Tiefgarage kommt nichts durch. Jede Meldung
    /// liegt deshalb zuerst hier und wird beim nächsten Netzkontakt
    /// nachgereicht – mit ihrem ursprünglichen Zeitstempel. Der Server
    /// bewertet die Frist nach dem Erfassungszeitpunkt, nicht nach dem
    /// Eingang; deshalb ist Nachreichen fair und kein Schummeln.
    var puffer: [PositionReport] {
        guard let roh = d.data(forKey: Feld.puffer),
              let liste = try? JSONDecoder().decode([GepufferteMeldung].self, from: roh)
        else { return [] }
        return liste.map { $0.bericht }
    }

    func puffern(_ bericht: PositionReport) {
        var liste = puffer
        liste.append(bericht)
        // Ein Puffer, der unbegrenzt wächst, verschickt nach zwei Stunden
        // Funkloch tausend Meldungen auf einmal. Die ältesten fallen zuerst –
        // sie sind ohnehin überholt.
        if liste.count > 200 { liste.removeFirst(liste.count - 200) }
        pufferSetzen(liste)
    }

    func pufferLeeren() { d.removeObject(forKey: Feld.puffer) }

    func pufferSetzen(_ liste: [PositionReport]) {
        let verpackt = liste.map { GepufferteMeldung(bericht: $0) }
        if let roh = try? JSONEncoder().encode(verpackt) {
            d.set(roh, forKey: Feld.puffer)
        }
    }

    /// PositionReport ist nur Encodable – für den Puffer braucht es beides.
    private struct GepufferteMeldung: Codable {
        var lat: Double
        var lng: Double
        var accuracy: Double
        var speed: Double
        var heading: Double
        var capturedAt: String
        var mocked: Bool

        init(bericht: PositionReport) {
            lat = bericht.lat
            lng = bericht.lng
            accuracy = bericht.accuracy
            speed = bericht.speed
            heading = bericht.heading
            capturedAt = bericht.capturedAt
            mocked = bericht.mocked
        }

        var bericht: PositionReport {
            PositionReport(lat: lat, lng: lng, accuracy: accuracy, speed: speed,
                           heading: heading, capturedAt: capturedAt, mocked: mocked)
        }
    }
}

/// Ein Server, den dieses Gerät schon einmal gesehen hat.
struct BekannterServer: Codable, Identifiable {
    var adresse: String
    var kennzeichen: String
    var zuletzt: Double

    var id: String { adresse }
}

/// Zeitstempel im Format, das der Server erwartet (RFC 3339 in UTC).
enum Zeit {
    static let iso: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime]
        f.timeZone = TimeZone(secondsFromGMT: 0)
        return f
    }()

    static func stempel(_ datum: Date = Date()) -> String { iso.string(from: datum) }

    static func lesen(_ text: String?) -> Date? {
        guard let text, !text.isEmpty else { return nil }
        if let d = iso.date(from: text) { return d }

        // Der Server schickt Zeiten gelegentlich mit Sekundenbruchteilen.
        let mitBruch = ISO8601DateFormatter()
        mitBruch.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return mitBruch.date(from: text)
    }

    /// Ortszeit zum Vorlesen: "14:37". Nicht UTC – wer auf die Uhr sieht,
    /// vergleicht mit seiner eigenen.
    static func uhrzeit(_ text: String?) -> String {
        guard let d = lesen(text) else { return "" }
        let f = DateFormatter()
        f.dateFormat = "HH:mm"
        f.timeZone = .current
        return f.string(from: d)
    }

    /// "7:34" aus einer Sekundenzahl, für die Fristanzeige.
    static func uhr(_ sekunden: Int) -> String {
        let s = max(0, sekunden)
        return String(format: "%d:%02d", s / 60, s % 60)
    }
}
