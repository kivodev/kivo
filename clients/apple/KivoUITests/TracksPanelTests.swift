import XCTest

/// Audio & Subtitles with a film of many tracks (the local test server's
/// Many Tracks, or `KIVO_TRACKS_MOVIE` in `KIVO_LIBRARY`), in landscape and
/// in portrait: Audio and Subtitles are lists side by side where the panel
/// is wide, each opened scrolled to its checked row, and two sections of
/// one list where it isn't. Dialogue boost is a switch above the audio
/// tracks, a subtitle far down the list applies and stays checked, Done
/// returns to the controls, and Playback settings' Audio & Subtitles opens
/// it in that panel's place.
final class TracksPanelTests: XCTestCase {
    private let app = XCUIApplication()
    private let environment = ProcessInfo.processInfo.environment

    override func setUpWithError() throws {
        continueAfterFailure = false
        app.launchEnvironment["KIVO_CLIENT_ID"] = "kivo-ui-tests"
        // Sign-in goes straight to the server under test, without browsing the network.
        app.launchEnvironment["KIVO_SERVER"] = environment["KIVO_SERVER"]
    }

    override func tearDown() {
        XCUIDevice.shared.orientation = .portrait
    }

    func testManyTracksInLandscape() throws {
        try manyTracks(.landscapeLeft, named: "landscape")
    }

    func testManyTracksInPortrait() throws {
        try manyTracks(.portrait, named: "portrait")
    }

    private func manyTracks(_ orientation: UIDeviceOrientation, named shape: String) throws {
        try signIn()
        let library = environment["KIVO_LIBRARY"] ?? "Movies"
        let movie = environment["KIVO_TRACKS_MOVIE"] ?? "Many Tracks"
        // Home's Recently Added has it on the test server; else its library.
        let card = app.buttons.matching(identifier: "work").matching(NSPredicate(format: "label CONTAINS %@", movie)).firstMatch
        if !card.waitForExistence(timeout: 10) {
            // The tab opens on the library last opened, over the list of them; choosing it again shows the list.
            app.tabBars.buttons["Libraries"].tap()
            let list = app.descendants(matching: .any)["library-list"]
            if !list.waitForExistence(timeout: 5) { app.tabBars.buttons["Libraries"].tap() }
            XCTAssert(list.waitForExistence(timeout: 10), "the Libraries tab has no list")
            app.descendants(matching: .any)["library:\(library)"].firstMatch.tap()
            if app.buttons["chip-titles"].waitForExistence(timeout: 5) { app.buttons["chip-titles"].tap() }
        }
        XCTAssert(card.waitForExistence(timeout: 20), "\(library) has no \(movie)")
        card.tap()
        let play = app.buttons["play"]
        XCTAssert(play.waitForExistence(timeout: 20), "the detail page has no Play")
        play.tap()
        XCTAssert(app.buttons["Pause"].waitForExistence(timeout: 60), "playback didn't start")
        sleep(2)
        XCUIDevice.shared.orientation = orientation
        sleep(2)
        if !app.buttons["tracks"].exists { _ = showControls() }
        keep("\(shape)-player-controls")

        // The gear's sheet first, to compare the two.
        app.buttons["settings"].tap()
        XCTAssert(app.navigationBars["Playback Settings"].waitForExistence(timeout: 5), "Playback settings didn't open")
        sleep(1)
        keep("\(shape)-playback-settings")
        app.navigationBars["Playback Settings"].buttons["Done"].tap()
        XCTAssert(showControls(), "closing Playback settings didn't return to the controls")

        app.buttons["tracks"].tap()
        let close = app.navigationBars["Audio & Subtitles"].buttons["Done"]
        XCTAssert(close.waitForExistence(timeout: 5), "Audio & Subtitles didn't open")
        let boost = app.switches["dialogue-boost"]
        XCTAssert(boost.exists, "Audio & Subtitles has no Dialogue boost")
        sleep(2)
        keep("\(shape)-tracks-open")

        let audioList = app.collectionViews.containing(.switch, identifier: "dialogue-boost").firstMatch
        let subtitleList = app.collectionViews.containing(NSPredicate(format: "label == 'Off'")).firstMatch
        let sideBySide = subtitleList.frame.minX >= audioList.frame.maxX - 1
        if UIDevice.current.userInterfaceIdiom == .pad || orientation.isLandscape {
            XCTAssert(sideBySide, "Audio and Subtitles aren't side by side")
        } else {
            XCTAssertFalse(sideBySide, "Audio and Subtitles aren't one list")
        }
        let english = app.buttons.matching(NSPredicate(format: "label BEGINSWITH 'English'")).firstMatch
        XCTAssert(boost.frame.minY < english.frame.minY, "Dialogue boost isn't above the audio tracks")

        // A list makes only the rows near the screen, so Audio scrolls to its last.
        let commentary = app.buttons.matching(NSPredicate(format: "label BEGINSWITH 'English · Commentary'")).element(boundBy: 1)
        for _ in 0..<4 where !commentary.isHittable {
            audioList.swipeUp()
        }
        XCTAssert(commentary.isHittable, "Audio has no second commentary")
        keep("\(shape)-tracks-long-names")

        let traditional = app.buttons.matching(NSPredicate(format: "label BEGINSWITH 'Chinese' AND label CONTAINS 'Traditional'")).firstMatch
        for _ in 0..<8 where !traditional.isHittable {
            subtitleList.swipeUp()
        }
        XCTAssert(traditional.isHittable, "Subtitles has no Chinese over Traditional")
        keep("\(shape)-tracks-subtitles-scrolled")
        traditional.tap()
        sleep(4)
        XCTAssert(traditional.isSelected, "Chinese over Traditional isn't checked")
        keep("\(shape)-tracks-subtitle-chosen")

        close.tap()
        XCTAssert(app.buttons["tracks"].waitForExistence(timeout: 5) || showControls(), "closing didn't return to the controls")
        sleep(1)
        if !app.buttons["tracks"].exists { _ = showControls() }
        app.buttons["tracks"].tap()
        XCTAssert(close.waitForExistence(timeout: 5), "Audio & Subtitles didn't open again")
        sleep(2)
        keep("\(shape)-tracks-reopened")
        if sideBySide {
            XCTAssert(traditional.isHittable, "Subtitles didn't open scrolled to the track playing")
        }
        let playing = app.buttons.matching(NSPredicate(format: "label BEGINSWITH 'English' AND selected == true")).firstMatch
        XCTAssert(playing.isHittable, "Audio didn't open scrolled to the track playing")
        close.tap()

        XCTAssert(showControls(), "closing didn't return to the controls")
        app.buttons["settings"].tap()
        XCTAssert(app.navigationBars["Playback Settings"].waitForExistence(timeout: 5), "Playback settings didn't open")
        app.buttons["Audio & Subtitles"].tap()
        XCTAssert(app.navigationBars["Audio & Subtitles"].waitForExistence(timeout: 5), "Audio & Subtitles didn't open the tracks")
        sleep(1)
        keep("\(shape)-tracks-from-settings")
        app.navigationBars["Audio & Subtitles"].buttons["Done"].tap()
        if showControls() { app.buttons["Close"].tap() }
    }

