import Foundation
import KivoCore
import Observation
import XCTest
@testable import Kivo

/// Observed mirrors a flow into a value SwiftUI follows: it starts at the
/// value given, takes each one on the main actor, tells its observers, and
/// stops following as it's released. Against a stream, and against the
/// core's own state flow.
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

    func testReadersAreToldAsTheValueChanges() async {
        let (values, send) = AsyncStream<String>.makeStream()
        let observed = Observed("starting", values)
        let changed = expectation(description: "a reader is told")
        withObservationTracking { _ = observed.value } onChange: { changed.fulfill() }

        send.yield("signed in")

        await fulfillment(of: [changed], timeout: 5)
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

    func testItFollowsTheCoresStateFlowFromItsValueNow() async throws {
        let directory = FileManager.default.temporaryDirectory.appending(path: UUID().uuidString)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let core = KivoClient(device: AppleDevice(
            directory: directory.path(percentEncoded: false), name: "iPhone", identity: AppleIdentity.current,
            clientId: nil, subtitles: [], trackSelection: false, transport: NoTransport()
        ))
        let state = Observed(core.state)
        guard case .starting = onEnum(of: state.value) else { return XCTFail("the app doesn't start out starting") }

        core.start()
        await until {
            if case .signedOut = onEnum(of: state.value) { true } else { false }
        }

        guard case let .signedOut(signedOut) = onEnum(of: state.value) else { return XCTFail("with no server signed in to, the app isn't signed out") }
        XCTAssertNil(signedOut.server)
    }

    /// Waits for [met], for up to five seconds.
    private func until(_ met: () -> Bool, file: StaticString = #filePath, line: UInt = #line) async {
        for _ in 0..<500 where !met() {
            try? await Task.sleep(for: .milliseconds(10))
        }
        XCTAssert(met(), "it never came", file: file, line: line)
    }
}
