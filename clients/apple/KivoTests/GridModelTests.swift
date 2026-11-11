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
            return (200, json(["items": []]))
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
        XCTAssertEqual(grid.total, Self.total)
        XCTAssertEqual(grid.letters.count, 26)
        XCTAssertNil(grid.failed)
        XCTAssertEqual(grid.item(at: 0)?.title, Self.name(0))
        XCTAssertEqual(grid.item(at: 59)?.title, Self.name(59))
        XCTAssertNil(grid.item(at: 60), "a page isn't read before it's shown")
        XCTAssertEqual(grid.letter(at: 0), "A")
        XCTAssertEqual(grid.letter(at: 1500), "N")
    }

    func testTheCardsOnScreenLoadTheirPagesAndFillTheirPagesCards() async throws {
        let grid = model()
        let run = Task { await grid.run() }
        defer { run.cancel() }
        await grid.ready()
        let page = grid.page(containing: 1210)
        XCTAssertNil(page.item(at: 1210))
        grid.show(1200...1230)
        try await until("page 20 loads") { page.item(at: 1210) != nil }
        XCTAssertEqual(page.item(at: 1210)?.title, Self.name(1210))
        XCTAssertEqual(grid.item(at: 1200)?.title, Self.name(1200))
        XCTAssertNil(grid.item(at: 2400), "a page far off isn't read")
    }

    func testARefreshReadsTheLoadedPagesAgainInPlace() async throws {
        let grid = model()
        let run = Task { await grid.run() }
        defer { run.cancel() }
        await grid.ready()
        grid.show(1200...1230)
        try await until("page 20 loads") { grid.item(at: 1210) != nil }
        let generation = grid.generation
        Self.prefix = "Renamed"
        await grid.refresh()
        try await until("page 20 reads again") { grid.item(at: 1210)?.title == Self.name(1210) }
        XCTAssertEqual(grid.item(at: 0)?.title, Self.name(0))
        XCTAssertEqual(grid.generation, generation, "the same grid, in place")
        XCTAssertEqual(grid.total, Self.total)
    }

    func testALibrarySeenLatelyShowsAtOnce() async throws {
        let first = model()
        let run = Task { await first.run() }
        await first.ready()
        run.cancel()
        let again = model()
        XCTAssertEqual(again.total, Self.total, "the count before any read")
        XCTAssertEqual(again.item(at: 0)?.title, Self.name(0), "the first cards before any read")
    }

    func testAnotherSortShowsItsOwnOrderFromTheTop() async throws {
        let grid = model()
        let run = Task { await grid.run() }
        defer { run.cancel() }
        await grid.ready()
        let generation = grid.generation
        grid.query.sort = grid.sorts.first { $0.value == .addedAt }!
        grid.reload()
        try await until("the new order shows") { grid.item(at: 0)?.title == Self.name(Self.total - 1) }
        XCTAssertGreaterThan(grid.generation, generation, "a new grid")
        XCTAssertEqual(grid.shown.sort.value, .addedAt)
        XCTAssertTrue(grid.letters.isEmpty, "no letters by date")
    }

    func testAnotherSortShowsItsPlaceholdersAtOnce() async throws {
        let grid = model()
        let run = Task { await grid.run() }
        defer { run.cancel() }
        await grid.ready()
        grid.query.sort = grid.sorts.first { $0.value == .addedAt }!
        grid.reload()
        XCTAssertEqual(grid.shown.sort.value, .addedAt, "the new sort shows at once")
        XCTAssertNil(grid.total, "its placeholders until its first page arrives, not the grid before")
        XCTAssertNil(grid.item(at: 0))
        try await until("its first page arrives") { grid.total == Self.total }
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
        XCTAssertEqual(grid.item(at: 0)?.title, Self.name(Self.total - 1), "newest first")
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
            grid.show(row...(row + 20))
            try await Task.sleep(for: .milliseconds(16))
        }
        try await until("where it rests loads") { grid.item(at: 2350) != nil }
        let starts = server.requests(to: Self.worksPath).dropFirst(before).map { request in
            Self.asked(request)["offset"] as? Int ?? 0
        }
        XCTAssertEqual(Set(starts), [2280, 2340], "the pages where it rests and the one before, none it passed")
    }

    func testAFirstPageThatCantBeReadSaysWhy() async throws {
        Self.failing = true
        let grid = model()
        let run = Task { await grid.run() }
        defer { run.cancel() }
        await grid.ready()
        XCTAssertNil(grid.total)
        XCTAssertEqual(grid.failed?.code, "notFound")
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
            "tab": "titles", "sort": "runtimeMs", "played": "inProgress",
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
            "tab": "titles", "sort": "lastEpisodeAddedAt", "played": "unplayed", "resolutions": "720p", "dynamicRanges": "dolbyVision",
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

private extension Kivo.LibraryItem {
    /// A work's name; nil for a card of another kind.
    var title: String? {
        guard case .work(let work) = self else { return nil }
        return work.title
    }
}
