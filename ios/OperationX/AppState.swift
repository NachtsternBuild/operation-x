import Foundation
import SwiftUI

/// Der Zustand der ganzen App an einer Stelle.
///
/// Dasselbe Muster wie in der Android-Fassung: Die Ansichten zeigen an, dieser
/// Zustand holt und schreibt. Wer wissen will, was die App kann, liest die
/// Methoden hier und braucht keine einzige Ansicht dafür.
@MainActor
final class AppState: ObservableObject {

    enum Stufe { case beitritt, anmeldung, feld }

    @Published var stufe: Stufe = .beitritt

    @Published var serverUrl = ""
    @Published var serverName = ""

    @Published var me: Me?
    @Published var live: LiveState?
    @Published var feld: FieldMap?
    @Published var intel: [IntelItem] = []
    @Published var raetsel: [Puzzle] = []
    @Published var mittel: [Joker] = []
    @Published var funkKanal: [RadioMessage] = []
    @Published var mission: MissionState?
    @Published var regeln: [RuleGroup] = []
    @Published var konto: LedgerBook?

    @Published var busy = false
    @Published var fehler: String?
    @Published var meldung: String?
    @Published var einweisungZeigen = false

    /// Steht hier etwas, hat ein bekannter Server ein neues Kennzeichen.
    @Published var kennzeichenWarnung: String?
    /// Server, die dieses Gerät schon kennt — zum Antippen statt Abtippen.
    @Published var bekannteServer: [BekannterServer] = []

    /// Die Ortung. Die Oberfläche beobachtet sie selbst – deshalb reicht der
    /// Einstieg sie zusätzlich als eigenes Umgebungsobjekt herum.
    let ortung = LocationService()

    private var takt: Task<Void, Never>?

    var istZielperson: Bool { me?.role == "misterx" }
    var istFahndung: Bool { me?.role == "detective" }
    var fingerprint: String { Api.shared.fingerprint }

    private var zugang: Api.Zugang? {
        guard let token = Session.shared.token, !serverUrl.isEmpty else { return nil }
        return Api.Zugang(basis: serverUrl, token: token)
    }

    init() {
        ortung.zugang = { [weak self] in self?.zugang }
        ortung.quittung = { [weak self] ack in
            // Die Frist kommt vom Server; die Anzeige zieht beim nächsten
            // Durchlauf nach. Ohne diesen Weg stünde nach einer Meldung
            // weiterhin die alte Zeit da.
            guard let self else { return }
            if let naechste = ack.nextDueAt, !naechste.isEmpty {
                Task { await self.auffrischen() }
            }
        }

        bekannteServer = Session.shared.bekannteServer

        // Eine gespeicherte Sitzung wiederaufnehmen.
        if let server = Session.shared.server {
            serverUrl = server
            if Session.shared.token != nil {
                stufe = .feld
                Task { await auffrischen() }
                taktStarten()
            } else {
                stufe = .anmeldung
                Task { await zustandHolen() }
            }
        }
    }

    // MARK: - Beitritt und Anmeldung

    func verbinden(_ adresse: String) async {
        let sauber = Adressen.normalisieren(adresse)
        guard !sauber.isEmpty else {
            fehler = "Diese Adresse ergibt keinen Sinn."
            return
        }

        busy = true
        fehler = nil
        defer { busy = false }

        do {
            let status = try await Api.shared.status(sauber)
            serverUrl = sauber
            Session.shared.server = sauber
            serverName = Self.serverName(status)
            // Das Kennzeichen ist die einzige Stelle, an der sich ein
            // untergeschobener Server verrät. Wer diese Adresse schon einmal
            // benutzt hat, bekommt es gesagt, wenn es ein anderes ist.
            let jetzt = await Api.shared.funkAufbauen(sauber)?.fingerprint ?? ""
            let frueher = Session.shared.kennzeichenVon(sauber)
            kennzeichenWarnung = nil
            if let frueher, !jetzt.isEmpty, frueher != jetzt {
                kennzeichenWarnung = "Dieser Server hatte zuletzt das Kennzeichen "
                    + "\(frueher), jetzt \(jetzt). Entweder wurde er neu aufgesetzt — "
                    + "oder es ist nicht derselbe. Vergleicht die Zeichen mit der Teamkarte."
            }

            stufe = .anmeldung
        } catch {
            fehler = (error as? ApiFehler)?.text ?? "Der Server antwortet nicht."
        }
    }

    private func zustandHolen() async {
        guard !serverUrl.isEmpty,
              let status = try? await Api.shared.status(serverUrl) else { return }
        serverName = Self.serverName(status)
    }

