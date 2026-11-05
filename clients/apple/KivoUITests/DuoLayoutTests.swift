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
    private let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
    private let environment = ProcessInfo.processInfo.environment
    private var library: String { environment["KIVO_LIBRARY"] ?? "Movies" }
    private var pose: String { environment["KIVO_DUO_POSE"] ?? "outer" }

    override func setUpWithError() throws {
        continueAfterFailure = false
        try XCTSkipUnless(UIDevice.current.userInterfaceIdiom == .phone, "the iPhone's layouts")
        app.launchEnvironment["KIVO_CLIENT_ID"] = "kivo-ui-tests"
        XCUIDevice.shared.orientation = .portrait
    }

    override func tearDown() {
        // hasSucceeded is false until the run stops, which is after tearDown.
        if let testRun, testRun.totalFailureCount > 0 { keep("failure", "end") }
        super.tearDown()
    }

    func testScreens() throws {
        try signIn()
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

    private func signIn() throws {
        let server = try XCTUnwrap(environment["KIVO_SERVER"], "set TEST_RUNNER_KIVO_SERVER")
        let username = try XCTUnwrap(environment["KIVO_USERNAME"], "set TEST_RUNNER_KIVO_USERNAME")
        let password = try XCTUnwrap(environment["KIVO_PASSWORD"], "set TEST_RUNNER_KIVO_PASSWORD")
        app.launch()
        // A first run asks to find devices on the local network.
        let allow = springboard.buttons["Allow"]
        if allow.waitForExistence(timeout: 3) { allow.tap() }
        let address = app.textFields["address"]
        let user = app.textFields["username"]
        let home = app.descendants(matching: .any).matching(NSPredicate(format: "label == %@", "Home")).firstMatch
        let start = NSPredicate { _, _ in address.exists || user.exists || home.exists }
        wait(for: [expectation(for: start, evaluatedWith: nil)], timeout: 20)
        if home.exists { return }
        if !user.waitForExistence(timeout: 6) || !app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", server)).firstMatch.exists {
            if user.exists { app.buttons["Change server"].tap() }
            XCTAssert(address.waitForExistence(timeout: 10), "sign-in has no address field")
            address.tap()
            address.typeText(server)
            app.buttons["Continue"].tap()
            XCTAssert(user.waitForExistence(timeout: 15), "the server at \(server) wasn't found")
        }
        user.tap()
        user.typeText(username)
        let secret = app.secureTextFields["password"]
        secret.tap()
        secret.typeText(password + "\n")
        XCTAssert(home.waitForExistence(timeout: 20), "Home didn't show after signing in")
        // iOS's offer to save the password belongs to Safari's view service.
        let notNow = XCUIApplication(bundleIdentifier: "com.apple.SafariViewService").buttons["Not Now"]
        if notNow.waitForExistence(timeout: 5) { notNow.tap() }
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
