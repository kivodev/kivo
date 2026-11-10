import Foundation
import KivoCore
import Observation

/// Where the signed-in app is: the tab showing, each browsing tab's stack
/// of pages, what Search shows, and the sheets over the tabs. A library is
/// a page of the Libraries tab in a narrow window and a tab of its own
/// beside the sidebar, and the library last chosen is kept on the device.
@MainActor
@Observable
final class Navigation {
    /// The tab showing, and each browsing tab's stack of pages.
    var tab: AppTab = .home
    var paths: [AppTab: [Route]] = [:]
    /// What Search shows, kept while the app moves between tabs.
    var searchQuery = ""
    /// Whether Settings shows, opened from Home's avatar or the sidebar's account row.
    var settingsShown = false
    /// Whether the window is wide enough for the sidebar, where each
    /// library is a tab of its own rather than a page of Libraries.
    private(set) var sidebar = false
    /// The work Add to List shows over the tabs, while it does.
    var addingToList: String?

    /// Every library the user sees, in their order, as last read.
    private var libraries: [Library] = []
    @ObservationIgnored private let serverId: String
    /// Whether the Libraries tab has opened on the page last chosen, once the libraries were first read.
    @ObservationIgnored private var restoredLibrariesPage = false

    init(serverId: String) {
        self.serverId = serverId
    }

    /// What the Libraries tab shows: the library last chosen on this
    /// device, else the first pinned library, else the first library; nil
    /// until the libraries are read.
    var librariesPage: Library? {
        libraries.first { $0.id == lastLibraryId } ?? libraries.first(where: \.pinned) ?? libraries.first
    }

    /// Pushes [route] onto the stack of the tab showing.
    func push(_ route: Route) {
        paths[tab, default: []].append(route)
    }

    /// The stack [tab] opens [library] with: over the list of libraries in
    /// the Libraries tab, else nothing over the page.
    private func stack(opening library: Library, in tab: AppTab) -> [Route] {
        tab == .libraries ? [.librariesPage(library)] : []
    }

    /// The libraries were read: the first time, the Libraries tab's list
    /// opens on the page last chosen on this device.
    func librariesRead(_ libraries: [Library]) {
        self.libraries = libraries
        guard !restoredLibrariesPage, !libraries.isEmpty else { return }
        restoredLibrariesPage = true
        guard !sidebar, lastLibraryId != nil, paths[.libraries, default: []].isEmpty, let library = librariesPage else { return }
        paths[.libraries] = stack(opening: library, in: .libraries)
    }

    /// Opens [library] in the Libraries tab, and remembers it on this device.
    func open(_ library: Library) {
        lastLibraryId = library.id
    }

    /// Chooses [tab], as the tab bar, the sidebar and the View menu do. A
    /// library is remembered on this device, as choosing it in the
    /// Libraries tab does; choosing Search again while there goes back
    /// to its results.
    func choose(_ tab: AppTab) {
        let again = tab == self.tab
        self.tab = tab
        switch tab {
        case .library(let id):
            if let library = libraries.first(where: { $0.id == id }) { open(library) }
        case .search:
            if again { paths[.search] = [] }
        default: break
        }
    }

    /// The tab [library] shows in: its own beside the sidebar, else Libraries.
    private func tab(of library: Library) -> AppTab {
        sidebar ? .library(library.id) : .libraries
    }

    /// Goes to the top of [library]'s grid, and remembers it on this device.
    func show(_ library: Library) {
        open(library)
        let tab = tab(of: library)
        paths[tab] = stack(opening: library, in: tab)
        self.tab = tab
    }

    /// Goes to the library a work is in, by the [libraryId] its card or page names, if it's one of the user's.
    func showLibrary(of libraryId: String?) {
        if let library = libraries.first(where: { $0.id == libraryId }) { show(library) }
    }

    /// Switches between the tab bar's Libraries tab and the sidebar's tab
    /// for each library as the window's width crosses between them, keeping the page showing and the pages pushed on it.
    /// The Libraries tab's list of libraries, which the sidebar has
    /// instead, shows in a narrow window only.
    func useSidebar(_ sidebar: Bool) {
        guard sidebar != self.sidebar else { return }
        self.sidebar = sidebar
        let from = tab
        var pushed = paths[from] ?? []
        let to: AppTab
        switch from {
        case .libraries:
            if case .librariesPage(let library) = pushed.first {
                open(library)
                pushed.removeFirst()
            }
            guard let library = librariesPage else { return }
            to = tab(of: library)
        case .library(let id):
            to = .libraries
            if let library = libraries.first(where: { $0.id == id }) {
                open(library)
                pushed.insert(contentsOf: stack(opening: library, in: to), at: 0)
            }
        default:
            return
        }
        paths[from] = nil
        paths[to] = pushed
        tab = to
    }

    private var lastLibraryKey: String { "lastLibrary.\(serverId)" }

    private var lastLibraryId: String? {
        get {
            access(keyPath: \.lastLibraryId)
            return UserDefaults.standard.string(forKey: lastLibraryKey)
        }
        set {
            withMutation(keyPath: \.lastLibraryId) {
                UserDefaults.standard.set(newValue, forKey: lastLibraryKey)
            }
        }
    }
}

/// The app's tabs. Libraries is the tab bar's, in a narrow window; each
/// library, by its ID, is the sidebar's.
enum AppTab: Hashable {
    case home
    case libraries
    case lists
    case library(String)
    case downloads
    case search
}