    /// Die Zeile unter der Überschrift auf der Anmeldeseite.
    ///
    /// Nennt der Server kein Spiel, steht hier ein neutraler Satz statt eines
    /// einsamen Trennpunkts. Das kommt vor: Ein Server, der noch nicht
    /// eingerichtet ist, nennt keines – und einer, der mehrere führt, nennt
    /// mit Absicht keines, weil die Namen niemanden etwas angehen, der noch
    /// nichts vorgewiesen hat.
    static func serverName(_ status: ServerStatus) -> String {
        let name = (status.gameName ?? "").trimmingCharacters(in: .whitespaces)
        let stadt = (status.city ?? "").trimmingCharacters(in: .whitespaces)
        if !name.isEmpty && !stadt.isEmpty { return "\(name) · \(stadt)" }
        if !name.isEmpty { return name }
        return "Rufzeichen von der Teamkarte"
    }

    func anmelden(callsign: String, kennwort: String) async {
        busy = true
        fehler = nil
        defer { busy = false }

        do {
            let (token, ich) = try await Api.shared.login(
                basis: serverUrl, callsign: callsign, password: kennwort)

            Session.shared.anmelden(token: token, gueltigBis: spielende(ich))
            // Erst jetzt merken: Vorher ist nicht belegt, dass hinter der
            // Adresse wirklich unser Spiel steckt.
            Session.shared.merkeServer(serverUrl, kennzeichen: fingerprint)
            bekannteServer = Session.shared.bekannteServer
            kennzeichenWarnung = nil
            me = ich
            stufe = .feld
            einweisungZeigen = (ich.onboarded != true)

            await auffrischen()
            taktStarten()
        } catch {
            fehler = (error as? ApiFehler)?.text ?? "Anmeldung fehlgeschlagen."
        }
    }

    /// Die Anmeldung soll nicht länger gelten als das Spiel – plus etwas Luft
    /// für die Nachbesprechung.
    private func spielende(_ ich: Me) -> Date? {
        guard let ende = Zeit.lesen(ich.game?.endsAt) else { return nil }
        return ende.addingTimeInterval(6 * 3600)
    }

    func abmelden() {
        taktBeenden()
        ortung.anhalten()
        Session.shared.abmelden()
        me = nil
        live = nil
        feld = nil
        mission = nil
        stufe = .anmeldung
    }

    func serverWechseln() {
        taktBeenden()
        ortung.anhalten()
        Session.shared.serverWechseln()
        Api.shared.funkZuruecksetzen()
        kennzeichenWarnung = nil
        serverUrl = ""
        serverName = ""
        me = nil
        stufe = .beitritt
    }

    // MARK: - Auffrischen

