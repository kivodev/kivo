package com.mmcdole.kivo.android.mobile.detail

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.android.mobile.downloads.DownloadTarget
import com.mmcdole.kivo.android.mobile.downloads.DownloadUiState
import com.mmcdole.kivo.android.mobile.downloads.WorkDownloads
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.ProblemCode
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.data.SeasonRef
import com.mmcdole.kivo.core.session.KivoSession
import com.mmcdole.kivo.core.session.WorkPage
import com.mmcdole.kivo.core.session.WorkPageState
import com.mmcdole.kivo.core.settings.DownloadQuality
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn

data class DetailUiState(
    val page: WorkPageState,
    /** The work's download, and a season's or series' episodes to download; null on a device that keeps none. */
    val download: DownloadUiState? = null,
) {
    /**
     * Why the page shows a failure in place of the work: it couldn't be
     * read and nothing is known of it, or the server no longer has it.
     */
    val failure: RequestProblem? = page.problem?.takeIf { page.head == null || (page.work == null && it.knownCode == ProblemCode.NotFound) }
}

/**
 * A work's page: the core's [page], open on the season [openOn] names, as
 * Go to series asks, and on a device that keeps downloads, the work's
 * download, at the download quality [setting] unless another is chosen.
 */
class DetailViewModel(
    session: KivoSession,
    id: String,
    openOn: SeasonRef? = null,
    setting: Flow<DownloadQuality?> = flowOf(null),
) : ViewModel() {
    val page: WorkPage = session.library.workPage(id, openOn = openOn)

    val marks = session.library.marks

    /** The server's name, for a page that can't reach it. */
    val serverName: String = session.savedServer.value.name

    /** The work's download: a movie's, episode's, video's or book's, with the version and tracks chosen here. */
    val downloads: WorkDownloads? = session.downloads?.let { downloads ->
        val target = page.state.map { state ->
            state.work?.takeIf { it.kind != WorkKind.Series && it.kind != WorkKind.Season }?.let {
                DownloadTarget(state.choice, it.kind == WorkKind.Book)
            }
        }
        WorkDownloads(downloads, session.library, target, setting, viewModelScope)
    }

    val state: StateFlow<DetailUiState> = combine(page.state, downloads?.state ?: flowOf(null), ::DetailUiState)
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), DetailUiState(page.state.value))

    fun setPlayed(work: WorkSummary, played: Boolean) = marks.setPlayed(work.id, played)

    /** Adds [work] to My List or takes it off; the live event redraws the page. */
    fun setOnMyList(work: WorkSummary, on: Boolean) {
        page.state.value.myListId?.let { marks.setOnList(it, work.id, on) }
    }

    override fun onCleared() {
        page.close()
    }
}
