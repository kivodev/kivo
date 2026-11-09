import KivoCore
import KivoUI
import Observation
import SwiftUI

/// The signed-in app's shared state: the server, the user, their
/// libraries in their order, whether the server answers, the video
/// player, which outlives its screen while it plays in Picture in Picture,
/// the book player, which plays while the app browses, and casting, which
/// takes over Play while connected to a TV.
@MainActor
@Observable
final class ServerModel {
    let session: KivoSession
    unowned let app: AppModel
    /// The server's name, as it last said it.
    private(set) var serverName: String
    /// Who the app is signed in as, kept on the device, so it shows without the server.
    private(set) var user: SignedInUser?
    /// Every library the user sees, in their order.
    private(set) var libraries: [Library] = []
    /// The user's lists of the kinds this app knows, My List first.
    private(set) var lists: [WorkList] = []
    /// Why the lists couldn't be read, when the server refused it.
    private(set) var listsFailure: RequestProblem?
    /// The server isn't answering reads, which retry until it does.
    private(set) var unreachable = false
    /// The base URL the server is reached at now, which artwork follows.
    private(set) var baseUrl: String
    /// The player, while there is one.
    private(set) var playback: PlayerModel?
    /// Whether the player's screen shows; false while it's in Picture in Picture.
    var playerShown = false
    /// The book player, with its mini player while a book is loaded.
    let book: BookPlayer
    /// Casting to a TV, with its mini controller and remote while something plays there.
    let cast: CastModel
    /// The tab showing, and each browsing tab's stack of pages.
    var tab: AppTab = .home
    var paths: [AppTab: [Route]] = [:]
    /// What Search shows, kept while the app moves between tabs.
    var searchQuery = ""
    /// Whether Settings shows, opened from Home's avatar or the sidebar's account row.
    var settingsShown = false
    /// Whether the window is wide enough for the sidebar, where each
    /// library is a tab of its own rather than a page of Libraries.
    private(set) var sidebar = false
    /// The work Add to List shows over the tabs, while it does.
    var addingToList: String?
    /// This server's downloads on the device, newest first.
    private(set) var downloads: [DownloadItem] = []
    /// The works whose downloads are being asked of the server, before they're listed.
    private(set) var asking: Set<String> = []
    /// How many downloads the availability check deleted, until Downloads says so.
    private(set) var deletedDownloads: Int32 = 0
    /// Why a download asked for from a long press couldn't start, until it's said.
    var downloadProblem: String?

    @ObservationIgnored private var tasks: [Task<Void, Never>] = []
    @ObservationIgnored private var librariesTask: Task<Void, Never>?
    @ObservationIgnored private var stopAfterCastMenu = false
    /// Whether the Libraries tab has opened on the page last chosen, once the libraries were first read.
    @ObservationIgnored private var restoredLibrariesPage = false

    init(session: KivoSession, app: AppModel) {
        self.session = session
        self.app = app
        baseUrl = session.baseUrl
        serverName = session.savedServer.value.name
        book = BookPlayer(session: session, artwork: { Self.artwork(session, at: session.baseUrl) }, display: app.display)
        cast = CastModel(session: session, core: app.core, display: app.display)
        user = session.user.value
        cast.connected = { [weak self] _ in self?.handOffToCast() }
        cast.stopped = { [weak self] in self?.castStopped() }
        reloadLibraries()
        tasks.append(Task { [weak self, session] in
            for await load in session.library.lists(counted: false) {
                if let problem = load.problemOrNull(), !problem.isUnreachable { self?.listsFailure = problem }
                guard let lists = load.getOrNull() as? [WorkList] else { continue }
                self?.lists = lists.filter { $0.kind == "manual" || $0.kind == "smart" }
            }
        })
        tasks.append(Task { [weak self, session] in
            for await user in session.user {
                self?.user = user
            }
        })
        tasks.append(Task { [weak self, session] in
            for await saved in session.savedServer {
                self?.serverName = saved.name
            }
        })
        tasks.append(Task { [weak self, session] in
            for await unreachable in session.unreachable {
                self?.unreachable = unreachable.boolValue
            }
        })
        tasks.append(Task { [weak self, session] in
            for await url in session.address {
                self?.baseUrl = String(url)
            }
        })
        if let onDevice = session.downloads {
            tasks.append(Task { [weak self] in
                for await downloads in onDevice.items {
                    self?.downloads = downloads
                }
            })
            tasks.append(Task { [weak self] in
                for await asking in onDevice.asking {
                    self?.asking = asking
                }
            })
            tasks.append(Task { [weak self] in
                for await deleted in onDevice.deleted {
                    self?.deletedDownloads = deleted.int32Value
                }
            })
        }
        tasks.append(Task { [weak self, session] in
            do {
                _ = try await session.account.currentUser()
            } catch {
                // With no server, the app opens on Downloads, and reads the user again once it answers.
                if error.requestProblem.isUnreachable { self?.tab = .downloads }
                try? await session.awaitReachable()
            }
        })
    }

