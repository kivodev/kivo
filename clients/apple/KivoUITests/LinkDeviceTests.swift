import UIKit
import XCTest

/// Link a device, from both sides, against the download test's server
/// (FIRST-RUN.md, The download test), with the same environment. The other
/// device is the test itself, through the API: it starts a link as a TV
/// would and polls it, or approves the app's code with a session of its
/// own. The app approves a typed code, denies one, warns of a device
/// asking from outside the home network, refuses a scanned code of another
/// server, and signs itself in with a code. Its screenshots are named
/// `ios-link-…`.
final class LinkDeviceTests: XCTestCase {
    private let app = XCUIApplication()
    private var server = ""
    private var username = ""
    private var password = ""

    override func setUpWithError() throws {
        continueAfterFailure = false
        server = try TestServer.download.address
        username = try TestServer.download.username
        password = try TestServer.download.password
    }

    func testAllowATypedCode() throws {
        try signIn(app, at: .download)
        let link = try startLink()
        openLinkADevice()
        enter(code: link.code)
        XCTAssert(app.staticTexts["Allow Living Room to sign in as \(username)?"].waitForExistence(timeout: 10), "no question")
        XCTAssertEqual(app.staticTexts["link-context"].label, "Android TV · On your home network · Just now")
        XCTAssertFalse(app.staticTexts["link-warning"].exists, "a nearby device was warned of")
        keep("ios-link-confirm")
        app.buttons["Allow"].tap()
        XCTAssert(app.staticTexts["Living Room is signed in"].waitForExistence(timeout: 10), "Allow didn't say so")
        keep("ios-link-allowed")
        let (status, body) = try call("POST", "device-codes/poll", ["secret": link.secret])
        XCTAssertEqual(status, 201, "the TV's poll after Allow")
        XCTAssertEqual(body["platform"] as? String, "androidTv")
    }

    func testDontAllow() throws {
        try signIn(app, at: .download)
        let link = try startLink()
        openLinkADevice()
        enter(code: link.code)
        let deny = app.buttons["Don't allow"]
        XCTAssert(deny.waitForExistence(timeout: 10), "no Don't allow")
        deny.tap()
        XCTAssert(app.staticTexts["Living Room wasn't signed in"].waitForExistence(timeout: 10), "Don't allow didn't say so")
        keep("ios-link-denied")
        let (status, body) = try call("POST", "device-codes/poll", ["secret": link.secret])
        XCTAssertEqual(status, 403, "the TV's poll after Don't allow")
        XCTAssertEqual(body["code"] as? String, "denied")
    }

    /// With local networks that leave out loopback, the test's own requests
    /// come from outside the home network.
    func testADeviceAwayFromHomeIsWarnedOf() throws {
        try signIn(app, at: .download)
        let token = try apiToken()
        XCTAssertEqual(try call("PATCH", "settings", ["localNetworks": ["203.0.113.0/24"]], token: token).0, 200)
        defer { _ = try? call("PATCH", "settings", ["localNetworks": [String]()], token: token) }
        let link = try startLink()
        openLinkADevice()
        enter(code: link.code)
        XCTAssert(app.staticTexts["link-context"].waitForExistence(timeout: 10), "no context")
        XCTAssertEqual(app.staticTexts["link-context"].label, "Android TV · Not on your home network · Just now")
        XCTAssert(app.descendants(matching: .any)["link-warning"].exists, "no warning for a device away from home")
        keep("ios-link-away")
        app.buttons["Don't allow"].tap()
        XCTAssert(app.staticTexts["Living Room wasn't signed in"].waitForExistence(timeout: 10))
    }

    /// The Simulator has no camera: the launch argument is what the scanner reads.
    func testAScannedCodeOfAnotherServer() throws {
        app.launchArguments += ["-KivoScan", "http://elsewhere:8484/link?code=K7Q2M9&server=01900000-0000-7000-8000-000000000000"]
        try signIn(app, at: .download)
        openLinkADevice()
        XCTAssert(app.staticTexts["This device is signing in to another server."].waitForExistence(timeout: 10), "a code of another server wasn't refused")
        XCTAssertFalse(app.buttons["Allow"].exists, "a code of another server can be allowed")
        keep("ios-link-other-server")
    }

