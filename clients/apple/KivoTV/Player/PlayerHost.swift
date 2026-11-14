import KivoCore
import Observation

/// The video player's place in the signed-in app: the run of plays while
/// there is one, and whether the player shows over everything.
@MainActor
@Observable
final class PlayerHost {
    /// The plays the player shows, while it's up.
    private(set) var playback: VideoPlayback?
    /// Whether the player shows.
    var shown = false

    @ObservationIgnored private unowned let server: ServerModel

    init(server: ServerModel) {
        self.server = server
    }

    /// Opens the player on [choice], replacing anything playing, from its
    /// resume position, or from the start with [fromStart].
    func play(_ choice: PlayChoice, fromStart: Bool = false) {
        playback?.close()
        let app = server.app
        let playback = VideoPlayback(
            session: server.session,
            display: server.display,
            artwork: server.artwork,
            subtitleStyle: SubtitleStyle(size: app.subtitleSize, background: app.subtitleBackground),
            workId: choice.workId
        )
        playback.exited = { [weak self] exit in
            guard let self else { return }
            self.shown = false
            // The last episode of a series played to its end: its page.
            if case let .series(seriesId) = exit {
                self.server.navigation.push(.work(WorkRoute(id: seriesId, kind: .series)))
            }
        }
        playback.start(choice, at: fromStart ? 0 : nil, downloadId: nil)
        self.playback = playback
        shown = true
    }

    /// The player went away: the play stops, and Top Shelf reads Continue
    /// Watching again once the position has reached the server.
    func dismissed() {
        guard let playback else { return }
        playback.close()
        self.playback = nil
        server.refreshTopShelf(after: 5)
    }

    /// Signed out: the player stops.
    func close() {
        playback?.close()
        playback = nil
    }
}
