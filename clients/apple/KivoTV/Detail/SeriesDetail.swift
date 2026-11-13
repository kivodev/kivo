import KivoCore
import KivoUI
import SwiftUI

/// A series: its title, details and overview; Play or Resume of Next Up,
/// with Play from beginning beside it once it's started and the episode's
/// title and runtime under it, My List, watched and ⋯; then the seasons as
/// pills, the open one outlined, and the open season's episodes as a row of
/// cards that opens on Next Up; then more like this, trailers, cast and
/// extras. It opens on the season the core's `WorkPage` opens it on.
struct SeriesDetail: View {
    let series: WorkSummary
    let page: WorkPageState
    let model: WorkPage
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette

    private var seasons: [WorkSummary] {
        page.seasons?.getOrNull() as? [WorkSummary] ?? []
    }

    var body: some View {
        DetailPage(art: series.images.backdrop ?? series.images.poster) {
            VStack(alignment: .leading, spacing: KivoTokens.spacing * 6) {
                HeroTitle(work: series, details: DetailWords.seriesDetails(series), overview: page.work.map { $0.overview } ?? " ")
                HStack(spacing: KivoTokens.spacing * 6) {
                    if let nextUp = series.series?.nextUp {
                        Button {
                            server.play(nextUp.id)
                        } label: {
                            Label(DetailWords.play(nextUp: nextUp), systemImage: Symbols.play)
                        }
                        if nextUp.progress != nil {
                            Button {
                                server.play(nextUp.id, fromStart: true)
                            } label: {
                                Image(systemName: Symbols.playFromBeginning)
                                    .accessibilityLabel(Text("Play from beginning"))
                            }
                            .buttonBorderShape(.circle)
                        }
                    }
                    UserStateButtons(work: series, page: page)
                    MoreMenu(work: series)
                }
                Text(series.series?.nextUp.map(DetailWords.secondLine(nextUp:)) ?? " ")
                    .kivoStyle(KivoTokens.TextSize.xs)
                    .foregroundStyle(palette.textMuted)
            }
        } rows: {
            if seasons.count > 1 {
                seasonPills
            }
            SeasonRow(
                title: page.season?.title ?? " ",
                detail: DetailWords.episodeCount(page.season),
                episodes: page.episodes?.getOrNull() as? [WorkSummary],
                focus: series.series?.nextUp?.id
            )
            .id(page.episodesOf)
            if let read = page.work {
                DetailRows(work: read, collections: false)
            }
        }
    }

    /// The seasons as pills, "S1" with its unwatched count or a check, the open one outlined in the accent.
    private var seasonPills: some View {
        ScrollView(.horizontal) {
            HStack(spacing: KivoTokens.spacing * 6) {
                ForEach(seasons, id: \.id) { season in
                    Button {
                        model.chooseSeason(id: season.id)
                    } label: {
                        HStack(spacing: KivoTokens.spacing * 2) {
                            Text(DetailWords.seasonLabel(season))
                            if season.userState?.played == true {
                                Image(systemName: "checkmark")
                            } else if let unplayed = season.season?.unplayedCount, unplayed > 0 {
                                Text(String(unplayed)).foregroundStyle(.secondary)
                            }
                        }
                    }
                    .overlay {
                        Capsule().strokeBorder(season.id == page.season?.id ? palette.accent : .clear, lineWidth: 3)
                    }
                    .buttonBorderShape(.capsule)
                    .accessibilityLabel(Text(season.title))
                    .accessibilityValue(Text(DetailWords.seasonState(season)))
                }
            }
        }
        .scrollClipDisabled()
        .focusSection()
    }
}

/// A season's episodes as a row of cards under its title, opening on the
/// episode [focus], such as Next Up or the episode whose page it's on.
/// Placeholders hold their place until they're read.
struct SeasonRow: View {
    let title: String
    var detail: String?
    let episodes: [WorkSummary]?
    let focus: String?
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        VStack(alignment: .leading, spacing: TVLayout.titleGap) {
            HStack(alignment: .firstTextBaseline) {
                Text(title)
                    .kivoStyle(KivoTokens.TextSize.lg, weight: KivoTokens.Weight.semibold)
                    .foregroundStyle(palette.text)
                Spacer()
                if let detail {
                    Text(detail)
                        .kivoStyle(KivoTokens.TextSize.xs)
                        .foregroundStyle(palette.textMuted)
                }
            }
            ScrollViewReader { scroller in
                ScrollView(.horizontal) {
                    LazyHStack(alignment: .top, spacing: TVLayout.cardGap) {
                        if let episodes {
                            ForEach(episodes, id: \.id) { episode in
                                EpisodeLockup(episode: episode)
                                    .id(episode.id)
                            }
                        } else {
                            ForEach(0..<4, id: \.self) { _ in
                                SkeletonCard(style: .landscape, width: EpisodeLockup.width)
                            }
                        }
                    }
                }
                .scrollClipDisabled()
                .onAppear {
                    if let focus { scroller.scrollTo(focus, anchor: .leading) }
                }
            }
        }
        .focusSection()
    }
}

/// An episode's card in its season's row: its still, with how far through
/// it the user is or a check once watched, then its number and title, and
/// its runtime or the time left.
struct EpisodeLockup: View {
    static let width: CGFloat = 400
    let episode: WorkSummary
    @Environment(\.openWork) private var openWork
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        Button {
            openWork(WorkRoute(episode))
        } label: {
            VStack(alignment: .leading, spacing: KivoTokens.spacing * 2) {
                ZStack(alignment: .bottom) {
                    ArtworkImage(episode.images.still ?? episode.images.backdrop, title: episode.title, pointWidth: Self.width)
                    if let progress = episode.progress {
                        ProgressBar(value: progress)
                    }
                }
                .frame(width: Self.width, height: Self.width / CardStyle.landscape.aspect)
                .overlay(alignment: .topTrailing) {
                    if episode.userState?.played == true {
                        PlayedCheck().padding(KivoTokens.spacing * 2)
                    }
                }
                .clipShape(RoundedRectangle(cornerRadius: KivoTokens.Radius.md))
                .background(palette.placeholder, in: RoundedRectangle(cornerRadius: KivoTokens.Radius.md))
                .cardLift()
                VStack(alignment: .leading, spacing: 0) {
                    Text(episode.episode.map { "\($0.number). \(episode.title)" } ?? episode.title)
                        .kivoStyle(KivoTokens.TextSize.sm, weight: KivoTokens.Weight.medium)
                        .foregroundStyle(palette.text)
                        .lineLimit(1)
                    Text(episode.timeLeft ?? episode.durationMs.map { Display.app.runtime(ms: $0.int64Value) } ?? " ")
                        .kivoStyle(KivoTokens.TextSize.xs)
                        .foregroundStyle(palette.textMuted)
                        .lineLimit(1)
                }
                .frame(width: Self.width, alignment: .leading)
            }
        }
        .buttonStyle(.borderless)
        .contextMenu { WorkMenu(work: episode) }
    }
}
