import KivoCore
import KivoUI
import SwiftUI

/// A library's grid under one row: its name, unless its tab names it, its
/// sections when it has collections, the count, "237 movies · A–Z", which
/// opens Sort, and Filter at the right, which gets a dot while any filter
/// is on. Then posters, or landscape cards for other videos, focus coming
/// in on the first, and at the right the letters of the title sort, which
/// jump the grid. The grid's pages load where it rests (GridModel),
/// a new section, sort or filter showing from the top at once.
struct LibraryView: View {
    let library: Library
    /// Whether it heads itself with its name: not in its own tab, which the sidebar names.
    var titled = true
    @Environment(ServerModel.self) private var server

    var body: some View {
        LibraryGrid(library: library, titled: titled, server: server)
            .id(library.id)
    }
}

/// A library's grid as this device last left it.
private struct LibraryGrid: View {
    let titled: Bool
    @State private var model: GridModel
    @Environment(\.kivoPalette) private var palette
    @Environment(\.artworkSource) private var artworkSource
    @State private var prefetcher = CardPrefetcher()
    @FocusState private var focus: GridPlace?
    /// Whether a card has had focus since the grid showed; until one has,
    /// focus coming to the header goes on to the first card.
    @State private var entered = false

    init(library: Library, titled: Bool, server: ServerModel) {
        self.titled = titled
        _model = State(initialValue: GridModel(library: library, session: server.session))
    }

    /// Posters and squares across the grid, and landscape cards.
    private var columns: Int { TVLayout.gridColumns(model.style) }

    private var cardWidth: CGFloat { TVLayout.gridWidth(model.style) }

    /// Whether the grid is by name, which the letters go with.
    private var lettered: Bool {
        (model.state.tab != .titles || model.state.sort == .title) && model.paged.letters.count > 1
    }

    var body: some View {
        ScrollViewReader { scroller in
            HStack(alignment: .top, spacing: TVLayout.cardGap) {
                ScrollView {
                    VStack(alignment: .leading, spacing: TVLayout.rowGap) {
                        header
                        content
                    }
                    .id("top")
                }
                .scrollClipDisabled()
                .onScrollTargetVisibilityChange(idType: Int.self) { shown($0) }
                .onChange(of: model.generation) {
                    prefetcher.stop()
                    scroller.scrollTo("top", anchor: .top)
                }
                if lettered {
                    LetterColumn(letters: model.paged.letters) { start in
                        scroller.scrollTo(start.offset == 0 ? AnyHashable("top") : AnyHashable(Int(start.offset)), anchor: .top)
                    }
                }
            }
        }
        .onChange(of: focus) { _, place in
            switch place {
            case .card: entered = true
            case .header where !entered && (model.paged.total ?? 0) > 0:
                // Once this move of focus has settled.
                Task { focus = .card(0) }
            default: break
            }
        }
        .task { await model.run() }
        .environment(\.currentLibraryId, model.library.id)
    }

    // MARK: Header

    private var header: some View {
        HStack(alignment: .center, spacing: KivoTokens.spacing * 6) {
            if titled {
                Text(model.library.name)
                    .kivoStyle(KivoTokens.TextSize.xl2, weight: KivoTokens.Weight.bold)
                    .foregroundStyle(palette.text)
                    .lineLimit(1)
            }
            if model.state.tabsKnown, model.sections.count > 1 {
                HStack(spacing: KivoTokens.spacing * 2) {
                    ForEach(model.sections) { section in
                        Button(section.label) { model.choose(tab: section.tab) }
                            .buttonStyle(TabStyle(current: section.id == model.state.tab.value))
                            .focused($focus, equals: .header)
                    }
                }
            }
            countLine
                .focused($focus, equals: .header)
            Spacer(minLength: 0)
            // Once the grid is in, so that focus doesn't settle here before it can go to the first card.
            if model.state.tab == .titles, model.paged.total != nil {
                LibraryFilterMenu(model: model)
                    .focused($focus, equals: .header)
            }
        }
        .focusSection()
    }

    /// "237 movies · A–Z", which opens the sorts; a section with no sort just says how many.
    @ViewBuilder private var countLine: some View {
        if let total = model.paged.total {
            if model.state.tab == .titles {
                Menu {
                    Picker(String(localized: "Sort by"), selection: Binding(get: { model.state.sort }, set: { model.choose(sort: $0) })) {
                        ForEach(model.sorts) { sort in
                            Button {} label: {
                                Text(sort.label)
                                Text(sort.direction)
                            }
                            .tag(sort.value)
                        }
                    }
                    .pickerStyle(.inline)
                } label: {
                    HStack(spacing: KivoTokens.spacing * 2) {
                        Text("\(model.noun(total)) · \(model.sort.short)")
                        Image(systemName: "chevron.down")
                            .font(.caption.weight(.semibold))
                    }
                    .font(.kivo(KivoTokens.TextSize.sm))
                }
            } else {
                Text(model.noun(total))
                    .kivoStyle(KivoTokens.TextSize.sm)
                    .foregroundStyle(palette.textMuted)
            }
        } else {
            Text(verbatim: " ").kivoStyle(KivoTokens.TextSize.sm).skeletonLine(width: 240)
        }
    }

