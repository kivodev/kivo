import Foundation
import KivoCore
import KivoUI

/// How the book player puts a book in words.
enum BookWords {
    /// The chapter's title, or "Chapter 12" for one without.
    static func chapter(_ progress: BookProgress) -> String {
        chapter(title: progress.chapterTitle, index: Int(progress.chapterIndex))
    }

    static func chapter(title: String?, index: Int) -> String {
        if let title, !title.isEmpty { return title }
        return String(localized: "Chapter \(index + 1)")
    }

    /// "Chapter 12 · 4 hr 21 min left", under the mini player's title.
    static func miniLine(_ progress: BookProgress) -> String {
        "\(String(localized: "Chapter \(progress.chapterIndex + 1)")) · \(Display.app.timeLeft(ms: progress.remainingMs))"
    }

    /// "Chapter 12 of 56 · 14h 2m left in the book".
    static func bookLine(_ progress: BookProgress) -> String {
        let place = String(localized: "Chapter \(progress.chapterIndex + 1) of \(progress.chapterCount)")
        let left = String(localized: "\(Display.app.runtime(ms: progress.remainingMs)) left in the book")
        return "\(place) · \(left)"
    }

    static func authors(_ work: WorkDetail) -> String? {
        guard let book = work.book, !book.authors.isEmpty else { return nil }
        return book.authors.map(\.name).joined(separator: ", ")
    }

    /// "1.25×".
    static func speed(_ speed: Double) -> String {
        String(format: "%g×", speed)
    }

    /// "15 minutes", or "End of chapter".
    static func sleep(_ choice: SleepChoice) -> String {
        switch onEnum(of: choice) {
        case let .minutes(m): String(localized: "\(m.minutes) minutes")
        case .endOfChapter: String(localized: "End of chapter")
        }
    }

}
