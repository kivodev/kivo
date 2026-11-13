import UIKit

/// This device in the app's words: "This iPhone", "This iPad" or "This Apple TV".
enum ThisDevice {
    static var name: String {
        switch UIDevice.current.userInterfaceIdiom {
        case .pad: String(localized: "This iPad")
        case .tv: String(localized: "This Apple TV")
        default: String(localized: "This iPhone")
        }
    }

    /// The question before signing this device out.
    static var signOutQuestion: String {
        switch UIDevice.current.userInterfaceIdiom {
        case .pad: String(localized: "Sign out of this iPad?")
        case .tv: String(localized: "Sign out of this Apple TV?")
        default: String(localized: "Sign out of this iPhone?")
        }
    }
}
