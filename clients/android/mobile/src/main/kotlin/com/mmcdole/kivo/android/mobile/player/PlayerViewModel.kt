package com.mmcdole.kivo.android.mobile.player

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.media3.common.MediaMetadata
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.mobile.app.PlayerRoute
import com.mmcdole.kivo.android.mobile.app.WorkRoute
import com.mmcdole.kivo.android.mobile.ui.display
import com.mmcdole.kivo.android.player.LoadedPlayer
import com.mmcdole.kivo.android.player.Media3Engine
import com.mmcdole.kivo.android.player.VideoEngine
import com.mmcdole.kivo.api.models.EpisodeInfo
import com.mmcdole.kivo.api.models.QualityRung
import com.mmcdole.kivo.api.models.Version
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.core.answer
import com.mmcdole.kivo.core.display.episodeCode
import com.mmcdole.kivo.core.play.PlayerEnd
import com.mmcdole.kivo.core.play.PlayerState
import com.mmcdole.kivo.core.play.PlayerUi
import com.mmcdole.kivo.core.player.SKIP_BACK_MS
import com.mmcdole.kivo.core.player.SKIP_FORWARD_MS
import com.mmcdole.kivo.core.session.KivoSession
import com.mmcdole.kivo.core.session.ScrubPreviews
import com.mmcdole.kivo.core.session.VideoPlayer
import com.mmcdole.kivo.core.settings.Quality
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.distinctUntilChangedBy
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class PlayerUiState(
    /** The play, as the core's controller has it. */
    val playback: PlayerState = PlayerState(),
    /** What shows over the video by the core's timed rules: the controls, Skip and up next. */
    val overlay: PlayerUi = PlayerUi(),
    /** The loaded play's player, for the video's surface. */
    val surface: LoadedPlayer? = null,
    val work: WorkDetail? = null,
    /** An episode's next one, for up next. */
    val next: WorkDetail? = null,
    /** The server's quality ladder, highest first. */
    val ladder: List<QualityRung> = emptyList(),
    /** How fast this video plays, 1 being normal. */
    val speed: Float = 1f,
    /** The play's scrub thumbnails, once they're ready. */
    val previews: ScrubPreviews? = null,
    /** In picture in picture, where only the video shows. */
    val pictureInPicture: Boolean = false,
    /** Playing a download from its files, with no server. */
    val offline: Boolean = false,
) {
    /** The version playing, once the work is read. */
    val version: Version? get() = work?.versions?.firstOrNull { it.id == playback.play?.versionId }
}

/** Where the player goes when it's done: back, or in its place to another page. */
sealed interface PlayerExit {
    data object Back : PlayerExit

    /** The next episode's player, or the series' page after its last episode. */
    data class Replace(val route: Any) : PlayerExit
}

/**
 * The phone's video player: the core's video player over the Media3 engine,
 * as the TV's is, and what the screen shows of it: the work and its next
 * episode for up next, the quality ladder, the speed, the scrub thumbnails
 * and whether it's in picture in picture, where the app can't draw
 * subtitles, so the core plays them burned in. The engine is made by
 * [engineOf], given what to call with each loaded player. A route with a
 * download plays it from its files, through the core's offline player, as
 * the work was downloaded; the next episode plays its download when
 * [readyDownload] has one ready.
 */
