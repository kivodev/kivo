import Foundation
import KivoCore
import XCTest
@testable import Kivo

/// A library's grid against a server on localhost with 3,000 works: its
/// first page brings the count and letters, the cards on screen load their
/// pages, a refresh reads them again in place, a library seen lately shows
/// at once, another sort shows its own order, a sort for once leaves the
/// library's kept choices alone, and a first page that can't be read says
/// why.
@MainActor
final class GridModelTests: XCTestCase {
    private static let total = 3000
    private var server: StubServer!
    private var session: KivoSession!
    private var address = ""
    private var library: Library!
    /// What the works are called, by index; a test renames some.
    nonisolated(unsafe) private static var prefix = "Title"
    nonisolated(unsafe) private static var failing = false
    /// The values the library's works have, by field.
    nonisolated private static let values: [String: [String]] = [
        "genre": ["Action, Adventure", "Comedy", "Drama"],
        "year": ["1899", "1985", "1992", "1999", "2003", "2021"],
        "contentRating": ["G", "PG-13", "R"],
        "resolution": ["4k", "1080p"],
        "dynamicRange": ["hdr"],
        "narrator": ["p1"],
    ]

    private static func letter(_ i: Int) -> String {
        String(UnicodeScalar(UInt8(65 + i * 26 / total)))
    }

    private static func name(_ i: Int) -> String {
        "\(letter(i)) \(prefix) \(String(format: "%04d", i))"
    }

    private static func json(_ value: Any) -> String {
        String(decoding: try! JSONSerialization.data(withJSONObject: value), as: UTF8.self)
    }

    private static let worksPath = "/api/v1/works/query"
    private static let listPath = "/api/v1/lists/l1/works"

    /// What a read of the grid asked for: its `WorkQueryRequest`.
    private static func asked(_ request: StubServer.Request) -> [String: Any] {
        (try? JSONSerialization.jsonObject(with: request.body)) as? [String: Any] ?? [:]
    }

    private static func answer(_ request: StubServer.Request) -> (Int, String) {
        switch request.path {
        case "/api/v1/server":
            return (200, json(["id": "grid-test", "name": "Grid test", "version": "1.0"]))
        case "/api/v1/libraries":
            return (200, json(["items": [["id": "lib", "name": "Movies", "pinned": true, "kind": "movies", "workCount": total]]]))
        case "/api/v1/libraries/lib/collections":
            return (200, json(["items": [], "total": 0]))
        case let path where path.hasPrefix("/api/v1/work-fields/"):
            let field = path.dropFirst("/api/v1/work-fields/".count).split(separator: "/").first.map(String.init) ?? ""
            return (200, json(["items": (values[field] ?? []).map { ["value": $0, "name": $0] }]))
        case listPath:
            let start = request.query["offset"].flatMap(Int.init) ?? 0
            let end = min(start + (request.query["limit"].flatMap(Int.init) ?? 60), total)
            let items = (start..<end).map { i -> [String: Any] in
                ["id": "w\(i)", "kind": "movie", "title": name(i), "matchState": "matched", "available": true, "images": [:]]
            }
            var page: [String: Any] = ["items": items]
            if start == 0 { page["total"] = total }
            return (200, json(page))
        case worksPath:
            let read = asked(request)
            if failing {
                return (404, json(["type": "about:blank", "title": "Not found", "status": 404, "code": "notFound", "requestId": "r"]))
            }
            let limit = read["limit"] as? Int ?? 60
            let start = read["offset"] as? Int ?? 0
            let end = min(start + limit, total)
            let sort = (read["query"] as? [String: Any])?["sort"] as? [[String: Any]]
            let byDate = sort?.first?["field"] as? String == "addedAt"
            let items = (start..<end).map { i -> [String: Any] in
                let n = byDate ? total - 1 - i : i
                return ["id": "w\(n)", "kind": "movie", "title": name(n), "matchState": "matched", "available": true, "images": [:]]
            }
            var page: [String: Any] = ["items": items]
            if start == 0 { page["total"] = total }
            if start == 0, !byDate {
                var letters: [[String: Any]] = []
                for i in 0..<total where i == 0 || letter(i) != letter(i - 1) {
                    letters.append(["letter": letter(i), "offset": i])
                }
                page["letters"] = letters
            }
            return (200, json(page))
        default:
            return (404, json(["type": "about:blank", "title": "Not found", "status": 404, "code": "notFound", "requestId": "r"]))
        }
    }

