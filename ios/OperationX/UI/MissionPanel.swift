import SwiftUI
import PhotosUI

/// Das Missionsbuch der Zielperson.
///
/// Zwei Zustände: Route wählen oder unterwegs sein. Im zweiten steht oben die
/// Aufgabe, die die Spielleitung an diesen Ort geschrieben hat – sie ist der
/// Grund, dort zu sein, und stand vor ihrer Anzeige nur in der Datenbank.
struct MissionPanel: View {
    @EnvironmentObject var zustand: AppState

    @State private var code = ""
    @State private var grund = ""
    @State private var verzoegerungOffen = false
    @State private var bildAuswahl: PhotosPickerItem?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                switch zustand.mission?.status {
                case "proposed": routenwahl
                case "active": unterwegs
                default:
                    Text(zustand.mission?.message
                         ?? "Noch kein Zwischenziel. Meldet einmal euren Standort.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
            }
            .padding(12)
        }
    }

    // MARK: - Route wählen

    private var routenwahl: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Zwischenziel \(zustand.mission?.seq ?? 0) von \(zustand.mission?.planned ?? 0)")
                .font(.caption)
                .foregroundStyle(.secondary)
            Text("Route wählen")
                .font(.title3.weight(.semibold))

            ForEach(zustand.mission?.options ?? []) { option in
                Button {
                    Task { await zustand.routeWaehlen(option.id ?? "") }
                } label: {
                    VStack(alignment: .leading, spacing: 4) {
                        Text(option.label ?? "")
                            .font(.subheadline.weight(.semibold))
                            .foregroundStyle(farbe(option.kind))
                        Text(String(format: "#%02d %@", option.number ?? 0, option.name ?? ""))
                            .font(.body)
                        if let beschreibung = option.description {
                            Text(beschreibung)
                                .font(.footnote)
                                .foregroundStyle(.secondary)
                        }
                        if let aufgabe = option.task, !aufgabe.isEmpty {
                            Text("Vor Ort: " + aufgabe)
                                .font(.footnote)
                                .foregroundStyle(.secondary)
                        }
                        Text("\(Int(option.distanceM ?? 0)) m · \(option.timeLimitMin ?? 0) min · "
                             + "\(option.rewardPoints ?? 0) Pkt"
                             + ((option.rewardFp ?? 0) > 0 ? " + \(option.rewardFp ?? 0) FP" : ""))
                            .ziffern()
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(12)
                    .background(Color(.secondarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 12))
                }
                .buttonStyle(.plain)
                .disabled(zustand.busy)
            }
        }
    }

    private func farbe(_ art: String?) -> Color {
        switch art {
        case "safe": return Farben.gut
        case "fast": return Farben.warnung
        default: return Farben.zentrale
        }
    }

    // MARK: - Unterwegs

    private var unterwegs: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text("Zwischenziel \(zustand.mission?.seq ?? 0) von \(zustand.mission?.planned ?? 0)")
                .font(.caption)
                .foregroundStyle(.secondary)

            Text(String(format: "#%02d %@",
                        zustand.mission?.target?.number ?? 0,
                        zustand.mission?.target?.name ?? ""))
                .font(.title3.weight(.semibold))

            if let aufgabe = zustand.mission?.target?.task, !aufgabe.isEmpty {
                Tafel(titel: "Aufgabe vor Ort", farbe: Farben.zielperson) {
                    Text(aufgabe)
                        .font(.subheadline)
                }
            }

            HStack(spacing: 24) {
                wert("Rest", Zeit.uhr(zustand.mission?.leftSec ?? 0),
                     (zustand.mission?.leftSec ?? 999) < 300 ? Farben.zielperson : nil)
                wert("Abstand", "\(Int(zustand.mission?.distanceM ?? 0)) m", nil)
                wert("Nachweis ab", "\(Int(zustand.mission?.rangeM ?? 0)) m", nil)
            }

            if zustand.mission?.inRange == true {
                Tafel(titel: "Nachweis führen") {
                    TextField("Code vor Ort", text: $code)
                        .textFieldStyle(.roundedBorder)
                        .textInputAutocapitalization(.characters)
                        .autocorrectionDisabled()

                    Button("Code einreichen") {
                        Task {
                            await zustand.codeEinreichen(code)
                            code = ""
                        }
                    }
                    .buttonStyle(.borderedProminent)
                    .disabled(code.isEmpty || zustand.busy)

                    PhotosPicker(selection: $bildAuswahl, matching: .images) {
                        Text("Stattdessen ein Foto einreichen")
                            .font(.footnote)
                    }
                }
            } else {
                Hinweiszeile(text: "Noch \(abstandRest) m. Der Nachweis lässt sich erst "
                             + "am Ziel führen.")
            }

            if let grace = zustand.mission?.grace, (grace.leftMin ?? 0) > 0 {
                Button("Ich brauche länger (noch \(grace.leftMin ?? 0) Min. Kulanz)") {
                    verzoegerungOffen = true
                }
                .font(.footnote)
            }
        }
        .onChange(of: bildAuswahl) { neu in
            guard let neu else { return }
            Task {
                if let daten = try? await neu.loadTransferable(type: Data.self) {
                    await zustand.beweisSchicken(daten)
                }
                bildAuswahl = nil
            }
        }
        .alert("Warum dauert es länger?", isPresented: $verzoegerungOffen) {
            TextField("z. B. Schlange an der Kasse", text: $grund)
            Button("Melden") {
                Task {
                    await zustand.verzoegerungMelden(grund)
                    grund = ""
                }
            }
            Button("Abbrechen", role: .cancel) {}
        } message: {
            Text("Die Zentrale erfährt den Grund. Die Fahndung nicht.")
        }
    }

    private var abstandRest: Int {
        max(0, Int((zustand.mission?.distanceM ?? 0) - (zustand.mission?.rangeM ?? 0)))
    }

    private func wert(_ titel: String, _ text: String, _ farbe: Color?) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(titel)
                .font(.caption2)
                .foregroundStyle(.secondary)
            Text(text)
                .ziffern()
                .font(.title3)
                .foregroundStyle(farbe ?? .primary)
        }
    }
}
