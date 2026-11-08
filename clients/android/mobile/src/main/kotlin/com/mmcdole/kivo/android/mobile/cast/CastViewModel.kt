package com.mmcdole.kivo.android.mobile.cast

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.android.mobile.app.PlayerRoute
import com.mmcdole.kivo.android.mobile.ui.display
import com.mmcdole.kivo.android.mobile.ui.sentence
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.cast.CastStatus
import com.mmcdole.kivo.core.display.problem
import com.mmcdole.kivo.core.session.KivoSession
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.Job
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** Why a cast stopped or couldn't start: a receiver's error code and an admin's words, or a sentence already worded. */
sealed interface CastProblem {
    data class Receiver(val code: String, val message: String?) : CastProblem

    data class Worded(val sentence: String) : CastProblem

    /** The TV can't reach the server, and keeps trying. */
    data object Unreachable : CastProblem
}

data class CastUiState(
    val link: LinkState = LinkState.Unavailable,
    /** What the receiver plays, by its media status. */
    val media: CastMedia? = null,
    /** What the receiver says of its play. */
    val status: CastStatus? = null,
    /** The work on the TV. */
    val work: WorkDetail? = null,
    /** Where the TV is, between its status updates. */
    val positionMs: Long = 0,
    /** A LOAD on its way, by its work, until the receiver's media names it. */
    val loading: String? = null,
    val problem: CastProblem? = null,
    val volume: Float = 0f,
) {
    val connected: Boolean get() = link is LinkState.Connected
    val device: String? get() = (link as? LinkState.Connected)?.device

    /** Something is on the TV, on its way, or stopped with an error: the mini controller and the remote show. */
    val active: Boolean get() = connected && (media != null || loading != null || problem != null)

    val playing: Boolean get() = media?.state == CastMedia.State.Playing

    /** Loading or buffering on the TV: a spinner takes play's place. */
    val waiting: Boolean get() = media?.let { it.state == CastMedia.State.Loading || it.state == CastMedia.State.Buffering } ?: (loading != null)

    val durationMs: Long get() = media?.durationMs?.takeIf { it > 0 } ?: work?.durationMs?.toLong() ?: 0
}

