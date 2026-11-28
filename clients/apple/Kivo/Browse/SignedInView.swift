import KivoCore
import KivoUI
import SwiftUI

/// The signed-in app: the system's tab bar with Home,
/// Libraries, Lists and Downloads, and Search as its own button, which on iOS 26
/// becomes the search field. In a regular-width iPad window the tab bar can
/// open into a sidebar, where each library is a tab of its
/// own, the pinned libraries under Libraries and the others under More
/// libraries, and the account at its foot, opening Settings as Home's
/// avatar does; crossing between the two widths keeps the page showing.
/// The user menu is the avatar on Home. The live events run while the app
/// is in the foreground, and coming to the foreground brings the downloads
/// up to date; leaving with one still on the server asks for the downloads'
/// background refresh. A `kivo://link` link opens Link a device over it. While a book is loaded its mini player
/// sits above the tabs and opens Now Playing, and while casting the cast's
/// mini controller does, opening the remote; the video player and the
/// remote cover everything. While the app and the server don't work
/// together, every tab but Downloads, whose downloads play without the
/// server, shows which of them needs an update.
struct SignedInView: View {
    @Environment(AppModel.self) private var app
    @Environment(ServerModel.self) private var server
    @Environment(\.scenePhase) private var scenePhase
    @Environment(\.horizontalSizeClass) private var horizontalSizeClass
    @Environment(\.verticalSizeClass) private var verticalSizeClass

    var body: some View {
        @Bindable var navigation = server.navigation
        @Bindable var player = server.player
        @Bindable var book = server.book
        // The sidebar is iPad's: an iPhone in landscape is regular width too, but keeps its tab bar.
        let sidebar = UIDevice.current.userInterfaceIdiom == .pad && horizontalSizeClass == .regular
        // Wide is the sidebar's grids and pages, and an unfolded iPhone Duo's too: regular in
        // both size classes, though it keeps its tab bar. An iPhone in landscape is compact height.
        let wide = sidebar || (horizontalSizeClass == .regular && verticalSizeClass == .regular)
        TabView(selection: selection) {
            Tab(String(localized: "Home"), systemImage: "house", value: AppTab.home) {
                BrowseStack(tab: .home) { HomeView() }
                    .modifier(MiniPlayerBar(book: book))
                    .modifier(CompatibilityGate())
            }
            Tab(String(localized: "Libraries"), systemImage: "square.grid.2x2", value: AppTab.libraries) {
                BrowseStack(tab: .libraries) { LibrariesView() }
                    .modifier(MiniPlayerBar(book: book))
                    .modifier(CompatibilityGate())
            }
            .hidden(sidebar)
            Tab(String(localized: "Lists"), systemImage: "list.bullet", value: AppTab.lists) {
                BrowseStack(tab: .lists) { ListsView() }
                    .modifier(MiniPlayerBar(book: book))
                    .modifier(CompatibilityGate())
            }
            TabSection(String(localized: "Libraries")) {
                ForEach(server.pinnedLibraries, id: \.id) { library in
                    libraryTab(library, book: book)
                }
            }
            .hidden(!sidebar)
            TabSection(String(localized: "More libraries")) {
                ForEach(server.otherLibraries, id: \.id) { library in
                    libraryTab(library, book: book)
                }
            }
            .hidden(!sidebar || server.otherLibraries.isEmpty)
            .tabPlacement(.sidebarOnly)
            Tab(String(localized: "Downloads"), systemImage: "arrow.down.circle", value: AppTab.downloads) {
                NavigationStack { DownloadsView().kivoPageBackground() }
                    .modifier(MiniPlayerBar(book: book))
            }
            Tab(String(localized: "Search"), systemImage: "magnifyingglass", value: AppTab.search, role: .search) {
                SearchTab(query: $navigation.searchQuery, chosen: navigation.tab == .search)
                    .modifier(MiniPlayerBar(book: book))
                    .modifier(CompatibilityGate())
            }
        }
        .modifier(SearchTabActivation())
        .tabViewStyle(.sidebarAdaptable)
        .tabViewSidebarBottomBar { SidebarAccount() }
        .onChange(of: sidebar, initial: true) { _, sidebar in navigation.useSidebar(sidebar) }
        .environment(\.wide, wide)
        .modifier(MiniPlayerAccessoryModifier(book: book, cast: server.cast, open: { player.shown = true }))
        .sheet(isPresented: castMenuShown(over: false), onDismiss: player.castMenuClosed) {
            CastMenu(cast: server.cast, stop: player.stopCastingAfterMenu)
                .environment(\.artworkSource, server.artwork)
        }
        .sheet(isPresented: $book.expanded) {
            NowPlayingView(book: book)
        }
        .sheet(isPresented: $navigation.settingsShown) {
            NavigationStack { SettingsView() }
        }
        .sheet(isPresented: addToListShown) {
            if let workId = navigation.addingToList {
                AddToListView(workId: workId)
            }
        }
        .sheet(isPresented: linkShown) {
            NavigationStack {
                LinkDeviceView(initial: app.pendingLink)
                    .toolbar {
                        ToolbarItem(placement: .cancellationAction) {
                            Button(String(localized: "Close")) { app.pendingLink = nil }
                        }
                    }
            }
        }
        .environment(\.artworkSource, server.artwork)
        .onChange(of: scenePhase) { _, phase in
            if phase == .background, DownloadRefresh.pending(server.downloads) { DownloadRefresh.schedule() }
        }
        .alert(server.problem ?? "", isPresented: problemShown) {
            Button(String(localized: "OK"), role: .cancel) {}
        }
        .fullScreenCover(isPresented: $player.shown, onDismiss: player.dismissed) {
            Group {
                if let model = player.model {
                    PlayerScreen(model: model)
                } else if server.cast.active {
                    CastRemoteScreen(cast: server.cast, close: { player.shown = false })
                }
            }
            .environment(\.artworkSource, server.artwork)
            .sheet(isPresented: castMenuShown(over: true), onDismiss: player.castMenuClosed) {
                CastMenu(cast: server.cast, stop: player.stopCastingAfterMenu)
                    .environment(\.artworkSource, server.artwork)
            }
        }
    }

