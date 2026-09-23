import Foundation
import CryptoKit

/// Der Lagefunk: eine zweite Verschlüsselung zwischen Telefon und Spielserver.
///
/// Warum, wo doch alles über HTTPS läuft: Der Tunnel endet nicht auf dem
/// Spielserver, sondern bei seinem Betreiber. Dort wird entschlüsselt und neu
/// verschlüsselt – das ist der Sinn eines Tunnels und der Grund, warum er ohne
/// eigenes Zertifikat funktioniert. Es heißt aber auch: Wer ihn betreibt, sieht
/// den Klartext. Bei diesem Spiel wäre das der Standortverlauf von zwanzig
/// Leuten über sechs Stunden, darunter Jugendliche, und gespielt wird dort, wo
/// sie wohnen.
///
/// Was das nicht leistet: Der Spielserver sieht weiterhin alles – er ist das
/// Spiel. Und wer mitliest, sieht weiterhin, wann wie viel an welche Adresse
/// geht.
///
/// Verfahren: ECDH auf P-256, daraus per HKDF-SHA256 ein Sitzungsschlüssel,
/// damit AES-256-GCM. Dasselbe steht in Go (pkg/crypt), im Browser
/// (web/src/lib/funk-kern.js) und in Kotlin (data/Funk.kt) noch einmal.
///
/// Dass die vier übereinstimmen, prüft ein gemeinsamer Vektor in
/// testdaten/lagefunk.json — feste Eingaben, feste Ausgaben. Für diese
/// Fassung tut das ios/Funkprobe.
///
/// Drei Einzelheiten müssen dabei auf allen vier Seiten übereinstimmen, sonst
/// scheitert die Entschlüsselung lautlos:
///
///   * Das Salz von HKDF ist "eigenerSchlüssel|Serverschlüssel", beide in
///     base64url ohne Auffüllzeichen.
///   * Das Beiwerk (AAD) lautet "METHODE /pfad?abfrage nummer".
///   * Übertragen werden Chiffrat und Siegel hintereinander, der Zufallswert
///     steht in der Kopfzeile.
final class Funk {

    private static let info = "operation-x/lagefunk/1"

    private let schluessel: SymmetricKey
    /// Der eigene öffentliche Schlüssel, wie er in die Kopfzeile geht.
    let publicKey: String
    /// Das Kennzeichen des Servers zum Vergleichen mit der Teamkarte.
    let fingerprint: String

    private var nummer: Int64 = 0
    private let sperre = NSLock()

    // Modulintern statt privat: Der Prüfstand baut damit eine Verschlüsselung
    // aus einem festen Schlüssel und rechnet nach, ob dieselben Bytes
    // herauskommen wie im Server. Von außen bleibt nur aufbauen() erreichbar.
    init(schluessel: SymmetricKey, publicKey: String, fingerprint: String) {
        self.schluessel = schluessel
        self.publicKey = publicKey
        self.fingerprint = fingerprint
    }

    /// Baut die Verschlüsselung zu einem Server auf.
    ///
    /// `serverPublicKey` kommt vom Server selbst. Besser wäre er von der
    /// gedruckten Teamkarte – die entsteht auf dem Rechner der Spielleitung und
    /// läuft nicht durch den Tunnel. Dafür ist das Kennzeichen da: Wer die
    /// Zeichen auf dem Gerät neben die auf der Karte hält und dieselben sieht,
    /// redet wirklich mit diesem Server.
    static func aufbauen(serverPublicKey: String, fingerprint: String) -> Funk? {
        guard let fremdRoh = entschluesselnB64(serverPublicKey),
              let fremd = try? P256.KeyAgreement.PublicKey(x963Representation: fremdRoh)
        else { return nil }

        let eigen = P256.KeyAgreement.PrivateKey()
        let eigenB64 = verschluesselnB64(eigen.publicKey.x963Representation)

        guard let gemeinsam = try? eigen.sharedSecretFromKeyAgreement(with: fremd)
        else { return nil }

        let key = ableiten(
            gemeinsam: gemeinsam.withUnsafeBytes { Data($0) },
            eigenB64: eigenB64,
            serverB64: serverPublicKey
        )

        return Funk(schluessel: key, publicKey: eigenB64, fingerprint: fingerprint)
    }

