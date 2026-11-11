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
            return (200, json(start == 0 ? ["items": items, "total": total] : ["items": items]))
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
        UserDefaults.standard.removeObject(forKey: "grid.lib")
        server = try StubServer(answer: Self.answer)
        let address = try await server.start()
        let directory = FileManager.default.temporaryDirectory.appending(path: UUID().uuidString)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let core = KivoClient(device: AppleDevice(
            directory: directory.path(percentEncoded: false), name: "Test", identity: AppleIdentity.current,
            clientId: nil, subtitles: [], trackSelection: false, transport: NoTransport()
        ))
        session = try await core.restore(server: SavedServer(id: "grid-test", name: "Grid test", addresses: [address]))
        for await load in session.library.libraries() {
            if let found = (load.getOrNull() as? [Library])?.first { library = found; break }
        }
    }

    override func tearDown() async throws {
        session.close()
        server.stop()
        UserDefaults.standard.removeObject(forKey: "grid.lib")
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
        grid.query.sort = grid.sorts.first { $0.value == .addedAt }!
        grid.reload()
        try await until("the new order shows") { grid.paged.item(at: 0)?.title == Self.name(Self.total - 1) }
        XCTAssertGreaterThan(grid.generation, generation, "a new grid")
        XCTAssertEqual(grid.shown.sort.value, .addedAt)
        XCTAssertTrue(grid.paged.letters.isEmpty, "no letters by date")
    }

    func testAnotherSortShowsItsPlaceholdersAtOnce() async throws {
        let grid = model()
        let run = Task { await grid.run() }
        defer { run.cancel() }
        await grid.ready()
        grid.query.sort = grid.sorts.first { $0.value == .addedAt }!
        grid.reload()
        XCTAssertEqual(grid.shown.sort.value, .addedAt, "the new sort shows at once")
        XCTAssertNil(grid.paged.total, "its placeholders until its first page arrives, not the grid before")
        XCTAssertNil(grid.paged.item(at: 0))
        try await until("its first page arrives") { grid.paged.total == Self.total }
    }

    func testASortForOnceOpensWithoutFiltersAndKeepsNothing() async throws {
        UserDefaults.standard.set(["tab": "collections", "sort": "title", "played": "unplayed"], forKey: "grid.lib")
        let grid = GridModel(library: library, session: session, sort: .addedAt)
        let run = Task { await grid.run() }
        defer { run.cancel() }
        await grid.ready()
        XCTAssertEqual(grid.shown.tab, .titles)
        XCTAssertEqual(grid.shown.sort.value, .addedAt)
        XCTAssertFalse(grid.shown.filtered, "none of the kept filters")
        XCTAssertEqual(grid.paged.item(at: 0)?.title, Self.name(Self.total - 1), "newest first")
        grid.query.resolutions = [.fourK]
        grid.reload()
        let kept = UserDefaults.standard.dictionary(forKey: "grid.lib") as? [String: String]
        XCTAssertEqual(kept, ["tab": "collections", "sort": "title", "played": "unplayed"], "the library's own choices stay")
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

    func testTheFiltersOfferTheLibrarysValuesWithItsDecadesNewestFirst() async throws {
        let grid = model()
        let run = Task { await grid.run() }
        defer { run.cancel() }
        try await until("the filters arrive") { !grid.decades.isEmpty }
        XCTAssertEqual(grid.decades, [2020, 2000, 1990, 1980, 1890])
        XCTAssertEqual(grid.genres, ["Action, Adventure", "Comedy", "Drama"])
        XCTAssertEqual(grid.contentRatings, ["G", "PG-13", "R"], "in the server's order")
        XCTAssertEqual(grid.resolutions, [.fourK, .p1080])
        XCTAssertEqual(grid.dynamicRanges, [.hdr])
        XCTAssertTrue(grid.narrators.isEmpty, "narrators are a books library's")
        XCTAssertEqual(GridModel.label(decade: 1990), "1990s")
    }

    func testTheChosenFiltersShowAsChipsEachTakingItselfOff() async throws {
        let grid = model()
        let run = Task { await grid.run() }
        defer { run.cancel() }
        try await until("the filters arrive") { !grid.genres.isEmpty }
        grid.query.played = grid.playedFilters[1]
        grid.query.genres = ["Drama", "Action, Adventure"]
        grid.query.decade = 1990
        grid.query.contentRatings = ["R", "G"]
        grid.query.resolutions = [.p1080, .fourK]
        grid.query.dynamicRanges = [.hdr]
        XCTAssertTrue(grid.query.filtered)
        XCTAssertEqual(grid.activeFilters.map(\.label), [
            "Unwatched", "Action, Adventure", "Drama", "G", "R", "1990s", "4K", "1080p", "HDR",
        ], "each group in the order the filters offer it")
        let decade = try XCTUnwrap(grid.activeFilters.first { $0.id == "decade" })
        decade.remove(&grid.query)
        XCTAssertNil(grid.query.decade)
        let drama = try XCTUnwrap(grid.activeFilters.first { $0.id == "genre:Drama" })
        drama.remove(&grid.query)
        XCTAssertEqual(grid.query.genres, ["Action, Adventure"])
        let sort = grid.sorts[1]
        grid.query.sort = sort
        grid.clearFilters()
        XCTAssertFalse(grid.query.filtered)
        XCTAssertTrue(grid.activeFilters.isEmpty)
        XCTAssertEqual(grid.query.sort, sort, "the sort stays")
    }

    func testTheChosenFiltersAreKeptForTheLibrary() async throws {
        let grid = model()
        grid.query.genres = ["Action, Adventure", "Drama"]
        grid.query.decade = 1980
        grid.query.contentRatings = ["PG-13"]
        grid.query.resolutions = [.fourK]
        grid.query.narrator = GridModel.Narrator(id: "p1", name: "Ray Porter")
        grid.reload()
        let again = model()
        XCTAssertEqual(again.query, grid.query)
        XCTAssertEqual(again.activeFilters.last?.label, "Narrated by Ray Porter", "named before the narrators are read")
    }

    func testTheGridAsksForTheChosenFilters() async throws {
        let grid = model()
        let run = Task { await grid.run() }
        defer { run.cancel() }
        await grid.ready()
        let before = server.requests(to: Self.worksPath).count
        grid.query.played = grid.playedFilters[3]
        grid.query.genres = ["Drama", "Comedy"]
        grid.query.decade = 1990
        grid.query.contentRatings = ["R"]
        grid.query.dynamicRanges = [.hdr]
        grid.reload()
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
        grid.query.genres = ["Drama"]
        grid.query.decade = 2000
        grid.query.contentRatings = ["PG-13"]
        let rules = grid.smartListQuery.where?.rules ?? []
        XCTAssertEqual(rules.map(\.field), ["library", "genre", "year", "contentRating"])
        XCTAssertEqual(rules[2].propertyValues, ["2000", "2009"])
        let books = GridModel.gridQuery(GridModel.Query(sort: grid.sorts[0], played: grid.playedFilters[0], narrator: .init(id: "p1", name: "")), of: library)
        XCTAssertEqual(books.narratorId, "p1")
    }

    // Migration (api-naming): remove after every client has updated past this release.
    func testAGridKeptByAnEarlierVersionOpensWithItsFilters() async throws {
        UserDefaults.standard.set(["tab": "titles", "sort": "addedAt", "hdr": "hdr"], forKey: "grid.lib")
        let grid = model()
        XCTAssertEqual(grid.query.sort.value, .addedAt)
        XCTAssertEqual(grid.query.dynamicRanges, [.hdr])
        let kept = UserDefaults.standard.dictionary(forKey: "grid.lib") as? [String: String]
        XCTAssertEqual(kept?["dynamicRanges"], "hdr", "kept in today's names")
        XCTAssertNil(kept?["hdr"])
    }

    func testAKeptGridOpensWithItsSortAndFiltersAndKeepsTheirSpelling() async throws {
        UserDefaults.standard.set([
            "tab": "titles", "sort": "runtimeMs", "played": "inProgress",
            "resolutions": "1080p,4k,sd", "dynamicRanges": "dolbyVision,hdr",
        ], forKey: "grid.lib")
        let grid = model()
        XCTAssertEqual(grid.query.sort.value, .runtime)
        XCTAssertEqual(grid.query.played.value, .inProgress)
        XCTAssertEqual(grid.query.resolutions, [.fourK, .p1080, .sd])
        XCTAssertEqual(grid.query.dynamicRanges, [.hdr, .dolbyVision])
        grid.query.resolutions.insert(.p720)
        grid.reload()
        let kept = UserDefaults.standard.dictionary(forKey: "grid.lib") as? [String: String]
        XCTAssertEqual(kept, [
            "tab": "titles", "sort": "runtimeMs", "played": "inProgress", "genres": "", "contentRatings": "",
            "resolutions": "1080p,4k,720p,sd", "dynamicRanges": "dolbyVision,hdr",
        ])
    }

    // Migration (api-naming): remove after every client has updated past this release.
    func testAGridKeptUnderEarlierNamesKeepsItsSortAndFiltersInTodaysNames() async throws {
        UserDefaults.standard.set([
            "tab": "titles", "sort": "lastEpisodeAdded", "played": "unplayed", "resolutions": "720p", "hdr": "dolbyVision",
        ], forKey: "grid.lib")
        // Only a shows library sorts by the last episode added.
        let shows = Library(
            id: library.id, name: "Shows", pinned: true, kind: .shows, workCount: library.workCount, folders: nil, useLocalMetadata: nil,
            previews: nil, markers: nil, subtitles: nil, refreshDays: nil, metadataLanguage: nil, contentRatingCountry: nil,
            unmatchedCount: nil, scan: nil
        )
        let grid = GridModel(library: shows, session: session)
        XCTAssertEqual(grid.query.sort.value, .lastEpisodeAddedAt)
        XCTAssertEqual(grid.query.played.value, .unplayed)
        XCTAssertEqual(grid.query.resolutions, [.p720])
        XCTAssertEqual(grid.query.dynamicRanges, [.dolbyVision])
        grid.reload()
        let kept = UserDefaults.standard.dictionary(forKey: "grid.lib") as? [String: String]
        XCTAssertEqual(kept, [
            "tab": "titles", "sort": "lastEpisodeAddedAt", "played": "unplayed", "genres": "", "contentRatings": "",
            "resolutions": "720p", "dynamicRanges": "dolbyVision",
        ])
    }

    func testAKeptSortOrFilterThisVersionDoesntKnowFallsBack() async throws {
        UserDefaults.standard.set([
            "tab": "titles", "sort": "loudness", "played": "abandoned", "resolutions": "8k,4k", "dynamicRanges": "hlg",
        ], forKey: "grid.lib")
        let grid = model()
        XCTAssertEqual(grid.query.sort.value, .title, "the first sort")
        XCTAssertNil(grid.query.played.value, "all")
        XCTAssertEqual(grid.query.resolutions, [.fourK])
        XCTAssertTrue(grid.query.dynamicRanges.isEmpty)
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