/**
 * Casting to a TV over [link]: the session with a Cast device and what its
 * receiver plays, which the Cast button, the mini controller and the remote
 * show. A LOAD carries a cast token, so the TV plays as this user; the
 * receiver starts, heartbeats and changes its play itself, and this model
 * only mirrors its status and sends its messages. A session that connects
 * newly, rather than one joined again, is said on [connections], so a play
 * here carries on on the TV.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class CastViewModel(private val session: KivoSession, private val link: CastLink?) : ViewModel() {
    private val mutableState = MutableStateFlow(CastUiState())
    val state: StateFlow<CastUiState> = mutableState.asStateFlow()

    private val connected = Channel<Unit>(Channel.CONFLATED)

    /** Each Cast session that connects newly. */
    val connections = connected.receiveAsFlow()

    /** Whether Cast is offered: Google Play services is here. */
    val offered: Boolean = link != null

    private var reading: Job? = null
    private var ticking: Job? = null

    /** The last cast asked for, for Try again before the TV has a play. */
    private var lastCast: PlayerRoute? = null

    init {
        if (link != null) {
            viewModelScope.launch { link.state.collect(::linkChanged) }
            viewModelScope.launch { link.media.collect(::mediaChanged) }
            viewModelScope.launch { link.volume.collect { v -> mutableState.update { it.copy(volume = v) } } }
            viewModelScope.launch { link.messages.collect { message -> CastStatus.parseOrNull(message)?.let(::statusChanged) } }
        }
    }

    /** Casts [route] from its position, or its start with fromStart, else its resume position; false when no TV is connected. */
    fun cast(route: PlayerRoute): Boolean {
        val link = link ?: return false
        val device = state.value.device ?: return false
        lastCast = route
        mutableState.update { it.copy(loading = route.workId, problem = null, status = null) }
        read(route.workId)
        viewModelScope.launch {
            try {
                val load = session.cast.load(
                    device, route.workId, route.versionId, route.audioTrackId, route.subtitleTrackId,
                    route.positionMs ?: if (route.fromStart) 0 else null,
                )
                val work = state.value.work?.takeIf { it.id == route.workId }
                val art = work?.images?.let { it.poster ?: it.still ?: it.backdrop }
                link.load(load, work?.title.orEmpty(), art?.let { session.imageUrl(it.id, 480) })
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                if (state.value.loading == route.workId) {
                    mutableState.update { it.copy(loading = null, problem = CastProblem.Worded(sentence(RequestProblem.of(e)))) }
                }
            }
        }
        return true
    }

    fun togglePlay() {
        if (state.value.playing) link?.pause() else link?.play()
    }

    fun skip(deltaMs: Long) = seek(state.value.positionMs + deltaMs)

    fun seek(positionMs: Long) {
        val target = positionMs.coerceIn(0, (state.value.durationMs - 1000).coerceAtLeast(0))
        mutableState.update { it.copy(positionMs = target) }
        link?.seek(target)
    }

    fun setVolume(volume: Float) {
        mutableState.update { it.copy(volume = volume) }
        link?.setVolume(volume)
    }

    /** Try again: the work cast again, with a fresh token, from where the TV was; a press means someone is watching. */
    fun retry() {
        session.playback.confirmStillWatching()
        val s = state.value
        val workId = s.media?.workId ?: s.status?.workId
        when {
            workId != null -> cast(current(workId).copy(positionMs = s.positionMs.takeIf { it > 0 } ?: lastCast?.positionMs))
            else -> lastCast?.let(::cast)
        }
    }

    /** Stop casting: the TV stops, and what played there is returned to carry on with here, at the TV's position. */
    fun stopCasting(): PlayerRoute? {
        val s = state.value
        val route = (s.media?.workId ?: s.status?.workId)?.let { current(it).copy(positionMs = s.positionMs) }
        link?.end(stopCasting = true)
        return route
    }

    /** Disconnect: this phone lets go, and the TV plays on. */
    fun disconnect() {
        link?.end(stopCasting = false)
    }

    override fun onCleared() {
        link?.close()
    }

    /** [workId] as the TV plays it: its version and tracks. */
    private fun current(workId: String): PlayerRoute {
        val status = state.value.status?.takeIf { it.workId == workId } ?: return PlayerRoute(workId)
        return PlayerRoute(workId, versionId = status.versionId, audioTrackId = status.audioTrackId, subtitleTrackId = status.subtitleTrackId)
    }

    private fun linkChanged(link: LinkState) {
        val was = state.value.link
        if (link !is LinkState.Connected) {
            mutableState.update { it.copy(link = link, media = null, status = null, loading = null, problem = null) }
            return
        }
        mutableState.update { it.copy(link = link) }
        if (link.resumed) {
            // The receiver plays on with a fresh token after this app was away.
            viewModelScope.launch { quietly { session.cast.tokenMessage(link.device) }?.let { this@CastViewModel.link?.send(it) } }
        } else if (was !is LinkState.Connected) {
            connected.trySend(Unit)
        }
        this.link?.send(com.mmcdole.kivo.core.cast.CastMessages.getStatus())
    }

    private fun mediaChanged(media: CastMedia?) {
        mutableState.update { s ->
            if (media == null) {
                s.copy(media = null)
            } else {
                s.copy(media = media, positionMs = media.positionMs, loading = s.loading.takeIf { it != media.workId }, problem = null)
            }
        }
        media?.let { read(it.workId) }
        media?.status?.let(CastStatus::parseOrNull)?.let(::statusChanged)
        tick()
    }

    private fun statusChanged(status: CastStatus) {
        status.workId?.let(::read)
        mutableState.update { s ->
            val problem = when {
                status.unreachable -> CastProblem.Unreachable
                status.error != null -> CastProblem.Receiver(status.error!!, status.message)
                else -> s.problem
            }
            s.copy(status = status, problem = problem, loading = if (status.error != null && !status.unreachable) null else s.loading)
        }
    }

    /** Keeps the position moving while the TV plays. */
    private fun tick() {
        ticking?.cancel()
        if (!state.value.playing) return
        ticking = viewModelScope.launch {
            while (true) {
                delay(TICK_MS)
                val position = link?.positionMs ?: return@launch
                mutableState.update { it.copy(positionMs = position) }
            }
        }
    }

    /** Reads the work on the TV, for its title and artwork. */
    private fun read(workId: String) {
        if (state.value.work?.id == workId && reading?.isActive == true) return
        reading?.cancel()
        reading = viewModelScope.launch {
            session.library.work(workId).collect { load ->
                (load as? Load.Loaded)?.value?.let { work -> mutableState.update { it.copy(work = work) } }
            }
        }
    }

    private companion object {
        const val TICK_MS = 500L
    }
}

/** A receiver's error in words: an admin's own when they stopped the play. */
fun CastProblem.words(unreachable: String, signedOut: String, stopped: String, outdated: String, receiverOutdated: String, notPlayable: String): String = when (this) {
    is CastProblem.Worded -> sentence
    CastProblem.Unreachable -> unreachable
    is CastProblem.Receiver -> message?.takeIf { it.isNotEmpty() } ?: when (code) {
        "signedOut" -> signedOut
        "playEnded" -> stopped
        "serverOutdated" -> outdated
        "receiverOutdated" -> receiverOutdated
        "notPlayable", "noFallback", "playbackFailed" -> notPlayable
        "invalidRequest" -> display.problem("")
        else -> display.problem(code)
    }
}

/** [read]'s answer; null when it fails. */
private suspend fun <T> quietly(read: suspend () -> T): T? = try {
    read()
} catch (e: CancellationException) {
    throw e
} catch (_: Exception) {
    null
}