    override func setUp() async throws {
        Self.prefix = "Title"
        Self.failing = false
        forgetEarlierGrid()
        server = try StubServer(answer: Self.answer)
        address = try await server.start()
        session = try await open()
        for await load in session.library.libraries() {
            if let found = (load.getOrNull() as? [Library])?.first { library = found; break }
        }
    }

    override func tearDown() async throws {
        session.close()
        server.stop()
        forgetEarlierGrid()
    }

    /// A session of the app on a device of its own, as the app opens one.
    private func open() async throws -> KivoSession {
        let directory = FileManager.default.temporaryDirectory.appending(path: UUID().uuidString)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let core = KivoClient(device: AppleDevice(
            directory: directory.path(percentEncoded: false), name: "Test", identity: AppleIdentity.current,
            clientId: nil, subtitles: [], trackSelection: false, transport: NoTransport()
        ))
        return try await core.restore(server: SavedServer(id: "grid-test", name: "Grid test", addresses: [address]))
    }

    // Migration (grid-choice): remove after every client has updated past this release.
    private func forgetEarlierGrid() {
        UserDefaults.standard.removeObject(forKey: "grid.lib")
        UserDefaults.standard.removeObject(forKey: "grid.lib.collections")
    }

    private func model() -> GridModel {
        GridModel(library: library, session: session)
    }

    /// Waits up to [seconds] for [condition].
    private func until(_ what: String, seconds: Double = 5, _ condition: () -> Bool) async throws {
        let end = Date.now.addingTimeInterval(seconds)
        while !condition() {
            if Date.now > end { XCTFail("timed out: \(what)"); return }
            try await Task.sleep(for: .milliseconds(20))
        }
    }

    func testTheFirstPageBringsTheCountLettersAndFirstCards() async throws {
        let grid = model()
        let run = Task { await grid.run() }
        defer { run.cancel() }
        await grid.ready()
        XCTAssertEqual(grid.paged.total, Self.total)
        XCTAssertEqual(grid.paged.letters.count, 26)
        XCTAssertNil(grid.paged.failed)
        XCTAssertEqual(grid.paged.item(at: 0)?.title, Self.name(0))
        XCTAssertEqual(grid.paged.item(at: 59)?.title, Self.name(59))
        XCTAssertNil(grid.paged.item(at: 60), "a page isn't read before it's shown")
        XCTAssertEqual(grid.paged.letter(at: 0), "A")
        XCTAssertEqual(grid.paged.letter(at: 1500), "N")
    }

    func testTheCardsOnScreenLoadTheirPagesAndFillTheirPagesCards() async throws {
        let grid = model()
        let run = Task { await grid.run() }
        defer { run.cancel() }
        await grid.ready()
        let page = grid.paged.page(containing: 1210)
        XCTAssertNil(page.item(at: 1210))
        grid.paged.show(1200...1230)
        try await until("page 20 loads") { page.item(at: 1210) != nil }
        XCTAssertEqual(page.item(at: 1210)?.title, Self.name(1210))
        XCTAssertEqual(grid.paged.item(at: 1200)?.title, Self.name(1200))
        XCTAssertNil(grid.paged.item(at: 2400), "a page far off isn't read")
    }

    func testARefreshReadsTheLoadedPagesAgainInPlace() async throws {
        let grid = model()
        let run = Task { await grid.run() }
        defer { run.cancel() }
        await grid.ready()
        grid.paged.show(1200...1230)
        try await until("page 20 loads") { grid.paged.item(at: 1210) != nil }
        let generation = grid.generation
        Self.prefix = "Renamed"
        await grid.paged.refresh()
        try await until("page 20 reads again") { grid.paged.item(at: 1210)?.title == Self.name(1210) }
        XCTAssertEqual(grid.paged.item(at: 0)?.title, Self.name(0))
        XCTAssertEqual(grid.generation, generation, "the same grid, in place")
        XCTAssertEqual(grid.paged.total, Self.total)
    }