    /// Whether the Cast menu shows over the player or the remote [over], or over the tabs.
    private func castMenuShown(over player: Bool) -> Binding<Bool> {
        Binding(
            get: { server.cast.menuShown && server.player.shown == player },
            set: { if !$0 { server.cast.menuShown = false } }
        )
    }

    /// A library's tab in the sidebar, which opens on its grid.
    private func libraryTab(_ library: Library, book: BookPlayer) -> some TabContent<AppTab> {
        Tab(library.name, systemImage: library.symbol, value: AppTab.library(library.id)) {
            BrowseStack(tab: .library(library.id)) {
                LibraryView(library: library, server: server)
            }
            .modifier(MiniPlayerBar(book: book))
            .modifier(CompatibilityGate())
        }
    }

    private var problemShown: Binding<Bool> {
        Binding(get: { server.problem != nil }, set: { if !$0 { server.problem = nil } })
    }

    /// Whether Add to List shows.
    private var addToListShown: Binding<Bool> {
        Binding(get: { server.navigation.addingToList != nil }, set: { if !$0 { server.navigation.addingToList = nil } })
    }

    /// The selected tab, chosen as ``Navigation/choose(_:)`` says.
    /// Whether Link a device shows for a `kivo://link` link opened from the system Camera.
    private var linkShown: Binding<Bool> {
        Binding(get: { app.pendingLink != nil }, set: { if !$0 { app.pendingLink = nil } })
    }

    private var selection: Binding<AppTab> {
        Binding(get: { server.navigation.tab }, set: { server.navigation.choose($0) })
    }
}

/// A tab's screens, or, while the app and the server don't work together,
/// which of them needs an update.
private struct CompatibilityGate: ViewModifier {
    @Environment(AppModel.self) private var app
    @Environment(ServerModel.self) private var server

    func body(content: Content) -> some View {
        if let problem = server.incompatibility {
            CompatibilityView(problem: problem, retry: server.checkCompatibility, changeServer: app.core.changeServer)
        } else {
            content
        }
    }
}

/// The search field's prompt.
private let searchPrompt = Text("Movies, shows, episodes, people")

/// The Search tab: its results under the search field, which on iOS 26
/// is the tab bar's. Arriving with nothing typed puts the cursor in it.
private struct SearchTab: View {
    @Binding var query: String
    /// Search is the chosen tab.
    let chosen: Bool
    @State private var presented = false

    var body: some View {
        if #available(iOS 26, *) {
            BrowseStack(tab: .search) { SearchView(query: $query) }
                .searchable(text: $query, isPresented: $presented, prompt: searchPrompt)
                .onChange(of: chosen, initial: true) { _, chosen in
                    if chosen, query.isEmpty { presented = true }
                }
        } else {
            BrowseStack(tab: .search) {
                SearchView(query: $query)
                    .onAppear { if query.isEmpty { presented = true } }
            }
            .searchable(text: $query, isPresented: $presented, prompt: searchPrompt)
        }
    }
}

/// On iOS 26, choosing Search activates its field, which also sets Search
/// apart from the other tabs as its own circle on iOS 27.
private struct SearchTabActivation: ViewModifier {
    func body(content: Content) -> some View {
        if #available(iOS 26, *) {
            content.tabViewSearchActivation(.searchTabSelection)
        } else {
            content
        }
    }
}

