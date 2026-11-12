import KivoCore
import KivoUI
import SwiftUI

/// A work's page, by its kind: a movie or other video, a series with its
/// seasons and episodes, an episode with its season, or a book, as the
/// core's `WorkPage` has it. A season opens its series on that season. It
/// opens at once on what the card it was opened from, or the session, knows
/// of the work, and fills in as the page is read. A failure shows under the
/// page's hero, or in its place when nothing was known.
struct DetailView: View {
    let kind: WorkKind?
    @State private var model: WorkPageModel
    @Environment(ServerModel.self) private var server

    init(route: WorkRoute, server: ServerModel) {
        kind = route.kind
        _model = State(initialValue: WorkPageModel(route: route, session: server.session))
    }

    var body: some View {
        let page = model.state
        Group {
            if let head = page.head {
                content(head, page)
                    .environment(\.detailLoad, DetailLoad(loaded: page.work != nil, problem: refusal, retry: model.page.retry))
            } else if let refusal {
                DetailFailed(failure: refusal)
            } else if server.unreachable {
                UnreachableView()
            } else {
                DetailSkeleton(kind: kind)
            }
        }
        .environment(\.currentLibraryId, nil)
    }

    /// Why the work couldn't be read, when the server refused it, as for a deleted work.
    private var refusal: RequestProblem? {
        model.state.problem.flatMap { $0.isUnreachable ? nil : $0 }
    }

    @ViewBuilder private func content(_ head: WorkSummary, _ page: WorkPageState) -> some View {
        switch head.kind {
        case .series:
            SeriesDetail(series: head, page: page, model: model.page)
        case .book:
            BookDetail(work: head, page: page)
        case .movie, .episode, .extra, .video, .unknown:
            PlayableDetail(work: head, page: page, model: model.page)
        case .season:
            // The core shows a season's page as its series'.
            DetailSkeleton(kind: .series)
        }
    }
}

/// A work's page as the core keeps it, followed for its view, which closes it as it goes.
@MainActor
@Observable
final class WorkPageModel {
    @ObservationIgnored let page: WorkPage
    private let observed: Observed<WorkPageState>

    var state: WorkPageState { observed.value }

    init(route: WorkRoute, session: KivoSession) {
        page = session.library.workPage(
            id: route.id,
            card: route.seed,
            openOn: route.episode.map { SeasonRef(id: $0.seasonId, number: $0.seasonNumber) }
        )
        observed = Observed(page.state)
    }

    deinit {
        page.close()
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
    let kind: WorkKind?

    var body: some View {
        DetailPage(work: nil, art: nil, eyebrow: kind == .episode ? (label: "", open: {}) : nil, details: []) {
            ActionRowSkeleton(roundButtons: kind == .series ? 2 : 3)
                .actionRowInsets()
            OverviewSkeleton()
        } actions: {
            EmptyView()
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text("Loading…"))
    }
}
