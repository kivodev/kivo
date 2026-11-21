import AVFoundation
import KivoCore
import KivoUI
import MediaPlayer
import Observation
import UIKit

/// The book player's platform side: the core's BookPlayerModel, which decides everything, over an audio-only
/// AVPlayer engine, with what only iOS does: the audio session in the
/// background and its interruptions, the sleep timer's fade, and Now
/// Playing with the lock screen's and headphones' commands. The mini player
/// and Now Playing draw it. One per signed-in server.
///
/// The [state] changes several times a second while a book plays, for Now
/// Playing's seek bar; what the mini player, the tab bar and a book's page
/// show is kept apart and set only when it changes, so they don't redraw
/// with every tick (a redrawn tab bar accessory drops a tap in progress).
@MainActor
@Observable
final class BookPlayer {
    let model: BookPlayerModel
    private(set) var state: BookPlayerState
    /// Now Playing shows over everything while this is set.
    var expanded = false
    /// A book is loaded: the mini player shows.
    private(set) var loaded = false
    private(set) var work: WorkDetail?
    private(set) var playing = false
    /// Opening the book or its play, before it can play.
    private(set) var loading = false
    /// The chapter playing, from 0.
    private(set) var chapterIndex: Int32?
    /// "Chapter 12 · 4 hr 21 min left", or why it isn't playing.
    private(set) var miniLine: String?
    /// Why it isn't playing, while it isn't: Play tries again.
    private(set) var failureLine: String?
    /// How far through the book, to a thousandth.
    private(set) var fraction: Double = 0
    private(set) var hasNext = false
    /// The user's speed for books.
    private(set) var speed: Double = 1
    /// The sleep timer while it runs, its time left to the second.
    private(set) var sleep: SleepShown?
    /// The timer that Play starts again, for the 15 seconds after it paused the book.
    private(set) var sleepRestart: SleepChoice?
    /// When that play stops starting it again, for the countdown.
    private(set) var sleepRestartEnds = Date.distantPast
    /// The title and cover the page that played the book already had, shown until the book is read.
    private var knownTitle: String?
    private var knownPoster: ImageRef?

    /// The book's title, the page's while the book opens.
    var title: String { work?.title ?? knownTitle ?? "" }

    /// The book's cover, the page's while the book opens.
    var poster: ImageRef? { work?.images.poster ?? knownPoster }

    /// Whether a video player has the audio, which the book then leaves to it. The server sets it.
    @ObservationIgnored var videoPlaying: () -> Bool = { false }

    @ObservationIgnored private let engine: AVPlayerEngine
    @ObservationIgnored private let audio: BookAudioOutput
    /// Artwork at the server's address in use.
    @ObservationIgnored private let artwork: () -> ArtworkSource
    @ObservationIgnored private var stateTask: Task<Void, Never>?
    @ObservationIgnored private var coverTask: Task<Void, Never>?
    @ObservationIgnored private var observers: [NSObjectProtocol] = []
    /// Now Playing and the remote commands, the book's own, as the video
    /// player has its own: the system sends them to the app's active session,
    /// which the book becomes as it loads.
    @ObservationIgnored let nowPlaying: MPNowPlayingSession
    @ObservationIgnored private var commands: [(MPRemoteCommand, Any)] = []
    @ObservationIgnored private var cover: UIImage?
    @ObservationIgnored private var coverOf: String?
    @ObservationIgnored private var published: (key: NowPlayingKey, elapsedMs: Int64, at: Date)?
    @ObservationIgnored private var resumeAfterInterruption = false
    /// The listener's last word, play or pause, whatever the engine is doing at the moment.
    @ObservationIgnored private var wantsPlaying = false
    /// The last book is being put back, paused: it leaves other apps' audio alone until it plays.
    @ObservationIgnored private var restoring = false
    @ObservationIgnored private var sessionActive = false
    /// Signing out, which gives the audio back as Stop listening does.
    @ObservationIgnored private var closing = false
    /// Stop listening forgets the book as it unloads.
    @ObservationIgnored private var forgetBook = false
    /// The book loaded, as kept in UserDefaults for the next launch.
    @ObservationIgnored private var savedBook: String?
    @ObservationIgnored private let lastBookKey: String

    @ObservationIgnored private let display: Display

