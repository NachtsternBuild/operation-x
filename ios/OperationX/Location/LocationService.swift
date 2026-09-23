import Foundation
import CoreLocation

/// Die Standortmeldung – der Grund, warum es diese App überhaupt gibt.
///
/// Im Browser hört die Ortung auf, sobald der Bildschirm ausgeht. Ein Spiel
/// über sechs Stunden mit dauernd eingeschaltetem Bildschirm ist kein Spiel,
/// sondern ein Akkutest. Deshalb hier: Standort im Hintergrund, gepuffert und
/// nachgereicht.
///
/// Drei Entscheidungen stecken darin, die auf dem Gerät teuer erkauft wurden –
/// sie stehen genauso in der Android-Fassung:
///
///   * **Kein Mindestabstand.** Wer sich nicht bewegt, muss trotzdem melden:
///     Die Meldepflicht misst Zeit, nicht Weg. Ein Abstandsfilter bestraft
///     genau den, der sich regelkonform irgendwo hinsetzt.
///   * **Melden nach Zeit, nicht nach jedem Messwert.** Jede Ortung zu
///     schicken kostet Funk und Akku für nichts.
///   * **Jede Meldung zuerst in den Puffer.** Was nicht durchkommt, geht beim
///     nächsten Kontakt mit seinem ursprünglichen Zeitstempel raus.
@MainActor
final class LocationService: NSObject, ObservableObject {

    /// So oft geht eine Meldung an den Server, solange die Erfassung läuft.
    /// Die Pflicht des Regelwerks liegt bei zehn Minuten; häufiger zu melden
    /// hält die Lagekarte lebendig, ohne dass es weh tut.
    private let meldeAbstand: TimeInterval = 60

    @Published private(set) var laeuft = false
    @Published private(set) var letzteMeldung: Date?
    @Published private(set) var wartend = 0
    @Published private(set) var hinweis: String?
    @Published private(set) var berechtigung: CLAuthorizationStatus = .notDetermined

    /// Woher Adresse und Zugangstoken kommen. Setzt der AppState.
    var zugang: (() -> Api.Zugang?)?
    /// Was der Server geantwortet hat – der AppState zieht daraus die Frist nach.
    var quittung: ((PositionAck) -> Void)?

    private let manager = CLLocationManager()
    private var letzterVersuch: Date?
    private var sendetGerade = false

    override init() {
        super.init()
        manager.delegate = self
        manager.desiredAccuracy = kCLLocationAccuracyBest
        // Kein Abstandsfilter: siehe oben.
        manager.distanceFilter = kCLDistanceFilterNone
        manager.activityType = .other
        // Das System hält die Ortung sonst an, sobald es meint, jemand sei
        // stehengeblieben – und genau dann wird die Meldung fällig.
        manager.pausesLocationUpdatesAutomatically = false
        berechtigung = manager.authorizationStatus
    }

    // MARK: - Steuerung

    func erlaubnisFragen() {
        // Zweistufig, wie iOS es verlangt: erst während der Benutzung, danach
        // – nach dem ersten Start – die Erlaubnis für den Hintergrund.
        if manager.authorizationStatus == .notDetermined {
            manager.requestWhenInUseAuthorization()
        } else {
            manager.requestAlwaysAuthorization()
        }
    }

    func starten() {
        guard !laeuft else { return }

        switch manager.authorizationStatus {
        case .notDetermined:
            erlaubnisFragen()
            return
        case .denied, .restricted:
            hinweis = "Die Ortung ist gesperrt. In den Einstellungen freigeben, "
                + "sonst meldet niemand einen Standort — auch ihr nicht."
            return
        default:
            break
        }

        // Nur erlaubt, wenn die App den Hintergrundmodus "Standort" führt und
        // die Erlaubnis "Immer" vorliegt. Sonst wirft das System.
        if manager.authorizationStatus == .authorizedAlways {
            manager.allowsBackgroundLocationUpdates = true
        }
        manager.showsBackgroundLocationIndicator = true

        manager.startUpdatingLocation()
        laeuft = true
        hinweis = nil
    }

