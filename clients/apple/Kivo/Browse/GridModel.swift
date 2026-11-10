import Foundation
import KivoCore
import KivoUI
import Observation

/// A library's grid, over the core's paged grid (`GridPager`) of the
/// section showing: its works, collections, authors or book series. The
/// first page brings the total and, in letter order, the letter scrubber's
/// starts, and the pages where the grid comes to rest load as it does. Each
/// page is observed on its own, so a page arriving updates only its own
/// cards. A new tab, sort or filter shows at once, its placeholders until
/// its first page arrives; a change the live events report, a pull or
/// coming back reads the pages again in place, keeping the place. A first
/// page seen lately shows at once while it's read again. It also finds out
/// which of the quick filters the library has anything for: its
/// collections, 4K, HDR and Dolby Vision. Whether it has collections is
/// kept on the device, so its chips are there from the first frame; the
/// first time, it comes with the first page. Opened sorted one way for
/// once, it starts from the works in that sort with no filters, and keeps
/// nothing on the device.
@MainActor
@Observable
final class GridModel {
    /// A page of the grid, its items once they've arrived.
    @MainActor
    @Observable
    final class Page {
        fileprivate(set) var items: [LibraryItem]?
        @ObservationIgnored fileprivate let offset: Int
        /// The items on their way to [items], their blurhashes being decoded.
        @ObservationIgnored fileprivate var coming: [LibraryItem]?

        fileprivate init(offset: Int) {
            self.offset = offset
        }

        /// The item at [index] of the grid, nil until the page arrives.
        func item(at index: Int) -> LibraryItem? {
            guard let items, items.indices.contains(index - offset) else { return nil }
            return items[index - offset]
        }
    }

    struct Sort: Hashable, Identifiable {
        let value: String
        let label: String
        /// The direction it sorts in: "A to Z", "Newest first".
        let direction: String
        var id: String { value }

        /// The sort in a word or two, for the count line: "A–Z", "Recently added".
        var short: String {
            switch value {
            case "title": String(localized: "A–Z")
            case "author": String(localized: "Author A–Z")
            case "addedAt", "dateAdded": String(localized: "Recently added")
            case "releaseDate": String(localized: "Newest")
            case "rating": String(localized: "Top rated")
            case "runtimeMs": String(localized: "Shortest")
            case "lastPlayedAt": String(localized: "Recently played")
            default: label
            }
        }
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
        let value: String?
        let label: String
        var id: String { value ?? "" }
    }

    /// What the grid shows: the tab, and for the works the sort and filters.
    struct Query: Hashable {
        var tab = Tab.titles
        var sort: Sort
        var played: Played
        var resolutions: Set<String> = []
        var dynamicRanges: Set<String> = []

        /// Whether a filter narrows the works.
        var filtered: Bool {
            played.value != nil || !resolutions.isEmpty || !dynamicRanges.isEmpty
        }
    }

    /// How long the grid holds still before the pages where it rests load,
    /// so a fling or a scrub reads none of the pages it passes.
    private static let settleTime = Duration.milliseconds(150)

    let library: Library
    /// The sorts this library offers, Title first.
    let sorts: [Sort]
    let playedFilters: [Played]
    /// The tab, sort and filters chosen.
    var query: Query
    /// Those of the grid showing: [query] as last reloaded.
    private(set) var shown: Query

    private(set) var total: Int?
    private(set) var letters: [LetterStart] = []
    /// Why the first page couldn't be read.
    private(set) var failed: RequestProblem?
    /// Whether the library has collections, nil until it's known on this device.
    private(set) var hasCollections: Bool?
    private(set) var resolutions: [String] = []
    private(set) var dynamicRanges: [String] = []
    /// Counts the grids shown, a new one with each tab, sort or filter.
    private(set) var generation = 0

