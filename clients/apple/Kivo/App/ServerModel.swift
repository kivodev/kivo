import KivoCore
import KivoUI
import Observation
import SwiftUI

/// The signed-in app's shared state: the server, the user, their
/// libraries in their order, whether the server answers and this server's
/// downloads; and the models that share it: where the app is
/// (``Navigation``), the video player (``PlayerHost``), the book player,
/// which plays while the app browses, and casting.
@MainActor
@Observable
final class ServerModel {
    let session: KivoSession
    unowned let app: AppModel
    /// The session as the core says it is: the server, the user, whether it answers, whether the app works with it, and its address.
    @ObservationIgnored private let state: Observed<SessionState>
    /// The server's name, as it last said it.
    var serverName: String { state.value.server.name }
    /// Who the app is signed in as, kept on the device, so it shows without the server.
    var user: SignedInUser? { state.value.user }
    /// Every library the user sees, in their order.
    private(set) var libraries: [Library] = []
    /// The user's lists of the kinds this app knows, My List first.
    private(set) var lists: [WorkList] = []
    /// Why the lists couldn't be read, when the server refused it.
    private(set) var listsFailure: RequestProblem?
    /// The server isn't answering reads, which retry until it does.
    var unreachable: Bool { state.value.unreachable }
    /// Why the app and the server don't work together, while they don't: its screen shows in place of the server's.
    var incompatibility: Incompatibility? { Incompatibility(state.value.compatibility, server: serverName) }
    /// The base URL the server is reached at now, which artwork follows.
    var baseUrl: String { state.value.address }
    /// The tab showing, its pages, and the sheets over the tabs.
    let navigation: Navigation
    /// The video player and its screen, made once the rest is.
    @ObservationIgnored private(set) lazy var player = PlayerHost(server: self)
    /// The book player, with its mini player while a book is loaded.
    let book: BookPlayer
    /// Casting to a TV, with its mini controller and remote while something plays there.
    let cast: CastModel
    /// This server's downloads on the device, newest first.
    var downloads: [DownloadItem] { onDevice?.items.value ?? [] }
    /// The works whose downloads are being asked of the server, before they're listed.
    var asking: Set<String> { onDevice?.asking.value ?? [] }
    /// How many downloads the availability check deleted, until Downloads says so.
    var deletedDownloads: Int32 { onDevice?.deleted.value ?? 0 }
    /// Why a download asked for from a long press couldn't start, or a
    /// mark on a work failed, until it's said.
    var problem: String?

    /// The downloads the device keeps of this server, as the core lists them.
    @ObservationIgnored private let onDevice: (items: Observed<[DownloadItem]>, asking: Observed<Set<String>>, deleted: Observed<Int32>)?
    @ObservationIgnored private var listsTask: Task<Void, Never>?
    @ObservationIgnored private var librariesTask: Task<Void, Never>?
    @ObservationIgnored private var marksTask: Task<Void, Never>?

    init(session: KivoSession, app: AppModel) {
        self.session = session
        self.app = app
        state = Observed(session.state)
        book = BookPlayer(session: session, artwork: { Self.artwork(session, at: session.baseUrl) }, display: app.display)
        cast = CastModel(session: session, core: app.core, display: app.display)
        navigation = Navigation(serverId: session.savedServer.value.id)
        if let downloads = session.downloads {
            onDevice = (Observed([], downloads.items), Observed([], downloads.asking), Observed(0, downloads.deleted.map { $0.int32Value }))
        } else {
            onDevice = nil
        }
        // Opened without the server, for what the device keeps of it: its downloads.
        if session.unreachable.value.boolValue { navigation.tab = .downloads }
        // Casting hands over to and from the player from the start.
        _ = player
        reloadLibraries()
        listsTask = Task { [weak self, session] in
            for await load in session.library.lists() {
                if let problem = load.problemOrNull(), !problem.isUnreachable { self?.listsFailure = problem }
                guard let lists = load.getOrNull() as? [WorkList] else { continue }
                self?.lists = lists.filter { $0.kind == .manual || $0.kind == .smart }
            }
        }
        marksTask = Task { [weak self, session] in
            for await failed in session.library.marks.problems {
                self?.problem = failed.sentence
            }
        }
    }

    /// The download of [workId], if there is one.
    func download(of workId: String) -> DownloadItem? {
        downloads.first { $0.workId == workId }
    }

    /// Downloads [workId] at the download quality setting, keeping why in
    /// ``problem`` when it can't start.
    func startDownload(_ workId: String) {
        Task {
            guard let start = try? await session.downloads?.download(choice: PlayChoice(workId: workId)) else { return }
            if let sentence = display.downloadProblem(start: start) {
                problem = sentence
            } else {
                Haptics.success()
            }
        }
    }

    /// Removes [workId]'s download, whatever its state.
    func removeDownload(of workId: String) {
        guard let item = download(of: workId) else { return }
        Haptics.success()
        Task { tell(try? await session.downloads?.delete(id: item.id)) }
    }

    /// Keeps why [done] failed in ``problem``, when it did.
    func tell<T: AnyObject>(_ done: Done<T>?) {
        if let failed = done?.problemOrNull() { problem = failed.sentence }
    }

    /// The download of [workId] once it's ready, which Play plays rather than streaming.
    func readyDownload(of workId: String) -> DownloadItem? {
        download(of: workId).flatMap { $0.status is DownloadStatusReady ? $0 : nil }
    }

    var display: Display { app.display }

    var artwork: ArtworkSource {
        Self.artwork(session, at: baseUrl)
    }

    private static func artwork(_ session: KivoSession, at baseUrl: String) -> ArtworkSource {
        ArtworkSource(baseURL: baseUrl, authorization: { [session] in session.authorization(url: $0) }, serverId: session.savedServer.value.id)
    }

    var pinnedLibraries: [Library] { libraries.filter(\.pinned) }

    var otherLibraries: [Library] { libraries.filter { !$0.pinned } }

    /// The library a work is in, by the [libraryId] its card or page names, if it's one of the user's.
    func library(of libraryId: String?) -> Library? {
        guard let libraryId else { return nil }
        return libraries.first { $0.id == libraryId }
    }

    /// Reads the libraries again, as after the user changes their order or which are pinned, then follows them.
    func reloadLibraries() {
        librariesTask?.cancel()
        librariesTask = Task { [weak self, session] in
            for await load in session.library.libraries() {
                guard let libraries = load.getOrNull() as? [Library] else { continue }
                self?.libraries = libraries
                self?.navigation.librariesRead(libraries)
            }
        }
    }

    /// Plays a book in the book player, from where the user left it or from [positionMs], such as a chapter's start.
    func listen(_ workId: String, at positionMs: Int64? = nil) {
        book.play(workId, at: positionMs)
    }

    /// Asks the server what it is again, as Try again does on a server the app doesn't work with.
    func checkCompatibility() async {
        _ = try? await session.serverInfo()
    }

    /// Signed out: everything stops.
    func close() {
        listsTask?.cancel()
        librariesTask?.cancel()
        marksTask?.cancel()
        player.close()
        book.close()
        cast.close()
        session.close()
    }

    func signOut() async {
        try? await session.signOut()
    }
}
