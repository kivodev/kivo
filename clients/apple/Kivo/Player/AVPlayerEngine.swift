import AVFoundation
import KivoCore

/// The core's PlayerEngine on AVPlayer. An HLS play is one item on the
/// version's timeline; direct play is each part's file in turn, positions
/// offset by the parts before it through the core's PartTimeline. The
/// subtitle and audio track the controller says are selected in AVPlayer:
/// a rendition of the stream, or a track of the file by its stream index.
/// A subtitle file is the overlay's to draw (SubtitleOverlay). A download's
/// URLs are its files here. It reports on the main thread, the controller's.
final class AVPlayerEngine: NSObject, PlayerEngine {
    let player = AVPlayer()
    var subtitleStyle: SubtitleStyle {
        didSet { player.currentItem?.textStyleRules = textStyleRules }
    }
    /// The title Now Playing shows, on every item the engine loads.
    var metadata: [AVMetadataItem] = [] {
        didSet { player.currentItem?.externalMetadata = metadata }
    }
    /// How a speed other than 1 keeps the pitch, on every item the engine loads; the book player's is for speech at up to 3×.
    var pitchAlgorithm: AVAudioTimePitchAlgorithm = .timeDomain

    /// Media URLs are relative to the API root and carry the play, so they need no token; a download's are absolute `file:` URLs.
    /// The root is at the server's address in use as each item loads.
    private var mediaBase: URL { URL(string: baseUrl() + "/api/v1/") ?? URL(fileURLWithPath: "/") }
    private let baseUrl: () -> String
    private var events: (any PlayerEngineEvents)?
    private var hls = false
    private var parts: [URL] = []
    private var timeline: PartTimeline?
    private var part = 0
    /// Where the part loading now starts, until it's ready.
    private var pendingOffsetMs: Int64 = 0
    private var loadingPart = false
    private var ready = false
    private var failed = false
    /// How the play's subtitle shows, as the controller last said; nil for none.
    private var subtitle: (any TrackShow)?
    /// The stream index of the file's audio track to play; nil for the file's default.
    private var audioIndex: Int?
    private var itemObservation: NSKeyValueObservation?
    private var statusObservation: NSKeyValueObservation?
    private var notifications: [NSObjectProtocol] = []
    private var timeObserver: Any?

    init(baseUrl: @escaping () -> String, subtitleStyle: SubtitleStyle) {
        self.baseUrl = baseUrl
        self.subtitleStyle = subtitleStyle
        super.init()
        // Kivo picks the subtitle: the system's caption preferences don't add one.
        player.appliesMediaSelectionCriteriaAutomatically = false
        player.allowsExternalPlayback = true
        statusObservation = player.observe(\.timeControlStatus, options: [.new]) { [weak self] player, _ in
            let status = player.timeControlStatus
            DispatchQueue.main.async { self?.timeControlChanged(status) }
        }
        timeObserver = player.addPeriodicTimeObserver(forInterval: CMTime(value: 250, timescale: 1000), queue: .main) { [weak self] _ in
            guard let self, self.ready, !self.loadingPart else { return }
            self.events?.onTime()
        }
    }

    deinit {
        if let timeObserver { player.removeTimeObserver(timeObserver) }
        notifications.forEach(NotificationCenter.default.removeObserver)
    }

    // MARK: PlayerEngine

    var positionMs: Int64 {
        let seconds = player.currentTime().seconds
        let offset = loadingPart ? pendingOffsetMs : (seconds.isFinite ? Int64(seconds * 1000) : 0)
        guard !hls, let timeline else { return offset }
        return timeline.position(part: Int32(part), offsetMs: offset)
    }

    var paused: Bool { player.rate == 0 }

    var bandwidthBps: KotlinLong? {
        guard let observed = player.currentItem?.accessLog()?.events.last?.observedBitrate, observed > 0 else { return nil }
        return KotlinLong(longLong: Int64(observed))
    }

    /// The play's first item replaces the one loaded, never leaving the
    /// player empty between them: Picture in Picture closes on an empty player.
    func load(play: PlayStarted, events: any PlayerEngineEvents) {
        reset()
        self.events = events
        hls = play.delivery == .stream
        let start = Int64(play.startMs)
        if hls {
            parts = [mediaURL(play.url)]
            open(part: 0, offsetMs: start, play: false)
        } else {
            let timeline = PartTimeline(durationsMs: play.parts.map { KotlinLong(longLong: Int64($0.durationMs)) })
            self.timeline = timeline
            parts = play.parts.map { mediaURL($0.url ?? play.url) }
            let at = timeline.locate(positionMs: start)
            open(part: Int(at.part), offsetMs: at.offsetMs, play: false)
        }
    }

    func unload() {
        reset()
        player.pause()
        player.replaceCurrentItem(with: nil)
    }

    private func reset() {
        events = nil
        clearItem()
        parts = []
        timeline = nil
        part = 0
        subtitle = nil
        audioIndex = nil
        ready = false
        failed = false
        loadingPart = false
    }

