import AVKit
import KivoCore
import KivoUI
import MediaPlayer
import Observation

/// The video player: the core's play controller over the
/// AVPlayer engine, and what the screen shows around it. The tracks and
/// quality menus, the subtitle the player shows, Skip Intro and Credits,
/// up next with autoplay, scrub previews, Picture in Picture, Now Playing
/// and the lock screen's controls.
@MainActor
@Observable
final class PlayerModel: PlayerChoices, Scrubbable {
    let engine: AVPlayerEngine
    let surface: PlayerSurfaceView
    let display: Display
    /// The work playing.
    private(set) var workId: String
    /// The core's player's play.
    private var playing: Observed<PlayerState>?
    /// What the core's player shows around the video.
    private var shown: Observed<PlayerScreenState>?
    var state: PlayerState? { playing?.value }
    /// Playing a download from its files: no quality, version or other track to choose.
    var offline: Bool { shown?.value.offline ?? false }
    /// Where a download playing came from, for stream info's Source line.
    var downloadSource: DownloadSource? { shown?.value.downloadSource }
    var work: WorkDetail? { shown?.value.work }
    /// The next episode, for up next: none without the server unless it's downloaded.
    var next: WorkDetail? { shown?.value.next }
    var qualities: [QualityOption] { shown?.value.qualities ?? [] }
    var previews: ScrubPreviews? { shown?.value.previews }
    /// What shows over the video by the core's timed rules: the controls, Skip and up next.
    var ui: PlayerUi? { shown?.value.ui }
    /// The intro, recap or credits marker whose Skip button shows.
    var marker: SkipMarker? { ui?.skipMarker }
    /// Up next's card, for an episode with a next one; it counts down while playing with no sheet open.
    var upNext: UpNext { ui?.upNext ?? UpNext() }
    private(set) var pictureInPictureActive = false
    /// Whether the controls show. They hide 3 seconds after the last input
    /// while playing, but never while paused, scrubbing or with a sheet open.
    var controlsVisible: Bool { ui?.controlsShown ?? true }
    /// The sheet open over the player: Audio & Subtitles, or Playback settings.
    var sheet: PlayerSheet? {
        didSet { player?.setMenuOpen(open: sheet != nil) }
    }
    /// A finger is on the scrub bar.
    var scrubbing = false {
        didSet { player?.setScrubbing(held: scrubbing) }
    }
    /// The playback speed, 1 for normal.
    private(set) var speed: Float = 1
    /// The video plays on an AirPlay device.
    private(set) var airPlaying = false
    /// The video fills the screen, cropping its edges, rather than fitting inside it.
    var fill = false {
        didSet { surface.playerLayer.videoGravity = fill ? .resizeAspectFill : .resizeAspect }
    }

    @ObservationIgnored private let server: ServerModel
    @ObservationIgnored private var player: KivoCore.VideoPlayer?
    /// The work Now Playing shows.
    @ObservationIgnored private var nowPlayingWork: WorkDetail?
    /// The player went where its end took it.
    @ObservationIgnored private var exited = false
    @ObservationIgnored private var tasks: [Task<Void, Never>] = []
    @ObservationIgnored private var pictureInPicture: AVPictureInPictureController?
    @ObservationIgnored private let pictureInPictureEvents = PictureInPictureEvents()
    /// Picture in Picture's handler, told once the player's screen is back.
    @ObservationIgnored private var restoring: ((Bool) -> Void)?
    @ObservationIgnored private var nowPlaying: MPNowPlayingSession?
    /// Now Playing's artwork, and the image it is.
    @ObservationIgnored private var artwork: (id: String, data: Data?)?
    @ObservationIgnored private var externalObservation: NSKeyValueObservation?

