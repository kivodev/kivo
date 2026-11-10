import XCTest

/// Drives the app against a real server: sign in, Home, a library chosen
/// from the Libraries tab's list with its chips, sort menu, letter scrubber
/// and a card's long press menu, a movie's page and a few seconds of its
/// playback, a series with its season menu and episodes, an episode's page,
/// Downloads, and Settings with a pushed choice. The server and the user
/// come from the environment, which `xcodebuild test` passes in with the
/// `TEST_RUNNER_` prefix (FIRST-RUN.md):
///
///     TEST_RUNNER_KIVO_SERVER=192.168.1.10 TEST_RUNNER_KIVO_USERNAME=… \
///     TEST_RUNNER_KIVO_PASSWORD=… [TEST_RUNNER_KIVO_LIBRARY=Movies] \
///     [TEST_RUNNER_KIVO_SHOWS=TV] xcodebuild test …
///
/// Each step's screen is kept as an attachment in the result bundle. It
/// changes nothing on the server but the movie's resume position.
final class SignInAndPlayTests: XCTestCase {
    private let app = XCUIApplication()
    private let environment = ProcessInfo.processInfo.environment

    override func setUpWithError() throws {
        continueAfterFailure = false
        // Every run signs in as one client, replacing the last run's session.
        app.launchEnvironment["KIVO_CLIENT_ID"] = "kivo-ui-tests"
    }

