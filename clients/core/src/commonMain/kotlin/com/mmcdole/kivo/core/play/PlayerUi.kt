package com.mmcdole.kivo.core.play

import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.core.player.Autoplay
import com.mmcdole.kivo.core.player.PlayEnd
import com.mmcdole.kivo.core.player.SKIP_SHOWS
import com.mmcdole.kivo.core.player.SkipMarker
import com.mmcdole.kivo.core.player.SkipShow
import com.mmcdole.kivo.core.player.UpNext
import com.mmcdole.kivo.core.player.UpNextPhase
import com.mmcdole.kivo.core.player.markersOf
import com.mmcdole.kivo.core.player.playEnd
import com.mmcdole.kivo.core.player.upNextCredits
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlin.coroutines.CoroutineContext
import kotlin.time.Duration.Companion.seconds


/**
 * What a video player shows over the video by the timed rules every player
 * keeps, for the app to draw: the controls, the Skip
 * Intro, Recap or Credits button, up next's card, and where the player goes
 * once it's done.
 */
data class PlayerUi(
    /** Whether the controls show. */
    val controlsShown: Boolean = true,
    /** The marker whose Skip button shows: Skip Intro, Skip Recap or Skip Credits. */
    val skipMarker: SkipMarker? = null,
    /** Up next's card, which shows while [UpNext.shows]. */
    val upNext: UpNext = UpNext(),
    /** Where the player goes now, for the app to take it there; null while it plays on. */
    val end: PlayerEnd? = null,
)

/** Where a video player goes when it's done. */
enum class PlayerEnd {
    /** The next episode: up next counted down at the credits, the episode played on at its end, or someone chose it. */
    PlayNext,

    /** The last episode ended: the series' page. */
    Series,

    /** Anything else ended: back where playback started. */
    Close,
}

/**
 * The timed rules behind [PlayerUi], over a play's [state]: the controls
 * show as an input, a menu, a held scrub or a pause shows them, and hide as
 * the app hides them, which hides them a while after the last input while
 * playing; Skip shows for
 * [SKIP_SHOWS] as playback enters a marker and with the controls while it
 * stays there; up next at the final credits counts down a second at a time
 * while playing with no menu open, and plays the next episode at zero, and
 * elsewhere counts the seconds left in the episode, unless [autoplay] has it
 * wait, with the user's autoplay off or asking "Still watching?"; and the
 * play's end goes to the next episode, the series or back.
 *
 * Inputs come on the main thread, the state's changes are followed in
 * [scope], and the waits run in [scope] with [clock] in place of its
 * dispatcher's, which a test makes virtual.
 */