    private func showControls() -> Bool {
        for _ in 0..<3 {
            if app.buttons["Close"].waitForExistence(timeout: 2) { return true }
            app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
        }
        return app.buttons["Close"].exists
    }

    /// Launches the app and signs in at `KIVO_SERVER`, unless still signed in.
    private func signIn() throws {
        let server = try XCTUnwrap(environment["KIVO_SERVER"], "set TEST_RUNNER_KIVO_SERVER")
        let username = try XCTUnwrap(environment["KIVO_USERNAME"], "set TEST_RUNNER_KIVO_USERNAME")
        let password = try XCTUnwrap(environment["KIVO_PASSWORD"], "set TEST_RUNNER_KIVO_PASSWORD")
        app.launch()
        let address = app.textFields["address"]
        let user = app.textFields["username"]
        let home = app.descendants(matching: .any).matching(NSPredicate(format: "label == %@", "Home")).firstMatch
        wait(for: [expectation(for: NSPredicate { _, _ in address.exists || user.exists || home.exists }, evaluatedWith: nil)], timeout: 20)
        guard !home.exists else { return }
        XCTAssert(user.waitForExistence(timeout: 15), "the server at \(server) wasn't found")
        user.tap()
        user.typeText(username)
        let secret = app.secureTextFields["password"]
        secret.tap()
        secret.typeText(password + "\n")
        XCTAssert(home.waitForExistence(timeout: 20), "Home didn't show after signing in")
        let notNow = XCUIApplication(bundleIdentifier: "com.apple.SafariViewService").buttons["Not Now"]
        if notNow.waitForExistence(timeout: 5) { notNow.tap() }
    }

    private func keep(_ name: String) {
        let shot = XCTAttachment(screenshot: XCUIScreen.main.screenshot())
        shot.name = name
        shot.lifetime = .keepAlways
        add(shot)
    }
}
