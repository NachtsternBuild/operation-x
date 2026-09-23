import SwiftUI

/// Einsatzmittel, Regeln und Punktekonto – alles, was man nachschlägt.
struct MittelPanel: View {
    @EnvironmentObject var zustand: AppState
    @State private var zielWahl: Joker?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {

                Tafel(titel: "Einsatzmittel",
                      farbe: Farben.fuerRolle(zustand.me?.role)) {
                    Text("\(zustand.live?.selfState?.fp ?? 0) Punkte verfügbar")
                        .ziffern()
                        .font(.footnote)
                        .foregroundStyle(.secondary)

                    if zustand.mittel.isEmpty {
                        Text("Noch keine Einsatzmittel verfügbar.")
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                    }

                    ForEach(zustand.mittel) { joker in
                        Button {
                            if brauchtZiel(joker.kind) {
                                zielWahl = joker
                            } else {
                                Task { await zustand.mittelEinsetzen(joker.kind ?? "") }
                            }
                        } label: {
                            VStack(alignment: .leading, spacing: 3) {
                                HStack {
                                    Text(joker.name ?? "")
                                        .font(.subheadline.weight(.semibold))
                                    Spacer()
                                    Text("\(joker.cost ?? 0) FP")
                                        .ziffern()
                                        .font(.caption)
                                        .foregroundStyle(Farben.zentrale)
                                }
                                Text(joker.description ?? "")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)

                                if joker.available != true, let grund = joker.reason,
                                   !grund.isEmpty {
                                    Text(grund)
                                        .font(.caption2)
                                        .foregroundStyle(Farben.warnung)
                                } else if (joker.maxUses ?? 0) > 0 {
                                    Text("noch \((joker.maxUses ?? 0) - (joker.used ?? 0)) "
                                         + "von \(joker.maxUses ?? 0)")
                                        .font(.caption2)
                                        .foregroundStyle(.secondary)
                                }
                            }
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .padding(10)
                            .background(Color(.tertiarySystemBackground))
                            .clipShape(RoundedRectangle(cornerRadius: 10))
                        }
                        .buttonStyle(.plain)
                        .disabled(joker.available != true || zustand.busy)
                    }
                }

                KontoTafel()
                RegelTafel()
            }
            .padding(12)
        }
        .sheet(item: $zielWahl) { joker in
            ZielWahlView(joker: joker)
                .environmentObject(zustand)
        }
    }

    /// Sperrzone und Scan brauchen einen Sektor, die Wanze einen Hotspot.
    private func brauchtZiel(_ art: String?) -> Bool {
        art == "lockdown" || art == "scan" || art == "bug"
    }
}

/// Wohin der Joker wirkt.
struct ZielWahlView: View {
    @EnvironmentObject var zustand: AppState
    @Environment(\.dismiss) private var schliessen

    let joker: Joker

    var body: some View {
        NavigationStack {
            List {
                if joker.kind == "bug" {
                    ForEach(zustand.feld?.hotspots ?? []) { h in
                        Button(String(format: "#%02d %@", h.number ?? 0, h.name ?? "")) {
                            Task {
                                await zustand.mittelEinsetzen(joker.kind ?? "",
                                                              hotspot: h.id)
                                schliessen()
                            }
                        }
                    }
                } else {
                    ForEach(zustand.feld?.sectors ?? []) { s in
                        Button("\(s.code ?? "") \(s.name ?? "")") {
                            Task {
                                await zustand.mittelEinsetzen(joker.kind ?? "", sektor: s.id)
                                schliessen()
                            }
                        }
                    }
                }
            }
            .navigationTitle(joker.name ?? "Ziel wählen")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Abbrechen") { schliessen() }
                }
            }
        }
    }
}

/// Das Punktekonto: warum der Stand so ist, wie er ist.
struct KontoTafel: View {
    @EnvironmentObject var zustand: AppState

