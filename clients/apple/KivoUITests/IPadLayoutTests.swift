import XCTest

/// The iPad's layouts against a real server, with the same environment as
/// SignInAndPlayTests (KIVO_SERVER, KIVO_USERNAME, KIVO_PASSWORD, and
/// KIVO_LIBRARY, Movies by default), on an iPad in landscape with
/// Windowed Apps on (iPadOS 26's default). A full-screen window has the
/// sidebar: each library is a tab with its own pages, a work opens its
/// page and plays and closes from the toolbar, and the View menu's shortcuts
/// change tab. Dragging the window down to a third of the screen gives the
/// iPhone's layout with the page that was showing still there under
/// Libraries, and back to full screen it's under the library's own tab.
/// The window is resized with the system's window controls: the Left tile
/// for a half, then the resize corner.
final class IPadLayoutTests: XCTestCase {
    private let app = XCUIApplication()
    private let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
    private let environment = ProcessInfo.processInfo.environment
    private var library: String { environment["KIVO_LIBRARY"] ?? "Movies" }

    override func setUpWithError() throws {
        continueAfterFailure = false
        try XCTSkipUnless(UIDevice.current.userInterfaceIdiom == .pad, "the iPad's layouts")
        app.launchEnvironment["KIVO_CLIENT_ID"] = "kivo-ui-tests"
        XCUIDevice.shared.orientation = .landscapeLeft
    }

    /// The sidebar's tabs and account, a work from a library's tab, its
    /// player's popover and Close, and the View menu's shortcuts.
    func testTheSidebarAtFullScreen() throws {
        try signIn()
        fill()
        XCTAssert(sidebarItem("Lists").waitForExistence(timeout: 10), "the sidebar has no Lists")
        XCTAssert(sidebarItem(library).exists, "the sidebar has no \(library)")
        XCTAssertFalse(app.buttons["Edit"].exists, "the sidebar has Edit")
        keep("ipad-full-home")

        let account = app.buttons["sidebar-account"]
        XCTAssert(account.waitForExistence(timeout: 5), "the sidebar has no account")
        account.tap()
        XCTAssert(app.navigationBars["Settings"].waitForExistence(timeout: 10), "the sidebar's account didn't open Settings")
        keep("ipad-full-settings")
        app.navigationBars["Settings"].buttons["Done"].tap()

        sidebarItem(library).tap()
        XCTAssert(app.staticTexts[library].waitForExistence(timeout: 10), "\(library)'s tab has no title")
        let card = app.buttons.matching(identifier: "work").firstMatch
        XCTAssert(card.waitForExistence(timeout: 15), "\(library) shows no cards")
        keep("ipad-full-library")
        card.tap()
        let play = app.buttons["play"]
        XCTAssert(play.waitForExistence(timeout: 15), "the title's page has no Play")
        keep("ipad-full-detail")

        play.tap()
        app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
        let close = app.buttons["Close"]
        XCTAssert(close.waitForExistence(timeout: 15), "the player has no Close")
        XCTAssert(close.isHittable, "the player's Close can't be tapped")
        keep("ipad-full-player")

        app.buttons["tracks"].tap()
        let tracks = app.navigationBars["Audio & Subtitles"]
        XCTAssert(tracks.waitForExistence(timeout: 10), "Audio & Subtitles didn't open")
        XCTAssertLessThan(tracks.frame.width, springboard.windows.firstMatch.frame.width / 2, "Audio & Subtitles isn't a popover")
        keep("ipad-full-player-popover")
        tracks.buttons["Done"].tap()
        if !close.isHittable { app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap() }
        close.tap()
        XCTAssert(play.waitForExistence(timeout: 10) && play.isHittable, "Close didn't close the player")

        // The page under the player takes the keyboard again with a tap.
        app.staticTexts.firstMatch.tap()
        app.typeKey("1", modifierFlags: .command)
        XCTAssert(app.buttons["user"].waitForExistence(timeout: 5), "⌘1 didn't go Home")
        app.typeKey("2", modifierFlags: .command)
        XCTAssert(app.staticTexts["Lists"].waitForExistence(timeout: 5), "⌘2 didn't open Lists")
    }

