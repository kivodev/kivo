import Foundation
import KivoCore
import UIKit
import XCTest
@testable import Kivo

/// The app and device every request says, in its identity headers:
/// the app, its version, iPhone or iPad, the system's version and the
/// machine identifier, never the device's name.
@MainActor
final class ClientIdentityTests: XCTestCase {
    func testEveryRequestSaysTheAppAndDevice() async throws {
        let server = try StubServer { request in
            request.path == "/api/v1/server"
                ? (200, #"{"id": "identity-test", "name": "Identity test", "version": "1.0"}"#)
                : (200, #"{"items": []}"#)
        }
        defer { server.stop() }
        let address = try await server.start()
        let directory = FileManager.default.temporaryDirectory.appending(path: UUID().uuidString)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let core = KivoClient(device: AppleDevice(
            directory: directory.path(percentEncoded: false), name: "Alex's iPhone", identity: AppleIdentity.current,
            clientId: nil, subtitles: [], trackSelection: false, transport: NoTransport()
        ))
        let session = try await core.restore(server: SavedServer(id: "identity-test", name: "Identity test", addresses: [address]))
        defer { session.close() }
        for await load in session.library.libraries() where load.getOrNull() != nil { break }

        let headers = try XCTUnwrap(server.requests(to: "/api/v1/libraries").first?.headers)
        let platform = UIDevice.current.userInterfaceIdiom == .pad ? "iPad" : "iPhone"
        let info = try XCTUnwrap(Bundle.main.infoDictionary)
        let version = try XCTUnwrap(info["CFBundleShortVersionString"] as? String)
        let build = try XCTUnwrap(info["CFBundleVersion"] as? String)
        XCTAssertFalse(version.isEmpty)
        XCTAssertEqual(headers["kivo-app"], "kivo-apple")
        XCTAssertEqual(headers["kivo-app-version"], "\(version)+\(build)")
        let semver = try XCTUnwrap(headers["kivo-app-version"]).wholeMatch(of: /\d+\.\d+\.\d+\+\d+/)
        XCTAssertNotNil(semver, "a semantic version with the build number: \(headers)")
        XCTAssertEqual(headers["kivo-platform"], platform)
        XCTAssertEqual(headers["kivo-os-version"], UIDevice.current.systemVersion)
        XCTAssertEqual(headers["kivo-device-model"], AppleIdentity.machine)
        XCTAssertFalse(headers.values.contains { $0.contains("Alex") }, "the device's name isn't sent: \(headers)")
        XCTAssertFalse(AppleIdentity.machine.isEmpty)
    }

    func testTheVersionNamesTheBuild() {
        XCTAssertEqual(AppleIdentity.version(["CFBundleShortVersionString": "0.3.0", "CFBundleVersion": "78"]), "0.3.0+78")
        XCTAssertEqual(AppleIdentity.version(["CFBundleShortVersionString": "0.3.0"]), "0.3.0")
        XCTAssertEqual(AppleIdentity.version([:]), "")
    }
}
