import Foundation
import KivoCore
import XCTest
@testable import Kivo

/// A Downloads row's line and its ring: while it's under way its state and
/// how much of the whole job is done, no size until it's here, then how
/// long it plays and its size, a book's authors in its runtime's place, and
/// only why when it failed. A show counts each of its episodes the same.
@MainActor
final class DownloadWordsTests: XCTestCase {
    private let display = Display()

    private func item(
        _ status: any DownloadStatus, kind: WorkKind = .movie, season: Int? = nil, episode: Int? = nil,
        authors: String? = nil, quality: DownloadQuality = .high, durationMs: Int64? = 2_820_000, size: Int64 = 1_200_000_000, exact: Bool = true,
        prepared: Bool = false
    ) -> DownloadItem {
        DownloadItem(
            id: "d1", workId: "w1", versionId: "v1", title: "Half Loop", kind: kind,
            seriesId: season == nil ? nil : "s1", seriesTitle: season == nil ? nil : "Severance",
            season: season.map { KotlinInt(int: Int32($0)) }, episode: episode.map { KotlinInt(int: Int32($0)) },
            authors: authors, quality: quality, durationMs: durationMs.map { KotlinLong(value: $0) }, sizeBytes: size, sizeExact: exact,
            prepared: prepared, status: status, artworkPath: nil, posterPath: nil, createdAtMs: 0
        )
    }

    private let ready = DownloadStatusReady.shared

    private func line(_ status: any DownloadStatus, prepared: Bool = false) -> String {
        DownloadWords.line(item(status, size: 420_000_000, exact: false, prepared: prepared), display: display)
    }

    private func episode(_ status: any DownloadStatus, prepared: Bool = true) -> DownloadItem {
        item(status, kind: .episode, season: 1, episode: 1, prepared: prepared)
    }

    private func preparing(_ progress: Double) -> any DownloadStatus {
        DownloadStatusConverting(progress: progress, remainingMs: nil)
    }

    private func downloading(_ percent: Int64) -> any DownloadStatus {
        DownloadStatusDownloading(receivedBytes: percent, totalBytes: 100, bytesPerSecond: 0)
    }

    func testAReadyDownloadSaysHowLongItPlaysAndItsSize() {
        XCTAssertEqual(DownloadWords.line(item(ready, kind: .episode, season: 1, episode: 2, size: 312_000_000), display: display), "47m · 312 MB")
        XCTAssertEqual(DownloadWords.line(item(ready, durationMs: 6_720_000, size: 1_800_000_000), display: display), "1h 52m · 1.8 GB")
        XCTAssertEqual(DownloadWords.line(item(ready, durationMs: nil), display: display), "1.2 GB")
        XCTAssertEqual(
            DownloadWords.line(item(ready, kind: .book, authors: "Frank Herbert", quality: .original, size: 320_000_000), display: display),
            "Frank Herbert · 320 MB"
        )
        XCTAssertNil(DownloadWords.progress(item(ready)))
    }

    func testNoRowSaysAResolution() {
        let rows = [DownloadQuality.high, .standard, .original].flatMap { quality in
            [item(ready, quality: quality), item(ready, kind: .episode, season: 1, episode: 1, quality: quality)]
        }
        for row in rows {
            let line = DownloadWords.line(row, display: display)
            for word in ["4K", "1080p", "720p", "SD", "Original"] {
                XCTAssertFalse(line.contains(word), "\(line) names a resolution")
            }
        }
        XCTAssertEqual(DownloadWords.line(show: rows.filter { $0.kind == .episode }, display: display), "3 episodes · 3.6 GB")
    }

    func testAnEpisodesNumberLeadsItsTitleOnItsShowsPage() {
        let episode = item(ready, kind: .episode, season: 1, episode: 2)
        XCTAssertEqual(DownloadWords.title(episode, numbered: true), "2. Half Loop")
        XCTAssertEqual(DownloadWords.title(episode, numbered: false), "Half Loop")
        XCTAssertEqual(DownloadWords.title(item(ready), numbered: true), "Half Loop")
    }

    func testOneUnderWaySaysItsStateAndPercentAndNoSize() {
        XCTAssertEqual(line(DownloadStatusQueued.shared), "Queued")
        XCTAssertEqual(line(preparing(0)), "Preparing")
        XCTAssertEqual(line(downloading(52)), "Downloading · 52%")
        XCTAssertEqual(line(downloading(100)), "Downloading · 99%")
        XCTAssertEqual(line(DownloadStatusWaiting(reason: .turn, receivedBytes: 0, totalBytes: 100)), "Queued")
        XCTAssertEqual(line(DownloadStatusWaiting(reason: .turn, receivedBytes: 30, totalBytes: 100)), "Queued · 30%")
        XCTAssertEqual(line(DownloadStatusWaiting(reason: .wifi, receivedBytes: 31, totalBytes: 100)), "Waiting for Wi-Fi · 31%")
        XCTAssertEqual(line(DownloadStatusWaiting(reason: .network, receivedBytes: 8, totalBytes: 100)), "Offline · 8%")
        XCTAssertEqual(line(DownloadStatusWaiting(reason: .server, receivedBytes: 8, totalBytes: 100)), "Waiting for server · 8%")
        XCTAssertEqual(line(DownloadStatusPaused(receivedBytes: 40, totalBytes: 100)), "Paused · 40%")
        XCTAssertEqual(DownloadWords.badge(item(DownloadStatusPaused(receivedBytes: 40, totalBytes: 100))), "Paused · 40%")
        XCTAssertEqual(DownloadWords.badge(item(ready)), "Downloaded")
    }

