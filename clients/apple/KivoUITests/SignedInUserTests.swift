import XCTest

/// Who is signed in shows without the server: after signing in, the app is
/// killed and opened again with the server unreachable, and Home's avatar
/// still names the user; once the server answers again it still does. Then
/// it signs out. It runs against the download test's server (FIRST-RUN.md,
/// The download test), whose proxy drops every connection between
/// `POST /__unreachable` and `POST /__reachable`, with the same environment.
final class SignedInUserTests: XCTestCase {
    private let app = XCUIApplication()
    private let environment = ProcessInfo.processInfo.environment

    override func setUpWithError() throws {
        continueAfterFailure = false
        // Every run signs in as one client, replacing the last run's session.
        app.launchEnvironment["KIVO_CLIENT_ID"] = "kivo-ui-tests"
        // Sign-in goes straight to the server under test, without browsing the network.
        app.launchEnvironment["KIVO_SERVER"] = environment["KIVO_DOWNLOAD_SERVER"]
    }

    func testTheAvatarShowsWithoutTheServer() throws {
        let server = try XCTUnwrap(environment["KIVO_DOWNLOAD_SERVER"], "set TEST_RUNNER_KIVO_DOWNLOAD_SERVER")
        let username = try XCTUnwrap(environment["KIVO_DOWNLOAD_USERNAME"], "set TEST_RUNNER_KIVO_DOWNLOAD_USERNAME")
        let avatar = app.buttons["user"]
        try signIn(server: server, username: username)
        expect(avatar, names: username, "after signing in")
        keep("ios-user-signed-in")

        app.terminate()
        proxy(server, "__unreachable")
        defer { proxy(server, "__reachable") }
        app.launch()
        // Without the server, the app opens on Downloads.
        XCTAssert(app.tabBars.buttons["Home"].waitForExistence(timeout: 30), "the app didn't open signed in")
        app.tabBars.buttons["Home"].tap()
        let unreachable = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "Can't reach")).firstMatch
        XCTAssert(unreachable.waitForExistence(timeout: 30), "Home doesn't say it can't reach the server")
        expect(avatar, names: username, "without the server")
        keep("ios-user-unreachable")

        proxy(server, "__reachable")
        XCTAssert(app.buttons["work"].firstMatch.waitForExistence(timeout: 60), "Home didn't fill in once the server answered")
        expect(avatar, names: username, "once the server answered")
        keep("ios-user-reachable")

        avatar.tap()
        app.buttons["account"].tap()
        app.buttons["Sign out"].tap()
        // Signing out asks first, in the question's own Sign out: Account's,
        // under it, comes first.
        let confirm = app.sheets.buttons["Sign out"]
        XCTAssert(confirm.waitForExistence(timeout: 5), "Sign out didn't ask first")
        keep("ios-user-sign-out-confirm")
        confirm.tap()
        let address = app.textFields["address"]
        let user = app.textFields["username"]
        let signIn = NSPredicate { _, _ in address.exists || user.exists }
        wait(for: [expectation(for: signIn, evaluatedWith: nil)], timeout: 15)
    }

    /// Settings names the user, and a wrong current password in Change
    /// password says so under its field and keeps the app signed in.
    func testAWrongCurrentPasswordKeepsTheAppSignedIn() throws {
        let server = try XCTUnwrap(environment["KIVO_DOWNLOAD_SERVER"], "set TEST_RUNNER_KIVO_DOWNLOAD_SERVER")
        let username = try XCTUnwrap(environment["KIVO_DOWNLOAD_USERNAME"], "set TEST_RUNNER_KIVO_DOWNLOAD_USERNAME")
        try signIn(server: server, username: username)
        app.buttons["user"].tap()
        XCTAssert(app.staticTexts[username].waitForExistence(timeout: 10), "Settings doesn't name the user")
        keep("ios-user-settings")

        app.buttons["account"].tap()
        let change = app.buttons["Change password…"]
        XCTAssert(change.waitForExistence(timeout: 5), "Account has no Change password…")
        change.tap()
        let current = app.secureTextFields["current-password"]
        XCTAssert(current.waitForExistence(timeout: 5), "Change password has no current password")
        current.typeText("not the password")
        let new = app.secureTextFields["new-password"]
        new.tap()
        new.typeText("a new password")
        app.buttons["Change"].tap()
        XCTAssert(app.staticTexts["That isn't your current password."].waitForExistence(timeout: 10), "a wrong password didn't say so")
        keep("ios-user-wrong-password")

        app.buttons["Cancel"].tap()
        app.navigationBars.buttons["Settings"].tap()
        app.buttons["Done"].tap()
        XCTAssert(app.buttons["user"].waitForExistence(timeout: 10), "a wrong password signed the app out")
        XCTAssertFalse(app.textFields["address"].exists || app.textFields["username"].exists, "a wrong password signed the app out")
    }

    /// Asserts that [avatar] is the user's: VoiceOver reads their name, which its initial is drawn from.
    private func expect(_ avatar: XCUIElement, names username: String, _ when: String) {
        XCTAssert(avatar.waitForExistence(timeout: 10), "Home has no avatar \(when)")
        let named = NSPredicate { _, _ in avatar.value as? String == username }
        wait(for: [expectation(for: named, evaluatedWith: nil)], timeout: 10)
    }

    /// Signs in to [server], which sign-in opens on, unless the app still is.
    private func signIn(server: String, username: String) throws {
        let password = try XCTUnwrap(environment["KIVO_DOWNLOAD_PASSWORD"], "set TEST_RUNNER_KIVO_DOWNLOAD_PASSWORD")
        app.launch()
        let address = app.textFields["address"]
        let user = app.textFields["username"]
        let home = app.buttons["user"]
        let start = NSPredicate { _, _ in address.exists || user.exists || home.exists }
        wait(for: [expectation(for: start, evaluatedWith: nil)], timeout: 20)
        if home.exists { return }
        XCTAssert(user.waitForExistence(timeout: 15), "the server at \(server) wasn't found")
        user.tap()
        user.typeText(username)
        let secret = app.secureTextFields["password"]
        secret.tap()
        secret.typeText(password)
        app.buttons["Sign in"].tap()
        XCTAssert(home.waitForExistence(timeout: 20), "Home didn't show after signing in")
        let notNow = XCUIApplication(bundleIdentifier: "com.apple.SafariViewService").buttons["Not Now"]
        if notNow.waitForExistence(timeout: 5) { notNow.tap() }
    }

    /// Asks the proxy in front of the server for [path]: `__unreachable` or `__reachable`.
    private func proxy(_ server: String, _ path: String) {
        var request = URLRequest(url: URL(string: server + "/" + path)!)
        request.httpMethod = "POST"
        let done = expectation(description: path)
        URLSession.shared.dataTask(with: request) { _, _, _ in done.fulfill() }.resume()
        wait(for: [done], timeout: 10)
    }

    private func keep(_ name: String) {
        let shot = XCTAttachment(screenshot: XCUIScreen.main.screenshot())
        shot.name = name
        shot.lifetime = .keepAlways
        add(shot)
    }
}
