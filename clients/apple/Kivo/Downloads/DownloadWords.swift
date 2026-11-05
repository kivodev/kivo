import Foundation
import KivoCore
import KivoUI

/// A download's state in words; its sizes, quality and failure are the
/// core's Display's.
@MainActor
enum DownloadWords {
    /// A row's line under its title. While it's under way, its state and
    /// how much of the whole job is done: "Queued", "Preparing · 26%",
    /// "Downloading · 70%", "Waiting for Wi-Fi · 31%", "Offline · 8%",
    /// "Paused · 40%". Once it's here, how long it plays and its size,
    /// "47m · 312 MB", a book's authors in its runtime's place. A failed
    /// one says only why.
    static func line(_ item: DownloadItem, display: Display) -> String {
        if let progress = progress(item) { return progress.words }
        if let failed = item.status as? DownloadStatusFailed { return display.downloadFailure(reason: failed.reason) }
        let what = item.kind == "book"
            ? item.authors
            : item.durationMs.map(\.int64Value).flatMap { $0 > 0 ? display.runtime(ms: $0) : nil }
        return [what, display.size(bytes: item.sizeBytes)].compactMap { $0 }.joined(separator: " · ")
    }

    /// A row's title: an episode's number leads it on its show's page
    /// ([numbered]), "2. Trouble in Lumpy Space".
    static func title(_ item: DownloadItem, numbered: Bool) -> String {
        guard numbered, let episode = item.episode?.intValue else { return item.title }
        return "\(episode). \(item.title)"
    }

    /// A show's line under its name: "6 episodes · 2.3 GB" once every
    /// episode that hasn't failed is here, else one state for the whole
    /// show and how much of it is done, "Downloading · 47%". When every one
    /// failed, why the first did.
    static func line(show episodes: [DownloadItem], display: Display) -> String {
        if let progress = progress(show: episodes) { return progress.words }
        let ready = episodes.filter { $0.status is DownloadStatusReady }
        if ready.isEmpty, let failed = episodes.lazy.compactMap({ $0.status as? DownloadStatusFailed }).first {
            return display.downloadFailure(reason: failed.reason)
        }
        let size = display.size(bytes: ready.reduce(Int64(0)) { $0 + $1.sizeBytes })
        return ready.count == 1 ? String(localized: "1 episode · \(size)") : String(localized: "\(ready.count) episodes · \(size)")
    }

    /// Where a download under way is, for its line and its ring; nil once
    /// it's here or failed.
    static func progress(_ item: DownloadItem) -> DownloadProgress? {
        let done = done(item)
        return switch onEnum(of: item.status) {
        case .queued: DownloadProgress(stage: .queued, percent: nil)
        case .converting: DownloadProgress(stage: .preparing, percent: done)
        case .downloading: DownloadProgress(stage: .downloading, percent: done)
        case let .waiting(w): DownloadProgress(stage: w.reason == .turn ? .queued : .waiting(w.reason), percent: done)
        case .paused: DownloadProgress(stage: .paused, percent: done)
        case .ready, .failed: nil
        }
    }

    /// Where a show's episodes are together, each that hasn't failed
    /// counting the same: Downloading while any is prepared or comes, else
    /// what holds them, a wait, a pause or the line; nil once every one of
    /// those is here.
    static func progress(show episodes: [DownloadItem]) -> DownloadProgress? {
        let counted = episodes.filter { !($0.status is DownloadStatusFailed) }
        let stages = counted.compactMap { progress($0)?.stage }
        let stage: DownloadProgress.Stage? = stages.contains { $0 == .preparing || $0 == .downloading }
            ? .downloading
            : stages.first { if case .waiting = $0 { true } else { false } }
                ?? [DownloadProgress.Stage.paused, .queued].first { stages.contains($0) }
        guard let stage else { return nil }
        let done = counted.reduce(0) { $0 + (self.done($1) ?? 0) } / counted.count
        return DownloadProgress(stage: stage, percent: done)
    }

    /// How much of a download's whole job is done, from 0 to 100, rounded
    /// down so it's 100 only once it's here. One the server converts is
    /// half done once converted and the rest as it comes; an original only
    /// comes. Nil while nothing has started, while the server prepares
    /// only an original's subtitle, and once it failed.
    static func done(_ item: DownloadItem) -> Int? {
        let share: Double
        switch onEnum(of: item.status) {
        case .ready: return 100
        case .queued, .failed: return nil
        case let .converting(c):
            guard item.prepared else { return nil }
            share = c.progress / 2
        case let .downloading(d): share = came(item, d.receivedBytes, of: d.totalBytes)
        case let .waiting(w): share = came(item, w.receivedBytes, of: w.totalBytes)
        case let .paused(p): share = came(item, p.receivedBytes, of: p.totalBytes)
        }
        return min(Int((min(max(share, 0), 1) * 100 + 1e-9).rounded(.down)), 99)
    }

    /// How much of [item]'s job is done with [received] of [total] here.
    private static func came(_ item: DownloadItem, _ received: Int64, of total: Int64) -> Double {
        let here = total > 0 ? Double(received) / Double(total) : 0
        return item.prepared ? 0.5 + here / 2 : here
    }

    /// The state in short, as a detail page shows it under its actions:
    /// "Downloaded" once it's here, "Download failed", else the line's
    /// words, "Downloading · 42%".
    static func badge(_ item: DownloadItem) -> String {
        if let progress = progress(item) { return progress.words }
        if item.status is DownloadStatusFailed { return String(localized: "Download failed") }
        return String(localized: "Downloaded")
    }
}

/// Where a download under way is, as its line says it and its ring draws
/// it: a stage, and how much of the whole job is done once there's
/// something to measure. The ring is the accent while the server prepares
/// it or it comes here, turning while the server prepares with no percent
/// yet, and grey while it's in line, waits or is paused.
struct DownloadProgress: Equatable {
    enum Stage: Equatable {
        case queued, preparing, downloading, waiting(WaitReason), paused
    }

    let stage: Stage
    /// From 0 to 100; nil while there's nothing to measure, as in line
    /// with nothing done.
    let percent: Int?

    init(stage: Stage, percent: Int?) {
        self.stage = stage
        self.percent = stage == .queued && percent == 0 ? nil : percent
    }

    /// The ring's fill, from 0 to 1: the same as [percent].
    var fraction: Double? { percent.map { Double($0) / 100 } }

    /// The accent while it's prepared or comes; grey otherwise.
    var active: Bool { stage == .preparing || stage == .downloading }

    /// "Downloading · 52%", or "Queued" with no percent.
    var words: String {
        let word = switch stage {
        case .queued: String(localized: "Queued")
        case .preparing: String(localized: "Preparing")
        case .downloading: String(localized: "Downloading")
        case .waiting(.wifi): String(localized: "Waiting for Wi-Fi")
        case .waiting(.server): String(localized: "Waiting for server")
        case .waiting: String(localized: "Offline")
        case .paused: String(localized: "Paused")
        }
        return percent.map { "\(word) · \($0)%" } ?? word
    }
}
