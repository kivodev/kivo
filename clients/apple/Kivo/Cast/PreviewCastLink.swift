#if DEBUG
import Foundation
import KivoCore

/// A Cast session without a Cast device, for the Simulator, which finds
/// none: `KIVO_CAST_PREVIEW` in the environment names the scene. `found`
/// has a device to connect to, `connecting` stays connecting, and
/// `connected` is connected with nothing playing; a cast then plays at once
/// (`playing`), stays buffering (`buffering`), can't reach the server
/// (`unreachable`), stops because the TV can't play it (`failed`), or opens
/// near its end with up next showing (`upNext`).
/// Casting asks the server for a real cast token; the stand-in receiver
/// plays the work's first version and answers Kivo's messages as the real
/// one does.
enum CastPreview {
    static var scene: String? { ProcessInfo.processInfo.environment["KIVO_CAST_PREVIEW"] }
}

final class PreviewCastLink: KivoCore.CastLink {
    private var events: (any CastLinkEvents)?
    private let scene: String
    private let session: KivoSession
    private let device = "Living Room TV"
    private var connected = false
    private var media: CastMedia?
    private var status: [String: Any] = [:]
    /// The version the stand-in receiver plays.
    private var version: Version?
    private var startedAt = Date()
    private var volume: Float = 0.45

    init(scene: String, session: KivoSession) {
        self.scene = scene
        self.session = session
    }

    func start(events: any CastLinkEvents) {
        self.events = events
        switch scene {
        case "found": events.onLink(state: CastLinkStateDisconnected.shared)
        case "connecting": events.onLink(state: CastLinkStateConnecting.shared)
        default: connect()
        }
    }

    private func connect() {
        connected = true
        events?.onLink(state: CastLinkStateConnected(device: device, resumed: false))
        events?.onVolume(volume: volume)
    }

    var positionMs: Int64 {
        guard let media else { return 0 }
        guard media.state == .playing else { return media.positionMs }
        return media.positionMs + Int64(Date().timeIntervalSince(startedAt) * 1000 * Double(media.speed))
    }

    func showDevices() {
        guard !connected else { return }
        events?.onLink(state: CastLinkStateConnecting.shared)
        Task { @MainActor [weak self] in
            try? await Task.sleep(for: .seconds(1.5))
            self?.connect()
        }
    }

    func load(load: CastLoad, title: String, imageUrl: String?) {
        let custom = (try? JSONSerialization.jsonObject(with: Data(load.customData.utf8))) as? [String: Any] ?? [:]
        Task { @MainActor [weak self, session] in
            let work = await session.library.work(id: load.contentId).answer()
            guard let self, let work else { return }
            self.play(work, custom: custom, from: load.currentTimeSec?.doubleValue)
        }
    }

    private func play(_ work: WorkDetail, custom: [String: Any], from seconds: Double?) {
        let version = work.versions.first { $0.id == custom["versionId"] as? String } ?? work.versions.first
        let durationMs: Int64 = version.map { Int64($0.durationMs) } ?? Int64(work.durationMs?.intValue ?? 0)
        let audio = custom["audioTrackId"] as? String ?? version?.audioTrackId
        self.version = version
        status = [
            "error": NSNull(),
            "workId": work.id,
            "versionId": version?.id ?? "",
            "quality": custom["localQuality"] ?? ["mode": "auto"] as [String: Any],
            "local": true,
        ]
        if let audio { status["audioTrackId"] = audio }
        if let subtitle = custom["subtitleTrackId"] as? String ?? version?.subtitleTrackId, subtitle != "off" { status["subtitleTrackId"] = subtitle }
        status["tracks"] = tracks()
        if let boost = custom["dialogueBoost"] as? Bool, boost { status["dialogueBoost"] = ["enabled": true] }
        if let nextId = work.episode?.nextId { status["nextWorkId"] = nextId }
        var positionMs = Int64((seconds ?? Double(work.userState?.positionMs?.int64Value ?? 0) / 1000) * 1000)
        var state = CastMedia.State.playing
        switch scene {
        case "buffering": state = .buffering
        case "unreachable": status["error"] = "serverUnreachable"
        case "failed":
            // The receiver stops its player and says why.
            status = ["error": "playbackFailed", "workId": work.id]
            broadcast()
            events?.onMedia(media: nil)
            return
        case "upNext":
            positionMs = max(durationMs - 42_000, 0)
            status["upNext"] = ["phase": "countingDown", "secondsLeft": 7]
        default: break
        }
        emit(CastMedia(workId: work.id, state: state, positionMs: positionMs, durationMs: durationMs, speed: 1, status: nil))
    }

