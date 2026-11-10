import KivoCore
import KivoUI
import SwiftUI

/// The Lists tab: a shelf for each list, My List first and the others by
/// name, its name and count over a row of its first titles; the name opens
/// its works in a grid. My List has no shelf while it's empty. The + makes
/// a list, or opens the smart list builder.
struct ListsView: View {
    @Environment(ServerModel.self) private var server
    @Environment(\.wide) private var wide

    @State private var naming = false
    @State private var newName = ""
    @State private var problem: String?
    /// Whether the name prompt comes back, with what was typed, once a taken name is said.
    @State private var namingAgain = false
    @State private var buildingSmart = false
    /// The lists with their counts and first works, read only while this tab shows.
    @State private var counted: [WorkList]?

    private static let rowSpacing = KivoTokens.spacing * 7
    /// The list kinds this app shows; a kind it doesn't know is skipped.
    private static let knownKinds: Set<String> = ["manual", "smart"]

    var body: some View {
        SkeletonGate(loaded: counted != nil || server.listsFailure != nil || server.unreachable) {
            if let counted {
                let shelves = counted.filter { Self.knownKinds.contains($0.kind) && (!$0.builtIn || $0.workCount > 0) }
                if shelves.isEmpty {
                    MessageView(
                        String(localized: "Nothing in your lists yet."),
                        sentence: String(localized: "Add something to Watchlist from a title's page.")
                    )
                    .frame(maxHeight: .infinity)
                } else {
                    ScrollView {
                        LazyVStack(alignment: .leading, spacing: Self.rowSpacing) {
                            ForEach(shelves, id: \.id) { list in
                                BrowseRow(
                                    title: list.name,
                                    items: list.items ?? [],
                                    smart: list.isSmart,
                                    detail: list.workCount == 0 ? String(localized: "Empty") : list.titleCount,
                                    placeholder: String(localized: "Nothing in this list yet")
                                ) {
                                    server.paths[server.tab, default: []].append(list.route)
                                }
                                .environment(\.currentList, ListRef(id: list.id, name: list.name, kind: list.kind))
                            }
                        }
                        .padding(.top, KivoTokens.spacing * 2)
                        .padding(.bottom, KivoTokens.spacing * 6)
                    }
                }
            } else if let failure = server.listsFailure {
                MessageView(String(localized: "Couldn't load your lists."), sentence: failure.sentence)
                    .frame(maxHeight: .infinity)
            } else {
                UnreachableView()
            }
        } skeleton: {
            ScrollView {
                VStack(alignment: .leading, spacing: Self.rowSpacing) {
                    BrowseRow.skeleton(.poster, wide: wide)
                    BrowseRow.skeleton(.poster, wide: wide)
                }
                .padding(.top, KivoTokens.spacing * 2)
            }
            .scrollDisabled(true)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(Text("Loading lists…"))
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .navigationTitle(String(localized: "Lists"))
        .toolbarTitleDisplayMode(.large)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    Button(String(localized: "New List"), systemImage: "list.bullet") {
                        newName = ""
                        naming = true
                    }
                    Button(String(localized: "New Smart List"), systemImage: "sparkles") { buildingSmart = true }
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
        .task {
            for await load in server.session.library.lists(counted: true) {
                if let lists = load.getOrNull() as? [WorkList] { counted = lists }
            }
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

    /// The page a link to the list opens.
    var route: Route {
        .list(ListRef(id: id, name: name, kind: kind))
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
