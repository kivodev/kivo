import Foundation

/// What the environment the app was launched with asks of it.
enum Launch {
    /// Whether the app hosts the unit tests, which XCTest says in the
    /// environment it injects; then the app starts nothing of its own.
    static func isTestHost(_ environment: [String: String]) -> Bool {
        environment["XCTestConfigurationFilePath"] != nil || environment["XCTestBundlePath"] != nil
    }

    /// The server `KIVO_SERVER` names, which sign-in connects to in place of
    /// browsing the network, in a Debug build.
    static func server(_ environment: [String: String]) -> String? {
        #if DEBUG
        guard let address = environment["KIVO_SERVER"]?.trimmingCharacters(in: .whitespacesAndNewlines), !address.isEmpty else { return nil }
        return address
        #else
        return nil
        #endif
    }

    static var isTestHost: Bool { isTestHost(ProcessInfo.processInfo.environment) }
    static var server: String? { server(ProcessInfo.processInfo.environment) }
}

/// The app, or an empty window while it hosts the unit tests: no session,
/// no discovery and no Cast.
@main
enum KivoMain {
    static func main() {
        if Launch.isTestHost {
            TestHostApp.main()
        } else {
            KivoApp.main()
        }
    }
}

private struct TestHostApp: App {
    var body: some Scene {
        WindowGroup { EmptyView() }
    }
}
