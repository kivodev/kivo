import Foundation

/// The app's theme on this device: System follows iOS's appearance.
enum Theme: String {
    case system = "System"
    case dark = "Dark"
    case light = "Light"
}

/// How text subtitles the player draws look; styled ones keep their own.
enum SubtitleSize: String {
    case small = "Small"
    case medium = "Medium"
    case large = "Large"
}

enum SubtitleBackground: String {
    case none = "None"
    case shadow = "Shadow"
    case box = "Box"
}

/// Where this device's settings that only the app's screens follow are kept in UserDefaults.
enum AppSettingsKey {
    static let theme = "settings.theme"
    static let subtitleSize = "settings.subtitleSize"
    static let subtitleBackground = "settings.subtitleBackground"
    static let keepScreenOn = "settings.keepScreenOn"
}
