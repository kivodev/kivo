import SwiftUI
import UIKit
import XCTest
@testable import Kivo

/// The action row measures each round button once, at its ideal size, and
/// places it at that size: Trailer measures the same as Play from
/// Beginning.
@MainActor
final class ActionRowTests: XCTestCase {
    func testTrailerIsTheSizeOfTheOtherRoundButtons() throws {
        let restart = idealSize(RestartButton {})
        let trailer = idealSize(TrailerButton(url: try XCTUnwrap(URL(string: "https://www.youtube.com/watch?v=yUQM7H4Swgw"))))
        XCTAssertGreaterThan(restart.width, 0)
        XCTAssertEqual(trailer, restart, "Trailer isn't the size of the row's other round buttons")
    }

    /// [view]'s size for the proposal ``ActionRowLayout`` measures with, in
    /// the row's control size.
    private func idealSize(_ view: some View) -> CGSize {
        let size = Measured()
        let host = UIHostingController(rootView: IdealSize(size: size) { view }.controlSize(.large))
        let window = UIWindow(frame: CGRect(x: 0, y: 0, width: 1024, height: 768))
        window.rootViewController = host
        window.makeKeyAndVisible()
        host.view.layoutIfNeeded()
        window.isHidden = true
        return size.value
    }
}

private final class Measured {
    var value = CGSize.zero
}

/// Records its one subview's size for an unspecified proposal.
private struct IdealSize: Layout {
    let size: Measured

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout Void) -> CGSize {
        let ideal = subviews.first?.sizeThatFits(.unspecified) ?? .zero
        size.value = ideal
        return ideal
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout Void) {
        subviews.first?.place(at: bounds.origin, proposal: ProposedViewSize(size.value))
    }
}
