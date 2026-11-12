package com.mmcdole.kivo.android.mobile.downloads

import com.mmcdole.kivo.android.mobile.ui.display
import com.mmcdole.kivo.android.mobile.ui.sentence
import com.mmcdole.kivo.android.quietly
import com.mmcdole.kivo.api.models.DownloadEstimate
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.downloads.DownloadItem
import com.mmcdole.kivo.core.downloads.downloadProblem
import com.mmcdole.kivo.core.play.PlayChoice
import com.mmcdole.kivo.core.session.SessionDownloads
import com.mmcdole.kivo.core.session.SessionLibrary
import com.mmcdole.kivo.core.settings.DownloadQuality
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** What a page's Download button downloads: a work, with the version and tracks chosen on its page; a book its original. */
data class DownloadTarget(val choice: PlayChoice, val book: Boolean)

/** A season's or a series' episodes to download: [season] null for the whole series, and their estimates once read. */
data class EpisodesSheet(val season: WorkSummary?, val estimates: List<DownloadEstimate>? = null) {
    /** The qualities on offer, as the first episode lists them. */
    fun qualities(wanted: List<DownloadEstimate>) = wanted.firstOrNull()?.options.orEmpty()
}

data class DownloadUiState(
    /** The page's work's download, once asked for. */
    val item: DownloadItem? = null,
    /** Asked of the server and not yet listed: the button waits. */
    val asking: Boolean = false,
    /** What downloading it would be at each quality, once the server says. */
    val estimate: DownloadEstimate? = null,
    /** The download quality setting, which a tap downloads at. */
    val setting: DownloadQuality? = null,
    val episodes: EpisodesSheet? = null,
    /** The works that have a download, so the episodes sheet leaves them as they are. */
    val downloaded: Set<String> = emptySet(),
    /** Why the last download couldn't start, until it's seen. */
    val problem: String? = null,
)

/**
 * A work's download for its page's view model: where its download is, what
 * each quality would come to, and downloading it, pausing, resuming,
 * retrying or removing it; and a season's or series' episodes at once. Each
 * runs on [scope]; a failure says why ([DownloadUiState.problem]).
 */
@OptIn(ExperimentalCoroutinesApi::class)
class WorkDownloads(
    private val downloads: SessionDownloads,
    private val library: SessionLibrary,
    target: Flow<DownloadTarget?>,
    setting: Flow<DownloadQuality?>,
    private val scope: CoroutineScope,
) {
    private val targets = target.distinctUntilChanged().stateIn(scope, SharingStarted.Eagerly, null)
    private val sheet = MutableStateFlow<EpisodesSheet?>(null)
    private val problem = MutableStateFlow<String?>(null)

    private val estimate: Flow<DownloadEstimate?> = targets.flatMapLatest { t ->
        if (t == null) {
            flowOf(null)
        } else {
            flow {
                emit(null)
                emit(quietly { downloads.estimates(listOf(t.choice)).firstOrNull() })
            }
        }
    }

    private val item = combine(downloads.items, targets) { items, t -> t?.let { items.firstOrNull { it.workId == t.choice.workId } } }
    private val asking = combine(downloads.asking, targets) { asking, t -> t != null && t.choice.workId in asking }
    private val downloaded = downloads.items.map { items -> items.map { it.workId }.toSet() }

    val state: StateFlow<DownloadUiState> = combine(
        combine(item, asking, estimate, ::Triple),
        setting,
        sheet,
        downloaded,
        problem,
    ) { (item, asking, estimate), setting, sheet, downloaded, problem ->
        DownloadUiState(item, asking, estimate, setting, sheet, downloaded, problem)
    }.stateIn(scope, SharingStarted.WhileSubscribed(5_000), DownloadUiState())

    /** Downloads the page's work at [quality], or the setting's when null; a book downloads its original. */
    fun download(quality: DownloadQuality? = null) {
        val t = targets.value ?: return
        act {
            display.downloadProblem(
                if (t.book) downloads.download(PlayChoice(t.choice.workId), DownloadQuality.Original) else downloads.download(t.choice, quality),
            )
        }
    }

    fun pause() = state.value.item?.let { act { downloads.pause(it.id).problemOrNull()?.let(::sentence) } }

    fun resume() = state.value.item?.let { act { downloads.resume(it.id).problemOrNull()?.let(::sentence) } }

    fun retry() = state.value.item?.let { act { display.downloadProblem(downloads.retry(it.id)) } }

    fun remove() = state.value.item?.let { act { downloads.delete(it.id).problemOrNull()?.let(::sentence) } }

    /** Opens the episodes sheet for [seasons]' episodes, [season] naming the one season or null for the whole series. */
    fun chooseEpisodes(season: WorkSummary?, seasons: List<WorkSummary>) {
        sheet.value = EpisodesSheet(season)
        scope.launch {
            try {
                val ids = mutableListOf<String>()
                for (s in if (season != null) listOf(season) else seasons) {
                    val read = library.children(s.id).first { it !is Load.Loading }
                    val episodes = read.getOrNull() ?: run {
                        sheet.value = null
                        problem.value = read.problemOrNull()?.let(::sentence)
                        return@launch
                    }
                    ids += episodes.map { it.id }
                }
                val estimates = downloads.estimates(ids.map { PlayChoice(it) })
                sheet.update { it?.copy(estimates = estimates) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                sheet.value = null
                problem.value = sentence(RequestProblem.of(e))
            }
        }
    }

    /** Downloads every episode in the sheet that can be and isn't downloaded already, at [quality]. */
    fun downloadEpisodes(quality: String) {
        val estimates = state.value.episodes?.estimates ?: return
        val downloaded = state.value.downloaded
        sheet.value = null
        act {
            estimates.filter { it.playable && it.workId !in downloaded }.firstNotNullOfOrNull { estimate ->
                display.downloadProblem(downloads.download(PlayChoice(estimate.workId), DownloadQuality.parseOrNull(quality)))
            }
        }
    }

    fun closeEpisodes() {
        sheet.value = null
    }

    fun problemSeen() {
        problem.value = null
    }

    /** Runs [call], showing the problem it words, when there's one. */
    private fun act(call: suspend () -> String?) {
        scope.launch { call()?.let { problem.value = it } }
    }
}