    /// Solange die App vorn ist, holt sie im Takt nach.
    ///
    /// Die Android-Fassung hält dafür eine offene Verbindung (SSE) und
    /// erfährt jede Änderung in unter einer Sekunde. Hier wird zunächst
    /// gefragt – einfacher, robuster, und der Unterschied fällt nur beim
    /// Sichtkontakt-Alarm auf. Siehe ios/README.md.
    func taktStarten() {
        taktBeenden()
        takt = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 10_000_000_000)
                guard let self else { return }
                await self.auffrischen()
            }
        }
    }

    func taktBeenden() {
        takt?.cancel()
        takt = nil
    }

    func auffrischen() async {
        guard let s = zugang else { return }

        live = try? await Api.shared.live(s)
        if let ich = try? await Api.shared.me(s) { me = ich }
        intel = (try? await Api.shared.intel(s))?.intel ?? intel
        funkKanal = (try? await Api.shared.radio(s))?.messages ?? funkKanal
        mittel = (try? await Api.shared.jokers(s))?.jokers ?? mittel

        // Was die Rolle angeht, wird nur für die Rolle geholt: Ein
        // Fahndungsteam bekommt auf /mission ohnehin eine Absage, die
        // Zielperson hat keinen Zugang zu den Rätseln.
        if istZielperson {
            mission = (try? await Api.shared.mission(s)) ?? mission
        } else if istFahndung {
            raetsel = (try? await Api.shared.puzzles(s))?.puzzles ?? raetsel
        }

        if feld == nil {
            feld = try? await Api.shared.map(s)
        }
        if regeln.isEmpty {
            regeln = (try? await Api.shared.rules(s))?.groups ?? []
        }
    }

    // MARK: - Handlungen

    func transit(_ aktiv: Bool) async {
        guard let s = zugang else { return }
        try? await Api.shared.setTransit(s, aktiv: aktiv)
        await auffrischen()
    }

    func routeWaehlen(_ optionId: String) async {
        guard let s = zugang else { return }
        await handeln { self.mission = try await Api.shared.chooseOption(s, optionId: optionId) }
    }

    func codeEinreichen(_ code: String) async {
        guard let s = zugang else { return }
        await handeln {
            try await Api.shared.submitPasscode(s, code: code.uppercased())
            self.meldung = "Code angenommen."
            await self.auffrischen()
        }
    }

    func beweisSchicken(_ bild: Data) async {
        guard let s = zugang else { return }
        await handeln {
            try await Api.shared.uploadEvidence(s, bild: bild)
            self.meldung = "Foto eingereicht. Die Einsatzzentrale prüft es."
            await self.auffrischen()
        }
    }

    func verzoegerungMelden(_ grund: String) async {
        guard let s = zugang else { return }
        await handeln {
            let ergebnis = try await Api.shared.reportDelay(s, reason: grund)
            self.meldung = ergebnis.message
            await self.auffrischen()
        }
    }

    func raetselLoesen(_ id: String, _ antwort: String) async -> SolveResult? {
        guard let s = zugang else { return nil }
        var ergebnis: SolveResult?
        await handeln {
            ergebnis = try await Api.shared.solvePuzzle(s, id: id, answer: antwort)
            await self.auffrischen()
        }
        return ergebnis
    }

    func sichtkontaktMelden() async -> SightingResult? {
        guard let s = zugang else { return nil }
        var ergebnis: SightingResult?
        await handeln { ergebnis = try await Api.shared.reportSighting(s) }
        return ergebnis
    }

    func zugriff(stufe: Int, hotspot: Int, zeit: String?, ziel: Int?) async -> ArrestResult? {
        guard let s = zugang else { return nil }
        var ergebnis: ArrestResult?
        await handeln {
            ergebnis = try await Api.shared.arrest(s, level: stufe, hotspotNumber: hotspot,
                                                   claimedTime: zeit, claimedTarget: ziel)
            await self.auffrischen()
        }
        return ergebnis
    }

    func mittelEinsetzen(_ kind: String, sektor: String? = nil,
                         hotspot: String? = nil, raetsel: String? = nil) async {
        guard let s = zugang else { return }
        await handeln {
            let ergebnis = try await Api.shared.useJoker(s, kind: kind, sectorId: sektor,
                                                         hotspotId: hotspot, puzzleId: raetsel)
            self.meldung = ergebnis.message
            await self.auffrischen()
        }
    }

    func funken(_ text: String) async {
        guard let s = zugang, !text.trimmingCharacters(in: .whitespaces).isEmpty else { return }
        await handeln {
            try await Api.shared.postRadio(s, text: text)
            await self.auffrischen()
        }
    }

    func kontoHolen() async {
        guard let s = zugang else { return }
        konto = try? await Api.shared.ledger(s)
    }

    func einweisungAbgeschlossen() async {
        guard let s = zugang else { return }
        einweisungZeigen = false
        Session.shared.onboarded = true
        try? await Api.shared.markOnboarded(s)
    }

    /// Ein Rahmen für alles, was schiefgehen kann: eine Meldung statt eines
    /// stummen Nichts.
    private func handeln(_ arbeit: @escaping () async throws -> Void) async {
        busy = true
        fehler = nil
        defer { busy = false }
        do {
            try await arbeit()
        } catch {
            fehler = (error as? ApiFehler)?.text ?? "Das hat nicht geklappt."
        }
    }
}

/// Was jemand eintippt, ist selten eine URL.
enum Adressen {
    /// Macht aus "192.168.1.5:8090", "opx.example.org" oder einer vollen
    /// Adresse eine brauchbare Basis.
    ///
    /// Ohne Schema wird geraten – und zwar richtig herum: Wer eine Adresse im
    /// eigenen Netz eintippt, meint http, alles andere https. Dieselbe Regel
    /// wie in der Android-Fassung (data/Adressen.kt), dort mit Tests.
    static func normalisieren(_ eingabe: String) -> String {
        var text = eingabe.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else { return "" }

        while text.hasSuffix("/") { text.removeLast() }

        if text.hasPrefix("http://") || text.hasPrefix("https://") {
            return text
        }

        let host = text.split(separator: "/").first.map(String.init) ?? text
        let ohnePort = host.split(separator: ":").first.map(String.init) ?? host

        return (imEigenenNetz(ohnePort) ? "http://" : "https://") + text
    }

    static func imEigenenNetz(_ host: String) -> Bool {
        if host == "localhost" || host.hasSuffix(".local") { return true }

        let teile = host.split(separator: ".").compactMap { Int($0) }
        guard teile.count == 4 else { return false }

        switch (teile[0], teile[1]) {
        case (127, _), (10, _): return true
        case (192, 168): return true
        case (172, let z) where (16...31).contains(z): return true
        default: return false
        }
    }
}
