import Foundation

/// The app's theme on this device: System follows iOS's appearance.
enum Theme: String {
    case system = "System"
    case dark = "Dark"
    case light = "Light"
}

extension AppSettingsKey {
    static let theme = "settings.theme"
    static let keepScreenOn = "settings.keepScreenOn"
}
