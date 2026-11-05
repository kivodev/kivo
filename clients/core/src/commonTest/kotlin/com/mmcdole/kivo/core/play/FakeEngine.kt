package com.mmcdole.kivo.core.play

import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.core.player.PlayerError
import com.mmcdole.kivo.core.player.PlayerEngine
import com.mmcdole.kivo.core.player.TrackShow

/** A [PlayerEngine] the test drives: it records what the controller asks and reports what the test says. */
class FakeEngine : PlayerEngine {
    override var positionMs = 0L
    override var paused = true
    override var bandwidthBps: Long? = null

    /** The plays loaded, in order. */
    val loads = mutableListOf<PlayStarted>()
    var unloads = 0
    val seeks = mutableListOf<Long>()

    /** How the controller asked each subtitle shown, in order. */
    val subtitles = mutableListOf<TrackShow>()

    /** The stream indexes of the audio tracks the controller selected, in order. */
    val audio = mutableListOf<Int>()

    /** The reports of the latest load, and of earlier ones, which the controller must ignore. */
    val events = mutableListOf<PlayerEngine.Events>()
    val current: PlayerEngine.Events get() = events.last()

    override fun load(play: PlayStarted, events: PlayerEngine.Events) {
        loads += play
        this.events += events
        positionMs = play.startMs.toLong()
        paused = true
    }

    override fun unload() {
        unloads++
    }

    override fun play() {
        paused = false
        current.onPlay()
    }

    override fun pause() {
        paused = true
        current.onPause()
    }

    override fun seek(positionMs: Long) {
        seeks += positionMs
        this.positionMs = positionMs
    }

    override fun showSubtitle(show: TrackShow) {
        subtitles += show
    }

    override fun selectAudio(index: Int) {
        audio += index
    }

    fun fail(code: String = "mediaDecodeError", detail: String = "decoder died") =
        current.onFailure(PlayerError(code, detail))
}
