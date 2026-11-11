import KivoCore
import KivoUI
import SwiftUI

/// A detail page's Download button, round beside My List and
/// Watched. A tap downloads at the download quality setting; a video's
/// long press lists every quality with its size, "1080p · 8 Mb/s" over
/// "~3.6 GB", exact where the original goes as it is, the setting's
/// checked; a book downloads its original. Once asked for, it shows where
/// the download is: a dashed ring while the server is asked, and taps wait
/// for its answer, then a ring while it comes, a check when it's ready, a
/// warning when it failed, and its menu says so, with Pause or Resume
/// while it comes, Retry when it failed, and Remove Download.
struct DownloadButton: View {
    let work: WorkDetail
    var versionId: String?
    var audioTrackId: String?
    var subtitleTrackId: String?
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette
    @State private var estimate: DownloadEstimate?
    @State private var problem: String?

    var body: some View {
        Group {
            if let item = server.download(of: work.id) {
                Menu {
                    Text(DownloadWords.line(item, display: server.display))
                    if item.status is DownloadStatusFailed {
                        Button {
                            act { try await server.session.downloads?.retry(id: item.id) }
                        } label: {
                            Label(String(localized: "Retry"), systemImage: "arrow.clockwise")
                        }
                    } else if item.status is DownloadStatusPaused {
                        Button {
                            act { try await server.session.downloads?.resume(id: item.id) }
                        } label: {
                            Label(String(localized: "Resume download"), systemImage: "arrow.down.circle")
                        }
                    } else if item.status is DownloadStatusDownloading || item.status is DownloadStatusWaiting {
                        Button {
                            act { try await server.session.downloads?.pause(id: item.id) }
                        } label: {
                            Label(String(localized: "Pause download"), systemImage: "pause.circle")
                        }
                    }
                    Button(role: .destructive) {
                        act { try await server.session.downloads?.delete(id: item.id) }
                    } label: {
                        Label(String(localized: "Remove download"), systemImage: "trash")
                    }
                } label: {
                    DownloadGlyph(item: item)
                }
            } else if server.asking.contains(work.id) {
                Button {} label: {
                    DownloadGlyph(item: nil, asking: true)
                }
            } else if let estimate, !estimate.playable {
                Button {
                    problem = estimate.reason.map { server.display.reason(code: $0) } ?? String(localized: "This can't be downloaded to this device.")
                } label: {
                    DownloadGlyph(item: nil)
                }
            } else if work.book != nil {
                Button {
                    download(nil)
                } label: {
                    DownloadGlyph(item: nil)
                }
            } else {
                Menu {
                    Section(String(localized: "Download")) {
                        ForEach(estimate?.options ?? [], id: \.quality) { option in
                            optionToggle(option)
                        }
                        if estimate == nil {
                            Text("Loading…")
                        }
                    }
                } label: {
                    DownloadGlyph(item: nil)
                } primaryAction: {
                    download(nil)
                }
            }
        }
        .buttonStyle(.bordered)
        .buttonBorderShape(.circle)
        .tint(palette.text)
        .accessibilityIdentifier("download")
        .task(id: [work.id, versionId, audioTrackId, subtitleTrackId].map { $0 ?? "" }) {
            // Without an estimate the menu goes on working out the sizes.
            estimate = try? await server.session.downloads?.estimates(
                workIds: [work.id], versionId: versionId, audioTrackId: audioTrackId, subtitleTrackId: subtitleTrackId
            ).first
        }
        .alert(problem ?? "", isPresented: Binding(get: { problem != nil }, set: { if !$0 { problem = nil } })) {
            Button(String(localized: "OK"), role: .cancel) {}
        }
    }

    /// A toggle, for the setting's checkmark; either way it downloads.
    private func optionToggle(_ option: DownloadOption) -> some View {
        let quality: DownloadQuality? = DownloadQuality.allCases.first { $0.value == option.quality }
        let isSetting: Bool = option.quality == server.app.settings?.downloadQuality.value
        let name: String = DownloadQuality.companion.parseOrNull(value: option.quality).map { server.display.downloadQuality(quality: $0) } ?? option.quality
        return Toggle(name, isOn: Binding(get: { isSetting }, set: { _ in download(quality) }))
        .accessibilityIdentifier("download-\(option.quality)")
    }

    /// Downloads at [quality], or the setting's when nil.
    private func download(_ quality: DownloadQuality?) {
        act {
            _ = try await server.session.downloads?.download(
                workId: work.id, versionId: versionId, audioTrackId: work.book == nil ? audioTrackId : nil,
                subtitleTrackId: work.book == nil ? subtitleTrackId : nil, quality: quality
            )
        }
    }

    private func act(_ action: @escaping () async throws -> Void) {
        Haptics.success()
        Task {
            do {
                try await action()
            } catch {
                problem = error.downloadSentence
            }
        }
    }
}

/// The button's glyph: an arrow to download, a dashed ring while it's
/// asked for, a ring filling with how much
/// of the whole job is done (dashed while the server has it, an arrow in
/// it while paused), a check when it's ready, a warning when it failed.
private struct DownloadGlyph: View {
    let item: DownloadItem?
    var asking = false
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        Group {
            switch item.map({ onEnum(of: $0.status) }) {
            case nil where asking:
                DownloadRing(value: 0, dashed: true)
                    .padding(2)
                    .accessibilityLabel(Text("Waiting on the server"))
            case nil:
                Image(systemName: "arrow.down.circle")
                    .accessibilityLabel(Text("Download"))
            case .ready:
                Image(systemName: "arrow.down.circle.fill")
                    .accessibilityLabel(Text("Downloaded"))
            case .failed:
                Image(systemName: "exclamationmark.circle")
                    .foregroundStyle(palette.danger)
                    .accessibilityLabel(Text("Download failed"))
            case .downloading:
                DownloadRing(value: done, center: "stop.fill")
                    .padding(2)
                    .accessibilityLabel(Text("Downloading"))
            case .waiting:
                DownloadRing(value: done, center: "stop.fill")
                    .padding(2)
                    .accessibilityLabel(Text("Waiting to download"))
            case .paused:
                DownloadRing(value: done, center: "arrow.down")
                    .padding(2)
                    .accessibilityLabel(Text("Download paused"))
            case .converting:
                DownloadRing(value: done, dashed: true)
                    .padding(2)
                    .accessibilityLabel(Text("Preparing on the server"))
            case .queued:
                DownloadRing(value: 0, dashed: true)
                    .padding(2)
                    .accessibilityLabel(Text("Waiting on the server"))
            }
        }
        .roundButtonSymbol()
        .contentTransition(.symbolEffect(.replace))
    }

    /// The ring's fill: the same share of the whole job as the badge's percent.
    private var done: Double { Double(item.flatMap(DownloadWords.done) ?? 0) / 100 }
}

/// A download's progress as a ring filling from the top, dashed while the
/// server has it, with a small symbol in its centre.
struct DownloadRing: View {
    let value: Double
    var dashed = false
    var center: String?
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        ZStack {
            Circle()
                .stroke(palette.textMuted.opacity(0.4), style: StrokeStyle(lineWidth: 2.5, dash: dashed ? [3, 3] : []))
            Circle()
                .trim(from: 0, to: max(0.02, value))
                .stroke(palette.accent, style: StrokeStyle(lineWidth: 2.5, lineCap: .round))
                .rotationEffect(.degrees(-90))
            if let center {
                Image(systemName: center).font(.system(size: 8, weight: .bold))
            }
        }
    }
}
