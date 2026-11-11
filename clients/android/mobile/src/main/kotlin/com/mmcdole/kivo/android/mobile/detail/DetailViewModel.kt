package com.mmcdole.kivo.android.mobile.detail

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.android.mobile.downloads.DownloadTarget
import com.mmcdole.kivo.android.mobile.downloads.DownloadUiState
import com.mmcdole.kivo.android.mobile.downloads.WorkDownloads
import com.mmcdole.kivo.android.mobile.ui.WorkMarks
import com.mmcdole.kivo.api.models.EpisodeInfo
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.data.Seasons
import com.mmcdole.kivo.core.data.summary
import com.mmcdole.kivo.core.session.KivoSession
import com.mmcdole.kivo.core.settings.DownloadQuality
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.onEach
import kotlinx.coroutines.flow.onStart
import kotlinx.coroutines.flow.shareIn
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update

/** The version and tracks chosen on a work's page for its next play; null for the server's choice. */
data class TrackChoices(val versionId: String? = null, val audioTrackId: String? = null, val subtitleTrackId: String? = null)

/** Whether the work's page says it's on My List. */
val DetailUiState.onMyList: Boolean
    get() = myListId != null && work?.listIds?.contains(myListId) == true

data class DetailUiState(
    /** What's shown of the work: its page once read, else what a list read of it. */
    val head: WorkSummary? = null,
    /** The work's page, once read. */
    val work: WorkDetail? = null,
    /** Why the page couldn't be read. */
    val problem: RequestProblem? = null,
    /** A series' seasons, once read. */
    val seasons: List<WorkSummary>? = null,
    /** The season a series' page shows. */
    val season: WorkSummary? = null,
    /** The episodes of that season, or of an episode's own season. */
    val episodes: Load<List<WorkSummary>>? = null,
    val choices: TrackChoices = TrackChoices(),
    /** The work's download, and a season's or series' episodes to download; null on a device that keeps none. */
    val download: DownloadUiState? = null,
    /** My List, once the lists are read. */
    val myListId: String? = null,
)

