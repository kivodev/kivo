import Foundation
import KivoCore
import Observation

/// Signing in on the phone: find the server (the one signed in to last,
/// Bonjour on this network, a typed address, or a scanned server-address
/// QR code), then a username and password, or a code another signed-in
/// device approves, which make this app's session, named after the
/// device's model ("iPhone 17 Pro").
@MainActor
@Observable
final class SignInModel {
    enum Step {
        case findServer
        case password(FoundServer)
        /// The app and the server don't work together: one of them needs an update.
        case incompatible(FoundServer, Incompatibility)
    }

    /// Where a server found goes, by how the app and it stand: its password
    /// when they work together, else why they don't.
    static func step(for found: FoundServer, _ compatibility: Compatibility) -> Step {
        Incompatibility(compatibility, server: found.info.name).map { .incompatible(found, $0) } ?? .password(found)
    }

    let saved: SavedServer?
    let browser: ServerBrowser
    var step: Step = .findServer
    /// Whether the code screen ("Sign in with another device") shows.
    var linking = false
    private(set) var discovering = false
    var address = ""
    var username = ""
    var password = ""
    private(set) var busy = false
    /// What went wrong, in a sentence.
    private(set) var message: String?

    private let app: AppModel

    init(app: AppModel, saved: SavedServer?) {
        self.app = app
        self.saved = saved
        browser = ServerBrowser(core: app.core)
    }

    /// The servers Bonjour found.
    var discovered: [FoundServer] { browser.found }

    /// Browses for servers; one found within 3 seconds, while nothing else
    /// was chosen, goes straight to its password. The browse goes on while
    /// sign-in shows. A server the launch names is connected to instead.
    func discover() async {
        if let server = Launch.server {
            address = server
            await connect()
            return
        }
        guard !discovering else { return }
        discovering = true
        browser.start()
        try? await Task.sleep(for: .seconds(3))
        discovering = false
        if saved == nil, discovered.count == 1, address.isEmpty, case .findServer = step {
            choose(discovered[0])
        }
    }

    func stopDiscovering() {
        browser.stop()
    }

    func choose(_ found: FoundServer) {
        message = nil
        step = Self.step(for: found, app.core.compatibility(info: found.info))
    }

    /// Asks the server the app doesn't work with what it is again; it stays as it is when it doesn't answer.
    func retryCompatibility() async {
        guard case let .incompatible(found, _) = step else { return }
        await run {
            if let again = try? await self.app.core.serverAt(address: found.baseUrl) { self.choose(again) }
        }
    }

    /// The server signed in to last, at the first of its addresses that answers as it.
    func chooseSaved() async {
        guard let saved else { return }
        await run {
            for address in saved.addresses {
                if let found = try? await self.app.core.serverAt(address: address), found.info.id == saved.id {
                    self.username = ""
                    self.choose(found)
                    return
                }
            }
            self.message = String(localized: "Can't reach \(saved.name). Check your connection or enter its address.")
        }
    }

    /// The server at the typed address: `http://` and port 8484 when it names none.
    func connect() async {
        let typed = address.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !typed.isEmpty else { return }
        await run {
            do {
                self.choose(try await self.app.core.serverAt(address: typed))
            } catch {
                self.message = String(localized: "No server found at that address.")
            }
        }
    }

    /// The server a scanned QR code names: a server's address, or the
    /// address of another device's `/link` page.
    func scanned(_ text: String) async {
        guard var url = URLComponents(string: text), url.scheme == "http" || url.scheme == "https" else {
            message = String(localized: "That QR code isn't a server address.")
            return
        }
        url.query = nil
        if url.path.hasSuffix("/link") { url.path.removeLast("/link".count) }
        address = url.string ?? text
        await connect()
    }

    func signIn(to found: FoundServer) async {
        await run {
            do {
                let server = try await self.app.core.signIn(
                    found: found, username: self.username, password: self.password, name: AppleIdentity.modelName
                )
                self.password = ""
                self.app.core.signedIn(session: server)
            } catch {
                self.message = error.problemSentence
            }
        }
    }

    func back() {
        step = .findServer
        message = nil
        password = ""
    }

    private func run(_ work: @escaping () async throws -> Void) async {
        guard !busy else { return }
        busy = true
        message = nil
        do {
            try await work()
        } catch {
            message = error.problemSentence
        }
        busy = false
    }
}
