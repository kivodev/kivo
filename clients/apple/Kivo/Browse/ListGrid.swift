import KivoCore
import KivoUI
import Observation
import SwiftUI

/// A list's works in a grid, each card in its own work's shape, over a
/// paged grid (``PagedGrid``): the pages around the cards on screen are
/// read once the grid holds still, and read again in place as the list changes. A manual list has
/// its count and its sort at the top, and Reorder in its menu, which shows
/// its works as rows to drag until Done. A list other than My List can be
/// renamed and deleted from its menu. A smart list also has its count and a
/// line of its rules at the top and Edit, which opens the builder on its
/// query.
struct ListGrid: View {
    private let list: ListRef
    @State private var model: PagedGrid
    @State private var renaming = false
    @State private var newName = ""
    @State private var confirmingDelete = false
    @State private var problem: String?
    /// Whether the rename prompt comes back, with what was typed, once a taken name is said.
    @State private var renamingAgain = false
    @State private var editingRules = false
    /// A smart list's rules in a line, once its fields and values are read.
    @State private var rulesLine: String?
    /// The works as rows to drag, while reordering.
    @State private var order: ListOrderModel?
    /// The sort just chosen, until the list is read with it.
    @State private var chosenSort: GridModel.Sort<ListSort>?
    @Environment(ServerModel.self) private var server
    @Environment(\.wide) private var wide
    @Environment(\.kivoPalette) private var palette
    @Environment(\.dismiss) private var dismiss

    init(list: ListRef, session: KivoSession) {
        self.list = list
        let grid = session.library.pagedListWorks(listId: list.id, kinds: nil, settleMs: PagedGrid.settleMs)
        _model = State(initialValue: PagedGrid(grid))
    }

    /// The list as last read: its name changes, and a heading on Home only knows its first.
    private var current: WorkList? { server.lists.first { $0.id == list.id } }
    private var title: String { current?.name ?? list.name }
    private var smart: WorkListSmart? { current?.smart }
    /// Why the first page couldn't be read, when the server refused it.
    private var failure: RequestProblem? { model.failed.flatMap { $0.isUnreachable ? nil : $0 } }

    /// The sorts of a manual list.
    static let sorts = [
        GridModel.Sort(value: ListSort.custom, label: String(localized: "Custom order"), direction: ""),
        GridModel.Sort(value: ListSort.title, label: String(localized: "Title"), direction: String(localized: "A to Z")),
        GridModel.Sort(value: ListSort.releaseDate, label: String(localized: "Release date"), direction: String(localized: "Newest first")),
        GridModel.Sort(value: ListSort.rating, label: String(localized: "Rating"), direction: String(localized: "Highest first")),
        GridModel.Sort(value: ListSort.dateAdded, label: String(localized: "Date added to list"), direction: String(localized: "Newest first")),
    ]

    /// A manual list's sort as chosen or as last read, nil for a smart list.
    private var sort: GridModel.Sort<ListSort>? {
        guard let current, let saved = ListSorts.shared.of(list: current) else { return nil }
        return chosenSort ?? Self.sorts.first { $0.value == saved }
    }

    var body: some View {
        Group {
            if let order {
                ListOrder(model: order, title: title)
            } else {
                grid
            }
        }
        .navigationTitle(title)
        // A list just made isn't read yet, and the mode can't grow once the page is up.
        .toolbarTitleDisplayMode(current != nil && smart == nil ? .inline : .large)
        .toolbar {
            if order != nil {
                ToolbarItem(placement: .topBarTrailing) {
                    Button(String(localized: "Done")) { finishReordering() }
                        .fontWeight(.semibold)
                        .accessibilityIdentifier("list-order-done")
                }
            } else {
                menus
            }
        }
        .listNameAlert(String(localized: "Rename List"), isPresented: $renaming, name: $newName, confirm: String(localized: "Rename"), save: rename)
        .alert(String(localized: "Delete \(title)?"), isPresented: $confirmingDelete) {
            Button(String(localized: "Delete"), role: .destructive) { delete() }
            Button(String(localized: "Cancel"), role: .cancel) {}
        } message: {
            Text("The titles stay in your libraries.")
        }
        .alert(problem ?? "", isPresented: Binding(get: { problem != nil }, set: { if !$0 { problem = nil } })) {
            Button(String(localized: "OK"), role: .cancel) {
                if renamingAgain {
                    renamingAgain = false
                    renaming = true
                }
            }
        }
        .sheet(isPresented: $editingRules) {
            if let current { SmartListBuilder(editing: current) }
        }
        .task { await model.run() }
        .task(id: smart?.query.description) { await readRules() }
        .onChange(of: current?.sort) { chosenSort = nil }
    }