    func anhalten() {
        manager.stopUpdatingLocation()
        manager.allowsBackgroundLocationUpdates = false
        laeuft = false
    }

    /// „Standort melden“ – meldet sofort, ohne auf den Takt zu warten.
    ///
    /// Der Knopf hieß in der Android-Fassung lange so, tat aber nichts, weil er
    /// nur den Dienst anstupste. Hier holt er eine Ortung und schickt sie.
    func sofortMelden() {
        if let letzte = manager.location {
            merkenUndSenden(letzte, erzwingen: true)
        } else {
            manager.requestLocation()
        }
    }

    // MARK: - Innenleben

    private func merkenUndSenden(_ ort: CLLocation, erzwingen: Bool = false) {
        let jetzt = Date()

        if !erzwingen, let letzter = letzterVersuch,
           jetzt.timeIntervalSince(letzter) < meldeAbstand {
            return
        }
        letzterVersuch = jetzt

        let bericht = PositionReport(
            lat: ort.coordinate.latitude,
            lng: ort.coordinate.longitude,
            accuracy: max(0, ort.horizontalAccuracy),
            speed: max(0, ort.speed),
            heading: max(0, ort.course),
            capturedAt: Zeit.stempel(ort.timestamp),
            mocked: gefaelscht(ort)
        )

        Session.shared.puffern(bericht)
        wartend = Session.shared.puffer.count

        Task { await senden() }
    }

    /// Schickt alles, was im Puffer liegt – in einem Zug, mit den echten
    /// Zeitstempeln.
    private func senden() async {
        guard !sendetGerade, let zugang = zugang?() else { return }

        let liste = Session.shared.puffer
        guard !liste.isEmpty else { return }

        sendetGerade = true
        defer { sendetGerade = false }

        do {
            let ack = try await Api.shared.sendPositions(zugang, liste)
            Session.shared.pufferLeeren()
            wartend = 0
            letzteMeldung = Date()

            if ack.paused == true {
                hinweis = "Pause — es wird nichts aufgezeichnet."
                anhalten()
            } else {
                hinweis = nil
            }
            quittung?(ack)
        } catch {
            // Bleibt im Puffer. Das ist kein Fehler, sondern der Normalfall in
            // der Bahn – aber es gehört sichtbar gemacht, sonst hält man die
            // App für kaputt.
            wartend = Session.shared.puffer.count
            hinweis = wartend == 1
                ? "Eine Meldung wartet auf Übertragung."
                : "\(wartend) Meldungen warten auf Übertragung."
        }
    }

    /// Fake-GPS erkennen. Der Server erfährt es und entscheidet selbst.
    private func gefaelscht(_ ort: CLLocation) -> Bool {
        if #available(iOS 15.0, *) {
            return ort.sourceInformation?.isSimulatedBySoftware ?? false
        }
        return false
    }
}

extension LocationService: CLLocationManagerDelegate {

    nonisolated func locationManager(_ manager: CLLocationManager,
                                     didUpdateLocations locations: [CLLocation]) {
        guard let ort = locations.last else { return }
        Task { @MainActor in
            // Uralte Messwerte aus dem Zwischenspeicher des Systems übergehen.
            guard abs(ort.timestamp.timeIntervalSinceNow) < 120 else { return }
            self.merkenUndSenden(ort)
        }
    }

    nonisolated func locationManager(_ manager: CLLocationManager,
                                     didFailWithError error: Error) {
        Task { @MainActor in
            self.hinweis = "Die Ortung meldet einen Fehler. Sicht zum Himmel prüfen."
        }
    }

    nonisolated func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        Task { @MainActor in
            self.berechtigung = manager.authorizationStatus
            if manager.authorizationStatus == .authorizedWhenInUse {
                // Zweiter Schritt: ohne "Immer" hört die Meldung auf, sobald
                // der Bildschirm ausgeht.
                manager.requestAlwaysAuthorization()
            }
            if self.laeuft { self.starten() }
        }
    }
}