    init(_ choice: PlayChoice, at positionMs: Int64?, downloadId: String?, server: ServerModel) {
        self.server = server
        workId = choice.workId
        display = server.display
        let app = server.app
        let session = server.session
        engine = AVPlayerEngine(
            baseUrl: { session.baseUrl },
            subtitleStyle: SubtitleStyle(size: app.subtitleSize, background: app.subtitleBackground)
        )
        surface = PlayerSurfaceView(player: engine.player)
        setUpPictureInPicture()
        setUpNowPlaying()
        externalObservation = engine.player.observe(\.isExternalPlaybackActive, options: .initial) { [weak self] player, _ in
            let active = player.isExternalPlaybackActive
            DispatchQueue.main.async {
                self?.airPlaying = active
                self?.tellOverlayShows()
            }
        }
        start(choice, at: positionMs, downloadId: downloadId)
    }

    /// Plays [choice] from [positionMs], or its resume position when nil.
    /// Everything but where the play comes from is the same for the
    /// download [downloadId] as for the server: a download plays from its
    /// files, without the server. The engine, its surface and Picture in
    /// Picture stay the same across the plays: the last play's picture shows
    /// until the next one's replaces it.
    private func start(_ choice: PlayChoice, at positionMs: Int64? = nil, downloadId: String?) {
        player?.handOver()
        workId = choice.workId
        // Playback audio, so the video plays with the silent switch on and
        // Picture in Picture and AirPlay are allowed. Set on every play, as
        // the system can reset it.
        try? AVAudioSession.sharedInstance().setCategory(.playback, mode: .moviePlayback)
        let player = server.session.playback.videoPlayer(
            engine: engine,
            choice: choice,
            hideControlsAfterMs: 3_000,
            downloadId: downloadId,
            positionMs: positionMs.map { KotlinLong(value: $0) }
        )
        self.player = player
        exited = false
        player.setMenuOpen(open: sheet != nil)
        player.setScrubbing(held: scrubbing)
        tellOverlayShows()
        playing = Observed(player.controller.state)
        shown = Observed(player.screen) { [weak self] screen in self?.screenChanged(screen, of: player) }
    }

    var controller: PlayController? { player?.controller }

    /// Casting, which the top bar's Cast button connects.
    var cast: CastModel { server.cast }

    var play: PlayStarted? { state?.play }

    /// The version the play plays.
    var version: Version? { shown?.value.version }

    /// The quality the play applies, as the server says.
    var quality: Quality? {
        play.map { Quality.companion.of(request: $0.quality) }
    }

    /// The subtitle track showing, or off, as the core's player says.
    var subtitle: String {
        state?.subtitleTrackId ?? SUBTITLES_OFF
    }

    /// The subtitle file the overlay draws, when the one showing is a file.
    var externalSubtitle: TrackShowExternal? {
        state.flatMap { $0.subtitle as? TrackShowExternal }
    }

    var audioTrackId: String? { state?.audioTrackId }

    var scrubDurationMs: Int64 { Int64(play?.durationMs ?? 0) }

    var scrubPositionMs: Int64 { state?.positionMs ?? 0 }

    var chapters: [Chapter] { play?.chapters ?? [] }

    var dialogueBoost: DialogueBoost? { play?.dialogueBoost }

    var streamInfo: StreamInfo? {
        play.map {
            StreamInfo(
                tracks: $0.tracks,
                delivery: $0.delivery,
                local: $0.local,
                bitrateBps: $0.bitrateBps?.int64Value,
                downloadSource: downloadSource,
                client: nil
            )
        }
    }

    /// A download has the tracks it kept, which play from its files; the server's play, every track.
    func offers(_ track: Track) -> Bool {
        guard offline, let play else { return true }
        return play.tracks.contains { $0.trackId == track.id && ($0.selected?.boolValue == true || $0.delivery != nil) }
    }

    /// Where the play is, to carry on from on a Cast device: the work, its version and tracks, and the position.
    var handoff: (choice: PlayChoice, positionMs: Int64)? {
        guard let play else { return nil }
        let choice = PlayChoice(workId: workId, versionId: play.versionId, audioTrackId: audioTrackId, subtitleTrackId: subtitle)
        return (choice, controller?.positionMs() ?? Int64(state?.positionMs ?? 0))
    }

