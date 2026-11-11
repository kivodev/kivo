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
/// over a list of episode rows. It opens on the season asked for, or Go to series'
/// episode's, else the one holding Next Up, as the core's `Seasons.openSeason`
/// picks; Specials are last, as the server orders them. The ⋯ menu downloads the season or the whole series.
struct SeriesDetail: View {
    let series: WorkDetail
    /// The season asked for, by a season's card; its episodes are read beside the seasons.
    let initialSeasonId: String?
    /// The episode Go to series came from; its season's episodes are read beside the seasons.
    let episode: EpisodeInfo?
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette
    @Environment(\.wide) private var wide
    @Environment(\.horizontalSizeClass) private var horizontalSizeClass
    @Environment(\.detailLoad) private var load
    @State private var seasons: [WorkSummary] = []
    /// Until the seasons are read: whether they have been, and why they couldn't be.
    @State private var seasonsRead = false
    @State private var seasonsProblem: RequestProblem?
    @State private var seasonsAttempt = 0
    /// The season picked from the menu or a pill, or the one the page opened on, kept as Next Up moves.
    @State private var chosen: String?
    /// The open season's episodes, the last season's kept until the next's are read; nil until the first are.
    @State private var episodes: [WorkSummary]?
    @State private var episodesProblem: RequestProblem?
    @State private var episodesAttempt = 0
    @State private var confirmingPlayed = false
    @State private var confirmingCheck = false
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

    /// The open season: the one chosen, else, once the seasons are read,
    /// the one the page opens on, worked out from what's read so far each
    /// time the page is drawn rather than set when a read lands; before
    /// they're read, the one asked for, so its episodes are read beside them.
    private var seasonId: String? {
        if let chosen, seasons.contains(where: { $0.id == chosen }) { return chosen }
        guard seasonsRead else { return initialSeasonId ?? episode?.seasonId }
        return Seasons.shared.openSeason(
            seasons: seasons,
            seasonId: initialSeasonId,
            episode: episode,
            nextUp: series.series?.nextUp?.episode,
            seriesRead: load.loaded || load.problem != nil
        )?.id
    }

    private var season: WorkSummary? {
        seasons.first { $0.id == seasonId }
    }