    func play() {
        player.play()
    }

    func pause() {
        player.pause()
    }

    func seek(positionMs: Int64) {
        guard let timeline, !hls else {
            seekInPart(to: positionMs)
            return
        }
        let at = timeline.locate(positionMs: positionMs)
        if Int(at.part) == part, !loadingPart {
            seekInPart(to: at.offsetMs)
        } else {
            open(part: Int(at.part), offsetMs: at.offsetMs, play: player.rate > 0)
        }
    }

    /// Selected in the item playing once it's ready, and in each part's as it becomes ready.
    func showSubtitle(show: any TrackShow) {
        subtitle = show
        if let item = player.currentItem, item.status == .readyToPlay { selectSubtitle(in: item) }
    }

    func selectAudio(index: Int32) {
        audioIndex = Int(index)
        if let item = player.currentItem, item.status == .readyToPlay { selectAudio(in: item) }
    }

    // MARK: Parts

    private func open(part index: Int, offsetMs: Int64, play: Bool) {
        clearItem()
        part = index
        pendingOffsetMs = offsetMs
        loadingPart = true
        let item = AVPlayerItem(url: parts[index])
        item.textStyleRules = textStyleRules
        item.externalMetadata = metadata
        item.audioTimePitchAlgorithm = pitchAlgorithm
        itemObservation = item.observe(\.status, options: [.new]) { [weak self] item, _ in
            DispatchQueue.main.async { self?.statusChanged(item, play: play) }
        }
        let center = NotificationCenter.default
        notifications = [
            center.addObserver(forName: AVPlayerItem.didPlayToEndTimeNotification, object: item, queue: .main) { [weak self] _ in
                self?.partEnded()
            },
            center.addObserver(forName: AVPlayerItem.failedToPlayToEndTimeNotification, object: item, queue: .main) { [weak self] note in
                self?.fail(note.userInfo?[AVPlayerItemFailedToPlayToEndTimeErrorKey] as? Error)
            },
        ]
        player.replaceCurrentItem(with: item)
    }

    private func clearItem() {
        itemObservation = nil
        notifications.forEach(NotificationCenter.default.removeObserver)
        notifications = []
    }

    private func statusChanged(_ item: AVPlayerItem, play: Bool) {
        guard item === player.currentItem, loadingPart else { return }
        switch item.status {
        case .readyToPlay:
            selectSubtitle(in: item)
            selectAudio(in: item)
            let begin = { [weak self] in
                guard let self, item === self.player.currentItem else { return }
                self.loadingPart = false
                if !self.ready {
                    self.ready = true
                    self.events?.onReady()
                } else {
                    if play { self.player.play() }
                    self.events?.onSeeked()
                }
            }
            if pendingOffsetMs > 0 {
                player.seek(to: time(pendingOffsetMs), toleranceBefore: .zero, toleranceAfter: .zero) { _ in
                    DispatchQueue.main.async(execute: begin)
                }
            } else {
                begin()
            }
        case .failed:
            fail(item.error)
        default:
            break
        }
    }

    private func seekInPart(to ms: Int64) {
        player.seek(to: time(ms), toleranceBefore: .zero, toleranceAfter: .zero) { [weak self] finished in
            guard finished else { return }
            DispatchQueue.main.async { self?.events?.onSeeked() }
        }
    }

    private func partEnded() {
        if !hls, part < parts.count - 1 {
            open(part: part + 1, offsetMs: 0, play: true)
        } else {
            events?.onEnded()
        }
    }

    private func timeControlChanged(_ status: AVPlayer.TimeControlStatus) {
        guard ready, !loadingPart, let events else { return }
        switch status {
        case .playing:
            events.onPlay()
            events.onPlaying()
        case .waitingToPlayAtSpecifiedRate:
            guard player.reasonForWaitingToPlay != .noItemToPlay else { return }
            events.onPlay()
            events.onWaiting()
        case .paused:
            events.onPause()
        @unknown default:
            break
        }
    }

    // MARK: Failure

    /// Media failed: once per load, in the fallback request's codes.
    private func fail(_ error: Error?) {
        guard !failed, let events else { return }
        failed = true
        let ns = error as NSError?
        events.onFailure(failure: PlayerError(code: Self.failureCode(ns), detail: Self.detail(ns)))
    }

    private static func failureCode(_ error: NSError?) -> String {
        guard let error else { return "mediaSourceError" }
        if error.domain == NSURLErrorDomain { return "networkError" }
        if error.domain == AVFoundationErrorDomain, let code = AVError.Code(rawValue: error.code) {
            switch code {
            case .decoderNotFound, .decodeFailed, .decoderTemporarilyUnavailable: return "mediaDecodeError"
            case .fileFormatNotRecognized, .failedToParse: return "unsupported"
            default: break
            }
        }
        if let underlying = error.userInfo[NSUnderlyingErrorKey] as? NSError { return failureCode(underlying) }
        return "mediaSourceError"
    }

