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
        let version = try XCTUnwrap(Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String)
        XCTAssertEqual(headers["kivo-app"], "kivo-apple")
        XCTAssertEqual(headers["kivo-app-version"], version)
        XCTAssertEqual(headers["kivo-platform"], platform)
        XCTAssertEqual(headers["kivo-os-version"], UIDevice.current.systemVersion)
        XCTAssertEqual(headers["kivo-device-model"], AppleIdentity.machine)
        XCTAssertFalse(headers.values.contains { $0.contains("Alex") }, "the device's name isn't sent: \(headers)")
        XCTAssertFalse(AppleIdentity.machine.isEmpty)
    }
}
