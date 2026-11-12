import XCTest

/// The server a test signs in to and its account, from the environment
/// `xcodebuild test` passes in with the `TEST_RUNNER_` prefix (FIRST-RUN.md):
/// `KIVO_SERVER`, `KIVO_USERNAME` and `KIVO_PASSWORD`, or for the download
/// test's server, `KIVO_DOWNLOAD_SERVER` and the rest.
enum TestServer: String {
    case main = "KIVO"
    case download = "KIVO_DOWNLOAD"

    var address: String { get throws { try value("SERVER") } }
    var username: String { get throws { try value("USERNAME") } }
    var password: String { get throws { try value("PASSWORD") } }

    private func value(_ name: String) throws -> String {
        let key = "\(rawValue)_\(name)"
        return try XCTUnwrap(ProcessInfo.processInfo.environment[key], "set TEST_RUNNER_\(key)")
    }
}

extension XCUIApplication {
    /// Home's tab or sidebar item, in either layout.
    var homeTab: XCUIElement {
        descendants(matching: .any).matching(NSPredicate(format: "label == %@", "Home")).firstMatch
    }
}

extension XCTestCase {
    /// Launches [app] at [server] and signs in, unless the Keychain still
    /// holds a sign-in from a last run. Home shows by [home], the user
    /// button unless given. With [allowingLocalNetwork], a first run's
    /// question about finding devices on the local network is answered.
    func signIn(
        _ app: XCUIApplication,
        at server: TestServer = .main,
        home: XCUIElement? = nil,
        allowingLocalNetwork: Bool = false
    ) throws {
        let address = try server.address
        // Every run signs in as one client, replacing the last run's session.
        app.launchEnvironment["KIVO_CLIENT_ID"] = "kivo-ui-tests"
        // Sign-in goes straight to the server under test, without browsing the network.
        app.launchEnvironment["KIVO_SERVER"] = address
        app.launch()
        if allowingLocalNetwork {
            let allow = XCUIApplication(bundleIdentifier: "com.apple.springboard").buttons["Allow"]
            if allow.waitForExistence(timeout: 3) { allow.tap() }
        }
        let field = app.textFields["address"]
        let user = app.textFields["username"]
        let home = home ?? app.buttons["user"]
        let start = NSPredicate { _, _ in field.exists || user.exists || home.exists }
        wait(for: [expectation(for: start, evaluatedWith: nil)], timeout: 20)
        if home.exists { return }
        XCTAssert(user.waitForExistence(timeout: 15), "the server at \(address) wasn't found")
        user.tap()
        user.typeText(try server.username)
        let secret = app.secureTextFields["password"]
        secret.tap()
        secret.typeText(try server.password + "\n")
        XCTAssert(home.waitForExistence(timeout: 20), "Home didn't show after signing in")
        // iOS offers to save the password, from Safari's view service. The
        // offer may go by itself between finding its button and tapping it.
        let notNow = XCUIApplication(bundleIdentifier: "com.apple.SafariViewService").buttons["Not Now"]
        if notNow.waitForExistence(timeout: 5) {
            XCTExpectFailure("the offer went by itself", options: .nonStrict()) { notNow.tap() }
        }
    }

    /// Keeps the screen in the result bundle, and with `KIVO_SHOTS_DIR`
    /// set, writes it there as a PNG too.
    func keep(_ name: String) {
        let screenshot = XCUIScreen.main.screenshot()
        let shot = XCTAttachment(screenshot: screenshot)
        shot.name = name
        shot.lifetime = .keepAlways
        add(shot)
        if let dir = ProcessInfo.processInfo.environment["KIVO_SHOTS_DIR"] {
            try? screenshot.pngRepresentation.write(to: URL(filePath: dir).appending(path: "\(name).png"))
        }
    }
}
