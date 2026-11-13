import KivoCore
import KivoUI
import SwiftUI

/// A detail page, by what it's of: a work's, a person's or a collection's.
struct DetailView: View {
    let route: WorkRoute
    @Environment(ServerModel.self) private var server

    var body: some View {
        switch route.page {
        case .work:
            WorkPageView(model: WorkPageModel(route: route, session: server.session))
                .id(route)
        case .person, .author:
            PersonView(id: route.id)
        case .collection:
            CollectionView(id: route.id)
        case .bookSeries:
            EmptyView()
        }
    }
}

/// A work's page, by its kind: a movie or other video, a series with its
/// seasons and episodes, or an episode with its season. What's known of the
/// work shows at once and the rest fills in as it's read; a failure shows
/// under the hero, or in its place when nothing was known.
private struct WorkPageView: View {
    @State var model: WorkPageModel
    @Environment(ServerModel.self) private var server

    var body: some View {
        let page = model.state
        Group {
            if let head = page.head {
                switch head.kind {
                case .series:
                    SeriesDetail(series: head, page: page, model: model.page)
                case .season:
                    // The core shows a season's page as its series'.
                    ProgressView()
                default:
                    PlayableDetail(work: head, page: page, model: model.page)
                }
            } else if let problem = page.problem, !problem.isUnreachable {
                MessageView(String(localized: "Couldn't load this page."), sentence: problem.sentence)
            } else if server.unreachable {
                UnreachableView()
            } else {
                ProgressView()
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}

/// The frame of a detail page: the backdrop across the whole screen,
/// darkened at the left so text reads on any artwork and fading into the
/// background below, under the hero at the foot of the first screen, and
/// [rows] under it.
struct DetailPage<Hero: View, Rows: View>: View {
    let art: ImageRef?
    @ViewBuilder let hero: () -> Hero
    @ViewBuilder let rows: () -> Rows
    @Environment(\.kivoPalette) private var palette
    /// How dark the backdrop is, from 0 at the top of the page to 1 once it has scrolled [dimmedAt], in twentieths.
    @State private var dim: CGFloat = 0

    /// How far it scrolls before the backdrop is as dark as it gets.
    private static var dimmedAt: CGFloat { 400 }

    var body: some View {
        ZStack(alignment: .topLeading) {
            ArtworkImage(art, title: "", pointWidth: 1920, blur: true)
                .overlay {
                    LinearGradient(
                        stops: [
                            .init(color: palette.bg.opacity(0.95), location: 0),
                            .init(color: palette.bg.opacity(0.7), location: 0.4),
                            .init(color: palette.bg.opacity(0), location: 0.75),
                        ],
                        startPoint: .leading,
                        endPoint: .trailing
                    )
                }
                .overlay {
                    LinearGradient(colors: [palette.bg.opacity(0), palette.bg], startPoint: .center, endPoint: .bottom)
                }
                // Darkening as the rows come up over it, so they read on any artwork.
                .overlay { palette.bg.opacity(dim * 0.85).animation(.linear(duration: 0.2), value: dim) }
                .ignoresSafeArea()
            ScrollView {
                VStack(alignment: .leading, spacing: TVLayout.rowGap) {
                    VStack(alignment: .leading, spacing: 0) {
                        Spacer(minLength: 0)
                        hero()
                    }
                    // The hero sits at the foot of the first screen, with the row under it peeking in.
                    .frame(minHeight: 600, alignment: .bottomLeading)
                    .focusSection()
                    rows()
                }
                .padding(.bottom, TVLayout.rowGap)
            }
            .scrollClipDisabled()
            .onScrollGeometryChange(for: CGFloat.self) { geometry in
                (min(1, max(0, geometry.contentOffset.y + geometry.contentInsets.top) / Self.dimmedAt) * 20).rounded() / 20
            } action: { _, level in
                dim = level
            }
        }
    }
}

/// A hero's title, as the work's logo where it has one, its details line
/// and its overview, whose lines are held while the page is read.
struct HeroTitle: View {
    let work: WorkSummary
    let details: [String]
    let overview: String?
    @Environment(\.kivoPalette) private var palette

    /// The work's logo: an episode is headed by its own title, and a movie
    /// whose cut is named by its labelled title, as their logos name the series or film alone.
    private var logo: ImageRef? {
        work.episode == nil && work.movie?.edition == nil ? work.images.logo : nil
    }

    var body: some View {
        VStack(alignment: .leading, spacing: KivoTokens.spacing * 4) {
            LogoImage(logo, pointWidth: 560) {
                Text(work.labelledTitle)
                    .kivoStyle(KivoTokens.TextSize.xl3, weight: KivoTokens.Weight.bold)
                    .foregroundStyle(palette.text)
                    .lineLimit(2)
            }
            .frame(maxWidth: logo == nil ? 1100 : 560, maxHeight: logo == nil ? nil : 200, alignment: .leading)
            if !details.isEmpty {
                Text(details.joined(separator: " · "))
                    .kivoStyle(KivoTokens.TextSize.sm)
                    .foregroundStyle(palette.textMuted)
            }
            if let overview {
                HeroOverview(title: work.labelledTitle, text: overview)
            }
        }
    }
}

/// A hero's overview, its paragraphs run together, in three lines ended at
/// a word with an ellipsis. When it's cut short, it takes focus, says More,
/// and Select shows the whole of it.
struct HeroOverview: View {
    let title: String
    let text: String
    @Environment(\.kivoPalette) private var palette
    @State private var open = false

    private static let lines = 3
    /// The text's width, short of the overview's, which More sits at the end of.
    private static let textWidth: CGFloat = 940
    private static let width: CGFloat = 1036

    var body: some View {
        let whole = text.split(whereSeparator: \.isWhitespace).joined(separator: " ")
        let cut = Self.cut(whole)
        Button {
            open = true
        } label: {
            ZStack(alignment: .bottomTrailing) {
                Text(cut ?? whole)
                    .kivoStyle(KivoTokens.TextSize.sm)
                    .foregroundStyle(palette.text.opacity(0.85))
                    .lineLimit(Self.lines, reservesSpace: true)
                    .frame(width: Self.textWidth, alignment: .leading)
                    .frame(width: Self.width, alignment: .leading)
                if cut != nil {
                    Text("More")
                        .kivoStyle(KivoTokens.TextSize.xs, weight: KivoTokens.Weight.semibold)
                        .foregroundStyle(palette.text)
                }
            }
        }
        .buttonStyle(OverviewStyle())
        .disabled(cut == nil)
        .fullScreenCover(isPresented: $open) {
            OverviewText(title: title, text: text)
        }
    }

    /// [text] ended at the last word that leaves room for an ellipsis in
    /// the overview's lines, or nil when the whole of it fits.
    static func cut(_ text: String) -> String? {
        let font = UIFont.systemFont(ofSize: KivoTokens.TextSize.sm.size)
        let room = font.lineHeight * (CGFloat(lines) + 0.5)
        func fits(_ text: String) -> Bool {
            let bounds = (text as NSString).boundingRect(
                with: CGSize(width: textWidth, height: .greatestFiniteMagnitude),
                options: .usesLineFragmentOrigin,
                attributes: [.font: font],
                context: nil
            )
            return bounds.height <= room
        }
        guard !fits(text) else { return nil }
        let words = text.split(separator: " ")
        var low = 0
        var high = words.count - 1
        while low < high {
            let mid = (low + high + 1) / 2
            if fits(words[..<mid].joined(separator: " ") + "…") { low = mid } else { high = mid - 1 }
        }
        let kept = words[..<low].joined(separator: " ").trimmingCharacters(in: .punctuationCharacters.union(.whitespaces))
        return kept + "…"
    }
}

/// The overview: a platter lights behind it and it lifts while it has
/// focus, as the system's focusable text does.
private struct OverviewStyle: ButtonStyle {
    @Environment(\.isFocused) private var focused

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .padding(.horizontal, 21)
            .padding(.vertical, 7)
            .background(.white.opacity(focused ? 0.16 : 0), in: RoundedRectangle(cornerRadius: KivoTokens.Radius.md))
            .scaleEffect(configuration.isPressed ? 0.98 : focused ? 1.02 : 1)
            .shadow(color: .black.opacity(focused ? 0.3 : 0), radius: 16, y: 8)
            .animation(.easeOut(duration: 0.15), value: focused)
            .padding(.horizontal, -21)
            .padding(.vertical, -7)
    }
}

/// The whole of an overview over the dimmed screen, under its work's
/// title, as the system shows a description's More; Menu closes it.
private struct OverviewText: View {
    let title: String
    let text: String
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        VStack(alignment: .leading, spacing: KivoTokens.spacing * 8) {
            Text(title)
                .kivoStyle(KivoTokens.TextSize.xl2, weight: KivoTokens.Weight.bold)
                .foregroundStyle(palette.text)
            Text(text.trimmingCharacters(in: .whitespacesAndNewlines))
                .kivoStyle(KivoTokens.TextSize.base)
                .foregroundStyle(palette.text)
                .lineSpacing(KivoTokens.spacing * 2)
                .minimumScaleFactor(0.6)
        }
        .frame(maxWidth: 1200, alignment: .leading)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(.ultraThickMaterial)
        .background(palette.bg.opacity(0.6))
        .ignoresSafeArea()
    }
}

/// My List and watched, as round buttons of a hero's row, showing the state the
/// server's events send back, so a mark that fails leaves them as they were.
struct UserStateButtons: View {
    let work: WorkSummary
    let page: WorkPageState
    @Environment(ServerModel.self) private var server

