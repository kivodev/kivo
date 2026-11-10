import KivoCore
import KivoUI
import SwiftUI

/// A library: its name as a large title (``libraryTitle(_:)``), chips for its sections, the filters that are on as chips that
/// turn them off, the count with the sort, and the grid, with the letter
/// scrubber on the title sort and on the sections listed by name. Sort and
/// Filter are in the top bar. The title and chips show from the first
/// frame, with the grid's first page when it was seen lately, else the
/// grid's skeleton until it arrives.
struct LibraryView: View {
    @State private var model: GridModel
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette
    @Environment(\.artworkSource) private var artworkSource
    @Environment(\.displayScale) private var displayScale
    @Environment(\.wide) private var wide
    /// The letter of the top card on screen, for the scrubber, set only when it changes.
    @State private var topLetter: String?
    /// The letter last scrubbed to, marked until the grid is scrolled by hand:
    /// the row above it can still show under the top bar.
    @State private var jumped: String?
    @State private var prefetcher = CardPrefetcher()
    @State private var savingAsSmartList = false

    /// Opens [library] as this device last left it, or with its works in [sort] for once.
    init(library: Library, server: ServerModel, sort: String? = nil) {
        _model = State(initialValue: GridModel(library: library, session: server.session, sort: sort))
    }

    /// Whether the grid showing is one the scrubber goes with: the works by
    /// title, or the Collections, Authors or Series tab, which are by name.
    private var takesScrubber: Bool {
        switch model.shown.tab {
        case .titles: model.shown.sort.value == "title"
        case .collections, .authors, .bookSeries: true
        }
    }

    private var showsScrubber: Bool {
        takesScrubber && model.letters.count > 1
    }

    var body: some View {
        GeometryReader { geometry in
            let stacked = model.shown.tab == .bookSeries
            let layout = GridLayout(width: geometry.size.width, style: model.style, scrubber: showsScrubber, stacked: stacked, wide: wide)
            ScrollViewReader { scroller in
                ScrollView {
                    VStack(alignment: .leading, spacing: KivoTokens.spacing * 2.5) {
                        header
                        // The skeleton shows at once, so a library chosen has its shape on the next frame.
                        ZStack(alignment: .top) {
                            if model.total != nil || model.failed != nil {
                                VStack(alignment: .leading, spacing: 0) {
                                    countLine
                                        .padding(.horizontal, KivoTokens.spacing * 4)
                                        .padding(.bottom, KivoTokens.spacing)
                                    grid(layout)
                                }
                                .onAppear { SwitchSignposts.cardsShown() }
                            } else {
                                skeleton(
                                    GridLayout(width: geometry.size.width, style: model.style, scrubber: takesScrubber, stacked: stacked, wide: wide),
                                    height: geometry.size.height
                                )
                                .onAppear { SwitchSignposts.gridShown() }
                            }
                        }
                        .fades(with: model.total != nil || model.failed != nil)
                    }
                    .id("top")
                    .padding(.top, KivoTokens.spacing)
                    .padding(.bottom, KivoTokens.spacing * 4)
                }
                .refreshable { await model.refresh() }
                // A new tab, sort or filter's grid starts from the top.
                .onChange(of: model.generation) {
                    prefetcher.stop()
                    scroller.scrollTo("top", anchor: .top)
                }
                .onScrollPhaseChange { _, phase in
                    if phase == .interacting { jumped = nil }
                }
                // Called as rows come and go, not on every frame or for every card.
                .onScrollTargetVisibilityChange(idType: Int.self) { indices in
                    shown(indices, layout: layout)
                }
                .overlay(alignment: .trailing) {
                    ZStack {
                        if showsScrubber {
                            LetterScrubber(letters: model.letters, current: jumped ?? topLetter) { start in
                                jumped = start.letter
                                withAnimation(nil) {
                                    if start.offset == 0 {
                                        scroller.scrollTo("top", anchor: .top)
                                    } else {
                                        scroller.scrollTo(Int(start.offset), anchor: .top)
                                    }
                                }
                            }
                        }
                    }
                    .fades(with: showsScrubber)
                    .ignoresSafeArea(.container, edges: .top)
                }
            }
        }
        .libraryTitle(model.library.name)
        .toolbar { toolbar }
        .task { await model.run() }
        .onChange(of: model.query) { model.reload() }
        .environment(\.currentLibraryId, model.library.id)
        .sheet(isPresented: $savingAsSmartList) {
            SmartListBuilder(name: model.library.name, query: model.smartListQuery)
        }
    }

    // MARK: Header

