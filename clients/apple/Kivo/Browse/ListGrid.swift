import KivoCore
import KivoUI
import Observation
import SwiftUI

/// A list's works in a grid, most recently added first, each card in its
/// own work's shape, over the core's paged grid: the pages around the cards
/// on screen are read as they appear, and read again in place as the list
/// changes. A list other than My List can be renamed and deleted from its
/// menu. A smart list also has its count and a line of its rules at the top
/// and Edit, which opens the builder on its query.
struct ListGrid: View {
    private let list: ListRef
    @State private var model: ListGridModel
    @State private var renaming = false
    @State private var newName = ""
    @State private var confirmingDelete = false
    @State private var problem: String?
    /// Whether the rename prompt comes back, with what was typed, once a taken name is said.
    @State private var renamingAgain = false
    @State private var editingRules = false
    /// A smart list's rules in a line, once its fields and values are read.
    @State private var rulesLine: String?
    @Environment(ServerModel.self) private var server
    @Environment(\.wide) private var wide
    @Environment(\.kivoPalette) private var palette
    @Environment(\.dismiss) private var dismiss

    init(list: ListRef, session: KivoSession) {
        self.list = list
        _model = State(initialValue: ListGridModel(listId: list.id, session: session))
    }

    /// The list as last read: its name changes, and a heading on Home only knows its first.
    private var current: WorkList? { server.lists.first { $0.id == list.id } }
    private var title: String { current?.name ?? list.name }
    private var smart: WorkListSmart? { current?.smart }

    var body: some View {
        GeometryReader { geometry in
            let layout = GridLayout(width: geometry.size.width, style: .poster, scrubber: false, wide: wide)
            ScrollView {
                if let line = smartLine {
                    Text(line)
                        .font(.subheadline)
                        .foregroundStyle(palette.textMuted)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(.horizontal, layout.insets.leading)
                        .accessibilityIdentifier("smart-list-summary")
                }
                SkeletonGate(loaded: model.total != nil || model.failure != nil || server.unreachable) {
                    if let failure = model.failure {
                        MessageView(String(localized: "Couldn't load \(title)."), sentence: failure.sentence)
                            .frame(maxWidth: .infinity)
                            .padding(.top, KivoTokens.spacing * 10)
                    } else if let total = model.total {
                        if total == 0 {
                            emptyMessage
                            .frame(maxWidth: .infinity)
                            .padding(.top, KivoTokens.spacing * 10)
                        } else {
                            LazyVGrid(columns: layout.columns, alignment: .leading, spacing: layout.rowSpacing) {
                                ForEach(0..<total, id: \.self) { index in
                                    Group {
                                        if let work = model.work(at: index) {
                                            WorkLink(work: work) {
                                                WorkCard(work, style: CardStyle.listed(work), width: layout.cardWidth, caption: layout.caption)
                                            }
                                        } else {
                                            SkeletonCard(style: .poster, width: layout.cardWidth)
                                        }
                                    }
                                    .onAppear { model.show(index) }
                                }
                            }
                            .padding(layout.insets)
                            .environment(\.currentList, ListRef(id: list.id, name: title))
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
        .navigationTitle(title)
        // A list just made isn't read yet, and the mode can't grow once the page is up.
        .toolbarTitleDisplayMode(current != nil && smart == nil ? .inline : .large)
        .toolbar {
            if let current, current.isSmart {
                ToolbarItem(placement: .topBarTrailing) {
                    Button(String(localized: "Edit")) { editingRules = true }
                        .accessibilityIdentifier("smart-list-edit")
                }
            }
            if let current, !current.builtIn {
                ToolbarItem(placement: .topBarTrailing) {
                    Menu {
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
                    } label: {
                        Label(String(localized: "More"), systemImage: "ellipsis")
                    }
                    .accessibilityIdentifier("list-menu")
                }
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
    }

    /// A smart list's count and rules: "42 movies. Comedy, no Animation".
    private var smartLine: String? {
        guard let smart, let current else { return nil }
        let count = SmartListWords.count(model.total ?? Int(current.workCount), kinds: smart.query.kinds)
        guard let rulesLine, !rulesLine.isEmpty else { return count }
        return "\(count). \(rulesLine)"
    }

    /// Reads what naming a smart list's rules takes.
    private func readRules() async {
        guard let query = smart?.query, let fields = try? await server.session.library.workFields() else { return }
        let names = await SmartListWords.names(of: query, fields: fields, session: server.session)
        rulesLine = SmartListWords.summary(of: query, fields: fields, names: names)
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
            do {
                _ = try await server.session.library.updateList(id: list.id, name: name, query: nil)
            } catch {
                renamingAgain = error.problemCode == "nameTaken"
                problem = error.listNameSentence
            }
        }
    }

    private func delete() {
        Task {
            do {
                try await server.session.library.deleteList(id: list.id)
                dismiss()
            } catch {
                problem = error.problemSentence
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

/// A list's works as the core's paged grid (`GridPager`) has read them.
@MainActor
@Observable
final class ListGridModel {
    /// The grid as read so far.
    private(set) var pages: GridPages<WorkSummary>
    @ObservationIgnored private let pager: GridPager<WorkSummary>

    init(listId: String, session: KivoSession) {
        pager = session.library.pagedListWorks(listId: listId, kinds: nil)
        pages = pager.state.value
    }

    deinit {
        pager.close()
    }

    /// How many works there are, nil until the first page answers.
    var total: Int? { pages.total.map { Int(truncating: $0) } }

    /// Why the first page couldn't be read, when the server refused it.
    var failure: RequestProblem? {
        pages.problem.flatMap { $0.isUnreachable ? nil : $0 }
    }

    /// The work at [index], nil while its page isn't read.
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
