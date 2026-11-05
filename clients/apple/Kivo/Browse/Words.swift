import Foundation

/// Words that depend on what a work is.
enum Words {
    /// "Mark as watched", or for a book "Mark as finished"; [played] false for the inverse.
    static func markPlayed(_ played: Bool, kind: String) -> String {
        let book = kind == "book"
        switch (played, book) {
        case (true, false): return String(localized: "Mark as watched")
        case (false, false): return String(localized: "Mark as unwatched")
        case (true, true): return String(localized: "Mark as finished")
        case (false, true): return String(localized: "Mark as unfinished")
        }
    }

    /// "Ends at 9:41 PM", in this locale's time format, for a movie or an
    /// episode with [remainingMs] left from [now]; nil for anything else.
    static func endsAt(kind: String, remainingMs: Int64, from now: Date) -> String? {
        guard kind == "movie" || kind == "episode", remainingMs > 0 else { return nil }
        let end = now.addingTimeInterval(Double(remainingMs) / 1000)
        return String(localized: "Ends at \(end.formatted(date: .omitted, time: .shortened))")
    }

    /// A credit's role as people read it: Actor, Director, Author.
    static func role(_ role: String) -> String {
        switch role {
        case "actor": return String(localized: "Actor")
        case "director": return String(localized: "Director")
        case "writer": return String(localized: "Writer")
        case "author": return String(localized: "Author")
        case "narrator": return String(localized: "Narrator")
        default: return role.capitalized
        }
    }
}
