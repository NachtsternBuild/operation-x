import SwiftUI

/// Beitritt: die Adresse des Servers.
///
/// Sie steht auf der Teamkarte. Den QR-Code scannt auf dem iPhone die
/// Kamera-App von selbst und öffnet die Beitrittsseite im Browser – deshalb
/// braucht diese App keinen eigenen Scanner, nur ein Feld.
struct JoinView: View {
    @EnvironmentObject var zustand: AppState
    @State private var adresse = ""

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                Text("Operation X")
                    .font(.largeTitle.weight(.semibold))
                    .kerning(1.5)

                Text("Verbinde dich mit dem Einsatzserver. Die Adresse steht auf "
                     + "der Teamkarte — dort, wo auch der QR-Code ist.")
                    .foregroundStyle(.secondary)

                TextField("z. B. 192.168.1.5:8090", text: $adresse)
                    .textFieldStyle(.roundedBorder)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .keyboardType(.URL)
                    .submitLabel(.go)
                    .onSubmit { verbinden() }

                Button(action: verbinden) {
                    Text(zustand.busy ? "Verbinde …" : "Verbinden")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .disabled(zustand.busy || adresse.isEmpty)

                if let fehler = zustand.fehler {
                    Hinweiszeile(text: fehler, farbe: Farben.zielperson)
                }

                if !zustand.bekannteServer.isEmpty {
                    Text("Schon einmal verbunden")
                        .font(.caption)
                        .kerning(1.2)
                        .foregroundStyle(.secondary)

                    ForEach(zustand.bekannteServer) { server in
                        Button {
                            adresse = server.adresse
                            verbinden()
                        } label: {
                            HStack {
                                Text(server.adresse)
                                    .font(.footnote)
                                Spacer()
                                Text("🔒 " + String(server.kennzeichen.prefix(4)))
                                    .ziffern()
                                    .font(.caption)
                                    .foregroundStyle(Farben.gut)
                            }
                        }
                        .buttonStyle(.plain)
                    }
                }

                Hinweiszeile(text: "Zugangsdaten und Kartendaten verfallen nach dem "
                             + "Spiel automatisch.")
            }
            .padding(20)
        }
    }

    private func verbinden() {
        Task { await zustand.verbinden(adresse) }
    }
}

/// Anmeldung mit Rufzeichen und Kennwort von der Teamkarte.
struct LoginView: View {
    @EnvironmentObject var zustand: AppState
    @State private var rufzeichen = ""
    @State private var kennwort = ""

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                Text("Anmelden")
                    .font(.largeTitle.weight(.semibold))
                    .kerning(1.5)

                if !zustand.serverName.isEmpty {
                    Text(zustand.serverName)
                        .ziffern()
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }

                if let warnung = zustand.kennzeichenWarnung {
                    Hinweiszeile(text: warnung, farbe: Farben.zielperson)
                } else if !zustand.fingerprint.isEmpty {
                    VStack(alignment: .leading, spacing: 2) {
                        Text("🔒 " + zustand.fingerprint)
                            .ziffern()
                            .font(.footnote)
                            .foregroundStyle(Farben.gut)
                        Text("Dieselben Zeichen wie auf der Teamkarte?")
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                    }
                }

                TextField("Rufzeichen", text: $rufzeichen)
                    .textFieldStyle(.roundedBorder)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()

                SecureField("Kennwort", text: $kennwort)
                    .textFieldStyle(.roundedBorder)
                    .submitLabel(.go)
                    .onSubmit { anmelden() }

                Button(action: anmelden) {
                    Text(zustand.busy ? "Prüfe Zugang …" : "Anmelden")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .disabled(zustand.busy || rufzeichen.isEmpty || kennwort.isEmpty)

                if let fehler = zustand.fehler {
                    Hinweiszeile(text: fehler, farbe: Farben.zielperson)
                }

                Button("Anderen Server wählen") { zustand.serverWechseln() }
                    .font(.footnote)
            }
            .padding(20)
        }
    }

    private func anmelden() {
        Task { await zustand.anmelden(callsign: rufzeichen, kennwort: kennwort) }
    }
}