    func testALibrarySeenLatelyShowsAtOnce() async throws {
        let first = model()
        let run = Task { await first.run() }
        await first.ready()
        run.cancel()
        let again = model()
        XCTAssertEqual(again.paged.total, Self.total, "the count before any read")
        XCTAssertEqual(again.paged.item(at: 0)?.title, Self.name(0), "the first cards before any read")
    }

    func testAnotherSortShowsItsOwnOrderFromTheTop() async throws {
        let grid = model()
        let run = Task { await grid.run() }
        defer { run.cancel() }
        await grid.ready()
        let generation = grid.generation
        grid.choose(sort: .addedAt)
        try await until("the new order shows") { grid.paged.item(at: 0)?.title == Self.name(Self.total - 1) }
        XCTAssertGreaterThan(grid.generation, generation, "a new grid")
        XCTAssertEqual(grid.state.sort, .addedAt)
        XCTAssertEqual(grid.sort.label, "Date added")
        XCTAssertTrue(grid.paged.letters.isEmpty, "no letters by date")
    }

    func testAnotherSortShowsItsPlaceholdersAtOnce() async throws {
        let grid = model()
        let run = Task { await grid.run() }
        defer { run.cancel() }
        await grid.ready()
        grid.choose(sort: .addedAt)
        XCTAssertEqual(grid.state.sort, .addedAt, "the new sort shows at once")
        XCTAssertNil(grid.paged.total, "its placeholders until its first page arrives, not the grid before")
        XCTAssertNil(grid.paged.item(at: 0))
        try await until("its first page arrives") { grid.paged.total == Self.total }
    }

    func testASortForOnceOpensWithoutFiltersAndKeepsNothing() async throws {
        let usual = model()
        usual.choose(sort: .rating)
        usual.toggle(GridFilter.Played(state: .unplayed))
        let grid = GridModel(library: library, session: session, sort: .addedAt)
        let run = Task { await grid.run() }
        defer { run.cancel() }
        await grid.ready()
        XCTAssertEqual(grid.state.tab, .titles)
        XCTAssertEqual(grid.state.sort, .addedAt)
        XCTAssertTrue(grid.state.filters.isEmpty, "none of the kept filters")
        XCTAssertEqual(grid.paged.item(at: 0)?.title, Self.name(Self.total - 1), "newest first")
        grid.toggle(GridFilter.Resolution(resolution: .fourK))
        grid.choose(sort: .runtime)
        let again = model()
        XCTAssertEqual(again.state.sort, .rating, "the library's own choices stay")
        XCTAssertEqual(again.state.filters.chosen.map(\.key), ["played:unplayed"])
    }

    func testAFlingReadsOnlyThePagesWhereItComesToRest() async throws {
        let grid = model()
        let run = Task { await grid.run() }
        defer { run.cancel() }
        await grid.ready()
        let before = server.requests(to: Self.worksPath).count
        for row in stride(from: 0, to: 2400, by: 60) {
            grid.paged.show(row...(row + 20))
            try await Task.sleep(for: .milliseconds(16))
        }
        try await until("where it rests loads") { grid.paged.item(at: 2350) != nil }
        let starts = server.requests(to: Self.worksPath).dropFirst(before).map { request in
            Self.asked(request)["offset"] as? Int ?? 0
        }
        XCTAssertEqual(Set(starts), [2280, 2340], "the pages where it rests and the one before, none it passed")
    }

