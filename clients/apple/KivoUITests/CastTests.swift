import XCTest

/// Casting's screens against a real server, with the app's stand-in for a
/// Cast device (`KIVO_CAST_PREVIEW`, in debug builds), since the Simulator
/// finds none: the Cast button not connected, connecting and connected on
/// Home, a library and a work's page, and in the player; the Cast menu;
/// the remote with its sheets, buffering, up next and a failed cast; the
/// mini controller, and when the TV can't reach the server; and Stop
/// casting carrying on here. Each cast asks the server for a real cast token. The server and
/// user come from the environment as in SignInAndPlayTests; it signs out at
/// the end.
final class CastTests: XCTestCase {
    private let app = XCUIApplication()
    private let environment = ProcessInfo.processInfo.environment

    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    func testCastScreens() throws {
        let library = environment["KIVO_LIBRARY"] ?? "Movies"
        let shows = environment["KIVO_SHOWS"] ?? "TV"

        // A Cast device found: the button, not connected, on each top bar and in the player.
        try launch("found")
        let cast = app.buttons["cast"].firstMatch
        XCTAssert(cast.waitForExistence(timeout: 10), "Home has no Cast button")
        sleep(2)
        keep("1-home-not-connected")
        open(library: library)
        XCTAssert(app.buttons["cast"].firstMatch.exists, "\(library) has no Cast button")
        keep("1-library-not-connected")
        openFirstTitle()
        keep("1-title-not-connected")
        app.buttons["play"].tap()
        keepPlayer("1-player-not-connected")

        // Connecting: the arcs light in turn.
        try launch("connecting")
        XCTAssert(app.buttons["cast"].firstMatch.waitForExistence(timeout: 10), "Home has no Cast button")
        sleep(1)
        keep("2-home-connecting")

        // Connected: the button lit, the Cast menu, the remote and its sheets.
        try launch("connected")
        XCTAssert(app.buttons["cast"].firstMatch.waitForExistence(timeout: 10), "Home has no Cast button")
        sleep(2)
        keep("3-home-connected")
        open(library: library)
        keep("3-library-connected")
        openFirstTitle()
        keep("3-title-connected")
        app.buttons["cast"].firstMatch.tap()
        XCTAssert(app.buttons["stopCasting"].waitForExistence(timeout: 5), "the Cast menu didn't open")
        sleep(1)
        keep("3-menu-nothing-playing")
        app.buttons["castMenuClose"].tap()
        sleep(1)

        app.buttons["play"].tap()
        XCTAssert(app.buttons["Pause"].waitForExistence(timeout: 30), "the remote didn't show the cast playing")
        sleep(3)
        keep("4-remote")
        app.buttons["tracks"].tap()
        sleep(2)
        keep("4-remote-audio-subtitles")
        app.buttons["Done"].firstMatch.tap()
        sleep(1)
        app.buttons["settings"].tap()
        sleep(2)
        keep("4-remote-playback-settings")
        if app.buttons["Stream info"].exists {
            app.buttons["Stream info"].tap()
            sleep(1)
            keep("4-remote-stream-info")
            app.navigationBars["Stream info"].buttons.firstMatch.tap()
            sleep(1)
        }
        app.buttons["Done"].firstMatch.tap()
        sleep(1)
        app.buttons["cast"].firstMatch.tap()
        XCTAssert(app.buttons["disconnect"].waitForExistence(timeout: 5), "the Cast menu didn't open over the remote")
        sleep(1)
        keep("5-menu-playing")
        app.buttons["castMenuClose"].tap()
        sleep(1)

        // Closed to the mini controller, which shows on every tab.
        app.buttons["Close"].firstMatch.tap()
        XCTAssert(app.descendants(matching: .any)["castMiniController"].firstMatch.waitForExistence(timeout: 5), "no mini controller")
        sleep(1)
        keep("6-title-mini-controller")
        app.tabBars.buttons["Home"].tap()
        sleep(2)
        keep("6-home-mini-controller")
        app.swipeUp()
        sleep(2)
        keep("6-home-mini-controller-scrolled")
        app.swipeDown()
        sleep(1)

        // Stop casting carries on here, at the TV's place.
        app.buttons["cast"].firstMatch.tap()
        XCTAssert(app.buttons["stopCasting"].waitForExistence(timeout: 5), "the Cast menu didn't open")
        app.buttons["stopCasting"].tap()
        keepPlayer("7-after-stop-casting")

        // The TV loading or buffering.
        try launch("buffering")
        open(library: library)
        openFirstTitle()
        app.buttons["play"].tap()
        sleep(4)
        keep("8-remote-buffering")

        // The TV can't reach the server: the mini controller says so.
        try launch("unreachable")
        open(library: library)
        openFirstTitle()
        app.buttons["play"].tap()
        XCTAssert(app.buttons["castRetry"].waitForExistence(timeout: 30), "the remote didn't say the server can't be reached")
        sleep(2)
        keep("9-remote-unreachable")
        app.buttons["Close"].firstMatch.tap()
        sleep(2)
        keep("9-mini-controller-unreachable")

        // Up next, as the TV shows it.
        try launch("upNext")
        open(library: shows)
        if app.buttons["chip-titles"].exists { app.buttons["chip-titles"].tap() }
        let series = app.buttons["work"].firstMatch
        XCTAssert(series.waitForExistence(timeout: 20), "\(shows)'s grid has no cards")
        series.tap()
        XCTAssert(app.buttons["play"].waitForExistence(timeout: 20), "the series has no Play")
        app.buttons["play"].tap()
        XCTAssert(app.buttons["Pause"].waitForExistence(timeout: 30), "the remote didn't show")
        sleep(4)
        keep("10-remote-up-next")
        app.buttons["Close"].firstMatch.tap()

        // The TV can't play it: the remote says why with Try again, and so does the mini controller.
        try launch("failed")
        open(library: library)
        openFirstTitle()
        app.buttons["play"].tap()
        XCTAssert(app.buttons["castRetry"].waitForExistence(timeout: 30), "the remote didn't say the cast failed")
        sleep(2)
        keep("11-remote-failed")
        app.buttons["Close"].firstMatch.tap()
        sleep(2)
        keep("11-mini-controller-failed")

        signOut()
    }

