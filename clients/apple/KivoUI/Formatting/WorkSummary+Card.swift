import Foundation
import KivoCore

public extension WorkSummary {
    /// The title with its edition, "Blade Runner · Final Cut", for a movie whose cut is named.
    var labelledTitle: String {
        movie?.edition.map { "\(title) · \($0)" } ?? title
    }

    /// How far through it the user is, 0 to 1; nil when it isn't in progress.
    var progress: Double? {
        guard let position = userState?.positionMs?.intValue, position > 0,
              let duration = durationMs?.intValue, duration > 0,
              userState?.played != true
        else { return nil }
        return min(1, Double(position) / Double(duration))
    }

    /// The second line of a card other than an episode's: a season's episode count, a movie's year.
    var caption: String? {
        if let season {
            return season.episodeCount.map { String(localized: "\($0.intValue) episodes") }
        }
        if let book, !book.authors.isEmpty { return book.authors.map(\.name).joined(separator: ", ") }
        return timeLeft ?? year.map { String($0.intValue) }
    }

    /// What a grid card adds after the title when it fits: the year.
    var gridDetail: String? {
        year.map { String($0.intValue) }
    }

    /// "18 min left", for an item in progress.
    var timeLeft: String? {
        guard progress != nil, let position = userState?.positionMs?.intValue, let duration = durationMs?.intValue else { return nil }
        return Display.app.timeLeft(ms: Int64(duration - position))
    }

    /// The badge for a provisional item: Identifying or Unmatched.
    var matchBadge: String? {
        switch matchState {
        case "identifying": String(localized: "Identifying")
        case "unmatched": String(localized: "Unmatched")
        default: nil
        }
    }
}