    /// The pages by number, made as cards ask for them; the cards observe
    /// each page, not this.
    @ObservationIgnored private var pages: [Int: Page] = [:]
    /// The grid showing, [shown]'s.
    @ObservationIgnored private var paged: Pager
    @ObservationIgnored private let pageSize: Int
    /// Following [paged] while the grid runs.
    @ObservationIgnored private var observing: Task<Void, Never>?
    /// The pages to load once the grid has held still.
    @ObservationIgnored private var settling: Task<Void, Never>?
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
    init(library: Library, session: KivoSession, sort: String? = nil) {
        self.library = library
        self.session = session
        keeps = sort == nil
        let books = library.kind == "audiobooks"
        sorts = books ? [
            Sort(value: "title", label: String(localized: "Title"), direction: String(localized: "A to Z")),
            Sort(value: "author", label: String(localized: "Author"), direction: String(localized: "A to Z")),
            Sort(value: "addedAt", label: String(localized: "Date added"), direction: String(localized: "Newest first")),
            Sort(value: "releaseDate", label: String(localized: "Release date"), direction: String(localized: "Newest first")),
        ] : [
            Sort(value: "title", label: String(localized: "Title"), direction: String(localized: "A to Z")),
            Sort(value: "addedAt", label: String(localized: "Date added"), direction: String(localized: "Newest first")),
        ] + (library.kind == "shows" ? [
            Sort(value: "lastEpisodeAddedAt", label: String(localized: "Last episode added"), direction: String(localized: "Newest first")),
        ] : []) + [
            Sort(value: "releaseDate", label: String(localized: "Release date"), direction: String(localized: "Newest first")),
            Sort(value: "rating", label: String(localized: "Rating"), direction: String(localized: "Highest first")),
            Sort(value: "runtimeMs", label: String(localized: "Runtime"), direction: String(localized: "Shortest first")),
            Sort(value: "lastPlayedAt", label: String(localized: "Last played"), direction: String(localized: "Most recent first")),
        ]
        playedFilters = [
            Played(value: nil, label: String(localized: "All")),
            Played(value: "unplayed", label: books ? String(localized: "Not started") : String(localized: "Unwatched")),
            Played(value: "inProgress", label: String(localized: "In progress")),
            Played(value: "played", label: books ? String(localized: "Finished") : String(localized: "Watched")),
        ]
        var saved: [String: String] = if let sort {
            ["sort": sort]
        } else {
            UserDefaults.standard.dictionary(forKey: Self.stateKey(library)) as? [String: String] ?? [:]
        }
        // Migration (api-naming): remove after every client has updated past this release.
        if sort == nil, let migrated = ApiNamingMigration.gridState(saved) {
            saved = migrated
            UserDefaults.standard.set(saved, forKey: Self.stateKey(library))
        }
        let list = { (key: String) in Set((saved[key] ?? "").split(separator: ",").map(String.init)) }
        let query = Query(
            tab: saved["tab"].flatMap(Tab.init(rawValue:)) ?? .titles,
            sort: sorts.first { $0.value == saved["sort"] } ?? sorts[0],
            played: playedFilters.first { $0.value == saved["played"] } ?? playedFilters[0],
            resolutions: list("resolutions"),
            dynamicRanges: list("dynamicRanges")
        )
        self.query = query
        shown = query
        hasCollections = UserDefaults.standard.object(forKey: Self.collectionsKey(library)) as? Bool
        paged = Self.pager(query, of: library, session: session)
        pageSize = paged.pageSize
        askCollections()
        apply(paged.current(), of: paged, atOnce: true)
    }

    deinit {
        observing?.cancel()
        settling?.cancel()
        paged.close()
    }

    private static func stateKey(_ library: Library) -> String { "grid.\(library.id)" }

    private static func collectionsKey(_ library: Library) -> String { "grid.\(library.id).collections" }

    /// Keeps whether the library has collections, on the device too.
    private func know(collections has: Bool) {
        if hasCollections != has { hasCollections = has }
        UserDefaults.standard.set(has, forKey: Self.collectionsKey(library))
    }

    var isBooks: Bool { library.kind == "audiobooks" }

    /// The shape of the cards on the section showing.
    var style: CardStyle { style(of: shown.tab) }