    /// Leitet den Sitzungsschlüssel aus dem gemeinsamen Geheimnis ab.
    ///
    /// Ein eigener Schritt, obwohl er nur einmal gerufen wird: Der Prüfvektor
    /// steigt genau hier ein. Wäre die Rechnung in aufbauen() eingebacken,
    /// müsste er sie nachbilden — und prüfte dann eine gleichwertige Rechnung
    /// statt dieser.
    ///
    /// Das Salz bindet den Schlüssel an genau dieses Paar. Reihenfolge und
    /// Trennzeichen stehen so auch im Server.
    static func ableiten(gemeinsam: Data, eigenB64: String, serverB64: String) -> SymmetricKey {
        HKDF<SHA256>.deriveKey(
            inputKeyMaterial: SymmetricKey(data: gemeinsam),
            salt: Data("\(eigenB64)|\(serverB64)".utf8),
            info: Data(info.utf8),
            outputByteCount: 32
        )
    }

    /// Die laufende Nummer gegen Wiedereinspielen.
    ///
    /// Der Server führt dazu ein Fenster wie IPsec: Er nimmt auch Nummern an,
    /// die kleiner als die zuletzt gesehene sind, solange sie noch nicht
    /// dagewesen sind. Ohne das scheiterten gleichzeitige Anfragen – und
    /// gleichzeitig ist hier der Normalfall.
    func naechsteNummer() -> Int64 {
        sperre.lock()
        defer { sperre.unlock() }
        nummer += 1
        return nummer
    }

    /// Das Beiwerk, mit dem versiegelt wird: Methode, Adresse samt Abfrageteil,
    /// laufende Nummer.
    static func beiwerk(methode: String, pfad: String, nummer: Int64) -> Data {
        Data("\(methode) \(pfad) \(nummer)".utf8)
    }

    /// Verschlüsselt einen Rumpf. Zurück kommen Chiffrat und Zufallswert.
    func sealen(_ klartext: Data, beiwerk: Data) -> (chiffre: Data, nonce: Data)? {
        guard let box = try? AES.GCM.seal(klartext, using: schluessel,
                                          authenticating: beiwerk) else { return nil }
        // Ohne "combined": Dort stünde der Zufallswert mit im Rumpf, er gehört
        // aber in die Kopfzeile – so erwartet es der Server.
        return (box.ciphertext + box.tag, Data(box.nonce))
    }

    /// Entschlüsselt eine Antwort.
    func oeffnen(_ chiffre: Data, nonce: Data, beiwerk: Data) -> Data? {
        guard chiffre.count > 16,
              let n = try? AES.GCM.Nonce(data: nonce) else { return nil }

        let siegel = chiffre.suffix(16)
        let rumpf = chiffre.prefix(chiffre.count - 16)

        guard let box = try? AES.GCM.SealedBox(nonce: n, ciphertext: rumpf, tag: siegel),
              let klar = try? AES.GCM.open(box, using: schluessel, authenticating: beiwerk)
        else { return nil }

        return klar
    }

    // MARK: - base64url ohne Auffüllzeichen

    static func verschluesselnB64(_ roh: Data) -> String {
        roh.base64EncodedString()
            .replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
    }

    static func entschluesselnB64(_ text: String) -> Data? {
        var s = text
            .replacingOccurrences(of: "-", with: "+")
            .replacingOccurrences(of: "_", with: "/")
        while s.count % 4 != 0 { s += "=" }
        return Data(base64Encoded: s)
    }
}
