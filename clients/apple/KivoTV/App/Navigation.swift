import KivoCore
import Observation

/// Where the signed-in Apple TV app is: the tab showing, and each tab's
/// stack of pages.
@MainActor
@Observable
final class Navigation {
    var tab: AppTab = .home
    var paths: [AppTab: [Route]] = [:]

    /// Pushes [route] onto the stack of the tab showing.
    func push(_ route: Route) {
        paths[tab, default: []].append(route)
    }

    /// Goes to the top of [library]'s grid: its own tab when it's pinned,
    /// else More libraries' with the library over its list.
    func show(_ library: Library) {
        if library.pinned {
            tab = .library(library.id)
            paths[tab] = []
        } else {
            tab = .moreLibraries
            paths[tab] = [.library(library)]
        }
    }
}

/// The app's tabs: Search, Home, Lists, each pinned library by its ID, More
/// libraries and Settings.
enum AppTab: Hashable {
    case search
    case home
    case lists
    case library(String)
    case moreLibraries
    case settings
}

/// A page pushed on a tab's stack.
enum Route: Hashable {
    case work(WorkRoute)
    /// A list's works, in a grid.
    case list(ListRef)
    /// A library that isn't pinned, opened from More libraries.
    case library(Library)
}
