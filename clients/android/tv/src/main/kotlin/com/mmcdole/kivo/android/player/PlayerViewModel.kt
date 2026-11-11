package com.mmcdole.kivo.android.player

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.media3.common.MediaMetadata
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.android.watchnext.WatchNext
import com.mmcdole.kivo.api.models.EpisodeInfo
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.PreviewManifest
import com.mmcdole.kivo.api.models.QualityRung
import com.mmcdole.kivo.api.models.SubtitleResult
import com.mmcdole.kivo.api.models.Version
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.core.answer
import com.mmcdole.kivo.core.data.withTrack
import com.mmcdole.kivo.core.display.episodeCode
import com.mmcdole.kivo.core.play.PlayChoice
import com.mmcdole.kivo.core.play.PlayerEnd
import com.mmcdole.kivo.core.player.SUBTITLES_OFF
import com.mmcdole.kivo.android.settings.ScreenSettings
import com.mmcdole.kivo.core.settings.Quality
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.distinctUntilChangedBy
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch

/** Where the player goes when it's done. */
sealed interface PlayerExit {
    data object Back : PlayerExit

    /** Plays [screen] in its place: the next episode, or another version. */
    data class Replace(val screen: Screen.Player) : PlayerExit

    /** The last episode ended: its series' page, in the player's place. */
    data class Series(val seriesId: String) : PlayerExit
}

/**
 * The player's state behind the screen: the core's video player over the
 * Media3 engine, the work and its next episode for up next, and the choices
 * made in the player (subtitle, audio, dialogue boost, quality, speed),
 * which the core shows at once or by a new play at the position.
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
     * with a message when they end elsewhere, and what shows over the video,
     * its [com.mmcdole.kivo.core.session.VideoPlayer.ui], follows its rules.
     */
    val video = server.playback.videoPlayer(engine)

    val controller = video.controller

    var work by mutableStateOf<WorkDetail?>(null)
        private set
    var next by mutableStateOf<WorkDetail?>(null)
        private set

    /** How this TV draws subtitles and sets up the display. */
    var screen by mutableStateOf<ScreenSettings?>(null)
        private set
    var ladder by mutableStateOf<List<QualityRung>>(emptyList())
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

    /** The play's preview thumbnails, once ready: the manifest and its URL, which sheets resolve against. */
    var previews by mutableStateOf<Pair<String, PreviewManifest>?>(null)
        private set

    val exits = Channel<PlayerExit>(Channel.BUFFERED)

    init {
        viewModelScope.launch {
            video.play(args.choice, positionMs = if (args.fromStart) 0 else null)
        }
        viewModelScope.launch { screen = graph.screenSettings.values.first() }
        viewModelScope.launch {
            ladder = quietly { server.playback.qualityLadder() } ?: return@launch // The quality menu offers Auto and Original.
        }
        viewModelScope.launch {
            work = fetch(args.choice.workId)?.also { w ->
                val e = w.episode
                engine.metadata = MediaMetadata.Builder()
                    .setTitle(w.title)
                    .setArtist(e?.let { "${it.seriesTitle} · ${graph.display.episodeCode(it)}" })
                    .build()
                if (e != null) {
                    video.setHasNextEpisode(hasNext = e.nextId != null)
                    engine.playPrevious = ::playPrevious.takeIf { e.previousId != null }
                    engine.playNext = video::playNext.takeIf { e.nextId != null }
                }
            }
            work?.episode?.nextId?.let { next = fetch(it) }
        }
        // Done, the player goes where the core says: the next episode, the series or back.
        viewModelScope.launch {
            video.ui.map { it.end }.distinctUntilChanged().collect { end ->
                exitAt(end, work?.episode)?.let(exits::trySend)
            }
        }
        // Preview thumbnails for the scrub bar, when the play says they're ready.
        viewModelScope.launch {
            controller.state.map { it.play }.distinctUntilChangedBy { it?.previews }.collect { play ->
                previews = null
                // Without them the bar shows the time and chapter alone.
                previews = play?.let { quietly { server.playback.previews(it) } }?.let { it.url to it.manifest }
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

    /** The work as the server says it is; null when it can't say. */
    private suspend fun fetch(id: String): WorkDetail? = server.library.work(id).answer()

    /** A subtitle track, or off: shown as the play's tracks say, at once or by a new play at the position. */
    fun chooseSubtitle(trackId: String) = video.chooseSubtitle(trackId)

    /** The version [play] plays, once the work is read. */
    fun version(play: PlayStarted): Version? = work?.versions?.firstOrNull { it.id == play.versionId }

    /** The subtitle search's results for [play]'s version, best first. */
    suspend fun searchSubtitles(play: PlayStarted): List<SubtitleResult> =
        server.playback.subtitleResults(checkNotNull(version(play)))

    /**
     * Downloads [result] to the server and plays on with it: the version's
     * tracks gain it, and a new play at the position shows it.
     */
    suspend fun downloadSubtitle(play: PlayStarted, result: SubtitleResult) {
        val version = checkNotNull(version(play))
        val track = server.playback.addSubtitle(version, result.id)
        work = work?.withTrack(version.id, track)
        chooseSubtitle(track.id)
    }

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
        val previousId = work?.episode?.previousId ?: return
        exits.trySend(PlayerExit.Replace(Screen.Player(previousId)))
    }

    fun playVersion(versionId: String) {
        exits.trySend(PlayerExit.Replace(Screen.Player(PlayChoice(args.choice.workId, versionId = versionId))))
    }

    override fun onCleared() {
        video.dispose()
        engine.unload()
        WatchNext.refresh(graph.context)
    }
}

/**
 * Where the player goes at the core's [end] of a play: of [episode], or of
 * anything else when it's null; null while it plays on, or for a next
 * episode it doesn't know.
 */
internal fun exitAt(end: PlayerEnd?, episode: EpisodeInfo?): PlayerExit? = when (end) {
    PlayerEnd.PlayNext -> episode?.nextId?.let { PlayerExit.Replace(Screen.Player(it)) }
    PlayerEnd.Series -> episode?.let { PlayerExit.Series(it.seriesId) }
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