    /// The download of [workId], if there is one.
    func download(of workId: String) -> DownloadItem? {
        downloads.first { $0.workId == workId }
    }

    /// Downloads [workId] at the download quality setting, keeping why in
    /// ``downloadProblem`` when it can't start.
    func startDownload(_ workId: String) {
        Task {
            do {
                _ = try await session.downloads?.download(workId: workId, versionId: nil, audioTrackId: nil, subtitleTrackId: nil, quality: nil)
                Haptics.success()
            } catch {
                downloadProblem = error.downloadSentence
            }
        }
    }

    /// Removes [workId]'s download, whatever its state.
    func removeDownload(of workId: String) {
        guard let item = download(of: workId) else { return }
        Haptics.success()
        Task { try? await session.downloads?.delete(id: item.id) }
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

    /// My List, once the lists are read.
    var myList: WorkList? { lists.first(where: \.builtIn) }

    /// What the Libraries tab shows: the library last chosen on this
    /// device, else the first pinned library, else the first library; nil
    /// until the libraries are read.
    var librariesPage: Library? {
        libraries.first { $0.id == lastLibraryId } ?? pinnedLibraries.first ?? libraries.first
    }

    /// The stack [tab] opens [library] with: over the list of libraries in
    /// the Libraries tab, else nothing over the page.
    private func stack(opening library: Library, in tab: AppTab) -> [Route] {
        tab == .libraries ? [.librariesPage(library)] : []
    }

    /// Opens the Libraries tab's list on the page last chosen on this
    /// device, once, when the libraries are first read.
    private func restoreLibrariesPage() {
        guard !restoredLibrariesPage, !libraries.isEmpty else { return }
        restoredLibrariesPage = true
        guard !sidebar, lastLibraryId != nil, paths[.libraries, default: []].isEmpty, let library = librariesPage else { return }
        paths[.libraries] = stack(opening: library, in: .libraries)
    }

    /// Opens [library] in the Libraries tab, and remembers it on this device.
    func open(_ library: Library) {
        lastLibraryId = library.id
    }

    /// Chooses [tab], as the tab bar, the sidebar and the View menu do. A
    /// library is remembered on this device, as choosing it in the
    /// Libraries tab does; choosing Search again while there goes back
    /// to its results.
    func choose(_ tab: AppTab) {
        let again = tab == self.tab
        self.tab = tab
        switch tab {
        case .library(let id):
            if let library = libraries.first(where: { $0.id == id }) { open(library) }
        case .search:
            if again { paths[.search] = [] }
        default: break
        }
    }

    /// The tab [library] shows in: its own beside the sidebar, else Libraries.
    private func tab(of library: Library) -> AppTab {
        sidebar ? .library(library.id) : .libraries
    }

    /// Goes to the top of [library]'s grid, and remembers it on this device.
    func show(_ library: Library) {
        open(library)
        let tab = tab(of: library)
        paths[tab] = stack(opening: library, in: tab)
        self.tab = tab
    }

    /// Switches between the tab bar's Libraries tab and the sidebar's tab
    /// for each library as the window's width crosses between them, keeping the page showing and the pages pushed on it.
    /// The Libraries tab's list of libraries, which the sidebar has
    /// instead, shows in a narrow window only.
    func useSidebar(_ sidebar: Bool) {
        guard sidebar != self.sidebar else { return }
        self.sidebar = sidebar
        let from = tab
        var pushed = paths[from] ?? []
        let to: AppTab
        switch from {
        case .libraries:
            if case .librariesPage(let library) = pushed.first {
                open(library)
                pushed.removeFirst()
            }
            guard let library = librariesPage else { return }
            to = tab(of: library)
        case .library(let id):
            to = .libraries
            if let library = libraries.first(where: { $0.id == id }) {
                open(library)
                pushed.insert(contentsOf: stack(opening: library, in: to), at: 0)
            }
        default:
            return
        }
        paths[from] = nil
        paths[to] = pushed
        tab = to
    }

    /// The library a work is in, by the [libraryId] its card or page names, if it's one of the user's.
    func library(of libraryId: String?) -> Library? {
        guard let libraryId else { return nil }
        return libraries.first { $0.id == libraryId }
    }

    /// Goes to the library a work is in, by the [libraryId] its card or page names.
    func showLibrary(of libraryId: String?) {
        if let library = library(of: libraryId) { show(library) }
    }

    private var lastLibraryKey: String { "lastLibrary.\(session.savedServer.value.id)" }

    private var lastLibraryId: String? {
        get {
            access(keyPath: \.lastLibraryId)
            return UserDefaults.standard.string(forKey: lastLibraryKey)
        }
        set {
            withMutation(keyPath: \.lastLibraryId) {
                UserDefaults.standard.set(newValue, forKey: lastLibraryKey)
            }
        }
    }

    /// Reads the libraries again, as after the user changes their order or which are pinned, then follows them.
    func reloadLibraries() {
        librariesTask?.cancel()
        librariesTask = Task { [weak self, session] in
            for await load in session.library.libraries() {
                guard let libraries = load.getOrNull() as? [Library] else { continue }
                self?.libraries = libraries
                self?.restoreLibrariesPage()
            }
        }
    }

    /// Plays a book in the book player, from where the user left it or from [positionMs], such as a chapter's start.
    func listen(_ workId: String, at positionMs: Int64? = nil) {
        book.play(workId, at: positionMs)
    }

    /// Opens the video player on [target], replacing anything playing; a
    /// book stops, keeping its place. Connected to a TV, it plays there,
    /// from the server even when it's downloaded, and the remote opens.
    func play(_ target: PlayTarget) {
        playback?.close()
        playback = nil
        if cast.connection == .connected {
            cast.cast(target)
            playerShown = true
            return
        }
        let model = PlayerModel(target: target, server: self)
        playback = model
        playerShown = true
    }

    /// A TV connected while the player plays: the TV carries on from here,
    /// and the player's screen becomes the remote.
    private func handOffToCast() {
        guard let playback, let handoff = playback.handoff else { return }
        playback.close()
        self.playback = nil
        cast.cast(handoff.target, positionMs: handoff.positionMs)
    }

    /// The Cast menu's Stop casting, which happens once the menu has closed,
    /// so the player takes the screen with nothing over it.
    func stopCastingAfterMenu() {
        stopAfterCastMenu = true
        cast.menuShown = false
    }

    func castMenuClosed() {
        guard stopAfterCastMenu else { return }
        stopAfterCastMenu = false
        stopCasting()
    }

    /// Stop casting: the TV stops, and the player carries on here from where the TV was.
    func stopCasting() {
        guard let stopped = cast.stopCasting() else { return }
        var target = stopped.target
        target.positionMs = stopped.positionMs
        play(target)
    }

    /// Nothing plays on the TV any more: the remote closes.
    private func castStopped() {
        if playback == nil, playerShown { playerShown = false }
    }

    /// The last episode of a series played to its end: the player closes on
    /// the series' page, or, for a download, on the page it opened from.
    func showSeries(afterPlaying seriesId: String, offline: Bool) {
        playerShown = false
        guard !offline else { return }
        if tab == .downloads { tab = .home }
        paths[tab, default: []].append(.work(WorkRoute(id: seriesId, kind: "series")))
    }

    /// The player's screen went away: the player stops unless it carries on in Picture in Picture.
    func playerDismissed() {
        guard let playback, !playback.pictureInPictureActive else { return }
        playback.close()
        self.playback = nil
    }

    /// Picture in Picture ended without returning to the app: the player stops.
    func pictureInPictureEnded() {
        guard !playerShown else { return }
        playback?.close()
        playback = nil
    }

    /// Signed out: everything stops.
    func close() {
        tasks.forEach { $0.cancel() }
        librariesTask?.cancel()
        playback?.close()
        playback = nil
        book.close()
        cast.close()
        session.close()
    }

    func signOut() async {
        try? await session.signOut()
    }
}

/// The app's tabs. Libraries is the tab bar's, in a narrow window; each
/// library, by its ID, is the sidebar's.
enum AppTab: Hashable {
    case home
    case libraries
    case lists
    case library(String)
    case downloads
    case search
}

/// What the video player opens on: a work from the server, or a download from its files here.
struct PlayTarget: Hashable {
    let workId: String
    var versionId: String?
    var audioTrackId: String?
    var subtitleTrackId: String?
    var fromStart = false
    /// A ready download, played without the server.
    var downloadId: String?
    /// Where to start, as where a cast stopped; nil for the resume position, or the start with [fromStart].
    var positionMs: Int64?
}