    /// A work's page open in a library's tab stays open as the window
    /// narrows to the iPhone's layout and widens again.
    func testTheOpenPageSurvivesAResize() throws {
        try signIn()
        fill()
        XCTAssert(sidebarItem(library).waitForExistence(timeout: 10), "the sidebar has no \(library)")
        sidebarItem(library).tap()
        let card = app.buttons.matching(identifier: "work").firstMatch
        XCTAssert(card.waitForExistence(timeout: 15), "\(library) shows no cards")
        card.tap()
        let play = app.buttons["play"]
        XCTAssert(play.waitForExistence(timeout: 15), "the title's page has no Play")

        resize(toFraction: 0.33)
        let libraries = app.tabBars.buttons["Libraries"].exists ? app.tabBars.buttons["Libraries"] : app.buttons["Libraries"].firstMatch
        XCTAssert(libraries.waitForExistence(timeout: 10), "a third of the screen has no Libraries tab")
        XCTAssert(libraries.isSelected, "Libraries isn't the tab showing")
        XCTAssert(play.waitForExistence(timeout: 5), "the title's page went away in a narrow window")
        keep("ipad-third-detail")

        // Back is the library, over the list of libraries.
        app.navigationBars.buttons.element(boundBy: 0).tap()
        XCTAssert(app.navigationBars[library].buttons["Libraries"].waitForExistence(timeout: 10), "\(library) isn't over the list of libraries")
        card.tap()
        XCTAssert(play.waitForExistence(timeout: 15), "the title's page has no Play")

        fill()
        XCTAssert(sidebarItem(library).waitForExistence(timeout: 10), "full screen has no sidebar")
        XCTAssert(play.waitForExistence(timeout: 5), "the title's page went away in a wide window")
        keep("ipad-full-again-detail")
    }

    /// A third of the screen is the iPhone's layout: the tab bar with
    /// Libraries, whose list opens a library, Lists, and Search.
    func testAThirdOfTheScreenIsTheIPhoneLayout() throws {
        try signIn()
        resize(toFraction: 0.33)
        let libraries = app.tabBars.buttons["Libraries"].exists ? app.tabBars.buttons["Libraries"] : app.buttons["Libraries"].firstMatch
        XCTAssert(libraries.waitForExistence(timeout: 10), "a third of the screen has no Libraries tab")
        XCTAssertFalse(sidebarItem(library).exists, "a third of the screen has the sidebar's \(library)")
        libraries.tap()
        // The tab opens on the library last opened, over the list of them; choosing it again shows the list.
        let list = app.descendants(matching: .any)["library-list"]
        if !list.waitForExistence(timeout: 5) { libraries.tap() }
        XCTAssert(list.waitForExistence(timeout: 10), "the Libraries tab has no list")
        app.descendants(matching: .any)["library:\(library)"].firstMatch.tap()
        XCTAssert(app.navigationBars[library].waitForExistence(timeout: 10), "\(library) didn't open")

        app.buttons["Lists"].firstMatch.tap()
        XCTAssert(app.navigationBars["Lists"].waitForExistence(timeout: 10), "Lists didn't open")
        keep("ipad-third-lists")

        app.buttons["Search"].firstMatch.tap()
        let field = app.searchFields.firstMatch
        XCTAssert(field.waitForExistence(timeout: 10), "Search has no field")
        field.typeText(String(library.prefix(1)))
        XCTAssert(app.buttons.matching(identifier: "work").firstMatch.waitForExistence(timeout: 15), "Search found nothing")
        keep("ipad-third-search")
    }

    /// A series' seasons at full screen are pills over the open season's
    /// name and its check, which asks before marking the season watched; a
    /// third of the screen has the season menu instead. The series is
    /// `KIVO_SERIES` (Severance by default), found by Search. It changes
    /// nothing on the server.
    func testSeasonPillsAtFullScreen() throws {
        let series = environment["KIVO_SERIES"] ?? "Severance"
        try signIn()
        fill()
        sidebarItem("Search").tap()
        let field = app.searchFields.firstMatch
        XCTAssert(field.waitForExistence(timeout: 10), "Search has no field")
        field.tap()
        // Return puts the keyboard away, which would cover a lower result.
        field.typeText(series + "\n")
        let result = app.buttons.matching(NSPredicate(format: "identifier == %@ AND (label == %@ OR label BEGINSWITH %@)", "work", series, series + ",")).firstMatch
        XCTAssert(result.waitForExistence(timeout: 15), "Search didn't find \(series)")
        // The results settle once the last letter's search is back.
        sleep(2)
        result.tap()

        let pills = app.buttons.matching(identifier: "season-pill")
        XCTAssert(app.buttons["play"].waitForExistence(timeout: 20), "\(series)'s page has no Play")
        // Under a long overview the seasons start below the fold.
        if !pills.firstMatch.waitForExistence(timeout: 5) { app.swipeUp() }
        XCTAssert(pills.firstMatch.waitForExistence(timeout: 10), "\(series) has no season pills at full screen")
        XCTAssertFalse(app.buttons["season"].exists, "full screen has the season menu")
        let check = app.buttons["season-watched"]
        XCTAssert(check.exists, "the season has no check")
        XCTAssertEqual(pills.allElementsBoundByIndex.filter(\.isSelected).count, 1, "not one season pill is selected")
        sleep(2)
        keep("ipad-full-seasons")

        let other = try XCTUnwrap(pills.allElementsBoundByIndex.first { !$0.isSelected && $0.isHittable }, "no other season pill in view")
        let name = other.label
        other.tap()
        XCTAssert(app.staticTexts[name].waitForExistence(timeout: 5), "the heading isn't \(name)")
        XCTAssert(other.isSelected, "the tapped pill isn't selected")
        sleep(2)
        keep("ipad-full-seasons-other")

        if check.label == "Mark season as watched" {
            check.tap()
            let question = app.staticTexts.matching(NSPredicate(format: "label ENDSWITH %@", "episodes as watched?")).firstMatch
            XCTAssert(question.waitForExistence(timeout: 5), "the check didn't ask first")
            keep("ipad-full-seasons-confirm")
            app.otherElements["PopoverDismissRegion"].firstMatch.tap()
            XCTAssert(question.waitForNonExistence(timeout: 5), "the confirmation stayed open")
        }

        // A shelf: the episodes side by side, in one row, under the seasons.
        let episodes = app.buttons.matching(identifier: "episode")
        if !episodes.firstMatch.waitForExistence(timeout: 5) { app.swipeUp() }
        XCTAssert(episodes.firstMatch.waitForExistence(timeout: 10), "the season has no episodes")
        let shelf = episodes.allElementsBoundByIndex.prefix(2)
        if shelf.count == 2 {
            XCTAssertEqual(shelf.first?.frame.minY, shelf.last?.frame.minY, "full screen's episodes aren't a shelf")
        }
        sleep(2)
        keep("ipad-full-episodes")
        // Back up to the seasons, which a third of the screen makes a menu.
        app.swipeDown()

        resize(toFraction: 0.33)
        XCTAssert(app.buttons["season"].waitForExistence(timeout: 10), "a third of the screen has no season menu")
        XCTAssertFalse(pills.firstMatch.exists, "a third of the screen has season pills")
        // A list: the episodes one above another.
        if !episodes.firstMatch.waitForExistence(timeout: 5) { app.swipeUp() }
        let rows = episodes.allElementsBoundByIndex.prefix(2)
        if rows.count == 2, let first = rows.first, let second = rows.last {
            XCTAssertLessThan(first.frame.minY, second.frame.minY, "a third of the screen's episodes aren't a list")
        }
        keep("ipad-third-seasons")
    }

