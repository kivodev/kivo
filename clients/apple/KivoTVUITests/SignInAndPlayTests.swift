import XCTest

/// Drives the Apple TV app with the remote against a real server: sign in
/// with a code, which the test approves as the user would on a phone, Home,
/// a library's grid, a movie's page and its playback with the transport
/// bar's Subtitles menu, a series' page and its Next Up's playback with its
/// styled subtitle and up next at its credits, then Search and Settings. The server and the user come
/// from the environment, which `xcodebuild test` passes in with the
/// `TEST_RUNNER_` prefix (FIRST-RUN.md):
///
///     TEST_RUNNER_KIVO_SERVER=http://127.0.0.1:18484 TEST_RUNNER_KIVO_USERNAME=… \
///     TEST_RUNNER_KIVO_PASSWORD=… TEST_RUNNER_KIVO_MOVIE=… TEST_RUNNER_KIVO_SHOW=… \
///     xcodebuild test …
///
/// The movie needs a subtitle track; the show's Next Up, resumed less than
/// half a minute before credits chapter, a styled subtitle the user's
/// subtitle setting shows. Each step's screen is kept as an attachment in the result
/// bundle. It changes nothing on the server but resume positions.
final class SignInAndPlayTests: XCTestCase {
    private let app = XCUIApplication()
    private let remote = XCUIRemote.shared
    private let environment = ProcessInfo.processInfo.environment

    override func setUpWithError() throws {
        continueAfterFailure = false
        // Every run signs in as one client, replacing the last run's session.
        app.launchEnvironment["KIVO_CLIENT_ID"] = "kivo-tv-ui-tests"
        // Sign-in goes straight to the server under test, without browsing the network.
        app.launchEnvironment["KIVO_SERVER"] = try value("SERVER")
    }

    func testSignInBrowseAndPlay() throws {
        let movie = try value("MOVIE")
        let show = try value("SHOW")
        try signIn()
        XCTAssert(app.staticTexts["Continue Watching"].waitForExistence(timeout: 1) || app.buttons.firstMatch.waitForExistence(timeout: 20))
        sleep(3)
        keep("home")

        // The library: along the sidebar to Movies, down into the grid.
        focusTab("Movies")
        sleep(3)
        keep("library")
        intoGrid(first: "Big Test")
        XCTAssert(focus(label: movie, moving: .right, limit: 6), "\(movie) isn't in the grid")
        press(.select)
        XCTAssert(app.buttons["Play"].waitForExistence(timeout: 15), "the movie's page has no Play")
        sleep(3)
        keep("movie")

        // Its playback, the transport bar, and its Subtitles menu.
        XCTAssert(focus(label: "Play", moving: .up, limit: 3))
        press(.select)
        sleep(30)
        keep("player")
        press(.select)
        sleep(2)
        keep("player-controls")
        dump("player-controls")
        if focus(label: "Subtitles", moving: .up, limit: 2) || focus(label: "Subtitles", moving: .right, limit: 6) {
            press(.select)
            sleep(2)
            keep("player-subtitles")
            press(.menu)
        }
        // Out of the player to the movie's page, and back to the grid.
        back(until: app.buttons["Play"])
        keep("after-player")
        press(.menu, pause: 2)

        // A series, and its first episode's Skip Intro.
        focusTab("Shows")
        intoGrid(first: show)
        XCTAssert(focus(label: show, moving: .right, limit: 6), "\(show) isn't in the grid")
        press(.select)
        sleep(4)
        keep("series")

        // Next Up, resumed under half a minute before its credits: its styled
        // subtitle over the video, then up next at the credits.
        press(.select)
        sleep(12)
        keep("styled-subtitles")
        sleep(17)
        keep("up-next")
        back(until: app.staticTexts["Season 1"])
        press(.menu, pause: 2)

        // Search, typed on the keyboard.
        focusTab("Search")
        sleep(1)
        app.typeText(String(show.prefix(6)))
        sleep(4)
        keep("search")
        // Menu leaves the keyboard for the sidebar.
        press(.menu, pause: 2)
        sidebarOpen = true

        focusTab("Settings")
        sleep(2)
        keep("settings")
    }

    // MARK: Steps

    /// Signs in with the code the app shows, which the test approves as the
    /// user, unless the app still holds a sign-in from a last run.
    private func signIn() throws {
        app.launch()
        let code = app.staticTexts["link-code"]
        let home = app.descendants(matching: .any).matching(NSPredicate(format: "label == %@", "Home")).firstMatch
        wait(for: [expectation(for: NSPredicate { _, _ in code.exists || home.exists }, evaluatedWith: nil)], timeout: 30)
        if home.exists { return }
        let ready = NSPredicate { _, _ in !code.label.trimmingCharacters(in: .whitespaces).isEmpty }
        wait(for: [expectation(for: ready, evaluatedWith: nil)], timeout: 20)
        sleep(1)
        keep("sign-in")
        try approve(code.label)
    }

