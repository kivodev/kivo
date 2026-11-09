import KivoCore
import KivoUI
import SwiftUI

/// The Lists tab: My List with a row of its covers and its count, then the
/// other lists, then the smart lists, each with its first cover and its
/// count; each opens its works in a grid. A section shows once it has any.
/// The + makes a list, or opens the smart list builder. A wide window has
/// My List as a Home row and the others as grids of cards.
struct ListsView: View {
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette
    @Environment(\.wide) private var wide

    @State private var naming = false
    @State private var newName = ""
    @State private var problem: String?
    /// Whether the name prompt comes back, with what was typed, once a taken name is said.
    @State private var namingAgain = false
    @State private var buildingSmart = false

    private static let cardColumns = [GridItem(.adaptive(minimum: 324), spacing: KivoTokens.Layout.gridGap, alignment: .top)]

    var body: some View {
        Group {
            let others = server.lists.filter { !$0.builtIn && !$0.isSmart }
            let smart = server.lists.filter(\.isSmart)
            if let myList = server.myList, myList.workCount == 0, others.isEmpty, smart.isEmpty {
                MessageView(
                    String(localized: "Nothing in your lists yet."),
                    sentence: String(localized: "Add something to My List from a title's page.")
                )
                .frame(maxHeight: .infinity)
            } else if let myList = server.myList {
                ScrollView {
                    VStack(alignment: .leading, spacing: KivoTokens.spacing * 7) {
                        if myList.workCount > 0 {
                            if wide {
                                MyListRow(list: myList)
                            } else {
                                MyListSection(list: myList)
                                    .padding(.horizontal, BrowseRow.inset)
                            }
                        }
                        section(String(localized: "Your Lists"), others)
                        section(String(localized: "Smart Lists"), smart)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.top, KivoTokens.spacing * 2)
                    .padding(.bottom, KivoTokens.spacing * 6)
                }
            } else if let failure = server.listsFailure {
                MessageView(String(localized: "Couldn't load your lists."), sentence: failure.sentence)
                    .frame(maxHeight: .infinity)
            } else if server.unreachable {
                UnreachableView()
            } else {
                LoadingView(String(localized: "Loading lists…"))
            }
        }
        .navigationTitle(String(localized: "Lists"))
        .toolbarTitleDisplayMode(.large)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    Button(String(localized: "New List")) {
                        newName = ""
                        naming = true
                    }
                    Button(String(localized: "New Smart List")) { buildingSmart = true }
                } label: {
                    SwiftUI.Label(String(localized: "New"), systemImage: "plus")
                }
                .accessibilityIdentifier("new-list-menu")
            }
        }
        .listNameAlert(String(localized: "New List"), isPresented: $naming, name: $newName, confirm: String(localized: "Create"), save: create)
        .alert(problem ?? "", isPresented: Binding(get: { problem != nil }, set: { if !$0 { problem = nil } })) {
            Button(String(localized: "OK"), role: .cancel) {
                if namingAgain {
                    namingAgain = false
                    naming = true
                }
            }
        }
        .sheet(isPresented: $buildingSmart) { SmartListBuilder() }
    }

    /// A headed section of [lists], nothing when there are none.
    @ViewBuilder private func section(_ title: String, _ lists: [WorkList]) -> some View {
        if !lists.isEmpty {
            VStack(alignment: .leading, spacing: BrowseRow.headingSpacing) {
                BrowseRow.heading(title)
                    .foregroundStyle(palette.text)
                    .accessibilityAddTraits(.isHeader)
                if wide {
                    LazyVGrid(columns: Self.cardColumns, spacing: KivoTokens.Layout.gridGap) {
                        ForEach(lists, id: \.id) { list in
                            ListRow(list: list)
                                .frame(maxHeight: .infinity)
                                .background(palette.surfaceRaised, in: RoundedRectangle(cornerRadius: KivoTokens.Radius.lg))
                        }
                    }
                } else {
                    LazyVStack(spacing: 0) {
                        ForEach(Array(lists.enumerated()), id: \.element.id) { index, list in
                            if index > 0 {
                                Divider().padding(.leading, BrowseRow.inset)
                            }
                            ListRow(list: list)
                        }
                    }
                    .background(palette.surfaceRaised, in: RoundedRectangle(cornerRadius: KivoTokens.Radius.lg))
                }
            }
            .padding(.horizontal, BrowseRow.inset)
        }
    }

    private func create(_ name: String) {
        Task {
            do {
                _ = try await server.session.library.createList(name: name)
            } catch {
                namingAgain = error.problemCode == "nameTaken"
                problem = error.listNameSentence
            }
        }
    }
}

extension WorkList {
    /// How many titles the list has, as its rows say it.
    var titleCount: String {
        workCount == 1 ? String(localized: "1 title") : String(localized: "\(workCount) titles")
    }

    var isSmart: Bool { kind == "smart" }

    /// A list's line under its name: a smart list's kinds and count, else its count.
    var detail: String {
        if let query = smart?.query {
            return "\(SmartListWords.kinds(query.kinds)), \(workCount.formatted())"
        }
        return titleCount
    }

    /// The page a link to the list opens.
    var route: Route {
        .list(ListRef(id: id, name: name))
    }
}

