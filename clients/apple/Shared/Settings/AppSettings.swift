import Foundation

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
    static let subtitleSize = "settings.subtitleSize"
    static let subtitleBackground = "settings.subtitleBackground"
}