    /// Signs in to the server as the user, and approves [code] as that user.
    private func approve(_ code: String) throws {
        let server = try value("SERVER")
        let token = try request("\(server)/api/v1/sessions", body: [
            "username": try value("USERNAME"), "password": try value("PASSWORD"), "kind": "app", "name": "Kivo TV UI tests",
        ])["token"] as? String
        _ = try request("\(server)/api/v1/device-codes/\(code)/approve", body: [:], token: XCTUnwrap(token))
    }

    private func request(_ url: String, body: [String: Any], token: String? = nil) throws -> [String: Any] {
        var request = URLRequest(url: try XCTUnwrap(URL(string: url)))
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        if let token { request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization") }
        request.httpBody = try JSONSerialization.data(withJSONObject: body)
        let done = expectation(description: url)
        var answer: Data?
        URLSession.shared.dataTask(with: request) { data, _, _ in
            answer = data
            done.fulfill()
        }.resume()
        wait(for: [done], timeout: 15)
        guard let answer, !answer.isEmpty else { return [:] }
        return (try? JSONSerialization.jsonObject(with: answer)) as? [String: Any] ?? [:]
    }

    /// The sidebar's items, in its order, and the one showing; the sidebar is open as the app opens.
    private let tabs = ["Search", "Home", "Lists", "Movies", "Shows", "Home Videos", "Settings"]
    private var tab = "Home"
    private var sidebarOpen = true

    /// Opens the sidebar with Menu from a tab's first screen, goes along it to [label], and into its screen.
    private func focusTab(_ label: String) {
        guard let from = tabs.firstIndex(of: tab), let to = tabs.firstIndex(of: label) else { return XCTFail("no tab \(label)") }
        if !sidebarOpen { press(.menu, pause: 1.5) }
        for _ in 0..<abs(to - from) { press(to > from ? .down : .up) }
        press(.select, pause: 3)
        tab = label
        sidebarOpen = false
    }

    /// Presses Menu until [element] shows, as out of the player, a step at a time.
    private func back(until element: XCUIElement) {
        for _ in 0..<6 where !element.exists {
            press(.menu, pause: 2)
        }
        XCTAssert(element.exists, "didn't get back to \(element)")
    }

    /// Moves focus down past the library's header to its grid's first card, [first].
    private func intoGrid(first: String) {
        for _ in 0..<4 where focused?.label.contains(first) != true {
            press(.down)
        }
    }

    /// Moves focus [moving] until the element with [label] has it, at most [limit] times.
    private func focus(label: String, moving direction: XCUIRemote.Button, limit: Int) -> Bool {
        for _ in 0...limit {
            if focused?.label.contains(label) == true { return true }
            press(direction)
        }
        if focused?.label.contains(label) == true { return true }
        dump("focus-\(label)")
        return false
    }

    private var focused: XCUIElement? {
        let element = app.descendants(matching: .any).element(matching: NSPredicate(format: "hasFocus == true"))
        return element.exists ? element : nil
    }

    private func press(_ button: XCUIRemote.Button, pause: Double = 0.6) {
        remote.press(button)
        Thread.sleep(forTimeInterval: pause)
    }

    private func value(_ name: String) throws -> String {
        try XCTUnwrap(environment["KIVO_\(name)"], "set TEST_RUNNER_KIVO_\(name)")
    }

    /// Keeps the screen in the result bundle, and with `KIVO_SHOTS_DIR`
    /// set, writes it there as a PNG too.
    private func keep(_ name: String) {
        let screenshot = XCUIScreen.main.screenshot()
        let shot = XCTAttachment(screenshot: screenshot)
        shot.name = name
        shot.lifetime = .keepAlways
        add(shot)
        if let dir = environment["KIVO_SHOTS_DIR"] {
            try? screenshot.pngRepresentation.write(to: URL(filePath: dir).appending(path: "\(name).png"))
        }
    }

    /// With `KIVO_SHOTS_DIR` set, writes what the screen holds there, for working out a step.
    private func dump(_ name: String) {
        guard let dir = environment["KIVO_SHOTS_DIR"] else { return }
        try? app.debugDescription.write(to: URL(filePath: dir).appending(path: "\(name).txt"), atomically: true, encoding: .utf8)
    }
}
