import KivoCore
import Observation

/// The video player's place in the signed-in app: the player while there
/// is one, which outlives its screen while it plays in Picture in Picture,
/// whether its screen shows, and the hand-off to and from a TV, which takes
/// over Play while one is connected and whose remote shows in the player's
/// place.
@MainActor
@Observable
final class PlayerHost {
    /// The player, while there is one.
    private(set) var model: PlayerModel?
    /// Whether the player's screen, or the Cast remote, shows; false while the player is in Picture in Picture.
    var shown = false

    @ObservationIgnored private unowned let server: ServerModel
    @ObservationIgnored private var stopAfterCastMenu = false

    init(server: ServerModel) {
        self.server = server
        server.cast.connected = { [weak self] in self?.handOffToCast() }
        server.cast.stopped = { [weak self] in self?.castStopped() }
    }

    /// Opens the video player on [choice], replacing anything playing; a
    /// book stops, keeping its place. It starts at [positionMs], as where
    /// a cast stopped, else at the start with [fromStart], else at the
    /// resume position, and plays the ready download [downloadId] without
    /// the server. Connected to a TV, it plays there, from the server even
    /// when it's downloaded, and the remote opens.
    func play(_ choice: PlayChoice, fromStart: Bool = false, at positionMs: Int64? = nil, downloadId: String? = nil) {
        model?.close()
        model = nil
        if server.cast.connection == .connected {
            server.cast.cast(choice, fromStart: fromStart)
            shown = true
            return
        }
        model = PlayerModel(choice, at: positionMs ?? (fromStart ? 0 : nil), downloadId: downloadId, server: server)
        shown = true
    }

    /// A TV connected while the player plays: the TV carries on from here,
    /// and the player's screen becomes the remote.
    private func handOffToCast() {
        guard let model, let handoff = model.handoff else { return }
        model.close()
        self.model = nil
        server.cast.cast(handoff.choice, at: handoff.positionMs)
    }

    /// The Cast menu's Stop casting, which happens once the menu has closed,
    /// so the player takes the screen with nothing over it.
    func stopCastingAfterMenu() {
        stopAfterCastMenu = true
        server.cast.menuShown = false
    }

    func castMenuClosed() {
        guard stopAfterCastMenu else { return }
        stopAfterCastMenu = false
        stopCasting()
    }

    /// Stop casting: the TV stops, and the player carries on here from where the TV was.
    func stopCasting() {
        guard let stopped = server.cast.stopCasting() else { return }
        play(stopped.choice, at: stopped.positionMs)
    }

    /// Nothing plays on the TV any more: the remote closes.
    private func castStopped() {
        if model == nil, shown { shown = false }
    }

    /// The last episode of a series played to its end: the player closes on
    /// the series' page.
    func showSeries(afterPlaying seriesId: String) {
        shown = false
        let navigation = server.navigation
        if navigation.tab == .downloads { navigation.tab = .home }
        navigation.push(.work(WorkRoute(id: seriesId, kind: .series)))
    }

    /// The player's screen went away: the player stops unless it carries on in Picture in Picture.
    func dismissed() {
        guard let model, !model.pictureInPictureActive else { return }
        model.close()
        self.model = nil
    }

    /// Picture in Picture ended without returning to the app: the player stops.
    func pictureInPictureEnded() {
        guard !shown else { return }
        model?.close()
        model = nil
    }

    /// Signed out: the player stops.
    func close() {
        model?.close()
        model = nil
    }
}
