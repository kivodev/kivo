import XCTest
@testable import KivoUI

/// Artwork carries the session's token only where the core says it may go now.
final class ArtworkSourceTests: XCTestCase {
    func testTheTokenGoesOnlyWhereTheGateGivesIt() throws {
        let gate: @Sendable (String) -> String? = { $0.hasPrefix("https://kivo.example.com/") ? "Bearer tok" : nil }

        let away = ArtworkSource(baseURL: "https://kivo.example.com", authorization: gate, serverId: "server-1")
        let request = try XCTUnwrap(away.request(id: "i1", pixelWidth: 300)?.urlRequest)
        XCTAssertEqual(request.url?.absoluteString, "https://kivo.example.com/api/v1/images/i1?width=320")
        XCTAssertEqual(request.value(forHTTPHeaderField: "Authorization"), "Bearer tok")

        let unproven = ArtworkSource(baseURL: "http://192.168.1.20:8484", authorization: gate, serverId: "server-1")
        let local = try XCTUnwrap(unproven.request(id: "i1", pixelWidth: 300)?.urlRequest)
        XCTAssertNil(local.value(forHTTPHeaderField: "Authorization"))
    }
}
