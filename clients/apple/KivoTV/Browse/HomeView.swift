import KivoCore
import KivoUI
import SwiftUI

/// A Home row as the TV shows it: its row, the library that names it if
/// it's a library's, and the shape of its cards, nil where each work's own decides.
struct ShownRow {
    let row: HomeRow
    let library: Library?
    let style: CardStyle?
    /// The row's works the TV shows.
    let works: [WorkSummary]

    /// The row kinds the TV shows: no book rows, and no kind it doesn't know.
    static let kinds: Set<HomeRowKind> = [.continueWatching, .recentlyAdded, .list]

    /// The kinds of work the TV shows: no books.
    static let workKinds: Set<WorkKind> = [.movie, .series, .episode, .season, .video, .extra]

    /// The [rows] the TV shows among the [libraries] it shows: a list's books
    /// left out, and empty rows and the rows of libraries it doesn't show
    /// left out. Continue Watching and an other-videos library's rows are
    /// landscape cards, a list's each work's own, the rest posters.
    static func shown(_ rows: [HomeRow], libraries: [Library]) -> [ShownRow] {
        rows.compactMap { row in
            guard kinds.contains(row.kind) else { return nil }
            let works = row.kind == .list ? row.items.filter { workKinds.contains($0.kind) } : row.items
            let library = row.library.flatMap { ref in libraries.first { $0.id == ref.id } }
            guard !works.isEmpty, row.library == nil || library != nil else { return nil }
            let style: CardStyle? = switch row.kind {
            case .list: nil
            case .continueWatching: .landscape
            default: library?.kind == .videos ? .landscape : .poster
            }
            return ShownRow(row: row, library: library, style: style, works: works)
        }
    }
}

/// Home: the server's rows the TV shows, each a row of cards, refreshed in
/// place by the live events; empty rows don't come. A list's row ends in
/// See all. With nothing to show, it says why.
struct HomeView: View {
    @Environment(ServerModel.self) private var server
    @State private var home: Home?
    /// Why Home couldn't be read, when the server refused it.
    @State private var failure: RequestProblem?

    var body: some View {
        SkeletonGate(loaded: home != nil || failure != nil || server.unreachable) {
            if let failure {
                MessageView(String(localized: "Couldn't load Home."), sentence: failure.sentence)
            } else if let home {
                let rows = ShownRow.shown(home.rows, libraries: server.tvLibraries)
                if rows.isEmpty {
                    empty(allHidden: home.allHidden?.boolValue == true)
                } else {
                    ScrollView {
                        LazyVStack(alignment: .leading, spacing: TVLayout.rowGap) {
                            ForEach(rows, id: \.row.id) { shown in
                                row(shown)
                            }
                        }
                    }
                }
            } else {
                UnreachableView()
            }
        } skeleton: {
            VStack(alignment: .leading, spacing: TVLayout.rowGap) {
                SkeletonCardRow(style: .landscape)
                SkeletonCardRow(style: .poster)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        }
        .task {
            for await load in server.session.library.home() {
                if let read = load.getOrNull() {
                    failure = nil
                    if home != read { home = read }
                } else if let problem = load.problemOrNull(), !problem.isUnreachable {
                    failure = problem
                }
            }
        }
    }

    private func row(_ shown: ShownRow) -> some View {
        let row = shown.row
        let list = row.list
        return WorkRow(
            title: homeRowTitle(kind: row.kind, libraryName: shown.library?.name, listName: list?.name),
            works: shown.works,
            style: shown.style,
            seeAll: list.map { list in { server.navigation.push(.list(list)) } }
        )
        .environment(\.continueRow, row.kind == .continueWatching ? row.kind : nil)
        .environment(\.currentList, list)
    }

    @ViewBuilder private func empty(allHidden: Bool) -> some View {
        if server.tvLibraries.isEmpty, server.user?.admin == true {
            MessageView(String(localized: "No libraries"), sentence: String(localized: "Add libraries in the web app. They appear here as they're scanned."))
        } else if server.tvLibraries.isEmpty {
            MessageView(String(localized: "No libraries"), sentence: String(localized: "Ask an admin for access."))
        } else if allHidden {
            MessageView(String(localized: "Your Home is empty"), sentence: String(localized: "Choose its rows in Settings, under Home."))
        } else {
            MessageView(String(localized: "Nothing here yet"), sentence: String(localized: "Items appear here as libraries are scanned."))
        }
    }
}

/// Where a screen's content would be, when it has none to show because the
/// server doesn't answer: Retry asks again at once.
struct UnreachableView: View {
    @Environment(ServerModel.self) private var server

    var body: some View {
        MessageView(
            String(localized: "Can't reach \(server.serverName)."),
            sentence: String(localized: "Check your connection. Retrying automatically."),
            action: (label: String(localized: "Retry"), run: server.session.retryNow)
        )
    }
}
