package com.mmcdole.kivo.android.mobile.downloads

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.android.mobile.ui.display
import com.mmcdole.kivo.android.mobile.ui.sentence
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.core.downloads.DownloadItem
import com.mmcdole.kivo.core.downloads.DownloadStorage
import com.mmcdole.kivo.core.downloads.downloadProblem
import com.mmcdole.kivo.core.session.SessionDownloads
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.mapLatest
import kotlinx.coroutines.flow.onStart
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

/** A show's downloaded episodes, as one row of Downloads. */
data class ShowDownloads(val seriesId: String, val title: String, val episodes: List<DownloadItem>)

/** A season's downloaded episodes on a show's page; season 0 holds the specials. */
data class SeasonDownloads(val season: Int, val episodes: List<DownloadItem>)

data class DownloadsUiState(
    /** Whether the list has been read. */
    val read: Boolean = false,
    val books: List<DownloadItem> = emptyList(),
    val movies: List<DownloadItem> = emptyList(),
    /** Shows, the one with the newest download first. */
    val shows: List<ShowDownloads> = emptyList(),
    /** A show's page: its episodes by season and number. */
    val seasons: List<SeasonDownloads> = emptyList(),
    val storage: DownloadStorage? = null,
    /** How many downloads the checks deleted as no longer available, until the list goes. */
    val deleted: Int = 0,
    /** Why the last action failed, until it's seen. */
    val problem: String? = null,
) {
    val empty: Boolean get() = read && books.isEmpty() && movies.isEmpty() && shows.isEmpty() && seasons.isEmpty()
}

/**
 * Downloads, or with [seriesId] one show's page of them: books, movies and
 * shows, newest first, each with its state as the core works it out, the
 * space they take and what's free, and pausing, resuming, retrying and
 * deleting them.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class DownloadsViewModel(private val downloads: SessionDownloads, private val seriesId: String? = null) : ViewModel() {
    private val problem = MutableStateFlow<String?>(null)

    private val items = downloads.items.map { all -> if (seriesId == null) all else all.filter { it.seriesId == seriesId } }

    /** The space taken, read again as downloads come, go or change state. */
    private val storage = items.map { list -> list.map { it.id to it.status::class } }
        .distinctUntilChanged()
        .mapLatest { quietly { downloads.storage() } }
        .onStart { emit(null) }

    val state: StateFlow<DownloadsUiState> = combine(items, storage, downloads.deleted, problem) { items, storage, deleted, problem ->
        val episodes = items.filter { it.seriesId != null }
        DownloadsUiState(
            read = true,
            books = items.filter { seriesId == null && it.kind == WorkKind.Book },
            movies = items.filter { seriesId == null && it.kind != WorkKind.Book && it.seriesId == null },
            shows = if (seriesId != null) {
                emptyList()
            } else {
                episodes.groupBy { it.seriesId!! }
                    .map { (id, list) -> ShowDownloads(id, list.first().seriesTitle.orEmpty(), list) }
                    .sortedByDescending { show -> show.episodes.maxOf { it.createdAtMs } }
            },
            seasons = if (seriesId == null) {
                emptyList()
            } else {
                episodes.sortedWith(compareBy({ it.season ?: 0 }, { it.episode ?: 0 }))
                    .groupBy { it.season ?: 0 }
                    .map { (season, list) -> SeasonDownloads(season, list) }
            },
            storage = storage,
            deleted = deleted,
            problem = problem,
        )
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), DownloadsUiState())

    fun delete(ids: List<String>) = act { downloads.delete(ids).problemOrNull()?.let(::sentence) }

    fun pause(id: String) = act { downloads.pause(id).problemOrNull()?.let(::sentence) }

    fun resume(id: String) = act { downloads.resume(id).problemOrNull()?.let(::sentence) }

    fun retry(id: String) = act { display.downloadProblem(downloads.retry(id)) }

    fun problemSeen() {
        problem.value = null
    }

    /** The list has said how many downloads the checks deleted. */
    fun deletionsSeen() {
        if (state.value.deleted > 0) downloads.deletionsSeen()
    }

    /** Runs [call], showing the problem it words, when there's one. */
    private fun act(call: suspend () -> String?) {
        viewModelScope.launch { call()?.let { problem.value = it } }
    }
}

/** [read]'s answer; null when it fails. */
internal suspend fun <T> quietly(read: suspend () -> T): T? = try {
    read()
} catch (e: CancellationException) {
    throw e
} catch (_: Exception) {
    null
}
