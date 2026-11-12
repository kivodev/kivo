package com.mmcdole.kivo.core.session

import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.data.SeasonRef
import com.mmcdole.kivo.core.data.Seasons
import com.mmcdole.kivo.core.data.summary
import com.mmcdole.kivo.core.play.PlayChoice
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/**
 * A work's page as its screen shows it ([WorkPage.state]). A season's page
 * is its series', open on that season.
 */
@ConsistentCopyVisibility
data class WorkPageState internal constructor(
    /** What shows of the work at once: as read, else as its card or the session knew it; null while nothing is known. */
    val head: WorkSummary?,
    /** The work's page, once read. */
    val work: WorkDetail?,
    /** Why the page couldn't be read when it last tried; null once it is. */
    val problem: RequestProblem?,
    /** A series' seasons as the server orders them; null for any other work. */
    val seasons: Load<List<WorkSummary>>?,
    /** The season open on a series' page, once its seasons are read. */
    val season: WorkSummary?,
    /**
     * The open season's episodes, or an episode's own season's; null when
     * there are none to read. Another season's stay until the next one's
     * are read, so [episodesOf] says whose they are.
     */
    val episodes: Load<List<WorkSummary>>?,
    /** The season whose episodes [episodes] are. */
    val episodesOf: String?,
    /** The work with the version and tracks chosen here for its next play. */
    val choice: PlayChoice,
    /** My List's ID, once the lists are read. */
    val myListId: String?,
) {
    /** Whether the work is on My List, once its page and the lists are read. */
    val onMyList: Boolean = myListId != null && work?.listIds?.contains(myListId) == true

    /** Whether the work is marked played. */
    val played: Boolean = head?.userState?.played == true
}

/**
 * A work's page behind its screen ([SessionLibrary.workPage]): the work,
 * shown at once from its card or what the session read of it, then as
 * read; a series' seasons, the one open and its episodes; an episode's
 * season's episodes; the play choices; and whether it's on My List. Every
 * read follows the live events, so a mark made through [SessionLibrary.marks]
 * shows once the server has it.
 *
 * A season's page is its series', open on that season: one known as a
 * season reads its series at once, and one that isn't reads the season,
 * then its series. A series opens on the season [openOn] names, else on the
 * season holding Next Up, and stays on the season it opened on, or the one
 * chosen, as Next Up moves.
 *
 * Called on the main thread, as the core's work runs; [close] it when the
 * screen goes.
 */
