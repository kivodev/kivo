package com.mmcdole.kivo.android.detail

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.ui.Loader
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkSummary
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.launch

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
            kind = "series",
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
    fun setPlayed(workId: String, played: Boolean) = act { session.session.library.setPlayed(workId, played) }

    fun setFavorite(workId: String, favorite: Boolean) = act { session.session.library.setFavorite(workId, favorite) }

    private fun act(call: suspend () -> Unit) {
        viewModelScope.launch {
            try {
                call()
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                // The mark shows as it was; the page stays as the server has it.
            }
        }
    }
}