    private var grid: some View {
        GeometryReader { geometry in
            let layout = GridLayout(width: geometry.size.width, style: .poster, scrubber: false, wide: wide)
            ScrollView {
                if let line = smartLine {
                    HStack(alignment: .firstTextBaseline, spacing: KivoTokens.spacing * 1.5) {
                        Image(systemName: "sparkles")
                            .foregroundStyle(palette.accent)
                            .accessibilityHidden(true)
                        Text(line)
                            .foregroundStyle(palette.textMuted)
                            .accessibilityIdentifier("smart-list-summary")
                    }
                    .font(.subheadline)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, layout.insets.leading)
                }
                SkeletonGate(loaded: model.total != nil || failure != nil || server.unreachable) {
                    if let failure {
                        MessageView(String(localized: "Couldn't load \(title)."), sentence: failure.sentence)
                            .frame(maxWidth: .infinity)
                            .padding(.top, KivoTokens.spacing * 10)
                    } else if let total = model.total {
                        if total == 0 {
                            emptyMessage
                            .frame(maxWidth: .infinity)
                            .padding(.top, KivoTokens.spacing * 10)
                        } else {
                            VStack(alignment: .leading, spacing: 0) {
                                if let sort {
                                    countLine(total, sort: sort)
                                        .padding(.bottom, KivoTokens.spacing)
                                }
                                LazyVGrid(columns: layout.columns, alignment: .leading, spacing: layout.rowSpacing) {
                                    ForEach(0..<total, id: \.self) { index in
                                        ListCell(page: model.page(containing: index), index: index, width: layout.cardWidth, caption: layout.caption)
                                            .onAppear { model.show(index...index) }
                                    }
                                }
                                .padding(layout.insets)
                                .environment(\.currentList, ListRef(id: list.id, name: title, kind: list.kind))
                            }
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
            .refreshable { await model.refresh() }
        }
    }

    /// "12 titles · Custom order", the sort opening the sort menu.
    private func countLine(_ total: Int, sort: GridModel.Sort<ListSort>) -> some View {
        CountAndSort(
            count: total == 1 ? String(localized: "1 title") : String(localized: "\(total.formatted()) titles"),
            sorts: Self.sorts,
            selection: Binding(get: { sort }, set: choose)
        )
        .frame(minHeight: 32)
        .font(.footnote)
        .foregroundStyle(palette.textMuted)
        .tint(palette.textMuted)
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, KivoTokens.spacing * 4)
    }

    /// Edit for a smart list, then the menu: Reorder for a manual list, Rename and Delete for one the user made.
    @ToolbarContentBuilder private var menus: some ToolbarContent {
        if let current, current.isSmart {
            ToolbarItem(placement: .topBarTrailing) {
                Button(String(localized: "Edit")) { editingRules = true }
                    .accessibilityIdentifier("smart-list-edit")
            }
        }
        if let current, !current.builtIn || !current.isSmart {
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    if !current.isSmart {
                        Button {
                            order = ListOrderModel(listId: list.id, session: server.session)
                        } label: {
                            Label(String(localized: "Reorder"), systemImage: "arrow.up.arrow.down")
                        }
                        .disabled((model.total ?? 0) < 2)
                    }
                    if !current.builtIn {
                        Button {
                            newName = current.name
                            renaming = true
                        } label: {
                            Label(String(localized: "Rename"), systemImage: "pencil")
                        }
                        Button(role: .destructive) {
                            confirmingDelete = true
                        } label: {
                            Label(String(localized: "Delete"), systemImage: "trash")
                        }
                    }
                } label: {
                    Label(String(localized: "More"), systemImage: "ellipsis")
                }
                .accessibilityIdentifier("list-menu")
            }
        }
    }