/**
 * A work's page: the work, read again as the live events say, shown at
 * once from what a list read of it; a series' seasons and the episodes of
 * the one open, or an episode's season's episodes. A season's page is its
 * series', open on that season: one known as a season reads its series at
 * once, and one that isn't reads the season, then its series. Go to series
 * asks for the season [seasonId] or [seasonNumber] names. On a device that
 * keeps downloads, the work's download, at the download quality [setting]
 * unless another is chosen.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class DetailViewModel(
    private val session: KivoSession,
    id: String,
    seasonId: String? = null,
    seasonNumber: Int? = null,
    setting: Flow<DownloadQuality?> = flowOf(null),
) : ViewModel() {
    private val library = session.library
    private val known = library.known(id)

    val marks = WorkMarks(library, viewModelScope)

    /** The server's name, for a page that can't reach it. */
    val serverName: String = session.savedServer.value.name

    /** The work whose page shows: a season's series. */
    private val target = MutableStateFlow(known?.season?.seriesId ?: id)

    /** The season asked for, by Go to series or by opening a season. */
    private val askedSeason = MutableStateFlow(seasonId ?: id.takeIf { known?.season != null })
    private val askedEpisode = seasonNumber?.let { EpisodeInfo(seasonNumber = it, number = 0, seriesId = id, seriesTitle = "", seasonId = seasonId) }

    /** My List, once the lists are read. */
    private val myListId: Flow<String?> = library.lists()
        .map { load -> load.getOrNull()?.firstOrNull { it.builtIn }?.id }
        .onStart { emit(null) }
        .distinctUntilChanged()

    /** The season the user chose, which sticks. */
    private val chosenSeason = MutableStateFlow<String?>(null)
    private val choices = MutableStateFlow(TrackChoices())
    private val retries = MutableStateFlow(0)

    private val work: Flow<Load<WorkDetail>> = combine(target, retries) { t, _ -> t }
        .flatMapLatest { library.work(it).onStart { emit(Load.Loading) } }
        .onEach { load ->
            val read = load.getOrNull()
            val season = read?.season
            if (read?.kind == WorkKind.Season && season != null) {
                askedSeason.value = read.id
                target.value = season.seriesId
            }
        }
        .shareIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), replay = 1)

    private val seasons: Flow<Load<List<WorkSummary>>?> = work
        .map { load -> load.getOrNull()?.takeIf { it.kind == WorkKind.Series }?.id }
        .distinctUntilChanged()
        .flatMapLatest { series -> if (series == null) flowOf(null) else library.children(series).onStart { emit(Load.Loading) } }
        .shareIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), replay = 1)

    private val openSeason: Flow<WorkSummary?> = combine(seasons, askedSeason, chosenSeason, work) { seasons, asked, chosen, work ->
        val list = seasons?.getOrNull() ?: return@combine null
        list.firstOrNull { it.id == chosen }
            ?: Seasons.openSeason(list, asked, askedEpisode, work.getOrNull()?.series?.nextUp?.episode, seriesRead = work !is Load.Loading)
    }.distinctUntilChanged()

    private val episodes: Flow<Load<List<WorkSummary>>?> = combine(
        openSeason.map { it?.id },
        work.map { load -> load.getOrNull()?.takeIf { it.kind == WorkKind.Episode }?.episode?.seasonId },
    ) { season, episodeSeason -> season ?: episodeSeason }
        .distinctUntilChanged()
        .flatMapLatest { season -> if (season == null) flowOf(null) else library.children(season).onStart { emit(Load.Loading) } }

    /** The work's download: a movie's, episode's, video's or book's, with the version and tracks chosen here. */
    val downloads: WorkDownloads? = session.downloads?.let { downloads ->
        val target = combine(work, choices) { load, c ->
            load.getOrNull()?.takeIf { it.kind != WorkKind.Series && it.kind != WorkKind.Season }?.let {
                DownloadTarget(it.id, it.kind == WorkKind.Book, c.versionId, c.audioTrackId, c.subtitleTrackId)
            }
        }
        WorkDownloads(downloads, library, target, setting, viewModelScope)
    }

    /** What a list read of the page's work: a season's series as its card names it. */
    private fun seed(): WorkSummary? = known?.season?.let { library.known(it.seriesId) } ?: known?.takeIf { it.season == null }

    val state: StateFlow<DetailUiState> = combine(
        combine(work, seasons, openSeason, episodes, choices) { work, seasons, season, episodes, choices ->
            val read = work.getOrNull()?.takeIf { it.kind != WorkKind.Season }
            DetailUiState(
                head = read?.summary() ?: seed(),
                work = read,
                problem = work.problemOrNull(),
                seasons = seasons?.getOrNull(),
                season = season,
                episodes = episodes,
                choices = choices,
            )
        },
        downloads?.state ?: flowOf(null),
        myListId,
    ) { state, download, myListId -> state.copy(download = download, myListId = myListId) }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), DetailUiState(head = seed()))

    fun chooseSeason(id: String) {
        chosenSeason.value = id
    }

    /** Another version: its tracks are its own, so the track choices go back to the server's. */
    fun chooseVersion(id: String) {
        choices.value = TrackChoices(versionId = id)
    }

    fun chooseAudio(id: String?) = choices.update { it.copy(audioTrackId = id) }

    fun chooseSubtitle(id: String?) = choices.update { it.copy(subtitleTrackId = id) }

    fun setPlayed(work: WorkSummary, played: Boolean) = marks.setPlayed(work, played)

    /** Adds [work] to My List or takes it off; the live event redraws the page. */
    fun setOnMyList(work: WorkSummary, on: Boolean) {
        state.value.myListId?.let { marks.setOnList(it, work.id, on) }
    }

    fun retry() {
        session.retryNow()
        retries.value++
    }
}
