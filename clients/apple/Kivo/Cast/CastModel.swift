import KivoCore
import KivoUI
import Observation
import UIKit

/// Casting to a TV: whether Cast devices are found, the session with
/// one, and what its receiver plays, which the Cast button, the mini
/// controller, the remote and the Cast menu show. A LOAD carries a cast
/// token, so the TV plays as this user; the receiver starts, heartbeats and
/// changes its play itself, and this model only mirrors its status and
/// sends its messages. The remote's sheets are the player's
/// (``PlayerChoices``), acting on the TV.
@MainActor
@Observable
final class CastModel: PlayerChoices, Scrubbable {
    enum Connection {
        case disconnected
        case connecting
        case connected
    }

    /// Whether Cast devices are found on this network.
    private(set) var available = false
    private(set) var connection = Connection.disconnected
    /// The Cast device's name, such as "Living Room TV".
    private(set) var device: String?
    /// The TV's volume, 0 to 1.
    private(set) var volume: Float = 0
    private(set) var media: CastMedia?
    /// What the receiver says of its play.
    private(set) var status: CastStatus?
    /// The work on the TV, and its next episode for up next.
    private(set) var work: WorkDetail?
    private(set) var next: WorkDetail?
    /// Where the TV is, between its status updates.
    private(set) var positionMs: Int64 = 0
    /// A LOAD on its way, by its work, until the receiver's media names it.
    private(set) var loading: String?
    /// Why the cast stopped or couldn't start, in words: the server
    /// refused a cast token, or the receiver stopped with an error.
    private(set) var failure: String?
    private(set) var qualities: [QualityOption] = []
    private(set) var qualityChoice: Quality?
    /// Whether the Cast menu shows: the lit Cast button opens it.
    var menuShown = false
    var sheet: PlayerSheet?
    /// A finger is on the remote's scrubber, which then shows where it is.
    var scrubbing = false

    /// The session connected; [resumed] for one joined again as the app came back.
    @ObservationIgnored var connected: (_ resumed: Bool) -> Void = { _ in }
    /// Nothing plays on the TV any more, or the session ended.
    @ObservationIgnored var stopped: () -> Void = {}

    let display: Display
    @ObservationIgnored private let session: KivoSession
    @ObservationIgnored private let core: KivoClient
    @ObservationIgnored private let link: CastLink
    @ObservationIgnored private var ticking: Task<Void, Never>?
    @ObservationIgnored private var reading: Task<Void, Never>?
    @ObservationIgnored private var readingNext: Task<Void, Never>?
    /// The last cast asked for, and where it started, for Try again before the TV has a play.
    @ObservationIgnored private var lastCast: (target: PlayTarget, positionMs: Int64?)?

    init(session: KivoSession, core: KivoClient, display: Display) {
        self.session = session
        self.core = core
        self.display = display
        link = Self.makeLink(session: session)
        link.delegate = self
        (link as? GoogleCastLink)?.start()
    }

    private static func makeLink(session: KivoSession) -> CastLink {
        #if DEBUG
        if let scene = CastPreview.scene { return PreviewCastLink(scene: scene, session: session) }
        #endif
        return GoogleCastLink()
    }

    /// Whether something is on the TV, on its way, or stopped with an
    /// error: the mini controller and the remote show.
    var active: Bool {
        connection == .connected && (media != nil || loading != nil || failure != nil)
    }

    /// Whether the Cast button shows: hidden with no Cast device found, as off Wi-Fi.
    var buttonShown: Bool {
        available || connection != .disconnected
    }

    var playing: Bool { media?.state == .playing }

    /// Loading or buffering on the TV: a spinner takes play's place.
    var waiting: Bool {
        guard let media else { return loading != nil }
        return media.state == .loading || media.state == .buffering
    }

    var durationMs: Int64 {
        if let media, media.durationMs > 0 { return media.durationMs }
        return Int64(work?.durationMs?.intValue ?? 0)
    }

    /// The receiver can't reach the server, and keeps trying.
    var unreachable: Bool { status?.unreachable == true }

