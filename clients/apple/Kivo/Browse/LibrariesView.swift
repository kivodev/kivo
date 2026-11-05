import KivoCore
import KivoUI
import SwiftUI

/// The Libraries tab: a list of Favorites, then the pinned libraries in the
/// user's order, then More libraries, each row opening its page over it.
/// The page opened last on this device opens over it at launch.
struct LibrariesView: View {
    @Environment(ServerModel.self) private var server
    /// How many works the user marked a favorite, once read.
    @State private var favorites: Int?

    var body: some View {
        Group {
            if !server.libraries.isEmpty {
                List {
                    Section {
                        row(.favorites, favorites)
                    }
                    Section {
                        ForEach(server.pinnedLibraries, id: \.id) { library in
                            row(.library(library), Int(library.workCount))
                        }
                    }
                    if !server.otherLibraries.isEmpty {
                        Section(String(localized: "More libraries")) {
                            ForEach(server.otherLibraries, id: \.id) { library in
                                row(.library(library), Int(library.workCount))
                            }
                        }
                    }
                }
                .listStyle(.insetGrouped)
                .scrollContentBackground(.hidden)
                .accessibilityIdentifier("library-list")
                .task {
                    for await load in server.session.library.favoritesCount() {
                        guard let count = load.getOrNull() else { continue }
                        favorites = count.intValue
                    }
                }
            } else if server.unreachable {
                UnreachableView()
            } else {
                LoadingView(String(localized: "Loading libraries…"))
            }
        }
        .navigationTitle(String(localized: "Libraries"))
        .toolbarTitleDisplayMode(.large)
        // A page opened over the list is a switch, for its signposts.
        .onChange(of: server.paths[.libraries]?.first) { _, opened in
            if opened != nil { SwitchSignposts.chose() }
        }
    }

    private func row(_ page: LibrariesPage, _ count: Int?) -> some View {
        NavigationLink(value: Route.librariesPage(page)) {
            LibraryRow(page: page, count: count)
        }
    }
}

/// A row of the list of libraries: the library's symbol and name, and how
/// many works it has.
struct LibraryRow: View {
    let page: LibrariesPage
    let count: Int?

    var body: some View {
        LabeledContent {
            if let count {
                Text(count.formatted())
            }
        } label: {
            Label {
                // A link in a list takes the tint; the name stays the text colour.
                Text(page.name).foregroundStyle(Color.primary)
            } icon: {
                Image(systemName: page.symbol)
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("library:\(page.name)")
    }
}

/// A library's grid or Favorites, as the Libraries tab shows it.
struct LibrariesPageView: View {
    let page: LibrariesPage
    @Environment(ServerModel.self) private var server

    var body: some View {
        switch page {
        case .favorites:
            FavoritesView(server: server)
        case .library(let library):
            LibraryView(library: library, server: server)
                .id(library.id)
        }
    }
}

extension View {
    /// A library's or Favorites' title: the system's large title, which
    /// collapses into the top bar as the page scrolls.
    func libraryTitle(_ title: String) -> some View {
        navigationTitle(title)
            .toolbarTitleDisplayMode(.large)
            .onChange(of: title, initial: true) { SwitchSignposts.titleShown() }
    }
}

extension LibrariesPage {
    var name: String {
        switch self {
        case .favorites: String(localized: "Favorites")
        case .library(let library): library.name
        }
    }

    /// The page's SF Symbol.
    var symbol: String {
        switch self {
        case .favorites: "heart"
        case .library(let library): library.symbol
        }
    }
}
