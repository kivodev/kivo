package com.mmcdole.kivo.android.mobile.player

import androidx.media3.common.MediaMetadata
import com.mmcdole.kivo.android.player.VideoEngine
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.core.player.PlayerEngine
import com.mmcdole.kivo.core.player.TrackShow

/** An engine that's ready as soon as it loads, and remembers how it was asked to show subtitles. */
internal class FakeEngine : VideoEngine {
    override var metadata: MediaMetadata = MediaMetadata.EMPTY
    override var speed: Float = 1f
    override var playPrevious: (() -> Unit)? = null
    override var playNext: (() -> Unit)? = null
    override var positionMs = 0L
    override var paused = true
    override val bandwidthBps: Long? = null
    val shown = mutableListOf<TrackShow>()
    private var events: PlayerEngine.Events? = null

    override fun load(play: PlayStarted, events: PlayerEngine.Events) {
        this.events = events
        positionMs = play.startMs.toLong()
        events.onReady()
    }

    override fun unload() {
        events = null
    }

    override fun play() {
        paused = false
        events?.onPlay()
        events?.onPlaying()
    }

    override fun pause() {
        paused = true
        events?.onPause()
    }

    override fun seek(positionMs: Long) {
        this.positionMs = positionMs
        events?.onSeeked()
    }

    override fun showSubtitle(show: TrackShow) {
        shown += show
    }

    override fun selectAudio(index: Int) = Unit
}
