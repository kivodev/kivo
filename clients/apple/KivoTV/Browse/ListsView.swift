import KivoCore
import KivoUI
import SwiftUI

/// Lists: a shelf for each list with something the TV shows, in the
/// lists' order, its name over a row of its first works that ends in See
/// all, which opens it. Refreshes in place as the lists change.
struct ListsView: View {
    @Environment(ServerModel.self) private var server
    @State private var lists: [WorkList]?
    @State private var failure: RequestProblem?

    var body: some View {
        SkeletonGate(loaded: lists != nil || failure != nil || server.unreachable) {
            if let lists {
                let shelves = Self.shelves(lists)
                if shelves.isEmpty {
                    MessageView(String(localized: "Nothing in your lists yet."), sentence: String(localized: "Add something to Watchlist from its page."))
                } else {
                    ScrollView {
                        LazyVStack(alignment: .leading, spacing: TVLayout.rowGap) {
                            ForEach(shelves, id: \.list.id) { shelf in
                                let list = ListRef(id: shelf.list.id, name: shelf.list.name, kind: shelf.list.kind)
                                WorkRow(title: shelf.list.name, works: shelf.works) {
                                    server.navigation.push(.list(list))
                                }
                                .environment(\.currentList, list)
                            }
                        }
                    }
                }
            } else if let failure {
                MessageView(String(localized: "Couldn't load your lists."), sentence: failure.sentence)
            } else {
                UnreachableView()
            }
        } skeleton: {
            VStack(alignment: .leading, spacing: TVLayout.rowGap) {
                SkeletonCardRow(style: .poster)
                SkeletonCardRow(style: .poster)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        }
        .task {
            for await load in server.session.library.lists(counted: true) {
                if let read = load.getOrNull() as? [WorkList] {
                    lists = read
                    failure = nil
                } else if let problem = load.problemOrNull(), !problem.isUnreachable {
                    failure = problem
                }
            }
        }
    }

    /// A list's shelf: the list and its first works, books left out.
    struct Shelf {
        let list: WorkList
        let works: [WorkSummary]
    }

    /// The lists as shelves, in their order; a list with nothing the TV shows has none.
    static func shelves(_ lists: [WorkList]) -> [Shelf] {
        lists.filter(\.hasShelf)
            .map { Shelf(list: $0, works: ($0.items ?? []).filter { ShownRow.workKinds.contains($0.kind) }) }
            .filter { !$0.works.isEmpty }
    }
}

/// An open list: its name, how many it holds, and its works in a grid, each
/// card in its own work's shape, the pages around the cards on screen read
/// as the grid rests. With nothing in it, it says so.
struct ListWorksView: View {
    let list: ListRef
    @Environment(ServerModel.self) private var server

    var body: some View {
        ListWorksGrid(list: list, session: server.session)
            .id(list.id)
    }
}

private struct ListWorksGrid: View {
    let list: ListRef
    @State private var paged: PagedGrid
    @Environment(\.kivoPalette) private var palette

    init(list: ListRef, session: KivoSession) {
        self.list = list
        let kinds = Array(ShownRow.workKinds)
        _paged = State(initialValue: PagedGrid(session.library.pagedListWorks(listId: list.id, kinds: kinds, settleMs: PagedGrid.settleMs)))
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: TVLayout.rowGap) {
                VStack(alignment: .leading, spacing: KivoTokens.spacing * 4) {
                    Text(list.name)
                        .kivoStyle(KivoTokens.TextSize.xl2, weight: KivoTokens.Weight.bold)
                        .foregroundStyle(palette.text)
                    if let total = paged.total {
                        Text(total == 1 ? String(localized: "1 title") : String(localized: "\(total) titles"))
                            .kivoStyle(KivoTokens.TextSize.sm)
                            .foregroundStyle(palette.textMuted)
                    }
                }
                if let total = paged.total {
                    if total == 0 {
                        MessageView(String(localized: "Nothing in this list yet"))
                            .frame(maxWidth: .infinity)
                    } else {
                        LazyVGrid(columns: Array(repeating: GridItem(.fixed(TVLayout.rowWidth(.poster)), spacing: TVLayout.cardGap, alignment: .top), count: 6), alignment: .leading, spacing: TVLayout.cardGap) {
                            ForEach(0..<total, id: \.self) { index in
                                ListCell(page: paged.page(containing: index), index: index)
                                    .id(index)
                            }
                        }
                        .scrollTargetLayout()
                        .focusSection()
                    }
                } else if let failed = paged.failed {
                    MessageView(String(localized: "Couldn't load this list."), sentence: failed.sentence)
                        .frame(maxWidth: .infinity)
                }
            }
        }
        .scrollClipDisabled()
        .onScrollTargetVisibilityChange(idType: Int.self) { indices in
            guard let first = indices.min(), let last = indices.max() else { return }
            paged.show(first...last)
        }
        .environment(\.currentList, list)
        .task { await paged.run() }
    }
}

/// A list's card in its own work's shape: an episode by its series' poster.
private struct ListCell: View {
    let page: PagedGrid.Page
    let index: Int

    var body: some View {
        if case let .work(item) = onEnum(of: page.item(at: index)) {
            WorkLockup(work: item.work, style: CardStyle.listed(item.work), width: TVLayout.rowWidth(.poster), caption: .oneLine)
        } else {
            SkeletonCard(style: .poster, width: TVLayout.rowWidth(.poster), caption: .oneLine)
        }
    }
}