    private func choose(_ sort: GridModel.Sort<ListSort>) {
        chosenSort = sort
        Task {
            guard let failed = try? await server.session.library.setListSort(id: list.id, sort: sort.value).problemOrNull() else { return }
            chosenSort = nil
            problem = failed.sentence
        }
    }

    /// Leaves the rows for the grid, read again once the moves are sent.
    private func finishReordering() {
        guard let done = order else { return }
        order = nil
        Task {
            await done.settle()
            await model.refresh()
        }
    }

    /// A smart list's count and rules: "42 movies. Comedy, no Animation".
    private var smartLine: String? {
        guard let smart, let total = model.total else { return nil }
        let count = SmartListWords.count(total, kinds: smart.query.kinds)
        guard let rulesLine, !rulesLine.isEmpty else { return count }
        return "\(count). \(rulesLine)"
    }

    /// Reads what naming a smart list's rules takes.
    private func readRules() async {
        guard let query = smart?.query else { return }
        do {
            let fields = try await server.session.library.workFields()
            let names = try await server.session.library.smartListNames(query: query, fields: fields)
            rulesLine = SmartListWords.summary(of: query, fields: fields, names: names)
        } catch {
            guard !Task.isCancelled else { return }
            problem = error.problemSentence
        }
    }

    /// Why a list shows nothing: rules that no longer run, rules matching nothing, or nothing added.
    @ViewBuilder private var emptyMessage: some View {
        if let smart {
            if smart.errors?.isEmpty == false {
                MessageView(String(localized: "These rules no longer work."))
            } else {
                MessageView(String(localized: "Nothing matches these rules yet."))
            }
        } else {
            MessageView(String(localized: "Nothing in this list yet."))
        }
    }

    private func rename(_ name: String) {
        Task {
            guard let failed = try? await server.session.library.updateList(id: list.id, name: name).problemOrNull() else { return }
            renamingAgain = failed.knownCode == .nameTaken
            problem = failed.listNameSentence
        }
    }

    private func delete() {
        Task {
            guard let done = try? await server.session.library.deleteList(id: list.id) else { return }
            if let failed = done.problemOrNull() {
                problem = failed.sentence
            } else {
                dismiss()
            }
        }
    }

    /// A screenful of posters in the grid's columns.
    private func skeleton(_ layout: GridLayout, height: CGFloat) -> some View {
        let rows = Int((height / (layout.cardWidth / CardStyle.poster.aspect + layout.rowSpacing)).rounded(.up))
        return LazyVGrid(columns: layout.columns, alignment: .leading, spacing: layout.rowSpacing) {
            ForEach(0..<(layout.columns.count * max(rows, 1)), id: \.self) { _ in
                SkeletonCard(style: .poster, width: layout.cardWidth)
            }
        }
        .padding(layout.insets)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text("Loading \(title)…"))
    }
}

extension ListSort: SortMenuKey {
    var short: String? {
        switch self {
        case .title: String(localized: "A–Z")
        case .dateAdded: String(localized: "Recently added")
        case .releaseDate: String(localized: "Newest")
        case .rating: String(localized: "Top rated")
        case .custom, .unknown: nil
        }
    }
}

/// A list's card, or its skeleton until its page arrives. It reads its own
/// page alone, so a page arriving updates only that page's cards.
private struct ListCell: View {
    let page: PagedGrid.Page
    let index: Int
    let width: CGFloat
    let caption: CardCaption

    var body: some View {
        if case .work(let card) = onEnum(of: page.item(at: index)) {
            WorkLink(work: card.work) {
                WorkCard(card.work, style: CardStyle.listed(card.work), width: width, caption: caption)
            }
        } else {
            SkeletonCard(style: .poster, width: width)
        }
    }
}
