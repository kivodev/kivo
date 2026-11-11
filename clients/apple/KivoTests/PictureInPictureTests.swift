import Foundation
import KivoCore
import XCTest
@testable import Kivo

/// Picture in Picture and AirPlay as the player tells the core's video
/// player (PlayerModel.tellOverlayShows), against a server on localhost
/// whose plays say how their subtitle is delivered: going there with a
/// subtitle file showing plays again without subtitle files, coming back
/// plays again as before when that play burned it in, and a subtitle in
/// the stream carries on with no new play.
@MainActor
final class PictureInPictureTests: XCTestCase {
    /// How the next plays deliver subtitle `s1`, in order; the last stays.
    nonisolated(unsafe) private static var deliveries: [String] = []
    private var server: StubServer!
    private var session: KivoSession!
    private var player: KivoCore.VideoPlayer!

    nonisolated private static func json(_ value: Any) -> String {
        String(decoding: try! JSONSerialization.data(withJSONObject: value), as: UTF8.self)
    }

    nonisolated private static func answer(_ request: StubServer.Request) -> (Int, String) {
        switch (request.method, request.path) {
        case (_, "/api/v1/server"):
            return (200, json(["id": "pip-test", "name": "PiP test", "version": "1.0"]))
        case ("POST", "/api/v1/plays"):
            let delivery = deliveries.count > 1 ? deliveries.removeFirst() : deliveries.first ?? "stream"
            var subtitle: [String: Any] = ["trackId": "s1", "type": "subtitle", "selected": true, "delivery": delivery]
            if delivery == "external" {
                subtitle["codec"] = "ass"
                subtitle["url"] = "subtitles/s1.ass"
            }
            return (201, json([
                "id": "p\(UUID().uuidString.prefix(8))", "workId": "w1", "versionId": "v1", "startMs": 0, "durationMs": 600_000,
                "local": true, "delivery": "stream", "url": "plays/p1/master.m3u8", "parts": [], "chapters": [],
                "tracks": [
                    ["trackId": "v", "type": "video", "selected": true, "delivery": "stream"],
                    subtitle,
                    ["trackId": "off", "type": "subtitle"],
                ],
            ]))
        case ("PATCH", _), ("POST", _):
            return (200, "{}")
        default:
            return (404, json(["type": "about:blank", "title": "Not found", "status": 404, "code": "notFound", "requestId": "r"]))
        }
    }

    override func setUp() async throws {
        server = try StubServer { Self.answer($0) }
        let address = try await server.start()
        let directory = FileManager.default.temporaryDirectory.appending(path: UUID().uuidString)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let core = KivoClient(device: AppleDevice(
            directory: directory.path(percentEncoded: false), name: "Test", identity: AppleIdentity.current,
            clientId: nil, subtitles: AppModel.subtitles, trackSelection: true, transport: NoTransport()
        ))
        session = try await core.restore(server: SavedServer(id: "pip-test", name: "PiP test", addresses: [address]))
    }

    override func tearDown() async throws {
        player?.dispose()
        session.close()
        server.stop()
    }

    func testASubtitleFilePlaysAgainBurnedInAndBackAsAFile() async throws {
        Self.deliveries = ["external", "burn", "external"]
        play()
        try await until("the subtitle file showing") { self.player.controller.state.value.subtitle is TrackShowExternal }
        XCTAssertEqual(try externalSubtitles(play: 0), ["ass", "hdmv_pgs_subtitle"])

        player.setOverlayVisible(visible: false)
        try await until("a play with the subtitle burned in") { self.player.controller.state.value.subtitle is TrackShowBurned }
        XCTAssertEqual(try externalSubtitles(play: 1), [], "a play in Picture in Picture asks for no subtitle files")
        XCTAssertEqual(try body(play: 1)["subtitleTrackId"] as? String, "s1")

        player.setOverlayVisible(visible: true)
        try await until("the subtitle file again") { self.player.controller.state.value.subtitle is TrackShowExternal }
        XCTAssertEqual(try externalSubtitles(play: 2), ["ass", "hdmv_pgs_subtitle"])
    }

    func testASubtitleInTheStreamCarriesOn() async throws {
        Self.deliveries = ["stream"]
        play()
        try await until("the subtitle in the stream") { self.player.controller.state.value.subtitle is TrackShowStream }

        player.setOverlayVisible(visible: false)
        try await Task.sleep(for: .milliseconds(500))
        player.setOverlayVisible(visible: true)
        try await Task.sleep(for: .milliseconds(500))
        XCTAssertEqual(plays().count, 1, "no new play for a subtitle in the stream")
    }

    /// A player on w1, which starts its play at once.
    private func play() {
        player = session.playback.videoPlayer(engine: IdleEngine(), choice: PlayChoice(workId: "w1"), hideControlsAfterMs: 3_000)
    }

    private func plays() -> [StubServer.Request] {
        server.requests(to: "/api/v1/plays").filter { $0.method == "POST" }
    }

    private func body(play index: Int) throws -> [String: Any] {
        let plays = plays()
        guard plays.indices.contains(index) else { throw XCTSkip("no play \(index)") }
        return try XCTUnwrap(JSONSerialization.jsonObject(with: plays[index].body) as? [String: Any])
    }

    /// The codecs play [index] asked for as subtitle files.
    private func externalSubtitles(play index: Int) throws -> [String] {
        let subtitles = (try body(play: index)["capabilities"] as? [String: Any])?["subtitles"] as? [[String: Any]] ?? []
        return subtitles.filter { ($0["delivery"] as? [String])?.contains("external") == true }.compactMap { $0["codec"] as? String }
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

/// An engine that loads nothing and never reports: the controller's play stays loading.
private final class IdleEngine: NSObject, PlayerEngine {
    var positionMs: Int64 { 0 }
    var paused: Bool { true }
    var bandwidthBps: KotlinLong? { nil }
    func load(play: PlayStarted, events: any PlayerEngineEvents) {}
    func unload() {}
    func play() {}
    func pause() {}
    func seek(positionMs: Int64) {}
    func showSubtitle(show: any TrackShow) {}
    func selectAudio(index: Int32) {}
}