internal class PlayerUiRules(
    private val state: StateFlow<PlayerState>,
    private val seek: (Long) -> Unit,
    private val autoplay: Autoplay,
    scope: CoroutineScope,
    clock: CoroutineContext,
) {
    private val job = SupervisorJob(scope.coroutineContext[Job])
    private val scope = CoroutineScope(scope.coroutineContext + job)
    private val timers = CoroutineScope(scope.coroutineContext + job + clock)

    private val mutableUi = MutableStateFlow(PlayerUi())
    val ui: StateFlow<PlayerUi> = mutableUi.asStateFlow()

    /** Null for anything but an episode; for one, whether it has a next one to play. */
    private var hasNext: Boolean? = null
    private var menuOpen = false
    private var controls = true
    private var upNext = UpNext()
    private val skip = SkipShow()
    private var markersOf: PlayStarted? = null
    private var markerList = emptyList<SkipMarker>()
    private var end: PlayerEnd? = null

    /** Whether playback went on, live and not paused, at the last change. */
    private var going = false
    private var skipShowing: Job? = null
    private var countdown: Job? = null

    init {
        this.scope.launch { state.collect(::changed) }
    }

    /** This video is an episode, with a next one to play or not. */
    fun episode(hasNext: Boolean) {
        this.hasNext = hasNext
        update()
    }

    /** Any input: someone is watching, so up next may play on again. */
    fun input() {
        autoplay.reset()
        update()
    }

    /** Input that shows the controls. */
    fun showControls() {
        controls = true
        input()
    }

    fun hideControls() {
        controls = false
        update()
    }

    /** A tap on the video: the controls hide while playing with no menu open, and show otherwise. */
    fun toggleControls() {
        if (controls && !state.value.paused && !menuOpen) hideControls() else showControls()
    }

    /** A menu or panel opened over the player, which shows the controls and holds up next's countdown; or it closed. */
    fun menu(open: Boolean) {
        menuOpen = open
        if (open) controls = true
        update()
    }

    /** A scrub is held, which shows the controls; or it let go. */
    fun scrubbing(held: Boolean) {
        if (held) controls = true
        update()
    }

    /** Skip: playback goes to the marker's end, and the button stays away for that marker. */
    fun skipMarker() {
        val marker = ui.value.skipMarker ?: return
        autoplay.reset()
        skip.skip()
        seek(marker.endMs)
        update()
    }

    /** "Hide", or "Watch credits": no card for the rest of the episode, which still plays on at its end. */
    fun hideUpNext() {
        autoplay.reset()
        upNext = upNext.hide()
        update()
    }

    /** Someone chose the next episode: Play now, or Next. */
    fun playNext() {
        if (hasNext != true) return
        autoplay.reset()
        end = PlayerEnd.PlayNext
        update()
    }

    /** The last part played to its end: the next episode plays on, or up next waits for an answer; else the player is done. */
    fun ended() {
        if (end == PlayerEnd.PlayNext) return
        when (playEnd(episode = hasNext != null, hasNext = hasNext == true, autoplayAllowed = autoplay.allowed)) {
            PlayEnd.PlayNext -> playOn()
            PlayEnd.Wait -> upNext = upNext.waits(autoplay.phase)
            PlayEnd.Series -> end = PlayerEnd.Series
            PlayEnd.Close -> {
                end = PlayerEnd.Close
                controls = true
            }
        }
        update()
    }

    /** The player is done: every wait stops. */
    fun dispose() {
        job.cancel()
    }

    private fun changed(state: PlayerState) {
        // A pause shows the controls.
        if (going && state.live && state.paused) controls = true
        going = state.live && !state.paused
        if (end == PlayerEnd.Close && !state.ended) end = null
        update()
    }

    /** The next episode plays on by itself, which extends the run. */
    private fun playOn() {
        autoplay.playedOnItsOwn()
        end = PlayerEnd.PlayNext
    }


    /** Works out what shows from the play's state and the inputs so far, starting and stopping the waits it needs. */
    private fun update() {
        val state = state.value
        val play = state.play?.takeIf { state.live }
        val markers = play?.let(::markers).orEmpty()
        if (play != null && hasNext == true) {
            upNext = upNext.at(state.positionMs, play.durationMs.toLong(), autoplay.phase, upNextCredits(markers))
        }
        countDown(upNext.phase == UpNextPhase.CountingDown && upNext.atCredits && end == null)
        val card = upNext.takeIf { play != null } ?: UpNext()

        val marker = play?.let { skip.at(markers, state.positionMs, controls, card.shows) }
        if (play != null && skip.entered) {
            skipShowing?.cancel()
            skipShowing = timers.launch {
                delay(SKIP_SHOWS)
                skip.expire()
                update()
            }
        }

        mutableUi.value = PlayerUi(controlsShown = controls, skipMarker = marker, upNext = card, end = end)
    }

    /** The play's markers, read once per play. */
    private fun markers(play: PlayStarted): List<SkipMarker> {
        if (play !== markersOf) {
            markersOf = play
            markerList = markersOf(play)
        }
        return markerList
    }

    /** Up next's countdown at the credits, a second at a time while it [runs]; it counts only while playing with no menu open. */
    private fun countDown(runs: Boolean) {
        if (!runs) {
            countdown?.cancel()
            countdown = null
            return
        }
        if (countdown != null) return
        countdown = timers.launch {
            while (true) {
                delay(1.seconds)
                val state = state.value
                upNext = upNext.tick(running = state.status == PlayerStatus.Playing && !state.paused && !menuOpen)
                if (upNext.playsNext) {
                    countdown = null
                    playOn()
                    update()
                    return@launch
                }
                update()
            }
        }
    }
}
