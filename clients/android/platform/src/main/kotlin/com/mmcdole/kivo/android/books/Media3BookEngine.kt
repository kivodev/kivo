package com.mmcdole.kivo.android.books

import android.content.Context
import androidx.annotation.OptIn
import androidx.media3.common.AudioAttributes
import androidx.media3.common.C
import androidx.media3.common.MediaItem
import androidx.media3.common.MediaMetadata
import androidx.media3.common.MimeTypes
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.common.util.UnstableApi
import androidx.media3.exoplayer.DefaultRenderersFactory
import androidx.media3.exoplayer.ExoPlayer
import com.mmcdole.kivo.android.player.SessionPlayer
import com.mmcdole.kivo.android.player.SessionPlayers
import com.mmcdole.kivo.android.player.failureCode
import com.mmcdole.kivo.api.models.PlayDelivery
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.core.books.BookAudio
import com.mmcdole.kivo.core.player.SKIP_BACK_MS
import com.mmcdole.kivo.core.player.SKIP_FORWARD_MS
import com.mmcdole.kivo.core.player.PartTimeline
import com.mmcdole.kivo.core.player.PlayerError
import com.mmcdole.kivo.core.player.PlayerEngine
import com.mmcdole.kivo.core.player.TrackShow
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

/**
 * The book player's engine: an audio-only ExoPlayer that plays a book's
 * parts in turn on its timeline, as spoken audio that takes audio focus and
 * pauses as headphones are unplugged, and keeps the device awake enough to
 * stream with the screen off. Its player is offered to the app's media
 * session ([sessions]) as a book loads, where previous and next move by
 * chapter, and withdrawn as it unloads. It keeps one player while a book
 * plays, so a play loaded again where it was, as after a dropped
 * connection, keeps the session's.
 *
 * [mediaUrl] turns a play's URLs, relative to the API, into full ones.
 * Everything runs on the main thread, [scope]'s.
 */
@OptIn(UnstableApi::class)
class Media3BookEngine(
    private val context: Context,
    private val mediaUrl: (String) -> String,
    private val scope: CoroutineScope,
    private val sessions: SessionPlayers,
) : PlayerEngine, BookAudio {
    /** What the notification and lock screen show: the book, its author and cover. */
    var metadata: MediaMetadata = MediaMetadata.EMPTY
        set(value) {
            field = value
            val player = player ?: return
            for (i in 0 until player.mediaItemCount) {
                player.replaceMediaItem(i, player.getMediaItemAt(i).buildUpon().setMediaMetadata(value).build())
            }
        }

    /** The session's previous and next: the chapter before and after; null where there's none. */
    var previous: (() -> Unit)? = null
    var next: (() -> Unit)? = null

    private var player: ExoPlayer? = null
    private var withdraw: (() -> Unit)? = null
    private var events: PlayerEngine.Events? = null
    private var timeline = PartTimeline(emptyList())
    private var ticker: Job? = null
    private var fade: Job? = null
    private var ready = false
    private var seeking = false
    private var waiting = false
    private var failed = false
    private var speed = 1f
    private var startMs = 0L

    override val positionMs: Long
        get() = player?.let { timeline.position(it.currentMediaItemIndex, it.currentPosition) } ?: startMs

    override val paused: Boolean get() = player?.playWhenReady != true

    override val bandwidthBps: Long? get() = null

    override fun load(play: PlayStarted, events: PlayerEngine.Events) {
        this.events = events
        timeline = PartTimeline(play.parts.map { it.durationMs.toLong() })
        startMs = play.startMs.toLong()
        ready = false
        seeking = false
        waiting = false
        failed = false
        val player = player ?: build().also { built ->
            player = built
            withdraw = sessions.offer(SessionPlayer(built, { previous }, { next }))
        }
        val start = timeline.locate(startMs)
        player.playWhenReady = false
        player.setMediaItems(mediaItems(play), start.part, start.offsetMs)
        player.prepare()
        ticker?.cancel()
        ticker = scope.launch {
            while (isActive) {
                delay(250)
                if (player.isPlaying) this@Media3BookEngine.events?.onTime()
            }
        }
    }

    override fun unload() {
        ticker?.cancel()
        ticker = null
        fade?.cancel()
        fade = null
        player?.release()
        player = null
        withdraw?.invoke()
        withdraw = null
        events = null
        ready = false
    }

    override fun play() {
        player?.play()
    }

    override fun pause() {
        player?.pause()
    }

    override fun seek(positionMs: Long) {
        val player = player ?: run {
            startMs = positionMs
            return
        }
        val to = timeline.locate(positionMs)
        seeking = true
        player.seekTo(to.part, to.offsetMs)
    }

    /** A book has no subtitles. */
    override fun showSubtitle(show: TrackShow) = Unit

    /** A book plays its one audio track. */
    override fun selectAudio(index: Int) = Unit

    override fun setSpeed(speed: Double) {
        this.speed = speed.toFloat()
        player?.setPlaybackSpeed(this.speed)
    }

    override fun fadeOut(durationMs: Long) {
        val player = player ?: return
        fade?.cancel()
        fade = scope.launch {
            val from = player.volume
            val steps = (durationMs / FADE_STEP_MS).coerceAtLeast(1)
            for (step in 1..steps) {
                delay(FADE_STEP_MS)
                player.volume = from * (1f - step.toFloat() / steps)
            }
        }
    }

    override fun restoreVolume() {
        fade?.cancel()
        fade = null
        player?.volume = 1f
    }

    private fun build(): ExoPlayer {
        val renderers = DefaultRenderersFactory(context).setExtensionRendererMode(DefaultRenderersFactory.EXTENSION_RENDERER_MODE_ON)
        val player = ExoPlayer.Builder(context, renderers)
            .setSeekBackIncrementMs(SKIP_BACK_MS)
            .setSeekForwardIncrementMs(SKIP_FORWARD_MS)
            .setAudioAttributes(
                AudioAttributes.Builder().setUsage(C.USAGE_MEDIA).setContentType(C.AUDIO_CONTENT_TYPE_SPEECH).build(),
                /* handleAudioFocus = */ true,
            )
            .setHandleAudioBecomingNoisy(true)
            .setWakeMode(C.WAKE_MODE_NETWORK)
            .build()
        player.setPlaybackSpeed(speed)
        player.addListener(listener)
        return player
    }

    /** One item per part, else the HLS playlist of a stream. */
    private fun mediaItems(play: PlayStarted): List<MediaItem> {
        if (play.delivery == PlayDelivery.Stream) {
            return listOf(
                MediaItem.Builder().setUri(mediaUrl(play.url)).setMimeType(MimeTypes.APPLICATION_M3U8).setMediaMetadata(metadata).build(),
            )
        }
        return play.parts.map { MediaItem.Builder().setUri(mediaUrl(it.url ?: play.url)).setMediaMetadata(metadata).build() }
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

        override fun onPlayerError(error: PlaybackException) {
            if (failed) return
            failed = true
            events?.onFailure(PlayerError(failureCode(error.errorCode), "${error.errorCodeName}: ${error.message}"))
        }
    }
}

/** How often the sleep timer's fade lowers the volume. */
private const val FADE_STEP_MS = 100L
