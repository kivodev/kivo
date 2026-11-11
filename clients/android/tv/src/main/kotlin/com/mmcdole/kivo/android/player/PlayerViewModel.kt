package com.mmcdole.kivo.android.player

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.media3.common.MediaMetadata
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.settings.ScreenSettings
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.android.watchnext.WatchNext
import com.mmcdole.kivo.api.models.SubtitleResult
import com.mmcdole.kivo.core.display.episodeCode
import com.mmcdole.kivo.core.play.PlayChoice
import com.mmcdole.kivo.core.player.SUBTITLES_OFF
import com.mmcdole.kivo.core.session.PlayerScreenState
import com.mmcdole.kivo.core.settings.Quality
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch

/** The controls hide this long after the last key while playing. */
private const val CONTROLS_HIDE_MS = 5_000L

/**
 * The player's state behind the screen: the core's video player over the
 * Media3 engine, what it shows around the video, and the choices made in
 * the player (subtitle, audio, dialogue boost, quality, speed), which the
 * core shows at once or by a new play at the position.
 * The engine is [Media3Engine] unless [engineOf] makes another, given what
 * to call with each loaded player.
 */
class PlayerViewModel(
    private val session: ServerSession,
    private val args: Screen.Player,
    engineOf: (onPlayer: (LoadedPlayer?) -> Unit) -> VideoEngine = { onPlayer -> media3Engine(session, onPlayer) },
) : ViewModel() {
    private val graph = session.graph
    private val server = session.session

    var player by mutableStateOf<LoadedPlayer?>(null)
        private set

    val engine = engineOf { player = it }

    /**
     * The core's player: its plays start with this TV's settings, and stop
     * with a message when they end elsewhere, and what it shows around the
     * video, [screen], follows its rules.
     */
    val video = server.playback.videoPlayer(engine, args.choice, CONTROLS_HIDE_MS, positionMs = if (args.fromStart) 0 else null)

    val controller = video.controller

    /** What the core's player shows around the video: the work, its next episode, the qualities, the controls, Skip, up next and where it goes once done. */
    var screen by mutableStateOf(PlayerScreenState())
        private set

    /** How this TV draws subtitles and sets up the display. */
    var screenSettings by mutableStateOf<ScreenSettings?>(null)
        private set

    /** The subtitle track showing, or [SUBTITLES_OFF], as the core's player says. */
    var subtitle by mutableStateOf(SUBTITLES_OFF)
        private set

    /** The audio track playing, as the core's player says. */
    var audio by mutableStateOf<String?>(null)
        private set

    /** How fast this video plays, chosen in the player; every play of it keeps it, and the next video starts at normal. */
    var speed by mutableStateOf(1f)
        private set

    /** Another play in the player's place: the episode before, or another version. */
    val replacements = Channel<Screen.Player>(Channel.BUFFERED)

    init {
        viewModelScope.launch { screenSettings = graph.screenSettings.values.first() }
        viewModelScope.launch { video.screen.collect { screen = it } }
        // The media session's title and its Previous and Next, as the work says.
        viewModelScope.launch {
            video.screen.map { it.work }.distinctUntilChanged { a, b -> a?.id == b?.id && a?.title == b?.title }.collect { w ->
                w ?: return@collect
                val e = w.episode
                engine.metadata = MediaMetadata.Builder()
                    .setTitle(w.title)
                    .setArtist(e?.let { "${it.seriesTitle} · ${graph.display.episodeCode(it)}" })
                    .build()
                engine.playPrevious = ::playPrevious.takeIf { e?.previousId != null }
                engine.playNext = video::playNext.takeIf { e?.nextId != null }
            }
        }
        // The tracks showing, which the core's player shows on the engine itself.
        viewModelScope.launch {
            controller.state.collect {
                subtitle = it.subtitleTrackId
                audio = it.audioTrackId
            }
        }
    }

    /** A subtitle track, or off: shown as the play's tracks say, at once or by a new play at the position. */
    fun chooseSubtitle(trackId: String) = video.chooseSubtitle(trackId)

    /** The subtitle search's results for the version playing, best first. */
    suspend fun searchSubtitles(): List<SubtitleResult> =
        server.playback.subtitleResults(checkNotNull(screen.version))

    /**
     * Downloads [result] to the server and plays on with it: the version's
     * tracks gain it, and a new play at the position shows it.
     */
    suspend fun downloadSubtitle(result: SubtitleResult) = video.addSubtitle(result.id)

    /** Another audio track: from the file at once, or by a new play at the position keeping the subtitle showing. */
    fun chooseAudio(trackId: String) = video.chooseAudio(trackId)

    /** Another version from the position; the tracks chosen belong to the version playing, so they start over. */
    fun chooseVersion(versionId: String) = video.chooseVersion(versionId)

    /** A quality chosen here lasts for this video, at home or away, keeping the subtitle showing. */
    fun chooseQuality(quality: Quality) = video.chooseQuality(quality)

    /** Switches the dialogue boost, kept in this device's settings for later plays, and plays on from the position with it. */
    fun boostDialogue(on: Boolean) = video.boostDialogue(on)

    fun chooseSpeed(speed: Float) {
        this.speed = speed
        engine.speed = speed
    }

    fun playPrevious() {
        val previousId = screen.work?.episode?.previousId ?: return
        replacements.trySend(Screen.Player(previousId))
    }

    fun playVersion(versionId: String) {
        replacements.trySend(Screen.Player(PlayChoice(args.choice.workId, versionId = versionId)))
    }

    override fun onCleared() {
        video.dispose()
        engine.unload()
        WatchNext.refresh(graph.context)
    }
}

/** The device's engine for a play of [session]'s, with this device's audio and frame-rate settings. */
private fun media3Engine(session: ServerSession, onPlayer: (LoadedPlayer?) -> Unit): VideoEngine {
    val graph = session.graph
    return Media3Engine(
        context = graph.context,
        session = session,
        scope = graph.scope,
        settings = { graph.core.settings.first().audio to graph.screenSettings.values.first().matchesFrameRate },
        onPlayer = onPlayer,
    )
}
