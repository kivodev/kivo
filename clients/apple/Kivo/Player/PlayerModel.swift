import AVKit
import KivoCore
import KivoUI
import MediaPlayer
import Observation

/// The video player on the iPhone and iPad: the run of plays the screen
/// shows (``VideoPlayback``), with what only this app's screen has around
/// it. Its sheets, the scrub bar held, fill or fit, speed, Picture in
/// Picture, AirPlay, Now Playing and the lock screen's controls, and the
/// hand-off to a Cast device.
@MainActor
@Observable
final class PlayerModel: PlayerChoices, Scrubbable {
    let playback: VideoPlayback
    let surface: PlayerSurfaceView
    private(set) var pictureInPictureActive = false
    /// The sheet open over the player: Audio & Subtitles, or Playback settings.
    var sheet: PlayerSheet? {
        didSet { playback.menuOpen = sheet != nil }
    }
    /// A finger is on the scrub bar.
    var scrubbing: Bool {
        get { playback.scrubbing }
        set { playback.scrubbing = newValue }
    }
    /// The video plays on an AirPlay device.
    private(set) var airPlaying = false
    /// The video fills the screen, cropping its edges, rather than fitting inside it.
    var fill = false {
        didSet { surface.playerLayer.videoGravity = fill ? .resizeAspectFill : .resizeAspect }
    }

    @ObservationIgnored private let server: ServerModel
    @ObservationIgnored private var pictureInPicture: AVPictureInPictureController?
    @ObservationIgnored private let pictureInPictureEvents = PictureInPictureEvents()
    /// Picture in Picture's handler, told once the player's screen is back.
    @ObservationIgnored private var restoring: ((Bool) -> Void)?
    @ObservationIgnored private var nowPlaying: MPNowPlayingSession?
    @ObservationIgnored private var externalObservation: NSKeyValueObservation?

    init(_ choice: PlayChoice, at positionMs: Int64?, downloadId: String?, server: ServerModel) {
        self.server = server
        let app = server.app
        playback = VideoPlayback(
            session: server.session,
            display: server.display,
            artwork: server.artwork,
            subtitleStyle: SubtitleStyle(size: app.subtitleSize, background: app.subtitleBackground),
            workId: choice.workId
        )
        surface = PlayerSurfaceView(player: playback.engine.player)
        playback.exited = { [weak server] exit in
            switch exit {
            case let .series(seriesId): server?.player.showSeries(afterPlaying: seriesId)
            case .back: server?.player.shown = false
            }
        }
        setUpPictureInPicture()
        setUpNowPlaying()
        externalObservation = engine.player.observe(\.isExternalPlaybackActive, options: .initial) { [weak self] player, _ in
            let active = player.isExternalPlaybackActive
            DispatchQueue.main.async {
                self?.airPlaying = active
                self?.tellOverlayShows()
            }
        }
        playback.start(choice, at: positionMs, downloadId: downloadId)
    }

    // MARK: The play

    var engine: AVPlayerEngine { playback.engine }
    var display: Display { playback.display }
    var workId: String { playback.workId }
    var state: PlayerState? { playback.state }
    var offline: Bool { playback.offline }
    var work: WorkDetail? { playback.work }
    var next: WorkDetail? { playback.next }
    var qualities: [QualityOption] { playback.qualities }
    var previews: ScrubPreviews? { playback.previews }
    var marker: SkipMarker? { playback.marker }
    var upNext: UpNext { playback.upNext }
    var controlsVisible: Bool { playback.controlsVisible }
    var speed: Float { playback.speed }
    var controller: PlayController? { playback.controller }
    var play: PlayStarted? { playback.play }
    var version: Version? { playback.version }
    var quality: Quality? { playback.quality }
    var subtitle: String { playback.subtitle }
    var externalSubtitle: TrackShowExternal? { playback.externalSubtitle }
    var audioTrackId: String? { playback.audioTrackId }
    var scrubDurationMs: Int64 { playback.durationMs }
    var scrubPositionMs: Int64 { playback.positionMs }
    var chapters: [Chapter] { playback.chapters }
    var dialogueBoost: DialogueBoost? { playback.dialogueBoost }
    var streamInfo: StreamInfo? { playback.streamInfo }
    var handoff: (choice: PlayChoice, positionMs: Int64)? { playback.handoff }
    var title: String { playback.title }
    var subtitleLine: String? { playback.subtitleLine }
    var searchesSubtitles: Bool { playback.searchesSubtitles }

    /// Casting, which the top bar's Cast button connects.
    var cast: CastModel { server.cast }

    func offers(_ track: Track) -> Bool { playback.offers(track) }
    func read(_ url: String) async throws -> Data { try await playback.read(url) }
    func endsAt(from now: Date) -> String? { playback.endsAt(from: now) }

    // MARK: Commands

    func input() { playback.input() }
    func toggleControls() { playback.toggleControls() }
    func setSpeed(_ speed: Float) { playback.setSpeed(speed) }
    func togglePlay() { playback.togglePlay() }
    func skip(by deltaMs: Int64) { playback.skip(by: deltaMs) }
    /// A double tap's skip: it counts as someone watching, and leaves the controls as they are.
    func nudge(by deltaMs: Int64) { playback.nudge(by: deltaMs) }
    func seek(to positionMs: Int64) { playback.seek(to: positionMs) }
    func skipPastMarker() { playback.skipPastMarker() }
    func chooseAudio(_ trackId: String) { playback.chooseAudio(trackId) }
    func chooseSubtitle(_ trackId: String) { playback.chooseSubtitle(trackId) }
    func searchSubtitles() async throws -> [SubtitleResult] { try await playback.searchSubtitles() }
    func downloadSubtitle(_ result: SubtitleResult) async throws { try await playback.downloadSubtitle(result) }
    func boostDialogue(_ on: Bool) { playback.boostDialogue(on) }
    func chooseQuality(_ option: QualityOption) { playback.chooseQuality(option) }
    func chooseVersion(_ versionId: String) { playback.chooseVersion(versionId) }
    func retry() { playback.retry() }
    func playNextNow() { playback.playNextNow() }
    func hideUpNext() { playback.hideUpNext() }

