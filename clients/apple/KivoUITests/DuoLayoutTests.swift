import XCTest

/// The iPhone Duo's screens against a real server, with the same
/// environment as SignInAndPlayTests (KIVO_SERVER, KIVO_USERNAME,
/// KIVO_PASSWORD, and KIVO_LIBRARY, Movies by default). It walks Home,
/// Libraries, a library, a work's page, Search, Downloads and Settings in
/// portrait and landscape, keeping a screenshot of each, named
/// `duo-<KIVO_DUO_POSE>-…` (`duo-outer-…` by default; `inner` once the
/// Simulator is unfolded), and with `KIVO_SHOTS_DIR` set, writes each there
/// as a PNG (see `keep` for the unfolded display). It plays nothing and
/// changes nothing on the server.
final class DuoLayoutTests: XCTestCase {
    private let app = XCUIApplication()
    private let environment = ProcessInfo.processInfo.environment
    private var library: String { environment["KIVO_LIBRARY"] ?? "Movies" }
    private var pose: String { environment["KIVO_DUO_POSE"] ?? "outer" }

    override func setUpWithError() throws {
        continueAfterFailure = false
        try XCTSkipUnless(UIDevice.current.userInterfaceIdiom == .phone, "the iPhone's layouts")
        XCUIDevice.shared.orientation = .portrait
    }

    override func tearDown() {
        // hasSucceeded is false until the run stops, which is after tearDown.
        if let testRun, testRun.totalFailureCount > 0 { keep("failure", "end") }
        super.tearDown()
    }

    func testScreens() throws {
        try signIn(app, home: app.homeTab, allowingLocalNetwork: true)
        // `KIVO_DUO_ORIENTATIONS` picks them, "portrait,landscape" by default:
        // the unfolded display stays landscape, and XCTest's taps turn with
        // the device while its screen doesn't.
        let orientations = (environment["KIVO_DUO_ORIENTATIONS"] ?? "portrait,landscape").split(separator: ",")
        for name in orientations {
            XCUIDevice.shared.orientation = name == "portrait" ? .portrait : .landscapeLeft
            sleep(2)
            walk(String(name))
        }
        XCUIDevice.shared.orientation = .portrait
    }

    // MARK: Steps

    private func walk(_ orientation: String) {
        tab("Home")
        XCTAssert(app.buttons["user"].waitForExistence(timeout: 15), "Home has no avatar")
        sleep(3)
        keep("home", orientation)
        app.swipeUp()
        sleep(2)
        keep("home-scrolled", orientation)

        tab("Libraries")
        let list = app.descendants(matching: .any)["library-list"]
        if !list.waitForExistence(timeout: 5) { tab("Libraries") }
        XCTAssert(list.waitForExistence(timeout: 10), "the Libraries tab has no list")
        keep("libraries", orientation)
        app.descendants(matching: .any)["library:\(library)"].firstMatch.tap()
        let card = app.buttons.matching(identifier: "work").firstMatch
        XCTAssert(card.waitForExistence(timeout: 15), "\(library) shows no cards")
        sleep(3)
        keep("library", orientation)
        card.tap()
        XCTAssert(app.buttons["play"].waitForExistence(timeout: 15), "the work's page has no Play")
        sleep(3)
        keep("detail", orientation)
        app.swipeUp()
        sleep(2)
        keep("detail-scrolled", orientation)

        tab("Search")
        let field = app.searchFields.firstMatch
        XCTAssert(field.waitForExistence(timeout: 10), "Search has no field")
        // Search keeps its query, so the second walk finds the first's results.
        let result = app.buttons.matching(identifier: "work").firstMatch
        if !result.waitForExistence(timeout: 3) {
            field.tap()
            field.typeText(String(library.prefix(1)) + "\n")
        }
        XCTAssert(result.waitForExistence(timeout: 15), "Search found nothing")
        sleep(3)
        keep("search", orientation)

        tab("Downloads")
        sleep(2)
        keep("downloads", orientation)

        // Choosing Home again scrolls it back up to the avatar.
        tab("Home")
        if !app.buttons["user"].waitForExistence(timeout: 3) { tab("Home") }
        XCTAssert(app.buttons["user"].waitForExistence(timeout: 5), "Home has no avatar")
        app.buttons["user"].tap()
        XCTAssert(app.navigationBars["Settings"].waitForExistence(timeout: 10), "the avatar didn't open Settings")
        keep("settings", orientation)
        app.navigationBars["Settings"].buttons["Done"].tap()
    }

    private func tab(_ name: String) {
        let bar = app.tabBars.buttons[name]
        (bar.exists ? bar : app.buttons[name].firstMatch).tap()
    }

    /// Attaches a screenshot, and in the Simulator also writes it to
    /// `KIVO_SHOTS_DIR` on the Mac, when that's set.
    /// Unfolded, XCTest's screenshots are black, so with `KIVO_SHOTS_FROM_HOST`
    /// set it asks the Mac for one instead: it writes `<name>.want` in
    /// `KIVO_SHOTS_DIR` and waits up to 15 seconds for a watcher there to
    /// write `<name>.png` with `simctl io <device> screenshot --display=3`.
    private func keep(_ name: String, _ orientation: String) {
        let name = "duo-\(pose)-\(orientation)-\(name)"
        if let folder = environment["KIVO_SHOTS_DIR"].map({ URL(filePath: $0) }), environment["KIVO_SHOTS_FROM_HOST"] != nil {
            let png = folder.appending(path: name + ".png")
            try? Data().write(to: folder.appending(path: name + ".want"))
            let written = NSPredicate { _, _ in FileManager.default.fileExists(atPath: png.path()) }
            _ = XCTWaiter.wait(for: [expectation(for: written, evaluatedWith: nil)], timeout: 15)
            return
        }
        let screenshot = XCUIScreen.main.screenshot()
        let shot = XCTAttachment(screenshot: screenshot)
        shot.name = name
        shot.lifetime = .keepAlways
        add(shot)
        if let folder = environment["KIVO_SHOTS_DIR"] {
            try? screenshot.pngRepresentation.write(to: URL(filePath: folder).appending(path: name + ".png"))
        }
    }
}
