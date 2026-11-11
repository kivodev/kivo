import XCTest

/// Search against a real server: going to Search puts the
/// cursor in its field over the placeholder, typing finds a book, a result's
/// page and Back find the same query and results without the keyboard, as
/// does another tab and back, and tapping Search on a result's page goes
/// back to the results with the cursor in the field. It reads the
/// server and the user as SignInAndPlayTests does, and `KIVO_SEARCH` for
/// what to type ("name of the rose" by default). It changes nothing on the
/// server.
final class SearchTests: XCTestCase {
    private let app = XCUIApplication()
    private let environment = ProcessInfo.processInfo.environment

    override func setUpWithError() throws {
        continueAfterFailure = false
        // Every run signs in as one client, replacing the last run's session.
        app.launchEnvironment["KIVO_CLIENT_ID"] = "kivo-ui-tests"
        // Sign-in goes straight to the server under test, without browsing the network.
        app.launchEnvironment["KIVO_SERVER"] = environment["KIVO_SERVER"]
    }

    func testSearch() throws {
        let text = environment["KIVO_SEARCH"] ?? "name of the rose"
        try signIn()

        // Search: the field focused, with the keyboard, over the placeholder.
        let tab = app.tabBars.buttons["Search"]
        tab.tap()
        let field = app.searchFields.firstMatch
        XCTAssert(field.waitForExistence(timeout: 10), "Search has no field")
        XCTAssert(app.keyboards.firstMatch.waitForExistence(timeout: 5), "going to Search didn't raise the keyboard")
        XCTAssert(focused(field), "going to Search didn't focus its field")
        XCTAssert(app.staticTexts["Search movies, shows, books and people"].exists, "Search has no placeholder")
        sleep(1)
        keep("ios-search-empty")

        // Typing finds it.
        field.typeText(text)
        let result = app.buttons["work"].firstMatch
        XCTAssert(result.waitForExistence(timeout: 15), "“\(text)” found nothing")
        sleep(2)
        keep("ios-search-results")
        let cards = app.buttons.matching(identifier: "work")
        let labels = cards.allElementsBoundByIndex.map(\.label)

        // A result's page, and Back: the same query and results, no keyboard.
        result.tap()
        let back = app.navigationBars.buttons.element(boundBy: 0)
        XCTAssert(back.waitForExistence(timeout: 10), "the result's page has no Back")
        sleep(2)
        keep("ios-search-result-page")
        back.tap()
        XCTAssert(result.waitForExistence(timeout: 10), "Back didn't return to the results")
        sleep(2)
        XCTAssertEqual(field.value as? String, text, "Back lost the query")
        XCTAssertEqual(cards.allElementsBoundByIndex.map(\.label), labels, "Back changed the results")
        XCTAssertFalse(app.keyboards.firstMatch.exists, "Back raised the keyboard")
        keep("ios-search-back")

        // Another tab and back: the search as it was, no keyboard.
        app.tabBars.buttons["Home"].tap()
        XCTAssert(app.buttons["user"].waitForExistence(timeout: 10), "Home didn't show")
        tab.tap()
        XCTAssert(result.waitForExistence(timeout: 10), "Search lost its results")
        sleep(2)
        XCTAssertEqual(field.value as? String, text, "coming back to Search lost the query")
        XCTAssertFalse(app.keyboards.firstMatch.exists, "coming back to Search raised the keyboard")
        keep("ios-search-return")

        // Tapping Search on a result's page goes back to the results and focuses the field.
        result.tap()
        XCTAssert(back.waitForExistence(timeout: 10), "the result's page has no Back")
        sleep(1)
        tab.tap()
        XCTAssert(app.keyboards.firstMatch.waitForExistence(timeout: 5), "tapping Search again didn't raise the keyboard")
        XCTAssert(focused(field), "tapping Search again didn't focus its field")
        XCTAssertEqual(field.value as? String, text, "tapping Search again lost the query")
        XCTAssertEqual(cards.allElementsBoundByIndex.map(\.label), labels, "tapping Search again didn't go back to the results")
        sleep(1)
        keep("ios-search-retap")
    }

    private func focused(_ element: XCUIElement) -> Bool {
        (element.value(forKey: "hasKeyboardFocus") as? Bool) == true
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

    private func keep(_ name: String) {
        let shot = XCTAttachment(screenshot: XCUIScreen.main.screenshot())
        shot.name = name
        shot.lifetime = .keepAlways
        add(shot)
    }
}
