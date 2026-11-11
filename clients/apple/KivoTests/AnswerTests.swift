import Foundation
import KivoCore
import XCTest
@testable import Kivo

/// A read's answer, once: the first it gives, and nil for a failure, a
/// server that isn't answering, or a task cancelled while it waits.
@MainActor
final class AnswerTests: XCTestCase {
    private var server: StubServer!
    private var session: KivoSession!
    /// What the server says to a read of the libraries.
    nonisolated(unsafe) private static var libraries: (Int, String) = (200, "{}")

    override func setUp() async throws {
        server = try StubServer { request in
            request.path == "/api/v1/server" ? (200, #"{"id": "answer-test", "name": "Answer test", "version": "1.0"}"#) : Self.libraries
        }
        let address = try await server.start()
        let directory = FileManager.default.temporaryDirectory.appending(path: UUID().uuidString)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let core = KivoClient(device: AppleDevice(
            directory: directory.path(percentEncoded: false), name: "Test", identity: AppleIdentity.current,
            clientId: nil, subtitles: [], trackSelection: false, transport: NoTransport()
        ))
        session = try await core.restore(server: SavedServer(id: "answer-test", name: "Answer test", addresses: [address]))
    }

    override func tearDown() async throws {
        session.close()
        server.stop()
    }

    func testTheAnswerIsWhatTheReadLoads() async throws {
        Self.libraries = (200, #"{"items": [{"id": "l1", "name": "Movies", "pinned": false, "kind": "movies", "workCount": 0}]}"#)

        let libraries = await session.library.libraries().answer() as? [Library]

        XCTAssertEqual(libraries?.map(\.id), ["l1"])
        XCTAssertEqual(server.requests(to: "/api/v1/libraries").count, 1, "one read, stopped once it answered")
    }

    func testAFailureIsNoAnswer() async throws {
        Self.libraries = (500, #"{"type": "about:blank", "title": "", "status": 500, "code": "internal", "requestId": "r"}"#)

        let libraries = await session.library.libraries().answer()

        XCTAssertNil(libraries)
    }

    func testAServerThatIsNotAnsweringIsNotWaitedFor() async throws {
        server.stop()

        let libraries = await session.library.libraries().answer()

        XCTAssertNil(libraries)
    }

    func testCancellingWhileItWaitsIsNoAnswer() async throws {
        Self.libraries = (200, #"{"items": []}"#)
        server.delay = 30
        let read = session.library.libraries()
        let waiting = Task { await read.answer() }
        try await Task.sleep(for: .milliseconds(300))
        let started = ContinuousClock.now

        waiting.cancel()
        let libraries = await waiting.value

        XCTAssertNil(libraries)
        XCTAssertLessThan(ContinuousClock.now - started, .seconds(5), "it returns when cancelled, not when the server answers")
    }
}
