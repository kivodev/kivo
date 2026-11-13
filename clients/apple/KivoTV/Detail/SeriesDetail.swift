import KivoCore
import KivoUI
import SwiftUI

/// A series: its title, details and overview; Play or Resume of Next Up,
/// with Play from beginning beside it once it's started, My List, watched
/// and ⋯, and under them the time left in Next Up once it's started,
/// "30 min left"; then the seasons as tabs, the open one lit, and the open season's
/// episodes as a row of cards that opens on Next Up; then more like this, trailers, cast and
/// extras. It opens on the season the core's `WorkPage` opens it on.
struct SeriesDetail: View {
    let series: WorkSummary
    let page: WorkPageState
    let model: WorkPage
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette
    /// Whether Play has focus, which it takes as the page opens.
    @FocusState private var playFocused: Bool

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
                        .focused($playFocused)
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
                Text(series.series?.nextUp?.timeLeft ?? " ")
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
        .defaultFocus($playFocused, true)
    }

    /// The seasons as tabs, "Season 1" with a check once watched, the open one lit.
    private var seasonPills: some View {
        ScrollView(.horizontal) {
            HStack(spacing: KivoTokens.spacing * 6) {
                ForEach(seasons, id: \.id) { season in
                    Button {
                        model.chooseSeason(id: season.id)
                    } label: {
                        HStack(spacing: KivoTokens.spacing * 2) {
                            Text(Self.seasonName(season))
                            if season.userState?.played == true {
                                Image(systemName: "checkmark")
                            }
                        }
                    }
                    .buttonStyle(TabStyle(current: season.id == page.season?.id))
                    .accessibilityLabel(Text(season.title))
                    .accessibilityValue(Text(DetailWords.seasonState(season)))
                }
            }
        }
        .scrollClipDisabled()
        .focusSection()
    }

    /// "Season 3", or Specials for season 0, or the season's title without a number.
    private static func seasonName(_ season: WorkSummary) -> String {
        guard let number = season.season?.number else { return season.title }
        return number == 0 ? String(localized: "Specials") : String(localized: "Season \(number)")
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
            // The count ends over the fourth card's right edge.
            .frame(width: TVLayout.episodeWidth * 4 + TVLayout.cardGap * 3)
            ScrollViewReader { scroller in
                ScrollView(.horizontal) {
                    LazyHStack(alignment: .top, spacing: TVLayout.cardGap) {
                        if let episodes {
                            // A row with no synopses keeps its cards a line shorter.
                            let lines = episodes.contains { $0.synopsis != nil } ? 2 : 1
                            ForEach(episodes, id: \.id) { episode in
                                EpisodeLockup(episode: episode, lines: lines)
                                    .id(episode.id)
                            }
                        } else {
                            ForEach(0..<4, id: \.self) { _ in
                                SkeletonCard(style: .landscape, width: TVLayout.episodeWidth)
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
/// its runtime, or the time left in the accent, running into [lines] lines
/// of its synopsis.
struct EpisodeLockup: View {
    let episode: WorkSummary
    var lines = 1
    @Environment(\.openWork) private var openWork
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        Button {
            openWork(WorkRoute(episode))
        } label: {
            VStack(alignment: .leading, spacing: KivoTokens.spacing) {
                ZStack(alignment: .bottom) {
                    ArtworkImage(episode.images.still ?? episode.images.backdrop, title: episode.title, pointWidth: TVLayout.episodeWidth)
                    if let progress = episode.progress {
                        ProgressBar(value: progress)
                    }
                }
                .frame(width: TVLayout.episodeWidth, height: TVLayout.episodeWidth / CardStyle.landscape.aspect)
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
                    detail
                        .kivoStyle(KivoTokens.TextSize.xs)
                        .foregroundStyle(palette.textMuted)
                        .lineLimit(lines, reservesSpace: true)
                        .multilineTextAlignment(.leading)
                }
                .frame(width: TVLayout.episodeWidth, alignment: .leading)
            }
        }
        .buttonStyle(.borderless)
        .contextMenu { WorkMenu(work: episode) }
    }

    /// The runtime, or the time left in the accent, then the synopsis.
    private var detail: Text {
        let time = (episode.timeLeft ?? episode.durationMs.map { Display.app.runtime(ms: $0.int64Value) }).map {
            Text(verbatim: $0)
                .fontWeight(KivoTokens.Weight.medium)
                .foregroundStyle(episode.timeLeft != nil ? palette.accent : palette.text)
        }
        switch (time, episode.synopsis) {
        case let (time?, synopsis?): return Text("\(time) · \(synopsis)")
        case let (time?, nil): return time
        case let (nil, synopsis?): return Text(verbatim: synopsis)
        case (nil, nil): return Text(verbatim: " ")
        }
    }
}
