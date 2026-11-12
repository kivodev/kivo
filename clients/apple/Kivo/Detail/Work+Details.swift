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

    /// Where the user is in it; nil unless it's in progress.
    var resumeMs: Int64? {
        guard let position = userState?.positionMs?.int64Value, position > 0, userState?.played != true else { return nil }
        return position
    }
}
