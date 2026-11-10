import KivoCore
import KivoUI
import SwiftUI

/// The Libraries tab: a list of the pinned libraries in the user's order,
/// then More libraries, each row opening its page over it.
/// The page opened last on this device opens over it at launch.
struct LibrariesView: View {
    @Environment(ServerModel.self) private var server

    var body: some View {
        Group {
            if !server.libraries.isEmpty {
                List {
                    Section {
                        ForEach(server.pinnedLibraries, id: \.id) { library in
                            row(library)
                        }
                    }
                    if !server.otherLibraries.isEmpty {
                        Section(String(localized: "More libraries")) {
                            ForEach(server.otherLibraries, id: \.id) { library in
                                row(library)
                            }
                        }
                    }
                }
                .listStyle(.insetGrouped)
                .scrollContentBackground(.hidden)
                .accessibilityIdentifier("library-list")
            } else if server.unreachable {
                UnreachableView()
            } else {
                LoadingView(String(localized: "Loading libraries…"))
            }
        }
        .navigationTitle(String(localized: "Libraries"))
        .toolbarTitleDisplayMode(.large)
        // A page opened over the list is a switch, for its signposts.
        .onChange(of: server.navigation.paths[.libraries]?.first) { _, opened in
            if opened != nil { SwitchSignposts.chose() }
        }
    }

    private func row(_ library: Library) -> some View {
        NavigationLink(value: Route.librariesPage(library)) {
            LibraryRow(library: library)
        }
    }
}

/// A row of the list of libraries: the library's symbol and name, and how
/// many works it has.
struct LibraryRow: View {
    let library: Library

    var body: some View {
        LabeledContent {
            Text(Int(library.workCount).formatted())
        } label: {
            Label {
                // A link in a list takes the tint; the name stays the text colour.
                Text(library.name).foregroundStyle(Color.primary)
            } icon: {
                Image(systemName: library.symbol)
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("library:\(library.name)")
    }
}

extension View {
    /// A library's title: the system's large title, which
    /// collapses into the top bar as the page scrolls.
    func libraryTitle(_ title: String) -> some View {
        navigationTitle(title)
            .toolbarTitleDisplayMode(.large)
            .onChange(of: title, initial: true) { SwitchSignposts.titleShown() }
    }
}