    /// What's wrong, in a short line that Try again answers; nil while all is well.
    var problem: String? {
        failure ?? (unreachable ? String(localized: "Can't reach the server.") : nil)
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
            link.showDevices()
        }
    }

    /// Casts [target] from [positionMs], else from its start with fromStart, else its resume position,
    /// carrying the run of episodes this app's players played on without input.
    func cast(_ target: PlayTarget, positionMs: Int64? = nil) {
        guard connection == .connected, let device else { return }
        lastCast = (target, positionMs)
        loading = target.workId
        failure = nil
        status = nil
        qualityChoice = nil
        read(workId: target.workId)
        let start = positionMs ?? (target.fromStart ? 0 : nil)
        Task { [weak self, session, link] in
            do {
                let load = try await session.cast.load(
                    receiverName: device,
                    workId: target.workId,
                    versionId: target.versionId,
                    audioTrackId: target.audioTrackId,
                    subtitleTrackId: target.subtitleTrackId,
                    positionMs: start.map { KotlinLong(value: $0) }
                )
                let work = self?.work
                let art = work?.images.poster ?? work?.images.still ?? work?.images.backdrop
                link.load(
                    load,
                    title: work?.title ?? "",
                    image: art.flatMap { URL(string: session.imageUrl(id: $0.id, width: 480)) }
                )
            } catch {
                guard let self, self.loading == target.workId else { return }
                self.loading = nil
                self.failure = error.problemSentence
                self.changed()
            }
        }
    }

    func togglePlay() {
        playing ? link.pause() : link.play()
    }

    func skip(by deltaMs: Int64) {
        seek(to: positionMs + deltaMs)
    }

    func seek(to positionMs: Int64) {
        let target = min(max(0, positionMs), max(durationMs - 1000, 0))
        self.positionMs = target
        link.seek(to: Double(target) / 1000)
    }

    func setVolume(_ volume: Float) {
        self.volume = volume
        link.setVolume(volume)
    }

    func chooseAudio(_ trackId: String) {
        link.send(CastMessages.shared.setTracks(audioTrackId: trackId, subtitleTrackId: nil))
    }

    func chooseSubtitle(_ trackId: String) {
        link.send(CastMessages.shared.setTracks(audioTrackId: nil, subtitleTrackId: trackId))
    }

    /// Switches the dialogue boost on the TV, kept on this device for later plays, as the player does.
    func boostDialogue(_ on: Bool) {
        Task { [core] in try? await core.setDialogueBoost(on: on) }
        link.send(CastMessages.shared.setDialogueBoost(enabled: on))
    }

    func chooseQuality(_ option: QualityOption) {
        qualityChoice = option.quality
        let request = option.quality.request(bandwidthBps: nil)
        link.send(CastMessages.shared.setQuality(localQuality: request, remoteQuality: request))
    }

    func chooseVersion(_ versionId: String) {
        link.send(CastMessages.shared.setVersion(versionId: versionId))
    }

    func setSpeed(_ speed: Float) {
        link.setRate(speed)
    }

    func playNextNow() {
        link.send(CastMessages.shared.playNext())
    }

    func hideUpNext() {
        link.send(CastMessages.shared.hideUpNext())
    }

    /// Try again: the work cast again, with a fresh token, from where the TV was. A press
    /// means someone is watching, so up next's run starts again.
    func retry() {
        session.playback.confirmStillWatching()
        if let workId = media?.workId ?? status?.workId {
            cast(current(workId: workId), positionMs: positionMs > 0 ? positionMs : lastCast?.positionMs)
        } else if let lastCast {
            cast(lastCast.target, positionMs: lastCast.positionMs)
        }
    }

    /// Stop casting: the TV stops, saving the position, and what played
    /// there is returned to carry on with here, at the TV's position.
    func stopCasting() -> (target: PlayTarget, positionMs: Int64)? {
        let workId = media?.workId ?? status?.workId
        let position = positionMs
        let target = workId.map(current)
        menuShown = false
        link.end(stopCasting: true)
        disconnected()
        return target.map { ($0, position) }
    }

    /// Disconnect: this device lets go, and the TV plays on.
    func disconnect() {
        menuShown = false
        link.end(stopCasting: false)
        disconnected()
    }

    /// The app signed out: the TV plays on, and this model stops following it.
    func close() {
        ticking?.cancel()
        reading?.cancel()
        readingNext?.cancel()
        link.close()
    }

    /// [workId] as the TV plays it: its version and tracks.
    private func current(workId: String) -> PlayTarget {
        guard let status, status.workId == workId else { return PlayTarget(workId: workId) }
        return PlayTarget(workId: workId, versionId: status.versionId, audioTrackId: status.audioTrackId, subtitleTrackId: status.subtitleTrackId)
    }

    // MARK: Choices

    var version: Version? {
        guard let versionId = status?.versionId else { return nil }
        return work?.versions.first { $0.id == versionId }
    }

    var offline: Bool { false }

    var audioTrackId: String? { status?.audioTrackId }

    var dialogueBoost: DialogueBoost? { status?.dialogueBoost }

    var subtitle: String { status?.subtitleTrackId ?? Subtitles.shared.OFF }

    var quality: Quality? {
        qualityChoice ?? status?.quality.map { Quality.companion.of(request: $0) }
    }

    var speed: Float { media?.rate ?? 1 }

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
        SubtitleSearch.shared.offered(version: version, offline: false)
    }

    func searchSubtitles() async throws -> [SubtitleResult] {
        guard let version else { return [] }
        return try await session.playback.subtitleResults(version: version)
    }

    /// The version's tracks gain the subtitle at once, and the TV plays on with it.
    func downloadSubtitle(_ result: SubtitleResult) async throws {
        guard let version else { return }
        let track = try await session.playback.addSubtitle(version: version, resultId: result.id)
        work = work?.withTrack(versionId: version.id, track: track)
        chooseSubtitle(track.id)
    }

    // MARK: Scrubbing

    var scrubDurationMs: Int64 { durationMs }

    var scrubPositionMs: Int64 { positionMs }

    var chapters: [Chapter] { version?.chapters ?? [] }

    /// The play's previews stay on the TV, whose play's ID mustn't leave it.
    var previews: ScrubPreviews? { nil }

    // MARK: State

    private func disconnected() {
        connection = .disconnected
        media = nil
        status = nil
        loading = nil
        failure = nil
        sheet = nil
        changed()
    }

    /// Reads the work on the TV, for its title and artwork, and its versions and tracks for the sheets.
    private func read(workId: String) {
        guard work?.id != workId || reading == nil else { return }
        reading?.cancel()
        work = nil
        next = nil
        reading = Task { [weak self, session] in
            for await load in session.library.work(id: workId) {
                guard let self, !Task.isCancelled else { return }
                guard let work = load.getOrNull() else { continue }
                self.work = work
            }
        }
        if qualities.isEmpty {
            Task { [weak self, session] in self?.qualities = (try? await session.playback.qualityMenu()) ?? [] }
        }
    }

    private func readNext(_ nextId: String?) {
        guard next?.id != nextId else { return }
        readingNext?.cancel()
        next = nil
        guard let nextId else { return }
        readingNext = Task { [weak self, session] in
            for await load in session.library.work(id: nextId) {
                guard let next = load.getOrNull() else { continue }
                self?.next = next
                break
            }
        }
    }

    private func statusChanged(_ status: CastStatus) {
        self.status = status
        if let workId = status.workId { read(workId: workId) }
        readNext(status.nextWorkId)
        if let error = status.error, !status.unreachable {
            loading = nil
            failure = Self.failure(error, message: status.message)
        }
        changed()
    }

    /// Keeps the position moving while the TV plays, and tells the app when nothing plays any more.
    private func changed() {
        ticking?.cancel()
        if media?.state == .playing {
            ticking = Task { [weak self] in
                while !Task.isCancelled {
                    try? await Task.sleep(for: .milliseconds(500))
                    guard let self, !Task.isCancelled else { return }
                    if !self.scrubbing { self.positionMs = Int64(self.link.positionSeconds * 1000) }
                }
            }
        }
        if !active { stopped() }
    }
}

