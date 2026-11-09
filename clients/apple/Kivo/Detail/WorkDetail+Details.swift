import Foundation
import KivoCore
import KivoUI

extension WorkDetail {
    /// A page from what a list showed of it, until the page is read: the
    /// summary's fields, with the lists only the page has empty.
    convenience init(summary: WorkSummary) {
        self.init(
            id: summary.id, kind: summary.kind, title: summary.title, matchState: summary.matchState, available: summary.available,
            images: summary.images, genres: [], studios: [], credits: [], externalIds: [], versions: [], extras: [], collections: [],
            year: summary.year, durationMs: summary.durationMs, communityRating: summary.communityRating,
            communityVotes: summary.communityVotes, contentRating: summary.contentRating, addedAt: summary.addedAt,
            userState: summary.userState, movie: summary.movie, series: summary.series, season: summary.season,
            episode: summary.episode, extra: summary.extra, book: summary.book,
            libraryId: summary.libraryId, path: nil, originalTitle: nil,
            sortTitle: nil, overview: nil, remembersTracks: nil, listIds: nil, lockedFields: nil
        )
    }

    /// A season's series, as much as the season's summary says of it: its
    /// title, the artwork the season shows (its series' where it has none of
    /// its own), its content rating and its library.
    convenience init(seriesOf season: WorkSummary) {
        self.init(
            id: season.season?.seriesId ?? season.id, kind: "series", title: season.season?.seriesTitle ?? season.title,
            matchState: season.matchState, available: season.available, images: season.images, genres: [], studios: [],
            credits: [], externalIds: [], versions: [], extras: [], collections: [], year: nil, durationMs: nil,
            communityRating: nil, communityVotes: nil, contentRating: season.contentRating, addedAt: nil, userState: nil,
            movie: nil, series: nil, season: nil, episode: nil, extra: nil, book: nil,
            libraryId: season.libraryId, path: nil, originalTitle: nil, sortTitle: nil, overview: nil,
            remembersTracks: nil, listIds: nil, lockedFields: nil
        )
    }

    /// The title with its edition, "Blade Runner · Final Cut", for a movie whose cut is named.
    var labelledTitle: String {
        movie?.edition.map { "\(title) · \($0)" } ?? title
    }

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

    /// "1 hr 36 min left", while it's in progress.
    var timeLeft: String? {
        guard let resumeMs, let duration = durationMs?.int64Value, duration > resumeMs else { return nil }
        return Display.app.timeLeft(ms: duration - resumeMs)
    }
}
