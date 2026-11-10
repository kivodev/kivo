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
    /// Whether the app and the server work together, as the server last said what it is.
    private(set) var compatibility: Compatibility
    /// Why they don't, while they don't: its screen shows in place of the server's.
    var incompatibility: Incompatibility? { Incompatibility(compatibility, server: serverName) }
    /// The base URL the server is reached at now, which artwork follows.
    private(set) var baseUrl: String
    /// The tab showing, its pages, and the sheets over the tabs.
    let navigation: Navigation
    /// The video player and its screen, made once the rest is.
    @ObservationIgnored private(set) lazy var player = PlayerHost(server: self)
    /// The book player, with its mini player while a book is loaded.
    let book: BookPlayer
    /// Casting to a TV, with its mini controller and remote while something plays there.
    let cast: CastModel
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

    init(session: KivoSession, app: AppModel) {
        self.session = session
        self.app = app
        baseUrl = session.baseUrl
        serverName = session.savedServer.value.name
        compatibility = session.compatibility.value
        book = BookPlayer(session: session, artwork: { Self.artwork(session, at: session.baseUrl) }, display: app.display)
        cast = CastModel(session: session, core: app.core, display: app.display)
        user = session.user.value
        navigation = Navigation(serverId: session.savedServer.value.id)
        // Casting hands over to and from the player from the start.
        _ = player
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
            for await compatibility in session.compatibility {
                self?.compatibility = compatibility
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
                if error.requestProblem.isUnreachable { self?.navigation.tab = .downloads }
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
        tasks.forEach { $0.cancel() }
        librariesTask?.cancel()
        player.close()
        book.close()
        cast.close()
        session.close()
    }

    func signOut() async {
        try? await session.signOut()
    }
}