    // MARK: - Steps

    /// Launches the app with the Cast stand-in in [scene], signing in if the Keychain holds no sign-in.
    private func launch(_ scene: String) throws {
        app.launchEnvironment["KIVO_CAST_PREVIEW"] = scene
        try signIn(app)
        XCTAssert(app.buttons["work"].firstMatch.waitForExistence(timeout: 20), "Home has no cards")
    }

    /// Opens [library] from the Libraries tab's list of libraries.
    private func open(library: String) {
        app.tabBars.buttons["Libraries"].tap()
        let list = app.descendants(matching: .any)["library-list"]
        if !list.waitForExistence(timeout: 5) { app.tabBars.buttons["Libraries"].tap() }
        XCTAssert(list.waitForExistence(timeout: 10), "the Libraries tab has no list")
        let row = app.descendants(matching: .any)["library:\(library)"].firstMatch
        XCTAssert(row.waitForExistence(timeout: 5), "the list has no \(library)")
        row.tap()
        XCTAssert(app.buttons["work"].firstMatch.waitForExistence(timeout: 20), "\(library)'s grid has no cards")
        sleep(2)
    }

    private func openFirstTitle() {
        app.buttons["work"].firstMatch.tap()
        XCTAssert(app.buttons["play"].waitForExistence(timeout: 20), "the title's page has no Play")
        sleep(2)
    }

    /// The player once it plays, its controls brought back after they hide, held in landscape for the screenshot; then closed.
    private func keepPlayer(_ name: String) {
        let pause = app.buttons["Pause"]
        XCTAssert(pause.waitForExistence(timeout: 30), "playback didn't start")
        XCUIDevice.shared.orientation = .landscapeLeft
        wait(for: [expectation(for: NSPredicate(format: "exists == false"), evaluatedWith: pause)], timeout: 15)
        app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.4)).tap()
        XCTAssert(pause.waitForExistence(timeout: 5), "the controls didn't come back")
        sleep(1)
        keep(name)
        app.buttons["Close"].tap()
        XCUIDevice.shared.orientation = .portrait
        sleep(1)
    }

    private func signOut() {
        app.tabBars.buttons["Home"].tap()
        app.buttons["user"].tap()
        app.buttons["account"].tap()
        app.buttons["Sign out"].tap()
        // The question's own Sign out: Account's, under it, comes first.
        let confirm = app.sheets.buttons["Sign out"]
        XCTAssert(confirm.waitForExistence(timeout: 5), "Sign out didn't ask first")
        confirm.tap()
        XCTAssert(app.textFields["username"].waitForExistence(timeout: 10) || app.textFields["address"].exists, "didn't sign out")
    }
}