    var body: some View {
        Tafel(titel: "Punktekonto") {
            if let konto = zustand.konto {
                Text("\(konto.points ?? 0) Punkte · \(konto.fp ?? 0) FP")
                    .ziffern()
                    .font(.subheadline)

                ForEach(Array((konto.entries ?? []).prefix(25))) { buchung in
                    HStack(alignment: .top) {
                        Text(Zeit.uhrzeit(buchung.occurredAt))
                            .ziffern()
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                            .frame(width: 44, alignment: .leading)

                        Text(buchung.reason ?? "")
                            .font(.caption)

                        Spacer()

                        Text(betrag(buchung))
                            .ziffern()
                            .font(.caption)
                            .foregroundStyle(farbe(buchung))
                    }
                }
            } else {
                Button("Konto anzeigen") {
                    Task { await zustand.kontoHolen() }
                }
                .font(.footnote)
            }
        }
    }

    private func betrag(_ b: LedgerEntry) -> String {
        var teile: [String] = []
        if let p = b.deltaPoints, p != 0 { teile.append("\(p > 0 ? "+" : "")\(p)") }
        if let f = b.deltaFp, f != 0 { teile.append("\(f > 0 ? "+" : "")\(f) FP") }
        return teile.joined(separator: " · ")
    }

    private func farbe(_ b: LedgerEntry) -> Color {
        let summe = (b.deltaPoints ?? 0) + (b.deltaFp ?? 0)
        if summe > 0 { return Farben.gut }
        if summe < 0 { return Farben.zielperson }
        return .secondary
    }
}

/// Der Regelnachschlag – mit den Werten dieses Spiels, nicht mit den
/// Voreinstellungen aus dem Regelwerk.
struct RegelTafel: View {
    @EnvironmentObject var zustand: AppState

    var body: some View {
        Tafel(titel: "Regeln") {
            if zustand.regeln.isEmpty {
                Text("Werden geladen …")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }

            ForEach(zustand.regeln) { gruppe in
                DisclosureGroup(gruppe.group ?? "") {
                    ForEach(gruppe.rules ?? []) { regel in
                        HStack(alignment: .top) {
                            VStack(alignment: .leading, spacing: 1) {
                                Text(regel.name ?? "")
                                    .font(.caption)
                                Text(regel.what ?? "")
                                    .font(.caption2)
                                    .foregroundStyle(.secondary)
                            }
                            Spacer()
                            Text("\(regel.value ?? 0) \(einheit(regel.unit))")
                                .ziffern()
                                .font(.caption)
                                .foregroundStyle(Farben.zentrale)
                        }
                        .padding(.vertical, 2)
                    }
                }
                .font(.subheadline)
            }
        }
    }

    private func einheit(_ kurz: String?) -> String {
        switch kurz {
        case "min": return "Min."
        case "sek": return "Sek."
        case "m": return "m"
        case "punkte": return "Punkte"
        case "fp": return "FP"
        default: return ""
        }
    }
}

/// Der Funkkanal.
struct RadioPanel: View {
    @EnvironmentObject var zustand: AppState
    @State private var text = ""

    var body: some View {
        VStack(spacing: 0) {
            ScrollView {
                VStack(alignment: .leading, spacing: 10) {
                    ForEach(zustand.funkKanal) { nachricht in
                        VStack(alignment: .leading, spacing: 2) {
                            HStack {
                                Text(nachricht.author ?? "")
                                    .font(.caption.weight(.semibold))
                                    .foregroundStyle(Farben.fuerRolle(nachricht.role))
                                Spacer()
                                Text(Zeit.uhrzeit(nachricht.at))
                                    .ziffern()
                                    .font(.caption2)
                                    .foregroundStyle(.secondary)
                            }
                            Text(nachricht.text ?? "")
                                .font(.subheadline)
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(10)
                        .background(Color(.secondarySystemBackground))
                        .clipShape(RoundedRectangle(cornerRadius: 10))
                    }
                }
                .padding(12)
            }

            if !zustand.istZielperson {
                HStack {
                    TextField("Funkspruch", text: $text)
                        .textFieldStyle(.roundedBorder)
                    Button("Senden") {
                        Task {
                            await zustand.funken(text)
                            text = ""
                        }
                    }
                    .disabled(text.isEmpty || zustand.busy)
                }
                .padding(12)
            } else {
                Hinweiszeile(text: "Die Zielperson hört mit, schreibt aber nicht.")
                    .padding(12)
            }
        }
    }
}
