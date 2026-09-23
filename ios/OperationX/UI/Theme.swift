import SwiftUI

/// Die Anmutung des Spiels.
///
/// Dieselbe Festlegung wie in der Android-Fassung: Das System bestimmt die
/// Anmutung, das Spiel die Bedeutung. Die drei Parteifarben bleiben fest,
/// weil sie Information tragen – wer sein Gerät auf ein anderes Farbschema
/// stellt, darf nicht plötzlich die Fahndung in der Farbe der Zielperson
/// sehen.
enum Farben {
    static let zielperson = Color(red: 1.00, green: 0.36, blue: 0.28)   // #ff5c47
    static let fahndung   = Color(red: 0.21, green: 0.75, blue: 0.84)   // #35c0d6
    static let zentrale   = Color(red: 0.85, green: 0.64, blue: 0.25)   // #d9a441
    static let gut        = Color(red: 0.35, green: 0.82, blue: 0.63)   // #5ad1a0
    static let warnung    = Color(red: 0.90, green: 0.63, blue: 0.24)   // #e5a03d

    static func fuerRolle(_ rolle: String?) -> Color {
        switch rolle {
        case "misterx": return zielperson
        case "hq": return zentrale
        default: return fahndung
        }
    }

    /// Hinweisqualität: grün belastbar, gelb ungenau, rot vage.
    static func fuerHinweis(_ kategorie: String?) -> Color {
        switch kategorie {
        case "green": return gut
        case "yellow": return warnung
        default: return zielperson
        }
    }
}

/// Zahlen, die man vergleicht, gehören in eine Schrift mit festen Breiten.
extension View {
    func ziffern() -> some View {
        self.font(.system(.body, design: .monospaced))
            .monospacedDigit()
    }
}

/// Ein Abschnitt mit Überschrift, wie ihn alle Reiter benutzen.
struct Tafel<Inhalt: View>: View {
    let titel: String
    let farbe: Color
    let inhalt: Inhalt

    init(titel: String, farbe: Color = .secondary,
         @ViewBuilder inhalt: () -> Inhalt) {
        self.titel = titel
        self.farbe = farbe
        self.inhalt = inhalt()
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(titel.uppercased())
                .font(.caption)
                .kerning(1.4)
                .foregroundStyle(farbe)
            inhalt
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(14)
        .background(Color(.secondarySystemBackground))
        .clipShape(RoundedRectangle(cornerRadius: 14))
    }
}

/// Eine Meldung, die nicht im Weg steht, aber auch nicht zu übersehen ist.
struct Hinweiszeile: View {
    let text: String
    var farbe: Color = .secondary

    var body: some View {
        HStack(alignment: .top, spacing: 8) {
            Rectangle()
                .fill(farbe)
                .frame(width: 2)
            Text(text)
                .font(.subheadline)
                .foregroundStyle(.secondary)
        }
        .fixedSize(horizontal: false, vertical: true)
    }
}
