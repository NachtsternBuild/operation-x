import SwiftUI

/// Die Feldansicht: alles, was man unterwegs braucht, auf einem Bildschirm.
///
/// Oben der Kopf mit Punkten, Fluchtpunkten und dem Kennzeichen des Servers,
/// darunter der Meldeknopf mit der Frist, dann die Reiter. Genau wie in der
/// Android-Fassung – wer beide nebeneinander hält, findet dieselben Dinge an
/// denselben Stellen.
struct FieldView: View {
    @EnvironmentObject var zustand: AppState
    /// Die Ortung als eigenes Umgebungsobjekt: Nur so bekommt die Leiste
    /// mit, wenn eine Meldung im Puffer hängen bleibt.
    @EnvironmentObject var ortung: LocationService
    @State private var reiter = 0

    var body: some View {
        VStack(spacing: 0) {
            kopf
            meldeleiste

            Picker("", selection: $reiter) {
                Text("Lage").tag(0)
                Text(zustand.istZielperson ? "Mission" : "Ermittlung").tag(1)
                Text("Mittel").tag(2)
                Text("Funk").tag(3)
            }
            .pickerStyle(.segmented)
            .padding(.horizontal, 12)
            .padding(.vertical, 8)

            Divider()

            inhalt
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
        .overlay(alignment: .bottom) { meldungen }
        .task {
            await zustand.auffrischen()
            zustand.taktStarten()
        }
        .onDisappear { zustand.taktBeenden() }
    }

    // MARK: - Kopf

    private var kopf: some View {
        HStack(alignment: .top) {
            VStack(alignment: .leading, spacing: 2) {
                Text(zustand.me?.display ?? zustand.me?.callsign ?? "")
                    .font(.headline)
                    .foregroundStyle(Farben.fuerRolle(zustand.me?.role))
                Text(rollenname)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }

            Spacer()

            VStack(alignment: .trailing, spacing: 2) {
                Text("\(zustand.live?.selfState?.points ?? zustand.me?.points ?? 0) Pkt")
                    .ziffern()
                    .font(.subheadline)
                Text("\(zustand.live?.selfState?.fp ?? zustand.me?.fp ?? 0) FP")
                    .ziffern()
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }

            if !zustand.fingerprint.isEmpty {
                // Das Kennzeichen des Servers. Stimmt es mit dem auf der
                // Teamkarte überein, sitzt niemand dazwischen.
                Text("🔒 " + String(zustand.fingerprint.prefix(4)))
                    .ziffern()
                    .font(.caption)
                    .foregroundStyle(Farben.gut)
            }

            Menu {
                Button("Punktekonto") { reiter = 2 }
                Button("Abmelden", role: .destructive) { zustand.abmelden() }
            } label: {
                Image(systemName: "ellipsis.circle")
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
        .background(Color(.secondarySystemBackground))
    }

    private var rollenname: String {
        switch zustand.me?.role {
        case "misterx": return "Zielperson"
        case "hq": return "Einsatzzentrale"
        default: return "Fahndungsteam"
        }
    }

    // MARK: - Meldeleiste

    private var meldeleiste: some View {
        VStack(spacing: 8) {
            HStack {
                Button {
                    ortung.sofortMelden()
                } label: {
                    Text("Standort melden")
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 6)
                }
                .buttonStyle(.borderedProminent)
                .tint(Farben.fuerRolle(zustand.me?.role))

                VStack(alignment: .trailing, spacing: 0) {
                    Text("Nächste Meldung in")
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                    Text(Zeit.uhr(zustand.live?.selfState?.dueInSec ?? 0))
                        .ziffern()
                        .font(.title3)
                        .foregroundStyle(faellig ? Farben.zielperson : .primary)
                }
            }

            HStack(spacing: 10) {
                Toggle("Transit", isOn: Binding(
                    get: { zustand.live?.selfState?.inTransit ?? false },
                    set: { neu in Task { await zustand.transit(neu) } }
                ))
                .toggleStyle(.button)
                .font(.caption)

                Toggle("Erfassung", isOn: Binding(
                    get: { ortung.laeuft },
                    set: { an in an ? ortung.starten() : ortung.anhalten() }
                ))
                .toggleStyle(.button)
                .font(.caption)
                .tint(ortung.laeuft ? Farben.gut : .secondary)

                Spacer()
            }

            if let pause = zustand.live?.pause {
                Hinweiszeile(
                    text: "Pause — \(pause.reason ?? "Unterbrechung")."
                        + (pause.until.map { " Weiter gegen \(Zeit.uhrzeit($0)) Uhr." } ?? ""),
                    farbe: Farben.zentrale)
            }

            if let hinweis = ortung.hinweis {
                Hinweiszeile(text: hinweis, farbe: Farben.warnung)
            }

            if !ortung.laeuft && zustand.live?.pause == nil {
                Hinweiszeile(text: "Die Erfassung ist aus. Ohne sie meldet niemand "
                             + "einen Standort — auch ihr nicht.", farbe: Farben.warnung)
            }
        }
        .padding(.horizontal, 12)
        .padding(.bottom, 8)
    }

    private var faellig: Bool {
        (zustand.live?.selfState?.dueInSec ?? 600) < 120
    }

    // MARK: - Reiter

    @ViewBuilder private var inhalt: some View {
        switch reiter {
        case 0: LagePanel()
        case 1:
            if zustand.istZielperson {
                MissionPanel()
            } else {
                PuzzlePanel()
            }
        case 2: MittelPanel()
        default: RadioPanel()
        }
    }

    @ViewBuilder private var meldungen: some View {
        VStack(spacing: 6) {
            if let fehler = zustand.fehler {
                melder(fehler, Farben.zielperson)
            }
            if let text = zustand.meldung {
                melder(text, Farben.gut)
            }
        }
        .padding(.bottom, 8)
    }

    private func melder(_ text: String, _ farbe: Color) -> some View {
        Text(text)
            .font(.footnote)
            .padding(.horizontal, 14)
            .padding(.vertical, 8)
            .background(Color(.secondarySystemBackground))
            .overlay(alignment: .leading) { Rectangle().fill(farbe).frame(width: 2) }
            .clipShape(RoundedRectangle(cornerRadius: 8))
            .padding(.horizontal, 12)
            .onTapGesture {
                zustand.fehler = nil
                zustand.meldung = nil
            }
    }
}

/// Karte oben, Hinweise und Teamliste darunter.
struct LagePanel: View {
    @EnvironmentObject var zustand: AppState

    var body: some View {
        VStack(spacing: 0) {
            MapPanel(serverUrl: zustand.serverUrl, feld: zustand.feld, live: zustand.live)
                .frame(maxWidth: .infinity)
                .frame(height: 300)
                // Die Kartendaten stammen von OpenStreetMap und stehen unter
                // der ODbL. Sie zu nennen ist keine Höflichkeit, sondern
                // Bedingung – und der Kachelspeicher des Servers ändert
                // daran nichts.
                .overlay(alignment: .bottomLeading) {
                    Text("© OpenStreetMap-Mitwirkende")
                        .font(.caption2)
                        .padding(.horizontal, 5)
                        .padding(.vertical, 2)
                        .background(.black.opacity(0.55))
                        .foregroundStyle(.white.opacity(0.9))
                        .padding(6)
                }

            ScrollView {
                VStack(alignment: .leading, spacing: 12) {
                    if let start = zustand.live?.selfState?.start {
                        Tafel(titel: "Euer Startpunkt", farbe: Farben.zentrale) {
                            Text(String(format: "#%02d %@", start.number ?? 0, start.name ?? ""))
                                .font(.headline)
                            Text("Dorthin geht es vor dem Start. Das Spiel läuft erst, "
                                 + "wenn die Zentrale es startet.")
                                .font(.footnote)
                                .foregroundStyle(.secondary)
                        }
                    }

                    if let sperren = zustand.live?.selfState?.lockouts, !sperren.isEmpty {
                        Tafel(titel: "Sperre", farbe: Farben.zielperson) {
                            ForEach(Array(sperren.enumerated()), id: \.offset) { _, sperre in
                                Text("\(sperre.reason ?? "Gesperrt") — noch "
                                     + Zeit.uhr(sperre.leftSec ?? 0))
                                    .font(.footnote)
                            }
                        }
                    }

                    if zustand.istFahndung {
                        Tafel(titel: "Hinweise") {
                            if zustand.intel.isEmpty {
                                Text("Noch nichts. Hinweise entstehen, wenn ihr Rätsel löst.")
                                    .font(.footnote)
                                    .foregroundStyle(.secondary)
                            } else {
                                ForEach(zustand.intel) { hinweis in
                                    HStack(alignment: .top, spacing: 8) {
                                        Circle()
                                            .fill(Farben.fuerHinweis(hinweis.category))
                                            .frame(width: 8, height: 8)
                                            .padding(.top, 6)
                                        VStack(alignment: .leading, spacing: 2) {
                                            Text(hinweis.text ?? "")
                                                .font(.footnote)
                                            Text(frische(hinweis))
                                                .font(.caption2)
                                                .foregroundStyle(.secondary)
                                        }
                                    }
                                }
                            }
                        }
                    }

                    Tafel(titel: "Wer im Feld ist") {
                        ForEach(zustand.live?.positions ?? []) { p in
                            HStack {
                                Circle()
                                    .fill(Farben.fuerRolle(p.role))
                                    .frame(width: 8, height: 8)
                                Text(p.display ?? p.callsign ?? "")
                                    .font(.footnote)
                                Spacer()
                                Text(alter(p))
                                    .font(.caption2)
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }

                    if let ausgang = zustand.live?.outcome, let sieger = ausgang.winner,
                       !sieger.isEmpty {
                        Tafel(titel: "Das Spiel ist entschieden", farbe: Farben.zentrale) {
                            Text(ausgang.reason ?? "")
                                .font(.footnote)
                        }
                    }
                }
                .padding(12)
            }
        }
    }

    private func frische(_ h: IntelItem) -> String {
        switch h.freshness {
        case "hot": return "heiße Spur"
        case "warm": return "laue Spur"
        default: return "kalte Spur"
        }
    }

    private func alter(_ p: LivePosition) -> String {
        let sek = p.ageSec ?? 0
        if p.stale == true { return "veraltet" }
        if sek < 60 { return "gerade eben" }
        return "vor \(sek / 60) min"
    }
}
