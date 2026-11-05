import KivoCore
import Nuke
import SwiftUI

/// Loads the artwork of a grid's cards before they scroll into view: about
/// the next two and a half screens of them in the direction it's scrolling,
/// at the size the cards ask for, so they find it in the memory cache. A
/// turn the other way stops what was ahead the old way.
@MainActor
public final class CardPrefetcher {
    private let prefetcher = ImagePrefetcher()
    private var shown: ClosedRange<Int>?
    private var forward = true
    /// The cards whose artwork has been asked for since the last turn.
    private var requested: Set<Int> = []

    public init() {}

    /// The artwork a grid's cards ask for: at their width in pixels, from [source].
    public struct Cards {
        let pixelWidth: CGFloat
        let source: ArtworkSource?

        public init(pixelWidth: CGFloat, source: ArtworkSource?) {
            self.pixelWidth = pixelWidth
            self.source = source
        }
    }

    /// The [cards] at [visible], of [total], are on screen. [artwork] gives
    /// the image of the card at an index, when its page has arrived and it
    /// has one.
    public func show(_ visible: ClosedRange<Int>, of total: Int, cards: Cards, artwork: (Int) -> ImageRef?) {
        if let shown, visible.lowerBound != shown.lowerBound {
            let forward = visible.lowerBound > shown.lowerBound
            if forward != self.forward {
                prefetcher.stopPrefetching()
                requested = []
                self.forward = forward
            }
        }
        shown = visible
        guard let source = cards.source else { return }
        let reach = visible.count * 5 / 2
        let ahead = forward
            ? (visible.upperBound + 1)..<min(total, visible.upperBound + 1 + reach)
            : max(0, visible.lowerBound - reach)..<visible.lowerBound
        var requests: [ImageRequest] = []
        for index in ahead where !requested.contains(index) {
            guard let image = artwork(index) else { continue }
            requested.insert(index)
            if let request = source.request(id: image.id, pixelWidth: cards.pixelWidth) {
                requests.append(request)
            }
        }
        if !requests.isEmpty { prefetcher.startPrefetching(with: requests) }
    }

    /// The grid starts again: what was ahead isn't.
    public func stop() {
        prefetcher.stopPrefetching()
        requested = []
        shown = nil
    }
}