    /// Reads a subtitle file or font the play names: a download's from its
    /// file here, else from the server with the session's authorization,
    /// where the session's token may go now.
    func read(_ url: String) async throws -> Data {
        var request = URLRequest(url: engine.mediaURL(url))
        if let target = request.url, !target.isFileURL, let authorization = server.session.authorization(url: target.absoluteString) {
            request.setValue(authorization, forHTTPHeaderField: "Authorization")
        }
        let (data, response) = try await URLSession.shared.data(for: request)
        if let http = response as? HTTPURLResponse, http.statusCode != 200 { throw URLError(.badServerResponse) }
        return data
    }

    /// The top bar's title: the movie with its edition, or the episode's series.
    var title: String {
        guard let work else { return "" }
        return work.episode?.seriesTitle ?? work.labelledTitle
    }

    /// An episode's second line: "S1 · E1 · The Goddamn Brownies".
    var subtitleLine: String? {
        guard let work, let episode = work.episode else { return nil }
        return "\(Display.app.episodeCode(episode: episode)) · \(work.title)"
    }

    /// "Ends at 9:41 PM" from [now], at the speed it plays, for a movie or an episode.
    func endsAt(from now: Date) -> String? {
        guard let work, scrubDurationMs > 0 else { return nil }
        let remaining = Double(scrubDurationMs - scrubPositionMs) / Double(speed)
        return Words.endsAt(kind: work.kind, remainingMs: Int64(remaining), from: now)
    }

    // MARK: Commands

    /// Any input: the controls show, their wait to hide starts again, and
    /// someone is watching, which resets up next's run.
    func input() {
        player?.showControls()
    }

    /// A tap on the video: the controls show, or hide if they're showing.
    func toggleControls() {
        player?.toggleControls()
    }

    /// Sets the playback speed: AVPlayer plays at its default rate from then on.
    func setSpeed(_ speed: Float) {
        input()
        self.speed = speed
        engine.player.defaultRate = speed
        if engine.player.rate > 0 { engine.player.rate = speed }
    }

    func togglePlay() {
        input()
        controller?.togglePlay()
    }

    func skip(by deltaMs: Int64) {
        input()
        controller?.skip(deltaMs: deltaMs)
    }

    /// A double tap's skip: it counts as someone watching, and leaves the controls as they are.
    func nudge(by deltaMs: Int64) {
        player?.userActivity()
        controller?.skip(deltaMs: deltaMs)
    }

    func seek(to positionMs: Int64) {
        input()
        controller?.seek(positionMs: positionMs)
    }

    /// Skip Intro, Recap or Credits: to the marker's end, with the controls shown as for any input.
    func skipPastMarker() {
        input()
        player?.skipMarker()
    }

    func chooseAudio(_ trackId: String) {
        input()
        player?.chooseAudio(trackId: trackId)
    }

    /// Shows a subtitle as the play's tracks say: at once, or by a new play at the position.
    func chooseSubtitle(_ trackId: String) {
        input()
        player?.chooseSubtitle(trackId: trackId)
    }

    var searchesSubtitles: Bool {
        SubtitleSearch.shared.offered(version: version, offline: offline)
    }

    func searchSubtitles() async throws -> [SubtitleResult] {
        guard let version else { return [] }
        return try await server.session.playback.subtitleResults(version: version)
    }

    /// The version's tracks gain the subtitle at once, before the work is read again with it, and a new play shows it.
    func downloadSubtitle(_ result: SubtitleResult) async throws {
        input()
        try await player?.addSubtitle(resultId: result.id)
    }

    /// Switches the dialogue boost, kept on this device for later plays: a new play at the position applies it.
    func boostDialogue(_ on: Bool) {
        input()
        player?.boostDialogue(on: on)
    }

    func chooseQuality(_ option: QualityOption) {
        input()
        player?.chooseQuality(quality: option.quality)
    }

    func chooseVersion(_ versionId: String) {
        input()
        player?.chooseVersion(versionId: versionId)
    }

    func retry() {
        controller?.retry()
    }

