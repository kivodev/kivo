import AVFoundation
import KivoCore
import KivoUI
import Observation

/// A video player's run, from play to play as up next moves it on: the
/// core's player over the AVPlayer engine, and what it shows around the
/// video. The work and the next episode, the tracks, versions and
/// qualities to choose, Skip Intro and Credits, up next with autoplay, and
/// the title, poster and overview Now Playing and the system's player show.
/// The controls around it are each app's own.
@MainActor
@Observable
final class VideoPlayback {
    /// Where the player goes once a play ends without a next episode to play here.
    enum Exit {
        /// The last episode of a series played to its end: the series' page.
        case series(String)
        /// Back where playback started.
        case back
    }

    let engine: AVPlayerEngine
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
    /// Up next's card, for an episode with a next one; it counts down while playing with no menu open.
    var upNext: UpNext { ui?.upNext ?? UpNext() }
    /// Whether the controls show. They hide 3 seconds after the last input
    /// while playing, but never while paused, scrubbing or with a menu open.
    var controlsVisible: Bool { ui?.controlsShown ?? true }
    /// The playback speed, 1 for normal.
    private(set) var speed: Float = 1
    /// A menu or panel is open over the video: the controls stay up and up next waits.
    var menuOpen = false {
        didSet { player?.setMenuOpen(open: menuOpen) }
    }
    /// The scrub bar is held.
    var scrubbing = false {
        didSet { player?.setScrubbing(held: scrubbing) }
    }
    /// Whether the subtitle overlay can show: not where only the video goes,
    /// as in Picture in Picture or on an AirPlay device, so plays there ask
    /// for no subtitle files.
    var overlayVisible = true {
        didSet { player?.setOverlayVisible(visible: overlayVisible) }
    }
    /// What the system's player says after an episode's series under the
    /// title, as Apple TV's "Ends at 9:41 PM"; nil for nothing.
    @ObservationIgnored var subtitleNote: String? {
        didSet { if subtitleNote != oldValue { updateNowPlaying() } }
    }
    /// Told where the player goes once a play ends without a next episode, once per play.
    @ObservationIgnored var exited: (Exit) -> Void = { _ in }

    @ObservationIgnored private let session: KivoSession
    @ObservationIgnored private let artwork: ArtworkSource
    @ObservationIgnored private var player: KivoCore.VideoPlayer?
    /// The work Now Playing shows.
    @ObservationIgnored private var nowPlayingWork: WorkDetail?
    /// The play went where its end took it.
    @ObservationIgnored private var ended = false
    @ObservationIgnored private var tasks: [Task<Void, Never>] = []
    /// Now Playing's poster, and the image it is.
    @ObservationIgnored private var poster: (id: String, data: Data?)?

    init(session: KivoSession, display: Display, artwork: ArtworkSource, subtitleStyle: SubtitleStyle, workId: String) {
        self.session = session
        self.display = display
        self.artwork = artwork
        self.workId = workId
        engine = AVPlayerEngine(baseUrl: { session.baseUrl }, subtitleStyle: subtitleStyle)
    }

    /// Plays [choice] from [positionMs], or its resume position when nil.
    /// Everything but where the play comes from is the same for the
    /// download [downloadId] as for the server: a download plays from its
    /// files, without the server. The engine stays the same across the
    /// plays: the last play's picture shows until the next one's replaces it.
    func start(_ choice: PlayChoice, at positionMs: Int64? = nil, downloadId: String?) {
        player?.handOver()
        workId = choice.workId
        // Playback audio, so the video plays with the silent switch on and
        // Picture in Picture and AirPlay are allowed. Set on every play, as
        // the system can reset it.
        try? AVAudioSession.sharedInstance().setCategory(.playback, mode: .moviePlayback)
        let player = session.playback.videoPlayer(
            engine: engine,
            choice: choice,
            hideControlsAfterMs: 3_000,
            downloadId: downloadId,
            positionMs: positionMs.map { KotlinLong(value: $0) }
        )
        self.player = player
        ended = false
        player.setMenuOpen(open: menuOpen)
        player.setScrubbing(held: scrubbing)
        player.setOverlayVisible(visible: overlayVisible)
        playing = Observed(player.controller.state)
        shown = Observed(player.screen) { [weak self] screen in self?.screenChanged(screen, of: player) }
    }

    var controller: PlayController? { player?.controller }

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

    var durationMs: Int64 { Int64(play?.durationMs ?? 0) }

    var positionMs: Int64 { state?.positionMs ?? 0 }

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

