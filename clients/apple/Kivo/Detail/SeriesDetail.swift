import KivoCore
import KivoUI
import SwiftUI

/// A series: Play or Resume of Next Up, with the episode's
/// title and runtime under the button and a round Play from Beginning
/// beside it once it's started, then the overview, the seasons,
/// then the season's episodes. In a regular width the seasons are a row of
/// pills, "S1" with its unwatched count, over the season's name and a check
/// that marks it watched or unwatched, and its episodes a shelf of cards
/// that opens on Next Up; in a compact one they're a "Season 1 ▾" menu,
/// each with its unwatched count or Watched, and Mark Season as Watched,
/// over a list of episode rows. It opens on the season the core's `WorkPage`
/// opens it on; Specials are last, as the server orders them. The ⋯ menu
/// downloads the season or the whole series.
struct SeriesDetail: View {
    /// What shows of the series: its page's summary once read.
    let series: WorkSummary
    let page: WorkPageState
    /// Where a season chosen goes.
    let model: WorkPage
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette
    @Environment(\.wide) private var wide
    @Environment(\.horizontalSizeClass) private var horizontalSizeClass
    @Environment(\.detailLoad) private var load
    @State private var confirmingPlayed = false
    @State private var confirmingCheck = false
    // The watched state a tap last asked for, by season, so the check
    // animates only when that answer arrives.
    @State private var askedSeasonPlayed: [String: Bool] = [:]
    @State private var downloading: EpisodesToDownload?

    /// What the download sheet is for.
    private enum EpisodesToDownload: Identifiable {
        case season(WorkSummary)
        case series

        var id: String {
            switch self {
            case let .season(season): season.id
            case .series: "series"
            }
        }
    }

    private var seasons: [WorkSummary] {
        page.seasons?.getOrNull() as? [WorkSummary] ?? []
    }

    private var seasonsRead: Bool {
        page.seasons?.getOrNull() != nil
    }

    /// Why the seasons couldn't be read, when the server refused them.
    private var seasonsProblem: RequestProblem? {
        page.seasons?.problemOrNull().flatMap { $0.isUnreachable ? nil : $0 }
    }

    /// The open season, once the seasons are read.
    private var season: WorkSummary? { page.season }

    private var seasonId: String? { page.season?.id }

    /// The episodes shown: the open season's, the last season's kept until the next's are read; nil until the first are.
    private var episodes: [WorkSummary]? {
        page.episodes?.getOrNull() as? [WorkSummary]
    }

    /// Why the open season's episodes couldn't be read, when the server refused them.
    private var episodesProblem: RequestProblem? {
        page.episodes?.problemOrNull().flatMap { $0.isUnreachable ? nil : $0 }
    }

    var body: some View {
        DetailPage(work: series, art: series.images.backdrop ?? series.images.poster, details: details) {
            actions
                .actionRowInsets()
            Overview(text: page.work?.overview)
            if let seasonsProblem {
                InlineFailure(title: String(localized: "Couldn't load the seasons."), problem: seasonsProblem, retry: model.retry)
            } else if !seasonsRead {
                seasonsSkeleton
            } else if !seasons.isEmpty, horizontalSizeClass == .regular {
                if seasons.count > 1 {
                    seasonPills
                        .detailRow(EdgeInsets(top: 20, leading: 0, bottom: 0, trailing: 0))
                }
                // Over the shelf, as wide as it is.
                seasonHeading
                    .detailRow(EdgeInsets(top: seasons.count > 1 ? 8 : 20, leading: 20, bottom: 4, trailing: 20))
            } else if !seasons.isEmpty {
                seasonMenu
                    // Over the episodes, as wide as they are.
                    .wideWidth(KivoTokens.Container.prose, wide: wide)
                    .wideWidth(.infinity, wide: wide)
                    .detailRow(EdgeInsets(top: 20, leading: 20, bottom: 4, trailing: 20))
            }
            if let episodesProblem {
                InlineFailure(title: String(localized: "Couldn't load this season."), problem: episodesProblem, retry: model.retry)
            } else if let episodes, horizontalSizeClass == .regular {
                EpisodeShelf(episodes: episodes, focus: series.series?.nextUp?.id)
                    // Each season's shelf starts at its own beginning, or its Next Up.
                    .id(page.episodesOf)
            } else if let episodes {
                ForEach(episodes, id: \.id) { episode in
                    EpisodeRow(episode: episode)
                }
            } else if seasonsProblem == nil, horizontalSizeClass == .regular {
                EpisodeShelfSkeleton()
            } else if seasonsProblem == nil {
                EpisodeRowSkeletons(count: (season ?? page.episodesOf.flatMap { server.session.library.known(id: $0) })?.season?.episodeCount?.intValue)
            }
            if let read = page.work {
                CardRow(title: String(localized: "More like this"), works: read.similar ?? [])
                TrailersRow(work: read)
                CastRow(credits: read.credits)
                CardRow(title: String(localized: "Extras"), works: read.otherExtras())
            }
            Color.clear.frame(height: KivoTokens.spacing * 6).detailRow()
        } actions: {
            UserStateMenuItems(work: series)
            GoToLibraryItem(libraryId: series.libraryId)
            Section {
                if let season {
                    Button {
                        downloading = .season(season)
                    } label: {
                        Label(String(localized: "Download \(season.title)"), systemImage: "arrow.down.circle")
                    }
                }
                Button {
                    downloading = .series
                } label: {
                    Label(String(localized: "Download series"), systemImage: "arrow.down.circle")
                }
            }
        }
        .sheet(item: $downloading) { what in
            switch what {
            case let .season(season):
                DownloadEpisodesSheet(title: String(localized: "Download \(season.title)")) {
                    try await episodeIds(of: [season.id])
                }
            case .series:
                DownloadEpisodesSheet(title: String(localized: "Download series")) {
                    try await episodeIds(of: seasons.map(\.id))
                }
            }
        }
    }

