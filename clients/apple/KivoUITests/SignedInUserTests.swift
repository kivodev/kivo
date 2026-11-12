import XCTest

/// A server that can't be reached as the app opens says so: after signing
/// in, the app is killed and opened again with the server unreachable, and
/// it names the server it can't reach, with Retry and Change server; once
/// the server answers again, Retry opens it, and Home's avatar names the
/// user. Then it signs out. It runs against the download test's server (FIRST-RUN.md,
/// The download test), whose proxy drops every connection between
/// `POST /__unreachable` and `POST /__reachable`, with the same environment.
final class SignedInUserTests: XCTestCase {
    private let app = XCUIApplication()

    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    func testAServerThatCantBeReachedSaysSoUntilRetryOpensIt() throws {
        let server = try TestServer.download.address
        let username = try TestServer.download.username
        let avatar = app.buttons["user"]
        try signIn(app, at: .download)
        expect(avatar, names: username, "after signing in")
        keep("ios-user-signed-in")

        app.terminate()
        proxy(server, "__unreachable")
        defer { proxy(server, "__reachable") }
        app.launch()
        // Without the server, and with no downloads of it, the app says it can't reach it.
        let unreachable = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "Can't reach")).firstMatch
        XCTAssert(unreachable.waitForExistence(timeout: 60), "the app doesn't say it can't reach its server")
        XCTAssert(app.buttons["Change server"].exists, "a server that can't be reached has no Change server")
        keep("ios-user-unreachable")

        proxy(server, "__reachable")
        app.buttons["Retry"].tap()
        XCTAssert(app.buttons["work"].firstMatch.waitForExistence(timeout: 60), "Retry didn't open the server once it answered")
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
        let username = try TestServer.download.username
        try signIn(app, at: .download)
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

    /// Asks the proxy in front of the server for [path]: `__unreachable` or `__reachable`.
    private func proxy(_ server: String, _ path: String) {
        var request = URLRequest(url: URL(string: server + "/" + path)!)
        request.httpMethod = "POST"
        let done = expectation(description: path)
        URLSession.shared.dataTask(with: request) { _, _, _ in done.fulfill() }.resume()
        wait(for: [done], timeout: 10)
    }
}
