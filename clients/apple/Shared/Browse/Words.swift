import Foundation
import KivoCore

/// Words that depend on what a work is.
enum Words {
    /// "Mark as watched", or for a book "Mark as finished"; [played] false for the inverse.
    static func markPlayed(_ played: Bool, kind: WorkKind) -> String {
        let book = kind == .book
        switch (played, book) {
        case (true, false): return String(localized: "Mark as watched")
        case (false, false): return String(localized: "Mark as unwatched")
        case (true, true): return String(localized: "Mark as finished")
        case (false, true): return String(localized: "Mark as unfinished")
        }
    }

    /// "Ends at 9:41 PM", in this locale's time format, for a movie or an
    /// episode with [remainingMs] left from [now]; nil for anything else.
    static func endsAt(kind: WorkKind, remainingMs: Int64, from now: Date) -> String? {
        guard kind == .movie || kind == .episode, remainingMs > 0 else { return nil }
        let end = now.addingTimeInterval(Double(remainingMs) / 1000)
        return String(localized: "Ends at \(end.formatted(date: .omitted, time: .shortened))")
    }

    /// A credit's role as people read it: Actor, Director, Author; one the
    /// app has no words for is [role] as the server sent it, capitalized.
    static func role(_ known: CreditRole, _ role: String) -> String {
        switch known {
        case .actor: return String(localized: "Actor")
        case .director: return String(localized: "Director")
        case .writer: return String(localized: "Writer")
        case .author: return String(localized: "Author")
        case .narrator: return String(localized: "Narrator")
        case .unknown: return role.capitalized
        }
    }
}

/// A Home row's heading, the same on Home and in Settings.
func homeRowTitle(kind: HomeRowKind, libraryName: String?, listName: String?) -> String {
    switch kind {
    case .continueWatching: String(localized: "Continue Watching")
    case .continueListening: String(localized: "Continue Listening")
    case .list: listName ?? ""
    case .recentlyAdded, .unknown:
        if let libraryName {
            String(localized: "Recently Added in \(libraryName)")
        } else {
            String(localized: "Recently Added")
        }
    }
}