    // MARK: Grid

    @ViewBuilder private var content: some View {
        let grid = Array(repeating: GridItem(.fixed(cardWidth), spacing: TVLayout.cardGap, alignment: .top), count: columns)
        if let total = model.paged.total {
            if total == 0 {
                emptyMessage
                    .frame(maxWidth: .infinity)
            } else {
                LazyVGrid(columns: grid, alignment: .leading, spacing: TVLayout.lineGap) {
                    ForEach(0..<total, id: \.self) { index in
                        GridCell(page: model.paged.page(containing: index), index: index, style: model.style, width: cardWidth, focus: $focus)
                            .id(index)
                    }
                }
                .scrollTargetLayout()
                // Down from anywhere in the header, Filter too, comes into the grid.
                .focusSection()
                .id(model.generation)
            }
        } else if model.paged.failed?.isUnreachable == true {
            UnreachableView()
        } else if let failed = model.paged.failed {
            MessageView(String(localized: "Couldn't load this library."), sentence: failed.sentence)
                .frame(maxWidth: .infinity)
        } else {
            LazyVGrid(columns: grid, alignment: .leading, spacing: TVLayout.lineGap) {
                ForEach(0..<columns * 2, id: \.self) { _ in
                    SkeletonCard(style: model.style, width: cardWidth)
                }
            }
        }
    }

    /// "No unwatched movies" with Clear filters, or nothing in the section at all.
    @ViewBuilder private var emptyMessage: some View {
        let clear: () -> Void = { model.clearFilters() }
        let plural = model.noun(2).split(separator: " ").dropFirst().joined(separator: " ")
        switch model.state.emptyCase {
        case .watchStatus:
            let played = model.state.filters.played.map { GridModel.emptyName(played: $0, books: false) } ?? ""
            MessageView(String(localized: "No \(played) \(plural)"), action: (label: String(localized: "Clear filters"), run: clear))
        case .filtered:
            MessageView(String(localized: "No \(plural) match these filters"), action: (label: String(localized: "Clear filters"), run: clear))
        case .unfiltered:
            MessageView(String(localized: "Nothing in this library yet."))
        }
    }

    /// The cards at [indices] are on screen: their pages load, and the artwork ahead is fetched.
    private func shown(_ indices: [Int]) {
        guard let first = indices.min(), let last = indices.max(), let total = model.paged.total else { return }
        model.paged.show(first...last)
        let cards = CardPrefetcher.Cards(pixelWidth: cardWidth, source: artworkSource)
        let style = model.style
        prefetcher.show(first...last, of: total, cards: cards) { model.paged.item(at: $0)?.artwork(style) }
    }
}

/// Where focus is on a library's screen: its header, or a card by its place in the grid.
private enum GridPlace: Hashable {
    case header
    case card(Int)
}

/// A card of the grid, or its skeleton until its page arrives. It reads its
/// own page alone, so a page arriving updates only that page's cards.
private struct GridCell: View {
    let page: PagedGrid.Page
    let index: Int
    let style: CardStyle
    let width: CGFloat
    let focus: FocusState<GridPlace?>.Binding
    @Environment(\.openWork) private var openWork

    var body: some View {
        switch onEnum(of: page.item(at: index)) {
        case .work(let item):
            WorkLockup(work: item.work, style: style, width: width)
                .focused(focus, equals: .card(index))
        case .collection(let item):
            Button {
                openWork(WorkRoute(id: item.collection.id, page: .collection))
            } label: {
                GroupCard(item.collection, width: width)
            }
            .buttonStyle(.borderless)
            .focused(focus, equals: .card(index))
        case .author, .bookSeries, nil:
            SkeletonCard(style: style, width: width)
        }
    }
}

/// The letters the grid's titles start with, top to bottom, sharing the
/// screen's height from under Filter down to its bottom margin; Select
/// jumps the grid to one.
private struct LetterColumn: View {
    let letters: [LetterStart]
    let jump: (LetterStart) -> Void

    /// Where the letters start and end, from the screen's top and bottom.
    private static let top: CGFloat = 220
    private static let bottom: CGFloat = 54
    /// The most a letter takes.
    private static let tallest: CGFloat = 34

