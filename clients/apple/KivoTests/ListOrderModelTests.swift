import Foundation
import KivoCore
import XCTest
@testable import Kivo

/// A manual list's rows against a server on localhost: they are read in
/// the custom order, a move shows at once and is sent with the row it
/// lands after, moves are sent in turn, and one the server refuses is
/// undone by reading the order again.
@MainActor
final class ListOrderModelTests: XCTestCase {
    private var server: StubServer!
    private var session: KivoSession!
    /// The list's order on the server, which a move it accepts changes.
    nonisolated(unsafe) private static var order: [String] = []
    nonisolated(unsafe) private static var refusing = false

    private static let worksPath = "/api/v1/lists/l1/works"
    private static let movePath = "/api/v1/lists/l1/move"

    private static func json(_ value: Any) -> String {
        String(decoding: try! JSONSerialization.data(withJSONObject: value), as: UTF8.self)
    }

    private static func answer(_ request: StubServer.Request) -> (Int, String) {
        switch request.path {
        case "/api/v1/server":
            return (200, json(["id": "order-test", "name": "Order test", "version": "1.0"]))
        case worksPath:
            let items = order.map { ["id": $0, "kind": "movie", "title": "Movie \($0)", "matchState": "matched", "available": true, "images": [:]] as [String: Any] }
            return (200, json(["items": items, "total": items.count]))
        case movePath:
            if refusing {
                return (409, json(["type": "about:blank", "title": "Conflict", "status": 409, "code": "conflict", "requestId": "r"]))
            }
            let move = (try? JSONSerialization.jsonObject(with: request.body)) as? [String: Any] ?? [:]
            let moved = move["workIds"] as? [String] ?? []
            var rest = order.filter { !moved.contains($0) }
            let at = (move["after"] as? String).flatMap { rest.firstIndex(of: $0) }.map { $0 + 1 } ?? 0
            rest.insert(contentsOf: moved, at: at)
            order = rest
            return (204, "")
        default:
            return (404, json(["type": "about:blank", "title": "Not found", "status": 404, "code": "notFound", "requestId": "r"]))
        }
    }

    override func setUp() async throws {
        Self.order = ["a", "b", "c", "d"]
        Self.refusing = false
        server = try StubServer(answer: Self.answer)
        let address = try await server.start()
        let core = try KivoClient.forTests()
        session = try await core.restore(server: SavedServer(id: "order-test", name: "Order test", addresses: [address]))
    }

    override func tearDown() async throws {
        session.close()
        server.stop()
    }

    private func loaded() async -> ListOrderModel {
        let model = ListOrderModel(listId: "l1", session: session)
        await model.load()
        return model
    }

    private func moves() -> [[String: Any]] {
        server.requests(to: Self.movePath).map { (try? JSONSerialization.jsonObject(with: $0.body)) as? [String: Any] ?? [:] }
    }

    func testTheRowsAreReadInTheCustomOrder() async throws {
        let model = await loaded()
        XCTAssertEqual(model.works?.map(\.id), ["a", "b", "c", "d"])
        XCTAssertEqual(server.requests(to: Self.worksPath).last?.query["sort"], "custom")
    }

    func testAMoveShowsAtOnceAndIsSentWithTheRowItLandsAfter() async throws {
        let model = await loaded()
        model.move(from: IndexSet(integer: 3), to: 1)
        XCTAssertEqual(model.works?.map(\.id), ["a", "d", "b", "c"])
        model.move(from: IndexSet(integer: 2), to: 0)
        XCTAssertEqual(model.works?.map(\.id), ["b", "a", "d", "c"])
        await model.settle()
        let sent = moves()
        XCTAssertEqual(sent.count, 2)
        XCTAssertEqual(sent[0]["workIds"] as? [String], ["d"])
        XCTAssertEqual(sent[0]["after"] as? String, "a")
        XCTAssertEqual(sent[1]["workIds"] as? [String], ["b"])
        XCTAssertNil(sent[1]["after"])
        XCTAssertEqual(Self.order, ["b", "a", "d", "c"])
        XCTAssertNil(model.problem)
    }

    func testARowDroppedWhereItWasSendsNothing() async throws {
        let model = await loaded()
        model.move(from: IndexSet(integer: 1), to: 2)
        await model.settle()
        XCTAssertEqual(model.works?.map(\.id), ["a", "b", "c", "d"])
        XCTAssertTrue(moves().isEmpty)
    }

    func testAMoveTheServerRefusesIsUndoneAndSaysWhy() async throws {
        let model = await loaded()
        Self.refusing = true
        model.move(from: IndexSet(integer: 0), to: 4)
        XCTAssertEqual(model.works?.map(\.id), ["b", "c", "d", "a"])
        await model.settle()
        XCTAssertEqual(model.works?.map(\.id), ["a", "b", "c", "d"])
        XCTAssertNotNil(model.problem)
    }
}