    func testSignInBrowseAndPlay() throws {
        let library = environment["KIVO_LIBRARY"] ?? "Movies"
        let shows = environment["KIVO_SHOWS"] ?? "TV"
        XCUIDevice.shared.orientation = .portrait
        let home = try signIn(showingSteps: true)

        XCTAssert(home.waitForExistence(timeout: 20), "Home didn't show")
        XCTAssert(app.buttons["work"].firstMatch.waitForExistence(timeout: 20), "Home has no cards")
        sleep(2)
        keep("home")

        // Libraries opens on the last library; its name in the top bar is the menu of the others.
        open(library: library)
        let card = app.buttons["work"].firstMatch
        XCTAssert(app.buttons["chip-titles"].exists, "\(library) has no section chips")
        XCTAssert(app.buttons["count-sort"].exists, "\(library) has no count with its sort")
        let scrubber = app.descendants(matching: .any)["scrubber"]
        XCTAssert(scrubber.waitForExistence(timeout: 10), "\(library) has no letter scrubber")
        sleep(2)
        keep("library")

        // The count line's sort, a menu of the sorts with their directions;
        // a row's label is both, "Date added, Newest first".
        app.buttons["count-sort"].tap()
        let dateAdded = app.buttons.matching(NSPredicate(format: "label BEGINSWITH 'Date added'")).firstMatch
        XCTAssert(dateAdded.waitForExistence(timeout: 5), "the sort menu didn't open")
        keep("sort-menu")
        dismissMenu()

        // A card's long press menu.
        XCTAssert(card.waitForExistence(timeout: 10), "\(library)'s grid has no cards")
        card.press(forDuration: 1.2)
        XCTAssert(app.buttons["Mark as watched"].waitForExistence(timeout: 5) || app.buttons["Mark as unwatched"].exists, "the long press menu didn't open")
        keep("context-menu")
        dismissMenu()

        // The scrubber mid-drag: the bubble follows the finger. The screen
        // is kept from another thread while the finger holds still.
        let from = scrubber.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.15))
        let to = scrubber.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.55))
        let midDrag = expectation(description: "mid-drag screenshot")
        DispatchQueue.global().asyncAfter(deadline: .now() + 2.5) {
            let shot = XCTAttachment(screenshot: XCUIScreen.main.screenshot())
            shot.name = "scrubber-drag"
            shot.lifetime = .keepAlways
            self.add(shot)
            midDrag.fulfill()
        }
        from.press(forDuration: 0.1, thenDragTo: to, withVelocity: 300, thenHoldForDuration: 3)
        wait(for: [midDrag], timeout: 10)
        sleep(1)
        keep("library-scrubbed")

        // A movie's page, from a card the scrubber brought into view.
        let movie = try XCTUnwrap(
            app.buttons.matching(identifier: "work").allElementsBoundByIndex.first { $0.frame.minY > 140 && $0.isHittable },
            "no card in view after scrubbing"
        )
        movie.tap()
        let play = app.buttons["play"]
        XCTAssert(play.waitForExistence(timeout: 20), "the detail page has no Play")
        sleep(2)
        keep("movie")
        app.swipeUp()
        sleep(1)
        keep("movie-below")
        app.swipeDown()

        // A few seconds of playback.
        play.tap()
        // Playing: the controls show Pause once frames move, then hide, and a tap away from them brings them back.
        // Upright they fade under the video and stay in the tree, so what
        // tells is whether Pause is drawn on the black there.
        let pause = app.buttons["Pause"]
        XCTAssert(pause.waitForExistence(timeout: 30), "playback didn't start")
        let pauseSpot = CGPoint(x: pause.frame.midX, y: pause.frame.midY)
        XCTAssert(waitUntil(15) { !drawn(at: pauseSpot) }, "the controls didn't hide while playing")
        keep("playing")
        app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.3)).tap()
        XCTAssert(waitUntil(5) { drawn(at: pauseSpot) } && pause.exists, "playback stopped by itself")
        // Paused, the controls stay up for Close.
        let close = app.buttons["Close"]
        let gone = NSPredicate(format: "exists == false")
        pause.tap()
        wait(for: [expectation(for: gone, evaluatedWith: pause)], timeout: 5)
        sleep(4)
        XCTAssert(drawn(at: CGPoint(x: close.frame.midX, y: close.frame.midY)), "the controls hid while paused")
        keep("paused")
        close.tap()
        wait(for: [expectation(for: NSPredicate(format: "isHittable == true"), evaluatedWith: play)], timeout: 10)
        sleep(2)
        keep("stopped")
        app.navigationBars.buttons.firstMatch.tap()

        // A series: Play Next Up, the season menu and the episodes.
        open(library: shows)
        if app.buttons["chip-titles"].exists { app.buttons["chip-titles"].tap() }
        let series = app.buttons["work"].firstMatch
        XCTAssert(series.waitForExistence(timeout: 20), "\(shows)'s grid has no cards")
        series.tap()
        let season = app.buttons["season"]
        XCTAssert(season.waitForExistence(timeout: 20), "the series has no season menu")
        sleep(2)
        keep("series")
        season.tap()
        sleep(1)
        keep("season-menu")
        dismissMenu()
        app.swipeUp()
        sleep(1)
        keep("series-episodes")
        // An episode row's swipe action, revealed and left alone.
        let rows = app.buttons.matching(identifier: "episode").allElementsBoundByIndex
        if let episode = rows.first(where: { $0.frame.minY > 200 && $0.isHittable }) {
            let row = episode.coordinate(withNormalizedOffset: CGVector(dx: 0.2, dy: 0.5))
            row.press(forDuration: 0.01, thenDragTo: row.withOffset(CGVector(dx: 110, dy: 0)), withVelocity: .slow, thenHoldForDuration: 0.3)
            sleep(1)
            keep("episode-swipe")
            // Closes the swipe action without taking it.
            app.coordinate(withNormalizedOffset: CGVector(dx: 0.9, dy: 0.5)).tap()
            sleep(1)
            let open = app.buttons.matching(identifier: "episode").allElementsBoundByIndex.first { $0.frame.minY > 200 && $0.isHittable }
            try XCTUnwrap(open, "no episode row in view").tap()
            XCTAssert(app.buttons["play"].waitForExistence(timeout: 20), "the episode's page has no Play")
            sleep(2)
            keep("episode")
        }

        // A filter from the menu: a dot on the button and a chip that turns it off.
        open(library: library)
        app.buttons["filter"].tap()
        XCTAssert(app.buttons["Unwatched"].waitForExistence(timeout: 5), "the filter menu has no Unwatched")
        keep("filter-menu")
        app.buttons["Unwatched"].tap()
        // Choosing may close the menu, and a tap outside it then would be Back.
        if !app.buttons["Unwatched"].waitForNonExistence(timeout: 2) { dismissMenu() }
        XCTAssert(app.buttons["filter-played"].waitForExistence(timeout: 5), "an active filter shows no chip")
        sleep(2)
        keep("library-filtered")
        app.buttons["filter-played"].tap()
        XCTAssert(app.buttons["count-sort"].waitForExistence(timeout: 10), "the filter didn't come off")

        // Coming back from a page keeps the grid where it was.
        app.swipeUp()
        app.swipeUp()
        sleep(2)
        let inView = app.buttons.matching(identifier: "work").allElementsBoundByIndex.first { $0.frame.minY > 140 && $0.isHittable }
        let shown = try XCTUnwrap(inView, "no card in view after scrolling")
        let label = shown.label
        let top = shown.frame.minY
        shown.tap()
        XCTAssert(app.buttons["play"].waitForExistence(timeout: 20), "the card didn't open its page")
        app.navigationBars.buttons.firstMatch.tap()
        let back = app.buttons.matching(identifier: "work").matching(NSPredicate(format: "label == %@", label)).firstMatch
        XCTAssert(back.waitForExistence(timeout: 10), "the card isn't in the grid after coming back")
        XCTAssertEqual(back.frame.minY, top, accuracy: 4, "coming back moved the grid")
        keep("library-returned")

        // An audiobooks library: Books, Authors and Series, and a series' and an author's pages.
        if let books = environment["KIVO_BOOKS"] ?? Optional("Audiobooks") {
            open(library: books)
            app.buttons["chip-titles"].tap()
            sleep(2)
            keep("books")
            app.buttons["chip-bookSeries"].tap()
            sleep(3)
            keep("books-series")
            app.buttons["work"].firstMatch.tap()
            sleep(3)
            keep("book-series-page")
            app.navigationBars.buttons.firstMatch.tap()
            app.buttons["chip-authors"].tap()
            sleep(3)
            keep("books-authors")
            app.buttons["work"].firstMatch.tap()
            sleep(3)
            keep("author-page")
            app.navigationBars.buttons.firstMatch.tap()
            app.buttons["chip-titles"].tap()
        }

        // Downloads.
        app.tabBars.buttons["Downloads"].tap()
        XCTAssert(app.staticTexts["Downloads"].waitForExistence(timeout: 5), "Downloads didn't open")
        keep("downloads")

        // Settings from Home's avatar, Audio, and a choice pushed as its own list.
        app.tabBars.buttons["Home"].tap()
        XCTAssert(home.waitForExistence(timeout: 10), "Home didn't come back")
        home.tap()
        let audioSettings = app.buttons["Audio"]
        XCTAssert(audioSettings.waitForExistence(timeout: 10), "Settings has no Audio")
        sleep(1)
        keep("settings")
        audioSettings.tap()
        // The row's label is the setting and its choice.
        let audio = app.buttons.matching(NSPredicate(format: "label BEGINSWITH 'Audio language'")).firstMatch
        XCTAssert(audio.waitForExistence(timeout: 10), "Audio has no Audio language")
        keep("settings-audio")
        audio.tap()
        sleep(1)
        keep("settings-picker")
        app.navigationBars.buttons.firstMatch.tap()
        app.navigationBars.buttons.firstMatch.tap()
        app.buttons["Done"].tap()

        // The app ends with the test: give the play's stop time to reach the server.
        sleep(3)
    }

    /// The player: its controls stay up while a sheet is open, while paused
    /// and after any input, and hide 3 seconds after the last one while
    /// playing; Audio & Subtitles chooses a subtitle and Off again, and
    /// switches Dialogue boost on and off, each a new play; Playback
    /// settings lists Audio & Subtitles, Quality, Speed and Stream info in
    /// that order and pushes each but Quality; the scrub bar's bubble and a
    /// double tap's skip. It plays the library's first work, so it moves
    /// that work's resume position on.
    func testPlayerControls() throws {
        // The controls over the video, as they are with the phone turned.
        XCUIDevice.shared.orientation = .landscapeLeft
        defer { XCUIDevice.shared.orientation = .portrait }
        try signIn()
        let library = environment["KIVO_LIBRARY"] ?? "Movies"
        // Its steps take a few minutes, which a short video ends before.
        if let video = environment["KIVO_CONTROLS_VIDEO"] {
            openMovie(library: library, titled: video)
        } else {
            open(library: library)
            if app.buttons["chip-titles"].exists { app.buttons["chip-titles"].tap() }
            app.buttons["work"].firstMatch.tap()
        }
        let play = app.buttons["play"]
        XCTAssert(play.waitForExistence(timeout: 20), "the detail page has no Play")
        sleep(2)
        add(named("player-detail-button", play.screenshot()))
        keep("player-detail")

        play.tap()
        let close = app.buttons["Close"]
        let pause = app.buttons["Pause"]
        XCTAssert(pause.waitForExistence(timeout: 30), "playback didn't start")
        sleep(2)
        keep("player-controls")
        sleep(5)
        XCTAssertFalse(close.exists, "the controls didn't hide while playing")
        keep("player-hidden")

        // A tap on the video brings them back, and each tap on them keeps them.
        app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
        XCTAssert(close.waitForExistence(timeout: 3), "a tap didn't show the controls")
        for _ in 0..<3 {
            sleep(1)
            app.buttons["fill"].tap()
        }
        XCTAssert(close.exists, "the controls hid while being tapped")

        // Audio & Subtitles: the controls stay while it's open; a subtitle applies at once.
        app.buttons["tracks"].tap()
        XCTAssert(app.navigationBars["Audio & Subtitles"].waitForExistence(timeout: 5), "Audio & Subtitles didn't open")
        sleep(6)
        keep("player-tracks-sheet")
        let subtitle = app.buttons.matching(NSPredicate(format: "label BEGINSWITH 'English'")).firstMatch
        XCTAssert(subtitle.exists, "Audio & Subtitles lists no English track")
        subtitle.tap()
        sleep(1)
        keep("player-tracks-chosen")
        // Dialogue boost: each switch starts a new play, which the switch then shows.
        let boost = app.switches["dialogue-boost"]
        XCTAssert(boost.waitForExistence(timeout: 5), "Audio & Subtitles has no Dialogue boost")
        XCTAssertEqual(boost.value as? String, "0", "Dialogue boost starts on")
        boost.switches.firstMatch.tap()
        wait(for: [expectation(for: NSPredicate(format: "value == '1'"), evaluatedWith: boost)], timeout: 20)
        sleep(2)
        keep("player-dialogue-boost")
        boost.switches.firstMatch.tap()
        wait(for: [expectation(for: NSPredicate(format: "value == '0'"), evaluatedWith: boost)], timeout: 20)
        app.buttons["Done"].tap()
        XCTAssert(close.waitForExistence(timeout: 2), "the controls hid while the sheet was open")
        sleep(4)
        keep("player-subtitle")

        // Playback settings: its rows in order, and Audio & Subtitles, Speed and Stream info pushed.
        app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
        if !app.buttons["settings"].waitForExistence(timeout: 2) {
            app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
        }
        app.buttons["settings"].tap()
        XCTAssert(app.navigationBars["Playback Settings"].waitForExistence(timeout: 5), "Playback settings didn't open")
        sleep(5)
        keep("player-settings-sheet")
        let rows = ["Audio & Subtitles", "Quality", "Speed", "Stream info"].map { name in
            app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", name)).firstMatch
        }
        for row in rows {
            XCTAssert(row.exists, "Playback settings has no \(row)")
        }
        XCTAssertEqual(rows.map(\.frame.minY), rows.map(\.frame.minY).sorted(), "Playback settings' rows are out of order")
        rows[2].tap()
        sleep(1)
        keep("player-speed")
        app.navigationBars["Speed"].buttons.firstMatch.tap()
        rows[3].tap()
        sleep(1)
        keep("player-stream-info")
        app.navigationBars["Stream info"].buttons.firstMatch.tap()
        // Audio & Subtitles opens the tracks' panel in this one's place.
        rows[0].tap()
        XCTAssert(app.navigationBars["Audio & Subtitles"].waitForExistence(timeout: 5), "Audio & Subtitles didn't open the tracks")
        XCTAssert(app.switches["dialogue-boost"].exists, "Audio & Subtitles has no Dialogue boost")
        sleep(1)
        keep("player-settings-tracks")
        app.buttons["Done"].tap()
        XCTAssert(close.waitForExistence(timeout: 2), "the controls hid while Playback settings was open")

        // The scrub bar's bubble, kept from another thread while the finger holds.
        let bar = app.descendants(matching: .any)["scrub bar"]
        XCTAssert(bar.exists, "the controls have no scrub bar")
        let from = bar.coordinate(withNormalizedOffset: CGVector(dx: 0.3, dy: 0.5))
        let to = bar.coordinate(withNormalizedOffset: CGVector(dx: 0.45, dy: 0.5))
        let midDrag = expectation(description: "mid-scrub screenshot")
        DispatchQueue.global().asyncAfter(deadline: .now() + 2) {
            self.add(self.named("player-scrubbing", XCUIScreen.main.screenshot()))
            midDrag.fulfill()
        }
        from.press(forDuration: 0.1, thenDragTo: to, withVelocity: 200, thenHoldForDuration: 3)
        wait(for: [midDrag], timeout: 10)
        XCTAssert(close.exists, "the controls hid after scrubbing")

        // A double tap on the right third skips forward, with its ripple.
        sleep(5)
        app.coordinate(withNormalizedOffset: CGVector(dx: 0.85, dy: 0.5)).doubleTap()
        keep("player-double-tap")
        sleep(2)

        // Subtitles off again, then paused: the controls stay.
        app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
        XCTAssert(app.buttons["tracks"].waitForExistence(timeout: 3), "a tap didn't show the controls")
        app.buttons["tracks"].tap()
        XCTAssert(app.navigationBars["Audio & Subtitles"].waitForExistence(timeout: 5), "Audio & Subtitles didn't open")
        app.buttons["Off"].tap()
        app.buttons["Done"].tap()
        XCTAssert(pause.waitForExistence(timeout: 20), "playback didn't carry on after turning subtitles off")
        pause.tap()
        sleep(6)
        XCTAssert(close.exists, "the controls hid while paused")
        keep("player-paused")
        close.tap()
        XCTAssert(play.waitForExistence(timeout: 10), "closing the player didn't return to the page")
        sleep(3)
    }

    /// Dialogue boost on a movie whose audio is transcoded anyway: switched
    /// on, the play carries on with the picture and controls; opened again
    /// after a relaunch, with the boost kept on, it plays from where it was;
    /// paused, Close closes it. The video is `KIVO_BOOST_VIDEO` in
    /// `KIVO_LIBRARY`, the local server's Night Shift by default, at least
    /// twelve minutes long with surround audio, resumed from 10:38; it
    /// moves the video's resume position.
    func testDialogueBoostKeepsPlaying() throws {
        let library = environment["KIVO_LIBRARY"] ?? "Movies"
        let movie = environment["KIVO_BOOST_VIDEO"] ?? "Night Shift"
        // The controls over the video, as they are with the phone turned.
        XCUIDevice.shared.orientation = .landscapeLeft
        defer { XCUIDevice.shared.orientation = .portrait }
        try setPosition(of: movie, in: library, ms: 638_000)
        try signIn()
        let play = app.buttons["play"]
        let close = app.buttons["Close"]
        let pause = app.buttons["Pause"]
        let boost = app.switches["dialogue-boost"]

        openMovie(library: library, titled: movie)
        play.tap()
        XCTAssert(pause.waitForExistence(timeout: 60), "playback didn't start")
        app.buttons["tracks"].tap()
        XCTAssert(boost.waitForExistence(timeout: 10), "Audio & Subtitles has no Dialogue boost")
        if boost.value as? String == "1" {
            boost.switches.firstMatch.tap()
            wait(for: [expectation(for: NSPredicate(format: "value == '0'"), evaluatedWith: boost)], timeout: 60)
        }
        boost.switches.firstMatch.tap()
        wait(for: [expectation(for: NSPredicate(format: "value == '1'"), evaluatedWith: boost)], timeout: 60)
        keep("boost-on")
        app.buttons["Done"].tap()
        XCTAssert(pause.waitForExistence(timeout: 60), "playback didn't carry on with the boost on")
        assertPlaying("with the boost switched on")
        keep("boost-playing")

        // A cold start with the boost kept on: the play shows its picture and controls.
        app.terminate()
        try setPosition(of: movie, in: library, ms: 679_000)
        try signIn()
        openMovie(library: library, titled: movie)
        play.tap()
        XCTAssert(pause.waitForExistence(timeout: 60), "playback didn't start with the boost kept on")
        assertPlaying("after a relaunch with the boost on")
        keep("boost-relaunched")

        // Paused, Close closes the player.
        XCTAssert(showControls(), "the player has no controls")
        pause.tap()
        XCTAssert(close.waitForExistence(timeout: 3), "the controls hid while paused")
        keep("boost-paused")
        close.tap()
        XCTAssert(play.waitForExistence(timeout: 10) && waitForHittable(play, timeout: 10), "closing the paused player didn't return to the page")
        keep("boost-closed")

        // The boost back off, as the other tests expect.
        play.tap()
        XCTAssert(pause.waitForExistence(timeout: 60), "playback didn't start")
        app.buttons["tracks"].tap()
        XCTAssert(boost.waitForExistence(timeout: 10), "Audio & Subtitles has no Dialogue boost")
        boost.switches.firstMatch.tap()
        wait(for: [expectation(for: NSPredicate(format: "value == '0'"), evaluatedWith: boost)], timeout: 60)
        app.buttons["Done"].tap()
        XCTAssert(showControls(), "the player has no Close")
        close.tap()
    }

    /// The player shows its title and controls, Close at the top's leading
    /// end, and its position moves on.
    private func assertPlaying(_ when: String) {
        let close = app.buttons["Close"]
        XCTAssert(showControls(), "the player has no Close \(when)")
        let bar = app.descendants(matching: .any)["scrub bar"]
        XCTAssert(bar.exists, "the player has no scrub bar \(when)")
        let window = app.windows.firstMatch.frame
        XCTAssertLessThan(close.frame.midX, window.midX / 2, "Close isn't at the top's leading end \(when)")
        let before = bar.value as? String
        sleep(4)
        XCTAssert(showControls(), "the controls didn't show \(when)")
        XCTAssertNotEqual(bar.value as? String, before, "the position didn't move \(when)")
    }

    /// Shows the player's controls with a tap on the video where they're hidden; whether Close shows.
    private func showControls() -> Bool {
        let close = app.buttons["Close"]
        for _ in 0..<3 {
            if close.waitForExistence(timeout: 2) { return true }
            app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
        }
        return close.waitForExistence(timeout: 2)
    }

    private func openMovie(library: String, titled title: String) {
        open(library: library)
        if app.buttons["chip-titles"].exists { app.buttons["chip-titles"].tap() }
        let card = app.buttons.matching(identifier: "work").matching(NSPredicate(format: "label CONTAINS %@", title)).firstMatch
        XCTAssert(card.waitForExistence(timeout: 20), "\(library) has no \(title)")
        card.tap()
        XCTAssert(app.buttons["play"].waitForExistence(timeout: 20), "the detail page has no Play")
    }

    /// Sets the user's place in the work titled [title] in [library] through the API.
    private func setPosition(of title: String, in library: String, ms: Int) throws {
        let server = try XCTUnwrap(environment["KIVO_SERVER"], "set TEST_RUNNER_KIVO_SERVER")
        let username = try XCTUnwrap(environment["KIVO_USERNAME"], "set TEST_RUNNER_KIVO_USERNAME")
        let password = try XCTUnwrap(environment["KIVO_PASSWORD"], "set TEST_RUNNER_KIVO_PASSWORD")
        let base = server.contains("://") ? server : "http://\(server):8484"
        let session = URLSession(configuration: .ephemeral)
        func call(_ method: String, _ path: String, _ body: [String: Any]? = nil) throws -> [String: Any] {
            var request = URLRequest(url: URL(string: base + "/api/v1/" + path)!)
            request.httpMethod = method
            request.setValue(base, forHTTPHeaderField: "Origin")
            if let body {
                request.setValue("application/json", forHTTPHeaderField: "Content-Type")
                request.httpBody = try JSONSerialization.data(withJSONObject: body)
            }
            var result: [String: Any] = [:]
            let done = expectation(description: "\(method) \(path)")
            session.dataTask(with: request) { data, _, _ in
                result = data.flatMap { try? JSONSerialization.jsonObject(with: $0) as? [String: Any] } ?? [:]
                done.fulfill()
            }.resume()
            wait(for: [done], timeout: 10)
            return result
        }
        _ = try call("POST", "sessions", ["username": username, "password": password])
        let libraries = try call("GET", "libraries")["items"] as? [[String: Any]]
        let found = try XCTUnwrap(libraries?.first { $0["name"] as? String == library }, "the server has no \(library)")
        let kind = ["movies": "movie", "shows": "series", "audiobooks": "book"][found["kind"] as? String ?? ""] ?? "video"
        let works = try call("POST", "works/query", ["query": ["kinds": [kind]], "library": found["id"] ?? ""])["items"] as? [[String: Any]]
        let work = try XCTUnwrap(works?.first { ($0["title"] as? String)?.contains(title) == true }?["id"] as? String, "\(library) has no \(title)")
        let versions = try call("GET", "works/\(work)")["versions"] as? [[String: Any]]
        let version = try XCTUnwrap(versions?.first?["id"] as? String, "\(title) has no version")
        let at = ISO8601DateFormatter().string(from: Date())
        let state = try call("PATCH", "works/\(work)/user-state", ["positionMs": ms, "playedAt": at, "versionId": version])
        XCTAssertNotNil(state["positionMs"], "the server didn't take the position: \(state)")
        _ = try call("DELETE", "sessions/current")
    }

    private func waitForHittable(_ element: XCUIElement, timeout: TimeInterval) -> Bool {
        let hittable = expectation(for: NSPredicate(format: "hittable == true"), evaluatedWith: element)
        return XCTWaiter.wait(for: [hittable], timeout: timeout) == .completed
    }

    /// How smoothly a library's grid scrolls: fast flings down the works
    /// grid, with pages and posters loading as they go, measured as the
    /// scroll's hitches (on a device; the Simulator gives only the scroll's
    /// duration) and the app's CPU time. Run it alone, on a freshly
    /// installed app for cold caches (FIRST-RUN.md).
    func testLibraryScrollPerformance() throws {
        try signIn()
        open(library: environment["KIVO_LIBRARY"] ?? "Movies")
        if app.buttons["chip-titles"].exists { app.buttons["chip-titles"].tap() }
        sleep(2)
        let options = XCTMeasureOptions()
        options.invocationOptions = [.manuallyStop]
        measure(metrics: [XCTOSSignpostMetric.scrollingAndDecelerationMetric, XCTCPUMetric(application: app)], options: options) {
            app.swipeUp(velocity: .fast)
            app.swipeUp(velocity: .fast)
            sleep(2)
            stopMeasuring()
        }
        keep("library-scrolled")
    }

    /// The player's layouts on a phone, which follow the phone: opened
    /// upright, the video sits clear with the controls under it, which fade
    /// away while it plays and come back on a tap; turned, the controls are
    /// over the video. Each is kept as `player-…`.
    func testPlayerUprightAndLandscape() throws {
        try XCTSkipUnless(UIDevice.current.userInterfaceIdiom == .phone, "the phone's player")
        XCUIDevice.shared.orientation = .portrait
        defer { XCUIDevice.shared.orientation = .portrait }
        try signIn()
        let card = app.buttons.matching(identifier: "work").firstMatch
        XCTAssert(card.waitForExistence(timeout: 20), "Home has no cards")
        card.tap()
        let play = app.buttons["play"]
        XCTAssert(play.waitForExistence(timeout: 20), "the detail page has no Play")
        play.tap()

        // Upright, as the phone is: the video clear, the controls under it,
        // fading away while it plays and back on a tap.
        let pause = app.buttons["Pause"]
        XCTAssert(pause.waitForExistence(timeout: 30), "playback didn't start")
        XCTAssertLessThan(app.frame.width, app.frame.height, "the player didn't open as the phone is held")
        // The faded controls keep their place, and their buttons stay in the
        // tree: what tells is whether they're drawn on the black under the video.
        let tracks = app.buttons["tracks"]
        XCTAssert(tracks.waitForExistence(timeout: 5), "upright has no Audio & Subtitles")
        XCTAssertGreaterThan(tracks.frame.minY, app.frame.height * 0.6, "the controls aren't under the video")
        let controls = CGPoint(x: tracks.frame.midX, y: tracks.frame.midY)
        XCTAssert(waitUntil(15) { !drawn(at: controls) }, "the controls didn't hide upright")
        keep("player-upright-hidden")
        app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.85)).tap()
        XCTAssert(waitUntil(5) { drawn(at: controls) }, "a tap didn't bring the controls back")
        XCTAssert(pause.exists, "upright has no Pause")
        keep("player-upright")

        // Turned: the controls over the video. The Duo's simulator doesn't
        // turn its screen, whatever XCTest says, so `KIVO_DUO_ORIENTATIONS`
        // of "portrait" leaves it upright, as the Duo's layout test does.
        if environment["KIVO_DUO_ORIENTATIONS"]?.contains("landscape") != false {
            XCUIDevice.shared.orientation = .landscapeLeft
            let turned = expectation(for: NSPredicate { _, _ in self.app.frame.width > self.app.frame.height }, evaluatedWith: nil)
            wait(for: [turned], timeout: 10)
            // A tap shows or hides them, and whether they show after turning
            // depends on the fade: a second tap if the first hid them.
            sleep(1)
            let middle = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5))
            middle.tap()
            if !pause.waitForExistence(timeout: 3) { middle.tap() }
            XCTAssert(pause.waitForExistence(timeout: 5), "landscape has no Pause")
            keep("player-landscape")
            XCUIDevice.shared.orientation = .portrait
        }

        app.buttons["Close"].tap()
        XCTAssert(play.waitForExistence(timeout: 10), "closing didn't go back to the page")
        keep("player-closed")
    }

    /// Launches the app and signs in, unless the Keychain still holds a
    /// sign-in from a last run; returns the user button that marks Home.
    @discardableResult
    private func signIn(showingSteps: Bool = false) throws -> XCUIElement {
        let server = try XCTUnwrap(environment["KIVO_SERVER"], "set TEST_RUNNER_KIVO_SERVER")
        let username = try XCTUnwrap(environment["KIVO_USERNAME"], "set TEST_RUNNER_KIVO_USERNAME")
        let password = try XCTUnwrap(environment["KIVO_PASSWORD"], "set TEST_RUNNER_KIVO_PASSWORD")

        app.launch()

        // Sign in, unless the Keychain still holds a sign-in from a last run.
        // Discovery goes straight to the password when it finds one server.
        let address = app.textFields["address"]
        let user = app.textFields["username"]
        let home = app.buttons["user"]
        let start = NSPredicate { _, _ in address.exists || user.exists || home.exists }
        wait(for: [expectation(for: start, evaluatedWith: nil)], timeout: 20)
        if !home.exists {
            if showingSteps { keep("sign-in") }
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
            // A wrong password first: the server's refusal in words.
            let secret = app.secureTextFields["password"]
            secret.tap()
            secret.typeText(password + "-wrong")
            app.buttons["Sign in"].tap()
            XCTAssert(app.staticTexts["Wrong username or password."].waitForExistence(timeout: 15), "a wrong password didn't say so")
            if showingSteps { keep("wrong-password") }
            // Typing into a secure field again replaces what it held.
            secret.tap()
            secret.typeText(password)
            app.buttons["Sign in"].tap()
            XCTAssert(home.waitForExistence(timeout: 20), "Home didn't show after signing in")
            // iOS offers to save the password, from Safari's view service.
            let notNow = XCUIApplication(bundleIdentifier: "com.apple.SafariViewService").buttons["Not Now"]
            if notNow.waitForExistence(timeout: 5) { notNow.tap() }
        }
        XCTAssert(home.waitForExistence(timeout: 20), "Home didn't show")
        return home
    }

    /// Opens [library] from the Libraries tab's list of libraries.
    private func open(library: String) {
        app.tabBars.buttons["Libraries"].tap()
        // The tab opens on the library last opened, over the list of them; choosing it again shows the list.
        let list = app.descendants(matching: .any)["library-list"]
        if !list.waitForExistence(timeout: 5) { app.tabBars.buttons["Libraries"].tap() }
        XCTAssert(list.waitForExistence(timeout: 10), "the Libraries tab has no list")
        let row = app.descendants(matching: .any)["library:\(library)"].firstMatch
        XCTAssert(row.waitForExistence(timeout: 5), "the list has no \(library)")
        keep("library-list")
        row.tap()
        XCTAssert(app.navigationBars[library].waitForExistence(timeout: 10), "\(library) didn't open")
        XCTAssert(app.buttons["work"].firstMatch.waitForExistence(timeout: 20), "\(library)'s grid has no cards")
    }

    /// Closes an open menu by tapping outside it.
    private func dismissMenu() {
        app.coordinate(withNormalizedOffset: CGVector(dx: 0.05, dy: 0.08)).tap()
        sleep(1)
    }

    /// Whether anything is drawn in the 20-point square around [point] on the screen, rather than black.
    private func drawn(at point: CGPoint) -> Bool {
        let shot = XCUIScreen.main.screenshot().image
        guard let image = shot.cgImage else { return false }
        let scale = CGFloat(image.width) / app.frame.width
        let side = Int(20 * scale)
        guard let square = image.cropping(to: CGRect(x: (point.x - 10) * scale, y: (point.y - 10) * scale, width: CGFloat(side), height: CGFloat(side))) else { return false }
        var pixels = [UInt8](repeating: 0, count: side * side * 4)
        guard let context = CGContext(
            data: &pixels, width: side, height: side, bitsPerComponent: 8, bytesPerRow: side * 4,
            space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue
        ) else { return false }
        context.draw(square, in: CGRect(x: 0, y: 0, width: side, height: side))
        return stride(from: 0, to: pixels.count, by: 4).contains { pixels[$0] > 60 || pixels[$0 + 1] > 60 || pixels[$0 + 2] > 60 }
    }

    /// Checks [condition] every half second for up to [seconds].
    private func waitUntil(_ seconds: TimeInterval, _ condition: () -> Bool) -> Bool {
        let end = Date().addingTimeInterval(seconds)
        while Date() < end {
            if condition() { return true }
            Thread.sleep(forTimeInterval: 0.5)
        }
        return condition()
    }

    /// Keeps the screen in the result bundle, and with `KIVO_SHOTS_DIR`
    /// set, writes it there as a PNG too.
    private func keep(_ name: String) {
        let screenshot = XCUIScreen.main.screenshot()
        add(named(name, screenshot))
        if let dir = environment["KIVO_SHOTS_DIR"] {
            try? screenshot.pngRepresentation.write(to: URL(fileURLWithPath: dir).appendingPathComponent("\(name).png"))
        }
    }

    private func named(_ name: String, _ screenshot: XCUIScreenshot) -> XCTAttachment {
        let shot = XCTAttachment(screenshot: screenshot)
        shot.name = name
        shot.lifetime = .keepAlways
        return shot
    }
}
