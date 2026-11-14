import KivoCore
import KivoUI
import SwiftUI
import UIKit

/// The app, or an empty window while it hosts the unit tests: no session
/// and no discovery.
@main
enum KivoTVMain {
    static func main() {
        if Launch.isTestHost {
            TestHostApp.main()
        } else {
            KivoTVApp.main()
        }
    }
}

struct KivoTVApp: App {
    @State private var app = AppModel()
    @Environment(\.scenePhase) private var scenePhase

    init() {
        ArtworkSource.configureCaches()
        // What's typed in Search reads in the primary text colour, not the placeholder's grey.
        UITextField.appearance(whenContainedInInstancesOf: [UISearchBar.self]).textColor = .white
    }

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(app)
                // The system's focus, white with dark text, rather than the accent's.
                .environment(\.kivoPalette, .dark)
                .preferredColorScheme(.dark)
                .task { app.start() }
                .onOpenURL { app.open($0) }
        }
        // The core runs the live events while the app is in the front, and
        // Top Shelf reads Continue Watching again as it comes back.
        .onChange(of: scenePhase, initial: true) { _, phase in
            app.core.setForeground(active: phase == .active)
            if phase == .active { app.server?.refreshTopShelf() }
        }
    }
}

/// Sign-in, the saved server that can't be reached, or the signed-in app,
/// by where the core says the app is; a spinner while it opens its server.
/// A server the app doesn't work with shows which of them needs an update
/// in place of its screens.
struct RootView: View {
    @Environment(AppModel.self) private var app
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        ZStack {
            palette.bg.ignoresSafeArea()
            switch onEnum(of: app.state.value) {
            case .starting:
                ProgressView()
            case let .signedOut(signedOut):
                SignInView(saved: signedOut.server)
            case let .unreachable(unreachable):
                MessageView(
                    String(localized: "Can't reach \(unreachable.server.name)."),
                    sentence: String(localized: "Check your connection, or choose another server."),
                    action: (String(localized: "Retry"), app.core.retry),
                    secondary: (String(localized: "Change server"), app.core.changeServer)
                )
            case .signedIn:
                if let server = app.server {
                    if let problem = server.incompatibility {
                        CompatibilityView(problem: problem, retry: server.checkCompatibility, changeServer: app.core.changeServer)
                    } else {
                        MainView()
                            .environment(server)
                            .environment(\.artworkSource, server.artwork)
                            .id(ObjectIdentifier(server))
                    }
                }
            }
        }
    }
}
