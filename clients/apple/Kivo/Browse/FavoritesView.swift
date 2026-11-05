import KivoCore
import KivoUI
import Observation
import SwiftUI

/// Favorites, chosen in the Libraries tab as a library is: under its large
/// title (``libraryTitle(_:)``), a row for each kind of work the user marked, Movies, Shows,
/// Episodes and Books first, each the kind's most recently marked; a row's
/// heading opens all of that kind in a grid. Each kind is read again as the
/// live events say, so a heart changed anywhere shows here.
struct FavoritesView: View {
    @Environment(ServerModel.self) private var server
    @State private var model: FavoritesModel

    private static let rowSpacing = KivoTokens.spacing * 7
    /// The rows the skeleton shows: two rows of posters.
    private static let skeletonStyles: [CardStyle] = [.poster, .poster]

    init(server: ServerModel) {
        _model = State(initialValue: FavoritesModel(session: server.session))
    }

    private var title: String { String(localized: "Favorites") }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: KivoTokens.spacing * 2.5) {
                // The skeleton shows at once, so Favorites chosen has its shape on the next frame.
                ZStack(alignment: .top) {
                    if model.loaded {
                        rows
                            .onAppear { SwitchSignposts.cardsShown() }
                    } else if let failure = model.failure {
                        MessageView(String(localized: "Couldn't load Favorites."), sentence: failure.sentence)
                            .frame(maxWidth: .infinity)
                            .padding(.top, KivoTokens.spacing * 10)
                    } else if server.unreachable {
                        UnreachableView()
                            .padding(.top, KivoTokens.spacing * 10)
                    } else {
                        skeleton
                            .onAppear { SwitchSignposts.gridShown() }
                    }
                }
                .fades(with: model.loaded || model.failure != nil || server.unreachable)
            }
            .padding(.top, KivoTokens.spacing)
            .padding(.bottom, KivoTokens.spacing * 6)
        }
        .refreshable { await model.refresh() }
        .libraryTitle(title)
        .task { await model.run() }
    }

    @ViewBuilder private var rows: some View {
        if model.shown.isEmpty {
            MessageView(
                String(localized: "No favorites yet."),
                sentence: String(localized: "Items you mark with ♥ appear here.")
            )
            .frame(maxWidth: .infinity)
            .padding(.top, KivoTokens.spacing * 10)
        } else {
            LazyVStack(alignment: .leading, spacing: Self.rowSpacing) {
                ForEach(model.shown, id: \.self) { kind in
                    BrowseRow(title: kind.title, items: model.items(of: kind), style: kind.style) {
                        server.paths[server.tab(of: .favorites), default: []].append(.favorites(kind))
                    }
                }
            }
            .padding(.top, KivoTokens.spacing * 2)
        }
    }

    private var skeleton: some View {
        VStack(alignment: .leading, spacing: Self.rowSpacing) {
            ForEach(Array(Self.skeletonStyles.enumerated()), id: \.offset) { _, style in
                BrowseRow.skeleton(style)
            }
        }
        .padding(.top, KivoTokens.spacing * 2)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text("Loading Favorites…"))
    }
}

/// Every kind's first page of favorites, each following its own reads.
@MainActor
@Observable
final class FavoritesModel {
    /// Each kind's first page, once it has arrived.
    private var pages: [FavoriteKind: WorkSummaryPage] = [:]
    /// Why a kind's first page couldn't be read, when the server refused it.
    private(set) var failure: RequestProblem?
    @ObservationIgnored private let session: KivoSession

    init(session: KivoSession) {
        self.session = session
    }

    /// Whether every kind's first page has arrived.
    var loaded: Bool { pages.count == FavoriteKind.rowOrder.count }

    /// The kinds with any favorites, in the order their rows show.
    var shown: [FavoriteKind] {
        FavoriteKind.rowOrder.filter { !(pages[$0]?.items.isEmpty ?? true) }
    }

    func items(of kind: FavoriteKind) -> [WorkSummary] {
        pages[kind]?.items ?? []
    }

    /// Follows every kind's first page until cancelled.
    func run() async {
        await withDiscardingTaskGroup { group in
            for kind in FavoriteKind.rowOrder {
                group.addTask { @MainActor in
                    for await load in self.session.library.favorites(kind: kind.rawValue, limit: nil) {
                        if let page = load.getOrNull() {
                            self.show(page, of: kind)
                        } else if let problem = load.problemOrNull(), !problem.isUnreachable {
                            self.failure = problem
                        }
                    }
                }
            }
        }
    }

    /// Reads every kind again now, for a pull to refresh.
    func refresh() async {
        await withDiscardingTaskGroup { group in
            for kind in FavoriteKind.rowOrder {
                group.addTask { @MainActor in
                    for await load in self.session.library.favorites(kind: kind.rawValue, limit: nil) {
                        if let page = load.getOrNull() { self.show(page, of: kind) }
                        break
                    }
                }
            }
        }
    }

    /// Shows [page] only if it changed, so a read that finds nothing new redraws nothing.
    private func show(_ page: WorkSummaryPage, of kind: FavoriteKind) {
        if pages[kind] != page { pages[kind] = page }
    }
}