    init(session: KivoSession, artwork: @escaping () -> ArtworkSource, display: Display) {
        self.artwork = artwork
        self.display = display
        lastBookKey = "lastBook.\(session.savedServer.value.id)"
        engine = AVPlayerEngine(baseUrl: { session.baseUrl }, subtitleStyle: SubtitleStyle(size: .medium, background: .none))
        engine.pitchAlgorithm = .spectral
        audio = BookAudioOutput(player: engine.player)
        nowPlaying = MPNowPlayingSession(players: [engine.player])
        model = session.playback.bookPlayer(engine: engine, audio: audio)
        state = model.state.value
        stateTask = Task { [weak self, model] in
            for await state in model.state {
                self?.changed(state)
            }
        }
        setUpCommands()
        observeAudioSession()
    }

    var progress: BookProgress? { state.progress }

    var chapters: [Chapter] { state.chapters?.list ?? [] }

    // MARK: Commands

    /// Plays a book from where the user left it, or from [positionMs], such as a chapter's start.
    /// The page's [title] and [poster] show while the book opens.
    func play(_ workId: String, at positionMs: Int64? = nil, title: String? = nil, poster: ImageRef? = nil) {
        knownTitle = title
        knownPoster = poster
        restoring = false
        forgetBook = false
        wantsPlaying = true
        model.play(workId: workId, positionMs: positionMs.map { KotlinLong(value: $0) })
    }

    /// Puts back the book that was loaded when the app last ran, paused at
    /// its place, where [canOpen] says it can be read.
    func restore(canOpen: (String) -> Bool) {
        guard !loaded, !videoPlaying(),
              let workId = UserDefaults.standard.string(forKey: lastBookKey), canOpen(workId)
        else { return }
        restoring = true
        model.load(workId: workId)
    }

    func togglePlay() {
        if !playing || failureLine != nil { intendsToPlay() } else { wantsPlaying = false }
        model.togglePlay()
    }

    /// The listener asked for it to play: a book put back paused is theirs again, its failures shown.
    private func intendsToPlay() {
        wantsPlaying = true
        if restoring {
            restoring = false
            takeAudioSession()
        }
    }

    func skipBack() { model.skipBack() }

    func skipForward() { model.skipForward() }

    func previousChapter() { model.previousChapter() }

    func nextChapter() { model.nextChapter() }

    func playChapter(_ index: Int) {
        intendsToPlay()
        model.playChapter(index: Int32(index))
    }

    func seek(chapter index: Int32, offsetMs: Int64) { model.seekInChapter(index: index, offsetMs: offsetMs) }

    func setSpeed(_ speed: Double) { model.setSpeed(speed: speed) }

    func setSleep(_ choice: SleepChoice) { model.setSleep(choice: choice) }

    func stopSleep() { model.stopSleep() }

    /// Stop listening: the place is saved and the player goes.
    func stop() {
        expanded = false
        forgetBook = loaded
        model.stop()
    }

    /// Signed out: the book stops and the lock screen lets go.
    func close() {
        model.stop()
        stateTask?.cancel()
        if loaded {
            loaded = false
            closing = true
            unloaded()
        }
        savedBook = nil
        UserDefaults.standard.removeObject(forKey: lastBookKey)
        observers.forEach(NotificationCenter.default.removeObserver)
        for (command, target) in commands { command.removeTarget(target) }
        commands = []
        nowPlaying.nowPlayingInfoCenter.nowPlayingInfo = nil
    }

    // MARK: State

    private func changed(_ next: BookPlayerState) {
        let wasLoaded = loaded
        state = next
        if restoring, next.failure != nil {
            // Not there to put back: no mini player rather than its error.
            restoring = false
            model.stop()
            return
        }
        update(\.loaded, next.workId != nil)
        update(\.work, next.work)
        update(\.playing, next.playing)
        update(\.loading, next.player.status == .loading && next.failure == nil)
        update(\.chapterIndex, next.progress?.chapterIndex)
        let failureLine = next.failure.map { display.failure(failure: $0) }
        update(\.failureLine, failureLine)
        update(\.miniLine, failureLine ?? next.progress.map { BookWords.miniLine($0, speed: next.speed) })
        update(\.fraction, ((next.progress?.fraction ?? 0) * 1000).rounded() / 1000)
        update(\.hasNext, next.progress?.hasNext == true)
        update(\.speed, next.speed)
        update(\.sleep, (next.sleep as? SleepState.Running).map { SleepShown(choice: $0.choice, seconds: ($0.remainingMs + 999) / 1000) })
        if next.sleepRestart != nil, sleepRestart == nil {
            sleepRestartEnds = Date().addingTimeInterval(Double(BookPlayerModel.companion.SLEEP_RESTART_MS) / 1000)
            wantsPlaying = false
        }
        update(\.sleepRestart, next.sleepRestart)
        if next.failure != nil || next.player.ended { wantsPlaying = false }
        if next.workId == nil {
            if wasLoaded { unloaded() }
            return
        }
        if savedBook != next.workId {
            savedBook = next.workId
            UserDefaults.standard.set(next.workId, forKey: lastBookKey)
        }
        if !wasLoaded { activateAudio() }
        if next.playing {
            restoring = false
            takeAudioSession()
        }
        if let work = next.work, coverOf != work.id {
            coverOf = work.id
            cover = nil
            coverTask?.cancel()
            coverTask = nil
            if let image = work.images.poster {
                coverTask = Task { [weak self, artwork = artwork()] in
                    let loaded = await artwork.image(id: image.id, pixelWidth: 600)
                    guard let self, self.coverOf == work.id else { return }
                    self.cover = loaded
                    self.published = nil
                    self.publishNowPlaying()
                }
            }
        }
        publishNowPlaying()
    }

