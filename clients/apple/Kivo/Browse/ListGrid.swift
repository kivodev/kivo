import KivoCore
import KivoUI
import Observation
import SwiftUI

/// A list's works in a grid, most recently added first, each card in its
/// own work's shape, over the core's paged grid: the pages around the cards
/// on screen are read as they appear, and read again in place as the list
/// changes. A list other than My List can be renamed and deleted from its
/// menu.
struct ListGrid: View {
    private let list: ListRef
    @State private var model: ListGridModel
    @State private var renaming = false
    @State private var newName = ""
    @State private var confirmingDelete = false
    @State private var problem: String?
    @Environment(ServerModel.self) private var server
    @Environment(\.wide) private var wide
    @Environment(\.dismiss) private var dismiss

    init(list: ListRef, session: KivoSession) {
        self.list = list
        _model = State(initialValue: ListGridModel(listId: list.id, session: session))
    }

    /// The list as last read: its name changes, and a heading on Home only knows its first.
    private var current: WorkList? { server.lists.first { $0.id == list.id } }
    private var title: String { current?.name ?? list.name }

    var body: some View {
        GeometryReader { geometry in
            let layout = GridLayout(width: geometry.size.width, style: .poster, scrubber: false, wide: wide)
            ScrollView {
                SkeletonGate(loaded: model.total != nil || model.failure != nil || server.unreachable) {
                    if let failure = model.failure {
                        MessageView(String(localized: "Couldn't load \(title)."), sentence: failure.sentence)
                            .frame(maxWidth: .infinity)
                            .padding(.top, KivoTokens.spacing * 10)
                    } else if let total = model.total {
                        if total == 0 {
                            MessageView(
                                String(localized: "No titles yet."),
                                sentence: String(localized: "Titles you add to this list appear here.")
                            )
                            .frame(maxWidth: .infinity)
                            .padding(.top, KivoTokens.spacing * 10)
                        } else {
                            LazyVGrid(columns: layout.columns, alignment: .leading, spacing: layout.rowSpacing) {
                                ForEach(0..<total, id: \.self) { index in
                                    Group {
                                        if let work = model.work(at: index) {
                                            WorkLink(work: work) {
                                                WorkCard(work, width: layout.cardWidth, caption: layout.caption)
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
        .toolbarTitleDisplayMode(.inline)
        .toolbar {
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
        .listNameAlert(String(localized: "Rename List"), isPresented: $renaming, name: $newName, confirm: String(localized: "Save"), save: rename)
        .alert(String(localized: "Delete \(title)?"), isPresented: $confirmingDelete) {
            Button(String(localized: "Delete"), role: .destructive) { delete() }
            Button(String(localized: "Cancel"), role: .cancel) {}
        }
        .alert(problem ?? "", isPresented: Binding(get: { problem != nil }, set: { if !$0 { problem = nil } })) {
            Button(String(localized: "OK"), role: .cancel) {}
        }
        .task { await model.run() }
    }

    private func rename(_ name: String) {
        Task {
            do {
                _ = try await server.session.library.renameList(id: list.id, name: name)
            } catch {
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
        pager = session.library.pagedListWorks(listId: listId)
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