class WorkPage internal constructor(
    id: String,
    card: WorkSummary?,
    openOn: SeasonRef?,
    private val reads: WorkPageReads,
    scope: CoroutineScope,
) {
    private val scope = CoroutineScope(scope.coroutineContext + SupervisorJob(scope.coroutineContext[Job]))

    /** The work whose page is read: a season's series. */
    private var target: String

    /** The season asked for, by Go to series or by opening a season. */
    private var asked: SeasonRef? = openOn

    /** The season chosen, or the one the page opened on. */
    private var chosen: String? = null

    private var workRead: Job? = null
    private var seasonsRead: Pair<String, Job>? = null
    private var episodesRead: Pair<String, Job>? = null

    private val mutableState: MutableStateFlow<WorkPageState>

    /** The page as its screen shows it. */
    val state: StateFlow<WorkPageState>

    init {
        val seed = card ?: reads.known(id)
        val season = seed?.season
        target = season?.seriesId ?: id
        if (seed != null && season != null) asked = SeasonRef(seed.id, season.number)
        val head = if (seed != null && season != null) seriesOf(seed) else seed
        mutableState = MutableStateFlow(
            WorkPageState(
                head = head,
                work = null,
                problem = null,
                seasons = null,
                season = null,
                episodes = null,
                episodesOf = null,
                choice = PlayChoice(target),
                myListId = null,
            ),
        )
        state = mutableState.asStateFlow()
        readWork()
        // What's known says which reads the page needs, so they start beside the work's.
        if (head?.kind == WorkKind.Series) readSeasons(target)
        (asked?.id ?: head?.episode?.seasonId)?.let(::readEpisodes)
        this.scope.launch { reads.myListId.collect { listId -> show { it.copy(myListId = listId) } } }
    }

    /** Opens the season [id] of a series' page, which then stays open. */
    fun chooseSeason(id: String) {
        chosen = id
        reopen()
    }

    /** Plays the version [id]: its tracks are its own, so the track choices go back to the server's. */
    fun chooseVersion(id: String) = show { it.copy(choice = PlayChoice(it.choice.workId, versionId = id)) }

    /** Plays the audio track [id], or the server's choice when null. */
    fun chooseAudio(id: String?) = show { it.copy(choice = it.choice.copy(audioTrackId = id)) }

    /** Plays the subtitle track [id], `off`, or the server's choice when null. */
    fun chooseSubtitle(id: String?) = show { it.copy(choice = it.choice.copy(subtitleTrackId = id)) }

    /** Asks the server again now, and reads again what failed, keeping what's shown meanwhile. */
    fun retry() {
        reads.retryNow()
        readWork()
        seasonsRead?.first?.let { seasonsRead = null; readSeasons(it) }
        episodesRead?.first?.let { episodesRead = null; readEpisodes(it) }
    }

    /** Stops reading: the screen is gone. */
    fun close() {
        scope.cancel()
    }

    private fun show(change: (WorkPageState) -> WorkPageState) {
        mutableState.value = change(mutableState.value)
    }

    private fun readWork() {
        workRead?.cancel()
        val id = target
        workRead = scope.launch {
            reads.work(id).collect { load ->
                when (load) {
                    is Load.Loaded -> read(load.value)
                    is Load.Failed -> {
                        show { it.copy(problem = load.problem) }
                        reopen()
                    }
                    Load.Loading -> Unit
                }
            }
        }
    }

    private fun read(work: WorkDetail) {
        val season = work.season
        if (work.kind == WorkKind.Season && season != null) {
            // Read without its card: its series' page, open on it.
            asked = SeasonRef(work.id, season.number)
            target = season.seriesId
            show { it.copy(head = seriesOf(work.summary()), choice = PlayChoice(target)) }
            readSeasons(target)
            readEpisodes(work.id)
            readWork()
            return
        }
        show { it.copy(head = work.summary(), work = work, problem = null) }
        when (work.kind) {
            WorkKind.Series -> readSeasons(work.id)
            WorkKind.Episode -> work.episode?.seasonId?.let(::readEpisodes)
            else -> Unit
        }
        reopen()
    }

    private fun readSeasons(seriesId: String) {
        if (seasonsRead?.first == seriesId) return
        seasonsRead?.second?.cancel()
        if (state.value.seasons == null) show { it.copy(seasons = Load.Loading) }
        seasonsRead = seriesId to scope.launch {
            reads.children(seriesId).collect { load ->
                if (load == Load.Loading) return@collect
                show { it.copy(seasons = load) }
                reopen()
            }
        }
    }

    /**
     * Opens the season chosen, else, once the seasons are read, the one the
     * page opens on, which then stays; and reads its episodes.
     */
    private fun reopen() {
        val now = state.value
        val seasons = now.seasons?.getOrNull() ?: return
        val open = chosen?.let { id -> seasons.firstOrNull { it.id == id } }
            ?: Seasons.openSeason(seasons, asked, now.work?.series?.nextUp?.episode, seriesRead = now.work != null || now.problem != null)
        if (open != null && chosen == null) chosen = open.id
        if (open != now.season) show { it.copy(season = open) }
        open?.let { readEpisodes(it.id) }
    }

    private fun readEpisodes(seasonId: String) {
        if (episodesRead?.first == seasonId) return
        episodesRead?.second?.cancel()
        if (state.value.episodes == null) show { it.copy(episodes = Load.Loading, episodesOf = seasonId) }
        episodesRead = seasonId to scope.launch {
            reads.children(seasonId).collect { load ->
                if (load != Load.Loading) show { it.copy(episodes = load, episodesOf = seasonId) }
            }
        }
    }

    /**
     * A season's series, as known, else as much as the season's card says
     * of it: its title, and the season's backdrop and logo, which are its
     * series' where it has none of its own.
     */
    private fun seriesOf(season: WorkSummary): WorkSummary {
        val info = season.season ?: return season
        return reads.known(info.seriesId) ?: WorkSummary(
            id = info.seriesId,
            kind = WorkKind.Series,
            title = info.seriesTitle,
            matchState = season.matchState,
            available = season.available,
            images = WorkImages(backdrop = season.images.backdrop, logo = season.images.logo),
            contentRating = season.contentRating,
            libraryId = season.libraryId,
        )
    }
}

/** What a [WorkPage] reads, from the session's library. */
internal class WorkPageReads(
    val work: (String) -> Flow<Load<WorkDetail>>,
    val children: (String) -> Flow<Load<List<WorkSummary>>>,
    val known: (String) -> WorkSummary?,
    val myListId: Flow<String?>,
    val retryNow: () -> Unit,
)
