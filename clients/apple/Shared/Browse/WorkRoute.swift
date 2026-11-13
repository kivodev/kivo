import KivoCore
import SwiftUI

/// A detail page to open: a work's, or a person's, an author's, a book
/// series' or a collection's page, whose IDs aren't works'.
struct WorkRoute: Hashable {
    enum Page: Hashable {
        case work
        case person
        /// A person opened from an audiobooks library's authors.
        case author
        case bookSeries
        case collection
    }

    let id: String
    var page = Page.work
    /// The work's kind, when the link knows it, so its page's skeleton takes the page's shape.
    var kind: WorkKind?
    /// The card it was opened from, which its page shows until it's read.
    var seed: WorkSummary?
    /// The episode a series' page was opened from by Go to series, whose season it opens on.
    var episode: EpisodeInfo?

    /// A work card's page.
    init(_ work: WorkSummary) {
        id = work.id
        kind = work.kind
        seed = work
    }

    init(id: String, page: Page = .work, kind: WorkKind? = nil) {
        self.id = id
        self.page = page
        self.kind = kind
    }

    /// Go to series: the episode's series, open on the episode's season.
    init(seriesOf episode: EpisodeInfo) {
        id = episode.seriesId
        kind = .series
        self.episode = episode
    }
}

extension Library {
    /// The library's kind as an SF Symbol.
    var symbol: String {
        switch kind {
        case .movies: "film"
        case .shows: "tv"
        case .audiobooks: "headphones"
        case .videos, .unknown: "play.rectangle"
        }
    }
}

/// Pushes a work's page onto the tab's stack.
struct OpenWork {
    let open: (WorkRoute) -> Void

    func callAsFunction(_ route: WorkRoute) {
        open(route)
    }
}

extension EnvironmentValues {
    @Entry var openWork = OpenWork { _ in }
    /// The library a screen shows, so a long press doesn't offer to go to it.
    @Entry var currentLibraryId: String?
    /// The Home row a card is in, Continue Watching or Continue Listening,
    /// so a long press offers to remove it from that row.
    @Entry var continueRow: HomeRowKind?
    /// The list a card is shown in, so a long press offers to remove it from that list.
    @Entry var currentList: ListRef?
}
