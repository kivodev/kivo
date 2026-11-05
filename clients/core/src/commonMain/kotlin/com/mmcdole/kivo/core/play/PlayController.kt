package com.mmcdole.kivo.core.play

import com.mmcdole.kivo.api.ProblemException
import com.mmcdole.kivo.api.models.Capabilities
import com.mmcdole.kivo.api.models.PlayFallback
import com.mmcdole.kivo.api.models.PlaybackSettings
import com.mmcdole.kivo.api.models.PlayUpdate
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.PlayCreate
import com.mmcdole.kivo.api.models.PlayStop
import com.mmcdole.kivo.api.models.QualityRequest
import com.mmcdole.kivo.api.models.TrackType
import com.mmcdole.kivo.core.player.PictureInPicture
import com.mmcdole.kivo.core.player.PlayTracks
import com.mmcdole.kivo.core.player.PlayerError
import com.mmcdole.kivo.core.player.PlayerEngine
import com.mmcdole.kivo.core.player.Subtitles
import com.mmcdole.kivo.core.player.TrackShow
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlin.time.Duration.Companion.seconds
import kotlin.time.TimeSource

/**
 * The play behind the player, a port of the web player's controller: it
 * starts plays with the capability report, loads their media into the
 * [engine], shows each track chosen as the play's tracks say (at once, and
 * reported in the heartbeats, or by a new play at the position),
 * heartbeats every 10 seconds and on pause, seek and end, loads the media
 * again where it was once the server answers when it got no
 * answer, retries once as a transcode through the fallback request when
 * media fails otherwise (never for a book, [canFallBack] false), starts
 * again at the position when the server has forgotten the play (a 404),
 * and stops the play when the player goes away. Every start, fallback and teardown bumps a generation, so late
 * answers to earlier ones are dropped. Its [plays] are the server's, or for a
 * download the app's own, which keep the position in the progress outbox.
 *
 * Call it, and let the engine report, on one thread; [scope] runs there
 * (the main thread in the apps) and should outlive the player screen, so
 * the stop sent as it closes isn't cancelled with it.
 */
