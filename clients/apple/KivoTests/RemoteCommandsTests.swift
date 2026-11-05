import AVFoundation
import MediaPlayer
import XCTest
@testable import Kivo

/// The lock screen's, CarPlay's, headphones' and a steering wheel's commands
/// for a book and a video.
final class RemoteCommandsTests: XCTestCase {
    func testABooksPreviousNextAndSeekSkipTenBackAndThirtyForward() {
        let book = RemoteCommands.actions(.book)
        XCTAssertEqual(book[.previousTrack], .skipBack)
        XCTAssertEqual(book[.nextTrack], .skipForward)
        XCTAssertEqual(book[.skipBackward], .skipBack)
        XCTAssertEqual(book[.skipForward], .skipForward)
        XCTAssertEqual(book[.changePlaybackPosition], .seek)
        XCTAssertEqual(book[.seekBackward], .skipBack)
        XCTAssertEqual(book[.seekForward], .skipForward)
        XCTAssertEqual(RemoteCommands.backSeconds, 10)
        XCTAssertEqual(RemoteCommands.forwardSeconds, 30)
    }

    /// A car's track buttons arrive as next and previous "in context", which
    /// the system turns into a chapter command no app can answer while Now
    /// Playing has a chapter number or count.
    func testABooksNowPlayingHasNoChapterNumberOrCount() {
        let info = BookPlayer.nowPlayingInfo(
            chapter: "038", book: "The Blade Itself", authors: "Joe Abercrombie",
            durationMs: 600_000, elapsedMs: 12_000, playing: true, speed: 1.5
        )
        XCTAssertNil(info[MPNowPlayingInfoPropertyChapterNumber])
        XCTAssertNil(info[MPNowPlayingInfoPropertyChapterCount])
        XCTAssertEqual(info[MPMediaItemPropertyTitle] as? String, "038")
        XCTAssertEqual(info[MPMediaItemPropertyAlbumTitle] as? String, "The Blade Itself")
        XCTAssertEqual(info[MPMediaItemPropertyArtist] as? String, "Joe Abercrombie")
        XCTAssertEqual(info[MPMediaItemPropertyPlaybackDuration] as? Double, 600)
        XCTAssertEqual(info[MPNowPlayingInfoPropertyElapsedPlaybackTime] as? Double, 12)
        XCTAssertEqual(info[MPNowPlayingInfoPropertyPlaybackRate] as? Double, 1.5)
        XCTAssertEqual(info[MPNowPlayingInfoPropertyDefaultPlaybackRate] as? Double, 1.5)
    }

    func testAPausedBookShowsRateZeroAndItsSpeedAsTheDefault() {
        let info = BookPlayer.nowPlayingInfo(
            chapter: "1", book: "B", authors: nil, durationMs: 1000, elapsedMs: 0, playing: false, speed: 1.25
        )
        XCTAssertEqual(info[MPNowPlayingInfoPropertyPlaybackRate] as? Double, 0)
        XCTAssertEqual(info[MPNowPlayingInfoPropertyDefaultPlaybackRate] as? Double, 1.25)
        XCTAssertNil(info[MPMediaItemPropertyArtist])
    }

    func testAVideoSkipsButHasNoPreviousOrNext() {
        let video = RemoteCommands.actions(.video)
        XCTAssertEqual(video[.skipBackward], .skipBack)
        XCTAssertEqual(video[.skipForward], .skipForward)
        XCTAssertEqual(video[.changePlaybackPosition], .seek)
        XCTAssertEqual(video[.togglePlayPause], .togglePlay)
        XCTAssertNil(video[.previousTrack])
        XCTAssertNil(video[.nextTrack])
        XCTAssertNil(video[.seekForward])
        XCTAssertNil(video[.seekBackward])
    }

    func testASeekPerformsOnceAsThePressEnds() {
        XCTAssertFalse(RemoteCommands.performs(.beginSeeking))
        XCTAssertTrue(RemoteCommands.performs(.endSeeking))
        XCTAssertTrue(RemoteCommands.performs(nil), "any other command performs")
    }

    func testSetUpShowsSkipButtonsAndTurnsTheRestOff() {
        for kind in [RemoteCommands.Kind.book, .video] {
            let session = MPNowPlayingSession(players: [AVPlayer()])
            let center = session.remoteCommandCenter
            let answered = RemoteCommands.setUp(center, for: kind) { _, _ in .success }

            XCTAssertEqual(center.skipBackwardCommand.preferredIntervals, [10])
            XCTAssertEqual(center.skipForwardCommand.preferredIntervals, [30])
            XCTAssertEqual(answered.count, RemoteCommands.actions(kind).count)
            for command in RemoteCommands.Command.allCases {
                let answers = RemoteCommands.actions(kind)[command] != nil
                XCTAssertEqual(command.of(center).isEnabled, answers, "\(kind) \(command)")
            }
            for (command, target) in answered { command.removeTarget(target) }
        }
    }

    func testABookLoadsWithItsCommandsOff() {
        let session = MPNowPlayingSession(players: [AVPlayer()])
        let center = session.remoteCommandCenter
        let answered = RemoteCommands.setUp(center, for: .book, enabled: false) { _, _ in .success }
        XCTAssertTrue(answered.allSatisfy { !$0.0.isEnabled })
        for (command, target) in answered { command.removeTarget(target) }
    }
}