    /// The seasons' place while they're read: the pills, as many as the series has, in a regular width, and the season's heading.
    @ViewBuilder private var seasonsSkeleton: some View {
        let count = Int(series.series?.seasonCount?.intValue ?? 0)
        if horizontalSizeClass == .regular, count > 1 {
            HStack(spacing: KivoTokens.spacing * 2) {
                ForEach(0..<min(count, 12), id: \.self) { _ in
                    SkeletonBlock(width: 64, height: 36, radius: 18)
                }
            }
            .padding(.horizontal, 20)
            .frame(maxWidth: .infinity, alignment: .leading)
            .detailRow(EdgeInsets(top: 20, leading: 0, bottom: 0, trailing: 0))
        }
        Text(verbatim: " ")
            .font(.title3.weight(.bold))
            .skeletonLine(width: 120)
            .frame(minHeight: 44)
            .detailRow(EdgeInsets(top: count > 1 && horizontalSizeClass == .regular ? 8 : 20, leading: 20, bottom: 4, trailing: 20))
    }

    private var unplayedInSeason: Int {
        Int(season?.season?.unplayedCount ?? 0)
    }

    private var markSeasonTitle: String {
        String(localized: "Mark \(unplayedInSeason) episodes as watched?")
    }

    /// Marks the season watched, asking first when that's more than one episode.
    private func markSeasonWatched(confirming: Binding<Bool>) {
        if unplayedInSeason > 1 {
            confirming.wrappedValue = true
        } else {
            setSeasonPlayed(true)
        }
    }

    private func setSeasonPlayed(_ played: Bool) {
        guard let season else { return }
        Haptics.success()
        askedSeasonPlayed[season.id] = played
        server.session.library.marks.setPlayed(workId: season.id, played: played)
    }

    private var markSeasonButton: some View {
        Button(String(localized: "Mark as watched")) {
            setSeasonPlayed(true)
        }
    }

    /// The episodes of these seasons, in order, each season read once.
    private func episodeIds(of seasonIds: [String]) async throws -> [String] {
        var ids: [String] = []
        for seasonId in seasonIds {
            let episodes = await server.session.library.children(workId: seasonId).answer() as? [WorkSummary]
            ids += episodes?.map(\.id) ?? []
        }
        return ids
    }

    private var details: [String] {
        DetailWords.seriesDetails(series)
    }

    @ViewBuilder private var actions: some View {
        if !load.loaded, load.problem == nil {
            // Next Up comes with the series' page.
            ActionRowSkeleton(roundButtons: 2)
                .frame(maxWidth: .infinity, alignment: wide ? .leading : .center)
        } else if let nextUp = series.series?.nextUp {
            ActionRow(work: series, page: page, secondary: DetailWords.secondLine(nextUp: nextUp)) {
                PlayButton(label: DetailWords.play(nextUp: nextUp)) {
                    server.player.play(PlayChoice(workId: nextUp.id))
                }
                if nextUp.progress != nil {
                    RestartButton { server.player.play(PlayChoice(workId: nextUp.id), fromStart: true) }
                }
            }
        } else {
            HStack(spacing: KivoTokens.spacing * 3) { UserStateButtons(work: series, page: page) }
                .controlSize(.large)
                .frame(maxWidth: .infinity)
        }
    }

    private var seasonMenu: some View {
        HStack {
            Menu {
                Picker(String(localized: "Season"), selection: Binding(get: { seasonId }, set: { id in if let id { model.chooseSeason(id: id) } })) {
                    ForEach(seasons, id: \.id) { season in
                        Button {} label: {
                            Text(season.title)
                            Text(seasonState(season))
                        }
                        .tag(String?.some(season.id))
                    }
                }
                .pickerStyle(.inline)
                if let season, season.userState?.played == true {
                    Section {
                        Button {
                            setSeasonPlayed(false)
                        } label: {
                            Label(String(localized: "Mark season as unwatched"), systemImage: "checkmark.circle.fill")
                        }
                    }
                } else if let season, (season.season?.unplayedCount ?? 0) > 0 {
                    Section {
                        Button {
                            markSeasonWatched(confirming: $confirmingPlayed)
                        } label: {
                            Label(String(localized: "Mark season as watched"), systemImage: "checkmark.circle")
                        }
                    }
                }
            } label: {
                HStack(spacing: KivoTokens.spacing * 1.5) {
                    Text(season?.title ?? "")
                        .font(.title3.weight(.bold))
                    Image(systemName: "chevron.down")
                        .font(.footnote.weight(.bold))
                        .foregroundStyle(palette.textMuted)
                }
                .foregroundStyle(palette.text)
                .tapTarget()
            }
            .accessibilityIdentifier("season")
            // Not for the season the page opens on.
            .sensoryFeedback(.selection, trigger: seasonId) { old, _ in old != nil }
            // On the menu, so a popover points at it.
            .confirmationDialog(markSeasonTitle, isPresented: $confirmingPlayed, titleVisibility: .visible) {
                markSeasonButton
            }
            Spacer()
            episodeCount
        }
    }

