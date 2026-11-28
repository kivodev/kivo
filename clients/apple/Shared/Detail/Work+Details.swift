import Foundation
import KivoCore
import KivoUI

extension WorkDetail {
    /// The title with its edition, "Blade Runner · Final Cut", for a movie whose cut is named.
    var labelledTitle: String {
        movie?.edition.map { "\(title) · \($0)" } ?? title
    }
}

extension WorkSummary {
    /// The community rating as the details line shows it: "★ 8.2", absent without votes.
    var communityRatingText: String? {
        guard let rating = communityRating?.doubleValue, (communityVotes?.intValue ?? 0) > 0 else { return nil }
        return String(format: "★ %.1f", rating)
    }

    /// Whether it plays straight from a card: a movie, an episode, a video or a book.
    var playable: Bool {
        [WorkKind.movie, .episode, .video, .extra, .book].contains(kind) && available
    }

    /// An episode's synopsis on one line, without markup; nil when it has none.
    var synopsis: String? {
        guard let overview = episode?.overview else { return nil }
        let text = overview
            .replacingOccurrences(of: "<[^>]+>", with: " ", options: .regularExpression)
            .split(whereSeparator: \.isWhitespace)
            .joined(separator: " ")
        return text.isEmpty ? nil : text
    }

    /// Where the user is in it; nil unless it's in progress.
    var resumeMs: Int64? {
        guard let position = userState?.positionMs?.int64Value, position > 0, userState?.played != true else { return nil }
        return position
    }
}
