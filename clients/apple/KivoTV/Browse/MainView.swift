import KivoCore
import KivoUI
import SwiftUI

/// The signed-in app: the system's sidebar at the left, as the Android TV
/// app's rail, with Search, Home, Lists, the pinned libraries in the user's
/// order, More libraries when there are others, and Settings; each tab's pages pushed over it; the player over
/// everything. A Top Shelf item opens its work's page over Home, or plays it.
struct MainView: View {
    @Environment(AppModel.self) private var app
    @Environment(ServerModel.self) private var server

    var body: some View {
        @Bindable var navigation = server.navigation
        let player = server.player
        TabView(selection: $navigation.tab) {
            Tab(String(localized: "Search"), systemImage: "magnifyingglass", value: AppTab.search, role: .search) {
                BrowseStack(tab: .search) { SearchView() }
            }
            Tab(String(localized: "Home"), systemImage: "house", value: AppTab.home) {
                BrowseStack(tab: .home) { HomeView() }
            }
            Tab(String(localized: "Lists"), systemImage: "list.bullet", value: AppTab.lists) {
                BrowseStack(tab: .lists) { ListsView() }
            }
            // In the sidebar's own order: a section would list after the tabs, Settings too.
            ForEach(server.pinnedLibraries, id: \.id) { library in
                Tab(library.name, systemImage: library.symbol, value: AppTab.library(library.id)) {
                    BrowseStack(tab: .library(library.id)) { LibraryView(library: library, titled: false) }
                }
            }
            if !server.otherLibraries.isEmpty {
                Tab(String(localized: "More libraries"), systemImage: Symbols.more, value: AppTab.moreLibraries) {
                    BrowseStack(tab: .moreLibraries) { MoreLibrariesView() }
                }
            }
            Tab(String(localized: "Settings"), systemImage: "gearshape", value: AppTab.settings) {
                NavigationStack { SettingsView() }
            }
        }
        .tabViewStyle(.sidebarAdaptable)
        .background {
            if player.shown, let playback = player.playback {
                PlayerScreen(playback: playback) { player.shown = false }
            }
        }
        .onChange(of: player.shown) { _, shown in
            if !shown { player.dismissed() }
        }
        .alert(server.problem ?? "", isPresented: Binding(get: { server.problem != nil }, set: { if !$0 { server.problem = nil } })) {
            Button(String(localized: "OK"), role: .cancel) {}
        }
        .onChange(of: app.pendingLink, initial: true) { _, link in
            guard let link else { return }
            app.pendingLink = nil
            switch link {
            case let .work(id):
                navigation.tab = .home
                navigation.paths[.home] = [.work(WorkRoute(id: id))]
            case let .play(id):
                server.play(id)
            }
        }
    }
}

/// A tab's stack of pages: [root], with a work's, person's or collection's
/// page, a list's grid and a library's grid pushed over it.
struct BrowseStack<Root: View>: View {
    let tab: AppTab
    @ViewBuilder let root: () -> Root
    @Environment(ServerModel.self) private var server

    var body: some View {
        NavigationStack(path: path) {
            root()
                .navigationDestination(for: Route.self) { route in
                    Group {
                        switch route {
                        case let .work(work):
                            DetailView(route: work)
                        case let .list(list):
                            ListWorksView(list: list)
                        case let .library(library):
                            LibraryView(library: library)
                        }
                    }
                    // A pushed page doesn't take the stack's environment, so each is given it.
                    .environment(\.openWork, openWork)
                }
        }
        .environment(\.openWork, openWork)
    }

    /// Pushes a work's page onto this tab's stack.
    private var openWork: OpenWork {
        OpenWork { route in
            server.navigation.paths[tab, default: []].append(.work(route))
        }
    }

    private var path: Binding<[Route]> {
        Binding(
            get: { server.navigation.paths[tab] ?? [] },
            set: { server.navigation.paths[tab] = $0 }
        )
    }
}

/// The libraries that aren't pinned, each opening its grid.
private struct MoreLibrariesView: View {
    @Environment(ServerModel.self) private var server

    var body: some View {
        List(server.otherLibraries, id: \.id) { library in
            NavigationLink(value: Route.library(library)) {
                Label(library.name, systemImage: library.symbol)
            }
        }
        .frame(maxWidth: 1000)
        .navigationTitle(String(localized: "More libraries"))
    }
}
