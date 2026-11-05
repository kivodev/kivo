package com.mmcdole.kivo.android.player

import android.content.Context
import androidx.annotation.OptIn
import androidx.core.net.toUri
import androidx.media3.common.AudioAttributes
import androidx.media3.common.C
import androidx.media3.common.ForwardingPlayer
import androidx.media3.common.MediaItem
import androidx.media3.common.MediaMetadata
import androidx.media3.common.MimeTypes
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.common.TrackSelectionOverride
import androidx.media3.common.Tracks
import androidx.media3.common.util.UnstableApi
import androidx.media3.datasource.DefaultDataSource
import androidx.media3.exoplayer.DefaultRenderersFactory
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.audio.AudioCapabilities
import androidx.media3.exoplayer.audio.AudioSink
import androidx.media3.exoplayer.audio.DefaultAudioSink
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory
import androidx.media3.exoplayer.upstream.DefaultBandwidthMeter
import androidx.media3.extractor.DefaultExtractorsFactory
import androidx.media3.session.MediaSession
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.PlayTrack
import com.mmcdole.kivo.core.caps.AudioSettings
import com.mmcdole.kivo.core.caps.PassthroughMode
import com.mmcdole.kivo.core.player.Markers
import com.mmcdole.kivo.core.player.PartTimeline
import com.mmcdole.kivo.core.player.PlayerError
import com.mmcdole.kivo.core.player.PlayerEngine
import com.mmcdole.kivo.core.player.TrackShow
import io.github.peerless2012.ass.media.AssHandler
import io.github.peerless2012.ass.media.factory.AssRenderersFactory
import io.github.peerless2012.ass.media.kt.withAssMkvSupport
import io.github.peerless2012.ass.media.parser.AssSubtitleParserFactory
import io.github.peerless2012.ass.media.type.AssRenderType
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

/** The loaded play's player, for the screen's surface: Media3's and its libass handler. */
data class LoadedPlayer(val player: ExoPlayer, val ass: AssHandler)

/**
 * The Android [VideoEngine]: Media3's ExoPlayer, built
 * afresh for each play so a play's decoders never outlive it, with
 * the FFmpeg audio extension for what the platform can't decode, the audio
 * sink passing through what the HDMI sink takes unless Passthrough is Off, styled ASS
 * drawn by libass (ass-media) from the play's prepared file or the played
 * file, with a file's fonts loaded before the first frame, and WebVTT and
 * PGS (a `.sup` file, through [SupParser]) drawn by Media3. A direct
 * play's parts play in turn on the version's timeline, its audio and
 * subtitle tracks selected by their stream index; a stream's subtitles
 * are its renditions.
 *
 * Each player plays as media with the movie content type and handles audio
 * focus, pausing for a call and ducking under a notification, and has a
 * media session, so the Assistant's pause and resume, Google TV's Now
 * Playing and media keys from HDMI-CEC or Bluetooth reach it; its previous
 * and next play the episodes either side.
 *
 * Everything runs on the main thread, which is ExoPlayer's application
 * thread and the controller's.
 */
