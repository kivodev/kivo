package com.mmcdole.kivo.android.detail

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.ui.Loader
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkSummary
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn

/** Whether the page's work, once read, is on the list [listId]. */
internal fun WorkDetail?.onList(listId: String?): Boolean = listId != null && this?.listIds?.contains(listId) == true

/**
 * A detail page's reads: the work, and the works it lists (a series'
 * seasons or a season's episodes), each refetched
 * when live events say it changed. A season's page is its series' page:
 * one known as a season reads its series at once, and one that isn't reads
 * the season first, then its series.
 */
class DetailViewModel(private val session: ServerSession, id: String) : ViewModel() {
    private val card = session.session.library.known(id)

    /** The series whose page a season known as one opens, on that season; null for any other work. */
    val seasonOf: String? = card?.season?.seriesId

    /**
     * What's known of the page's work as it opens, from a list or a page
     * read earlier, which shows at once while [work] is read; null when
     * nothing is. A season's is its series', as its card names it.
     */
    val seed: WorkSummary? = card?.let { if (it.season != null) seriesSeed(it) else it }

    val work = Loader(viewModelScope) { session.session.library.work(seasonOf ?: id) }

    /** What's known of [season]'s series: its own card, else what the season's card says of it. */
    fun seriesSeed(season: WorkSummary): WorkSummary? {
        val info = season.season ?: return null
        return session.session.library.known(info.seriesId) ?: WorkSummary(
            id = info.seriesId,
            kind = WorkKind.Series,
            title = info.seriesTitle,
            matchState = season.matchState,
            available = season.available,
            // A season's card without art of its own has its series'.
            images = WorkImages(backdrop = season.images.backdrop, logo = season.images.logo),
        )
    }

    private val children = HashMap<String, Loader<List<WorkSummary>>>()
    private val others = HashMap<String, Loader<WorkDetail>>()

    fun children(workId: String): Loader<List<WorkSummary>> =
        children.getOrPut(workId) { Loader(viewModelScope) { session.session.library.children(workId) } }

    fun other(workId: String): Loader<WorkDetail> = others.getOrPut(workId) { Loader(viewModelScope) { session.session.library.work(workId) } }

    /** What's known of another work, such as an episode's season, before it's read. */
    fun known(workId: String): WorkSummary? = session.session.library.known(workId)

    /** Marks played or unplayed; the live events refresh every screen showing it. */
    fun setPlayed(workId: String, played: Boolean) = session.session.library.marks.setPlayed(workId, played)

    /** My List's id, once the lists are read. */
    val myListId: StateFlow<String?> = session.session.library.lists()
        .map { load -> load.getOrNull()?.firstOrNull { it.builtIn }?.id }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), null)

    /** Adds the work to My List or takes it off; the live event redraws the page. */
    fun setOnMyList(workId: String, on: Boolean) {
        val id = myListId.value ?: return
        session.session.library.marks.setOnList(id, workId, on)
    }
}
