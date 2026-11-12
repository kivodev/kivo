import SwiftUI
import UIKit
import XCTest
@testable import Kivo

/// The count line's sort menu gets its whole width, beside the count at the
/// default text size and under it at the largest.
@MainActor
final class CountAndSortTests: XCTestCase {
    private enum Key: SortMenuKey {
        case title, releaseDate
        var short: String? { self == .title ? "A–Z" : "Newest" }
    }

    private static let sorts: [GridModel.Sort<Key>] = [
        .init(value: .title, label: "Title", direction: "A to Z"),
        .init(value: .releaseDate, label: "Release date", direction: "Newest first"),
    ]

    func testTheSortShowsWhole() throws {
        for size in [DynamicTypeSize.large, .accessibility5] {
            // As the library lays it out, above its grid.
            let line = VStack(alignment: .leading) {
                CountAndSort(count: "5 movies", sorts: Self.sorts, selection: .constant(Self.sorts[1]))
                    .frame(minHeight: 32)
                    .font(.footnote)
                    .padding(.horizontal, 16)
                Spacer()
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .dynamicTypeSize(size)
            let window = UIWindow(frame: CGRect(x: 0, y: 0, width: 402, height: 400))
            window.rootViewController = UIHostingController(rootView: line)
            window.makeKeyAndVisible()
            window.layoutIfNeeded()

            let menu = try XCTUnwrap(Self.button(in: window), "\(size)")
            XCTAssertGreaterThanOrEqual(menu.bounds.width, menu.intrinsicContentSize.width - 0.5, "\(size)")
            XCTAssertLessThanOrEqual(menu.convert(menu.bounds, to: window).maxX, window.bounds.width, "\(size)")
            window.isHidden = true
        }
    }

    private static func button(in view: UIView) -> UIButton? {
        if let button = view as? UIButton { return button }
        return view.subviews.lazy.compactMap { button(in: $0) }.first
    }
}