    var body: some View {
        let marks = server.session.library.marks
        let onMyList = page.onMyList
        let played = page.played
        Button {
            if let myListId = page.myListId { marks.setOnList(listId: myListId, workId: work.id, on: !onMyList) }
        } label: {
            Image(systemName: Symbols.myList(onMyList))
                .accessibilityLabel(onMyList ? Text("Remove from Watchlist") : Text("Add to Watchlist"))
        }
        .buttonBorderShape(.circle)
        .disabled(page.myListId == nil)
        Button {
            marks.setPlayed(workId: work.id, played: !played)
        } label: {
            Image(systemName: Symbols.watched(played))
                .accessibilityLabel(Text(Words.markPlayed(!played, kind: work.kind)))
        }
        .buttonBorderShape(.circle)
    }
}

/// The ⋯ of a hero's row: Go to series for an episode, and Go to library.
struct MoreMenu: View {
    let work: WorkSummary
    @Environment(ServerModel.self) private var server
    @Environment(\.openWork) private var openWork

    var body: some View {
        let library = server.library(of: work.libraryId)
        if work.episode != nil || library != nil {
            Menu {
                if let episode = work.episode {
                    Button(String(localized: "Go to series"), systemImage: "tv") { openWork(WorkRoute(seriesOf: episode)) }
                }
                if let library {
                    Button(String(localized: "Go to library"), systemImage: "square.grid.2x2") { server.navigation.show(library) }
                }
            } label: {
                Image(systemName: Symbols.more)
                    .accessibilityLabel(Text("More"))
            }
            .buttonBorderShape(.circle)
        }
    }
}
