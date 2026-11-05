import Foundation
import KivoCore

public extension Display {
    /// The core's words for bitrates, sizes, runtimes, episode codes,
    /// quality choices, problems and tracks, in the language the app's
    /// text is in, as the core's client has them.
    static var app: Display {
        Display.companion.forLocale(tag: Bundle.main.preferredLocalizations.first ?? "en")
    }
}