    /// The section chips and the filters that are on, under the title. They
    /// come from what the device knows of the library, so they're there
    /// from the first frame, before the grid.
    @ViewBuilder private var header: some View {
        if model.sectionsKnown, model.sections.count > 1 {
            ChipBar(model.sections.map { Chip(id: $0.id, label: $0.label, selected: $0.tab == model.query.tab) }) { chip in
                model.query.tab = model.sections.first { $0.id == chip.id }?.tab ?? .titles
            }
        }
        let filters = model.activeFilters
        if !filters.isEmpty {
            ActiveFilterBar(filters: filters) { filter in filter.remove(&model.query) }
        }
    }

    // MARK: Count

    /// "1,996 movies · A–Z", so the count and sort are always in view; the
    /// sort opens the sort menu.
    private var countLine: some View {
        HStack(spacing: KivoTokens.spacing * 1.5) {
            if let total = model.total {
                Text(model.noun(total))
                if model.query.tab == .titles {
                    Text("·")
                    sortMenu
                }
            }
        }
        // As tall with the sort menu as without, before the count arrives or on a tab.
        .frame(minHeight: 32)
        .font(.footnote)
        .foregroundStyle(palette.textMuted)
        .tint(palette.textMuted)
    }

    /// The plural the empty state names: "movies", "shows".
    private var plural: String {
        switch model.library.kind {
        case "movies": String(localized: "movies")
        case "shows": String(localized: "shows")
        case "audiobooks": String(localized: "books")
        default: String(localized: "videos")
        }
    }

    // MARK: Grid

    @ViewBuilder private func grid(_ layout: GridLayout) -> some View {
        if let total = model.total {
            if total == 0 {
                emptyMessage
                    .frame(maxWidth: .infinity)
                    .padding(.top, KivoTokens.spacing * 10)
            } else {
                LazyVGrid(columns: layout.columns, alignment: .leading, spacing: layout.rowSpacing) {
                    ForEach(0..<total, id: \.self) { index in
                        GridCell(page: model.page(containing: index), index: index, style: model.style, width: layout.cardWidth, caption: layout.caption)
                            .id(index)
                    }
                }
                .scrollTargetLayout()
                .padding(layout.insets)
                .id(model.generation)
                .fades(with: model.generation)
            }
        } else if model.failed?.isUnreachable == true {
            UnreachableView { Task { await model.refresh() } }
                .padding(.top, KivoTokens.spacing * 10)
        } else if let failed = model.failed {
            MessageView(String(localized: "Couldn't load this library."), sentence: failed.sentence)
                .frame(maxWidth: .infinity)
        }
    }

    /// The grid's skeleton, in its layout: the chips while it isn't known
    /// whether the library has collections, the count line, and a screenful
    /// of cards in the grid's columns.
    private func skeleton(_ layout: GridLayout, height: CGFloat) -> some View {
        let rows = Int((height / (layout.cardWidth / model.style.aspect + layout.rowSpacing)).rounded(.up))
        return VStack(alignment: .leading, spacing: 0) {
            if !model.sectionsKnown {
                SkeletonChipBar(labels: model.sections.map(\.label) + [String(localized: "Collections")])
                    .padding(.bottom, KivoTokens.spacing * 2.5)
            }
            ZStack(alignment: .leading) {
                countLine
                Text(verbatim: " ").font(.footnote).skeletonLine(width: 110)
            }
            .padding(.horizontal, KivoTokens.spacing * 4)
                .padding(.bottom, KivoTokens.spacing)
            LazyVGrid(columns: layout.columns, alignment: .leading, spacing: layout.rowSpacing) {
                ForEach(0..<(layout.columns.count * max(rows, 1)), id: \.self) { _ in
                    SkeletonCard(style: model.style, width: layout.cardWidth, caption: layout.caption, person: model.shown.tab == .authors)
                }
            }
            .padding(layout.insets)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text("Loading \(model.library.name)…"))
    }

    /// "No unwatched movies" with Clear Filters, or nothing in the section at all.
    @ViewBuilder private var emptyMessage: some View {
        let filters = model.activeFilters
        if !filters.isEmpty {
            let clear: () -> Void = { model.clearFilters() }
            let title = filters.count == 1 && filters[0].id == "played"
                ? String(localized: "No \(filters[0].label.lowercased()) \(plural)")
                : String(localized: "No \(plural) match these filters")
            MessageView(title, action: (label: String(localized: "Clear filters"), run: clear))
        } else {
            MessageView(String(localized: "Nothing here yet."))
        }
    }

    /// The cards at [indices] are on screen: their pages load, the scrubber
    /// marks the top one's letter, and the artwork ahead is fetched.
    private func shown(_ indices: [Int], layout: GridLayout) {
        guard let first = indices.min(), let last = indices.max(), let total = model.total else { return }
        model.show(first...last)
        let letter = model.letter(at: first)
        if letter != topLetter { topLetter = letter }
        let cards = CardPrefetcher.Cards(pixelWidth: layout.cardWidth * displayScale, source: artworkSource)
        let style = model.style
        prefetcher.show(first...last, of: total, cards: cards) { model.item(at: $0)?.artwork(style) }
    }

