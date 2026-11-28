import KivoCore
import Observation

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
