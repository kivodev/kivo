import KivoCore
import KivoUI
import Observation
import UIKit

/// The app's state for its life: the core, this device's settings, and
/// whether a server is signed in. A signed-in server is a ServerModel; signing out, by the user, a `401` or the live events
/// closing with 4401, returns to sign-in with that server filled in.
@MainActor
@Observable
final class AppModel {
    enum Phase {
        case starting
        /// Sign-in, offering the server last signed in to, if any.
        case signedOut(SavedServer?)
        case signedIn(ServerModel)
    }

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
    private(set) var phase: Phase = .starting
    /// The server signed in to, if any.
    var server: ServerModel? {
        if case let .signedIn(server) = phase { server } else { nil }
    }
    /// This device's settings the core plays and downloads by.
    private(set) var settings: DeviceSettings?
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
    }

    /// Restores the last server signed in to, or goes to sign-in, once the
    /// downloads' transfers under way are known.
    func start() async {
        guard !started else { return }
        started = true
        await takeAppSettings()
        await transport.prepare()
        Task { [weak self, core] in
            for await settings in core.settings {
                self?.settings = settings
            }
        }
        var saved: SavedServer?
        for await servers in core.servers {
            saved = servers.last
            break
        }
        guard let saved else {
            phase = .signedOut(nil)
            return
        }
        do {
            let server = try await core.restore(server: saved)
            if server.isSignedIn {
                signedIn(server)
            } else {
                phase = .signedOut(saved)
            }
        } catch {
            phase = .signedOut(saved)
        }
    }

    /// Keeps the settings an earlier version kept in the core here, once.
    private func takeAppSettings() async {
        guard let taken = try? await device.takeAppSettings() else { return }
        if let theme = taken["theme"].flatMap(Theme.init(rawValue:)) { self.theme = theme }
        if let size = taken["subtitleSize"].flatMap(SubtitleSize.init(rawValue:)) { subtitleSize = size }
        if let background = taken["subtitleBackground"].flatMap(SubtitleBackground.init(rawValue:)) { subtitleBackground = background }
        if let on = taken["keepScreenOn"] { keepScreenOn = on == "true" }
    }

    /// The downloads' background refresh: starts the transfers of what the
    /// server finished preparing, and asks for the refresh again while
    /// anything is still there.
    func refreshDownloads() async {
        await start()
        guard let downloads = server?.session.downloads else { return }
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
        if let other, case let .signedIn(current) = phase, current.session.savedServer.value.id != other {
            var saved: SavedServer?
            for await servers in core.servers {
                saved = servers.first { $0.id == other }
                break
            }
            if let saved, let session = try? await core.restore(server: saved), session.isSignedIn {
                current.close()
                signedIn(session)
            }
        }
        pendingLink = url.absoluteString
    }

    /// Leaves the signed-in server for sign-in, to choose a server again.
    /// The device stays signed in to it, and keeps its downloads.
    func changeServer() {
        guard case let .signedIn(current) = phase else { return }
        current.close()
        phase = .signedOut(nil)
    }

    /// The device holds a token for [session]'s server: show the app until it's signed out.
    func signedIn(_ session: KivoSession) {
        let model = ServerModel(session: session, app: self)
        phase = .signedIn(model)
        Task { [weak self] in
            try? await session.awaitSignOut()
            guard let self, case let .signedIn(current) = self.phase, current === model else { return }
            model.close()
            self.phase = .signedOut(session.savedServer.value)
        }
    }
}