/// All of one kind's favorites in a grid, most recently marked first,
/// over the core's paged grid: the pages around the cards on screen are
/// read as they appear, and read again in place as the user's marks change.
struct FavoritesGrid: View {
    let kind: FavoriteKind
    @State private var list: FavoriteList
    @Environment(ServerModel.self) private var server
    @Environment(\.wide) private var wide

    init(kind: FavoriteKind, session: KivoSession) {
        self.kind = kind
        _list = State(initialValue: FavoriteList(kind: kind, session: session))
    }

    var body: some View {
        GeometryReader { geometry in
            let layout = GridLayout(width: geometry.size.width, style: kind.style, scrubber: false, wide: wide)
            ScrollView {
                SkeletonGate(loaded: list.total != nil || list.failure != nil || server.unreachable) {
                    if let failure = list.failure {
                        MessageView(String(localized: "Couldn't load Favorites."), sentence: failure.sentence)
                            .frame(maxWidth: .infinity)
                            .padding(.top, KivoTokens.spacing * 10)
                    } else if let total = list.total {
                        if total == 0 {
                            MessageView(
                                String(localized: "No favorites yet."),
                                sentence: String(localized: "Items you mark with ♥ appear here.")
                            )
                            .frame(maxWidth: .infinity)
                            .padding(.top, KivoTokens.spacing * 10)
                        } else {
                            LazyVGrid(columns: layout.columns, alignment: .leading, spacing: layout.rowSpacing) {
                                ForEach(0..<total, id: \.self) { index in
                                    Group {
                                        if let work = list.work(at: index) {
                                            WorkLink(work: work) {
                                                WorkCard(work, style: kind.style, width: layout.cardWidth)
                                            }
                                        } else {
                                            SkeletonCard(style: kind.style, width: layout.cardWidth)
                                        }
                                    }
                                    .onAppear { list.show(index) }
                                }
                            }
                            .padding(layout.insets)
                        }
                    } else {
                        UnreachableView()
                            .padding(.top, KivoTokens.spacing * 10)
                    }
                } skeleton: {
                    skeleton(layout, height: geometry.size.height)
                }
                .padding(.top, KivoTokens.spacing * 2)
                .padding(.bottom, KivoTokens.spacing * 6)
            }
            .refreshable { await list.refresh() }
        }
        .navigationTitle(kind.title)
        .toolbarTitleDisplayMode(.inline)
        .task { await list.run() }
    }

    /// A screenful of cards in the grid's columns.
    private func skeleton(_ layout: GridLayout, height: CGFloat) -> some View {
        let rows = Int((height / (layout.cardWidth / kind.style.aspect + layout.rowSpacing)).rounded(.up))
        return LazyVGrid(columns: layout.columns, alignment: .leading, spacing: layout.rowSpacing) {
            ForEach(0..<(layout.columns.count * max(rows, 1)), id: \.self) { _ in
                SkeletonCard(style: kind.style, width: layout.cardWidth)
            }
        }
        .padding(layout.insets)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text("Loading \(kind.title)…"))
    }
}

/// One kind's favorites as the core's paged grid (`GridPager`) has read them.
@MainActor
@Observable
final class FavoriteList {
    /// The grid as read so far.
    private(set) var pages: GridPages<WorkSummary>
    @ObservationIgnored private let pager: GridPager<WorkSummary>

    init(kind: FavoriteKind, session: KivoSession) {
        pager = session.library.pagedFavorites(kind: kind.rawValue)
        pages = pager.state.value
    }

    deinit {
        pager.close()
    }

    /// How many favorites there are, nil until the first page answers.
    var total: Int? { pages.total.map { Int(truncating: $0) } }

    /// Why the first page couldn't be read, when the server refused it.
    var failure: RequestProblem? {
        pages.problem.flatMap { $0.isUnreachable ? nil : $0 }
    }

    /// The favorite at [index], nil while its page isn't read.
    func work(at index: Int) -> WorkSummary? {
        pages.item(index: Int32(index))
    }

    /// Follows the grid until cancelled.
    func run() async {
        for await pages in pager.state {
            self.pages = pages
        }
    }

    /// The card at [index] appeared: its page and the next are read.
    func show(_ index: Int) {
        pager.load(first: Int32(index), last: Int32(index) + pages.pageSize)
    }

    /// Reads the grid again in place, for a pull to refresh.
    func refresh() async {
        // SKIE gives a generic class's suspend functions through skie().
        try? await skie(pager).refresh()
    }
}

/// A kind of work Favorites lists apart, its raw value the spec's `WorkKind`.
enum FavoriteKind: String, Hashable {
    case movie
    case series
    case season
    case episode
    case extra
    case video
    case book

    /// The kinds in the order Favorites lists them: what's watched and read most first.
    static let rowOrder: [FavoriteKind] = [.movie, .series, .episode, .book, .season, .video, .extra]

    /// The kind's row heading and grid title.
    var title: String {
        switch self {
        case .movie: String(localized: "Movies")
        case .series: String(localized: "Shows")
        case .season: String(localized: "Seasons")
        case .episode: String(localized: "Episodes")
        case .extra: String(localized: "Extras")
        case .video: String(localized: "Videos")
        case .book: String(localized: "Books")
        }
    }

    /// The kind's card shape: an episode shows its series' poster.
    var style: CardStyle {
        switch self {
        case .movie, .series, .season, .episode: .poster
        case .book: .square
        case .extra, .video: .landscape
        }
    }
}
