import XCTest

/// Search against a real server: going to Search puts the
/// cursor in its field over the placeholder, typing finds a book, a result's
/// page and Back find the same query and results without the keyboard, as
/// does another tab and back, and tapping Search on a result's page goes
/// back to the results with the cursor in the field. Choosing Search also
/// focuses its field after each other tab, with the keyboard put away first
/// by the field's Close, since it covers the tab bar. It reads the
/// server and the user as SignInAndPlayTests does, and `KIVO_SEARCH` for
/// what to type ("name of the rose" by default). It changes nothing on the
/// server.
final class SearchTests: XCTestCase {
    private let app = XCUIApplication()
    private let environment = ProcessInfo.processInfo.environment

    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    func testSearch() throws {
        let text = environment["KIVO_SEARCH"] ?? "name of the rose"
        try signIn(app)

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

    /// Choosing Search with nothing typed focuses its field every time: the
    /// first time and after each of the other tabs, twice.
    func testChoosingSearchFocusesItsField() throws {
        try signIn(app)
        let search = app.tabBars.buttons["Search"]

        chooseSearch(search, "the first time")
        for tab in ["Home", "Libraries", "Downloads", "Home", "Libraries", "Downloads"] {
            // The keyboard covers the tab bar, so it goes first, as a person would put it away.
            putKeyboardAway()
            go(to: tab)
            chooseSearch(search, "after \(tab)")
        }
    }

    private func focused(_ element: XCUIElement) -> Bool {
        (element.value(forKey: "hasKeyboardFocus") as? Bool) == true
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

    /// Chooses [search] and waits for its field to have the cursor, with the keyboard up.
    private func chooseSearch(_ search: XCUIElement, _ when: String) {
        XCTAssert(search.waitForExistence(timeout: 10), "there's no Search tab \(when)")
        search.tap()
        let field = app.searchFields.firstMatch
        XCTAssert(field.waitForExistence(timeout: 10), "Search has no field after choosing it \(when)")
        XCTAssert(app.keyboards.firstMatch.waitForExistence(timeout: 5), "choosing Search \(when) didn't raise the keyboard")
        let focused = NSPredicate { _, _ in self.focused(field) }
        wait(for: [expectation(for: focused, evaluatedWith: nil)], timeout: 5)
    }
}
