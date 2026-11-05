import XCTest

/// Choosing Search with nothing typed puts the cursor in its field with
/// the keyboard up, every time: the first time and after each of the other
/// tabs. Choosing it again on a result's page goes back to the results,
/// with the cursor in the tab bar's field on iOS 26. With the keyboard up
/// the tab bar is under it, so the keyboard goes first, by the field's Close. It runs against the download test's
/// server (FIRST-RUN.md, The download test), with the same environment,
/// and changes nothing on it.
final class SearchFocusTests: XCTestCase {
    private let app = XCUIApplication()
    private let environment = ProcessInfo.processInfo.environment

    override func setUpWithError() throws {
        continueAfterFailure = false
        // Every run signs in as one client, replacing the last run's session.
        app.launchEnvironment["KIVO_CLIENT_ID"] = "kivo-ui-tests"
    }

    func testChoosingSearchFocusesItsField() throws {
        try signIn()
        let search = app.tabBars.buttons["Search"]

        chooseSearch(search, "the first time")
        for tab in ["Home", "Libraries", "Downloads", "Home", "Libraries", "Downloads"] {
            // The keyboard covers the tab bar, so it goes first, as a person would put it away.
            putKeyboardAway()
            go(to: tab)
            chooseSearch(search, "after \(tab)")
        }

        // Again while there, on a result's page: back to the results.
        app.searchFields.firstMatch.typeText("Big")
        let result = app.buttons["work"].firstMatch
        XCTAssert(result.waitForExistence(timeout: 15), "“Big” found nothing")
        result.tap()
        XCTAssert(app.navigationBars.buttons.element(boundBy: 0).waitForExistence(timeout: 10), "the result's page has no Back")
        sleep(1)
        search.tap()
        XCTAssert(result.waitForExistence(timeout: 5), "choosing Search again didn't go back to the results")
        if #available(iOS 26, *) { expectFocused("choosing Search again on a result's page") }
        keep("ios-search-focus-again")
    }

    /// Goes to [tab]. On iOS 26 the tab bar stays collapsed to the last
    /// other tab while Search's field shows, so that one comes first.
    private func go(to tab: String) {
        let button = app.tabBars.buttons[tab]
        if !button.exists || button.value as? String == "Collapsed" {
            app.tabBars.buttons.firstMatch.tap()
        }
        if !(button.isSelected) { button.tap() }
        XCTAssert(button.waitForExistence(timeout: 5) && button.isSelected, "\(tab) didn't show")
    }

    /// Puts the keyboard away with the search field's Close (Cancel before iOS 26).
    private func putKeyboardAway() {
        (app.buttons["Close"].exists ? app.buttons["Close"] : app.buttons["Cancel"]).tap()
        let gone = NSPredicate { _, _ in !self.app.keyboards.firstMatch.exists }
        wait(for: [expectation(for: gone, evaluatedWith: nil)], timeout: 5)
    }

    private func chooseSearch(_ search: XCUIElement, _ when: String) {
        XCTAssert(search.waitForExistence(timeout: 10), "there's no Search tab \(when)")
        search.tap()
        expectFocused("choosing Search \(when)")
    }

    /// Asserts that Search's field has the cursor, with the keyboard up.
    private func expectFocused(_ when: String) {
        let field = app.searchFields.firstMatch
        XCTAssert(field.waitForExistence(timeout: 10), "Search has no field after \(when)")
        XCTAssert(app.keyboards.firstMatch.waitForExistence(timeout: 5), "\(when) didn't raise the keyboard")
        let focused = NSPredicate { _, _ in (field.value(forKey: "hasKeyboardFocus") as? Bool) == true }
        wait(for: [expectation(for: focused, evaluatedWith: nil)], timeout: 5)
    }

    /// Launches the app, signed in already or signing in to the server by its typed address.
    private func signIn() throws {
        let server = try XCTUnwrap(environment["KIVO_DOWNLOAD_SERVER"], "set TEST_RUNNER_KIVO_DOWNLOAD_SERVER")
        let username = try XCTUnwrap(environment["KIVO_DOWNLOAD_USERNAME"], "set TEST_RUNNER_KIVO_DOWNLOAD_USERNAME")
        let password = try XCTUnwrap(environment["KIVO_DOWNLOAD_PASSWORD"], "set TEST_RUNNER_KIVO_DOWNLOAD_PASSWORD")
        app.launch()
        let address = app.textFields["address"]
        let user = app.textFields["username"]
        let home = app.buttons["user"]
        let start = NSPredicate { _, _ in address.exists || user.exists || home.exists }
        wait(for: [expectation(for: start, evaluatedWith: nil)], timeout: 20)
        if home.exists { return }
        // Discovery may go straight to another server's password.
        if user.waitForExistence(timeout: 6) { app.buttons["Change server"].tap() }
        XCTAssert(address.waitForExistence(timeout: 10), "sign-in has no address field")
        address.tap()
        address.typeText(server)
        app.buttons["Continue"].tap()
        XCTAssert(user.waitForExistence(timeout: 15), "the server at \(server) wasn't found")
        user.tap()
        user.typeText(username)
        let secret = app.secureTextFields["password"]
        secret.tap()
        secret.typeText(password)
        app.buttons["Sign in"].tap()
        let notNow = XCUIApplication(bundleIdentifier: "com.apple.SafariViewService").buttons["Not Now"]
        if notNow.waitForExistence(timeout: 8) { notNow.tap() }
        XCTAssert(home.waitForExistence(timeout: 20), "Home didn't show after signing in")
    }

    private func keep(_ name: String) {
        let shot = XCTAttachment(screenshot: XCUIScreen.main.screenshot())
        shot.name = name
        shot.lifetime = .keepAlways
        add(shot)
    }
}
