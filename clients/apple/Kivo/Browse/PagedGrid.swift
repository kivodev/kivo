import Foundation
import KivoCore
import KivoUI
import Observation

/// One of the core's paged grids (`GridPager`) as a screen shows it: its
/// count, the letter scrubber's starts, why its first page couldn't be
/// read, and its pages. Each page is observed on its own, so a page
/// arriving updates only its own cards. The screen says which cards it
/// shows, and the pages there load once the grid has held still. A first
/// page seen lately shows at once while it's read again.
@MainActor
@Observable
final class PagedGrid {
    /// How long a grid holds still before the pages where it rests load, so
    /// a fling or a scrub reads none of the pages it passes; each grid is
    /// made with it.
    static let settleMs: Int64 = 150

    /// A page of the grid, its items once they've arrived.
    @MainActor
    @Observable
    final class Page {
        fileprivate(set) var items: [GridItem]?
        @ObservationIgnored fileprivate let offset: Int

        fileprivate init(offset: Int) {
            self.offset = offset
        }

        /// The item at [index] of the grid, nil until the page arrives.
        func item(at index: Int) -> GridItem? {
            guard let items, items.indices.contains(index - offset) else { return nil }
            return items[index - offset]
        }
    }

    private(set) var total: Int?
    private(set) var letters: [LetterStart] = []
    /// Why the first page couldn't be read.
    private(set) var failed: RequestProblem?

    /// Whether the first page waits to show, for what the screen shows it with.
    @ObservationIgnored var holdsFirstPage: () -> Bool = { false }

    /// The pages by number, made as cards ask for them; the cards observe
    /// each page, not this.
    @ObservationIgnored private var pages: [Int: Page] = [:]
    @ObservationIgnored private let grid: GridPager
    @ObservationIgnored private let pageSize: Int
    @ObservationIgnored private var closed = false

    init(_ grid: GridPager) {
        self.grid = grid
        pageSize = Int(grid.state.value.pageSize)
        apply(grid.state.value, atOnce: true)
    }

    deinit {
        grid.close()
    }

    /// Stops reading: another grid shows in its place.
    func close() {
        closed = true
        grid.close()
    }

    /// Follows the grid until cancelled, showing each change.
    func run() async {
        for await state in grid.state {
            apply(state)
        }
    }

    /// Returns once the grid has something to show: its first page, from
    /// the cache or the server, or why it couldn't be read.
    func ready() async {
        for await state in grid.state {
            apply(state)
            if total != nil || failed != nil || closed { return }
        }
    }

    /// Shows the grid as read so far, as once what the first page waited for is known.
    func catchUp() {
        apply(grid.state.value)
    }

    /// Reads the pages on screen again, keeping the place, as for a pull to refresh.
    func refresh() async {
        try? await grid.refresh()
    }

    /// The page holding the card at [index].
    func page(containing index: Int) -> Page {
        let number = index / pageSize
        if let page = pages[number] { return page }
        let page = Page(offset: number * pageSize)
        page.items = grid.state.value.items(page: Int32(number))
        pages[number] = page
        return page
    }

    /// The item at [index], nil until its page arrives.
    func item(at index: Int) -> GridItem? {
        grid.state.value.item(index: Int32(index))
    }

    /// The cards at [visible] are on screen: once the grid holds still,
    /// their pages load, and the next page either way once they're past the
    /// middle of theirs.
    func show(_ visible: ClosedRange<Int>) {
        let size = pageSize
        var low = visible.lowerBound / size
        var high = visible.upperBound / size
        if visible.upperBound % size >= size / 2 { high += 1 }
        if visible.lowerBound % size < size / 2 { low = max(low - 1, 0) }
        grid.load(first: Int32(low * size), last: Int32((high + 1) * size - 1))
    }

    /// The letter a card at [index] files under.
    func letter(at index: Int) -> String? {
        letters.last { Int($0.offset) <= index }?.letter
    }

    /// Shows [state]; set only where changed, so a read that finds nothing
    /// new redraws nothing; [atOnce] shows a first page seen before.
    private func apply(_ state: GridPages, atOnce: Bool = false) {
        guard !closed else { return }
        if total == nil, !atOnce, state.problem == nil, holdsFirstPage() { return }
        let count = state.total.map { Int(truncating: $0) }
        if total != count { total = count }
        if letters != state.letters { letters = state.letters }
        if failed != state.problem { failed = state.problem }
        for (number, page) in pages {
            let items = state.items(page: Int32(number))
            if items != page.items { page.items = items }
        }
    }
}

extension GridItem {
    /// The image its card shows, in a card of [style].
    func artwork(_ style: CardStyle) -> ImageRef? {
        switch onEnum(of: self) {
        case .work(let item): style.artwork(of: item.work)
        case .collection: nil
        case .author(let item): item.author.photo
        case .bookSeries(let item): item.series.cover
        }
    }
}
