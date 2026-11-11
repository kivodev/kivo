import XCTest

/// The book player against a real server (FIRST-RUN.md): a book from the
/// audiobooks library streams from its page, its mini player shows and
/// keeps playing on another tab, and Now Playing changes the speed and puts
/// it back, jumps from the chapter list, sets a sleep timer and turns it
/// off, goes to the next chapter, shows in the light theme too, and stops.
/// It reads the server and the user as SignInAndPlayTests does, and
/// `KIVO_BOOKS` for the library (Audiobooks by default). It changes nothing
/// on the server but the book's listening position.
final class BookPlayerTests: XCTestCase {
    private let app = XCUIApplication()
    private let environment = ProcessInfo.processInfo.environment

    override func setUpWithError() throws {
        continueAfterFailure = false
        // Every run signs in as one client, replacing the last run's session.
        app.launchEnvironment["KIVO_CLIENT_ID"] = "kivo-ui-tests"
        // Sign-in goes straight to the server under test, without browsing the network.
        app.launchEnvironment["KIVO_SERVER"] = environment["KIVO_SERVER"]
    }

    func testListening() throws {
        try signIn()
        openBook()
        keep("ios-book-detail")

        // Play streams it: the mini player shows, playing.
        app.buttons["play"].tap()
        let mini = app.descendants(matching: .any)["miniPlayer"]
        XCTAssert(mini.waitForExistence(timeout: 20), "the mini player didn't show")
        let pause = app.buttons["miniPlay"]
        XCTAssert(pause.waitForExistence(timeout: 10), "the mini player has no Play or Pause")
        wait(for: [expectation(for: NSPredicate(format: "label == 'Pause'"), evaluatedWith: pause)], timeout: 30)
        sleep(2)
        keep("ios-book-mini")

        // It keeps playing on another tab.
        app.tabBars.buttons["Home"].tap()
        sleep(4)
        XCTAssert(mini.exists, "the mini player went away on Home")
        XCTAssert(app.buttons.matching(identifier: "miniPlay").allElementsBoundByIndex.contains { $0.label == "Pause" }, "the book stopped on another tab")
        keep("ios-book-mini-home")

        // Now Playing.
        openNowPlaying()
        let title = app.staticTexts["nowPlayingTitle"]
        XCTAssert(title.waitForExistence(timeout: 10), "Now Playing didn't open")
        sleep(2)
        keep("ios-book-full-dark")

        // Speed: a quick list; the user's speed is put back at the end.
        let speed = app.buttons["speed"]
        let original = speed.label.components(separatedBy: " ").last ?? "1×"
        speed.tap()
        let faster = original == "1.5×" ? "1.25×" : "1.5×"
        let userSpeed = "1×"
        XCTAssert(app.buttons[faster].waitForExistence(timeout: 5), "the speed list didn't open")
        keep("ios-book-speed")
        tapWhenHittable(app.buttons[faster])
        wait(for: [expectation(for: NSPredicate(format: "label ENDSWITH %@", " " + faster), evaluatedWith: speed)], timeout: 5)

        // Chapters: the list, and a jump.
        let chapterTitle = app.staticTexts["chapterTitle"]
        app.buttons["chapters"].tap()
        let rows = app.buttons.matching(identifier: "chapterRow")
        XCTAssert(rows.firstMatch.waitForExistence(timeout: 5), "the chapter list didn't open")
        sleep(1)
        keep("ios-book-chapters")
        let target = rows.element(boundBy: min(2, rows.count - 1))
        let targetName = target.label
        target.tap()
        XCTAssert(title.waitForExistence(timeout: 5), "the chapter list didn't close")
        sleep(2)
        XCTAssert(targetName.contains(chapterTitle.label), "the jump didn't reach \(targetName): \(chapterTitle.label)")

        // Sleep: 15 minutes counts down on the button, then Off.
        let sleepButton = app.buttons["sleep"]
        sleepButton.tap()
        XCTAssert(app.buttons["15 minutes"].waitForExistence(timeout: 5), "the sleep menu didn't open")
        keep("ios-book-sleep")
        tapWhenHittable(app.buttons["15 minutes"])
        sleep(3)
        keep("ios-book-sleep-running")
        sleepButton.tap()
        XCTAssert(app.buttons["Off"].waitForExistence(timeout: 5), "a running timer offers no Off")
        tapWhenHittable(app.buttons["Off"])

        // Next chapter, once the sleep menu has closed off it.
        let before = chapterTitle.label
        let next = app.buttons["nextChapter"]
        wait(for: [expectation(for: NSPredicate(format: "isHittable == true"), evaluatedWith: next)], timeout: 5)
        next.tap()
        sleep(2)
        XCTAssertNotEqual(chapterTitle.label, before, "Next chapter didn't move")
        keep("ios-book-next")

        // The user's speed as it was.
        speed.tap()
        XCTAssert(app.buttons[userSpeed].waitForExistence(timeout: 5), "the speed list didn't open again")
        tapWhenHittable(app.buttons[userSpeed])
        sleep(1)

        // Closed with a swipe down; the light theme from Settings.
        app.swipeDown(velocity: .fast)
        XCTAssert(mini.waitForExistence(timeout: 5), "closing Now Playing lost the mini player")
        setTheme("Light")
        openNowPlaying()
        XCTAssert(title.waitForExistence(timeout: 10), "Now Playing didn't open again")
        sleep(2)
        keep("ios-book-full-light")
        app.swipeDown(velocity: .fast)
        sleep(1)
        keep("ios-book-mini-light")
        setTheme("Dark")

        // Stop listening saves the place; the mini player goes.
        openNowPlaying()
        XCTAssert(title.waitForExistence(timeout: 10), "Now Playing didn't open to stop")
        app.buttons["nowPlayingMore"].tap()
        tapWhenHittable(app.buttons["Stop listening"])
        wait(for: [expectation(for: NSPredicate(format: "exists == false"), evaluatedWith: mini)], timeout: 10)
        sleep(3)
    }