    func play() { change(.playing) }

    func pause() { change(.paused) }

    func seek(positionMs: Int64) {
        guard let media else { return }
        emit(CastMedia(workId: media.workId, state: media.state, positionMs: positionMs, durationMs: media.durationMs, speed: media.speed, status: nil))
    }

    func setSpeed(speed: Float) {
        guard let media else { return }
        emit(CastMedia(workId: media.workId, state: media.state, positionMs: positionMs, durationMs: media.durationMs, speed: speed, status: nil))
    }

    func setVolume(volume: Float) {
        self.volume = volume
    }

    func send(message: String) {
        guard let body = (try? JSONSerialization.jsonObject(with: Data(message.utf8))) as? [String: Any] else { return }
        switch body["type"] as? String {
        case "kivo.setTracks":
            if let audio = body["audioTrackId"] { status["audioTrackId"] = audio }
            if let subtitle = body["subtitleTrackId"] as? String {
                status["subtitleTrackId"] = subtitle == "off" ? nil : subtitle
            }
            status["tracks"] = tracks()
        case "kivo.setDialogueBoost":
            status["dialogueBoost"] = (body["enabled"] as? Bool) == true ? ["enabled": true] : nil
        case "kivo.setVersion":
            status["versionId"] = body["versionId"]
        case "kivo.setQuality":
            status["quality"] = body["localQuality"]
        case "kivo.playNext", "kivo.hideUpNext":
            status["upNext"] = nil
        default:
            break
        }
        broadcast()
    }

    func end(stopCasting: Bool) {
        connected = false
        media = nil
        events?.onMedia(media: nil)
        events?.onLink(state: CastLinkStateDisconnected.shared)
    }

    func close() {
        events = nil
    }

    private func change(_ state: CastMedia.State) {
        guard let media else { return }
        emit(CastMedia(workId: media.workId, state: state, positionMs: positionMs, durationMs: media.durationMs, speed: media.speed, status: nil))
    }

    private func emit(_ media: CastMedia) {
        self.media = media
        startedAt = Date()
        events?.onMedia(media: media)
        broadcast()
    }

    /// The stand-in play's tracks as a receiver's play gives them: the
    /// video and audio direct, each subtitle in the stream, and off; the
    /// ones the status names selected.
    private func tracks() -> [[String: Any]] {
        let audio = status["audioTrackId"] as? String
        let subtitle = status["subtitleTrackId"] as? String ?? "off"
        var tracks: [[String: Any]] = (version?.tracks ?? []).compactMap { track in
            var entry: [String: Any] = ["trackId": track.id, "type": track.type.value]
            switch track.type {
            case .video:
                entry["delivery"] = "file"
                entry["selected"] = true
            case .audio:
                entry["delivery"] = "file"
                if track.id == audio { entry["selected"] = true }
            case .subtitle:
                entry["delivery"] = "stream"
                entry["codec"] = "webvtt"
                if track.id == subtitle { entry["selected"] = true }
            case .unknown:
                return nil
            }
            return entry
        }
        var off: [String: Any] = ["trackId": "off", "type": "subtitle", "delivery": "none"]
        if subtitle == "off" { off["selected"] = true }
        tracks.append(off)
        return tracks
    }

    private func broadcast() {
        var message = status
        message["type"] = "kivo.status"
        guard let data = try? JSONSerialization.data(withJSONObject: message) else { return }
        events?.onMessage(message: String(decoding: data, as: UTF8.self))
    }
}
#endif
