package com.mmcdole.kivo.android.mobile.books

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.api.models.Chapter
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.core.books.BookPlayerModel
import com.mmcdole.kivo.core.books.BookPlayerState
import com.mmcdole.kivo.core.books.BookProgress
import com.mmcdole.kivo.core.books.SleepChoice
import com.mmcdole.kivo.core.books.SleepState
import com.mmcdole.kivo.core.play.PlayerFailure
import com.mmcdole.kivo.core.play.PlayerStatus
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn

data class NowPlayingUiState(
    /** A book is loaded; without one, Now Playing closes. */
    val shown: Boolean = false,
    val work: WorkDetail? = null,
    val progress: BookProgress? = null,
    val chapters: List<Chapter> = emptyList(),
    val playing: Boolean = false,
    val loading: Boolean = false,
    /** The user's speed for books. */
    val speed: Double = 1.0,
    val sleep: SleepState = SleepState.Off,
    /** The last sleep timer chosen, which the menu offers first. */
    val lastSleep: SleepChoice? = null,
    val failure: PlayerFailure? = null,
)

/**
 * Now Playing: the book the core's book player has loaded, where it is in
 * its chapter and in the book, its chapters, speed and sleep timer, and
 * their commands.
 */
class NowPlayingViewModel(private val books: BookPlayerModel) : ViewModel() {
    val state: StateFlow<NowPlayingUiState> = books.state
        .map(::nowPlayingOf)
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), nowPlayingOf(books.state.value))

    private fun nowPlayingOf(state: BookPlayerState) = NowPlayingUiState(
        shown = state.workId != null,
        work = state.work,
        progress = state.progress,
        chapters = state.chapters?.list.orEmpty(),
        playing = state.playing,
        loading = state.player.status == PlayerStatus.Loading,
        speed = state.speed,
        sleep = state.sleep,
        lastSleep = books.lastSleep,
        failure = state.failure,
    )

    fun togglePlay() = books.togglePlay()

    fun skipBack() = books.skipBack()

    fun skipForward() = books.skipForward()

    fun previousChapter() = books.previousChapter()

    fun nextChapter() = books.nextChapter()

    /** The seek bar let go [offsetMs] into the chapter at [index], the one it spanned. */
    fun seekInChapter(index: Int, offsetMs: Long) = books.seekInChapter(index, offsetMs)

    fun playChapter(index: Int) = books.playChapter(index)

    fun setSpeed(speed: Double) = books.setSpeed(speed)

    fun setSleep(choice: SleepChoice) = books.setSleep(choice)

    fun stopSleep() = books.stopSleep()

    /** Stop listening: the place is saved and the player goes. */
    fun stop() = books.stop()
}