    func startPictureInPicture() {
        pictureInPicture?.startPictureInPicture()
    }

    /// The app went to the background with the player up: the play stops, unless the video carries on in
    /// Picture in Picture or on an AirPlay device; coming back starts it again where it was.
    func background() {
        guard !pictureInPictureActive, !engine.player.isExternalPlaybackActive else { return }
        playback.stopOnExit()
    }

    func foreground() {
        playback.restore()
    }

    /// The player's screen appeared: Picture in Picture, if it's returning
    /// to it, hands the video back.
    func screenShown() {
        restoring?(true)
        restoring = nil
    }

    /// The player is done: the play stops and everything is released.
    func close() {
        pictureInPicture?.stopPictureInPicture()
        restoring?(false)
        restoring = nil
        playback.close()
        nowPlaying = nil
    }

    // MARK: Picture in Picture and Now Playing

    /// The core's player learns whether the subtitle overlay can show: not
    /// in Picture in Picture or on an AirPlay device, where only the video goes.
    private func tellOverlayShows() {
        playback.overlayVisible = !pictureInPictureActive && !engine.player.isExternalPlaybackActive
    }

    private func setUpPictureInPicture() {
        guard AVPictureInPictureController.isPictureInPictureSupported(),
              let controller = AVPictureInPictureController(playerLayer: surface.playerLayer)
        else { return }
        controller.canStartPictureInPictureAutomaticallyFromInline = true
        controller.delegate = pictureInPictureEvents
        pictureInPictureEvents.willStart = { [weak self] in
            guard let self else { return }
            self.pictureInPictureActive = true
            self.tellOverlayShows()
            self.server.player.shown = false
        }
        pictureInPictureEvents.failedToStart = { [weak self] error in
            NSLog("Kivo: Picture in Picture didn't start: \(error)")
            guard let self, self.pictureInPictureActive else { return }
            self.pictureInPictureActive = false
            self.tellOverlayShows()
            self.server.player.shown = true
        }
        pictureInPictureEvents.didStop = { [weak self] in
            guard let self else { return }
            self.pictureInPictureActive = false
            self.tellOverlayShows()
            self.server.player.pictureInPictureEnded()
        }
        pictureInPictureEvents.restore = { [weak self] completion in
            guard let self else { return completion(false) }
            self.restoring = completion
            self.server.player.shown = true
        }
        pictureInPicture = controller
    }

    /// The lock screen and Control Center (MPNowPlayingSession publishes
    /// the player's rate and position, and the title and poster from the
    /// player's items), as RemoteCommands has them for a video; their
    /// commands go through the controller, so the server hears of them.
    private func setUpNowPlaying() {
        let session = MPNowPlayingSession(players: [engine.player])
        session.automaticallyPublishesNowPlayingInfo = true
        _ = RemoteCommands.setUp(session.remoteCommandCenter, for: .video) { [weak self] action, event in
            guard let controller = self?.controller else { return .success }
            switch action {
            case .play: controller.resume()
            case .pause: controller.pause()
            case .togglePlay: controller.togglePlay()
            case .skipBack: controller.skip(deltaMs: -SKIP_BACK_MS)
            case .skipForward: controller.skip(deltaMs: SKIP_FORWARD_MS)
            case .seek:
                guard let event = event as? MPChangePlaybackPositionCommandEvent else { return .commandFailed }
                controller.seek(positionMs: Int64(event.positionTime * 1000))
            }
            return .success
        }
        session.becomeActiveIfPossible { _ in }
        nowPlaying = session
    }
}

/// The sheets the player's bottom buttons open.
enum PlayerSheet: String, Identifiable {
    /// Audio & Subtitles.
    case tracks
    /// Playback settings: Audio & Subtitles, quality, version, speed and stream info.
    case settings

    var id: String { rawValue }
}

/// Picture in Picture's delegate, forwarding to the player model.
private final class PictureInPictureEvents: NSObject, AVPictureInPictureControllerDelegate {
    var willStart: () -> Void = {}
    var failedToStart: (Error) -> Void = { _ in }
    var didStop: () -> Void = {}
    var restore: (@escaping (Bool) -> Void) -> Void = { $0(false) }

    func pictureInPictureControllerWillStartPictureInPicture(_ controller: AVPictureInPictureController) {
        willStart()
    }

    func pictureInPictureController(_ controller: AVPictureInPictureController, failedToStartPictureInPictureWithError error: Error) {
        failedToStart(error)
    }

    func pictureInPictureControllerDidStopPictureInPicture(_ controller: AVPictureInPictureController) {
        didStop()
    }

    func pictureInPictureController(
        _ controller: AVPictureInPictureController,
        restoreUserInterfaceForPictureInPictureStopWithCompletionHandler completion: @escaping (Bool) -> Void
    ) {
        restore(completion)
    }
}