    func testAListsGridReadsOnlyThePagesWhereItComesToRest() async throws {
        let grid = PagedGrid(session.library.pagedListWorks(listId: "l1", kinds: nil, settleMs: PagedGrid.settleMs))
        let run = Task { await grid.run() }
        defer { run.cancel() }
        await grid.ready()
        XCTAssertEqual(grid.total, Self.total)
        XCTAssertEqual(grid.item(at: 0)?.title, Self.name(0))
        let page = grid.page(containing: 1210)
        // Cards appearing down the list, as it's flung.
        for index in stride(from: 0, to: 1230, by: 30) {
            grid.show(index...index)
            try await Task.sleep(for: .milliseconds(16))
        }
        try await until("where it rests loads") { page.item(at: 1210) != nil }
        XCTAssertEqual(page.item(at: 1210)?.title, Self.name(1210))
        let starts = server.requests(to: Self.listPath).compactMap { $0.query["offset"] }
        XCTAssertEqual(Set(starts), ["1140", "1200"], "the page where it rests and the one before, none it passed")
    }

    func testAFirstPageThatCantBeReadSaysWhy() async throws {
        Self.failing = true
        let grid = model()
        let run = Task { await grid.run() }
        defer { run.cancel() }
        await grid.ready()
        XCTAssertNil(grid.paged.total)
        XCTAssertEqual(grid.paged.failed?.code, "notFound")
    }

    func testTheFiltersOfferTheLibrarysValuesInTheirGroupsWithEachNamed() async throws {
        let grid = model()
        let run = Task { await grid.run() }
        defer { run.cancel() }
        try await until("the filters arrive") { grid.state.offer.count > 1 }
        let offer = grid.state.offer
        XCTAssertEqual(offer.map(\.kind), [.watchStatus, .decade, .genre, .contentRating, .quality], "narrators are a books library's")
        XCTAssertEqual(offer.map { GridModel.title($0.kind) }, ["Watch status", "Decade", "Genre", "Content rating", "Quality"])
        XCTAssertEqual(offer[0].values.map { grid.label($0) }, ["Unwatched", "In progress", "Watched"])
        XCTAssertEqual(offer[1].values.map { grid.label($0) }, ["2020s", "2000s", "1990s", "1980s", "1890s"])
        XCTAssertEqual(offer[2].values.map { grid.label($0) }, ["Action, Adventure", "Comedy", "Drama"])
        XCTAssertEqual(offer[3].values.map { grid.label($0) }, ["G", "PG-13", "R"], "in the server's order")
        XCTAssertEqual(offer[4].values.map { grid.label($0) }, ["4K", "1080p", "HDR"])
        XCTAssertEqual(grid.label(played: nil), "All")
        XCTAssertEqual([WorkPlayState.unplayed, .inProgress, .played].map { GridModel.emptyName(played: $0, books: false) }, ["unwatched", "in-progress", "watched"])
        XCTAssertEqual([WorkPlayState.unplayed, .inProgress, .played].map { GridModel.emptyName(played: $0, books: true) }, ["unstarted", "in-progress", "finished"])
        XCTAssertEqual(grid.sections.map(\.label), ["Movies"])
        XCTAssertEqual(grid.sorts.map(\.label), ["Title", "Date added", "Release date", "Rating", "Runtime", "Last played"])
        XCTAssertEqual(grid.noun(41), "41 movies")
    }

    func testTheChosenFiltersShowAsChipsEachTakingItselfOff() async throws {
        let grid = model()
        let run = Task { await grid.run() }
        defer { run.cancel() }
        try await until("the filters arrive") { grid.state.offer.count > 1 }
        for filter in [
            GridFilter.Genre(name: "Drama"), GridFilter.Genre(name: "Action, Adventure"), GridFilter.Decade(year: 1990),
            GridFilter.ContentRating(name: "R"), GridFilter.ContentRating(name: "G"), GridFilter.Resolution(resolution: .p1080),
            GridFilter.Resolution(resolution: .fourK), GridFilter.DynamicRange(range: .hdr), GridFilter.Played(state: .unplayed),
        ] {
            grid.toggle(filter)
        }
        XCTAssertFalse(grid.state.filters.isEmpty)
        XCTAssertEqual(grid.state.activeFilters.map { grid.chipLabel($0) }, [
            "Unwatched", "1990s", "Action, Adventure", "Drama", "G", "R", "4K", "1080p", "HDR",
        ], "the groups in the sheet's order, each as the filters offer it")
        grid.toggle(GridFilter.Decade(year: 1990))
        XCTAssertFalse(grid.state.filters.has(filter: GridFilter.Decade(year: 1990)))
        grid.toggle(GridFilter.Genre(name: "Drama"))
        XCTAssertEqual(grid.state.activeFilters.map(\.key).filter { $0.hasPrefix("genre:") }, ["genre:Action, Adventure"])
        grid.choose(tab: .collections)
        XCTAssertTrue(grid.state.activeFilters.isEmpty, "no chips over a tab they don't filter")
        grid.choose(tab: .titles)
        grid.choose(sort: .addedAt)
        grid.clearFilters()
        XCTAssertTrue(grid.state.filters.isEmpty)
        XCTAssertTrue(grid.state.activeFilters.isEmpty)
        XCTAssertEqual(grid.state.sort, .addedAt, "the sort stays")
    }