extension CastModel {
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

extension CastModel: CastLinkDelegate {
    func linkChanged(_ state: CastLinkState, device: String?) {
        let was = connection
        switch state {
        case .unavailable:
            available = false
            if was != .disconnected { disconnected() }
        case .disconnected:
            available = true
            if was != .disconnected { disconnected() }
        case .connecting:
            available = true
            connection = .connecting
        case let .connected(resumed):
            available = true
            connection = .connected
            if let device { self.device = device }
            if resumed, let name = self.device {
                // The receiver plays on with a fresh token after this app was away.
                Task { [session, link] in
                    if let message = try? await session.cast.tokenMessage(receiverName: name) { link.send(message) }
                }
            }
            link.send(CastMessages.shared.getStatus())
            if was != .connected || resumed { connected(resumed) }
        }
    }

    func volumeChanged(_ volume: Float) {
        self.volume = volume
    }

    func mediaChanged(_ media: CastMedia?) {
        self.media = media
        if let media {
            if !scrubbing { positionMs = media.positionMs }
            if media.workId == loading { loading = nil }
            failure = nil
            read(workId: media.workId)
            if let json = media.status, let status = CastStatus.companion.parseOrNull(json: json) { statusChanged(status) }
        }
        changed()
    }

    func received(_ message: String) {
        guard let status = CastStatus.companion.parseOrNull(json: message) else { return }
        statusChanged(status)
    }
}
