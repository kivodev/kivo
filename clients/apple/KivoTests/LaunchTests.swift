import XCTest
@testable import Kivo

/// What the launch environment asks of the app: hosting the unit tests,
/// and the server sign-in goes to.
final class LaunchTests: XCTestCase {
    func testTheseTestsRunInAnInertHost() {
        XCTAssertTrue(Launch.isTestHost)
    }

    func testAnAppLaunchedNormallyIsNoTestHost() {
        XCTAssertFalse(Launch.isTestHost([:]))
        XCTAssertFalse(Launch.isTestHost(["KIVO_SERVER": "http://127.0.0.1:18484"]))
        XCTAssertTrue(Launch.isTestHost(["XCTestConfigurationFilePath": "/tmp/x.xctestconfiguration"]))
        XCTAssertTrue(Launch.isTestHost(["XCTestBundlePath": "/tmp/KivoTests.xctest"]))
    }

    func testTheServerComesFromTheEnvironment() {
        XCTAssertEqual(Launch.server(["KIVO_SERVER": " http://127.0.0.1:18484\n"]), "http://127.0.0.1:18484")
        XCTAssertNil(Launch.server(["KIVO_SERVER": "  "]))
        XCTAssertNil(Launch.server([:]))
    }
}
