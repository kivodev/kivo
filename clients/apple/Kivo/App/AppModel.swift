import KivoCore
import KivoUI
import Observation
import UIKit

/// The app's state for its life: the core, where it says the app is, and
/// this device's settings. While the core is signed in to a server, that
/// server's ServerModel.
@MainActor
@Observable
final class AppModel {
    /// The subtitles this app shows, for the capability report: plain text
    /// as the stream's renditions, which AVPlayer draws, and PGS and styled
    /// ASS as files the overlay draws (SubtitleOverlay).
    static let subtitles = [
        SubtitleEntry(codec: "webvtt", delivery: ["stream"], containers: nil),
        SubtitleEntry(codec: "ass", delivery: ["external"], containers: nil),
        SubtitleEntry(codec: "hdmv_pgs_subtitle", delivery: ["external"], containers: nil),
    ]

    let core: KivoClient
    let display: Display
    /// The downloads' background transfers, for the app's life.
    let transport: BackgroundTransport
    /// The device's network, told to the downloads.
    @ObservationIgnored private let connectivity: Connectivity
    /// Where the app is, which the root shows.
    @ObservationIgnored private(set) lazy var state = Observed(core.state) { [weak self] in self?.changed($0) }
    /// The server signed in to, if any.
    private(set) var server: ServerModel?
    /// This device's settings the core plays and downloads by.
    var settings: DeviceSettings? { deviceSettings.value }
    @ObservationIgnored private let deviceSettings: Observed<DeviceSettings?>
    /// This device's settings only the app's screens follow, kept in UserDefaults.
    var theme = Theme(rawValue: UserDefaults.standard.string(forKey: AppSettingsKey.theme) ?? "") ?? .system {
        didSet { UserDefaults.standard.set(theme.rawValue, forKey: AppSettingsKey.theme) }
    }
    var subtitleSize = SubtitleSize(rawValue: UserDefaults.standard.string(forKey: AppSettingsKey.subtitleSize) ?? "") ?? .medium {
        didSet { UserDefaults.standard.set(subtitleSize.rawValue, forKey: AppSettingsKey.subtitleSize) }
    }
    var subtitleBackground = SubtitleBackground(rawValue: UserDefaults.standard.string(forKey: AppSettingsKey.subtitleBackground) ?? "") ?? .shadow {
        didSet { UserDefaults.standard.set(subtitleBackground.rawValue, forKey: AppSettingsKey.subtitleBackground) }
    }
    /// Whether the screen stays on while Downloads shows downloads converting or coming.
    var keepScreenOn = UserDefaults.standard.object(forKey: AppSettingsKey.keepScreenOn) as? Bool ?? true {
        didSet { UserDefaults.standard.set(keepScreenOn, forKey: AppSettingsKey.keepScreenOn) }
    }
    /// A `kivo://link` link to answer in Link a device once a server is
    /// signed in: the system Camera's way in from another device's QR code.
    var pendingLink: String?

    @ObservationIgnored private let device: AppleDevice
    @ObservationIgnored private var started = false

    init(transport: BackgroundTransport) {
        let directory = URL.applicationSupportDirectory
        try? FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        #if DEBUG
        // UI tests launch the app with the client ID it signs in as.
        let clientId = ProcessInfo.processInfo.environment["KIVO_CLIENT_ID"]
        #else
        let clientId: String? = nil
        #endif
        self.transport = transport
        device = AppleDevice(
            directory: directory.path(percentEncoded: false),
            name: AppleIdentity.modelName,
            identity: AppleIdentity.current,
            clientId: clientId,
            subtitles: Self.subtitles,
            // The engine plays any audio track of a file (AVPlayerEngine).
            trackSelection: true,
            transport: transport
        )
        let core = KivoClient(device: device)
        transport.events = { event in core.transferChanged(event: event) }
        connectivity = Connectivity(core: core)
        self.core = core
        display = core.display
        deviceSettings = Observed(nil, core.settings.map { $0 as DeviceSettings? })
        _ = state
    }

    /// Has the core open the last server signed in to, or sign in, once
    /// the downloads' transfers under way are known.
    func start() async {
        guard !started else { return }
        started = true
        await transport.prepare()
        core.start()
    }

    /// The signed-in server's model, made as the core opens a server and closed as it leaves it.
    private func changed(_ state: AppState) {
        var session: KivoSession?
        if case let .signedIn(signedIn) = onEnum(of: state) { session = signedIn.session }
        guard server?.session !== session else { return }
        server?.close()
        server = session.map { ServerModel(session: $0, app: self) }
    }

    /// The downloads' background refresh: starts the transfers of what the
    /// server finished preparing, and asks for the refresh again while
    /// anything is still there.
    func refreshDownloads() async {
        await start()
        var session: KivoSession?
        for await state in core.state {
            if case .starting = onEnum(of: state) { continue }
            if case let .signedIn(signedIn) = onEnum(of: state) { session = signedIn.session }
            break
        }
        guard let downloads = session?.downloads else { return }
        if (try? await downloads.startReady())?.boolValue == true {
            DownloadRefresh.schedule()
        }
    }

    /// Opens a `kivo://link?code=…&server=…` link in Link a device. When
    /// it names a saved server other than the one signed in, that server is
    /// signed in first, if the device still holds its sign-in.
    func open(_ url: URL) async {
        guard url.scheme == "kivo", url.host() == "link" else { return }
        let other = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems?.first { $0.name == "server" }?.value
        if let other, let current = server, current.session.savedServer.value.id != other {
            let saved = try? await core.savedServers().first { $0.id == other }
            if let saved, let session = try? await core.restore(server: saved), session.isSignedIn {
                core.signedIn(session: session)
            }
        }
        pendingLink = url.absoluteString
    }
}