    func testTheChosenFiltersAreKeptForTheLibrary() async throws {
        let grid = model()
        grid.toggle(GridFilter.Genre(name: "Action, Adventure"))
        grid.toggle(GridFilter.Genre(name: "Drama"))
        grid.toggle(GridFilter.Decade(year: 1980))
        grid.toggle(GridFilter.ContentRating(name: "PG-13"))
        grid.toggle(GridFilter.Resolution(resolution: .fourK))
        grid.toggle(GridFilter.Narrator(id: "p1", name: "Ray Porter"))
        let again = model()
        XCTAssertEqual(again.state.filters, grid.state.filters)
        XCTAssertEqual(again.state.activeFilters.last.map { again.chipLabel($0) }, "Narrated by Ray Porter", "named before the narrators are read")
    }

    func testTheSheetsChoicesReachTheGridOnceTheyHoldStill() async throws {
        let grid = model()
        let run = Task { await grid.run() }
        defer { run.cancel() }
        await grid.ready()
        let before = server.requests(to: Self.worksPath).count
        let generation = grid.generation
        grid.toggle(GridFilter.Genre(name: "Drama"), settle: true)
        grid.toggle(GridFilter.Genre(name: "Comedy"), settle: true)
        XCTAssertEqual(grid.state.filters.chosen.count, 2, "chosen at once")
        XCTAssertEqual(grid.generation, generation, "the grid waits")
        try await until("the grid follows") { grid.generation > generation }
        try await until("the grid reads again") { server.requests(to: Self.worksPath).count > before }
        XCTAssertEqual(grid.generation, generation + 1, "one grid for the choices")
    }

    func testTheGridAsksForTheChosenFilters() async throws {
        let grid = model()
        let run = Task { await grid.run() }
        defer { run.cancel() }
        await grid.ready()
        let before = server.requests(to: Self.worksPath).count
        var filters = grid.state.filters
        for filter in [
            GridFilter.Played(state: .played), GridFilter.Genre(name: "Drama"), GridFilter.Genre(name: "Comedy"), GridFilter.Decade(year: 1990),
            GridFilter.ContentRating(name: "R"), GridFilter.DynamicRange(range: .hdr),
        ] {
            filters = filters.toggling(filter: filter)
        }
        grid.choose(filters)
        try await until("the grid reads again") { server.requests(to: Self.worksPath).count > before }
        let read = Self.asked(server.requests(to: Self.worksPath)[before])
        let rules = ((read["query"] as? [String: Any])?["where"] as? [String: Any])?["rules"] as? [[String: Any]] ?? []
        XCTAssertEqual(rules.map { [$0["field"] as? String ?? "", $0["op"] as? String ?? ""] + ($0["values"] as? [String] ?? []) }, [
            ["playState", "anyOf", "played"],
            ["genre", "anyOf", "Comedy", "Drama"],
            ["year", "between", "1990", "1999"],
            ["contentRating", "anyOf", "R"],
            ["dynamicRange", "anyOf", "hdr"],
        ])
        XCTAssertEqual(read["library"] as? String, "lib")
    }

