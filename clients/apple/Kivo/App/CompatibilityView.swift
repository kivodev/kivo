import KivoCore
import KivoUI
import SwiftUI

/// Why the app and a server don't work together, as its screen says it: a
/// title and a sentence. Nil when they do.
struct Incompatibility: Equatable {
    let title: String
    let sentence: String

    init?(_ compatibility: Compatibility, server: String) {
        switch onEnum(of: compatibility) {
        case .compatible:
            return nil
        case let .serverTooOld(old):
            title = String(localized: "This server needs an update")
            sentence = String(localized: "Kivo \(old.appVersion) works with server \(old.minServerVersion) or newer. \(server) runs \(old.serverVersion). Ask whoever runs it to update.")
        case let .appTooOld(old):
            title = String(localized: "Update Kivo")
            sentence = String(localized: "\(server) needs Kivo \(old.minAppVersion) or newer. You have \(old.appVersion).")
        }
    }
}

/// A server the app doesn't work with, in place of its sign-in or its
/// screens: which of the two needs an update, with Try again, which asks
/// the server what it is again, and Change server.
struct CompatibilityView: View {
    let problem: Incompatibility
    let retry: () async -> Void
    let changeServer: () -> Void
    @Environment(\.kivoPalette) private var palette
    @State private var retrying = false

    var body: some View {
        MessageView(
            problem.title,
            sentence: problem.sentence,
            action: (String(localized: "Try again"), {
                guard !retrying else { return }
                retrying = true
                Task {
                    await retry()
                    retrying = false
                }
            }),
            secondary: (String(localized: "Change server"), changeServer)
        )
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(palette.bg)
        .accessibilityIdentifier("incompatible")
    }
}