    /// Signed out, the app shows a code, which the test approves, and lands
    /// on Home, its session named after the device's model.
    func testSignInWithAnotherDevice() throws {
        try signIn(app, at: .download)
        signOut()
        XCTAssert(app.textFields["username"].waitForExistence(timeout: 15), "the server at \(server) wasn't found")
        let other = app.buttons["sign-in-with-another-device"]
        XCTAssert(other.waitForExistence(timeout: 5), "no Sign in with another device")
        other.tap()
        let code = app.staticTexts["link-code"]
        XCTAssert(code.waitForExistence(timeout: 15), "no code")
        keep("ios-link-code")
        let token = try apiToken()
        XCTAssertEqual(try call("POST", "device-codes/\(code.label)/approve", nil, token: token).0, 204)
        XCTAssert(app.buttons["user"].waitForExistence(timeout: 20), "Home didn't show after the code was approved")
        let (_, sessions) = try call("GET", "sessions", nil, token: token)
        let items = sessions["items"] as? [[String: Any]] ?? []
        let platform = UIDevice.current.userInterfaceIdiom == .pad ? "iPad" : "iPhone"
        // "iPhone 17 Pro", or "iPhone" for a model the app doesn't know.
        XCTAssert(items.contains { ($0["name"] as? String)?.hasPrefix(platform) == true && ($0["device"] as? [String: Any])?["platform"] as? String == platform }, "no session named after the model: \(items)")
    }

    // MARK: - The app

    private func openLinkADevice() {
        app.buttons["user"].tap()
        let row = app.buttons["Link a device"]
        XCTAssert(row.waitForExistence(timeout: 10), "Settings has no Link a device")
        row.tap()
    }

    private func enter(code: String) {
        let field = app.textFields["link-code-field"]
        XCTAssert(field.waitForExistence(timeout: 10), "no code field")
        field.tap()
        // As people type it: lower case, with a space.
        field.typeText(code.prefix(3).lowercased() + " " + code.suffix(3).lowercased())
        app.buttons["Continue"].tap()
    }

    private func signOut() {
        app.buttons["user"].tap()
        app.buttons["account"].tap()
        app.buttons["Sign out"].tap()
        // The question's own Sign out: Account's, under it, comes first.
        let confirm = app.sheets.buttons["Sign out"]
        XCTAssert(confirm.waitForExistence(timeout: 5), "Sign out didn't ask first")
        confirm.tap()
    }

    // MARK: - The other device, through the API

    private struct Link {
        let code: String
        let secret: String
    }

    /// A link started as a TV starts one.
    private func startLink() throws -> Link {
        let (status, body) = try call("POST", "device-codes", ["name": "Living Room"], platform: "androidTv")
        XCTAssertEqual(status, 201)
        return Link(code: try XCTUnwrap(body["code"] as? String), secret: try XCTUnwrap(body["secret"] as? String))
    }

    /// A session of the test's own, as another signed-in device.
    private func apiToken() throws -> String {
        let (status, body) = try call("POST", "sessions", [
            "username": username, "password": password, "kind": "app", "name": "Link tests", "clientId": "kivo-link-tests",
        ])
        XCTAssertEqual(status, 201)
        return try XCTUnwrap(body["token"] as? String)
    }

    private func call(_ method: String, _ path: String, _ body: [String: Any]?, token: String? = nil, platform: String? = nil) throws -> (Int, [String: Any]) {
        var request = URLRequest(url: URL(string: "\(server)/api/v1/\(path)")!)
        request.httpMethod = method
        if let body {
            request.httpBody = try JSONSerialization.data(withJSONObject: body)
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        if let token { request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization") }
        if let platform { request.setValue(platform, forHTTPHeaderField: "Kivo-Platform") }
        var result: (Int, [String: Any]) = (0, [:])
        let done = expectation(description: "\(method) \(path)")
        URLSession.shared.dataTask(with: request) { data, response, _ in
            let json = data.flatMap { try? JSONSerialization.jsonObject(with: $0) } as? [String: Any]
            result = ((response as? HTTPURLResponse)?.statusCode ?? 0, json ?? [:])
            done.fulfill()
        }.resume()
        wait(for: [done], timeout: 15)
        return result
    }
}