    /// Where the play is, to carry on from elsewhere: the work, its version and tracks, and the position.
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
        if let target = request.url, !target.isFileURL, let authorization = session.authorization(url: target.absoluteString) {
            request.setValue(authorization, forHTTPHeaderField: "Authorization")
        }
        let (data, response) = try await URLSession.shared.data(for: request)
        if let http = response as? HTTPURLResponse, http.statusCode != 200 { throw URLError(.badServerResponse) }
        return data
    }

    /// The title: the movie with its edition, or the episode's series.
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
        guard let work, durationMs > 0 else { return nil }
        let remaining = Double(durationMs - positionMs) / Double(speed)
        return Words.endsAt(kind: work.kind, remainingMs: Int64(remaining), from: now)
    }

    /// Whether Subtitles offers "Search subtitles…": a video the server
    /// plays, even with no subtitle track, never a download from its files.
    var searchesSubtitles: Bool {
        SubtitleSearch.shared.offered(version: version, offline: offline)
    }

    // MARK: Commands

    /// Any input: the controls show, their wait to hide starts again, and
    /// someone is watching, which resets up next's run.
    func input() {
        player?.showControls()
    }

    /// Someone is watching, with the controls left as they are.
    func userActivity() {
        player?.userActivity()
    }

    /// The controls show, or hide if they're showing.
    func toggleControls() {
        player?.toggleControls()
    }

    /// The controls hide, as the system's own do.
    func hideControls() {
        player?.hideControls()
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

    /// A skip that counts as someone watching and leaves the controls as they are.
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

    func searchSubtitles() async throws -> [SubtitleResult] {
        guard let version else { return [] }
        return try await session.playback.subtitleResults(version: version)
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

    /// The app went to the background: the play stops; coming back starts it again where it was.
    func stopOnExit() {
        controller?.stopOnExit()
    }

    func restore() {
        controller?.restore()
    }

    /// The player is done: the play stops and everything is released.
    func close() {
        tasks.forEach { $0.cancel() }
        player?.dispose()
        player = nil
    }

    // MARK: State

    /// What [player] shows around the video changed: Now Playing follows
    /// the work, and once the player is done, the next episode plays in its
    /// place, or the player gives way to the series' page or closes, once.
    private func screenChanged(_ screen: PlayerScreenState, of player: KivoCore.VideoPlayer) {
        guard player === self.player else { return }
        if let work = screen.work, work !== nowPlayingWork {
            nowPlayingWork = work
            updateNowPlaying()
        }
        guard let exit = screen.exit, !ended else { return }
        ended = true
        switch onEnum(of: exit) {
        case let .next(next):
            start(next.choice, downloadId: next.downloadId)
        case let .series(series):
            exited(.series(series.seriesId))
        case .back:
            exited(.back)
        }
    }

    /// What Now Playing and the system's player show of the work: the
    /// title, the series and the subtitle note, the overview and the
    /// poster, on every item the engine loads. The poster comes through the
    /// artwork caches, so one seen before shows without the server.
    private func updateNowPlaying() {
        guard let work else { return }
        let image = work.images.poster ?? work.images.still
        if let image, poster?.id != image.id {
            poster = (image.id, nil)
            tasks.append(Task { [weak self, artwork] in
                let loaded = await artwork.image(id: image.id, pixelWidth: 600)
                guard let self, self.poster?.id == image.id else { return }
                self.poster = (image.id, loaded?.jpegData(compressionQuality: 0.9))
                self.updateNowPlaying()
            })
        }
        let parts = [work.episode?.seriesTitle, subtitleNote].compactMap { $0 }
        let subtitle = parts.isEmpty ? nil : parts.joined(separator: " · ")
        engine.metadata = [
            Self.metadataItem(.commonIdentifierTitle, work.title as NSString),
            subtitle.map { Self.metadataItem(.iTunesMetadataTrackSubTitle, $0 as NSString) },
            work.overview.map { Self.metadataItem(.commonIdentifierDescription, $0 as NSString) },
            poster?.data.map { Self.metadataItem(.commonIdentifierArtwork, $0 as NSData, dataType: kCMMetadataBaseDataType_JPEG as String) },
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

/// What Stream info shows: the play's tracks, each as it's delivered, the play's delivery and where it plays.
struct StreamInfo {
    let tracks: [PlayTrack]
    /// `stream` or `file`; nil on a cast, which only the TV knows.
    let delivery: PlayDelivery?
    let local: Bool
    let bitrateBps: Int64?
    /// Where a download playing came from.
    let downloadSource: DownloadSource?
    /// The Cast device playing it.
    let client: String?
}
