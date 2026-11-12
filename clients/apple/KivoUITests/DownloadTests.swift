import XCTest

/// Downloads end to end against a local server (FIRST-RUN.md, The download
/// test): a book downloads as its original; a movie at Standard is converted on
/// the server, then comes to the phone, and the app is killed part way
/// through the transfer and relaunched, and the transfer goes on; then,
/// with the server stopped, the app opens on Downloads, Home says where its
/// rows would be that it can't reach the server and goes back to Downloads,
/// and both play from their files, the movie with its subtitle. The server is reached through
/// a proxy that slows download files, so the transfer can be caught part
/// way, drops the connection once in the movie's, so it resumes by range,
/// and stops the server on `POST /__stop`. From the environment, with the
/// `TEST_RUNNER_` prefix:
///
///     KIVO_DOWNLOAD_SERVER=http://127.0.0.1:18490
///     KIVO_DOWNLOAD_USERNAME=test KIVO_DOWNLOAD_PASSWORD=…
///
/// Each step's screen is kept as an attachment named `ios-dl-…`.
final class DownloadTests: XCTestCase {
    private let app = XCUIApplication()

    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    func testDownloadAndPlayOffline() throws {
        let server = try TestServer.download.address
        try signIn(app, at: .download)

        // A book: its original, at once.
        open(library: "Audiobooks")
        app.buttons["work"].firstMatch.tap()
        let download = app.buttons["download"]
        XCTAssert(download.waitForExistence(timeout: 20), "the book's page has no Download")
        sleep(2)
        keep("ios-dl-book-page")
        download.tap()
        app.tabBars.buttons["Downloads"].tap()
        XCTAssert(row(containing: "Downloaded").waitForExistence(timeout: 60), "the book didn't download")
        keep("ios-dl-book-ready")

        // A movie at Standard: its quality menu with sizes, on a long press, then converted on the server.
        open(library: "Movies")
        app.buttons["work"].firstMatch.tap()
        XCTAssert(download.waitForExistence(timeout: 20), "the movie's page has no Download")
        sleep(2)
        // Its subtitle, which comes inside the converted file.
        app.buttons["Subtitles"].tap()
        let english = app.buttons.matching(NSPredicate(format: "label BEGINSWITH 'English'")).firstMatch
        XCTAssert(english.waitForExistence(timeout: 5), "the subtitle menu has no English")
        english.tap()
        sleep(1)
        download.press(forDuration: 1.2)
        let standard = app.buttons.matching(NSPredicate(format: "label BEGINSWITH 'Standard'")).firstMatch
        XCTAssert(standard.waitForExistence(timeout: 10), "the download menu has no Standard")
        sleep(1)
        keep("ios-dl-quality-menu")
        standard.tap()
        sleep(1)
        keep("ios-dl-movie-asked")
        app.tabBars.buttons["Downloads"].tap()
        let converting = row(containing: "Preparing")
        if converting.waitForExistence(timeout: 30) {
            sleep(2)
            keep("ios-dl-converting")
        }
        let downloading = row(containing: "Downloading")
        XCTAssert(downloading.waitForExistence(timeout: 300), "the movie didn't start coming to the phone")
        sleep(6)
        keep("ios-dl-downloading")
        XCTAssert(downloading.exists, "the transfer finished before it could be interrupted")

        // Killed part way through, and opened again: the transfer goes on.
        app.terminate()
        sleep(3)
        app.launch()
        XCTAssert(app.buttons["user"].waitForExistence(timeout: 30), "the app didn't open signed in")
        app.tabBars.buttons["Downloads"].tap()
        sleep(3)
        keep("ios-dl-relaunched")
        XCTAssert(rows(containing: "Downloaded").count == 1 || row(containing: "Downloading").exists, "the movie's download was lost")
        let ready = NSPredicate { _, _ in self.rows(containing: "Downloaded").count == 2 }
        wait(for: [expectation(for: ready, evaluatedWith: nil)], timeout: 300)
        sleep(2)
        keep("ios-dl-ready")

        // The server stops: the app opens on Downloads, and both play from their files.
        stopServer(server)
        app.terminate()
        sleep(2)
        app.launch()
        let movie = row(containing: "Big Test")
        XCTAssert(movie.waitForExistence(timeout: 60), "offline, the app didn't open on Downloads")
        sleep(2)
        keep("ios-dl-offline")
        app.tabBars.buttons["Home"].tap()
        let unreachable = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "Can't reach")).firstMatch
        XCTAssert(unreachable.waitForExistence(timeout: 30), "offline, Home doesn't say it can't reach the server")
        XCTAssert(app.buttons["Retry"].exists, "offline, Home has no Retry")
        keep("ios-dl-offline-home")
        app.buttons["Go to Downloads"].tap()
        XCTAssert(movie.waitForExistence(timeout: 5), "Go to Downloads didn't go to Downloads")
        movie.tap()
        let pause = app.buttons["Pause"]
        XCTAssert(pause.waitForExistence(timeout: 30), "the movie didn't play from its file")
        sleep(4)
        keep("ios-dl-offline-playing")
        // Stream info: the file plays directly, and its Source is the download.
        app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
        XCTAssert(app.buttons["settings"].waitForExistence(timeout: 3), "the player has no settings")
        app.buttons["settings"].tap()
        XCTAssert(app.buttons["Stream info"].waitForExistence(timeout: 5), "Playback settings has no Stream info")
        app.buttons["Stream info"].tap()
        let source = app.descendants(matching: .any).matching(NSPredicate(format: "label BEGINSWITH 'Downloaded (converted from'")).firstMatch
        XCTAssert(source.waitForExistence(timeout: 5), "Stream info's Source doesn't name the download")
        let direct = app.descendants(matching: .any).matching(NSPredicate(format: "label CONTAINS 'Direct Play'")).firstMatch
        XCTAssert(direct.exists, "the download doesn't play directly")
        XCTAssertFalse(app.staticTexts["Quality"].exists, "a download has a Quality line")
        let streamingWhy = app.descendants(matching: .any).matching(NSPredicate(format: "label CONTAINS %@", "can't draw")).firstMatch
        XCTAssertFalse(streamingWhy.exists, "the download's subtitle gives a streaming reason")
        keep("ios-si-offline")
        // Back in the panel's bar, not the player's under it.
        app.navigationBars["Stream info"].buttons.firstMatch.tap()
        XCTAssert(app.buttons["Done"].waitForExistence(timeout: 3), "Stream info didn't go back")
        app.buttons["Done"].tap()
        if !pause.waitForExistence(timeout: 2) { app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap() }
        if pause.waitForExistence(timeout: 3) { pause.tap() }
        let close = app.buttons["Close"]
        XCTAssert(close.waitForExistence(timeout: 5), "the player has no Close")
        keep("ios-dl-offline-paused")
        close.tap()
        let book = row(containing: "Test Book")
        XCTAssert(book.waitForExistence(timeout: 10), "Downloads didn't come back")
        book.tap()
        XCTAssert(pause.waitForExistence(timeout: 30), "the book didn't play from its file")
        sleep(3)
        keep("ios-dl-offline-book")
        app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
        if close.waitForExistence(timeout: 3) { close.tap() }
        sleep(2)
    }

    /// A downloaded work's page plays its download, or streams it from the
    /// ⋯ menu; then Downloads deletes a season, a whole show, and two items
    /// selected in Edit. Run on a fresh server.sh, as testDownloadAndPlayOffline
    /// stops it. Screenshots are named `ios-dl2-…`.
    func testTitlePagePlaysItsDownloadAndDeleting() throws {
        let server = try TestServer.download.address
        try signIn(app, at: .download)
        let download = app.buttons["download"]

        // The book, the movie at Standard, and the show's every episode as it is.
        open(library: "Audiobooks")
        app.buttons["work"].firstMatch.tap()
        XCTAssert(download.waitForExistence(timeout: 20), "the book's page has no Download")
        sleep(2)
        download.tap()
        open(library: "Shows")
        app.buttons["work"].firstMatch.tap()
        let more = app.buttons["more"]
        XCTAssert(more.waitForExistence(timeout: 20), "the series' page has no ⋯")
        sleep(2)
        more.tap()
        app.buttons["Download series"].tap()
        let original = app.buttons["download-original"]
        XCTAssert(original.waitForExistence(timeout: 20), "Download Series has no Original")
        original.tap()
        // Somewhere to resume the movie from: 2:30 of its 3 minutes.
        try setMoviePosition(server, ms: 150_000)
        open(library: "Movies")
        app.buttons["work"].firstMatch.tap()
        XCTAssert(download.waitForExistence(timeout: 20), "the movie's page has no Download")
        sleep(2)
        download.press(forDuration: 1.2)
        let standard = app.buttons.matching(NSPredicate(format: "label BEGINSWITH 'Standard'")).firstMatch
        XCTAssert(standard.waitForExistence(timeout: 10), "the download menu has no Standard")
        standard.tap()

        // While it comes, the page says where it is; once ready, "Downloaded".
        let badge = app.descendants(matching: .any)["download-badge"].firstMatch
        XCTAssert(badge.waitForExistence(timeout: 10), "the page has no download badge")
        sleep(4)
        keep("ios-dl2-movie-progress")
        let ready = NSPredicate(format: "label CONTAINS 'Downloaded'")
        wait(for: [expectation(for: ready, evaluatedWith: badge)], timeout: 300)
        XCTAssertFalse(badge.label.contains("1080p"), "the badge names a resolution: \(badge.label)")
        keep("ios-dl2-movie-badge")

        // Stream Instead plays from the server.
        more.tap()
        app.buttons["Stream instead"].tap()
        let pause = app.buttons["Pause"]
        XCTAssert(pause.waitForExistence(timeout: 30), "Stream Instead didn't play")
        sleep(4)
        keep("ios-dl2-streaming")
        closePlayer()

        let play = app.buttons["play"]
        XCTAssert(play.label.hasPrefix("Resume"), "the page's Play isn't Resume: \(play.label)")

        // With the server stopped, Resume plays the download.
        app.tabBars.buttons["Downloads"].tap()
        let show = app.buttons.matching(NSPredicate(format: "label CONTAINS 'Test Show'")).firstMatch
        XCTAssert(show.waitForExistence(timeout: 10), "Downloads has no show")
        wait(for: [expectation(for: NSPredicate(format: "value CONTAINS 'Downloaded'"), evaluatedWith: show)], timeout: 120)
        XCTAssert(row(containing: "Test Book").waitForExistence(timeout: 60), "the book didn't download")
        stopServer(server)
        app.tabBars.buttons["Libraries"].tap()
        XCTAssert(play.waitForExistence(timeout: 10), "the movie's page has no Play")
        play.tap()
        XCTAssert(pause.waitForExistence(timeout: 30), "Resume didn't play the download with the server stopped")
        sleep(4)
        keep("ios-dl2-local-playing")
        closePlayer()

        // Play from Beginning, beside Resume, plays the download from the start.
        XCTAssert(play.waitForExistence(timeout: 10), "the movie's page has no Resume")
        let fromStart = app.buttons["play-from-beginning"]
        XCTAssert(fromStart.waitForExistence(timeout: 5), "Resume has no Play from Beginning beside it")
        sleep(1)
        keep("ios-dl2-restart")
        fromStart.tap()
        XCTAssert(pause.waitForExistence(timeout: 30), "Play from Beginning didn't play the download")
        sleep(3)
        keep("ios-dl2-from-beginning")
        closePlayer()

        // A show's episode, selected in Edit on its page.
        app.tabBars.buttons["Downloads"].tap()
        sleep(1)
        keep("ios-dl2-downloads")
        show.tap()
        let edit = app.buttons["downloads-edit"]
        XCTAssert(edit.waitForExistence(timeout: 10), "the show has no Edit")
        sleep(1)
        keep("ios-dl2-show-seasons")
        edit.tap()
        // Season 1's two episodes, then Season 2's one.
        app.cells.element(boundBy: 2).tap()
        let deleteEpisode = app.buttons["downloads-delete-selected"]
        XCTAssert(deleteEpisode.waitForExistence(timeout: 5), "Edit has no Delete in the bottom bar")
        deleteEpisode.tap()
        confirmDelete("Delete 1 item", shot: "ios-dl2-season-confirm")
        let seasonTwo = app.staticTexts["Season 2"]
        wait(for: [expectation(for: NSPredicate(format: "exists == false"), evaluatedWith: seasonTwo)], timeout: 10)
        XCTAssertEqual(app.buttons.matching(identifier: "download").count, 2, "Delete took more than the episode selected")
        keep("ios-dl2-season-deleted")

        // The whole show, by a swipe.
        app.navigationBars.buttons.firstMatch.tap()
        XCTAssert(show.waitForExistence(timeout: 5), "Downloads didn't come back")
        show.swipeLeft()
        app.buttons["Delete"].tap()
        confirmDelete("Delete 2 episodes", shot: "ios-dl2-show-confirm")
        wait(for: [expectation(for: NSPredicate(format: "exists == false"), evaluatedWith: show)], timeout: 10)

        // Edit: the book and the movie, selected and deleted together.
        app.buttons["downloads-edit"].tap()
        app.staticTexts["Test Book"].firstMatch.tap()
        app.staticTexts["Big Test"].firstMatch.tap()
        let deleteSelected = app.buttons["downloads-delete-selected"]
        XCTAssert(deleteSelected.waitForExistence(timeout: 5), "Edit has no Delete in the bottom bar")
        XCTAssert(deleteSelected.label.hasPrefix("Delete (2)"), "the bottom bar says \(deleteSelected.label)")
        keep("ios-dl2-edit-selected")
        deleteSelected.tap()
        confirmDelete("Delete 2 items", shot: "ios-dl2-edit-confirm")
        XCTAssert(app.staticTexts["Downloaded books and videos appear here for offline playback."].waitForExistence(timeout: 10), "Downloads isn't empty")
        keep("ios-dl2-empty")
    }

    /// Sets the user's place in the server's one movie, past the 2 minutes a video must play to resume.
    private func setMoviePosition(_ server: String, ms: Int) throws {
        let password = try TestServer.download.password
        let session = URLSession(configuration: .ephemeral)
        func call(_ method: String, _ path: String, _ body: [String: Any]? = nil) throws -> [String: Any] {
            var request = URLRequest(url: URL(string: server + "/api/v1/" + path)!)
            request.httpMethod = method
            request.setValue(server, forHTTPHeaderField: "Origin")
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
        _ = try call("POST", "sessions", ["username": "test", "password": password])
        let libraries = try call("GET", "libraries")["items"] as? [[String: Any]]
        let library = try XCTUnwrap(libraries?.first { $0["name"] as? String == "Movies" }?["id"] as? String, "the server has no Movies")
        let movies = try call("POST", "works/query", ["query": ["kinds": ["movie"]], "library": library])["items"] as? [[String: Any]]
        let id = try XCTUnwrap(movies?.first?["id"] as? String, "the server lists no movie")
        let versions = try call("GET", "works/\(id)")["versions"] as? [[String: Any]]
        let version = try XCTUnwrap(versions?.first?["id"] as? String, "the movie has no version")
        let at = ISO8601DateFormatter().string(from: Date())
        let state = try call("PATCH", "works/\(id)/user-state", ["positionMs": ms, "playedAt": at, "versionId": version])
        XCTAssertNotNil(state["positionMs"], "the server didn't take the position: \(state)")
        _ = try call("DELETE", "sessions/current")
    }

    /// A movie on its way goes on from where it was when cellular downloads
    /// are allowed, which moves its transfer to the other background
    /// session, and when it's paused and resumed from its page. The proxy's
    /// log shows each move resuming by range.
    func testAMovieGoesOnAfterTheCellularSettingAndAPause() throws {
        try signIn(app, at: .download)
        open(library: "Movies")
        app.buttons["work"].firstMatch.tap()
        let download = app.buttons["download"]
        XCTAssert(download.waitForExistence(timeout: 20), "the movie's page has no Download")
        sleep(2)
        download.press(forDuration: 1.2)
        let standard = app.buttons.matching(NSPredicate(format: "label BEGINSWITH 'Standard'")).firstMatch
        XCTAssert(standard.waitForExistence(timeout: 10), "the download menu has no Standard")
        standard.tap()
        app.tabBars.buttons["Downloads"].tap()
        XCTAssert(row(containing: "Downloading").waitForExistence(timeout: 300), "the movie didn't start coming to the phone")

        setCellularDownloads(on: true)
        app.tabBars.buttons["Downloads"].tap()
        XCTAssert(row(containing: "Downloading").waitForExistence(timeout: 10), "the movie stopped coming once cellular was allowed")
        keep("ios-dl3-cellular-allowed")

        app.tabBars.buttons["Libraries"].tap()
        XCTAssert(download.waitForExistence(timeout: 10), "the movie's page has no Download")
        download.tap()
        app.buttons["Pause download"].tap()
        app.tabBars.buttons["Downloads"].tap()
        XCTAssert(row(containing: "Paused").waitForExistence(timeout: 10), "the movie didn't pause")
        keep("ios-dl3-paused")

        app.tabBars.buttons["Libraries"].tap()
        download.tap()
        app.buttons["Resume download"].tap()
        app.tabBars.buttons["Downloads"].tap()
        XCTAssert(row(containing: "Downloaded").waitForExistence(timeout: 300), "the movie didn't finish after resuming")
        keep("ios-dl3-ready")
        setCellularDownloads(on: false)
    }

    /// Settings › Downloads › Allow cellular downloads, from Home's user menu.
    private func setCellularDownloads(on: Bool) {
        app.tabBars.buttons["Home"].tap()
        let user = app.buttons["user"]
        XCTAssert(user.waitForExistence(timeout: 10), "Home has no user button")
        user.tap()
        let downloads = app.collectionViews.buttons["Downloads"]
        for _ in 0 ..< 6 where !(downloads.exists && downloads.isHittable) { app.swipeUp() }
        XCTAssert(downloads.exists, "Settings has no Downloads")
        downloads.tap()
        let toggle = app.switches["Allow cellular downloads"]
        XCTAssert(toggle.waitForExistence(timeout: 5), "Downloads has no Allow cellular downloads")
        if (toggle.value as? String == "1") != on {
            toggle.switches.firstMatch.exists ? toggle.switches.firstMatch.tap() : toggle.tap()
        }
        let set = NSPredicate(format: "value == %@", on ? "1" : "0")
        wait(for: [expectation(for: set, evaluatedWith: toggle)], timeout: 5)
        app.navigationBars.buttons["Settings"].tap()
        app.buttons["Done"].tap()
        sleep(1)
    }

    /// Closes the player, past its hidden controls.
    private func closePlayer() {
        app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
        let pause = app.buttons["Pause"]
        if pause.waitForExistence(timeout: 3) { pause.tap() }
        let close = app.buttons["Close"]
        XCTAssert(close.waitForExistence(timeout: 5), "the player has no Close")
        close.tap()
        sleep(2)
    }

    /// Answers a deletion's confirmation, which asks [question] with the size.
    private func confirmDelete(_ question: String, shot: String) {
        let asked = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", question)).firstMatch
        XCTAssert(asked.waitForExistence(timeout: 5), "no confirmation asking \(question)")
        keep(shot)
        app.buttons["download-delete-confirm"].firstMatch.tap()
    }

    /// A download's row whose words, or its state as its value says it ("Downloaded"), contain [text].
    private func row(containing text: String) -> XCUIElement {
        app.buttons.matching(identifier: "download").matching(NSPredicate(format: "label CONTAINS %@ OR value CONTAINS %@", text, text)).firstMatch
    }

    private func rows(containing text: String) -> [XCUIElement] {
        app.buttons.matching(identifier: "download").matching(NSPredicate(format: "label CONTAINS %@ OR value CONTAINS %@", text, text)).allElementsBoundByIndex
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
        row.tap()
        XCTAssert(app.navigationBars[library].waitForExistence(timeout: 10), "\(library) didn't open")
        XCTAssert(app.buttons["work"].firstMatch.waitForExistence(timeout: 20), "\(library)'s grid has no cards")
    }

    /// Stops the server, through the proxy in front of it.
    private func stopServer(_ server: String) {
        var request = URLRequest(url: URL(string: server + "/__stop")!)
        request.httpMethod = "POST"
        let done = expectation(description: "server stopped")
        URLSession.shared.dataTask(with: request) { _, _, _ in done.fulfill() }.resume()
        wait(for: [done], timeout: 10)
    }
}
