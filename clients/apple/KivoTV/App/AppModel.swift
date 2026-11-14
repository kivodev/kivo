import KivoCore
import KivoUI
import Observation
import UIKit

/// The Apple TV app's state for its life: the core, where it says the app
/// is, and this device's settings. While the core is signed in to a server,
/// that server's ServerModel. The app keeps no downloads.
@MainActor
@Observable
final class AppModel {
    let core: KivoClient
    let display: Display
    /// The device's network, told to the core.
    @ObservationIgnored private let connectivity: Connectivity
    /// Where the app is, which the root shows.
    @ObservationIgnored private(set) lazy var state = Observed(core.state) { [weak self] in self?.changed($0) }
    /// The server signed in to, if any.
    private(set) var server: ServerModel?
    /// This device's settings the core plays by.
    var settings: DeviceSettings? { deviceSettings.value }
    @ObservationIgnored private let deviceSettings: Observed<DeviceSettings?>
    /// This device's settings only the app's player follows, kept in UserDefaults.
    var subtitleSize = SubtitleSize(rawValue: UserDefaults.standard.string(forKey: AppSettingsKey.subtitleSize) ?? "") ?? .medium {
        didSet { UserDefaults.standard.set(subtitleSize.rawValue, forKey: AppSettingsKey.subtitleSize) }
    }
    var subtitleBackground = SubtitleBackground(rawValue: UserDefaults.standard.string(forKey: AppSettingsKey.subtitleBackground) ?? "") ?? .shadow {
        didSet { UserDefaults.standard.set(subtitleBackground.rawValue, forKey: AppSettingsKey.subtitleBackground) }
    }
    /// A Top Shelf item's link, opened once a server is signed in.
    var pendingLink: TopShelfLink?

    @ObservationIgnored private var started = false

    init() {
        // An Apple TV keeps an app's files in its caches, which the system
        // may clear when space runs short; the session tokens are in the Keychain.
        let directory = URL.cachesDirectory
        try? FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        #if DEBUG
        // UI tests launch the app with the client ID it signs in as.
        let clientId = ProcessInfo.processInfo.environment["KIVO_CLIENT_ID"]
        #else
        let clientId: String? = nil
        #endif
        let device = AppleDevice(
            directory: directory.path(percentEncoded: false),
            name: AppleIdentity.modelName,
            identity: AppleIdentity.current,
            clientId: clientId,
            subtitles: SubtitleOverlayView.shown,
            // The engine plays any audio track of a file (AVPlayerEngine).
            trackSelection: true,
            transport: nil
        )
        let core = KivoClient(device: device)
        connectivity = Connectivity(core: core)
        self.core = core
        display = core.display
        deviceSettings = Observed(nil, core.settings.map { $0 as DeviceSettings? })
        _ = state
    }

    /// Has the core open the last server signed in to, or sign in.
    func start() {
        guard !started else { return }
        started = true
        core.start()
    }

    /// The signed-in server's model, made as the core opens a server and
    /// closed as it leaves it; signed out, Top Shelf empties.
    private func changed(_ state: AppState) {
        var session: KivoSession?
        if case let .signedIn(signedIn) = onEnum(of: state) { session = signedIn.session }
        if case .signedOut = onEnum(of: state) { TopShelf.clear() }
        guard server?.session !== session else { return }
        server?.close()
        server = session.map { ServerModel(session: $0, app: self) }
    }

    /// Opens a Top Shelf item's `kivo://work/{id}` or `kivo://play/{id}`.
    func open(_ url: URL) {
        pendingLink = TopShelfLink(url)
    }
}
