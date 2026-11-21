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
    static func miniLine(_ progress: BookProgress, speed: Double) -> String {
        "\(String(localized: "Chapter \(progress.chapterIndex + 1)")) · \(Display.app.timeLeft(ms: listeningMs(progress.remainingMs, speed: speed)))"
    }

    /// "Chapter 12 of 56 · 14h 2m left in the book", or "7h 1m left at 2×" at another speed.
    static func bookLine(_ progress: BookProgress, speed: Double) -> String {
        let place = String(localized: "Chapter \(progress.chapterIndex + 1) of \(progress.chapterCount)")
        let time = Display.app.runtime(ms: listeningMs(progress.remainingMs, speed: speed))
        let left = speed == 1
            ? String(localized: "\(time) left in the book")
            : String(localized: "\(time) left at \(BookWords.speed(speed))")
        return "\(place) · \(left)"
    }

    /// "15h 2m left of 19h 38m", on the page of a book in progress.
    static func leftOf(positionMs: Int64, durationMs: Int64) -> String {
        String(localized: "\(Display.app.runtime(ms: max(0, durationMs - positionMs))) left of \(Display.app.runtime(ms: durationMs))")
    }

    /// How long a stretch of the book takes to hear at a speed.
    private static func listeningMs(_ ms: Int64, speed: Double) -> Int64 {
        speed > 0 ? Int64(Double(ms) / speed) : ms
    }

    static func authors(_ work: WorkDetail) -> String? {
        guard let book = work.book, !book.authors.isEmpty else { return nil }
        return book.authors.map(\.name).joined(separator: ", ")
    }

    /// "1.25×".
    static func speed(_ speed: Double) -> String {
        "\(speed.formatted(.number.precision(.fractionLength(0...2))))×"
    }

    /// "15 minutes", or "End of chapter".
    static func sleep(_ choice: SleepChoice) -> String {
        switch onEnum(of: choice) {
        case let .minutes(m): String(localized: "\(m.minutes) minutes")
        case .endOfChapter: String(localized: "End of chapter")
        }
    }

    /// "15 more minutes", or "another chapter": what Play restarts the sleep timer for.
    static func sleepAgain(_ choice: SleepChoice) -> String {
        switch onEnum(of: choice) {
        case let .minutes(m): String(localized: "\(m.minutes) more minutes")
        case .endOfChapter: String(localized: "another chapter")
        }
    }

}