    /// The player's own words for the server log: each error's domain, code and description, outermost first.
    private static func detail(_ error: NSError?) -> String {
        var lines: [String] = []
        var next = error
        while let error = next {
            lines.append("\(error.domain) \(error.code): \(error.localizedDescription)")
            next = error.userInfo[NSUnderlyingErrorKey] as? NSError
        }
        return lines.isEmpty ? "AVPlayerItem failed without an error" : lines.joined(separator: " / ")
    }

    // MARK: Tracks

    /// Selects the subtitle as the controller last said: the stream's
    /// rendition at its position among the play's streamed subtitles, as the
    /// playlist lists them in the same order, since AVPlayer doesn't say a
    /// rendition's ID; a text track of the file by its stream index; or none.
    private func selectSubtitle(in item: AVPlayerItem) {
        Task { @MainActor [weak self] in
            guard let group = try? await item.asset.loadMediaSelectionGroup(for: .legible) else { return }
            var option: AVMediaSelectionOption?
            switch self?.subtitle.map({ onEnum(of: $0) }) {
            case let .stream(stream):
                let renditions = group.options.filter { $0.mediaType == .subtitle }
                option = renditions.indices.contains(Int(stream.position)) ? renditions[Int(stream.position)] : nil
            case let .file(file):
                // AVFoundation adds a forced-only option beside each of a file's subtitle tracks.
                let options = group.options.filter { !$0.hasMediaCharacteristic(.containsOnlyForcedSubtitles) }
                option = await Self.option(of: options, in: item.asset, index: Int(file.index), types: Self.legible)
            default:
                option = nil
            }
            guard let self, item === self.player.currentItem else { return }
            item.select(option, in: group)
        }
    }

    /// Plays the file's audio track the controller chose, by its stream
    /// index: that option of the file's audio alternatives, or where its
    /// audio tracks aren't alternatives, which AVPlayer would play together,
    /// that track alone.
    private func selectAudio(in item: AVPlayerItem) {
        Task { @MainActor [weak self] in
            guard let index = self?.audioIndex else { return }
            let group = try? await item.asset.loadMediaSelectionGroup(for: .audible)
            var option: AVMediaSelectionOption?
            if let group { option = await Self.option(of: group.options, in: item.asset, index: index, types: [.audio]) }
            let tracks = (try? await item.asset.load(.tracks)) ?? []
            guard let self, item === self.player.currentItem, self.audioIndex == index else { return }
            if let group, let option {
                item.select(option, in: group)
            } else if tracks.indices.contains(index) {
                let chosen = tracks[index].trackID
                for track in item.tracks where track.assetTrack?.mediaType == .audio {
                    track.isEnabled = track.assetTrack?.trackID == chosen
                }
            }
        }
    }

    private static let legible: Set<AVMediaType> = [.subtitle, .text, .closedCaption]

    /// The option of a group's [options] that plays the file's track at
    /// stream [index]: the asset lists the file's tracks in their order, and
    /// the options are its tracks of [types] in the same order.
    private static func option(of options: [AVMediaSelectionOption], in asset: AVAsset, index: Int, types: Set<AVMediaType>) async -> AVMediaSelectionOption? {
        guard let tracks = try? await asset.load(.tracks), tracks.indices.contains(index), types.contains(tracks[index].mediaType) else { return nil }
        let position = tracks[..<index].filter { types.contains($0.mediaType) }.count
        let options = options.filter { types.contains($0.mediaType) }
        return options.indices.contains(position) ? options[position] : nil
    }

    private var textStyleRules: [AVTextStyleRule] {
        var attributes: [String: Any] = [
            kCMTextMarkupAttribute_RelativeFontSize as String: subtitleStyle.size.relativeFontSize,
        ]
        switch subtitleStyle.background {
        case .box:
            attributes[kCMTextMarkupAttribute_CharacterBackgroundColorARGB as String] = [0.75, 0, 0, 0]
        case .shadow:
            attributes[kCMTextMarkupAttribute_CharacterEdgeStyle as String] = kCMTextMarkupCharacterEdgeStyle_DropShadow as String
        case .none:
            attributes[kCMTextMarkupAttribute_CharacterEdgeStyle as String] = kCMTextMarkupCharacterEdgeStyle_None as String
        }
        return AVTextStyleRule(textMarkupAttributes: attributes).map { [$0] } ?? []
    }

    /// A media URL the play names, relative to the API root, or a download's file.
    func mediaURL(_ relative: String) -> URL {
        URL(string: relative, relativeTo: mediaBase)?.absoluteURL ?? mediaBase
    }

    private func time(_ ms: Int64) -> CMTime {
        CMTime(value: ms, timescale: 1000)
    }
}

private extension SubtitleSize {
    /// Percent of AVPlayer's default size.
    var relativeFontSize: Int {
        switch self {
        case .small: 80
        case .medium: 100
        case .large: 130
        }
    }
}

/// How the stream's plain text subtitles look: this device's size and background.
struct SubtitleStyle: Equatable {
    var size: SubtitleSize
    var background: SubtitleBackground
}
