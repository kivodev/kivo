import Foundation
import KivoCore
import KivoUI
import Observation

/// A library's grid as its screen draws it: the core's `LibraryGrid`, which
/// holds the tab, sort and filters, what the library offers and the grid
/// they choose, with the paged grid (``PagedGrid``) of the tab showing and
/// the words for each. A new tab, sort or filter shows at once, its
/// placeholders until its first page arrives; a change the live events
/// report, a pull or coming back reads the pages again in place, keeping
/// the place. The first page waits to show until it's known whether the
/// library has collections, so it comes with the chips it brings.
@MainActor
@Observable
final class GridModel {
    /// A sort a sort menu offers: a library's [GridSort] or a list's [ListSort].
    struct Sort<Key: SortMenuKey>: Hashable, Identifiable {
        let value: Key
        let label: String
        /// The direction it sorts in: "A to Z", "Newest first".
        let direction: String
        var id: Key { value }

        /// The sort in a word or two, for the count line: "A–Z", "Recently added".
        var short: String { value.short ?? label }
    }

    /// A section of the library, one of its chips.
    struct Section: Identifiable {
        let tab: LibraryTab
        let label: String
        var id: String { tab.value }
    }

    let library: Library
    /// What's chosen and what the library offers, as the core has it.
    private(set) var state: LibraryGridState
    /// The grid showing, [state]'s: its count, letters and pages.
    private(set) var paged: PagedGrid
    /// Counts the grids shown, a new one with each tab, sort or filter.
    private(set) var generation = 0

    /// Following [paged] while the grid runs.
    @ObservationIgnored private var following: Task<Void, Never>?
    @ObservationIgnored private var running = false
    @ObservationIgnored private var hasRun = false
    @ObservationIgnored private let grid: LibraryGrid

    /// Opens [library] with the tab, sort and filters this device last used
    /// in it, or with its works in [sort] alone for once, and its first page
    /// if it was seen lately.
    init(library: Library, session: KivoSession, sort: GridSort? = nil) {
        self.library = library
        let grid = session.library.libraryGrid(libraryId: library.id, oneOffSort: sort, settleMs: PagedGrid.settleMs)
        let state = grid.state.value
        self.grid = grid
        self.state = state
        paged = Self.paged(state)
        holdFirstPage()
    }

    deinit {
        following?.cancel()
        grid.close()
    }

    var isBooks: Bool { library.kind == .audiobooks }

    /// The shape of the cards on the section showing.
    var style: CardStyle { Self.style(state) }

    private static func style(_ state: LibraryGridState) -> CardStyle {
        switch state.cardShape {
        case .poster: .poster
        case .landscape: .landscape
        case .square: .square
        }
    }

    /// The library's sections, each with its name.
    var sections: [Section] {
        state.tabs.map { tab in
            let label = switch tab {
            case .titles:
                switch library.kind {
                case .movies: String(localized: "Movies")
                case .shows: String(localized: "Shows")
                case .audiobooks: String(localized: "Books")
                case .videos, .unknown: String(localized: "Videos")
                }
            case .collections: String(localized: "Collections")
            case .authors: String(localized: "Authors")
            case .bookSeries: String(localized: "Series")
            }
            return Section(tab: tab, label: label)
        }
    }

    /// The sorts this library offers, Title first.
    var sorts: [Sort<GridSort>] { state.sorts.map(Self.sort) }

    /// The sort chosen.
    var sort: Sort<GridSort> { Self.sort(state.sort) }

    // MARK: Choosing

    func choose(tab: LibraryTab) {
        grid.choose(tab: tab)
        take(grid.state.value)
    }

    func choose(sort: GridSort) {
        grid.choose(sort: sort)
        take(grid.state.value)
    }

    /// Chooses [filters]: the grid shows them at once, or with [settle], as
    /// the filter sheet asks, once they've held still.
    func choose(_ filters: GridFilters, settle: Bool = false) {
        grid.choose(filters: filters, settle: settle)
        take(grid.state.value)
    }

    /// Turns [filter] on, or off when it's on.
    func toggle(_ filter: GridFilter, settle: Bool = false) {
        choose(state.filters.toggling(filter: filter), settle: settle)
    }

    /// Clears the filters, keeping the tab and sort.
    func clearFilters(settle: Bool = false) {
        grid.clearFilters(settle: settle)
        take(grid.state.value)
    }

    // MARK: Following

    /// Follows the grid while it shows, until the view's task is cancelled:
    /// its first page, then what's chosen and offered as the core has it.
    /// The pages keep up with changes for as long as the grid exists, so
    /// coming back to it reads nothing again.
    func run() async {
        running = true
        follow()
        defer {
            running = false
            following?.cancel()
        }
        if !hasRun {
            hasRun = true
            await ready()
        }
        for await _ in grid.state {
            take(grid.state.value)
        }
    }

    /// Returns once the grid has something to show: its first page, from
    /// the cache or the server, or why it couldn't be read.
    func ready() async {
        for await _ in grid.state {
            take(grid.state.value)
            if state.tabsKnown { break }
        }
        await paged.ready()
    }