    /// Up next's Play now: the core's player says to play the next episode (screenChanged).
    func playNextNow() {
        player?.playNext()
    }

    /// "Hide" or "Watch credits": no card for the rest of the episode, which still plays on at its end.
    func hideUpNext() {
        input()
        player?.hideUpNext()
    }

    func startPictureInPicture() {
        pictureInPicture?.startPictureInPicture()
    }

    /// The app went to the background with the player up: the play stops, unless the video carries on in
    /// Picture in Picture or on an AirPlay device; coming back starts it again where it was.
    func background() {
        guard !pictureInPictureActive, !engine.player.isExternalPlaybackActive else { return }
        controller?.stopOnExit()
    }

    func foreground() {
        controller?.restore()
    }

    /// The player's screen appeared: Picture in Picture, if it's returning
    /// to it, hands the video back.
    func screenShown() {
        restoring?(true)
        restoring = nil
    }

    /// The player is done: the play stops and everything is released.
    func close() {
        tasks.forEach { $0.cancel() }
        pictureInPicture?.stopPictureInPicture()
        restoring?(false)
        restoring = nil
        player?.dispose()
        player = nil
        nowPlaying = nil
    }

    // MARK: State

    /// The core's player learns whether the subtitle overlay can show: not
    /// in Picture in Picture or on an AirPlay device, where only the video
    /// goes, so plays there ask for no subtitle files.
    private func tellOverlayShows() {
        player?.setOverlayVisible(visible: !pictureInPictureActive && !engine.player.isExternalPlaybackActive)
    }

    /// What [player] shows around the video changed: Now Playing follows
    /// the work, and once the player is done, the next episode plays in its
    /// place, or the player gives way to the series' page or closes, once.
    private func screenChanged(_ screen: PlayerScreenState, of player: KivoCore.VideoPlayer) {
        guard player === self.player else { return }
        if let work = screen.work, work !== nowPlayingWork {
            nowPlayingWork = work
            updateNowPlaying()
        }
        guard let exit = screen.exit, !exited else { return }
        exited = true
        switch onEnum(of: exit) {
        case let .next(next):
            start(next.choice, downloadId: next.downloadId)
        case let .series(series):
            server.player.showSeries(afterPlaying: series.seriesId, offline: offline)
        case .back:
            server.player.shown = false
        }
    }

    // MARK: Picture in Picture and Now Playing

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
    /// the player's rate and position), as RemoteCommands has them for a
    /// video; their commands go through the controller, so the server hears
    /// of them.
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

    /// What Now Playing shows, the title and the poster: the session
    /// publishes it from the player's items, since it publishes
    /// automatically and can't be given it directly. The poster comes
    /// through the artwork caches, so one seen before shows without the server.
    private func updateNowPlaying() {
        guard let work else { return }
        let image = work.images.poster ?? work.images.still
        if let image, artwork?.id != image.id {
            artwork = (image.id, nil)
            tasks.append(Task { [weak self, source = server.artwork] in
                let loaded = await source.image(id: image.id, pixelWidth: 600)
                guard let self, self.artwork?.id == image.id else { return }
                self.artwork = (image.id, loaded?.jpegData(compressionQuality: 0.9))
                self.updateNowPlaying()
            })
        }
        engine.metadata = [
            Self.metadataItem(.commonIdentifierTitle, work.title as NSString),
            work.episode.map { Self.metadataItem(.iTunesMetadataTrackSubTitle, $0.seriesTitle as NSString) },
            artwork?.data.map { Self.metadataItem(.commonIdentifierArtwork, $0 as NSData, dataType: kCMMetadataBaseDataType_JPEG as String) },
        ].compactMap { $0 }
    }

    private static func metadataItem(_ identifier: AVMetadataIdentifier, _ value: NSCopying & NSObjectProtocol, dataType: String? = nil) -> AVMetadataItem {
        let item = AVMutableMetadataItem()
        item.identifier = identifier
        item.value = value
        if let dataType { item.dataType = dataType }
        item.extendedLanguageTag = "und"
        return item
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
