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
    /// Playing a download from its files: no quality, version or other track to choose.
    private(set) var offline = false
    /// Where a download playing came from, for stream info's Source line.
    private(set) var downloadSource: DownloadSource?
    /// The work playing.
    private(set) var workId: String
    private(set) var state: PlayerState?
    private(set) var work: WorkDetail?
    /// The next episode, for up next: none without the server unless it's downloaded.
    private(set) var next: WorkDetail?
    private(set) var qualities: [QualityOption] = []
    private(set) var previews: ScrubPreviews?
    /// What shows over the video by the core's timed rules: the controls, Skip and up next.
    private(set) var ui: PlayerUi?
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
        didSet {
            player?.setMenuOpen(open: sheet != nil)
            waitToHide(again: false)
        }
    }
    /// A finger is on the scrub bar.
    var scrubbing = false {
        didSet {
            player?.setScrubbing(held: scrubbing)
            waitToHide(again: false)
        }
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
    @ObservationIgnored private var playId: String?
    @ObservationIgnored private var tasks: [Task<Void, Never>] = []
    /// The controls' wait to hide, while it runs.
    @ObservationIgnored private var hiding: Task<Void, Never>?
    @ObservationIgnored private var following: [Task<Void, Never>] = []
    @ObservationIgnored private var pictureInPicture: AVPictureInPictureController?
    @ObservationIgnored private let pictureInPictureEvents = PictureInPictureEvents()
    /// Picture in Picture's handler, told once the player's screen is back.
    @ObservationIgnored private var restoring: ((Bool) -> Void)?
    @ObservationIgnored private var nowPlaying: MPNowPlayingSession?
    /// Now Playing's artwork, and the image it is.
    @ObservationIgnored private var artwork: (id: String, data: Data?)?
    @ObservationIgnored private var externalObservation: NSKeyValueObservation?

    init(target: PlayTarget, server: ServerModel) {
        self.server = server
        workId = target.workId
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
        tasks.append(Task { [weak self] in
            guard let self else { return }
            // Without the menu, Playback Settings offers no quality.
            qualities = (try? await self.server.session.playback.qualityMenu()) ?? []
        })
        start(target)
    }

    /// Plays [target]. Everything but where the play comes from is the
    /// same for a download as for the server: a download plays from its
    /// files, without the server. The engine, its surface and Picture in
    /// Picture stay the same across the plays: the last play's picture shows
    /// until the next one's replaces it.
    private func start(_ target: PlayTarget) {
        following.forEach { $0.cancel() }
        following = []
        player?.handOver()
        player = nil
        ui = nil
        workId = target.workId
        offline = target.downloadId != nil
        downloadSource = nil
        load(workId: target.workId)
        tasks.append(Task { [weak self, engine, server, display] in
            do {
                let player: KivoCore.VideoPlayer
                var source: DownloadSource?
                if let downloadId = target.downloadId {
                    player = try await server.session.playback.offlinePlayer(engine: engine, downloadId: downloadId)
                    source = player.downloadSource(display: display)
                } else {
                    player = server.session.playback.videoPlayer(engine: engine)
                }
                guard let self, self.workId == target.workId else {
                    player.dispose()
                    return
                }
                downloadSource = source
                attach(player)
                // The controller reports a play that fails to start in its state.
                try await player.play(
                    workId: target.workId,
                    versionId: target.versionId,
                    audioTrackId: target.audioTrackId,
                    subtitleTrackId: target.subtitleTrackId,
                    positionMs: (target.positionMs ?? (target.fromStart ? Int64(0) : nil)).map { KotlinLong(value: $0) }
                )
            } catch {
                // A download that can't play now, such as one that needs the server to check it first.
                if target.downloadId != nil { server.playerShown = false }
            }
        })
    }

    /// Plays through [player] from now on, following its state and what it shows over the video.
    private func attach(_ player: KivoCore.VideoPlayer) {
        self.player = player
        player.setMenuOpen(open: sheet != nil)
        player.setScrubbing(held: scrubbing)
        tellEpisode()
        tellOverlayShows()
        following = [
            Task { [weak self] in
                for await state in player.controller.state {
                    self?.stateChanged(state)
                }
            },
            Task { [weak self] in
                for await ui in player.ui {
                    self?.uiChanged(ui)
                }
            },
        ]
    }

    var controller: PlayController? { player?.controller }

    /// Casting, which the top bar's Cast button connects.
    var cast: CastModel { server.cast }

    var play: PlayStarted? { state?.play }

    /// The version the play plays.
    var version: Version? {
        guard let play else { return nil }
        return work?.versions.first { $0.id == play.versionId }
    }

    /// The quality the play applies, as the server says.
    var quality: Quality? {
        play.map { Quality.companion.of(request: $0.quality) }
    }

    /// The subtitle track showing, or off, as the core's player says.
    var subtitle: String {
        state?.subtitleTrackId ?? Subtitles.shared.OFF
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
    var handoff: (target: PlayTarget, positionMs: Int64)? {
        guard let play else { return nil }
        let target = PlayTarget(workId: workId, versionId: play.versionId, audioTrackId: audioTrackId, subtitleTrackId: subtitle)
        return (target, controller?.positionMs() ?? Int64(state?.positionMs ?? 0))
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
        waitToHide(again: true)
    }

    /// A tap on the video: the controls show, or hide if they're showing.
    func toggleControls() {
        player?.toggleControls()
        waitToHide(again: true)
    }

    /// The controls hide 3 seconds after the last input while playing, with
    /// no sheet open or scrub held; [again] starts a running wait over, as an input does.
    private func waitToHide(again: Bool) {
        let waits = controlsVisible && state?.live == true && state?.paused == false && sheet == nil && !scrubbing
        guard waits else {
            hiding?.cancel()
            hiding = nil
            return
        }
        guard again || hiding == nil else { return }
        hiding?.cancel()
        hiding = Task { [weak self] in
            try? await Task.sleep(for: .seconds(3))
            guard let self, !Task.isCancelled else { return }
            hiding = nil
            player?.hideControls()
        }
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
        waitToHide(again: true)
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
        guard let version else { return }
        let track = try await server.session.playback.addSubtitle(version: version, resultId: result.id)
        work = work?.withTrack(versionId: version.id, track: track)
        chooseSubtitle(track.id)
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

    /// Up next's Play now: the core's player says to play the next episode (uiChanged).
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
        hiding?.cancel()
        tasks.forEach { $0.cancel() }
        following.forEach { $0.cancel() }
        pictureInPicture?.stopPictureInPicture()
        restoring?(false)
        restoring = nil
        player?.dispose()
        player = nil
        nowPlaying = nil
    }

    // MARK: State

    /// The work and its next episode, for a download as for the server:
    /// as downloaded while the server doesn't answer.
    private func load(workId: String) {
        tasks.append(Task { [weak self, server] in
            for await load in server.session.playback.playableWork(id: workId) {
                guard let self, self.workId == workId else { return }
                guard let work = load.getOrNull() else { continue }
                self.work = work
                self.updateNowPlaying()
                self.tellEpisode()
                guard let nextId = work.episode?.nextId, self.next?.id != nextId else { continue }
                // Without the server, the next episode only if it's downloaded.
                for await nextLoad in server.session.playback.playableWork(id: nextId) {
                    guard let next = nextLoad.getOrNull() else { continue }
                    if self.workId == workId {
                        self.next = next
                        self.tellEpisode()
                    }
                    break
                }
            }
        })
    }

    /// The core's player learns whether the subtitle overlay can show: not
    /// in Picture in Picture or on an AirPlay device, where only the video
    /// goes, so plays there ask for no subtitle files.
    private func tellOverlayShows() {
        player?.setOverlayVisible(visible: !pictureInPictureActive && !engine.player.isExternalPlaybackActive)
    }

    /// The core's player learns whether the work is an episode with a next one to play, as the two are read.
    private func tellEpisode() {
        guard let player, work?.episode != nil else { return }
        player.setHasNextEpisode(hasNext: next != nil)
    }

    private func stateChanged(_ state: PlayerState) {
        self.state = state
        waitToHide(again: false)
        if let play = state.play, play.id != playId {
            playId = play.id
            previews = nil
            tasks.append(Task { [weak self, server] in
                // Without previews, scrubbing shows the time and chapter alone.
                let previews = try? await server.session.playback.previews(play: play)
                guard let self, self.playId == play.id else { return }
                self.previews = previews
            })
        }
    }

    /// What shows over the video changed. When the player is done, the next
    /// episode plays, or the player gives way to the series' page; anything
    /// else stays with the controls shown.
    private func uiChanged(_ ui: PlayerUi) {
        let end = ui.end != self.ui?.end ? ui.end : nil
        self.ui = ui
        waitToHide(again: false)
        switch end {
        case .playNext:
            playNext()
        case .series:
            guard let episode = work?.episode else { return }
            server.showSeries(afterPlaying: episode.seriesId, offline: offline)
        case .close, nil:
            break
        }
    }

    /// The next episode plays: its download when it's ready, as Play does on its page, else from the server.
    private func playNext() {
        guard let next else { return }
        work = nil
        self.next = nil
        start(PlayTarget(workId: next.id, downloadId: server.readyDownload(of: next.id)?.id))
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
            self.server.playerShown = false
        }
        pictureInPictureEvents.didStop = { [weak self] in
            guard let self else { return }
            self.pictureInPictureActive = false
            self.tellOverlayShows()
            self.server.pictureInPictureEnded()
        }
        pictureInPictureEvents.restore = { [weak self] completion in
            guard let self else { return completion(false) }
            self.restoring = completion
            self.server.playerShown = true
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
            case .skipBack: controller.skip(deltaMs: -Markers.shared.BACK_MS)
            case .skipForward: controller.skip(deltaMs: Markers.shared.FORWARD_MS)
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
    var didStop: () -> Void = {}
    var restore: (@escaping (Bool) -> Void) -> Void = { $0(false) }

    func pictureInPictureControllerWillStartPictureInPicture(_ controller: AVPictureInPictureController) {
        willStart()
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