    func testSaveAsSmartListCarriesEveryFilter() {
        let grid = model()
        grid.toggle(GridFilter.Genre(name: "Drama"))
        grid.toggle(GridFilter.Decade(year: 2000))
        grid.toggle(GridFilter.ContentRating(name: "PG-13"))
        let rules = grid.state.smartListQuery.where?.rules ?? []
        XCTAssertEqual(rules.map(\.field), ["library", "genre", "year", "contentRating"])
        XCTAssertEqual(rules[2].propertyValues, ["2000", "2009"])
    }

    // Migration (grid-choice): remove after every client has updated past this release.
    func testAGridKeptByAnEarlierVersionOpensAsItWasLeft() async throws {
        UserDefaults.standard.set([
            "tab": "collections", "sort": "runtimeMs", "played": "inProgress", "decade": "1990",
            "genres": "Action, Adventure\nDrama", "contentRatings": "PG-13",
            "resolutions": "1080p,4k,sd", "dynamicRanges": "dolbyVision,hdr", "narrator": "p1", "narratorName": "Ray Porter",
        ], forKey: "grid.lib")
        UserDefaults.standard.set(true, forKey: "grid.lib.collections")
        let upgraded = try await open()
        defer { upgraded.close() }

        let grid = GridModel(library: library, session: upgraded)

        XCTAssertEqual(grid.state.tab, .collections)
        XCTAssertEqual(grid.state.sort, .runtime)
        XCTAssertEqual(grid.state.filters.chosen.map(\.key), [
            "played:inProgress", "decade:1990", "genre:Action, Adventure", "genre:Drama", "contentRating:PG-13",
            "resolution:1080p", "resolution:4k", "resolution:sd", "dynamicRange:dolbyVision", "dynamicRange:hdr", "narrator:p1",
        ])
        XCTAssertTrue(grid.state.tabsKnown, "whether it has collections too")
        XCTAssertEqual(grid.sections.map(\.label), ["Movies", "Collections"])
        XCTAssertNil(UserDefaults.standard.object(forKey: "grid.lib"), "moved to where it's kept now")
        XCTAssertNil(UserDefaults.standard.object(forKey: "grid.lib.collections"))
    }

    // Migration (grid-choice): remove after every client has updated past this release.
    // Migration (api-naming): remove after every client has updated past this release.
    func testAGridKeptUnderEarlierNamesOpensInTodaysNames() async throws {
        UserDefaults.standard.set([
            "tab": "titles", "sort": "lastEpisodeAdded", "played": "unplayed", "resolutions": "720p", "hdr": "dolbyVision",
        ], forKey: "grid.lib")
        let upgraded = try await open()
        defer { upgraded.close() }

        let grid = GridModel(library: library, session: upgraded)

        XCTAssertEqual(grid.state.sort, .lastEpisodeAddedAt)
        XCTAssertEqual(grid.state.filters.chosen.map(\.key), ["played:unplayed", "resolution:720p", "dynamicRange:dolbyVision"])
    }

    // Migration (grid-choice): remove after every client has updated past this release.
    func testAKeptSortOrFilterThisVersionDoesntKnowFallsBack() async throws {
        UserDefaults.standard.set([
            "tab": "titles", "sort": "loudness", "played": "abandoned", "resolutions": "8k,4k", "dynamicRanges": "hlg",
        ], forKey: "grid.lib")
        let upgraded = try await open()
        defer { upgraded.close() }

        let grid = GridModel(library: library, session: upgraded)

        XCTAssertEqual(grid.state.sort, .title, "the first sort")
        XCTAssertEqual(grid.state.filters.chosen.map(\.key), ["resolution:4k"])
    }
}

/// A device that downloads nothing.
/// Downloads that never transfer, for a test's device.
final class NoTransport: NSObject, DownloadTransport {
    func start(request: TransferRequest) {}
    func stop(id: String) {}
    func cancel(id: String) {}
    func running() -> [String] { [] }
}

private extension GridItem {
    /// A work's name; nil for a card of another kind.
    var title: String? {
        guard case .work(let item) = onEnum(of: self) else { return nil }
        return item.work.title
    }
}