    /// A tap on the mini player's title, beside its buttons.
    private func openNowPlaying() {
        // XCUITest gives the tab bar accessory's views no steady frame: tap just above the tab bar, on the title.
        let bar = app.tabBars.firstMatch.frame
        app.coordinate(withNormalizedOffset: .zero).withOffset(CGVector(dx: bar.minX + bar.width * 0.4, dy: bar.minY - 30)).tap()
    }

    /// A book of the audiobooks library: `KIVO_BOOK` by its title, else the first.
    private func openBook() {
        let library = environment["KIVO_BOOKS"] ?? "Audiobooks"
        app.tabBars.buttons["Libraries"].tap()
        // The tab opens on the library last opened, over the list of them; choosing it again shows the list.
        let list = app.descendants(matching: .any)["library-list"]
        if !list.waitForExistence(timeout: 5) { app.tabBars.buttons["Libraries"].tap() }
        XCTAssert(list.waitForExistence(timeout: 10), "the Libraries tab has no list")
        let row = app.descendants(matching: .any)["library:\(library)"].firstMatch
        XCTAssert(row.waitForExistence(timeout: 5), "the list has no \(library)")
        row.tap()
        if app.buttons["chip-titles"].waitForExistence(timeout: 10) { app.buttons["chip-titles"].tap() }
        XCTAssert(app.buttons["work"].firstMatch.waitForExistence(timeout: 20), "\(library)'s grid has no cards")
        sleep(2)
        keep("ios-book-library")
        let wanted = environment["KIVO_BOOK"].map { app.buttons.matching(identifier: "work").matching(NSPredicate(format: "label BEGINSWITH %@", $0)).firstMatch }
        (wanted?.exists == true ? wanted! : app.buttons["work"].firstMatch).tap()
        XCTAssert(app.buttons["play"].waitForExistence(timeout: 20), "the book's page has no Play")
        sleep(2)
    }

    /// Settings › Appearance › Theme, from Home's user menu.
    private func setTheme(_ theme: String) {
        app.tabBars.buttons["Home"].tap()
        let user = app.buttons["user"]
        XCTAssert(user.waitForExistence(timeout: 10), "Home has no user button")
        user.tap()
        XCTAssert(app.buttons["Playback"].waitForExistence(timeout: 10), "Settings didn't open")
        let appearance = app.buttons["Appearance"]
        for _ in 0 ..< 6 where !(appearance.exists && appearance.isHittable) { app.swipeUp() }
        XCTAssert(appearance.exists, "Settings has no Appearance")
        appearance.tap()
        let picker = app.buttons["Theme"]
        XCTAssert(picker.waitForExistence(timeout: 5), "Appearance has no Theme")
        picker.tap()
        app.buttons[theme].firstMatch.tap()
        sleep(1)
        app.navigationBars.buttons["Settings"].tap()
        app.buttons["Done"].tap()
        sleep(1)
    }

    /// Launches the app, signed in already or signing in.
    private func signIn() throws {
        let server = try XCTUnwrap(environment["KIVO_SERVER"], "set TEST_RUNNER_KIVO_SERVER")
        let username = try XCTUnwrap(environment["KIVO_USERNAME"], "set TEST_RUNNER_KIVO_USERNAME")
        let password = try XCTUnwrap(environment["KIVO_PASSWORD"], "set TEST_RUNNER_KIVO_PASSWORD")
        app.launch()
        let address = app.textFields["address"]
        let user = app.textFields["username"]
        let home = app.buttons["user"]
        let start = NSPredicate { _, _ in address.exists || user.exists || home.exists }
        wait(for: [expectation(for: start, evaluatedWith: nil)], timeout: 20)
        if !home.exists {
            XCTAssert(user.waitForExistence(timeout: 15), "the server at \(server) wasn't found")
            user.tap()
            user.typeText(username)
            let secret = app.secureTextFields["password"]
            secret.tap()
            secret.typeText(password)
            app.buttons["Sign in"].tap()
            let notNow = XCUIApplication(bundleIdentifier: "com.apple.SafariViewService").buttons["Not Now"]
            if notNow.waitForExistence(timeout: 8) { notNow.tap() }
        }
        XCTAssert(home.waitForExistence(timeout: 20), "Home didn't show")
    }

    /// Taps a menu's item once its menu has opened.
    private func tapWhenHittable(_ element: XCUIElement) {
        XCTAssert(element.waitForExistence(timeout: 5), "\(element) didn't show")
        sleep(1)
        element.tap()
    }

    private func keep(_ name: String) {
        let shot = XCTAttachment(screenshot: XCUIScreen.main.screenshot())
        shot.name = name
        shot.lifetime = .keepAlways
        add(shot)
    }
}
