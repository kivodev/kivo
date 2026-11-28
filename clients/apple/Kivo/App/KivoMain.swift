import SwiftUI

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
