package com.mmcdole.kivo.android.player

import androidx.media3.common.Player
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/**
 * The player the phone's media session controls: the video's or the
 * book's, whichever offered itself last. The app's playback service follows
 * [current], so the notification, the lock screen, audio focus and
 * background playback reach it. A player withdrawn stays current for
 * [GRACE_MS], so a play's next player, as a fallback or a new play at the
 * position builds, takes its place without the notification going.
 *
 * Call it on the main thread, [scope]'s.
 */
class SessionPlayers(private val scope: CoroutineScope) {
    private val mutableCurrent = MutableStateFlow<Player?>(null)
    val current: StateFlow<Player?> = mutableCurrent.asStateFlow()

    private var leaving: Job? = null

    /** Makes [player] the session's; returns what withdraws it, which does nothing once another has taken its place. */
    fun offer(player: Player): () -> Unit {
        leaving?.cancel()
        leaving = null
        mutableCurrent.value = player
        return { withdraw(player) }
    }

    private fun withdraw(player: Player) {
        if (mutableCurrent.value !== player) return
        leaving?.cancel()
        leaving = scope.launch {
            delay(GRACE_MS)
            if (mutableCurrent.value === player) mutableCurrent.value = null
            leaving = null
        }
    }

    companion object {
        /** How long a withdrawn player stays the session's, waiting for the next. */
        const val GRACE_MS = 1_500L
    }
}