class PlayerViewModel(
    private val session: KivoSession,
    private val route: PlayerRoute,
    private val readyDownload: (workId: String) -> String? = { null },
    engineOf: (onPlayer: (LoadedPlayer?) -> Unit) -> VideoEngine,
) : ViewModel() {
    private val mutableState = MutableStateFlow(PlayerUiState(offline = route.downloadId != null))
    val state: StateFlow<PlayerUiState> = mutableState.asStateFlow()

    private val engine = engineOf { player -> mutableState.update { it.copy(surface = player) } }

    /** The core's player, once it's made: at once from the server, or once a download's play is read. */
    private var video: VideoPlayer? = null

    val exits = Channel<PlayerExit>(Channel.BUFFERED)

    init {
        viewModelScope.launch {
            val player = if (route.downloadId == null) {
                session.playback.videoPlayer(engine)
            } else {
                // A download that can't play now, such as one the availability check just removed, closes the player.
                quietly { session.playback.offlinePlayer(engine, route.downloadId) } ?: return@launch run { exits.trySend(PlayerExit.Back) }
            }
            video = player
            follow(player)
            player.play(route.choice, route.startMs)
        }
        viewModelScope.launch {
            val ladder = quietly { session.playback.qualityLadder() } ?: return@launch // Quality offers Auto and Original.
            mutableState.update { it.copy(ladder = ladder) }
        }
        viewModelScope.launch {
            val work = fetch(route.workId) ?: return@launch
            mutableState.update { it.copy(work = work) }
            val episode = work.episode
            engine.metadata = MediaMetadata.Builder()
                .setTitle(work.title)
                .setArtist(episode?.let { "${it.seriesTitle} · ${display.episodeCode(it)}" })
                .build()
            if (episode != null) {
                engine.playPrevious = ::playPrevious.takeIf { episode.previousId != null }
                episode.nextId?.let { id -> fetch(id)?.let { next -> mutableState.update { it.copy(next = next) } } }
            }
        }
    }

    /** Follows [player]: its play, what shows over it, where it goes once done, and its scrub thumbnails. */
    private fun follow(player: VideoPlayer) {
        val controller = player.controller
        viewModelScope.launch { controller.state.collect { s -> mutableState.update { it.copy(playback = s) } } }
        viewModelScope.launch { player.ui.collect { ui -> mutableState.update { it.copy(overlay = ui) } } }
        // Up next, once the work says whether there's a next episode.
        viewModelScope.launch {
            val episode = state.first { it.work != null }.work?.episode ?: return@launch
            player.setHasNextEpisode(hasNext = episode.nextId != null)
            engine.playNext = player::playNext.takeIf { episode.nextId != null }
        }
        // Done, the player goes where the core says: the next episode, its download when it's ready, the series or back.
        viewModelScope.launch {
            player.ui.map { it.end }.distinctUntilChanged().collect { end ->
                exitAt(end, state.value.work?.episode, readyDownload)?.let(exits::trySend)
            }
        }
        // Scrub thumbnails, when the play says they're ready.
        viewModelScope.launch {
            controller.state.map { it.play }.distinctUntilChangedBy { it?.previews }.collect { play ->
                mutableState.update { it.copy(previews = null) }
                val previews = play?.let { quietly { session.playback.previews(it) } } ?: return@collect
                mutableState.update { it.copy(previews = previews) }
            }
        }
    }

    /** A work as it was downloaded, when it is, then as the server has it. */
    private suspend fun fetch(id: String): WorkDetail? = session.playback.playableWork(id).answer()

    /**
     * What plays now, to carry on with on a TV a Cast session just connected
     * to: the work with its version and tracks, from where it is. The play here stops.
     */
    fun handOff(): PlayerRoute? {
        val playback = state.value.playback
        val play = playback.play ?: return null
        video?.controller?.stopOnExit()
        return PlayerRoute(play.workId, versionId = play.versionId, audioTrackId = playback.audioTrackId, subtitleTrackId = playback.subtitleTrackId, positionMs = playback.positionMs)
    }

    fun togglePlay() {
        video?.userActivity()
        video?.controller?.togglePlay()
    }

    fun seek(positionMs: Long) {
        video?.userActivity()
        video?.controller?.seek(positionMs)
    }

    /** Back 10 or forward 30 seconds from the controls, which show. */
    fun skip(deltaMs: Long) {
        video?.showControls()
        video?.controller?.skip(deltaMs)
    }

    /** A double tap's skip on one side of the video: someone is watching, and the controls stay as they are. */
    fun nudge(forward: Boolean) {
        video?.userActivity()
        video?.controller?.skip(if (forward) SKIP_FORWARD_MS else -SKIP_BACK_MS)
    }

    /** A tap on the video: the controls hide while playing, and show otherwise. */
    fun toggleControls() = video?.toggleControls()

    fun showControls() = video?.showControls()

    fun hideControls() = video?.hideControls()

    /** A popup opened over the controls, holding them up and up next's countdown; or it closed. */
    fun setMenuOpen(open: Boolean) = video?.setMenuOpen(open)

    /** A finger is on the scrub bar, holding the controls up; or it let go. */
    fun setScrubbing(held: Boolean) = video?.setScrubbing(held)

    fun skipMarker() = video?.skipMarker()

    fun playNext() = video?.playNext()

    fun hideUpNext() = video?.hideUpNext()

    fun chooseAudio(trackId: String) = video?.chooseAudio(trackId)

    fun chooseSubtitle(trackId: String) = video?.chooseSubtitle(trackId)

    fun chooseQuality(quality: Quality) = video?.chooseQuality(quality)

    fun chooseVersion(versionId: String) = video?.chooseVersion(versionId)

    /** Switches the dialogue boost, kept for later plays, playing on from the position with it. */
    fun boostDialogue(on: Boolean) = video?.boostDialogue(on)

    /** How fast this video plays; the next starts at normal. */
    fun chooseSpeed(speed: Float) {
        engine.speed = speed
        mutableState.update { it.copy(speed = speed) }
    }

    /**
     * Picture in picture began or ended. While in it the app draws nothing
     * over the video, so a subtitle it draws plays burned in by the server
     * instead, and back out of it as before.
     */
    fun setPictureInPicture(active: Boolean) {
        mutableState.update { it.copy(pictureInPicture = active) }
        video?.setOverlayVisible(!active)
    }

    /** The app went to the background or the screen locked, outside picture in picture: the play stops. */
    fun stopOnExit() = video?.controller?.stopOnExit()

    /** Back in front: the play starts again where it stopped. */
    fun restore() = video?.controller?.restore()

    fun retry() = video?.controller?.retry()

    private fun playPrevious() {
        val previousId = state.value.work?.episode?.previousId ?: return
        exits.trySend(PlayerExit.Replace(PlayerRoute(previousId)))
    }

    override fun onCleared() {
        video?.dispose()
        engine.unload()
    }

    companion object {
        /** The phone's engine for [session]: Media3 with this device's audio settings, its player offered to the app's media session. */
        fun engineFor(session: ServerSession): (onPlayer: (LoadedPlayer?) -> Unit) -> VideoEngine = { onPlayer ->
            val graph = session.graph
            Media3Engine(
                context = graph.context,
                session = session,
                scope = graph.scope,
                settings = { graph.core.settings.first().audio to false },
                onPlayer = onPlayer,
                sessions = graph.sessionPlayers,
            )
        }
    }
}

/**
 * Where the player goes at the core's [end] of [episode]'s play, or of
 * anything else's when it's null; null while it plays on. The next episode
 * plays its download when [readyDownload] has one ready.
 */
internal fun exitAt(end: PlayerEnd?, episode: EpisodeInfo?, readyDownload: (String) -> String? = { null }): PlayerExit? = when (end) {
    PlayerEnd.PlayNext -> episode?.nextId?.let { PlayerExit.Replace(PlayerRoute(it, downloadId = readyDownload(it))) }
    PlayerEnd.Series -> episode?.let { PlayerExit.Replace(WorkRoute(it.seriesId)) }
    PlayerEnd.Close -> PlayerExit.Back
    null -> null
}

/** [read]'s answer; null when it fails. */
private suspend fun <T> quietly(read: suspend () -> T): T? = try {
    read()
} catch (e: CancellationException) {
    throw e
} catch (_: Exception) {
    null
}
