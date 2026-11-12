import XCTest

/// Switching libraries in the Libraries tab on an iPhone: the list of
/// libraries pushes each and is there again on choosing the tab. It also
/// times opening a library from the list by the app's signposts: from the
/// choice to the library's title, and to its first cards, switching back
/// and forth between `KIVO_LIBRARY` and `KIVO_OTHER_LIBRARY` (Movies and
/// TV by default), each seen once first, so it times libraries seen lately.
/// It signs in as SignInAndPlayTests does and changes nothing on the
/// server. The baselines are in Kivo.xcodeproj/xcshareddata/xcbaselines.
final class LibrarySwitchTests: XCTestCase {
    private let app = XCUIApplication()
    private let environment = ProcessInfo.processInfo.environment
    private var libraries: [String] { [environment["KIVO_LIBRARY"] ?? "Movies", environment["KIVO_OTHER_LIBRARY"] ?? "TV"] }
    private var librariesTab: XCUIElement { app.tabBars.buttons["Libraries"] }

    override func setUpWithError() throws {
        continueAfterFailure = false
        try XCTSkipUnless(UIDevice.current.userInterfaceIdiom == .phone, "the iPhone's Libraries tab")
    }

    /// The list of libraries, each opening its page with
    /// Back to the list; choosing the tab again goes back to the list, and
    /// the app opens again on the page last opened.
    func testTheList() throws {
        let library = libraries[0]
        try signIn(app)
        librariesTab.tap()
        let list = app.descendants(matching: .any)["library-list"]
        // The tab opens on the page last opened; choosing it again goes back to the list.
        if !list.waitForExistence(timeout: 10) { librariesTab.tap() }
        XCTAssert(list.waitForExistence(timeout: 10), "the Libraries tab has no list")
        XCTAssert(app.navigationBars["Libraries"].exists, "the list isn't titled Libraries")
        sleep(1)
        keep("list")

        row(library).tap()
        expectPage(library)
        XCTAssert(app.navigationBars[library].buttons["Libraries"].exists, "\(library) has no Back to the list")
        sleep(1)
        keep("list-library-top")
        app.swipeUp()
        sleep(1)
        keep("list-library-scrolled")

        librariesTab.tap()
        XCTAssert(list.waitForExistence(timeout: 5), "choosing the tab again didn't go back to the list")

        row(library).tap()
        expectPage(library)
        app.terminate()
        app.launch()
        XCTAssert(librariesTab.waitForExistence(timeout: 20), "the app didn't open signed in")
        librariesTab.tap()
        expectPage(library)
        XCTAssert(app.navigationBars[library].buttons["Libraries"].exists, "the app didn't open \(library) over the list")

        app.navigationBars[library].buttons["Libraries"].tap()
        XCTAssert(list.waitForExistence(timeout: 10), "\(library) didn't go back to the list")
    }

    /// How quickly opening a library from the list shows it.
    func testSwitchingLibraries() throws {
        try signIn(app)
        librariesTab.tap()
        for library in libraries { choose(library) }

        let metrics = ["Switch title", "Switch cards"].map {
            XCTOSSignpostMetric(subsystem: "com.kivo.player", category: "PointsOfInterest", name: $0)
        }
        let options = XCTMeasureOptions()
        options.iterationCount = 6
        var next = 0
        measure(metrics: metrics, options: options) {
            choose(libraries[next])
            next = 1 - next
        }
    }

    // MARK: Steps

    /// A row of the list.
    private func row(_ name: String) -> XCUIElement {
        app.descendants(matching: .any)["library:\(name)"].firstMatch
    }

    /// Goes Back to the list, from the page open over it if any, then
    /// chooses [library] and waits for its title and cards.
    private func choose(_ library: String) {
        let list = app.descendants(matching: .any)["library-list"]
        let back = app.navigationBars.buttons["Libraries"].firstMatch
        let shown = NSPredicate { _, _ in list.exists || back.exists }
        wait(for: [expectation(for: shown, evaluatedWith: nil)], timeout: 20)
        if !list.exists { back.tap() }
        let item = row(library)
        XCTAssert(item.waitForExistence(timeout: 5), "the list has no \(library)")
        item.tap()
        expectPage(library)
        // The first cards' artwork settles before the next switch.
        sleep(1)
    }

    /// [library]'s page shows, with its title and cards.
    private func expectPage(_ library: String) {
        let shown = NSPredicate { _, _ in self.app.navigationBars[library].exists && self.app.buttons["work"].firstMatch.exists }
        wait(for: [expectation(for: shown, evaluatedWith: nil)], timeout: 20)
    }

}
