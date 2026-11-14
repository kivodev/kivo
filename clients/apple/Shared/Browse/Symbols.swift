/// The SF Symbols both Apple apps draw for the same things, as the Android
/// TV app draws its own icons for them.
enum Symbols {
    /// Watchlist: add, or on it.
    static func myList(_ on: Bool) -> String { on ? "text.badge.checkmark" : "text.badge.plus" }
    /// Mark as watched, filled once watched.
    static func watched(_ played: Bool) -> String { played ? "checkmark.circle.fill" : "checkmark.circle" }
    static let play = "play.fill"
    static let playFromBeginning = "arrow.counterclockwise"
    static let more = "ellipsis"
    static let audio = "speaker.wave.2"
    static let subtitles = "captions.bubble"
    static let version = "film"
    /// Playback settings: quality and version.
    static let playback = "gearshape"
}
