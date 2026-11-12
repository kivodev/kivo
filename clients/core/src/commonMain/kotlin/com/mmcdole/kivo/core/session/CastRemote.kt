package com.mmcdole.kivo.core.session

import co.touchlab.skie.configuration.annotations.DefaultArgumentInterop
import com.mmcdole.kivo.api.models.Version
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.answer
import com.mmcdole.kivo.core.cast.CastLink
import com.mmcdole.kivo.core.cast.CastLinkState
import com.mmcdole.kivo.core.cast.CastMedia
import com.mmcdole.kivo.core.cast.CastMessages
import com.mmcdole.kivo.core.cast.CastStatus
import com.mmcdole.kivo.core.data.withTrack
import com.mmcdole.kivo.core.display.QualityOption
import com.mmcdole.kivo.core.play.PlayChoice
import com.mmcdole.kivo.core.quietly
import com.mmcdole.kivo.core.settings.Quality
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/** Why a cast stopped or couldn't start. */
sealed interface CastProblem {
    /** The receiver stopped with [code], such as `signedOut`; [message] is an admin's words when they stopped the play. */
    data class Receiver(val code: String, val message: String?) : CastProblem

    /** The server didn't give a cast token. */
    data class Refused(val problem: RequestProblem) : CastProblem

    /** The TV can't reach the server, and keeps trying. */
    data object Unreachable : CastProblem
}

/** Where a play stopped on a TV, to carry on with here. */
data class ResumePoint(val choice: PlayChoice, val positionMs: Long)

/** The Cast session and what its receiver plays, as the Cast button, the mini controller and the remote show it ([CastRemote.state]). */
@ConsistentCopyVisibility
data class CastRemoteState internal constructor(
    val link: CastLinkState = CastLinkState.Unavailable,
    /** What the receiver plays, by its media status. */
    val media: CastMedia? = null,
    /** What the receiver says of its play. */
    val status: CastStatus? = null,
    /** The work on the TV, once read. */
    val work: WorkDetail? = null,
    /** Its next episode, once read, for up next. */
    val next: WorkDetail? = null,
    /** Where the TV is, between its status updates. */
    val positionMs: Long = 0,
    /** A LOAD on its way, by its work, until the receiver's media names it. */
    val loading: String? = null,
    val problem: CastProblem? = null,
    /** The TV's volume, 0 to 1. */
    val volume: Float = 0f,
    /** Auto, Original and the server's ladder, for the TV's quality menu. */
    val qualities: List<QualityOption> = emptyList(),
    /** The quality chosen here, else the one the receiver plays at. */
    val quality: Quality? = null,
) {
    val connected: Boolean get() = link is CastLinkState.Connected

    /** The Cast device's name while connected. */
    val device: String? get() = (link as? CastLinkState.Connected)?.device

    /** Something is on the TV, on its way, or stopped with a problem: the mini controller and the remote show. */
    val active: Boolean get() = connected && (media != null || loading != null || problem != null)

    val playing: Boolean get() = media?.state == CastMedia.State.Playing

    /** Loading or buffering on the TV: a spinner takes Play's place. */
    val waiting: Boolean get() = media?.let { it.state == CastMedia.State.Loading || it.state == CastMedia.State.Buffering } ?: (loading != null)

    val durationMs: Long get() = media?.durationMs?.takeIf { it > 0 } ?: work?.durationMs?.toLong() ?: 0

    /** The version the receiver plays, once the work is read. */
    val version: Version? get() = status?.versionId?.let { id -> work?.versions?.firstOrNull { it.id == id } }
}

/**
 * Casting to a TV over the app's [link] ([SessionCast.remote]): the session
 * with a Cast device and what its receiver plays. A LOAD carries a cast
 * token, so the TV plays as this user; the receiver starts, heartbeats and
 * changes its play itself, and the remote mirrors its status, reads the
 * work and its next episode, and sends Kivo's messages.
 *
 * Called on the main thread; [close] it when the app signs out.
 */
class CastRemote internal constructor(private val session: KivoSession, private val link: CastLink) {
    private val scope = CoroutineScope(session.scope.coroutineContext + SupervisorJob(session.scope.coroutineContext[Job]))

    private val mutableState = MutableStateFlow(CastRemoteState())

    val state: StateFlow<CastRemoteState> = mutableState.asStateFlow()

    private val newly = MutableSharedFlow<String>(extraBufferCapacity = 1)

