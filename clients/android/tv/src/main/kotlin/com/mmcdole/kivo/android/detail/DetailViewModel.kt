package com.mmcdole.kivo.android.detail

import androidx.lifecycle.ViewModel
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.api.models.EpisodeInfo
import com.mmcdole.kivo.core.data.SeasonRef
import com.mmcdole.kivo.core.session.WorkPage

/**
 * A detail page: the core's [page], a series' open on the season of the
 * [episode] Go to series came from.
 */
class DetailViewModel(session: ServerSession, id: String, episode: EpisodeInfo?) : ViewModel() {
    val page: WorkPage = session.session.library.workPage(id, openOn = episode?.let { SeasonRef(it.seasonId, it.seasonNumber) })

    val marks = session.session.library.marks

    /** Marks played or unplayed; the live events refresh every screen showing it. */
    fun setPlayed(workId: String, played: Boolean) = marks.setPlayed(workId, played)

    /** Adds the work to My List or takes it off; the live event redraws the page. */
    fun setOnMyList(workId: String, on: Boolean) {
        val id = page.state.value.myListId ?: return
        marks.setOnList(id, workId, on)
    }

    override fun onCleared() {
        page.close()
    }
}
