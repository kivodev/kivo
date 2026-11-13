import Foundation
import SwiftUI

/// What the environment the app was launched with asks of it.
enum Launch {
    /// Whether the app hosts the unit tests, which XCTest says in the
    /// environment it injects; then the app starts nothing of its own.
    static var isTestHost: Bool {
        let environment = ProcessInfo.processInfo.environment
        return environment["XCTestConfigurationFilePath"] != nil || environment["XCTestBundlePath"] != nil
    }

    /// The server `KIVO_SERVER` names, which sign-in connects to in place of
    /// browsing the network, in a Debug build.
    static var server: String? {
        #if DEBUG
        guard let address = ProcessInfo.processInfo.environment["KIVO_SERVER"]?.trimmingCharacters(in: .whitespacesAndNewlines), !address.isEmpty else { return nil }
        return address
        #else
        return nil
        #endif
    }
}

/// The empty window an app shows while it hosts the unit tests: no
/// session, no discovery.
struct TestHostApp: App {
    var body: some Scene {
        WindowGroup { EmptyView() }
    }
}