/// My List in the Lists tab: its name and count over a row of its covers.
private struct MyListSection: View {
    let list: WorkList
    @Environment(\.kivoPalette) private var palette

    private static let coverCount = 4

    var body: some View {
        NavigationLink(value: list.route) {
            VStack(alignment: .leading, spacing: BrowseRow.headingSpacing) {
                HStack(spacing: KivoTokens.spacing * 1.5) {
                    BrowseRow.heading(list.name)
                        .foregroundStyle(palette.text)
                    Image(systemName: "chevron.right")
                        .font(.subheadline.weight(.bold))
                        .foregroundStyle(palette.textMuted)
                    Spacer()
                    Text(list.workCount.formatted())
                        .foregroundStyle(palette.textMuted)
                }
                if list.workCount > 0 {
                    ListWorks(listId: list.id, limit: Self.coverCount) { works in
                        HStack(spacing: KivoTokens.spacing * 2) {
                            ForEach(0..<Self.coverCount, id: \.self) { index in
                                Color.clear
                                    .aspectRatio(CardStyle.poster.aspect, contentMode: .fit)
                                    .overlay {
                                        if index < works.count {
                                            ListCover(work: works[index], pointWidth: 90)
                                        }
                                    }
                            }
                        }
                    }
                }
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("list:\(list.name)")
    }
}

/// My List in a wide Lists tab: a row of its covers, as on Home.
private struct MyListRow: View {
    let list: WorkList
    @Environment(ServerModel.self) private var server

    /// As many as Home's rows hold.
    private static let limit = 20

    var body: some View {
        ListWorks(listId: list.id, limit: Self.limit) { works in
            if works.isEmpty {
                BrowseRow.skeleton(.poster, wide: true)
            } else {
                BrowseRow(title: list.name, items: works) {
                    server.paths[server.tab, default: []].append(list.route)
                }
                .environment(\.currentList, ListRef(id: list.id, name: list.name))
            }
        }
    }
}

/// One of the other lists in the Lists tab: its first cover, name and count.
private struct ListRow: View {
    let list: WorkList
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        NavigationLink(value: list.route) {
            HStack(spacing: KivoTokens.spacing * 3) {
                Group {
                    if list.workCount == 0 {
                        Image(systemName: "list.bullet")
                            .foregroundStyle(palette.textMuted)
                            .frame(maxWidth: .infinity, maxHeight: .infinity)
                            .background(palette.placeholder, in: RoundedRectangle(cornerRadius: KivoTokens.Radius.sm))
                            .accessibilityHidden(true)
                    } else {
                        ListWorks(listId: list.id, limit: 1) { works in
                            ListCover(work: works.first, pointWidth: 36)
                        }
                    }
                }
                .frame(width: 36, height: 36 / CardStyle.poster.aspect)
                VStack(alignment: .leading, spacing: KivoTokens.spacing * 0.5) {
                    Text(list.name)
                        .foregroundStyle(palette.text)
                        .lineLimit(1)
                    Text(list.detail)
                        .font(.footnote)
                        .foregroundStyle(palette.textMuted)
                }
                Spacer()
                Image(systemName: "chevron.right")
                    .font(.footnote.weight(.bold))
                    .foregroundStyle(palette.textMuted)
            }
            .padding(.horizontal, KivoTokens.spacing * 3.5)
            .padding(.vertical, KivoTokens.spacing * 2)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("list:\(list.name)")
    }
}

/// A work's poster in a list's row, or an empty cover until it's read.
private struct ListCover: View {
    let work: WorkSummary?
    let pointWidth: CGFloat
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        ArtworkImage(work.flatMap(CardStyle.poster.artwork(of:)), title: work?.title ?? "", pointWidth: pointWidth)
            .clipShape(RoundedRectangle(cornerRadius: KivoTokens.Radius.sm))
            .background(palette.placeholder, in: RoundedRectangle(cornerRadius: KivoTokens.Radius.sm))
    }
}

/// A list's first [limit] works, passed to [content] and read again as the list changes.
private struct ListWorks<Content: View>: View {
    let listId: String
    let limit: Int
    @ViewBuilder let content: ([WorkSummary]) -> Content
    @Environment(ServerModel.self) private var server
    @State private var works: [WorkSummary] = []

    var body: some View {
        content(works)
            .task(id: listId) {
                for await load in server.session.library.listWorks(listId: listId, limit: KotlinInt(int: Int32(limit)), kinds: nil) {
                    if let page = load.getOrNull(), page.items != works { works = page.items }
                }
            }
    }
}

extension View {
    /// An alert asking for a list's [name], whose [confirm] button passes it to [save], trimmed.
    func listNameAlert(
        _ title: String,
        isPresented: Binding<Bool>,
        name: Binding<String>,
        confirm: String,
        save: @escaping (String) -> Void
    ) -> some View {
        alert(title, isPresented: isPresented) {
            TextField(String(localized: "Name"), text: name)
            // Read at the tap: an alert can keep actions built from an earlier name.
            Button(confirm) { save(name.wrappedValue.trimmingCharacters(in: .whitespacesAndNewlines)) }
                .disabled(name.wrappedValue.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            Button(String(localized: "Cancel"), role: .cancel) {}
        }
    }
}