    var body: some View {
        GeometryReader { geometry in
            let frame = geometry.frame(in: .global)
            let screen = frame.maxY + geometry.safeAreaInsets.bottom
            let above = max(0, Self.top - frame.minY)
            let room = min(frame.maxY, screen - Self.bottom) - frame.minY - above
            let slot = min(Self.tallest, room / CGFloat(max(letters.count, 1)))
            VStack(spacing: 0) {
                ForEach(letters, id: \.letter) { start in
                    Button(start.letter) { jump(start) }
                        .buttonStyle(LetterStyle(height: slot))
                }
            }
            .focusSection()
            .padding(.top, above)
        }
        .frame(width: 84)
    }
}

/// A letter [height] tall: muted, lit in a pill while focused.
private struct LetterStyle: ButtonStyle {
    let height: CGFloat
    @Environment(\.isFocused) private var focused
    @Environment(\.kivoPalette) private var palette

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.kivo(KivoTokens.TextSize.xs, weight: KivoTokens.Weight.semibold))
            .foregroundStyle(focused ? palette.bg : palette.textMuted)
            .frame(width: 70, height: height)
            .background(focused ? palette.text : .clear, in: Capsule())
            .frame(width: 84)
            .scaleEffect(configuration.isPressed ? 0.95 : 1)
            .animation(.easeOut(duration: 0.15), value: focused)
    }
}

/// Filter, at the top right of a library's titles, with a dot while any
/// filter is on: the watch status, then each group the library offers,
/// each naming what's chosen in it, and Clear filters while any is on.
private struct LibraryFilterMenu: View {
    let model: GridModel
    @Environment(\.kivoPalette) private var palette

    /// All, then each watch status.
    private static let statuses: [WorkPlayState?] = [nil, .unplayed, .inProgress, .played]

    private var chosen: GridFilters { model.state.filters }

    var body: some View {
        Menu {
            Picker(selection: one(of: .watchStatus)) {
                ForEach(Self.statuses, id: \.self) { status in
                    Text(model.label(played: status)).tag(status.map { GridFilter.Played(state: $0).key })
                }
            } label: {
                Text("Watch status")
            }
            .pickerStyle(.inline)
            Section {
                let quality = values(of: .quality)
                one(String(localized: "Decade"), .decade)
                several(String(localized: "Genre"), values(of: .genre))
                several(String(localized: "Content rating"), values(of: .contentRating))
                several(String(localized: "Resolution"), quality.filter { $0 is GridFilter.Resolution })
                several(String(localized: "Dynamic range"), quality.filter { $0 is GridFilter.DynamicRange })
            }
            if !chosen.isEmpty {
                Section {
                    Button(String(localized: "Clear filters"), systemImage: "xmark.circle") { model.clearFilters() }
                }
            }
        } label: {
            HStack(spacing: KivoTokens.spacing * 2) {
                Image(systemName: "line.3.horizontal.decrease")
                Text("Filter")
                if !chosen.isEmpty {
                    Circle().fill(palette.accent).frame(width: 12, height: 12)
                }
            }
        }
    }

    /// The values [group] offers; none when the library has none of it.
    private func values(of group: FilterGroupKind) -> [GridFilter] {
        model.state.offer.first { $0.kind == group }?.values ?? []
    }

    /// The one value on in [group], by its key; nil turns the group off.
    private func one(of group: FilterGroupKind) -> Binding<String?> {
        Binding(
            get: { chosen.chosen.first { $0.group == group }?.key },
            set: { key in
                let picked = values(of: group).first { $0.key == key }
                model.choose(picked.map { chosen.choosing(filter: $0) } ?? chosen.clearing(group: group))
            }
        )
    }

    /// A submenu of [group]'s values to take one of, or Any, named with what's chosen.
    @ViewBuilder private func one(_ title: String, _ group: FilterGroupKind) -> some View {
        let values = values(of: group)
        if !values.isEmpty {
            Picker(selection: one(of: group)) {
                Text("Any").tag(String?.none)
                ForEach(values, id: \.key) { Text(model.label($0)).tag(String?.some($0.key)) }
            } label: {
                Text(title)
                if let on = chosen.chosen.first(where: { $0.group == group }) { Text(model.label(on)) }
            }
            .pickerStyle(.menu)
        }
    }

    /// A submenu of [values] to take any of, named with those chosen.
    @ViewBuilder private func several(_ title: String, _ values: [GridFilter]) -> some View {
        if !values.isEmpty {
            let on = values.filter { chosen.has(filter: $0) }.map { model.label($0) }
            Menu {
                ForEach(values, id: \.key) { value in
                    Toggle(model.label(value), isOn: Binding(get: { chosen.has(filter: value) }, set: { _ in model.toggle(value) }))
                        .menuActionDismissBehavior(.disabled)
                }
            } label: {
                Text(title)
                if !on.isEmpty {
                    Text(on.joined(separator: ", "))
                }
            }
        }
    }
}
