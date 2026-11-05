import KivoCore
import KivoUI
import SwiftUI

/// A work's page, by its kind: a movie or other video, a series
/// with its seasons and episodes, an episode with its season, a book, or a
/// collection's members. A season opens its series on that season. Each
/// refetches when the live events say it changed. It opens at once on what
/// the card it was opened from, or the session, knows of the work, and fills
/// in as the page is read.
struct DetailView: View {
    let route: WorkRoute
    @Environment(ServerModel.self) private var server

    var body: some View {
        SeededPage(route: route, seed: route.seed ?? server.session.library.known(id: route.id))
            .id(route.id)
            .environment(\.currentLibraryId, nil)
    }
}

/// A page with what was known of its work as it opened, kept while it's open.
private struct SeededPage: View {
    let route: WorkRoute
    @State private var seed: WorkSummary?
    @Environment(ServerModel.self) private var server

    init(route: WorkRoute, seed: WorkSummary?) {
        self.route = route
        _seed = State(initialValue: seed)
    }

    var body: some View {
        if let seed, let season = seed.season {
            // A season's own page is its series', open on that season, read at once.
            WorkPage(
                id: season.seriesId,
                kind: "series",
                seed: server.session.library.known(id: season.seriesId).map(WorkDetail.init(summary:)) ?? WorkDetail(seriesOf: seed),
                seasonId: seed.id,
                episode: nil
            )
        } else {
            WorkPage(id: route.id, kind: route.kind, seed: seed.map(WorkDetail.init(summary:)), seasonId: nil, episode: route.episode)
        }
    }
}

/// A work's page read from the server, showing [seed] until it answers. A
/// failure shows under the page's hero, or in its place when nothing was
/// known.
private struct WorkPage: View {
    let id: String
    let kind: String?
    @State private var seed: WorkDetail?
    let seasonId: String?
    let episode: EpisodeInfo?
    @Environment(ServerModel.self) private var server
    @State private var work: WorkDetail?

    init(id: String, kind: String?, seed: WorkDetail?, seasonId: String?, episode: EpisodeInfo?) {
        self.id = id
        self.kind = kind
        _seed = State(initialValue: seed)
        self.seasonId = seasonId
        self.episode = episode
    }
    /// Why the work couldn't be read, when the server refused it, as for a deleted work.
    @State private var failure: RequestProblem?
    @State private var attempt = 0

    var body: some View {
        Group {
            if let shown = work ?? seed {
                page(shown)
                    .environment(\.detailLoad, DetailLoad(loaded: work != nil, problem: failure, retry: { attempt += 1 }))
            } else if let failure {
                DetailFailed(failure: failure)
            } else if server.unreachable {
                UnreachableView()
            } else {
                DetailSkeleton(kind: kind)
            }
        }
        .task(id: "\(id)/\(attempt)") {
            failure = nil
            for await load in server.session.library.work(id: id) {
                if let work = load.getOrNull() {
                    failure = nil
                    self.work = work
                } else if let problem = load.problemOrNull(), !problem.isUnreachable {
                    failure = problem
                }
            }
        }
    }

    @ViewBuilder private func page(_ work: WorkDetail) -> some View {
        switch work.kind {
        case "series":
            SeriesDetail(series: work, initialSeasonId: seasonId, episode: episode)
        case "season":
            // Read without its card: its series' page, once the season says which.
            if let season = work.season {
                WorkPage(id: season.seriesId, kind: "series", seed: WorkDetail(seriesOf: work.summary()), seasonId: work.id, episode: nil)
            }
        case "book":
            BookDetail(work: work)
        default:
            PlayableDetail(work: work)
        }
    }
}

/// How far a detail page's read has got: whether the page is read or shows
/// only what was known when it opened, and why the read failed, with Retry.
struct DetailLoad {
    var loaded = true
    var problem: RequestProblem?
    var retry: () -> Void = {}
}

extension EnvironmentValues {
    @Entry var detailLoad = DetailLoad()
}

/// A detail page the server refused, as for a work deleted or out of reach: what it said.
struct DetailFailed: View {
    let failure: RequestProblem

    var body: some View {
        MessageView(String(localized: "Couldn't load this page."), sentence: failure.sentence)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}

/// A read that failed, under what the page already shows: what went wrong, with Retry.
struct InlineFailure: View {
    let title: String
    let problem: RequestProblem
    let retry: () -> Void
    @Environment(\.kivoPalette) private var palette
    @Environment(\.wide) private var wide

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: KivoTokens.spacing * 3) {
            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(palette.text)
                Text(problem.sentence)
                    .font(.footnote)
                    .foregroundStyle(palette.textMuted)
            }
            Spacer(minLength: 0)
            Button(String(localized: "Retry"), action: retry)
                .buttonStyle(.bordered)
                .buttonBorderShape(.capsule)
                .accessibilityIdentifier("retry")
        }
        .wideWidth(KivoTokens.Container.prose, wide: wide)
        .detailRow()
    }
}

/// A detail page's skeleton, in the page's own frame: an episode's has its
/// series' line over the title, a series' two round buttons.
private struct DetailSkeleton: View {
    let kind: String?

    var body: some View {
        DetailPage(work: nil, art: nil, eyebrow: kind == "episode" ? (label: "", open: {}) : nil, details: []) {
            ActionRowSkeleton(roundButtons: kind == "series" ? 2 : 3)
                .actionRowInsets()
            OverviewSkeleton()
        } actions: {
            EmptyView()
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text("Loading…"))
    }
}