/// Die Einweisung, einmal je Zugang.
///
/// Kürzer als in der Android-Fassung und ohne Handlungsprobe: Wer sie hier
/// sieht, hat die App schon installiert – der schwierigste Schritt ist getan.
/// Was bleibt, sind die drei Sätze, die man am Spieltag wirklich braucht.
struct OnboardingView: View {
    @EnvironmentObject var zustand: AppState
    @State private var seite = 0

    /// Dieselbe Seite für jede Rolle, und zwar bevor die Erfassung anläuft.
    ///
    /// Wer Standortdaten von Leuten verarbeitet, sagt ihnen vorher, was
    /// erhoben wird und wie lange es bleibt. Wortgleich in der Android-Fassung
    /// und in der Weboberfläche.
    private let datenschutz = (
        "Was dieses Spiel über euch speichert",
        "Euren Standort, solange die Erfassung läuft — im Spiel etwa alle "
         + "zehn Minuten einer. Dazu Punkte, Buchungen und was ihr in den Funk "
         + "schreibt. Alles hängt an diesem einen Spiel und liegt auf dem "
         + "Rechner der Spielleitung, nicht bei uns; wir bekommen nichts davon "
         + "zu sehen. Nach dem Spiel wird die Bewegungsspur automatisch "
         + "gelöscht — ab Werk 24 Stunden nach Spielende. Was bleibt, ist der "
         + "Punktestand ohne Koordinaten. Wer damit nicht einverstanden ist, "
         + "spielt nicht mit; das ist in Ordnung und hat keine Folgen.")

    private var seiten: [(String, String)] {
        rollenseiten + [datenschutz]
    }

    private var rollenseiten: [(String, String)] {
        if zustand.istZielperson {
            return [
                ("Ihr seid die Zielperson",
                 "Ihr bewegt euch unerkannt durch die Stadt, arbeitet Zwischenziele ab "
                 + "und wollt am Ende das geheime Fluchtziel erreichen. Die Fahndung "
                 + "erscheint als unscharfe Kreise — rund 200 Meter Unschärfe."),
                ("Meldepflicht",
                 "Alle zehn Minuten muss euer Standort beim Server sein, im Transit "
                 + "alle dreizehn. Die App erledigt das, solange die Erfassung läuft. "
                 + "Der zweite Verstoß kostet Punkte, ab dem dritten kommt eine Sperre."),
                ("Stillstand kostet",
                 "Wer sich zwanzig Minuten weder bewegt noch etwas tut, verliert einen "
                 + "Fluchtpunkt. Gezählt wird das nur außerhalb von Mission und Transit. "
                 + "Verstecken ist erlaubt, Aussitzen nicht."),
            ]
        }
        return [
            ("Ihr seid die Fahndung",
             "Mehrere Teams arbeiten zusammen gegen eine Zielperson. Ihr seht euch "
             + "gegenseitig immer genau — von der Zielperson seht ihr nur, was ihr "
             + "euch erarbeitet."),
            ("Meldepflicht",
             "Alle zehn Minuten muss euer Standort beim Server sein, im Transit alle "
             + "dreizehn. Die App erledigt das, solange die Erfassung läuft."),
            ("Hinweise erarbeiten",
             "Rätsel lösen schaltet Hinweise frei — sie entstehen aus der echten Lage. "
             + "Wer schnell löst, bekommt eine frische Spur. Nicht jeder Hinweis stimmt: "
             + "Die Zielperson kann falsche Fährten legen."),
        ]
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            Text("Einweisung \(seite + 1) von \(seiten.count)")
                .font(.caption)
                .kerning(1.4)
                .foregroundStyle(.secondary)

            Text(seiten[seite].0)
                .font(.title2.weight(.semibold))

            Text(seiten[seite].1)
                .foregroundStyle(.secondary)

            Spacer()

            HStack {
                if seite > 0 {
                    Button("Zurück") { seite -= 1 }
                }
                Spacer()
                Button(seite == seiten.count - 1 ? "Los geht's" : "Weiter") {
                    if seite == seiten.count - 1 {
                        Task { await zustand.einweisungAbgeschlossen() }
                    } else {
                        seite += 1
                    }
                }
                .buttonStyle(.borderedProminent)
            }
        }
        .padding(24)
    }
}
