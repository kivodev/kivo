import KivoCore
import KivoUI
import Observation
import UIKit

/// Casting to a TV, as the Cast button, the mini controller, the remote and
/// the Cast menu show it: the core's ``CastRemote`` over Google Cast, which
/// decides everything, and what's on screen. The remote's sheets are the
/// player's (``PlayerChoices``), acting on the TV.
@MainActor
@Observable
final class CastModel: PlayerChoices, Scrubbable {
    enum Connection {
        case disconnected
        case connecting
        case connected
    }

    /// Whether the Cast menu shows: the lit Cast button opens it.
    var menuShown = false
    var sheet: PlayerSheet?
    /// A finger is on the remote's scrubber.
    var scrubbing = false

    /// A Cast device connected newly, not one joined again as the app came back.
    @ObservationIgnored var connected: () -> Void = {}
    /// Nothing plays on the TV any more, or the session ended.
    @ObservationIgnored var stopped: () -> Void = {}

    let display: Display
    @ObservationIgnored private let session: KivoSession
    @ObservationIgnored private let remote: CastRemote
    /// The system's list of Cast devices to connect to.
    @ObservationIgnored private let showDevices: () -> Void
    @ObservationIgnored private var observed: Observed<CastRemoteState>?
    @ObservationIgnored private var connections: Task<Void, Never>?
    /// Whether the last state had something on the TV.
    @ObservationIgnored private var wasActive = false

    init(session: KivoSession, display: Display) {
        self.session = session
        self.display = display
        let (link, showDevices) = Self.makeLink(session: session)
        self.showDevices = showDevices
        remote = session.cast.remote(link: link)
        observed = Observed(remote.state) { [weak self] state in
            self?.changed(state)
        }
        connections = Task { [weak self, remote] in
            for await _ in remote.connections { self?.connected() }
        }
    }

    private static func makeLink(session: KivoSession) -> (any KivoCore.CastLink, () -> Void) {
        #if DEBUG
        if let scene = CastPreview.scene {
            let link = PreviewCastLink(scene: scene, session: session)
            return (link, link.showDevices)
        }
        #endif
        return (GoogleCastLink(), GoogleCastLink.showDevices)
    }

    private var state: CastRemoteState { observed?.value ?? remote.state.value }

    /// Tells the app once the TV has nothing on it any more. A state with
    /// nothing on it yet, as a newly connected TV's before a hand-off's
    /// cast, isn't a stop: it can arrive after the hand-off began.
    private func changed(_ state: CastRemoteState) {
        if wasActive, !state.active { stopped() }
        wasActive = state.active
    }

    var connection: Connection {
        switch onEnum(of: state.link) {
        case .unavailable, .disconnected: .disconnected
        case .connecting: .connecting
        case .connected: .connected
        }
    }

    /// The Cast device's name, such as "Living Room TV".
    var device: String? { state.device }
    /// The TV's volume, 0 to 1.
    var volume: Float { state.volume }
    var media: CastMedia? { state.media }
    /// What the receiver says of its play.
    var status: CastStatus? { state.status }
    /// The work on the TV, and its next episode for up next.
    var work: WorkDetail? { state.work }
    var next: WorkDetail? { state.next }
    var positionMs: Int64 { state.positionMs }
    var durationMs: Int64 { state.durationMs }
    var active: Bool { state.active }
    var playing: Bool { state.playing }
    /// Loading or buffering on the TV: a spinner takes play's place.
    var waiting: Bool { state.waiting }

    /// Whether the Cast button shows: hidden with no Cast device found, as off Wi-Fi.
    var buttonShown: Bool { !(state.link is CastLinkStateUnavailable) }

    /// What's wrong, in a short line that Try again answers; nil while all is well.
    var problem: String? {
        state.problem.map(Self.words)
    }

    /// The movie, or the episode's series.
    var title: String {
        guard let work else { return "" }
        return work.episode?.seriesTitle ?? work.title
    }

    /// An episode's "S3 · E4 · Reload", or a movie's "1990 · PG-13".
    var line: String? {
        guard let work else { return nil }
        if let episode = work.episode { return "\(Display.app.episodeCode(episode: episode)) · \(work.title)" }
        let parts = [work.year.map { String($0.intValue) }, work.contentRating].compactMap { $0 }
        return parts.isEmpty ? nil : parts.joined(separator: " · ")
    }

    var casting: String {
        String(localized: "Casting to \(device ?? "")")
    }

    // MARK: Commands

    /// The Cast button: the list of Cast devices, or connected, the Cast menu.
    func buttonTapped() {
        if connection == .connected {
            menuShown = true
        } else {
            showDevices()
        }
    }