    // MARK: Sort and filter

    /// Cast, then filter, in one group; an audiobooks library has no Cast.
    @ToolbarContentBuilder private var toolbar: some ToolbarContent {
        let cast = server.cast.buttonShown && model.library.kind != "audiobooks"
        if cast || model.query.tab == .titles {
            ToolbarItemGroup(placement: .topBarTrailing) {
                if cast {
                    CastButton(cast: server.cast)
                        .tint(palette.text)
                }
                if model.query.tab == .titles {
                    // FilterStyleTrial: becomes the chosen style's control.
                    FilterControl(model: model, savingAsSmartList: $savingAsSmartList)
                }
            }
        }
    }

    private var sortMenu: some View {
        Menu {
            Picker(selection: $model.query.sort) {
                ForEach(model.sorts) { sort in
                    Button {} label: {
                        Text(sort.label)
                        Text(sort.direction)
                    }
                    .tag(sort)
                }
            } label: {
                Text("Sort by")
            }
            .pickerStyle(.inline)
        } label: {
            HStack(spacing: 3) {
                Text(model.query.sort.short)
                Image(systemName: "chevron.up.chevron.down").font(.caption2.weight(.semibold))
            }
            .frame(minHeight: 32)
            .contentShape(Rectangle())
        }
        .accessibilityIdentifier("count-sort")
    }
}

/// A card of the grid, or its skeleton until its page arrives, when the card
/// fades in. It reads its own page alone, so a page arriving updates only
/// that page's cards.
private struct GridCell: View {
    let page: GridModel.Page
    let index: Int
    let style: CardStyle
    let width: CGFloat
    let caption: CardCaption
    @Environment(\.openWork) private var openWork

    var body: some View {
        let item = page.item(at: index)
        ZStack {
            switch item {
            case .work(let work):
                WorkLink(work: work) {
                    WorkCard(work, style: style, width: width, caption: caption)
                }
            case .collection(let collection):
                link(WorkRoute(id: collection.id, page: .collection)) {
                    GroupCard(collection, width: width, caption: caption)
                }
            case .author(let author):
                link(WorkRoute(id: author.id, page: .author)) {
                    GroupCard(author: author, width: width, caption: caption)
                }
            case .bookSeries(let series):
                link(WorkRoute(id: series.id, page: .bookSeries)) {
                    GroupCard(series, width: width, caption: caption)
                }
            case nil:
                SkeletonCard(style: style, width: width, caption: caption)
            }
        }
        .fades(with: item == nil)
    }

    /// A card that opens [route]'s page.
    private func link(_ route: WorkRoute, @ViewBuilder label: () -> some View) -> some View {
        Button {
            openWork(route)
        } label: {
            label()
        }
        .buttonStyle(.plain)
        .hoverEffect(.highlight)
        .accessibilityIdentifier("work")
    }
}

/// The Filter button's icon, with a dot while any filter is on.
struct FilterLabel: View {
    let on: Bool
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        Image(systemName: "line.3.horizontal.decrease")
            .overlay(alignment: .topTrailing) {
                if on {
                    Circle()
                        .fill(palette.accent)
                        .frame(width: 8, height: 8)
                        .offset(x: 4, y: -3)
                }
            }
            .accessibilityLabel(on ? Text("Filter, on") : Text("Filter"))
    }
}

/// The filters that are on, as small outlined chips with ✕ that turn them
/// off: secondary to the section chips above them.
private struct ActiveFilterBar: View {
    let filters: [GridModel.ActiveFilter]
    let remove: (GridModel.ActiveFilter) -> Void
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: KivoTokens.spacing * 2) {
                ForEach(filters) { filter in
                    Button {
                        remove(filter)
                    } label: {
                        HStack(spacing: KivoTokens.spacing * 1.5) {
                            Text(filter.label)
                            Image(systemName: "xmark")
                                .font(.caption2.weight(.bold))
                                .foregroundStyle(palette.textMuted)
                        }
                        .font(.footnote.weight(.medium))
                        .foregroundStyle(palette.text)
                        .padding(.horizontal, KivoTokens.spacing * 3)
                        .frame(minHeight: 28)
                        .overlay(Capsule().strokeBorder(palette.border))
                        .padding(.vertical, 8)
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel(Text("Remove filter \(filter.label)"))
                    .accessibilityIdentifier("filter-\(filter.id)")
                }
            }
            .padding(.horizontal, KivoTokens.spacing * 4)
        }
        .scrollClipDisabled()
        .padding(.vertical, -8)
    }
}
