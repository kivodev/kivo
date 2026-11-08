package com.mmcdole.kivo.android.mobile.books

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.api.models.ImageRef
import com.mmcdole.kivo.core.books.BookPlayerModel
import com.mmcdole.kivo.core.books.BookPlayerState
import com.mmcdole.kivo.core.play.PlayerStatus
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn

data class MiniPlayerUiState(
    /** A book is loaded, and the mini player shows. */
    val shown: Boolean = false,
    val title: String = "",
    val cover: ImageRef? = null,
    /** The chapter playing, from 0; null until the book's chapters are known. */
    val chapterIndex: Int? = null,
    /** The time left in the book. */
    val remainingMs: Long = 0,
    /** How far through the book, to a thousandth. */
    val fraction: Float = 0f,
    val playing: Boolean = false,
    /** Opening the book or its play, before it can play. */
    val loading: Boolean = false,
)

/**
 * The mini player over the tabs while a book is loaded: what the core's book
 * player says of it, set only as it changes to the minute and the
 * thousandth, so the bar doesn't redraw with every tick. Books start
 * playing here, from a card or a book's page.
 */
class MiniPlayerViewModel(private val books: BookPlayerModel) : ViewModel() {
    val state: StateFlow<MiniPlayerUiState> = books.state
        .map(::miniOf)
        .distinctUntilChanged()
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), miniOf(books.state.value))

    /** Plays a book from where the user left it, or from its start; the mini player shows it. */
    fun play(workId: String, fromStart: Boolean) = books.play(workId, positionMs = if (fromStart) 0 else null)

    fun togglePlay() = books.togglePlay()

    fun skipBack() = books.skipBack()
}

internal fun miniOf(state: BookPlayerState): MiniPlayerUiState {
    if (state.workId == null) return MiniPlayerUiState()
    val progress = state.progress
    return MiniPlayerUiState(
        shown = true,
        title = state.work?.title.orEmpty(),
        cover = state.work?.images?.poster,
        chapterIndex = progress?.chapterIndex,
        remainingMs = (progress?.remainingMs ?: 0) / 60_000 * 60_000,
        fraction = ((progress?.fraction ?: 0.0) * 1000).toInt() / 1000f,
        playing = state.playing,
        loading = state.player.status == PlayerStatus.Loading,
    )
}