/// The account at the foot of the sidebar: the avatar and the username, opening Settings.
private struct SidebarAccount: View {
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        Button {
            server.navigation.settingsShown = true
        } label: {
            // Inset and sized as the sidebar's rows are, so the avatar sits in their icon column.
            Label {
                Text(server.user?.username ?? "")
                    .foregroundStyle(palette.text)
                    .lineLimit(1)
            } icon: {
                Avatar(initial: server.user?.initial, size: 28)
            }
            .frame(maxWidth: .infinity, minHeight: 44, alignment: .leading)
            .padding(.horizontal, 16)
            .contentShape(.rect(cornerRadius: 12))
        }
        .buttonStyle(.plain)
        .hoverEffect(.highlight)
        .padding(.horizontal, 16)
        .padding(.bottom, 8)
        .accessibilityLabel(Text("Account"))
        .accessibilityValue(server.user?.username ?? "")
        .accessibilityIdentifier("sidebar-account")
    }
}

/// On iOS 26.1 and later, the mini player is the tab bar's accessory
/// while a book is loaded, and the cast's mini controller while something
/// plays on a TV.
private struct MiniPlayerAccessoryModifier: ViewModifier {
    let book: BookPlayer
    let cast: CastModel
    let open: () -> Void

    func body(content: Content) -> some View {
        if #available(iOS 26.1, *) {
            content.tabViewBottomAccessory(isEnabled: book.loaded || cast.active) {
                if cast.active {
                    CastMiniAccessory(cast: cast, open: open)
                } else {
                    MiniPlayerAccessory(book: book)
                }
            }
        } else {
            content
        }
    }
}

/// A tab's navigation, with the route every browse screen links to. Pages
/// below it push a work with `openWork`, as a long press's Go to Series does.
struct BrowseStack<Root: View>: View {
    let tab: AppTab
    @ViewBuilder let root: () -> Root
    @Environment(ServerModel.self) private var server

    var body: some View {
        NavigationStack(path: path) {
            root()
                .kivoPageBackground()
                .navigationDestination(for: Route.self) { route in
                    Group {
                        switch route {
                        case .work(let route):
                            switch route.page {
                            case .work: DetailView(route: route, server: server).id(route.id)
                            case .person: PersonDetail(id: route.id, author: false)
                            case .author: PersonDetail(id: route.id, author: true)
                            case .bookSeries: BookSeriesDetail(id: route.id)
                            case .collection: CollectionDetail(id: route.id)
                            }
                        case .list(let list):
                            ListGrid(list: list, session: server.session)
                        case .library(let library, let sort):
                            LibraryView(library: library, server: server, sort: sort)
                        case .librariesPage(let library):
                            LibraryView(library: library, server: server)
                                .id(library.id)
                                .onAppear { server.navigation.open(library) }
                        }
                    }
                    .kivoPageBackground()
                }
        }
        .environment(\.openWork, OpenWork { route in
            server.navigation.paths[tab, default: []].append(.work(route))
        })
    }

    private var path: Binding<[Route]> {
        Binding(
            get: { server.navigation.paths[tab] ?? [] },
            set: { server.navigation.paths[tab] = $0 }
        )
    }
}

/// A page pushed on a tab's stack.
enum Route: Hashable {
    case work(WorkRoute)
    /// A list's works, in a grid.
    case list(ListRef)
    /// A library's works in one sort, for once, as Recently Added's heading opens them.
    case library(Library, sort: GridSort)
    /// A library, opened from the Libraries tab's list of them.
    case librariesPage(Library)
}

extension EnvironmentValues {
    /// The window has the wider grids and pages: a regular-width iPad's, which
    /// also has the sidebar, where each library is a tab, or an
    /// unfolded iPhone Duo's, which keeps its tab bar.
    @Entry var wide = false
}

/// Where a screen's content would be, when it has none to show because the
/// server doesn't answer: Retry asks again at once, and Go to Downloads
/// shows while there are downloads. A screen whose read doesn't retry by
/// itself passes [reload], run on Retry and once the server answers.
struct UnreachableView: View {
    var reload: (() -> Void)?
    @Environment(ServerModel.self) private var server

    var body: some View {
        MessageView(
            String(localized: "Can't reach \(server.serverName)."),
            sentence: String(localized: "Check your connection. Retrying automatically."),
            action: (label: String(localized: "Retry"), run: {
                server.session.retryNow()
                reload?()
            }),
            secondary: server.downloads.isEmpty ? nil : (label: String(localized: "Go to Downloads"), run: { server.navigation.tab = .downloads })
        )
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .task {
            guard let reload else { return }
            try? await server.session.awaitReachable()
            if !Task.isCancelled { reload() }
        }
    }
}
