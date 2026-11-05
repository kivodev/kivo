import AVFoundation
import Foundation
import KivoCore
import KivoUI
import MediaPlayer
import XCTest
@testable import Kivo

/// Which player's commands the system sends to: the app's active Now
/// Playing session, the last of the book and a video to start. A video's
/// session stays the active one after the video closes, so the book takes
/// it back as it loads, its commands with their previous and next track.
@MainActor
final class NowPlayingSessionTests: XCTestCase {
    private var server: StubServer!
    private var session: KivoSession!
    private var book: BookPlayer!

    nonisolated private static func answer(_ request: StubServer.Request) -> (Int, String) {
        switch request.path {
        case "/api/v1/server":
            (200, #"{"id":"now-playing-test","name":"Now Playing test","version":"1.0"}"#)
        default:
            (404, #"{"type":"about:blank","title":"Not found","status":404,"code":"notFound","requestId":"r"}"#)
        }
    }

    override func setUp() async throws {
        server = try StubServer { Self.answer($0) }
        let address = try await server.start()
        let directory = FileManager.default.temporaryDirectory.appending(path: UUID().uuidString)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let core = KivoClient(device: AppleDevice(
            directory: directory.path(percentEncoded: false), name: "Test", identity: AppleIdentity.current,
            clientId: nil, subtitles: [], trackSelection: true, transport: NoTransport()
        ))
        let session = try await core.restore(server: SavedServer(id: "now-playing-test", name: "Now Playing test", addresses: [address]))
        self.session = session
        book = BookPlayer(
            session: session,
            artwork: { ArtworkSource(baseURL: address, authorization: { _ in nil }, serverId: "now-playing-test") },
            display: core.display
        )
    }

    override func tearDown() async throws {
        book.close()
        session.close()
        server.stop()
    }

    func testABookAfterAVideoIsTheActiveNowPlayingWithItsCommands() async throws {
        var video: MPNowPlayingSession? = playVideo()
        try await until("the video active") { video?.isActive == true }
        video = nil

        book.play("w1")
        try await until("the book active") { self.book.loaded && self.book.nowPlaying.isActive }
        let center = book.nowPlaying.remoteCommandCenter
        for command in RemoteCommands.Command.allCases {
            XCTAssertTrue(command.of(center).isEnabled, "\(command)")
        }
        XCTAssertEqual(center.skipForwardCommand.preferredIntervals, [30])
    }

    func testAVideoAfterABookIsTheActiveNowPlayingWithoutPreviousOrNext() async throws {
        book.play("w1")
        try await until("the book active") { self.book.loaded && self.book.nowPlaying.isActive }

        book.stop()
        try await until("the book stopped") { !self.book.loaded }
        let video = playVideo()
        try await until("the video active") { video.isActive && !self.book.nowPlaying.isActive }
        XCTAssertFalse(video.remoteCommandCenter.nextTrackCommand.isEnabled)
        XCTAssertFalse(video.remoteCommandCenter.previousTrackCommand.isEnabled)
        XCTAssertTrue(video.remoteCommandCenter.skipForwardCommand.isEnabled)
        XCTAssertTrue(book.nowPlaying.remoteCommandCenter.commands.allSatisfy { !$0.isEnabled }, "a stopped book's commands are off")

        book.play("w1")
        try await until("the book active again") { self.book.nowPlaying.isActive && !video.isActive }
    }

    /// A video's session as PlayerModel makes it.
    private func playVideo() -> MPNowPlayingSession {
        let video = MPNowPlayingSession(players: [AVPlayer()])
        video.automaticallyPublishesNowPlayingInfo = true
        _ = RemoteCommands.setUp(video.remoteCommandCenter, for: .video) { _, _ in .success }
        video.becomeActiveIfPossible { _ in }
        return video
    }

    /// Waits up to [seconds] for [condition].
    private func until(_ what: String, seconds: Double = 10, _ condition: () -> Bool) async throws {
        let deadline = Date().addingTimeInterval(seconds)
        while Date() < deadline {
            if condition() { return }
            try await Task.sleep(for: .milliseconds(20))
        }
        XCTFail("timed out waiting for \(what)")
    }
}

private extension MPRemoteCommandCenter {
    var commands: [MPRemoteCommand] { RemoteCommands.Command.allCases.map { $0.of(self) } }
}
