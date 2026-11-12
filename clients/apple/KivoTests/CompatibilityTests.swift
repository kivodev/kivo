import Foundation
import KivoCore
import XCTest
@testable import Kivo

/// Whether the app and a server work together, as the app finds it against
/// a server on localhost: a server too old for the app and an app too old
/// for the server each say so in place of sign-in and of the server's
/// screens, and a server that says nothing of versions is let through.
@MainActor
final class CompatibilityTests: XCTestCase {
    /// What the server says it is, which a test changes.
    nonisolated(unsafe) private static var info = ""

    private static func serverIs(version: String, minAppVersion: String?) {
        let oldest = minAppVersion.map { #", "minAppVersion": "\#($0)""# } ?? ""
        info = #"{"id": "compat-test", "name": "Den", "version": "\#(version)"\#(oldest)}"#
    }

    private var server: StubServer!
    private var address = ""
    private var core: KivoClient!

    private var appVersion: String {
        Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? ""
    }

    override func setUp() async throws {
        server = try StubServer { request in
            request.path == "/api/v1/server" ? (200, Self.info) : (200, #"{"items": []}"#)
        }
        address = try await server.start()
        let directory = FileManager.default.temporaryDirectory.appending(path: UUID().uuidString)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        core = KivoClient(device: AppleDevice(
            directory: directory.path(percentEncoded: false), name: "iPhone", identity: AppleIdentity.current,
            clientId: nil, subtitles: [], trackSelection: false, transport: NoTransport()
        ))
    }

    override func tearDown() {
        server.stop()
    }

    func testAServerTooOldForTheAppSaysSoInPlaceOfItsPassword() async throws {
        Self.serverIs(version: "0.0.1", minAppVersion: "0.0.1")
        let found = try await core.serverAt(address: address)

        guard case let .incompatible(_, problem) = SignInModel.step(for: found, core.compatibility(info: found.info)) else {
            return XCTFail("a server older than the app works with goes to its password")
        }
        XCTAssertEqual(problem.title, "This server needs an update")
        XCTAssertTrue(problem.sentence.hasPrefix("Kivo \(appVersion) works with server "), problem.sentence)
        XCTAssertTrue(problem.sentence.hasSuffix(" or newer. Den runs 0.0.1. Ask whoever runs it to update."), problem.sentence)
    }

    func testAnAppTooOldForTheServerSaysSoInPlaceOfItsPassword() async throws {
        Self.serverIs(version: "99.0.0", minAppVersion: "99.0.0")
        let found = try await core.serverAt(address: address)

        guard case let .incompatible(_, problem) = SignInModel.step(for: found, core.compatibility(info: found.info)) else {
            return XCTFail("a server that needs a newer app goes to its password")
        }
        XCTAssertEqual(problem.title, "Update Kivo")
        XCTAssertEqual(problem.sentence, "Den needs Kivo 99.0.0 or newer. You have \(appVersion).")
    }

    func testAServerTheAppWorksWithGoesToItsPassword() async throws {
        for (version, oldest) in [(appVersion, appVersion as String?), ("99.0.0", nil), ("dev", "dev"), ("d76b7e0", "")] {
            Self.serverIs(version: version, minAppVersion: oldest)
            let found = try await core.serverAt(address: address)
            guard case .password = SignInModel.step(for: found, core.compatibility(info: found.info)) else {
                return XCTFail("a server at \(version) that supports apps from \(oldest ?? "any") is refused")
            }
        }
    }

    func testASavedServerIsCheckedAsItOpensAndAgainOnTryAgain() async throws {
        Self.serverIs(version: "0.0.1", minAppVersion: nil)
        let session = try await core.restore(server: SavedServer(id: "compat-test", name: "Den", addresses: [address]))
        defer { session.close() }
        XCTAssertEqual(Incompatibility(session.compatibility.value, server: "Den")?.title, "This server needs an update")

        Self.serverIs(version: "99.0.0", minAppVersion: "99.0.0")
        _ = try await session.serverInfo()
        XCTAssertEqual(Incompatibility(session.compatibility.value, server: "Den")?.title, "Update Kivo")

        Self.serverIs(version: "99.0.0", minAppVersion: appVersion)
        _ = try await session.serverInfo()
        XCTAssertNil(Incompatibility(session.compatibility.value, server: "Den"))
    }
}
