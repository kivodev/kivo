import Foundation
import KivoCore
import XCTest
@testable import Kivo

/// What the Apple TV shows of the server's rows, lists and libraries, how
/// its settings move a library or a row, and what Top Shelf keeps.
@MainActor
final class TVRulesTests: XCTestCase {
    private func work(_ id: String, _ kind: WorkKind, positionMs: Int32? = nil, episode: EpisodeInfo? = nil) -> WorkSummary {
        WorkSummary(
            id: id, kind: kind, title: "Title \(id)", matchState: .matched, available: true,
            images: WorkImages(poster: ImageRef(id: "poster-\(id)", blurhash: nil, width: nil, height: nil), backdrop: nil, still: nil, logo: nil),
            year: nil, durationMs: KotlinInt(int: 600_000), communityRating: nil, communityVotes: nil, contentRating: nil, addedAt: nil,
            userState: positionMs.map { UserState(played: false, positionMs: KotlinInt(int: $0), versionId: nil, lastPlayedAt: nil) },
            movie: nil, series: nil, season: nil, episode: episode, extra: nil, book: nil, libraryId: nil, path: nil
        )
    }

    private func library(_ id: String, _ kind: LibraryKind = .movies, pinned: Bool = true) -> Library {
        Library(
            id: id, name: id, pinned: pinned, kind: kind, workCount: 1, folders: nil, useLocalMetadata: nil, previews: nil, markers: nil,
            subtitles: nil, refreshDays: nil, metadataLanguage: nil, contentRatingCountry: nil, unmatchedCount: nil, scan: nil
        )
    }

    private func row(_ id: String, _ kind: HomeRowKind, _ items: [WorkSummary], library: Library? = nil, list: ListRef? = nil) -> HomeRow {
        HomeRow(id: id, kind: kind, items: items, library: library.map { LibraryRef(id: $0.id, name: $0.name, kind: $0.kind) }, list: list)
    }

    func testHomeShowsTheRowsTheTVHasAndLeavesTheBooksOut() {
        let movies = library("movies")
        let videos = library("videos", .videos)
        let books = library("books", .audiobooks)
        let list = ListRef(id: "my", name: "Watchlist", kind: .manual)
        let shown = ShownRow.shown([
            row("continue", .continueWatching, [work("e1", .episode)]),
            row("listening", .continueListening, [work("b1", .book)]),
            row("recent-movies", .recentlyAdded, [work("m1", .movie)], library: movies),
            row("recent-videos", .recentlyAdded, [work("v1", .video)], library: videos),
            row("recent-books", .recentlyAdded, [work("b2", .book)], library: books),
            row("list", .list, [work("m2", .movie), work("b3", .book)], list: list),
            row("books-list", .list, [work("b4", .book)], list: list),
            row("empty", .recentlyAdded, [], library: movies),
        ], libraries: ServerModel.tvLibraries([movies, videos, books]))

        XCTAssertEqual(shown.map(\.row.id), ["continue", "recent-movies", "recent-videos", "list"])
        XCTAssertEqual(shown.map(\.style), [.landscape, .poster, .landscape, nil])
        XCTAssertEqual(shown.last?.works.map(\.id), ["m2"])
    }

    func testAListHasAShelfOnlyWithSomethingTheTVShows() {
        let lists = [
            WorkList(id: "my", name: "Watchlist", kind: .manual, builtIn: true, workCount: 2, smart: nil, items: [work("m1", .movie), work("b1", .book)], sort: nil),
            WorkList(id: "books", name: "Books", kind: .manual, builtIn: false, workCount: 1, smart: nil, items: [work("b2", .book)], sort: nil),
        ]
        let shelves = ListsView.shelves(lists)
        XCTAssertEqual(shelves.map(\.list.id), ["my"])
        XCTAssertEqual(shelves.first?.works.map(\.id), ["m1"])
    }

    func testMovingSkipsWhatTheTVDoesntList() {
        let all = [library("movies"), library("books", .audiobooks), library("shows", .shows), library("home", .videos)]
        let listed = { (library: Library) in library.kind != .audiobooks }
        XCTAssertEqual(moved(all, id: "shows", by: -1, of: \.id, listed: listed).map(\.id), ["shows", "books", "movies", "home"])
        XCTAssertEqual(moved(all, id: "shows", by: 1, of: \.id, listed: listed).map(\.id), ["movies", "books", "home", "shows"])
        XCTAssertEqual(moved(all, id: "movies", by: -1, of: \.id, listed: listed).map(\.id), all.map(\.id))
        XCTAssertEqual(moved(all, id: "books", by: 1, of: \.id, listed: listed).map(\.id), all.map(\.id))
    }

    func testTopShelfKeepsContinueWatchingOnceEachInItsOrder() {
        let episode = EpisodeInfo(seasonNumber: 2, number: 5, seriesId: "s", seriesTitle: "The Show", endNumber: nil, airDate: nil, seasonId: nil, previousId: nil, nextId: nil, overview: nil)
        let home = Home(rows: [
            row("continue", .continueWatching, [work("e1", .episode, positionMs: 300_000, episode: episode), work("m1", .movie), work("e1", .episode, episode: episode)]),
            row("recent", .recentlyAdded, [work("m2", .movie)]),
        ], allHidden: nil)
        let shelved = TopShelf.shelved(home)
        XCTAssertEqual(shelved.map(\.id), ["e1", "m1"])
        XCTAssertEqual(TopShelf.title(shelved[0]), "The Show · S2 · E5")
        XCTAssertEqual(TopShelf.title(shelved[1]), "Title m1")
        XCTAssertEqual(shelved[0].progress, 0.5)
    }

    func testTopShelfLinksOpenAWorkOrPlayIt() {
        XCTAssertEqual(TopShelfLink(URL(string: "kivo://work/w1")!), .work("w1"))
        XCTAssertEqual(TopShelfLink(URL(string: "kivo://play/w1")!), .play("w1"))
        XCTAssertNil(TopShelfLink(URL(string: "kivo://link?code=ABC123")!))
        XCTAssertNil(TopShelfLink(URL(string: "https://work/w1")!))
        XCTAssertEqual(TopShelfLink(TopShelfLink.play("w1").url), .play("w1"))
    }
}
