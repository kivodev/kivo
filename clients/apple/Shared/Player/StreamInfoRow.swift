import Foundation
import KivoCore

/// A line of Stream info: the play's method, each track's method with
/// what it was and became, and why for every line that isn't direct; the
/// subtitle showing, the delivery and the bitrate. Words come from the
/// shared display tables.
struct StreamInfoRow: Identifiable {
    let label: String
    let value: String
    var why: String?
    /// A line under the row that isn't a reason: how a download holds its subtitle.
    var note: String?
    var id: String { label }

    /// Stream info's rows for [play], of [version], with the track [subtitle] showing.
    static func rows(_ play: StreamInfo, display: Display, version: Version?, subtitle: String, offline: Bool, quality: Quality?) -> [StreamInfoRow] {
        let words = StreamInfoWords(display: display, offline: offline)
        let source = { (id: String) in version?.tracks.first { $0.id == id } }
        let selected = play.tracks.filter { $0.selected?.boolValue == true }
        let main = selected.first { $0.type == .video } ?? selected.first { $0.type == .audio }
        var rows = [
            StreamInfoRow(label: String(localized: "Playback"), value: main.map { display.method(track: $0) } ?? display.method(method: "directPlay")),
            StreamInfoRow(label: String(localized: "Source"), value: words.source(play)),
        ]

        if play.delivery == .stream, let container = version?.container {
            let name = display.container(container: container)
            let repackaged = selected.contains { $0.reason == .containerUnsupported }
            rows.append(StreamInfoRow(
                label: String(localized: "Container"),
                value: String(localized: "\(name) → HLS stream"),
                why: repackaged ? String(localized: "This device can't play \(name) files.") : nil
            ))
        }

        for track in selected where track.type != .subtitle {
            let from = source(track.trackId)?.fullName
            let output = track.transcode.map { transcode in
                [display.delivered(track: track), track.type == .video ? transcode.encoder : nil].compactMap { $0 }.joined(separator: " · ")
            }
            // A file's track with what it became is a download's conversion, which is what the file holds.
            let what = track.delivery == .file && output != nil
                ? output ?? ""
                : [from, output].compactMap { $0 }.joined(separator: " → ")
            let method = display.method(track: track)
            rows.append(StreamInfoRow(
                label: track.type == .video ? String(localized: "Video") : String(localized: "Audio"),
                value: what.isEmpty ? method : "\(method) · \(what)",
                why: track.delivery == .file ? nil : words.why(track)
            ))
        }

        rows.append(words.subtitleRow(play: play, subtitle: subtitle, track: source(subtitle)))

        if !offline, let quality, let cap = Int64(quality.key) {
            let limited = selected.contains { $0.reason == .clientQualityLimit }
            let source = version?.bitrateBps?.int64Value
            rows.append(StreamInfoRow(
                label: String(localized: "Quality"),
                value: !limited && source.map { $0 <= cap } == true
                    ? String(localized: "Original: the file (\(Display.app.bitrate(bps: source ?? 0))) is under your \(Display.app.bitrate(bps: cap)) limit")
                    : String(localized: "Limited to \(Display.app.bitrate(bps: cap))")
            ))
        }
        rows.append(StreamInfoRow(
            label: String(localized: "Delivery"),
            value: [
                play.delivery.map { $0 == .stream ? "HLS" : String(localized: "File") },
                play.local ? String(localized: "Local") : String(localized: "Remote"),
            ].compactMap { $0 }.joined(separator: " · ")
        ))
        if let client = play.client {
            rows.append(StreamInfoRow(label: String(localized: "Client"), value: client))
        }
        if let bitrate = play.bitrateBps {
            rows.append(StreamInfoRow(label: String(localized: "Bitrate"), value: Display.app.bitrate(bps: bitrate)))
        }
        return rows
    }

    /// The capability a file failed: "maxLevel 5.1; this device: 4.1".
    static func words(_ mismatch: CapabilityMismatch) -> String {
        guard let declared = mismatch.declared else { return "\(mismatch.field) \(mismatch.actual)" }
        return String(localized: "\(mismatch.field) \(mismatch.actual); this device: \(declared)")
    }
}

/// Stream info's words for one play.
private struct StreamInfoWords {
    let display: Display
    let offline: Bool

    /// "Server" when streaming; a download's "Downloaded", or "Downloaded (converted from HEVC 1080p)".
    func source(_ play: StreamInfo) -> String {
        guard offline else { return String(localized: "Server") }
        guard let from = play.downloadSource?.convertedFrom else { return String(localized: "Downloaded") }
        return String(localized: "Downloaded (converted from \(from))")
    }

    /// The subtitle showing, the track [subtitle], as the play delivers it.
    func subtitleRow(play: StreamInfo, subtitle: String, track: Track?) -> StreamInfoRow {
        let label = String(localized: "Subtitles")
        guard subtitle != SUBTITLES_OFF, let shown = play.tracks.first(where: { $0.type == .subtitle && $0.trackId == subtitle }) else {
            return StreamInfoRow(label: label, value: String(localized: "Off"))
        }
        let styled = ["ass", "ssa"]
        // A styled track shown plain doesn't name its codec: "Plain (ASS)" would read as styled.
        let plainStyled = track.map { styled.contains($0.codec) } == true && shown.delivery != .burn && !styled.contains(shown.codec ?? "")
        var kind = display.subtitleKind(track: shown)
        if let track, !plainStyled { kind += " (\(display.codec(codec: track.codec)))" }
        let source = track?.subtitle.flatMap { display.subtitleSource(source: $0.source) }
        let value = [track?.name, kind, source].compactMap { $0 }.joined(separator: " · ")
        if offline {
            // A download's subtitle is as the server prepared it; the streaming reason doesn't apply.
            return StreamInfoRow(label: label, value: value, note: downloadedNote(shown.delivery))
        }
        return StreamInfoRow(label: label, value: value, why: why(shown))
    }

    /// How a download holds its subtitle: burned into the conversion, a text track of its file, or a file beside it.
    private func downloadedNote(_ delivery: TrackDelivery?) -> String {
        switch delivery {
        case .burn: String(localized: "Burned in at download")
        case .file: String(localized: "A text track in the downloaded file")
        default: String(localized: "A subtitle file downloaded with it")
        }
    }

    /// Why a track isn't as the file has it, with the capability it failed; repackaging is the Container line's.
    func why(_ track: PlayTrack) -> String? {
        guard let reason = track.reason, reason != .containerUnsupported else { return nil }
        let why = display.reason(code: reason.value)
        guard let mismatch = track.mismatch else { return why }
        return "\(why) (\(StreamInfoRow.words(mismatch)))"
    }
}
