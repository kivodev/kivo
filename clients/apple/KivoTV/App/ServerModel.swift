import KivoCore
import KivoUI
import Observation
import SwiftUI

/// The signed-in Apple TV app's shared state: the server, the user, the
/// libraries the TV shows in the user's order, and whether the server
/// answers; and the models that share it: where the app is
/// (``Navigation``) and the video player (``PlayerHost``). The screens the
/// TV shares with the iPhone app read from it as from the iPhone's.
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
    /// Every library the user sees, in their order, audiobooks too.
    private(set) var libraries: [Library] = []
    /// The server isn't answering reads, which retry until it does.
    var unreachable: Bool { state.value.unreachable }
    /// Why the app and the server don't work together, while they don't: its screen shows in place of the server's.
    var incompatibility: Incompatibility? { Incompatibility(state.value.compatibility, server: serverName) }
    /// The base URL the server is reached at now, which artwork follows.
    var baseUrl: String { state.value.address }
    /// The tab showing and each tab's pages.
    let navigation = Navigation()
    /// The video player and its screen, made once the rest is.
    @ObservationIgnored private(set) lazy var player = PlayerHost(server: self)
    /// Why a mark on a work failed, until it's said.
    var problem: String?

    @ObservationIgnored private var librariesTask: Task<Void, Never>?
    @ObservationIgnored private var marksTask: Task<Void, Never>?

    init(session: KivoSession, app: AppModel) {
        self.session = session
        self.app = app
        state = Observed(session.state)
        reloadLibraries()
        marksTask = Task { [weak self, session] in
            for await failed in session.library.marks.problems {
                self?.problem = failed.sentence
            }
        }
        refreshTopShelf()
    }

    var display: Display { app.display }

    var artwork: ArtworkSource {
        ArtworkSource(baseURL: baseUrl, authorization: { [session] in session.authorization(url: $0) }, serverId: session.savedServer.value.id)
    }

    /// The libraries the TV shows, in the user's order: every kind but audiobooks.
    var tvLibraries: [Library] { Self.tvLibraries(libraries) }

    var pinnedLibraries: [Library] { tvLibraries.filter(\.pinned) }

    var otherLibraries: [Library] { tvLibraries.filter { !$0.pinned } }

    /// [libraries] as the TV shows them: books aren't on the TV.
    static func tvLibraries(_ libraries: [Library]) -> [Library] {
        libraries.filter { $0.kind != .audiobooks }
    }

    /// The library a work is in, by the [libraryId] its card or page names, if the TV shows it.
    func library(of libraryId: String?) -> Library? {
        guard let libraryId else { return nil }
        return tvLibraries.first { $0.id == libraryId }
    }

    /// Reads the libraries again, as after the user changes their order or which are pinned, then follows them.
    func reloadLibraries() {
        librariesTask?.cancel()
        librariesTask = Task { [weak self, session] in
            for await load in session.library.libraries() {
                guard let libraries = load.getOrNull() as? [Library] else { continue }
                self?.libraries = libraries
            }
        }
    }

    /// Plays [workId]'s video from where the user left it, or from the start.
    func play(_ workId: String, fromStart: Bool = false) {
        player.play(PlayChoice(workId: workId), fromStart: fromStart)
    }

    /// Reads Continue Watching again for Top Shelf, after [delay] seconds.
    func refreshTopShelf(after delay: Double = 0) {
        Task { [weak self] in
            if delay > 0 { try? await Task.sleep(for: .seconds(delay)) }
            guard let self else { return }
            await TopShelf.publish(self.session, artwork: self.artwork)
        }
    }

    /// Asks the server what it is again, as Try again does on a server the app doesn't work with.
    func checkCompatibility() async {
        _ = try? await session.serverInfo()
    }

    /// Signed out: everything stops.
    func close() {
        librariesTask?.cancel()
        marksTask?.cancel()
        player.close()
        session.close()
    }

    func signOut() async {
        try? await session.signOut()
    }
}