    /// Sets an observed property only when it changes, so its views redraw only then.
    private func update<Value: Equatable>(_ key: ReferenceWritableKeyPath<BookPlayer, Value>, _ value: Value) {
        if self[keyPath: key] != value { self[keyPath: key] = value }
    }

    /// The book stopped, by Stop listening, a video starting or signing out.
    private func unloaded() {
        expanded = false
        knownTitle = nil
        knownPoster = nil
        restoring = false
        wantsPlaying = false
        resumeAfterInterruption = false
        // Stop listening gives the audio back; a video starting takes it over instead.
        let releasesAudio = forgetBook || closing
        if forgetBook {
            forgetBook = false
            savedBook = nil
            UserDefaults.standard.removeObject(forKey: lastBookKey)
        }
        coverTask?.cancel()
        coverTask = nil
        coverOf = nil
        cover = nil
        published = nil
        nowPlaying.nowPlayingInfoCenter.nowPlayingInfo = nil
        nowPlaying.nowPlayingInfoCenter.playbackState = .stopped
        for (command, _) in commands { command.isEnabled = false }
        // The video player's mode again.
        try? AVAudioSession.sharedInstance().setCategory(.playback, mode: .moviePlayback)
        // Other apps' audio plays on, unless a video has the session now.
        if sessionActive, releasesAudio, !videoPlaying() {
            try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
        }
        sessionActive = false
    }

    /// Spoken audio that plays in the background and over AirPlay as
    /// long-form audio, with the book's commands as the active Now Playing.
    /// A book put back paused takes the session only as it plays.
    private func activateAudio() {
        try? AVAudioSession.sharedInstance().setCategory(.playback, mode: .spokenAudio, policy: .longFormAudio)
        if !restoring { takeAudioSession() }
        for (command, _) in commands { command.isEnabled = true }
        nowPlaying.becomeActiveIfPossible { _ in }
    }

    private func takeAudioSession() {
        guard !sessionActive else { return }
        sessionActive = true
        try? AVAudioSession.sharedInstance().setActive(true)
    }

    // MARK: Interruptions and routes

    private func observeAudioSession() {
        let center = NotificationCenter.default
        observers.append(center.addObserver(forName: AVAudioSession.interruptionNotification, object: nil, queue: .main) { [weak self] note in
            MainActor.assumeIsolated { self?.interrupted(note) }
        })
        observers.append(center.addObserver(forName: AVAudioSession.routeChangeNotification, object: nil, queue: .main) { [weak self] note in
            MainActor.assumeIsolated { self?.routeChanged(note) }
        })
    }

    /// A call or Siri took the audio: the player paused itself; afterwards it plays on if the listener had it playing and iOS says it may.
    private func interrupted(_ note: Notification) {
        guard loaded,
              let raw = note.userInfo?[AVAudioSessionInterruptionTypeKey] as? UInt,
              let type = AVAudioSession.InterruptionType(rawValue: raw)
        else { return }
        switch type {
        case .began:
            resumeAfterInterruption = wantsPlaying
            wantsPlaying = false
        case .ended:
            let options = (note.userInfo?[AVAudioSessionInterruptionOptionKey] as? UInt).map(AVAudioSession.InterruptionOptions.init) ?? []
            if resumeAfterInterruption, options.contains(.shouldResume) {
                wantsPlaying = true
                model.resume()
            }
            resumeAfterInterruption = false
        @unknown default:
            break
        }
    }

