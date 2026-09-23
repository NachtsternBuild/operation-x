import SwiftUI

@main
struct OperationXApp: App {
    @StateObject private var zustand = AppState()

    var body: some Scene {
        WindowGroup {
            Wurzel()
                .environmentObject(zustand)
                .environmentObject(zustand.ortung)
                .preferredColorScheme(.dark)
        }
    }
}

/// Drei Zustände, mehr gibt es nicht: verbinden, anmelden, spielen.
struct Wurzel: View {
    @EnvironmentObject var zustand: AppState

    var body: some View {
        Group {
            switch zustand.stufe {
            case .beitritt: JoinView()
            case .anmeldung: LoginView()
            case .feld: FieldView()
            }
        }
        .sheet(isPresented: $zustand.einweisungZeigen) {
            OnboardingView()
                .environmentObject(zustand)
                .interactiveDismissDisabled()
        }
    }
}