    // MARK: Steps

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
        // Discovery takes up to 3 seconds, and may go straight to another
        // server's password; then it's the typed address.
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
        // iPadOS's offer to save the password goes away with the next tap
        // elsewhere, so it's left alone.
    }

    /// A tab in the sidebar, which shows as a cell or a button.
    private func sidebarItem(_ name: String) -> XCUIElement {
        let cell = app.cells.matching(NSPredicate(format: "label == %@", name)).firstMatch
        return cell.exists ? cell : app.buttons.matching(NSPredicate(format: "label == %@ AND NOT (identifier IN %@)", name, ["work"])).firstMatch
    }

    // MARK: Windows

    /// Full screen, from the window controls' Fill.
    private func fill() {
        windowControls(choose: "Fill")
        sleep(2)
    }

    /// The Left tile, then the window's resize corner dragged until the
    /// window is about [fraction] of the screen's width.
    private func resize(toFraction fraction: CGFloat) {
        windowControls(choose: "Left")
        sleep(2)
        let target = (springboard.windows.firstMatch.frame.width * fraction).rounded()
        for _ in 0..<4 {
            let frame = app.windows.firstMatch.frame
            if abs(frame.width - target) < 24 { break }
            let origin = springboard.coordinate(withNormalizedOffset: .zero)
            let corner = origin.withOffset(CGVector(dx: frame.maxX - 14, dy: frame.maxY - 14))
            corner.press(forDuration: 0.4, thenDragTo: origin.withOffset(CGVector(dx: frame.maxX - 14 - (frame.width - target), dy: frame.maxY - 14)))
            sleep(2)
        }
    }

    /// Chooses [item] in the window controls' menu. A full-screen window
    /// shows its controls once its resize corner has been dragged in.
    private func windowControls(choose item: String) {
        let controls = springboard.buttons.matching(NSPredicate(format: "identifier BEGINSWITH %@ AND label ENDSWITH %@", "window-controls:", ", Kivo")).firstMatch
        if !controls.waitForExistence(timeout: 3) {
            let screen = springboard.windows.firstMatch.frame
            let origin = springboard.coordinate(withNormalizedOffset: .zero)
            origin.withOffset(CGVector(dx: screen.maxX - 14, dy: screen.maxY - 14))
                .press(forDuration: 0.4, thenDragTo: origin.withOffset(CGVector(dx: screen.maxX - 300, dy: screen.maxY - 14)))
            sleep(2)
        }
        XCTAssert(controls.waitForExistence(timeout: 5), "the window has no window controls: is Windowed Apps on?")
        controls.press(forDuration: 1)
        let button = springboard.buttons[item]
        XCTAssert(button.waitForExistence(timeout: 3), "the window controls have no \(item)")
        button.tap()
    }

    private func keep(_ name: String) {
        let shot = XCTAttachment(screenshot: XCUIScreen.main.screenshot())
        shot.name = name
        shot.lifetime = .keepAlways
        add(shot)
    }
}