@OptIn(UnstableApi::class)
class Media3Engine(
    private val context: Context,
    private val session: ServerSession,
    private val scope: CoroutineScope,
    private val settings: suspend () -> Pair<AudioSettings, Boolean>,
    private val onPlayer: (LoadedPlayer?) -> Unit,
) : VideoEngine {
    private var loaded: LoadedPlayer? = null
    private var mediaSession: MediaSession? = null
    private var sessionPlayer: SessionPlayer? = null

    override var metadata: MediaMetadata = MediaMetadata.EMPTY
    override var speed: Float = 1f
        set(value) {
            field = value
            loaded?.player?.setPlaybackSpeed(value)
        }
    override var playPrevious: (() -> Unit)? = null
        set(value) {
            field = value
            sessionPlayer?.episodesChanged()
        }
    override var playNext: (() -> Unit)? = null
        set(value) {
            field = value
            sessionPlayer?.episodesChanged()
        }
    private var events: PlayerEngine.Events? = null
    private var timeline = PartTimeline(emptyList())
    private var job: Job? = null
    private var ticker: Job? = null
    private var ready = false
    private var seeking = false
    private var waiting = false
    private var failed = false
    private var subtitle: TrackShow = TrackShow.Off

    /** The stream index of the file's audio track to play; null for the player's own choice. */
    private var audioIndex: Int? = null

    /** The track IDs of the play's subtitle files, told apart from the media's own text tracks. */
    private var externalTrackIds: Set<String> = emptySet()
    private val fonts = HashMap<String, ByteArray>()
    private val bandwidth = DefaultBandwidthMeter.getSingletonInstance(context)

    /** Where the play starts, which is the position while its player is being built. */
    private var startMs = 0L

    override val positionMs: Long
        get() = loaded?.player?.let { timeline.position(it.currentMediaItemIndex, it.currentPosition) } ?: startMs

    override val paused: Boolean get() = loaded?.player?.playWhenReady != true

    override val bandwidthBps: Long? get() = bandwidth.bitrateEstimate.takeIf { it > 0 }

    override fun load(play: PlayStarted, events: PlayerEngine.Events) {
        unload()
        this.events = events
        timeline = PartTimeline(play.parts.map { it.durationMs.toLong() })
        val externals = externalTracksOf(play)
        externalTrackIds = externals.map { it.trackId }.toSet()
        subtitle = TrackShow.Off
        audioIndex = null
        startMs = play.startMs.toLong()
        job = scope.launch {
            // The styled subtitles' fonts, loaded before the first frame; each URL's bytes never change.
            val needed = externals.filter { it.codec == "ass" }.flatMap { it.fonts.orEmpty() }.distinct()
            val loadedFonts = needed.mapNotNull { url -> font(url)?.let { url to it } }
            val (audio, frameRateMatching) = settings()
            val built = build(audio, frameRateMatching)
            val start = timeline.locate(startMs)
            built.player.setMediaItems(mediaItems(play, externals), start.part, start.offsetMs)
            for ((url, bytes) in loadedFonts) built.ass.addFont(url.substringAfterLast('/'), bytes)
            built.player.prepare()
            val forSession = SessionPlayer(built.player, { playPrevious }, { playNext })
            sessionPlayer = forSession
            mediaSession = MediaSession.Builder(context, forSession).build()
            loaded = built
            applySubtitle()
            applyAudio()
            onPlayer(built)
            ticker = scope.launch {
                while (isActive) {
                    delay(250)
                    if (built.player.isPlaying) this@Media3Engine.events?.onTime()
                }
            }
        }
    }

    override fun unload() {
        job?.cancel()
        ticker?.cancel()
        mediaSession?.release()
        mediaSession = null
        sessionPlayer = null
        loaded?.let {
            it.player.release()
            it.ass.release()
        }
        loaded = null
        onPlayer(null)
        events = null
        ready = false
        seeking = false
        waiting = false
        failed = false
    }

    override fun play() {
        loaded?.player?.play()
    }

    override fun pause() {
        loaded?.player?.pause()
    }

    override fun seek(positionMs: Long) {
        val player = loaded?.player ?: run {
            startMs = positionMs
            return
        }
        val to = timeline.locate(positionMs)
        seeking = true
        player.seekTo(to.part, to.offsetMs)
    }

    override fun showSubtitle(show: TrackShow) {
        subtitle = show
        applySubtitle()
    }

    override fun selectAudio(index: Int) {
        audioIndex = index
        applyAudio()
    }

    private suspend fun font(url: String): ByteArray? {
        fonts[url]?.let { return it }
        return try {
            session.session.bytes(url).also { fonts[url] = it }
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            // Drawn in the fallback font instead.
            null
        }
    }

    private fun build(audio: AudioSettings, frameRateMatching: Boolean): LoadedPlayer {
        val ass = AssHandler(AssRenderType.OVERLAY_OPEN_GL)
        val parsers = AssSubtitleParserFactory(ass)
        val mediaSources = DefaultMediaSourceFactory(
            DefaultDataSource.Factory(context),
            DefaultExtractorsFactory().withAssMkvSupport(parsers, ass),
        ).setSubtitleParserFactory(SupParser.Factory(parsers))
        // Off sends the sink PCM alone, so Media3 or its FFmpeg extension decodes every format.
        @Suppress("DEPRECATION") // The one way to give Media3 a fixed set of encodings; see SinkRenderersFactory.
        val pcm = AudioCapabilities(intArrayOf(C.ENCODING_PCM_16BIT), 8).takeIf { audio.passthrough == PassthroughMode.Off }
        val renderers = SinkRenderersFactory(context, pcm)
            .setExtensionRendererMode(DefaultRenderersFactory.EXTENSION_RENDERER_MODE_ON)
            .setEnableDecoderFallback(true)
        val player = ExoPlayer.Builder(context)
            .setRenderersFactory(AssRenderersFactory(ass, renderers))
            .setMediaSourceFactory(mediaSources)
            .setBandwidthMeter(bandwidth)
            // A media key the screen doesn't take reaches the media session, which skips as the player's own keys do.
            .setSeekBackIncrementMs(Markers.BACK_MS)
            .setSeekForwardIncrementMs(Markers.FORWARD_MS)
            .setAudioAttributes(
                AudioAttributes.Builder().setUsage(C.USAGE_MEDIA).setContentType(C.AUDIO_CONTENT_TYPE_MOVIE).build(),
                /* handleAudioFocus = */ true,
            )
            .build()
        player.setVideoChangeFrameRateStrategy(
            if (frameRateMatching) C.VIDEO_CHANGE_FRAME_RATE_STRATEGY_ONLY_IF_SEAMLESS else C.VIDEO_CHANGE_FRAME_RATE_STRATEGY_OFF,
        )
        player.setPlaybackSpeed(speed)
        ass.init(player)
        player.addListener(listener)
        return LoadedPlayer(player, ass)
    }

    /** The play's subtitle files the app draws, in a format Media3 takes. */
    private fun externalTracksOf(play: PlayStarted): List<PlayTrack> =
        play.tracks.filter { it.delivery == "external" && it.url != null && it.codec in externalTypes }

    /** One item per part for direct play, else the HLS playlist; the play's subtitle files ride on the first. */
    private fun mediaItems(play: PlayStarted, externals: List<PlayTrack>): List<MediaItem> {
        val configurations = externals.map {
            MediaItem.SubtitleConfiguration.Builder(this.session.session.mediaUrl(it.url!!).toUri())
                .setId(it.trackId)
                .setMimeType(externalTypes.getValue(it.codec!!))
                .setSelectionFlags(0)
                .build()
        }
        if (play.delivery == "stream") {
            return listOf(
                MediaItem.Builder()
                    .setUri(this.session.session.mediaUrl(play.url))
                    .setMediaMetadata(metadata)
                    .setMimeType(MimeTypes.APPLICATION_M3U8)
                    .setSubtitleConfigurations(configurations)
                    .build(),
            )
        }
        return play.parts.mapIndexed { index, part ->
            MediaItem.Builder()
                .setUri(this.session.session.mediaUrl(part.url ?: play.url))
                .setMediaMetadata(metadata)
                .setSubtitleConfigurations(if (index == 0) configurations else emptyList())
                .build()
        }
    }

    /** Selects the text track the subtitle choice names, or none. */
    private fun applySubtitle() {
        val player = loaded?.player ?: return
        val params = player.trackSelectionParameters.buildUpon().clearOverridesOfType(C.TRACK_TYPE_TEXT)
        when (val show = subtitle) {
            TrackShow.Off, TrackShow.Burned, TrackShow.NewPlay -> params.setTrackTypeDisabled(C.TRACK_TYPE_TEXT, true)
            else -> {
                // Its track isn't known yet: chosen once the tracks are in.
                val wanted = textGroup(player.currentTracks, show) ?: return
                params.setTrackTypeDisabled(C.TRACK_TYPE_TEXT, false).setOverrideForType(TrackSelectionOverride(wanted.mediaTrackGroup, 0))
            }
        }
        val next = params.build()
        if (next != player.trackSelectionParameters) player.trackSelectionParameters = next
    }

    /** Selects the file's audio track by its stream index, once the tracks are in; else the player's own choice stays. */
    private fun applyAudio() {
        val player = loaded?.player ?: return
        val index = audioIndex ?: return
        val wanted = player.currentTracks.groups.firstOrNull { it.type == C.TRACK_TYPE_AUDIO && fileTrack(it, index) } ?: return
        val next = player.trackSelectionParameters.buildUpon()
            .setOverrideForType(TrackSelectionOverride(wanted.mediaTrackGroup, 0))
            .build()
        if (next != player.trackSelectionParameters) player.trackSelectionParameters = next
    }

    /**
     * The text group that shows [show]: a subtitle file by its track ID, a
     * track of the played file by its stream index, or the stream's
     * rendition at its place among the stream's own text groups, which
     * Media3 lists in the playlist's order.
     */
    private fun textGroup(tracks: Tracks, show: TrackShow): Tracks.Group? {
        val text = tracks.groups.filter { it.type == C.TRACK_TYPE_TEXT }
        val own = text.filter { id(it) !in externalTrackIds }
        return when (show) {
            is TrackShow.External -> text.firstOrNull { id(it) == show.trackId }
            is TrackShow.File -> own.firstOrNull { fileTrack(it, show.index) }
            is TrackShow.Stream -> own.getOrNull(show.position)
            else -> null
        }
    }

    private val listener = object : Player.Listener {
        override fun onPlaybackStateChanged(state: Int) {
            val events = events ?: return
            when (state) {
                Player.STATE_READY -> {
                    if (!ready) {
                        ready = true
                        events.onReady()
                    } else if (seeking) {
                        seeking = false
                        events.onSeeked()
                    }
                    if (waiting) {
                        waiting = false
                        events.onPlaying()
                    }
                }
                Player.STATE_BUFFERING -> if (ready && !seeking) {
                    waiting = true
                    events.onWaiting()
                }
                Player.STATE_ENDED -> events.onEnded()
                else -> Unit
            }
        }

        override fun onRenderedFirstFrame() {
            if (seeking && ready) {
                seeking = false
                events?.onSeeked()
            }
        }

        override fun onPlayWhenReadyChanged(playWhenReady: Boolean, reason: Int) {
            val events = events ?: return
            if (!ready) return
            if (playWhenReady) {
                events.onPlay()
            } else if (reason != Player.PLAY_WHEN_READY_CHANGE_REASON_END_OF_MEDIA_ITEM) {
                events.onPause()
            }
        }

        override fun onIsPlayingChanged(isPlaying: Boolean) {
            if (isPlaying) events?.onPlaying()
        }

        override fun onTracksChanged(tracks: Tracks) {
            applySubtitle()
            applyAudio()
        }

        override fun onPlayerError(error: PlaybackException) {
            if (failed) return
            failed = true
            events?.onFailure(PlayerError(failureCode(error.errorCode), "${error.errorCodeName}: ${error.message}"))
        }
    }
}

