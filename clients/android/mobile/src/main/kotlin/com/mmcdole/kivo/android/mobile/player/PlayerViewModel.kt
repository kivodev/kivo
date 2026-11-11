package com.mmcdole.kivo.android.mobile.player

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.media3.common.MediaMetadata
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.mobile.app.PlayerRoute
import com.mmcdole.kivo.android.mobile.ui.display
import com.mmcdole.kivo.android.player.LoadedPlayer
import com.mmcdole.kivo.android.player.Media3Engine
import com.mmcdole.kivo.android.player.VideoEngine
import com.mmcdole.kivo.core.display.episodeCode
import com.mmcdole.kivo.core.play.PlayerState
import com.mmcdole.kivo.core.player.SKIP_BACK_MS
import com.mmcdole.kivo.core.player.SKIP_FORWARD_MS
import com.mmcdole.kivo.core.session.KivoSession
import com.mmcdole.kivo.core.session.PlayerScreenState
import com.mmcdole.kivo.core.settings.Quality
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** The controls hide this long after the last touch while playing. */
private const val CONTROLS_HIDE_MS = 3_000L

data class PlayerUiState(
    /** The play, as the core's controller has it. */
    val playback: PlayerState = PlayerState(),
    /** What the core's player shows around the video: the work, its next episode, the qualities, the controls, Skip, up next and where it goes once done. */
    val screen: PlayerScreenState = PlayerScreenState(),
    /** The loaded play's player, for the video's surface. */
    val surface: LoadedPlayer? = null,
    /** How fast this video plays, 1 being normal. */
    val speed: Float = 1f,
    /** In picture in picture, where only the video shows. */
    val pictureInPicture: Boolean = false,
)

/**
 * The phone's video player: the core's video player over the Media3 engine,
 * as the TV's is, playing the route's download from its files when it has
 * one, and what the screen shows of it, with the speed and whether it's in
 * picture in picture, where the app can't draw subtitles, so the core plays
 * them burned in. The engine is made by [engineOf], given what to call with
 * each loaded player.
 */
class PlayerViewModel(
    session: KivoSession,
    route: PlayerRoute,
    engineOf: (onPlayer: (LoadedPlayer?) -> Unit) -> VideoEngine,
) : ViewModel() {
    private val mutableState = MutableStateFlow(PlayerUiState())
    val state: StateFlow<PlayerUiState> = mutableState.asStateFlow()

    private val engine = engineOf { player -> mutableState.update { it.copy(surface = player) } }

    private val video = session.playback.videoPlayer(engine, route.choice, CONTROLS_HIDE_MS, route.downloadId, route.startMs)

    /** The media session's Previous: the episode before this one, in the player's place. */
    val previous = Channel<String>(Channel.BUFFERED)

    init {
        viewModelScope.launch { video.controller.state.collect { s -> mutableState.update { it.copy(playback = s) } } }
        viewModelScope.launch { video.screen.collect { s -> mutableState.update { it.copy(screen = s) } } }
        // The media session's title and its Previous and Next, as the work says.
        viewModelScope.launch {
            video.screen.map { it.work }.distinctUntilChanged { a, b -> a?.id == b?.id && a?.title == b?.title }.collect { work ->
                work ?: return@collect
                val episode = work.episode
                engine.metadata = MediaMetadata.Builder()
                    .setTitle(work.title)
                    .setArtist(episode?.let { "${it.seriesTitle} · ${display.episodeCode(it)}" })
                    .build()
                engine.playPrevious = episode?.previousId?.let { id -> { previous.trySend(id).let {} } }
                engine.playNext = video::playNext.takeIf { episode?.nextId != null }
            }
        }
    }

    /**
     * What plays now, to carry on with on a TV a Cast session just connected
     * to: the work with its version and tracks, from where it is. The play here stops.
     */
    fun handOff(): PlayerRoute? {
        val playback = state.value.playback
        val play = playback.play ?: return null
        video.controller.stopOnExit()
        return PlayerRoute(play.workId, versionId = play.versionId, audioTrackId = playback.audioTrackId, subtitleTrackId = playback.subtitleTrackId, positionMs = playback.positionMs)
    }

    fun togglePlay() {
        video.userActivity()
        video.controller.togglePlay()
    }

    fun seek(positionMs: Long) {
        video.userActivity()
        video.controller.seek(positionMs)
    }

    /** Back 10 or forward 30 seconds from the controls, which show. */
    fun skip(deltaMs: Long) {
        video.showControls()
        video.controller.skip(deltaMs)
    }

    /** A double tap's skip on one side of the video: someone is watching, and the controls stay as they are. */
    fun nudge(forward: Boolean) {
        video.userActivity()
        video.controller.skip(if (forward) SKIP_FORWARD_MS else -SKIP_BACK_MS)
    }

    /** A tap on the video: the controls hide while playing, and show otherwise. */
    fun toggleControls() = video.toggleControls()

    /** A popup opened over the controls, holding them up and up next's countdown; or it closed. */
    fun setMenuOpen(open: Boolean) = video.setMenuOpen(open)

    /** A finger is on the scrub bar, holding the controls up; or it let go. */
    fun setScrubbing(held: Boolean) = video.setScrubbing(held)

    fun skipMarker() = video.skipMarker()

    fun playNext() = video.playNext()

    fun hideUpNext() = video.hideUpNext()

    fun chooseAudio(trackId: String) = video.chooseAudio(trackId)

    fun chooseSubtitle(trackId: String) = video.chooseSubtitle(trackId)

    fun chooseQuality(quality: Quality) = video.chooseQuality(quality)

    fun chooseVersion(versionId: String) = video.chooseVersion(versionId)

    /** Switches the dialogue boost, kept for later plays, playing on from the position with it. */
    fun boostDialogue(on: Boolean) = video.boostDialogue(on)

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
        video.setOverlayVisible(!active)
    }

    /** The app went to the background or the screen locked, outside picture in picture: the play stops. */
    fun stopOnExit() = video.controller.stopOnExit()

    /** Back in front: the play starts again where it stopped. */
    fun restore() = video.controller.restore()

    fun retry() = video.controller.retry()

    override fun onCleared() {
        video.dispose()
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