    var body: some View {
        DetailPage(work: series, art: series.images.backdrop ?? series.images.poster, details: details) {
            actions
                .actionRowInsets()
            Overview(text: series.overview)
            if let seasonsProblem {
                InlineFailure(title: String(localized: "Couldn't load the seasons."), problem: seasonsProblem) { seasonsAttempt += 1 }
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
                InlineFailure(title: String(localized: "Couldn't load this season."), problem: episodesProblem) { episodesAttempt += 1 }
            } else if let episodes, horizontalSizeClass == .regular {
                EpisodeShelf(episodes: episodes, focus: series.series?.nextUp?.id)
                    // Each season's shelf starts at its own beginning, or its Next Up.
                    .id(seasonId)
            } else if let episodes {
                ForEach(episodes, id: \.id) { episode in
                    EpisodeRow(episode: episode)
                }
            } else if seasonsProblem == nil, horizontalSizeClass == .regular {
                EpisodeShelfSkeleton()
            } else if seasonsProblem == nil {
                EpisodeRowSkeletons(count: (season ?? seasonId.flatMap { server.session.library.known(id: $0) })?.season?.episodeCount?.intValue)
            }
            TrailersRow(work: series)
            CastRow(credits: series.credits)
            CardRow(title: String(localized: "Extras"), works: series.otherExtras())
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
        .task(id: "\(series.id)/\(seasonsAttempt)") {
            seasonsProblem = nil
            for await load in server.session.library.children(workId: series.id) {
                if let children = load.getOrNull() as? [WorkSummary] {
                    seasonsProblem = nil
                    seasons = children
                    seasonsRead = true
                } else if let problem = load.problemOrNull(), !problem.isUnreachable {
                    seasonsProblem = problem
                }
            }
        }
        // The season the page opened on stays open as Next Up moves on.
        .onChange(of: seasonId) { _, id in
            if seasonsRead, chosen == nil { chosen = id }
        }
        .task(id: "\(seasonId ?? "")/\(episodesAttempt)") {
            guard let seasonId else { return }
            episodesProblem = nil
            for await load in server.session.library.children(workId: seasonId) {
                if let children = load.getOrNull() as? [WorkSummary] {
                    episodesProblem = nil
                    episodes = children
                } else if let problem = load.problemOrNull(), !problem.isUnreachable {
                    episodesProblem = problem
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
        Task { _ = try? await server.session.library.setPlayed(workId: season.id, played: played) }
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

    /// "2015 to 2022 · 6 seasons · ★ 8.5 · TV-MA".
    private var details: [String] {
        var parts: [String] = []
        if let year = series.year?.intValue {
            if let last = series.series?.lastAirYear?.intValue, last != year {
                parts.append(String(localized: "\(String(year)) to \(String(last))"))
            } else {
                parts.append(String(year))
            }
        }
        if let count = series.series?.seasonCount?.intValue {
            parts.append(count == 1 ? String(localized: "1 season") : String(localized: "\(count) seasons"))
        }
        if let rating = series.communityRatingText { parts.append(rating) }
        if let content = series.contentRating { parts.append(content) }
        return parts
    }

    @ViewBuilder private var actions: some View {
        if !load.loaded, load.problem == nil {
            // Next Up comes with the series' page.
            ActionRowSkeleton(roundButtons: 2)
                .frame(maxWidth: .infinity, alignment: wide ? .leading : .center)
        } else if let nextUp = series.series?.nextUp {
            ActionRow(work: series, secondary: secondLine(nextUp)) {
                PlayButton(label: playLabel(nextUp)) {
                    server.player.play(PlayChoice(workId: nextUp.id))
                }
                if nextUp.progress != nil {
                    RestartButton { server.player.play(PlayChoice(workId: nextUp.id), fromStart: true) }
                }
            }
        } else {
            HStack(spacing: KivoTokens.spacing * 3) { UserStateButtons(work: series) }
                .controlSize(.large)
                .frame(maxWidth: .infinity)
        }
    }

    /// "Play S1 E1", or "Resume 12:03" for one in progress.
    private func playLabel(_ nextUp: WorkSummary) -> String {
        if nextUp.progress != nil, let position = nextUp.userState?.positionMs?.int64Value {
            return String(localized: "Resume \(Display.app.clock(ms: position))")
        }
        guard let episode = nextUp.episode else { return String(localized: "Play") }
        return String(localized: "Play S\(episode.seasonNumber) E\(episode.number)")
    }

    /// The episode's title and runtime, or the time left in it.
    private func secondLine(_ nextUp: WorkSummary) -> String {
        let time = nextUp.timeLeft ?? nextUp.durationMs.map { Display.app.runtime(ms: $0.int64Value) }
        return [nextUp.title, time].compactMap { $0 }.joined(separator: " · ")
    }

    private var seasonMenu: some View {
        HStack {
            Menu {
                Picker(String(localized: "Season"), selection: Binding(get: { seasonId }, set: { chosen = $0 })) {
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
            .sensoryFeedback(.selection, trigger: seasonId)
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
            .sensoryFeedback(.selection, trigger: seasonId)
            .onAppear {
                if let seasonId { proxy.scrollTo(seasonId, anchor: .center) }
            }
        }
    }

    private func seasonPill(_ season: WorkSummary) -> some View {
        let selected = season.id == seasonId
        let unplayed = season.season?.unplayedCount ?? 0
        return Button {
            chosen = season.id
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
                        .contentTransition(.symbolEffect(.replace))
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
        if let count = season?.season?.episodeCount?.intValue {
            Text(count == 1 ? String(localized: "1 episode") : String(localized: "\(count) episodes"))
                .font(.footnote)
                .foregroundStyle(palette.textMuted)
        }
    }

    /// "S1", or Specials for season 0, or the season's title without a number.
    private func seasonLabel(_ season: WorkSummary) -> String {
        guard let number = season.season?.number else { return season.title }
        return number == 0 ? String(localized: "Specials") : String(localized: "S\(number)")
    }

    /// "3 unwatched", or Watched.
    private func seasonState(_ season: WorkSummary) -> String {
        guard let info = season.season else { return "" }
        return info.unplayedCount == 0 ? String(localized: "Watched") : String(localized: "\(info.unplayedCount) unwatched")
    }
}