    /// The shape of the cards on [tab].
    private func style(of tab: Tab) -> CardStyle {
        switch tab {
        case .collections: .poster
        case .authors, .bookSeries: .square
        case .titles:
            switch library.kind {
            case "audiobooks": .square
            case "videos": .landscape
            default: .poster
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
        case "movies": String(localized: "Movies")
        case "shows": String(localized: "Shows")
        default: String(localized: "Videos")
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
        for resolution in query.resolutions.sorted() {
            filters.append(ActiveFilter(id: resolution, label: Self.label(resolution)) { $0.resolutions.remove(resolution) })
        }
        for format in query.dynamicRanges.sorted() {
            filters.append(ActiveFilter(id: format, label: Self.label(format)) { $0.dynamicRanges.remove(format) })
        }
        return filters
    }

    /// Follows the grid while it shows, until the view's task is cancelled:
    /// its first page, then what the library's filters offer and whether it
    /// has collections. The pages keep up with changes for as long as the
    /// grid exists, so coming back to it reads nothing again.
    func run() async {
        running = true
        observe()
        defer {
            running = false
            observing?.cancel()
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
        let paged = paged
        for await state in paged.states() {
            apply(state, of: paged)
            if total != nil || failed != nil || paged !== self.paged { return }
        }
    }

    /// Reads the pages on screen again, keeping the place, as for a pull to refresh.
    func refresh() async {
        await paged.refresh()
    }

    /// The page holding the card at [index].
    func page(containing index: Int) -> Page {
        let number = index / pageSize
        if let page = pages[number] { return page }
        let page = Page(offset: number * pageSize)
        page.items = paged.current().items(number)
        pages[number] = page
        return page
    }

    /// The item at [index], nil until its page arrives.
    func item(at index: Int) -> LibraryItem? {
        paged.current().items(index / pageSize)?[safe: index % pageSize]
    }

    /// The cards at [visible] are on screen: once the grid holds still,
    /// their pages load, and the next page either way once they're past the
    /// middle of theirs.
    func show(_ visible: ClosedRange<Int>) {
        let size = pageSize
        var low = visible.lowerBound / size
        var high = visible.upperBound / size
        if visible.upperBound % size >= size / 2 { high += 1 }
        if visible.lowerBound % size < size / 2 { low = max(low - 1, 0) }
        settling?.cancel()
        let paged = paged
        settling = Task { [weak self] in
            try? await Task.sleep(for: Self.settleTime)
            guard !Task.isCancelled, let self, paged === self.paged else { return }
            paged.load(Int32(low * size), Int32((high + 1) * size - 1))
        }
    }

    /// The letter a card at [index] files under.
    func letter(at index: Int) -> String? {
        letters.last { Int($0.offset) <= index }?.letter
    }

    /// The tab, sort or filters changed: the device keeps them for the
    /// library, unless it was opened sorted for once, and the grid starts
    /// again from the top once the first page arrives, at once if it was
    /// seen lately.
    func reload() {
        if keeps {
            var state = [
                "sort": query.sort.value,
                "resolutions": query.resolutions.sorted().joined(separator: ","),
                "dynamicRanges": query.dynamicRanges.sorted().joined(separator: ","),
            ]
            state["tab"] = query.tab.rawValue
            state["played"] = query.played.value
            UserDefaults.standard.set(state, forKey: Self.stateKey(library))
        }
        let old = paged
        paged = Self.pager(query, of: library, session: session)
        old.close()
        settling?.cancel()
        shown = query
        generation += 1
        pages = [:]
        total = nil
        letters = []
        failed = nil
        askCollections()
        apply(paged.current(), of: paged, atOnce: true)
        observe()
    }

    /// Clears the filters, keeping the tab and sort.
    func clearFilters() {
        query.played = playedFilters[0]
        query.resolutions = []
        query.dynamicRanges = []
    }

    /// Follows [paged] while the grid runs, showing each change.
    private func observe() {
        observing?.cancel()
        guard running else { return }
        let paged = paged
        observing = Task { [weak self] in
            for await state in paged.states() {
                self?.apply(state, of: paged)
            }
        }
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
            apply(paged.current(), of: paged)
        }
    }

    /// Shows [state] of [from], the grid showing; set only where changed, so
    /// a read that finds nothing new redraws nothing. A page's cards show
    /// once their blurhashes are decoded, or [atOnce], for those seen before.
    private func apply(_ state: Pager.State, of from: Pager, atOnce: Bool = false) {
        guard from === paged else { return }
        // The first page shows with whether the library has collections.
        if total == nil, askingCollections != nil, !atOnce, state.problem == nil { return }
        if total != state.total { total = state.total }
        if letters != state.letters { letters = state.letters }
        if failed != state.problem { failed = state.problem }
        let style = style(of: shown.tab)
        for (number, page) in pages {
            let items = state.items(number)
            guard items != page.items, items != page.coming else { continue }
            guard let items, !atOnce else {
                page.coming = nil
                page.items = items
                continue
            }
            page.coming = items
            Task {
                await BlurHash.prepare(items.compactMap { $0.artwork(style)?.blurhash })
                guard page.coming == items else { return }
                page.coming = nil
                page.items = items
            }
        }
    }

    /// Offers the filters that find something in the library, and knows whether it has collections.
    private func offer(_ filters: LibraryFilters) {
        if !isBooks { know(collections: filters.collections) }
        if resolutions != filters.resolutions { resolutions = filters.resolutions }
        if dynamicRanges != filters.dynamicRanges { dynamicRanges = filters.dynamicRanges }
    }

    /// Whether the library has collections, by its filters' first answer; a failed read counts as none.
    private func hasCollections() async -> Bool {
        for await load in session.library.filters(libraryId: library.id) {
            if let filters = load.getOrNull() { return filters.collections }
            if load.problemOrNull() != nil { return false }
        }
        return false
    }

    /// The grid of [query]'s tab: the works with their sort and filters, or a tab whole.
    private static func pager(_ query: Query, of library: Library, session: KivoSession) -> Pager {
        switch query.tab {
        case .titles:
            Pager(session.library.pagedWorks(query: GridQuery(
                libraryId: library.id,
                sort: query.sort.value,
                played: query.played.value,
                resolutions: Array(query.resolutions),
                dynamicRanges: Array(query.dynamicRanges)
            )), item: LibraryItem.work)
        case .collections: Pager(session.library.pagedCollections(libraryId: library.id), item: LibraryItem.collection)
        case .authors: Pager(session.library.pagedAuthors(libraryId: library.id), item: LibraryItem.author)
        case .bookSeries: Pager(session.library.pagedBookSeries(libraryId: library.id), item: LibraryItem.bookSeries)
        }
    }

    /// The query for the grid showing, for Save as Smart List.
    var smartListQuery: WorkQuery {
        SmartLists.shared.fromGrid(
            grid: GridQuery(
                libraryId: library.id,
                sort: shown.sort.value,
                played: shown.played.value,
                resolutions: Array(shown.resolutions),
                dynamicRanges: Array(shown.dynamicRanges)
            ),
            libraryKind: library.kind
        )
    }

    /// A resolution or HDR format as its chip and menu name it.
    static func label(_ value: String) -> String {
        switch value {
        case "4k": "4K"
        case "1080p": "1080p"
        case "720p": "720p"
        case "sd": "SD"
        case "hdr": "HDR"
        case "dolbyVision": "Dolby Vision"
        default: value
        }
    }
}

private extension GridQuery {
    /// A grid query from the spec's values (`title`, `addedAt`; `unplayed`,
    /// `inProgress`), nil for the defaults; [resolutions] (`4k`, `1080p`,
    /// `720p`, `sd`) and [dynamicRanges] (`hdr`, `dolbyVision`) match any
    /// of theirs, empty for all.
    convenience init(
        libraryId: String,
        sort: String? = nil,
        played: String? = nil,
        resolutions: [String] = [],
        dynamicRanges: [String] = []
    ) {
        self.init(
            libraryId: libraryId,
            sort: sort,
            played: played,
            genres: nil,
            yearFrom: nil,
            yearTo: nil,
            contentRatings: nil,
            resolutions: resolutions.isEmpty ? nil : resolutions,
            dynamicRanges: dynamicRanges.isEmpty ? nil : dynamicRanges
        )
    }
}


/// A card of a library's grid: a work, or a collection, author or book series.
enum LibraryItem: Equatable {
    case work(WorkSummary)
    case collection(CollectionSummary)
    case author(PersonSummary)
    case bookSeries(BookSeriesSummary)

    /// The image its card shows, in a card of [style].
    func artwork(_ style: CardStyle) -> ImageRef? {
        switch self {
        case .work(let work): style.artwork(of: work)
        case .collection: nil
        case .author(let author): author.photo
        case .bookSeries(let series): series.cover
        }
    }
}

/// One of the core's paged grids, whichever item type it lists, its items
/// as [LibraryItem]s.
private final class Pager {
    /// A grid as read so far: its total once known, the letter scrubber's
    /// starts, why its first page failed, and each page's items once read.
    struct State {
        let total: Int?
        let letters: [LetterStart]
        let problem: RequestProblem?
        let items: (Int) -> [LibraryItem]?
    }

    let pageSize: Int
    let current: () -> State
    let states: () -> AsyncStream<State>
    let load: (Int32, Int32) -> Void
    let refresh: () async -> Void
    let close: () -> Void

    init<T: AnyObject>(_ grid: GridPager<T>, item: @escaping (T) -> LibraryItem) {
        func state(_ pages: GridPages<T>) -> State {
            State(
                total: pages.total.map { Int(truncating: $0) },
                letters: pages.letters,
                problem: pages.problem,
                items: { pages.items(page: Int32($0))?.map(item) }
            )
        }
        pageSize = Int(grid.state.value.pageSize)
        current = { state(grid.state.value) }
        states = {
            AsyncStream { continuation in
                let task = Task {
                    for await pages in grid.state { continuation.yield(state(pages)) }
                    continuation.finish()
                }
                continuation.onTermination = { _ in task.cancel() }
            }
        }
        load = { grid.load(first: $0, last: $1) }
        // SKIE gives a generic class's suspend functions through skie().
        refresh = { try? await skie(grid).refresh() }
        close = { grid.close() }
    }
}

private extension Array {
    subscript(safe index: Int) -> Element? {
        indices.contains(index) ? self[index] : nil
    }
}
