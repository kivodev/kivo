package com.mmcdole.kivo.core.play

import com.mmcdole.kivo.api.models.CapabilityMismatch
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.PlaySummary
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.display.Display
import com.mmcdole.kivo.core.display.problem
import com.mmcdole.kivo.core.player.Subtitles
import com.mmcdole.kivo.core.player.TrackShow

data class PlayerState(
    val status: PlayerStatus = PlayerStatus.Idle,
    val play: PlayStarted? = null,
    val paused: Boolean = true,
    /** On the version's timeline. While seeking, the target. */
    val positionMs: Long = 0,
    /** A fallback is starting at the same position. */
    val adjusting: Boolean = false,
    val ended: Boolean = false,
    val failure: PlayerFailure? = null,
    /** What the engine said when it failed, for diagnostics. */
    val detail: String? = null,
    /** The audio track playing; null without a play, or with no audio. */
    val audioTrackId: String? = null,
    /** The subtitle track showing, or [Subtitles.OFF]. */
    val subtitleTrackId: String = Subtitles.OFF,
    /** How the player shows [subtitleTrackId]: the app draws a [TrackShow.External] where its engine doesn't. */
    val subtitle: TrackShow = TrackShow.Off,
) {
    /** Whether media is loaded, so controls act on it. */
    val live: Boolean
        get() = status == PlayerStatus.Playing || status == PlayerStatus.Buffering || status == PlayerStatus.Seeking
}

/**
 * The player's state: idle, loading, playing, buffering, seeking or error.
 * Pause is a flag rather than a state: a paused player is still playing in
 * the sense that matters here, with media loaded and controls live. A port of
 * the web player's reducer.
 */
enum class PlayerStatus { Idle, Loading, Playing, Buffering, Seeking, Error }

/** Why the player stopped. */
sealed interface PlayerFailure {
    /**
     * A problem code and, for playback refusals, a reason code: the
     * server's; the server's `notPlayable` or `noFallback` when a failure
     * here has nothing to fall back to; or another [RequestProblem.code],
     * `network` when no answer came; for `fileUnplayable`, the capability
     * the file failed ([mismatch]).
     */
    data class Problem(
        val code: String,
        val reason: String? = null,
        val plays: List<PlaySummary>? = null,
        val mismatch: CapabilityMismatch? = null,
    ) : PlayerFailure

    /** Stopped elsewhere: by an admin, another client or expiry. The player says so, with the admin's [message] if there is one. */
    data class Ended(val message: String? = null) : PlayerFailure
}

/** Why the player stopped, in a sentence: stopped elsewhere, the server's reason for a refused play, or the problem. */
fun Display.failure(failure: PlayerFailure): String = when (failure) {
    is PlayerFailure.Ended -> failure.message?.let { table.adminStopped.replace("{message}", it) } ?: table.playStopped
    is PlayerFailure.Problem -> failure.reason?.let(::reason) ?: problem(failure.code)
}

/** The failure a request's [error] means, by its [RequestProblem.code], with the server's reason and plays when it gave them. */
internal fun failureOf(error: Exception): PlayerFailure.Problem {
    val problem = RequestProblem.of(error)
    val document = (problem as? RequestProblem.Refused)?.problem
    return PlayerFailure.Problem(problem.code, document?.reason, document?.plays, document?.mismatch)
}

/** What changes the state. */
internal sealed interface PlayerAction {
    /** A play is being requested, in place of the one there is, if any, which shows until the server answers. */
    data object Start : PlayerAction

    /** The server answered with a play (a start or a fallback): its media loads. */
    data class Started(val play: PlayStarted) : PlayerAction

    /** Media failed and a fallback was asked for. */
    data class Fallback(val detail: String) : PlayerAction

    /** The tracks the player shows now: the play's own, or ones chosen since that it shows as it is. */
    data class Shown(val audioTrackId: String?, val subtitleTrackId: String, val subtitle: TrackShow) : PlayerAction

    /** The media is loaded at the start position and can play. */
    data object Ready : PlayerAction

    data object Play : PlayerAction

    data object Pause : PlayerAction

    /** Playback stalled waiting for data. */
    data object Waiting : PlayerAction

    /** Frames are moving again. */
    data object Playing : PlayerAction

    /** The user moved the position. */
    data class Seek(val positionMs: Long) : PlayerAction

    data object Seeked : PlayerAction

    data class Time(val positionMs: Long) : PlayerAction

    data object Ended : PlayerAction

    data class Fail(val failure: PlayerFailure, val detail: String? = null) : PlayerAction
}

internal fun PlayerState.reduce(action: PlayerAction): PlayerState = when (action) {
    PlayerAction.Start -> PlayerState(
        status = PlayerStatus.Loading,
        play = play,
        positionMs = positionMs,
        audioTrackId = audioTrackId,
        subtitleTrackId = subtitleTrackId,
        subtitle = subtitle,
    )
    is PlayerAction.Started ->
        copy(status = PlayerStatus.Loading, play = action.play, positionMs = action.play.startMs.toLong(), ended = false)
    is PlayerAction.Fallback -> copy(status = PlayerStatus.Loading, adjusting = true, detail = action.detail)
    is PlayerAction.Shown -> copy(audioTrackId = action.audioTrackId, subtitleTrackId = action.subtitleTrackId, subtitle = action.subtitle)
    PlayerAction.Ready -> if (status == PlayerStatus.Loading) copy(status = PlayerStatus.Playing) else this
    PlayerAction.Play -> copy(paused = false, ended = false)
    PlayerAction.Pause -> copy(paused = true)
    PlayerAction.Waiting -> if (status == PlayerStatus.Playing) copy(status = PlayerStatus.Buffering) else this
    PlayerAction.Playing ->
        if (status == PlayerStatus.Error || status == PlayerStatus.Idle) {
            this
        } else {
            copy(
                status = if (status == PlayerStatus.Seeking) PlayerStatus.Seeking else PlayerStatus.Playing,
                paused = false,
                adjusting = false,
            )
        }
    is PlayerAction.Seek -> if (live) copy(status = PlayerStatus.Seeking, positionMs = action.positionMs, ended = false) else this
    PlayerAction.Seeked -> if (status == PlayerStatus.Seeking) copy(status = PlayerStatus.Playing) else this
    is PlayerAction.Time -> if (live && status != PlayerStatus.Seeking) copy(positionMs = action.positionMs) else this
    PlayerAction.Ended -> if (live) copy(status = PlayerStatus.Playing, paused = true, ended = true) else this
    is PlayerAction.Fail ->
        copy(
            status = PlayerStatus.Error,
            failure = action.failure,
            adjusting = false,
            paused = true,
            detail = action.detail ?: detail,
        )
}
