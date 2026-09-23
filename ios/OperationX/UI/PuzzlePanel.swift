import SwiftUI

/// Die Ermittlungsseite der Fahndung: Rätsel, Sichtkontakt, Zugriff.
///
/// Die drei gehören zusammen, weil sie eine Reihenfolge sind: Rätsel lösen,
/// bis man weiß, wo gesucht wird. Sichtkontakt melden, wenn man dort jemanden
/// sieht. Zugreifen, wenn man sicher ist – und das kostet, wenn man es nicht
/// ist.
struct PuzzlePanel: View {
    @EnvironmentObject var zustand: AppState

    @State private var antworten: [String: String] = [:]
    @State private var ergebnis: String?
    @State private var zugriffOffen = false

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {

                Tafel(titel: "Zugriff", farbe: Farben.fahndung) {
                    Text("Sichtkontakt gilt ab 50 Metern, gehalten über 20 Sekunden. "
                         + "Der Zugriff ist ein Formular — Stufe 3 daneben kostet "
                         + "Punkte und eine Sperre.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)

                    HStack {
                        Button("Sichtkontakt melden") {
                            Task {
                                let r = await zustand.sichtkontaktMelden()
                                ergebnis = r?.message
                            }
                        }
                        .buttonStyle(.bordered)

                        Button("Zugriff") { zugriffOffen = true }
                            .buttonStyle(.borderedProminent)
                            .tint(Farben.fahndung)
                    }

                    if let ergebnis {
                        Hinweiszeile(text: ergebnis, farbe: Farben.zentrale)
                    }
                }

                if zustand.raetsel.isEmpty {
                    Text("Noch keine Rätsel freigeschaltet. Die Zentrale gibt sie frei.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }

                ForEach(zustand.raetsel) { raetsel in
                    Tafel(titel: "\(raetsel.code ?? "") · Typ \(raetsel.type ?? "")") {
                        Text(raetsel.title ?? "")
                            .font(.headline)
                        Text(raetsel.question ?? "")
                            .font(.subheadline)

                        if let tipp = raetsel.hint, !tipp.isEmpty {
                            Hinweiszeile(text: tipp)
                        }

                        if raetsel.solved == true {
                            Text("Gelöst · \(raetsel.points ?? 0) Punkte")
                                .font(.footnote)
                                .foregroundStyle(Farben.gut)
                        } else {
                            HStack {
                                TextField("Antwort", text: Binding(
                                    get: { antworten[raetsel.id ?? ""] ?? "" },
                                    set: { antworten[raetsel.id ?? ""] = $0 }
                                ))
                                .textFieldStyle(.roundedBorder)
                                .autocorrectionDisabled()

                                Button("Prüfen") {
                                    let id = raetsel.id ?? ""
                                    Task {
                                        let r = await zustand.raetselLoesen(
                                            id, antworten[id] ?? "")
                                        ergebnis = r?.message
                                        if r?.correct == true { antworten[id] = "" }
                                    }
                                }
                                .buttonStyle(.bordered)
                                .disabled((antworten[raetsel.id ?? ""] ?? "").isEmpty)
                            }

                            if (raetsel.attempts ?? 0) > 0 {
                                Text("\(raetsel.attempts ?? 0) Versuche")
                                    .font(.caption2)
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }
                }
            }
            .padding(12)
        }
        .sheet(isPresented: $zugriffOffen) {
            ZugriffView()
                .environmentObject(zustand)
        }
    }
}

/// Das Zugriffsformular in drei Stufen.
struct ZugriffView: View {
    @EnvironmentObject var zustand: AppState
    @Environment(\.dismiss) private var schliessen

    @State private var stufe = 1
    @State private var hotspot = ""
    @State private var zeit = ""
    @State private var ziel = ""
    @State private var antwort: ArrestResult?

    var body: some View {
        NavigationStack {
            Form {
                Section("Stufe") {
                    Picker("Stufe", selection: $stufe) {
                        Text("1 — Lokalisierung").tag(1)
                        Text("2 — Rekonstruktion").tag(2)
                        Text("3 — Vollzugriff").tag(3)
                    }
                    .pickerStyle(.inline)
                    .labelsHidden()
                }

                Section("Behauptung") {
                    TextField("Hotspotnummer, z. B. 7", text: $hotspot)
                        .keyboardType(.numberPad)

                    if stufe >= 2 {
                        TextField("Uhrzeit, z. B. 14:20", text: $zeit)
                    }
                    if stufe >= 3 {
                        TextField("Nächstes Fluchtziel (Nummer)", text: $ziel)
                            .keyboardType(.numberPad)
                    }
                }

                if stufe == 3 {
                    Section {
                        Text("Stufe 3 daneben kostet 15 Punkte und zehn Minuten Sperre. "
                             + "Raten lohnt sich nicht.")
                            .font(.footnote)
                            .foregroundStyle(Farben.zielperson)
                    }
                }

                if let antwort {
                    Section("Ergebnis") {
                        Text(antwort.message ?? "")
                            .foregroundStyle(antwort.correct == true ? Farben.gut : Farben.zielperson)
                    }
                }
            }
            .navigationTitle("Zugriff")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Schließen") { schliessen() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Absenden") { absenden() }
                        .disabled(hotspot.isEmpty || zustand.busy)
                }
            }
        }
    }

    private func absenden() {
        Task {
            antwort = await zustand.zugriff(
                stufe: stufe,
                hotspot: Int(hotspot) ?? 0,
                zeit: stufe >= 2 ? uhrzeitAlsStempel() : nil,
                ziel: stufe >= 3 ? Int(ziel) : nil
            )
        }
    }

    /// "14:20" meint heute um 14:20 Ortszeit – der Server erwartet einen
    /// vollständigen Zeitstempel.
    private func uhrzeitAlsStempel() -> String? {
        let teile = zeit.split(separator: ":").compactMap { Int($0) }
        guard teile.count == 2 else { return nil }

        var felder = Calendar.current.dateComponents([.year, .month, .day], from: Date())
        felder.hour = teile[0]
        felder.minute = teile[1]

        guard let datum = Calendar.current.date(from: felder) else { return nil }
        return Zeit.stempel(datum)
    }
}