    func testOneTheServerConvertsIsHalfDoneOnceConverted() {
        XCTAssertEqual(line(DownloadStatusQueued.shared, prepared: true), "Queued")
        XCTAssertEqual(line(preparing(0.52), prepared: true), "Preparing · 26%")
        XCTAssertEqual(line(preparing(0.99), prepared: true), "Preparing · 49%")
        XCTAssertEqual(line(downloading(0), prepared: true), "Downloading · 50%")
        XCTAssertEqual(line(downloading(40), prepared: true), "Downloading · 70%")
        XCTAssertEqual(line(downloading(100), prepared: true), "Downloading · 99%")
        XCTAssertEqual(line(DownloadStatusWaiting(reason: .turn, receivedBytes: 0, totalBytes: 100), prepared: true), "Queued · 50%")
        XCTAssertEqual(line(DownloadStatusWaiting(reason: .wifi, receivedBytes: 20, totalBytes: 100), prepared: true), "Waiting for Wi-Fi · 60%")
        XCTAssertEqual(line(DownloadStatusPaused(receivedBytes: 40, totalBytes: 100), prepared: true), "Paused · 70%")
        XCTAssertEqual(DownloadWords.badge(item(preparing(0.4), prepared: true)), "Preparing · 20%")
    }

    func testTheRingIsTheLinesPercentAndTurnsUntilThereIsOne() {
        let queued = DownloadWords.progress(item(DownloadStatusQueued.shared))
        XCTAssertEqual(queued, DownloadProgress(stage: .queued, percent: nil))
        XCTAssertEqual(queued?.active, false)
        let converting = DownloadWords.progress(item(preparing(0.45), prepared: true))
        XCTAssertEqual(converting, DownloadProgress(stage: .preparing, percent: 22))
        XCTAssertEqual(converting?.fraction, 0.22)
        XCTAssertEqual(converting?.active, true)
        XCTAssertNil(DownloadWords.progress(item(preparing(0)))?.fraction)
        XCTAssertEqual(DownloadWords.progress(item(downloading(63)))?.active, true)
        let paused = DownloadWords.progress(item(DownloadStatusPaused(receivedBytes: 40, totalBytes: 100), prepared: true))
        XCTAssertEqual(paused?.percent, 70)
        XCTAssertEqual(paused?.fraction, 0.7)
        XCTAssertEqual(paused?.active, false)
    }

    func testAShowIsOneStateAndTheAverageOfItsEpisodes() {
        func show(_ episodes: [DownloadItem]) -> String { DownloadWords.line(show: episodes, display: display) }
        let queued = episode(DownloadStatusQueued.shared)
        let eight = Array(repeating: queued, count: 8)
        XCTAssertEqual(show([episode(preparing(0.52)), episode(preparing(0.40))]), "Downloading · 23%")
        XCTAssertEqual(DownloadWords.progress(show: [episode(preparing(0.52))])?.stage, .downloading)
        XCTAssertEqual(show([episode(ready), episode(downloading(50))] + eight), "Downloading · 17%")
        XCTAssertEqual(show([episode(ready), episode(downloading(50), prepared: false)] + eight), "Downloading · 15%")
        XCTAssertEqual(show([episode(preparing(0.8)), episode(downloading(30))]), "Downloading · 52%")
        XCTAssertEqual(show([episode(preparing(0.99))]), "Downloading · 49%")
        XCTAssertEqual(show([episode(downloading(0))]), "Downloading · 50%")
        XCTAssertEqual(show([queued, queued]), "Queued")
        XCTAssertEqual(show([episode(ready), queued]), "Queued · 50%")
        XCTAssertEqual(show([episode(ready), episode(DownloadStatusPaused(receivedBytes: 30, totalBytes: 100), prepared: false)]), "Paused · 65%")
        XCTAssertEqual(
            show([episode(DownloadStatusWaiting(reason: .wifi, receivedBytes: 0, totalBytes: 100)), episode(DownloadStatusPaused(receivedBytes: 0, totalBytes: 100))]),
            "Waiting for Wi-Fi · 50%"
        )
        let failed = episode(DownloadStatusFailed(reason: "notEnoughSpace"))
        XCTAssertEqual(show([episode(preparing(0.52)), failed]), "Downloading · 26%")
        XCTAssertEqual(show([failed, failed]), "Not enough space")
    }

    func testAFinishedShowSaysItsEpisodesAndSize() {
        let episodes = [
            item(ready, kind: .episode, season: 3, episode: 1, size: 380_000_000),
            item(ready, kind: .episode, season: 3, episode: 2, size: 410_000_000),
            item(DownloadStatusFailed(reason: "notEnoughSpace"), kind: .episode, season: 3, episode: 3),
        ]
        XCTAssertEqual(DownloadWords.line(show: Array(episodes.prefix(2)), display: display), "2 episodes · 790 MB")
        XCTAssertNil(DownloadWords.progress(show: Array(episodes.prefix(2))))
        XCTAssertEqual(DownloadWords.line(show: [episodes[0], episodes[2]], display: display), "1 episode · 380 MB")
    }

    func testAFailedOneSaysOnlyWhy() {
        let failed = item(DownloadStatusFailed(reason: "notEnoughSpace"), kind: .episode, season: 1, episode: 2)
        XCTAssertEqual(DownloadWords.line(failed, display: display), "Not enough space")
        XCTAssertEqual(DownloadWords.line(item(DownloadStatusFailed(reason: "checkFailed")), display: display), "Didn't finish")
        XCTAssertEqual(DownloadWords.line(item(DownloadStatusFailed(reason: "notPlayable")), display: display), "Can't play here")
        XCTAssertNil(DownloadWords.progress(failed))
    }
}