    /// Headphones unplugged, or a Bluetooth device gone: the book pauses rather than playing out of the speaker.
    private func routeChanged(_ note: Notification) {
        guard loaded,
              let raw = note.userInfo?[AVAudioSessionRouteChangeReasonKey] as? UInt,
              AVAudioSession.RouteChangeReason(rawValue: raw) == .oldDeviceUnavailable
        else { return }
        wantsPlaying = false
        model.pause()
    }

    // MARK: Now Playing and remote commands

    /// The lock screen, Control Center, CarPlay, headphones and a car's
    /// steering wheel, as RemoteCommands has them for a book: off until a
    /// book loads. The scrubber spans the chapter.
    private func setUpCommands() {
        commands = RemoteCommands.setUp(nowPlaying.remoteCommandCenter, for: .book, enabled: false) { [weak self] action, event in
            guard let self else { return .commandFailed }
            switch action {
            case .play:
                intendsToPlay()
                model.resume()
            case .pause:
                wantsPlaying = false
                model.pause()
            case .togglePlay: togglePlay()
            case .skipBack: model.skipBack()
            case .skipForward: model.skipForward()
            case .seek:
                guard let event = event as? MPChangePlaybackPositionCommandEvent, let progress else { return .commandFailed }
                seek(chapter: progress.chapterIndex, offsetMs: Int64(event.positionTime * 1000))
            }
            return .success
        }
    }

    /// Publishes the book to Now Playing when what it shows changes: the
    /// chapter as the title, the book and its author, the chapter's time,
    /// the rate and the cover. Between, the system runs the clock from the rate.
    private func publishNowPlaying() {
        guard let work = state.work, let progress else { return }
        // The clock runs only while the book does: not while it buffers or seeks.
        let running = playing && state.player.status == .playing
        let key = NowPlayingKey(workId: work.id, chapter: progress.chapterIndex, playing: playing, running: running, speed: state.speed)
        if let published, published.key == key {
            // The clock as the system runs it; further off than 2 seconds is a seek.
            let ran = running ? Int64(Date().timeIntervalSince(published.at) * 1000 * state.speed) : 0
            if abs(published.elapsedMs + ran - progress.chapterElapsedMs) < 2000 { return }
        }
        published = (key, progress.chapterElapsedMs, Date())
        var info = Self.nowPlayingInfo(
            chapter: BookWords.chapter(progress),
            book: work.title,
            authors: BookWords.authors(work),
            durationMs: progress.chapterDurationMs,
            elapsedMs: progress.chapterElapsedMs,
            playing: running,
            speed: state.speed
        )
        if let cover {
            info[MPMediaItemPropertyArtwork] = MPMediaItemArtwork(boundsSize: cover.size) { _ in cover }
        }
        let center = nowPlaying.nowPlayingInfoCenter
        center.nowPlayingInfo = info
        center.playbackState = playing ? .playing : .paused
    }

    /// Now Playing for a book: the chapter as the title, the book as the
    /// album, the chapter's time and the rate. The chapter's number and
    /// count stay out, so a car's track buttons reach previous and next track.
    nonisolated static func nowPlayingInfo(
        chapter: String, book: String, authors: String?, durationMs: Int64, elapsedMs: Int64, playing: Bool, speed: Double
    ) -> [String: Any] {
        var info: [String: Any] = [
            MPMediaItemPropertyTitle: chapter,
            MPMediaItemPropertyAlbumTitle: book,
            MPMediaItemPropertyPlaybackDuration: Double(durationMs) / 1000,
            MPNowPlayingInfoPropertyElapsedPlaybackTime: Double(elapsedMs) / 1000,
            MPNowPlayingInfoPropertyPlaybackRate: playing ? speed : 0.0,
            MPNowPlayingInfoPropertyDefaultPlaybackRate: speed,
            MPNowPlayingInfoPropertyMediaType: MPNowPlayingInfoMediaType.audio.rawValue,
        ]
        if let authors { info[MPMediaItemPropertyArtist] = authors }
        return info
    }
}

/// The sleep timer as its button shows it.
struct SleepShown: Equatable {
    let choice: SleepChoice
    let seconds: Int64
}

/// What Now Playing shows that the system's clock doesn't move on by itself.
private struct NowPlayingKey: Equatable {
    let workId: String
    let chapter: Int32
    let playing: Bool
    let running: Bool
    let speed: Double
}