    /** Each Cast device connected newly, by name, rather than joined again: a play here carries on there. */
    val connections: Flow<String> = newly.asSharedFlow()

    /** The last cast asked for, for Try again before the TV has a play. */
    private var lastCast: Pair<PlayChoice, Long?>? = null

    private var chosenQuality: Quality? = null
    private var reading: Pair<String, Job>? = null
    private var readingNext: Pair<String?, Job?>? = null
    private var ticking: Job? = null
    private var askedQualities = false

    init {
        link.start(
            object : CastLink.Events {
                override fun onLink(state: CastLinkState) = linkChanged(state)

                override fun onMedia(media: CastMedia?) = mediaChanged(media)

                override fun onVolume(volume: Float) = show { it.copy(volume = volume) }

                override fun onMessage(message: String) {
                    CastStatus.parseOrNull(message)?.let(::statusChanged)
                }
            },
        )
    }

    /**
     * Casts [choice] from [positionMs], 0 for its start, else its resume
     * position; false while no TV is connected.
     */
    @DefaultArgumentInterop.Enabled
    fun cast(choice: PlayChoice, positionMs: Long? = null): Boolean {
        val device = state.value.device ?: return false
        lastCast = choice to positionMs
        chosenQuality = null
        show { it.copy(loading = choice.workId, problem = null, status = null, quality = null) }
        read(choice.workId)
        scope.launch {
            try {
                val load = session.cast.load(device, choice, positionMs)
                val work = state.value.work?.takeIf { it.id == choice.workId }
                val art = work?.images?.let { it.poster ?: it.still ?: it.backdrop }
                link.load(load, work?.title.orEmpty(), art?.let { session.imageUrl(it.id, ART_WIDTH) })
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                if (state.value.loading == choice.workId) show { it.copy(loading = null, problem = CastProblem.Refused(RequestProblem.of(e))) }
            }
        }
        return true
    }

    fun togglePlay() {
        if (state.value.playing) link.pause() else link.play()
    }

    fun skip(deltaMs: Long) = seek(state.value.positionMs + deltaMs)

    /** Seeks to [positionMs], short of the last second, which would end the play. */
    fun seek(positionMs: Long) {
        val target = positionMs.coerceIn(0, (state.value.durationMs - END_MARGIN_MS).coerceAtLeast(0))
        show { it.copy(positionMs = target) }
        link.seek(target)
    }

    fun setSpeed(speed: Float) = link.setSpeed(speed)

    fun setVolume(volume: Float) {
        show { it.copy(volume = volume) }
        link.setVolume(volume)
    }

    fun chooseAudio(trackId: String) = link.send(CastMessages.setTracks(audioTrackId = trackId, subtitleTrackId = null))

    /** Shows a subtitle track, or `off`. */
    fun chooseSubtitle(trackId: String) = link.send(CastMessages.setTracks(audioTrackId = null, subtitleTrackId = trackId))

    /** Another version, with its own default tracks. */
    fun chooseVersion(versionId: String) = link.send(CastMessages.setVersion(versionId))

    /** A quality for the TV, at home or away. */
    fun chooseQuality(quality: Quality) {
        chosenQuality = quality
        show { it.copy(quality = quality) }
        val request = quality.request()
        link.send(CastMessages.setQuality(request, request))
    }

    /** Switches the dialogue boost on the TV, kept on this device for later plays, as the player does. */
    fun boostDialogue(on: Boolean) {
        session.core.scope.launch { session.core.setDialogueBoost(on) }
        link.send(CastMessages.setDialogueBoost(on))
    }

    /**
     * Adds the subtitle search's result [resultId] to the version on the TV
     * and shows it: the version's tracks gain it at once.
     */
    @Throws(Exception::class)
    suspend fun addSubtitle(resultId: String) {
        val version = state.value.version ?: return
        val track = session.playback.addSubtitle(version, resultId)
        show { it.copy(work = it.work?.withTrack(version.id, track)) }
        chooseSubtitle(track.id)
    }

    /** Up next's Play now. */
    fun playNext() = link.send(CastMessages.playNext())

    /** Up next's Hide. */
    fun hideUpNext() = link.send(CastMessages.hideUpNext())

    /** Try again: the work cast again, with a fresh token, from where the TV was; a press means someone is watching. */
    fun retry() {
        session.playback.confirmStillWatching()
        val s = state.value
        val workId = s.media?.workId ?: s.status?.workId
        val last = lastCast
        when {
            workId != null -> cast(current(workId), s.positionMs.takeIf { it > 0 } ?: last?.second)
            last != null -> cast(last.first, last.second)
        }
    }

