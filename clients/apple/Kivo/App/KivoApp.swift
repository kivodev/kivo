import KivoUI
import SwiftUI

@main
struct KivoApp: App {
    @UIApplicationDelegateAdaptor private var delegate: AppDelegate
    @State private var app = AppModel(transport: .shared)

    init() {
        ArtworkSource.configureCaches()
    }

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(app)
                .modifier(AppTheme(theme: app.theme))
                .task { await app.start() }
                .onOpenURL { url in Task { await app.open(url) } }
        }
        .commands {
            TabCommands(app: app)
            PlaybackCommands(app: app)
        }
        .backgroundTask(.appRefresh(DownloadRefresh.identifier)) {
            await app.refreshDownloads()
        }
    }
}

/// Google Cast, set up at launch; the orientations the interface may
/// take, which the Cast remote narrows to upright as it opens (Orientation);
/// and the downloads' background sessions' events when iOS launches the
/// app to deliver them.
final class AppDelegate: NSObject, UIApplicationDelegate {
    func application(_ application: UIApplication, didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil) -> Bool {
        #if DEBUG
        if CastPreview.scene != nil { return true }
        #endif
        GoogleCastLink.configure()
        return true
    }

    func application(_ application: UIApplication, supportedInterfaceOrientationsFor window: UIWindow?) -> UIInterfaceOrientationMask {
        MainActor.assumeIsolated { Orientation.allowed }
    }

    func application(_ application: UIApplication, handleEventsForBackgroundURLSession identifier: String, completionHandler: @escaping () -> Void) {
        guard BackgroundTransport.identifiers.contains(identifier) else { return }
        BackgroundTransport.shared.reconnect(identifier: identifier, completion: completionHandler)
    }
}

/// The theme's palette; System's is the one for iOS's appearance.
private struct AppTheme: ViewModifier {
    let theme: Theme
    @Environment(\.colorScheme) private var scheme

    func body(content: Content) -> some View {
        let palette: KivoPalette = switch theme {
        case .system: scheme == .light ? .light : .dark
        case .dark: .dark
        case .light: .light
        }
        content.kivoTheme(palette, followsSystem: theme == .system)
    }
}

/// Sign-in or the signed-in app, by where the app is; the background alone
/// while it starts. A server the app doesn't work with shows which of them
/// needs an update in place of its screens; with downloads on the device,
/// the tabs stay, for Downloads.
struct RootView: View {
    @Environment(AppModel.self) private var app
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        ZStack {
            palette.bg.ignoresSafeArea()
            switch app.phase {
            case .starting:
                EmptyView()
            case let .signedOut(saved):
                SignInView(saved: saved)
            case let .signedIn(server):
                if let problem = server.incompatibility, server.downloads.isEmpty {
                    CompatibilityView(problem: problem, retry: server.checkCompatibility, changeServer: app.changeServer)
                } else {
                    SignedInView()
                        .environment(server)
                        .id(ObjectIdentifier(server))
                }
            }
        }
    }
}
