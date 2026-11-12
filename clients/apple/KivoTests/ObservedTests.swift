import Foundation
import XCTest
@testable import Kivo

/// Observed mirrors a flow into a value SwiftUI follows: it starts at the
/// value given, takes each one on the main actor, and stops following as
/// it's released.
@MainActor
final class ObservedTests: XCTestCase {
    func testItStartsAtTheValueGivenAndTakesEachOne() async {
        let (values, send) = AsyncStream<Int>.makeStream()
        var told: [Int] = []
        let observed = Observed(1, values) { told.append($0) }
        XCTAssertEqual(observed.value, 1)

        send.yield(2)
        send.yield(3)
        await until { observed.value == 3 }

        XCTAssertEqual(told, [2, 3])
    }

    func testItStopsFollowingAsItIsReleased() async {
        let stopped = expectation(description: "the flow is no longer followed")
        let (values, send) = AsyncStream<Int>.makeStream()
        send.onTermination = { _ in stopped.fulfill() }
        var observed: Observed<Int>? = Observed(0, values)
        send.yield(1)
        await until { observed?.value == 1 }
        weak let released = observed

        observed = nil

        XCTAssertNil(released, "following a flow keeps its Observed alive")
        await fulfillment(of: [stopped], timeout: 5)
    }

    /// Waits for [met], for up to five seconds.
    private func until(_ met: () -> Bool, file: StaticString = #filePath, line: UInt = #line) async {
        for _ in 0..<500 where !met() {
            try? await Task.sleep(for: .milliseconds(10))
        }
        XCTAssert(met(), "it never came", file: file, line: line)
    }
}