    /** Stop casting: the TV stops, and what played there is returned to carry on with here, at the TV's position. */
    fun stopCasting(): ResumePoint? {
        val s = state.value
        val point = (s.media?.workId ?: s.status?.workId)?.let { ResumePoint(current(it), s.positionMs) }
        link.end(stopCasting = true)
        letGo()
        return point
    }

    /** Disconnect: this device lets go, and the TV plays on. */
    fun disconnect() {
        link.end(stopCasting = false)
        letGo()
    }

    /** Stops following the session, which carries on: the app signed out. */
    fun close() {
        scope.cancel()
        link.close()
    }

    private fun show(change: (CastRemoteState) -> CastRemoteState) {
        mutableState.value = change(mutableState.value)
    }

    /** [workId] as the TV plays it: its version and tracks. */
    private fun current(workId: String): PlayChoice {
        val status = state.value.status?.takeIf { it.workId == workId } ?: return PlayChoice(workId)
        return PlayChoice(workId, status.versionId, status.audioTrackId, status.subtitleTrackId)
    }

    private fun letGo() = linkChanged(CastLinkState.Disconnected)

    private fun linkChanged(now: CastLinkState) {
        val was = state.value.link
        if (now !is CastLinkState.Connected) {
            show { it.copy(link = now, media = null, status = null, loading = null, problem = null) }
            tick()
            return
        }
        show { it.copy(link = now) }
        if (now.resumed) {
            // The receiver plays on with a fresh token after this app was away.
            scope.launch { quietly { session.cast.tokenMessage(now.device) }?.let(link::send) }
        } else if (was !is CastLinkState.Connected) {
            newly.tryEmit(now.device)
        }
        link.send(CastMessages.getStatus())
    }

    private fun mediaChanged(media: CastMedia?) {
        show { s ->
            if (media == null) {
                s.copy(media = null)
            } else {
                s.copy(media = media, positionMs = media.positionMs, loading = s.loading.takeIf { it != media.workId }, problem = null)
            }
        }
        if (media != null) {
            read(media.workId)
            media.status?.let(CastStatus::parseOrNull)?.let(::statusChanged)
        }
        tick()
    }

    private fun statusChanged(status: CastStatus) {
        status.workId?.let(::read)
        readNext(status.nextWorkId)
        val error = status.error
        show { s ->
            val problem = when {
                status.unreachable -> CastProblem.Unreachable
                error != null -> CastProblem.Receiver(error, status.message)
                s.problem == CastProblem.Unreachable -> null
                else -> s.problem
            }
            s.copy(
                status = status,
                problem = problem,
                loading = if (error != null && !status.unreachable) null else s.loading,
                quality = chosenQuality ?: status.quality?.let { Quality.of(it) },
            )
        }
    }

    /** Keeps the position moving while the TV plays. */
    private fun tick() {
        ticking?.cancel()
        if (!state.value.playing) return
        ticking = scope.launch {
            while (true) {
                delay(TICK_MS)
                show { it.copy(positionMs = link.positionMs) }
            }
        }
    }

    /** Reads the work on the TV, for its title and artwork, and its versions and tracks for the menus. */
    private fun read(workId: String) {
        val (id, job) = reading ?: (null to null)
        if (id == workId && job?.isActive == true) return
        job?.cancel()
        if (state.value.work?.id != workId) {
            readingNext?.second?.cancel()
            readingNext = null
            show { it.copy(work = null, next = null) }
        }
        reading = workId to scope.launch {
            session.library.work(workId).collect { load ->
                if (load is Load.Loaded) show { it.copy(work = load.value) }
            }
        }
        if (!askedQualities) {
            askedQualities = true
            scope.launch { quietly { session.playback.qualityMenu() }?.let { menu -> show { it.copy(qualities = menu) } } }
        }
    }

    private fun readNext(nextId: String?) {
        if (readingNext?.first == nextId && readingNext != null) return
        readingNext?.second?.cancel()
        if (state.value.next?.id != nextId) show { it.copy(next = null) }
        readingNext = nextId to nextId?.let { id ->
            scope.launch { session.library.work(id).answer()?.let { next -> show { it.copy(next = next) } } }
        }
    }

    private companion object {
        const val TICK_MS = 500L
        const val END_MARGIN_MS = 1_000L
        const val ART_WIDTH = 480
    }
}