class PlayController internal constructor(
    private val plays: PlaySource,
    private val engine: PlayerEngine,
    /** The capability report; with `external` false, without subtitle files. */
    private val capabilities: suspend (external: Boolean) -> Capabilities,
    private val scope: CoroutineScope,
    private val canFallBack: Boolean = true,
    private val onEnded: () -> Unit = {},
) {
    private val mutableState = MutableStateFlow(PlayerState())
    val state: StateFlow<PlayerState> = mutableState.asStateFlow()

    /** The play that's playing, or being loaded. */
    internal var current: PlayStarted? = null
        private set

    private var request: PlayRequest? = null
    private var loaded = false
    private var generation = 0
    private var heartbeat: Job? = null
    private var seekStartedAt: TimeSource.Monotonic.ValueTimeMark? = null
    private var waiting = false
    private var lastPositionMs = 0L

    /** Whether this play has fallen back already: it does once, and a second failure shows the error. */
    private var fellBack = false

    /** A subtitle shown without a new play, a track's ID or `off`, for the heartbeats. */
    private var shownSubtitle: String? = null

    /** An audio track of the file played without a new play, for the heartbeats. */
    private var shownAudio: String? = null

    /** Whether plays ask for subtitle files, which the app can draw over the video. */
    private var askExternal = true

    /** Whether the current play was asked for without subtitle files. */
    private var playWithoutExternal = false

    /** Whether the media loading now stays paused once ready, as it was before it got no answer. */
    private var startPaused = false

    /** Whether the media loading now is loaded again after it got no answer: no answer again before it's ready falls back. */
    private var reconnected = false

    /** The position on the version's timeline. */
    fun positionMs(): Long {
        if (loaded) lastPositionMs = engine.positionMs
        return lastPositionMs
    }

    /**
     * Starts a play, ending the current one first. Its media stays in the
     * engine, paused, until the new play's replaces it, so the picture never
     * goes empty between them.
     */
    internal fun play(request: PlayRequest) {
        val generation = end(keepMedia = true)
        this.request = request
        shownSubtitle = null
        shownAudio = null
        playWithoutExternal = !askExternal
        fellBack = false
        reconnected = false
        request.positionMs?.let { lastPositionMs = it }
        dispatch(PlayerAction.Start)
        scope.launch {
            try {
                val body = PlayCreate(
                    workId = request.workId,
                    capabilities = capabilities(!playWithoutExternal),
                    versionId = request.versionId,
                    audioTrackId = request.audioTrackId,
                    subtitleTrackId = request.subtitleTrackId,
                    settings = request.settings(),
                    positionMs = request.positionMs?.toInt(),
                    dialogueBoost = request.dialogueBoost,
                )
                val play = plays.start(body)
                if (generation != this@PlayController.generation) {
                    // Superseded while starting.
                    stop(play.id, play.startMs.toLong())
                    return@launch
                }
                load(play, generation)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                if (generation == this@PlayController.generation) {
                    release()
                    dispatch(PlayerAction.Fail(failureOf(e)))
                }
            }
        }
    }

    /** Plays the current request again from the current position, with [change] applied, such as another audio track. */
    internal fun switchTo(change: (PlayRequest) -> PlayRequest = { it }) {
        val last = request ?: return
        play(change(last.copy(positionMs = positionMs())))
    }

    /** Plays other tracks from the current position, with [change] applied, on the play's version: a track belongs to one version. */
    internal fun switchTracks(change: (PlayRequest) -> PlayRequest) =
        switchTo { change(it).copy(versionId = current?.versionId ?: it.versionId) }

    /** Plays on from the current position with the dialogue boost switched [on], which the server applies to a new play. */
    internal fun boostDialogue(on: Boolean) = switchTracks { it.copy(dialogueBoost = on) }

    /**
     * Shows a subtitle track, or [Subtitles.OFF], as the play's tracks say:
     * at once where it has a delivery, the heartbeats reporting it so the
     * server remembers it as a choice and a later switch or retry keeps it;
     * else by a new play at the position, with the audio playing.
     */
    internal fun chooseSubtitle(trackId: String) {
        val play = current ?: return
        if (trackId == state.value.subtitleTrackId) return
        val show = PlayTracks.show(play, trackId)
        if (show == TrackShow.NewPlay) {
            switchTracks { it.copy(subtitleTrackId = trackId, audioTrackId = state.value.audioTrackId ?: it.audioTrackId) }
            return
        }
        shownSubtitle = trackId
        request = request?.copy(subtitleTrackId = trackId)
        dispatch(PlayerAction.Shown(state.value.audioTrackId, trackId, show))
        if (loaded) engine.showSubtitle(show)
        beat()
    }

    /**
     * Plays an audio track as the play's tracks say: one in the file at
     * once, the heartbeats reporting it as [chooseSubtitle] does; else by a
     * new play at the position, with the subtitle showing.
     */
    internal fun chooseAudio(trackId: String) {
        val play = current ?: return
        if (trackId == state.value.audioTrackId) return
        val show = PlayTracks.show(play, trackId)
        if (show !is TrackShow.File) {
            switchTracks { it.copy(audioTrackId = trackId, subtitleTrackId = state.value.subtitleTrackId) }
            return
        }
        shownAudio = trackId
        request = request?.copy(audioTrackId = trackId)
        dispatch(PlayerAction.Shown(trackId, state.value.subtitleTrackId, state.value.subtitle))
        if (loaded) engine.selectAudio(show.index)
        beat()
    }

    /**
     * Whether the app can draw over the video: not in picture-in-picture or
     * on an AirPlay device, where only the video goes. While it can't,
     * plays ask for no subtitle files; a subtitle file showing as it goes
     * there plays again from the position without one, and coming back, a
     * subtitle the play started there burned in plays again as before
     * ([PictureInPicture]).
     */
    internal fun setOverlayVisible(visible: Boolean) {
        if (visible == askExternal) return
        askExternal = visible
        val play = current ?: return
        val trackId = state.value.subtitleTrackId
        val newPlay = if (visible) {
            playWithoutExternal && PictureInPicture.leavingNeedsNewPlay(play, trackId)
        } else {
            PictureInPicture.enteringNeedsNewPlay(play, trackId)
        }
        if (newPlay) switchTracks { it.copy(subtitleTrackId = trackId) }
    }

    /** Tries the last request again after a failure, from where it stopped. */
    fun retry() = switchTo()

    fun seek(positionMs: Long) {
        val duration = current?.durationMs ?: return
        if (!loaded) return
        val target = positionMs.coerceIn(0, duration.toLong())
        seekStartedAt = TimeSource.Monotonic.markNow()
        dispatch(PlayerAction.Seek(target))
        engine.seek(target)
    }

    fun skip(deltaMs: Long) = seek(positionMs() + deltaMs)

    fun togglePlay() = if (engine.paused) resume() else pause()

    fun resume() {
        if (!loaded) return
        if (state.value.ended) seek(0)
        engine.play()
    }

    fun pause() {
        if (loaded) engine.pause()
    }

    /**
     * A play ended elsewhere, from the live events' `playEnded`:
     * if it's this one (an admin or another client stopped it, or it
     * expired), the player stops without a stop request and says so,
     * with the admin's [message] if there is one.
     */
    internal fun playEnded(playId: String, message: String? = null) {
        if (current?.id != playId) return
        generation++
        release()
        current = null
        dispatch(PlayerAction.Fail(PlayerFailure.Ended(message)))
    }

    /** The app is leaving the player or going to the background: stop the play, keeping the request. */
    fun stopOnExit() {
        val play = current ?: return
        stop(play.id, positionMs())
        this.current = null
        generation++
        release()
    }

    /** The app came back after [stopOnExit]: start again where it was. */
    fun restore() {
        if (request != null && current == null) retry()
    }

    /** Stops the play and releases everything. */
    internal fun dispose() {
        end()
    }

    /**
     * Stops the play and releases everything but the engine's media, which
     * stays, paused, for the next controller on the engine to replace with
     * its play's.
     */
    internal fun handOver() {
        end(keepMedia = true)
    }

    /** Ends the current play, if any, and returns the next generation. */
    private fun end(keepMedia: Boolean = false): Int {
        current?.let { stop(it.id, positionMs()) }
        current = null
        generation++
        release(keepMedia)
        return generation
    }

    /**
     * Releases the heartbeat, keeping the play, and the engine's media, even
     * media an earlier controller handed over, unless [keepMedia] keeps it paused.
     */
    private fun release(keepMedia: Boolean = false) {
        positionMs()
        heartbeat?.cancel()
        heartbeat = null
        if (!keepMedia) engine.unload() else if (loaded) engine.pause()
        loaded = false
        startPaused = false
        seekStartedAt = null
        waiting = false
    }

    private fun load(play: PlayStarted, generation: Int) {
        this.current = play
        lastPositionMs = play.startMs.toLong()
        dispatch(PlayerAction.Started(play))
        engine.load(play, events(generation))
        loaded = true
        showTracks(play)
        heartbeat = scope.launch {
            while (true) {
                delay(HEARTBEAT_INTERVAL)
                beat()
            }
        }
    }

    /**
     * Shows [play]'s tracks: its own, or those shown at once since, which
     * a play loaded again where it was keeps. An audio track of a file is
     * selected by its index, as it may not be the file's default.
     */
    private fun showTracks(play: PlayStarted) {
        val subtitle = shownSubtitle ?: PlayTracks.selected(play, TrackType.Subtitle) ?: Subtitles.OFF
        val audio = shownAudio ?: PlayTracks.selected(play, TrackType.Audio)
        val show = PlayTracks.show(play, subtitle)
        dispatch(PlayerAction.Shown(audio, subtitle, show))
        engine.showSubtitle(show)
        val audioShow = audio?.let { PlayTracks.show(play, it) }
        if (audioShow is TrackShow.File) engine.selectAudio(audioShow.index)
    }

    /** The engine's reports for the load of [generation], dropped once it's past. */
    private fun events(generation: Int) = object : PlayerEngine.Events {
        private val latest get() = generation == this@PlayController.generation

        override fun onReady() {
            if (!latest) return
            reconnected = false
            dispatch(PlayerAction.Ready)
            if (startPaused) startPaused = false else engine.play()
        }

        override fun onPlay() {
            if (latest) dispatch(PlayerAction.Play)
        }

        override fun onPause() {
            if (!latest) return
            dispatch(PlayerAction.Pause)
            beat("paused")
        }

        override fun onWaiting() {
            if (!latest) return
            waiting = true
            dispatch(PlayerAction.Waiting)
        }

        override fun onPlaying() {
            if (!latest) return
            waiting = false
            dispatch(PlayerAction.Playing)
        }

        override fun onSeeked() {
            if (!latest) return
            val started = seekStartedAt ?: return
            seekStartedAt = null
            dispatch(PlayerAction.Seeked)
            beat(seekToFirstFrameMs = started.elapsedNow().inWholeMilliseconds.toInt())
        }

        override fun onTime() {
            if (latest) dispatch(PlayerAction.Time(positionMs()))
        }

        override fun onEnded() {
            if (!latest) return
            dispatch(PlayerAction.Ended)
            beat("ended")
            this@PlayController.onEnded()
        }

        override fun onFailure(failure: PlayerError) {
            if (!latest) return
            if (failure.code == NETWORK_ERROR && !reconnected) reconnect() else fallBack(failure)
        }
    }

    /**
     * The media got no answer, as when the device leaves home, and the
     * server's address may have moved. Once the server answers, at its
     * address picked again, the play's media loads again there, where it
     * was, paused or playing as it was, replacing the media that failed; a
     * play the server forgot meanwhile starts again at the position.
     */
    private fun reconnect() {
        val play = current ?: return
        val positionMs = positionMs()
        val paused = state.value.paused
        val generation = ++generation
        release(keepMedia = true)
        reconnected = true
        dispatch(PlayerAction.Waiting)
        scope.launch {
            try {
                val update = PlayUpdate(
                    positionMs.toInt(),
                    if (paused) "paused" else "buffering",
                    subtitleTrackId = shownSubtitle,
                    audioTrackId = shownAudio,
                )
                plays.reconnect(play.id, update)
                if (generation != this@PlayController.generation) return@launch
                startPaused = paused
                load(play.copy(startMs = positionMs.toInt()), generation)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                if (generation != this@PlayController.generation) return@launch
                if (isGone(e)) {
                    this@PlayController.current = null
                    switchTo { it.copy(positionMs = positionMs) }
                    return@launch
                }
                end()
                dispatch(PlayerAction.Fail(failureOf(e)))
            }
        }
    }

    /**
     * Media failed. The first time in a play, the server ends the play and
     * retries it as a transcode, which plays from the same position, its
     * media replacing the failed media; the second time, the player shows
     * the error. A book's player has no transcode to try, and shows that
     * it can't play.
     */
    private fun fallBack(failure: PlayerError) {
        val play = current ?: return
        val positionMs = positionMs()
        val generation = ++generation
        release(keepMedia = true)
        val retry = canFallBack && !fellBack
        fellBack = true
        if (retry) dispatch(PlayerAction.Fallback(failure.detail))
        scope.launch {
            try {
                if (generation != this@PlayController.generation) return@launch
                if (!retry) {
                    end()
                    val code = if (canFallBack) "noFallback" else "notPlayable"
                    dispatch(PlayerAction.Fail(PlayerFailure.Problem(code), failure.detail))
                    return@launch
                }

                val next = plays.fallback(
                    play.id,
                    PlayFallback(positionMs.toInt(), failure.code, failure.detail.take(1000)),
                )
                if (generation != this@PlayController.generation) {
                    stop(next.id, next.startMs.toLong())
                    return@launch
                }
                // The server took the tracks shown since from the heartbeats: the new play shows them as its own.
                shownSubtitle = null
                shownAudio = null
                load(next, generation)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                if (generation != this@PlayController.generation) return@launch
                if (isGone(e)) {
                    // The server restarted and forgot the play: start a new one here.
                    this@PlayController.current = null
                    switchTo { it.copy(positionMs = positionMs) }
                    return@launch
                }
                end()
                dispatch(PlayerAction.Fail(failureOf(e)))
            }
        }
    }

    private fun beat(state: String? = null, seekToFirstFrameMs: Int? = null) {
        val play = current ?: return
        if (!loaded) return
        val body = PlayUpdate(
            positionMs = positionMs().toInt(),
            state = state ?: when {
                engine.paused -> "paused"
                waiting -> "buffering"
                else -> "playing"
            },
            seekToFirstFrameMs = seekToFirstFrameMs,
            bandwidthBps = engine.bandwidthBps,
            subtitleTrackId = shownSubtitle,
            audioTrackId = shownAudio,
        )
        scope.launch {
            try {
                plays.heartbeat(play.id, body)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                // An unknown play means the server restarted: start again at the saved position.
                if (isGone(e) && play === this@PlayController.current) {
                    this@PlayController.current = null
                    switchTo { it.copy(positionMs = body.positionMs.toLong()) }
                }
            }
        }
    }

    /** Saves the position and ends a play, without waiting or minding failure. */
    private fun stop(id: String, positionMs: Long) {
        scope.launch {
            try {
                plays.stop(id, PlayStop(positionMs.toInt()))
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                // The play expires on the server by itself.
            }
        }
    }

    private fun dispatch(action: PlayerAction) {
        mutableState.value = mutableState.value.reduce(action)
    }
}

/** The failure of media that got no answer, in the fallback request's codes. */
private const val NETWORK_ERROR = "networkError"

/** How often a play reports its position while playing. */
internal val HEARTBEAT_INTERVAL = 10.seconds

/** What to play: the screen's choices, or the player's own on a switch. */
internal data class PlayRequest(
    val workId: String,
    val versionId: String? = null,
    val audioTrackId: String? = null,
    /** A subtitle track's ID, or `off`. */
    val subtitleTrackId: String? = null,
    /** The quality on the server's local networks. */
    val localQuality: QualityRequest? = null,
    /** The quality anywhere else, when it differs: the server picks between the two. */
    val remoteQuality: QualityRequest? = null,
    /** Where to start on the version's timeline; the resume position when null. */
    val positionMs: Long? = null,
    /** Whether to boost dialogue: this device's setting, or the switch in the player. */
    val dialogueBoost: Boolean = false,
) {
    /** The settings the play request carries: the qualities, when there are any. */
    internal fun settings(): PlaybackSettings? =
        if (localQuality == null && remoteQuality == null) null else PlaybackSettings(localQuality = localQuality, remoteQuality = remoteQuality)
}

private fun isGone(error: Exception) = error is ProblemException && error.status == 404
