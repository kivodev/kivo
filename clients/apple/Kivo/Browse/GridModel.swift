import Foundation
import KivoCore
import KivoUI
import Observation

/// A library's grid: the paged grid (``PagedGrid``) of the section showing,
/// its works, collections, authors or book series, with the tab, sort and
/// filters that choose it. A new tab, sort or filter shows at once, its
/// placeholders until its first page arrives; a change the live events
/// report, a pull or coming back reads the pages again in place, keeping
/// the place. It also finds out
/// what the library's filters offer, of the values its works have: genres,
/// decades, content ratings, resolutions and dynamic ranges, or a books
/// library's genres, decades and narrators; and whether it has
/// collections. Whether it has collections is
/// kept on the device, so its chips are there from the first frame; the
/// first time, it comes with the first page. Opened sorted one way for
/// once, it starts from the works in that sort with no filters, and keeps
/// nothing on the device.
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

    /// What a library lists: its works, or its collections, authors or book series.
    enum Tab: String {
        case titles
        case collections
        case authors
        case bookSeries
    }

    /// A section of the library, one of its chips.
    struct Section: Identifiable {
        let tab: Tab
        let label: String
        var id: String { tab.rawValue }
    }

    /// A filter that's on, shown as a chip that turns it off.
    struct ActiveFilter: Identifiable {
        let id: String
        let label: String
        let remove: (inout Query) -> Void
    }

    struct Played: Hashable, Identifiable {
        let value: WorkPlayState?
        let label: String
        var id: String { value?.value ?? "" }
    }

    /// A books library's narrator, by person ID.
    struct Narrator: Hashable, Identifiable {
        let id: String
        let name: String
    }

    /// What the grid shows: the tab, and for the works the sort and filters.
    struct Query: Hashable {
        var tab = Tab.titles
        var sort: Sort<GridSort>
        var played: Played
        var genres: Set<String> = []
        /// The decade's first year, 1990 for the 1990s; nil for any.
        var decade: Int?
        var contentRatings: Set<String> = []
        var resolutions: Set<Resolution> = []
        var dynamicRanges: Set<DynamicRange> = []
        var narrator: Narrator?

        /// Whether a filter narrows the works.
        var filtered: Bool {
            played.value != nil || !genres.isEmpty || decade != nil || !contentRatings.isEmpty
                || !resolutions.isEmpty || !dynamicRanges.isEmpty || narrator != nil
        }
    }

    let library: Library
    /// The sorts this library offers, Title first.
    let sorts: [Sort<GridSort>]
    let playedFilters: [Played]
    /// The tab, sort and filters chosen.
    var query: Query
    /// Those of the grid showing: [query] as last reloaded.
    private(set) var shown: Query

    /// The grid showing, [shown]'s: its count, letters and pages.
    private(set) var paged: PagedGrid
    /// Whether the library has collections, nil until it's known on this device.
    private(set) var hasCollections: Bool?
    /// What the filters offer, in the order to show them; none of a kind the library's filters don't have.
    private(set) var genres: [String] = []
    /// Newest first.
    private(set) var decades: [Int] = []
    private(set) var contentRatings: [String] = []
    private(set) var resolutions: [Resolution] = []
    private(set) var dynamicRanges: [DynamicRange] = []
    private(set) var narrators: [Narrator] = []
    /// Counts the grids shown, a new one with each tab, sort or filter.
    private(set) var generation = 0

    /// Following [paged] while the grid runs.
    @ObservationIgnored private var following: Task<Void, Never>?
    /// Asking whether the library has collections, which the first page waits for.
    @ObservationIgnored private var askingCollections: Task<Void, Never>?
    @ObservationIgnored private var running = false
    @ObservationIgnored private var hasRun = false
    @ObservationIgnored private let session: KivoSession
    /// Whether the tab, sort and filters chosen are kept for the library on the device.
    @ObservationIgnored private let keeps: Bool

    /// Opens [library] with the tab, sort and filters this device last used
    /// in it, or with its works in [sort] alone for once, and its first page
    /// if it was seen lately.
    init(library: Library, session: KivoSession, sort: GridSort? = nil) {
        self.library = library
        self.session = session
        keeps = sort == nil
        let books = library.kind == .audiobooks
        sorts = books ? [
            Sort(value: GridSort.title, label: String(localized: "Title"), direction: String(localized: "A to Z")),
            Sort(value: GridSort.author, label: String(localized: "Author"), direction: String(localized: "A to Z")),
            Sort(value: GridSort.addedAt, label: String(localized: "Date added"), direction: String(localized: "Newest first")),
            Sort(value: GridSort.releaseDate, label: String(localized: "Release date"), direction: String(localized: "Newest first")),
        ] : [
            Sort(value: GridSort.title, label: String(localized: "Title"), direction: String(localized: "A to Z")),
            Sort(value: GridSort.addedAt, label: String(localized: "Date added"), direction: String(localized: "Newest first")),
        ] + (library.kind == .shows ? [
            Sort(value: GridSort.lastEpisodeAddedAt, label: String(localized: "Last episode added"), direction: String(localized: "Newest first")),
        ] : []) + [
            Sort(value: GridSort.releaseDate, label: String(localized: "Release date"), direction: String(localized: "Newest first")),
            Sort(value: GridSort.rating, label: String(localized: "Rating"), direction: String(localized: "Highest first")),
            Sort(value: GridSort.runtime, label: String(localized: "Runtime"), direction: String(localized: "Shortest first")),
            Sort(value: GridSort.lastPlayedAt, label: String(localized: "Last played"), direction: String(localized: "Most recent first")),
        ]
        playedFilters = [
            Played(value: nil, label: String(localized: "All")),
            Played(value: .unplayed, label: books ? String(localized: "Not started") : String(localized: "Unwatched")),
            Played(value: .inProgress, label: String(localized: "In progress")),
            Played(value: .played, label: books ? String(localized: "Finished") : String(localized: "Watched")),
        ]
        var saved: [String: String] = if let sort {
            ["sort": sort.value]
        } else {
            UserDefaults.standard.dictionary(forKey: Self.stateKey(library)) as? [String: String] ?? [:]
        }
        // Migration (api-naming): remove after every client has updated past this release.
        if sort == nil, let migrated = ApiNamingMigration.gridState(saved) {
            saved = migrated
            UserDefaults.standard.set(saved, forKey: Self.stateKey(library))
        }
        let list = { (key: String, separator: Character) in (saved[key] ?? "").split(separator: separator).map(String.init) }
        let query = Query(
            tab: saved["tab"].flatMap(Tab.init(rawValue:)) ?? .titles,
            sort: sorts.first { $0.value == GridSort.companion.of(value: saved["sort"]) } ?? sorts[0],
            played: playedFilters.first { $0.value == WorkPlayState.companion.of(value: saved["played"]) } ?? playedFilters[0],
            genres: Set(list("genres", Self.nameSeparator)),
            decade: saved["decade"].flatMap(Int.init),
            contentRatings: Set(list("contentRatings", Self.nameSeparator)),
            resolutions: Set(list("resolutions", ",").map { Resolution.companion.of(value: $0) }.filter { $0 != .unknown }),
            dynamicRanges: Set(list("dynamicRanges", ",").compactMap { DynamicRange.companion.of(value: $0) }),
            narrator: saved["narrator"].map { Narrator(id: $0, name: saved["narratorName"] ?? "") }
        )
        self.query = query
        shown = query
        hasCollections = UserDefaults.standard.object(forKey: Self.collectionsKey(library)) as? Bool
        paged = Self.grid(query, of: library, session: session)
        holdFirstPage()
        askCollections()
    }

    deinit {
        following?.cancel()
    }

    private static func stateKey(_ library: Library) -> String { "grid.\(library.id)" }

    /// Between the genres or content ratings kept, which are names and may hold a comma.
    private static let nameSeparator: Character = "\n"

    private static func collectionsKey(_ library: Library) -> String { "grid.\(library.id).collections" }

    /// Keeps whether the library has collections, on the device too.
    private func know(collections has: Bool) {
        if hasCollections != has { hasCollections = has }
        UserDefaults.standard.set(has, forKey: Self.collectionsKey(library))
    }

    var isBooks: Bool { library.kind == .audiobooks }

    /// The shape of the cards on the section showing.
    var style: CardStyle { Self.style(of: shown.tab, in: library) }

    /// The shape of the cards on [tab] of [library].
    private static func style(of tab: Tab, in library: Library) -> CardStyle {
        switch tab {
        case .collections: .poster
        case .authors, .bookSeries: .square
        case .titles:
            switch library.kind {
            case .audiobooks: .square
            case .videos: .landscape
            case .movies, .shows, .unknown: .poster
            }
        }
    }

    /// The library's sections: Movies or Shows with Collections
    /// when it has any; an audiobooks library's Books, Authors and Series.
    var sections: [Section] {
        if isBooks {
            return [
                Section(tab: .titles, label: String(localized: "Books")),
                Section(tab: .authors, label: String(localized: "Authors")),
                Section(tab: .bookSeries, label: String(localized: "Series")),
            ]
        }
        let titles = switch library.kind {
        case .movies: String(localized: "Movies")
        case .shows: String(localized: "Shows")
        case .videos, .audiobooks, .unknown: String(localized: "Videos")
        }
        var sections = [Section(tab: .titles, label: titles)]
        if hasCollections == true || query.tab == .collections {
            sections.append(Section(tab: .collections, label: String(localized: "Collections")))
        }
        return sections
    }

    /// Whether [sections] are the library's own yet: a video library's wait
    /// on whether it has collections.
    var sectionsKnown: Bool {
        isBooks || hasCollections != nil || query.tab == .collections
    }

    /// The filters that are on, each with what turns it off.
    var activeFilters: [ActiveFilter] {
        guard query.tab == .titles else { return [] }
        var filters: [ActiveFilter] = []
        if query.played.value != nil {
            filters.append(ActiveFilter(id: "played", label: query.played.label) { [playedFilters] in $0.played = playedFilters[0] })
        }
        for genre in Self.ordered(query.genres, as: genres, by: \.self) {
            filters.append(ActiveFilter(id: "genre:\(genre)", label: genre) { $0.genres.remove(genre) })
        }
        for rating in Self.ordered(query.contentRatings, as: contentRatings, by: \.self) {
            filters.append(ActiveFilter(id: "contentRating:\(rating)", label: rating) { $0.contentRatings.remove(rating) })
        }
        if let decade = query.decade {
            filters.append(ActiveFilter(id: "decade", label: Self.label(decade: decade)) { $0.decade = nil })
        }
        for resolution in Self.ordered(query.resolutions, as: resolutions, by: \.value) {
            filters.append(ActiveFilter(id: resolution.value, label: Self.label(resolution)) { $0.resolutions.remove(resolution) })
        }
        for format in Self.ordered(query.dynamicRanges, as: dynamicRanges, by: \.value) {
            filters.append(ActiveFilter(id: format.value, label: Self.label(format)) { $0.dynamicRanges.remove(format) })
        }
        if let narrator = query.narrator {
            let name = narrators.first { $0.id == narrator.id }?.name ?? narrator.name
            filters.append(ActiveFilter(id: "narrator", label: String(localized: "Narrated by \(name)")) { $0.narrator = nil })
        }
        return filters
    }

    /// Follows the grid while it shows, until the view's task is cancelled:
    /// its first page, then what the library's filters offer and whether it
    /// has collections. The pages keep up with changes for as long as the
    /// grid exists, so coming back to it reads nothing again.
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
        for await load in session.library.filters(libraryId: library.id) {
            if let filters = load.getOrNull() { offer(filters) }
        }
    }

    /// Returns once the grid has something to show: its first page, from
    /// the cache or the server, or why it couldn't be read.
    func ready() async {
        await askingCollections?.value
        await paged.ready()
    }

    /// The tab, sort or filters changed: the device keeps them for the
    /// library, unless it was opened sorted for once, and the grid starts
    /// again from the top once the first page arrives, at once if it was
    /// seen lately.
    func reload() {
        if keeps {
            var state = [
                "sort": query.sort.value.value,
                "genres": query.genres.sorted().joined(separator: String(Self.nameSeparator)),
                "contentRatings": query.contentRatings.sorted().joined(separator: String(Self.nameSeparator)),
                "resolutions": query.resolutions.map(\.value).sorted().joined(separator: ","),
                "dynamicRanges": query.dynamicRanges.map(\.value).sorted().joined(separator: ","),
            ]
            state["tab"] = query.tab.rawValue
            state["played"] = query.played.value?.value
            state["decade"] = query.decade.map(String.init)
            state["narrator"] = query.narrator?.id
            state["narratorName"] = query.narrator?.name
            UserDefaults.standard.set(state, forKey: Self.stateKey(library))
        }
        let old = paged
        paged = Self.grid(query, of: library, session: session)
        holdFirstPage()
        old.close()
        shown = query
        generation += 1
        askCollections()
        follow()
    }

    /// Clears the filters, keeping the tab and sort.
    func clearFilters() {
        query = withoutFilters(query)
    }

    /// [query] with no filters, its tab and sort as they are.
    func withoutFilters(_ query: Query) -> Query {
        Query(tab: query.tab, sort: query.sort, played: playedFilters[0])
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
        paged.holdsFirstPage = { [weak self] in self?.askingCollections != nil }
    }

    /// Asks, when it isn't known yet, whether the library has collections,
    /// so the first page shows with the chips it brings.
    private func askCollections() {
        guard !isBooks, hasCollections == nil, askingCollections == nil else { return }
        askingCollections = Task { [weak self] in
            guard let self else { return }
            let has = await hasCollections()
            know(collections: has)
            askingCollections = nil
            paged.catchUp()
        }
    }

    /// Offers the filters that find something in the library, and knows whether it has collections.
    /// A books library offers genres, decades and narrators; the others
    /// genres, decades, content ratings, resolutions and dynamic ranges.
    private func offer(_ filters: LibraryFilters) {
        if !isBooks { know(collections: filters.collections) }
        let decades = Self.decades(of: filters.years.map { Int(truncating: $0) })
        let narrators = isBooks ? filters.narrators.map { Narrator(id: $0.id, name: $0.name) } : []
        if genres != filters.genres { genres = filters.genres }
        if self.decades != decades { self.decades = decades }
        if contentRatings != (isBooks ? [] : filters.contentRatings) { contentRatings = isBooks ? [] : filters.contentRatings }
        if resolutions != (isBooks ? [] : filters.resolutions) { resolutions = isBooks ? [] : filters.resolutions }
        if dynamicRanges != (isBooks ? [] : filters.dynamicRanges) { dynamicRanges = isBooks ? [] : filters.dynamicRanges }
        if self.narrators != narrators { self.narrators = narrators }
    }

    /// The decades [years] fall in, by their first year, newest first.
    static func decades(of years: [Int]) -> [Int] {
        Set(years.map { $0 - $0 % 10 }).sorted(by: >)
    }

    /// [values] in [offered]'s order, then any it doesn't have by [name].
    private static func ordered<Value: Hashable>(_ values: Set<Value>, as offered: [Value], by name: (Value) -> String) -> [Value] {
        offered.filter(values.contains) + values.subtracting(offered).sorted { name($0) < name($1) }
    }

    /// Whether the library has collections, by its filters' first answer; a failed read counts as none.
    private func hasCollections() async -> Bool {
        await session.library.filters(libraryId: library.id).answer()?.collections ?? false
    }

    /// The grid of [query]'s tab: the works with their sort and filters, or a tab whole.
    private static func grid(_ query: Query, of library: Library, session: KivoSession) -> PagedGrid {
        let settle = PagedGrid.settleMs
        let grid = switch query.tab {
        case .titles: session.library.pagedWorks(query: gridQuery(query, of: library), settleMs: settle)
        case .collections: session.library.pagedCollections(libraryId: library.id, settleMs: settle)
        case .authors: session.library.pagedAuthors(libraryId: library.id, settleMs: settle)
        case .bookSeries: session.library.pagedBookSeries(libraryId: library.id, settleMs: settle)
        }
        let style = style(of: query.tab, in: library)
        return PagedGrid(grid) { $0.artwork(style) }
    }

    /// The query for the grid chosen, for Save as Smart List.
    var smartListQuery: WorkQuery {
        SmartLists.shared.fromGrid(grid: Self.gridQuery(query, of: library), libraryKind: library.kind)
    }

    /// The core's query for [query]'s works in [library]: a decade is its ten
    /// years, 1990 to 1999; each other filter matches any of its values, and
    /// one left empty matches all.
    static func gridQuery(_ query: Query, of library: Library) -> GridQuery {
        let list = { (values: Set<String>) in values.isEmpty ? nil : values.sorted() }
        let resolutions = query.resolutions.sorted { $0.value < $1.value }
        let dynamicRanges = query.dynamicRanges.sorted { $0.value < $1.value }
        return GridQuery(
            libraryId: library.id,
            sort: query.sort.value,
            played: query.played.value,
            genres: list(query.genres),
            yearFrom: query.decade.map { KotlinInt(value: Int32($0)) },
            yearTo: query.decade.map { KotlinInt(value: Int32($0 + 9)) },
            contentRatings: list(query.contentRatings),
            resolutions: resolutions.isEmpty ? nil : resolutions,
            dynamicRanges: dynamicRanges.isEmpty ? nil : dynamicRanges,
            narratorId: query.narrator?.id
        )
    }

    /// The count with the noun for what the section showing lists: "41 movies".
    func noun(_ count: Int) -> String {
        let n = count.formatted()
        let one = count == 1
        switch shown.tab {
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

    /// A decade as its chip and menu name it: "1990s".
    static func label(decade: Int) -> String {
        String(localized: "\(String(decade))s")
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
