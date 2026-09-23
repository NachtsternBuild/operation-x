// swift-tools-version: 5.9
import PackageDescription

// Prüfstand für die iOS-Verschlüsselung, auf Linux.
//
// Der Trick steckt im Ziel namens "CryptoKit": Es reicht swift-crypto durch,
// das dieselbe Schnittstelle hat wie Apples CryptoKit. Damit lässt sich die
// echte Funk.swift aus der App unverändert übersetzen — ohne Mac, ohne iPhone.
let package = Package(
    name: "Funkprobe",
    platforms: [.macOS(.v13)],
    dependencies: [
        .package(url: "https://github.com/apple/swift-crypto.git", from: "3.0.0"),
    ],
    targets: [
        .target(name: "CryptoKit", dependencies: [
            .product(name: "Crypto", package: "swift-crypto"),
        ]),
        .executableTarget(name: "Funkprobe", dependencies: ["CryptoKit"]),
    ]
)