/** The subtitle files a play draws, by codec, as Media3 takes them. */
@OptIn(UnstableApi::class)
private val externalTypes = mapOf("webvtt" to MimeTypes.TEXT_VTT, "ass" to MimeTypes.TEXT_SSA, "hdmv_pgs_subtitle" to SUP_MIME)

/** A group's track ID, without the source index Media3 puts before it when a play's subtitle files ride along. */
private fun id(group: Tracks.Group): String? = group.getTrackFormat(0).id?.substringAfter(':')

/**
 * Whether [group] is the played file's track with stream [index]. Media3
 * names a file's track by its number in the container, Matroska's track
 * number or MP4's track ID, which muxers number from 1 in stream order.
 */
private fun fileTrack(group: Tracks.Group, index: Int): Boolean = id(group) == (index + 1).toString()

/** A Media3 error as the fallback request's codes. */
fun failureCode(errorCode: Int): String = when (errorCode) {
    PlaybackException.ERROR_CODE_DECODING_FORMAT_UNSUPPORTED,
    PlaybackException.ERROR_CODE_DECODING_FORMAT_EXCEEDS_CAPABILITIES,
    PlaybackException.ERROR_CODE_PARSING_CONTAINER_UNSUPPORTED,
    PlaybackException.ERROR_CODE_PARSING_MANIFEST_UNSUPPORTED,
    -> "unsupported"
    in 4000..4999, in 5000..5999 -> "mediaDecodeError"
    in 2000..2999 -> "networkError"
    else -> "mediaSourceError"
}

