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
        server.cast.connected = { [weak self] _ in self?.handOffToCast() }
        server.cast.stopped = { [weak self] in self?.castStopped() }
    }

    /// Opens the video player on [target], replacing anything playing; a
    /// book stops, keeping its place. Connected to a TV, it plays there,
    /// from the server even when it's downloaded, and the remote opens.
    func play(_ target: PlayTarget) {
        model?.close()
        model = nil
        if server.cast.connection == .connected {
            server.cast.cast(target)
            shown = true
            return
        }
        model = PlayerModel(target: target, server: server)
        shown = true
    }

    /// A TV connected while the player plays: the TV carries on from here,
    /// and the player's screen becomes the remote.
    private func handOffToCast() {
        guard let model, let handoff = model.handoff else { return }
        model.close()
        self.model = nil
        server.cast.cast(handoff.target, positionMs: handoff.positionMs)
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
        var target = stopped.target
        target.positionMs = stopped.positionMs
        play(target)
    }

    /// Nothing plays on the TV any more: the remote closes.
    private func castStopped() {
        if model == nil, shown { shown = false }
    }

    /// The last episode of a series played to its end: the player closes on
    /// the series' page, or, for a download, on the page it opened from.
    func showSeries(afterPlaying seriesId: String, offline: Bool) {
        shown = false
        guard !offline else { return }
        let navigation = server.navigation
        if navigation.tab == .downloads { navigation.tab = .home }
        navigation.push(.work(WorkRoute(id: seriesId, kind: "series")))
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
