import XCTest
@testable import Kivo

/// The unit tests run in a host that starts nothing of its own.
final class LaunchTests: XCTestCase {
    func testTheseTestsRunInAnInertHost() {
        XCTAssertTrue(Launch.isTestHost)
    }
}