/**
 * The player as the media session offers it: previous and next play the
 * episode before and after ([previous] and [next]), offered only while
 * there is one, rather than moving between a play's parts or starting the
 * video over.
 */
@OptIn(UnstableApi::class)
private class SessionPlayer(
    player: Player,
    private val previous: () -> (() -> Unit)?,
    private val next: () -> (() -> Unit)?,
) : ForwardingPlayer(player) {
    /** The session's listeners, each told the commands this player offers rather than the wrapped player's. */
    private val listeners = mutableMapOf<Player.Listener, Player.Listener>()

    override fun addListener(listener: Player.Listener) {
        val offered = Offered(listener)
        listeners[listener] = offered
        super.addListener(offered)
    }

    override fun removeListener(listener: Player.Listener) {
        listeners.remove(listener)?.let { super.removeListener(it) }
    }

    /** Tells the session what it offers now that an episode either side is known or gone. */
    fun episodesChanged() {
        listeners.values.toList().forEach { it.onAvailableCommandsChanged(availableCommands) }
    }

    private inner class Offered(private val listener: Player.Listener) : Player.Listener by listener {
        override fun onAvailableCommandsChanged(availableCommands: Player.Commands) =
            listener.onAvailableCommandsChanged(this@SessionPlayer.availableCommands)
    }

    override fun getAvailableCommands(): Player.Commands = super.getAvailableCommands().buildUpon()
        .removeAll(*Previous, *Next)
        .addIf(Player.COMMAND_SEEK_TO_PREVIOUS, previous() != null)
        .addIf(Player.COMMAND_SEEK_TO_PREVIOUS_MEDIA_ITEM, previous() != null)
        .addIf(Player.COMMAND_SEEK_TO_NEXT, next() != null)
        .addIf(Player.COMMAND_SEEK_TO_NEXT_MEDIA_ITEM, next() != null)
        .build()

    override fun isCommandAvailable(command: Int): Boolean = when (command) {
        in Previous -> previous() != null
        in Next -> next() != null
        else -> super.isCommandAvailable(command)
    }

    override fun hasPreviousMediaItem(): Boolean = previous() != null

    override fun hasNextMediaItem(): Boolean = next() != null

    override fun seekToPrevious() {
        previous()?.invoke()
    }

    override fun seekToPreviousMediaItem() {
        previous()?.invoke()
    }

    override fun seekToNext() {
        next()?.invoke()
    }

    override fun seekToNextMediaItem() {
        next()?.invoke()
    }

    private companion object {
        val Previous = intArrayOf(Player.COMMAND_SEEK_TO_PREVIOUS, Player.COMMAND_SEEK_TO_PREVIOUS_MEDIA_ITEM)
        val Next = intArrayOf(Player.COMMAND_SEEK_TO_NEXT, Player.COMMAND_SEEK_TO_NEXT_MEDIA_ITEM)
    }
}

/**
 * Media3's renderers with an audio sink that takes exactly the
 * [capabilities] it's given, when given; else the sink follows the HDMI
 * sink as Media3 reads it. A sink with a context always reads the HDMI
 * sink, so a fixed set of formats needs the context-less builder, which is
 * deprecated but the one way Media3 takes a fixed set.
 */
@OptIn(UnstableApi::class)
private class SinkRenderersFactory(context: Context, private val capabilities: AudioCapabilities?) : DefaultRenderersFactory(context) {
    @Suppress("DEPRECATION")
    override fun buildAudioSink(context: Context, enableFloatOutput: Boolean, enableAudioOutputPlaybackParams: Boolean): AudioSink {
        val builder = if (capabilities == null) DefaultAudioSink.Builder(context) else DefaultAudioSink.Builder().setAudioCapabilities(capabilities)
        return builder
            .setEnableFloatOutput(enableFloatOutput)
            .setEnableAudioOutputPlaybackParameters(enableAudioOutputPlaybackParams)
            .build()
    }
}
