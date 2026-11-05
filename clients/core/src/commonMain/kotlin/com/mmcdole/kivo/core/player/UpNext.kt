package com.mmcdole.kivo.core.player

/** Seconds up next counts down at the final credits before the next episode plays. */
internal const val UP_NEXT_COUNTDOWN_SECONDS = 10

/** An episode shorter than this has no up next card. */
internal const val UP_NEXT_MIN_EPISODE_MS = 10 * 60_000L

/** Up next appears only with at least this long left, so a seek to the last moments doesn't bring it. */
internal const val UP_NEXT_MIN_LEFT_MS = 20_000L

/**
 * Where up next appears in an episode of [durationMs]: 30 seconds before
 * the end, 35 for an episode of 40 minutes or more, 40 for 50 minutes or
 * more, or where final credits start ([creditsStartMs]) if
 * that's sooner; null for one shorter than 10 minutes.
 */
internal fun upNextAtMs(durationMs: Long, creditsStartMs: Long? = null): Long? {
    if (durationMs < UP_NEXT_MIN_EPISODE_MS) return null
    val leadMs = when {
        durationMs >= 50 * 60_000L -> 40_000L
        durationMs >= 40 * 60_000L -> 35_000L
        else -> 30_000L
    }
    return minOf(durationMs - leadMs, creditsStartMs ?: Long.MAX_VALUE)
}

/** Up next's card in an episode with a next one. */
enum class UpNextPhase {
    /** Before the card's time, or a seek back before it. */
    Hidden,

    /** Counting down to the next episode: 10 seconds at the credits, otherwise the seconds left in the episode. */
    CountingDown,

    /** With the user's autoplay off: no countdown, waiting for Play now, with nothing playing on at the end. */
    Waiting,

    /** Hidden for the rest of the episode, which still plays on at its end, as autoplay allows. */
    Dismissed,

    /** After three episodes without input: waiting for an answer, with nothing playing on. */
    StillWatching,
}

/**
 * Up next's card for one episode with a next one: the phase and the
 * seconds left to count, as a player's [com.mmcdole.kivo.core.play.PlayerUi]
 * shows it and a Cast receiver's status reports it, and whether it came at
 * the credits, where its second button is "Watch credits" rather than
 * "Hide". Each change returns the card as it is after it.
 */
data class UpNext(val phase: UpNextPhase, val secondsLeft: Int, val atCredits: Boolean = false) {
    constructor() : this(UpNextPhase.Hidden, UP_NEXT_COUNTDOWN_SECONDS)

    /** Whether the card shows. */
    val shows: Boolean get() = phase == UpNextPhase.CountingDown || phase == UpNextPhase.Waiting || phase == UpNextPhase.StillWatching

    /** Whether the countdown at the credits ran out: the next episode plays now. Away from the credits, the episode's end plays it. */
    internal val playsNext: Boolean get() = phase == UpNextPhase.CountingDown && atCredits && secondsLeft == 0

    /**
     * The card at [positionMs] of an episode of [durationMs], with the
     * [credits] that run to its end if it has them: it appears at
     * [upNextAtMs] with 20 seconds or more left, as [appearsAs] (the
     * [Autoplay.phase]), and a position before its time hides it and counts
     * again from the start the next time it appears. It is at the credits
     * only when it appears at their start, not when credits shorter than its
     * lead start after it. Away from the credits, a counting card follows
     * the seconds left in the episode.
     */
    internal fun at(positionMs: Long, durationMs: Long, appearsAs: UpNextPhase, credits: SkipMarker? = null): UpNext {
        val showAt = upNextAtMs(durationMs, credits?.startMs)
        if (positionMs < (showAt ?: (durationMs - UP_NEXT_MIN_LEFT_MS))) return UpNext()
        if (phase == UpNextPhase.CountingDown && !atCredits) return copy(secondsLeft = secondsToEnd(positionMs, durationMs))
        val appears = phase == UpNextPhase.Hidden && showAt != null && durationMs - positionMs >= UP_NEXT_MIN_LEFT_MS
        if (!appears) return this
        val atCredits = credits != null && showAt == credits.startMs
        val secondsLeft = if (atCredits || appearsAs != UpNextPhase.CountingDown) UP_NEXT_COUNTDOWN_SECONDS else secondsToEnd(positionMs, durationMs)
        return UpNext(appearsAs, secondsLeft, atCredits)
    }

    /** A second passed: the countdown at the credits runs only while [running], playing with no menu or panel open. */
    internal fun tick(running: Boolean): UpNext =
        if (phase == UpNextPhase.CountingDown && atCredits && running && secondsLeft > 0) copy(secondsLeft = secondsLeft - 1) else this

    /** "Hide", or "Watch credits": the card stays away for the rest of the episode. */
    internal fun hide(): UpNext = if (shows) copy(phase = UpNextPhase.Dismissed) else this

    /** The episode ended with nothing playing on: the card shows in [phase], Waiting or StillWatching, until answered. */
    internal fun waits(phase: UpNextPhase): UpNext = UpNext(phase, UP_NEXT_COUNTDOWN_SECONDS)
}

/** The whole seconds left in an episode of [durationMs] at [positionMs]. */
private fun secondsToEnd(positionMs: Long, durationMs: Long): Int = maxOf(0L, (durationMs - positionMs + 999) / 1000).toInt()

/** What a player does when its video plays to the end. */
internal enum class PlayEnd {
    /** The next episode plays on its own. */
    PlayNext,

    /** Nothing plays on; up next waits for an answer. */
    Wait,

    /** The last episode ended: the series' page. */
    Series,

    /** Anything else ended: back where playback started. */
    Close,
}

/**
 * What follows a video playing to its end: for an episode, the next one,
 * unless [autoplayAllowed] is false, or with none next, the series' page.
 * Hiding up next doesn't keep the next episode from playing.
 */
internal fun playEnd(episode: Boolean, hasNext: Boolean, autoplayAllowed: Boolean): PlayEnd = when {
    !episode -> PlayEnd.Close
    !hasNext -> PlayEnd.Series
    autoplayAllowed -> PlayEnd.PlayNext
    else -> PlayEnd.Wait
}

/**
 * Whether up next plays on by itself: as the user's autoplay preference
 * has it, and for a run of episodes played on without input, after three
 * of which it stops counting down and asks "Still watching?". One per app,
 * across players.
 */
internal class Autoplay(private val limit: Int = 3) {
    private var run = 0

    /** The user's `autoplayNext`, as last read; on until it is. */
    var enabled = true

    /**
     * How up next appears, and waits once an episode ends without playing
     * on: counting down; Waiting, with [enabled] off; or after the run,
     * asking "Still watching?".
     */
    val phase: UpNextPhase
        get() = when {
            !enabled -> UpNextPhase.Waiting
            run < limit -> UpNextPhase.CountingDown
            else -> UpNextPhase.StillWatching
        }

    /** Whether up next may play on by itself. */
    val allowed: Boolean get() = phase == UpNextPhase.CountingDown

    /** Episodes played on without input so far, which a cast carries to the TV. */
    val count: Int get() = run

    /** An episode started on its own, extending the run. */
    fun playedOnItsOwn() {
        run++
    }

    /** Any input, choosing the next episode included, means someone is watching: up next may play on again. */
    fun reset() {
        run = 0
    }
}