    /// Shows [next], the core's state now, never one before it, whose grid
    /// has stopped reading: a new grid when it has one, from the top once
    /// its first page arrives, at once if it was seen lately.
    private func take(_ next: LibraryGridState) {
        let before = state
        guard next != before else { return }
        state = next
        if next.pager !== before.pager {
            let old = paged
            paged = Self.paged(next)
            holdFirstPage()
            old.close()
            generation += 1
            follow()
        } else if next.tabsKnown, !before.tabsKnown {
            paged.catchUp()
        }
    }

    /// Follows [paged] while the grid runs, showing each change.
    private func follow() {
        following?.cancel()
        guard running else { return }
        let paged = paged
        following = Task { await paged.run() }
    }

    /// The first page shows with whether the library has collections.
    private func holdFirstPage() {
        paged.holdsFirstPage = { [weak self] in self?.state.tabsKnown == false }
    }

    /// [state]'s grid as the screen draws it.
    private static func paged(_ state: LibraryGridState) -> PagedGrid {
        let style = style(state)
        return PagedGrid(state.pager) { $0.artwork(style) }
    }

    // MARK: Words

    private static func sort(_ sort: GridSort) -> Sort<GridSort> {
        let words: (label: String, direction: String) = switch sort {
        case .title: (String(localized: "Title"), String(localized: "A to Z"))
        case .author: (String(localized: "Author"), String(localized: "A to Z"))
        case .addedAt: (String(localized: "Date added"), String(localized: "Newest first"))
        case .lastEpisodeAddedAt: (String(localized: "Last episode added"), String(localized: "Newest first"))
        case .releaseDate: (String(localized: "Release date"), String(localized: "Newest first"))
        case .rating: (String(localized: "Rating"), String(localized: "Highest first"))
        case .runtime: (String(localized: "Runtime"), String(localized: "Shortest first"))
        case .lastPlayedAt: (String(localized: "Last played"), String(localized: "Most recent first"))
        }
        return Sort(value: sort, label: words.label, direction: words.direction)
    }

    /// A watch status as the library names it, nil being all of them.
    func label(played: WorkPlayState?) -> String {
        switch played {
        case nil: String(localized: "All")
        case .unplayed: isBooks ? String(localized: "Not started") : String(localized: "Unwatched")
        case .inProgress: String(localized: "In progress")
        case .played: isBooks ? String(localized: "Finished") : String(localized: "Watched")
        }
    }

    /// A value to filter by, as its group names it.
    func label(_ filter: GridFilter) -> String {
        switch onEnum(of: filter) {
        case .played(let filter): label(played: filter.state)
        case .decade(let filter): String(localized: "\(String(filter.year))s")
        case .genre(let filter): filter.name
        case .contentRating(let filter): filter.name
        case .resolution(let filter): Self.label(filter.resolution)
        case .dynamicRange(let filter): Self.label(filter.range)
        case .narrator(let filter): filter.name ?? ""
        }
    }

    /// A filter that's on, as its chip names it.
    func chipLabel(_ filter: GridFilter) -> String {
        guard case .narrator(let narrator) = onEnum(of: filter) else { return label(filter) }
        return String(localized: "Narrated by \(narrator.name ?? "")")
    }

    /// A group of filters as its heading names it.
    static func title(_ group: FilterGroupKind) -> String {
        switch group {
        case .watchStatus: String(localized: "Watch status")
        case .decade: String(localized: "Decade")
        case .genre: String(localized: "Genre")
        case .contentRating: String(localized: "Content rating")
        case .quality: String(localized: "Quality")
        case .narrator: String(localized: "Narrator")
        }
    }

    /// The count with the noun for what the section showing lists: "41 movies".
    func noun(_ count: Int) -> String {
        let n = count.formatted()
        let one = count == 1
        switch state.tab {
        case .collections: return one ? String(localized: "1 collection") : String(localized: "\(n) collections")
        case .authors: return one ? String(localized: "1 author") : String(localized: "\(n) authors")
        case .bookSeries: return String(localized: "\(n) series")
        case .titles:
            switch library.kind {
            case .movies: return one ? String(localized: "1 movie") : String(localized: "\(n) movies")
            case .shows: return one ? String(localized: "1 show") : String(localized: "\(n) shows")
            case .audiobooks: return one ? String(localized: "1 book") : String(localized: "\(n) books")
            case .videos, .unknown: return one ? String(localized: "1 video") : String(localized: "\(n) videos")
            }
        }
    }

    /// A resolution as its chip and menu name it.
    static func label(_ resolution: Resolution) -> String {
        switch resolution {
        case .fourK: "4K"
        case .p1080: "1080p"
        case .p720: "720p"
        case .sd: "SD"
        case .unknown: resolution.value
        }
    }

    /// An HDR format as its chip and menu name it.
    static func label(_ format: DynamicRange) -> String {
        switch format {
        case .hdr: "HDR"
        case .dolbyVision: "Dolby Vision"
        }
    }
}

extension GridSort: SortMenuKey {
    var short: String? {
        switch self {
        case .title: String(localized: "A–Z")
        case .author: String(localized: "Author A–Z")
        case .addedAt: String(localized: "Recently added")
        case .releaseDate: String(localized: "Newest")
        case .rating: String(localized: "Top rated")
        case .runtime: String(localized: "Shortest")
        case .lastPlayedAt: String(localized: "Recently played")
        case .lastEpisodeAddedAt: nil
        }
    }
}