    /// Casts [choice] from [positionMs], else from its start with [fromStart], else its resume position.
    func cast(_ choice: PlayChoice, fromStart: Bool = false, at positionMs: Int64? = nil) {
        let start = positionMs ?? (fromStart ? 0 : nil)
        _ = remote.cast(choice: choice, positionMs: start.map { KotlinLong(value: $0) })
    }

    func togglePlay() { remote.togglePlay() }

    func skip(by deltaMs: Int64) { remote.skip(deltaMs: deltaMs) }

    func seek(to positionMs: Int64) { remote.seek(positionMs: positionMs) }

    func setVolume(_ volume: Float) { remote.setVolume(volume: volume) }

    func chooseAudio(_ trackId: String) { remote.chooseAudio(trackId: trackId) }

    func chooseSubtitle(_ trackId: String) { remote.chooseSubtitle(trackId: trackId) }

    func boostDialogue(_ on: Bool) { remote.boostDialogue(on: on) }

    func chooseQuality(_ option: QualityOption) { remote.chooseQuality(quality: option.quality) }

    func chooseVersion(_ versionId: String) { remote.chooseVersion(versionId: versionId) }

    func setSpeed(_ speed: Float) { remote.setSpeed(speed: speed) }

    func playNextNow() { remote.playNext() }

    func hideUpNext() { remote.hideUpNext() }

    func retry() { remote.retry() }

    /// Stop casting: the TV stops, saving the position, and what played
    /// there is returned to carry on with here, at the TV's position.
    func stopCasting() -> ResumePoint? {
        menuShown = false
        sheet = nil
        return remote.stopCasting()
    }

    /// Disconnect: this device lets go, and the TV plays on.
    func disconnect() {
        menuShown = false
        sheet = nil
        remote.disconnect()
    }

    /// The app signed out: the TV plays on, and this model stops following it.
    func close() {
        connections?.cancel()
        remote.close()
    }

    // MARK: Choices

    var version: Version? { state.version }

    var offline: Bool { false }

    var audioTrackId: String? { status?.audioTrackId }

    var dialogueBoost: DialogueBoost? { status?.dialogueBoost }

    var subtitle: String { status?.subtitleTrackId ?? SUBTITLES_OFF }

    var qualities: [QualityOption] { state.qualities }

    var quality: Quality? { state.quality }

    var speed: Float { media?.speed ?? 1 }

    var streamInfo: StreamInfo? {
        guard let status, !status.tracks.isEmpty else { return nil }
        return StreamInfo(
            tracks: status.tracks,
            delivery: nil,
            local: status.local?.boolValue ?? true,
            bitrateBps: nil,
            downloadSource: nil,
            client: device
        )
    }

    func offers(_ track: Track) -> Bool { true }

    /// The TV plays the server's play, so its subtitles can be searched for as the player's can.
    var searchesSubtitles: Bool {
        SubtitleSearch.shared.offered(version: version)
    }

    func searchSubtitles() async throws -> [SubtitleResult] {
        guard let version else { return [] }
        return try await session.playback.subtitleResults(version: version)
    }

    /// The version's tracks gain the subtitle at once, and the TV plays on with it.
    func downloadSubtitle(_ result: SubtitleResult) async throws {
        try await remote.addSubtitle(resultId: result.id)
    }

    // MARK: Scrubbing

    var scrubDurationMs: Int64 { durationMs }

    var scrubPositionMs: Int64 { positionMs }

    var chapters: [Chapter] { version?.chapters ?? [] }

    /// The play's previews stay on the TV, whose play's ID mustn't leave it.
    var previews: ScrubPreviews? { nil }
}

extension CastModel {
    /// Why the cast stopped or couldn't start, in words: an admin's own when they stopped the play.
    static func words(_ problem: CastProblem) -> String {
        switch onEnum(of: problem) {
        case let .refused(refused): return refused.problem.sentence
        case .unreachable: return String(localized: "Can't reach the server.")
        case let .receiver(receiver): return failure(receiver.code, message: receiver.message)
        }
    }

    /// The receiver's error in words: an admin's own when they stopped the play.
    static func failure(_ code: String, message: String?) -> String {
        if let message, !message.isEmpty { return message }
        switch ProblemCode.companion.of(value: code) {
        case .signedOut: return String(localized: "Signed out on the TV.")
        case .playEnded: return String(localized: "Playback was stopped.")
        case .serverOutdated: return String(localized: "Update the server to cast to this TV.")
        case .receiverOutdated: return String(localized: "The TV's receiver is older than the server.")
        case .notPlayable, .noFallback, .playbackFailed: return String(localized: "This can't be played on this TV.")
        case .invalidRequest: return Display.app.problem(code: "")
        default: return Display.app.problem(code: code)
        }
    }
}