    /// The seasons as pills that scroll sideways, the open one outlined in
    /// the accent and kept in view.
    private var seasonPills: some View {
        ScrollViewReader { proxy in
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: KivoTokens.spacing * 2) {
                    ForEach(seasons, id: \.id) { season in
                        seasonPill(season)
                    }
                }
                .padding(.horizontal, 20)
            }
            .scrollClipDisabled()
            // Not for the season the page opens on.
            .sensoryFeedback(.selection, trigger: seasonId) { old, _ in old != nil }
            .onAppear {
                if let seasonId { proxy.scrollTo(seasonId, anchor: .center) }
            }
        }
    }

    private func seasonPill(_ season: WorkSummary) -> some View {
        let selected = season.id == seasonId
        let unplayed = season.season?.unplayedCount ?? 0
        return Button {
            model.chooseSeason(id: season.id)
        } label: {
            HStack(spacing: KivoTokens.spacing * 1.5) {
                Text(seasonLabel(season))
                    .foregroundStyle(palette.text)
                if season.userState?.played == true {
                    Image(systemName: "checkmark")
                        .font(.caption.weight(.bold))
                        .foregroundStyle(palette.accent)
                } else if unplayed > 0 {
                    Text(String(unplayed))
                        .font(.footnote)
                        .foregroundStyle(palette.textMuted)
                }
            }
            .font(.subheadline.weight(.semibold))
            .padding(.horizontal, KivoTokens.spacing * 4)
            .frame(minHeight: 36)
            .background(palette.surfaceRaised, in: Capsule())
            .overlay {
                Capsule().strokeBorder(selected ? palette.accent : Color.clear, lineWidth: 1.5)
            }
            .contentShape(Capsule())
        }
        .buttonStyle(.plain)
        .id(season.id)
        .accessibilityLabel(Text(season.title))
        .accessibilityValue(Text(seasonState(season)))
        .accessibilityAddTraits(selected ? .isSelected : [])
        .accessibilityIdentifier("season-pill")
    }

    /// The open season's name, its check, and its episode count.
    private var seasonHeading: some View {
        HStack(spacing: KivoTokens.spacing) {
            Text(season?.title ?? "")
                .font(.title3.weight(.bold))
                .foregroundStyle(palette.text)
            if let season {
                let played = season.userState?.played == true
                Button {
                    if played {
                        setSeasonPlayed(false)
                    } else {
                        markSeasonWatched(confirming: $confirmingCheck)
                    }
                } label: {
                    Image(systemName: played ? "checkmark.circle.fill" : "checkmark.circle")
                        .font(.title3)
                        .foregroundStyle(played ? palette.accent : palette.textMuted)
                        .symbolReplace(animated: askedSeasonPlayed[season.id] == played)
                        .frame(minWidth: 44, minHeight: 44)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                // Beside the check, so the seasons above stay in view.
                .popover(isPresented: $confirmingCheck, arrowEdge: .leading) {
                    VStack(spacing: KivoTokens.spacing * 3) {
                        Text(markSeasonTitle)
                            .font(.subheadline)
                            .foregroundStyle(palette.textMuted)
                            .multilineTextAlignment(.center)
                        Button {
                            confirmingCheck = false
                            setSeasonPlayed(true)
                        } label: {
                            Text("Mark as watched").frame(maxWidth: .infinity)
                        }
                        .buttonStyle(.bordered)
                        .buttonBorderShape(.capsule)
                        .controlSize(.large)
                    }
                    .padding(KivoTokens.spacing * 4)
                    .frame(width: 280)
                    .presentationCompactAdaptation(.popover)
                }
                .accessibilityLabel(played ? Text("Mark season as unwatched") : Text("Mark season as watched"))
                .accessibilityIdentifier("season-watched")
            }
            Spacer()
            episodeCount
        }
    }

    @ViewBuilder private var episodeCount: some View {
        if let count = DetailWords.episodeCount(season) {
            Text(count)
                .font(.footnote)
                .foregroundStyle(palette.textMuted)
        }
    }

    private func seasonLabel(_ season: WorkSummary) -> String {
        DetailWords.seasonLabel(season)
    }

    private func seasonState(_ season: WorkSummary) -> String {
        DetailWords.seasonState(season)
    }
}
