import KivoCore
import KivoUI
import SwiftUI

/// Home: the user's rows, each a horizontal row of cards,
/// refetched when the live events say or on a pull; empty rows don't come.
/// A compact top bar names the server and holds the user menu, the
/// avatar, so Continue Watching starts near the top. In a wide window the
/// server's name is the system's large title, which collapses into the top
/// bar as the rows scroll.
struct HomeView: View {
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette
    @Environment(\.wide) private var wide
    @State private var home: Home?
    /// Why Home couldn't be read, when the server refused it.
    @State private var failure: RequestProblem?

    private static let rowSpacing = KivoTokens.spacing * 7
    private static let top = KivoTokens.spacing * 2
    /// The rows the skeleton shows: Continue Watching's stills and two rows of posters.
    private static let skeletonStyles: [CardStyle] = [.landscape, .poster, .poster]
    /// The rows a card's long press can remove it from.
    private static let continueRows: Set<String> = ["continueWatching", "continueListening"]

    var body: some View {
        SkeletonGate(loaded: home != nil || failure != nil || server.unreachable) {
            if let failure {
                MessageView(String(localized: "Couldn't load Home."), sentence: failure.sentence)
                    .frame(maxHeight: .infinity)
            } else if let home {
                if home.rows.isEmpty {
                    MessageView(
                        String(localized: "Nothing here yet"),
                        sentence: String(localized: "Items appear here as libraries are scanned.")
                    )
                    .frame(maxHeight: .infinity)
                } else {
                    ScrollView {
                        LazyVStack(alignment: .leading, spacing: Self.rowSpacing) {
                            ForEach(home.rows, id: \.id) { row in
                                BrowseRow(title: title(of: row), items: row.items, style: Self.fixedStyle(row.kind), more: more(of: row))
                                    .environment(\.continueRow, Self.continueRows.contains(row.kind) ? row.kind : nil)
                            }
                        }
                        .padding(.top, Self.top)
                        .padding(.bottom, KivoTokens.spacing * 6)
                    }
                    .refreshable { await refresh() }
                }
            } else {
                UnreachableView()
            }
        } skeleton: {
            ScrollView {
                VStack(alignment: .leading, spacing: Self.rowSpacing) {
                    ForEach(Array(Self.skeletonStyles.enumerated()), id: \.offset) { _, style in
                        BrowseRow.skeleton(style, wide: wide)
                    }
                }
                .padding(.top, Self.top)
            }
            .scrollDisabled(true)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(Text("Loading Home…"))
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .modifier(HomeTitle(server: server.serverName, wide: wide))
        .toolbar {
            if !wide {
                ToolbarItem(placement: .topBarLeading) {
                    Text(server.serverName)
                        .font(.title3.weight(.bold))
                        .foregroundStyle(palette.text)
                        .fixedSize()
                        .accessibilityAddTraits(.isHeader)
                }
                .bareToolbarItem()
            }
            ToolbarItem(placement: .topBarTrailing) {
                HStack(spacing: KivoTokens.spacing * 4) {
                    CastButton(cast: server.cast)
                        .tint(palette.text)
                    AccountButton { server.settingsShown = true }
                }
            }
            .bareToolbarItem()
        }
        // Coming back to Home reads it again behind the rows already showing.
        .task {
            for await load in server.session.library.home() {
                if let home = load.getOrNull() {
                    failure = nil
                    show(home)
                } else if let problem = load.problemOrNull(), !problem.isUnreachable {
                    failure = problem
                }
            }
        }
    }

    /// The shape every card of a row of [kind] takes, nil where each
    /// work's own decides: Continue Watching as landscape cards, where the
    /// frame says where you are.
    private static func fixedStyle(_ kind: String) -> CardStyle? {
        switch kind {
        case "continueWatching": .landscape
        default: nil
        }
    }

    /// Reads Home again now, for a pull to refresh.
    private func refresh() async {
        for await load in server.session.library.home() {
            if let home = load.getOrNull() { show(home) }
            break
        }
    }

    /// Shows [home] only if it changed, so a read that finds nothing new redraws nothing.
    private func show(_ home: Home) {
        if self.home != home { self.home = home }
    }

    /// What a row's heading opens: Favorites' opens the Favorites page.
    private func more(of row: HomeRow) -> (() -> Void)? {
        row.kind == "favorites" ? { server.showFavorites() } : nil
    }

    private func title(of row: HomeRow) -> String {
        switch row.kind {
        case "continueWatching": String(localized: "Continue Watching")
        case "continueListening": String(localized: "Continue Listening")
        case "continueSeries": String(localized: "Continue Series")
        case "favorites": String(localized: "Favorites")
        case "recentlyAdded":
            if let library = server.libraries.first(where: { $0.id == row.library?.id }) {
                String(localized: "Recently Added in \(library.name)")
            } else {
                String(localized: "Recently Added")
            }
        default: ""
        }
    }
}

/// Home's title: in a wide window the server's name as the large title;
/// else "Home", kept for the back menu and VoiceOver, with the bar's title
/// removed, as the server's name stands in for it.
private struct HomeTitle: ViewModifier {
    let server: String
    let wide: Bool

    func body(content: Content) -> some View {
        if wide {
            content
                .navigationTitle(server)
                .toolbarTitleDisplayMode(.large)
        } else {
            content
                .navigationTitle(String(localized: "Home"))
                .toolbarTitleDisplayMode(.inline)
                .toolbar(removing: .title)
        }
    }
}

extension ToolbarContent {
    /// A toolbar item without the shared glass behind it on iOS 26, for a
    /// title or a control that draws its own shape.
    @ToolbarContentBuilder func bareToolbarItem() -> some ToolbarContent {
        if #available(iOS 26.0, *) {
            sharedBackgroundVisibility(.hidden)
        } else {
            self
        }
    }
}

/// The avatar, which opens Settings.
private struct AccountButton: View {
    let action: () -> Void
    @Environment(ServerModel.self) private var server

    var body: some View {
        Button(action: action) {
            Avatar(initial: server.user?.initial)
                // The top bar has no room for the largest sizes.
                .dynamicTypeSize(...DynamicTypeSize.accessibility2)
        }
        .accessibilityLabel(Text("Account"))
        .accessibilityValue(server.user?.username ?? "")
        .accessibilityIdentifier("user")
    }
}
