package com.mmcdole.kivo.core.player

import com.mmcdole.kivo.api.models.PlayStarted

/**
 * The player each platform implements (Media3 on Android, AVPlayer on Apple),
 * driven by the play controller. One engine plays one play's media at a time,
 * every position on the version's timeline across its parts.
 */
interface PlayerEngine {
    /** The position on the version's timeline. */
    val positionMs: Long

    val paused: Boolean

    /** The measured download rate, where the engine measures one. */
    val bandwidthBps: Long?

    /**
     * Loads [play]'s media at its `startMs`, replacing anything loaded,
     * and reports to [events] until [unload] or the next load.
     */
    fun load(play: PlayStarted, events: Events)

    /** Releases the media and the decoder: a leaked decoder breaks the next play on a TV. */
    fun unload()

    fun play()

    fun pause()

    fun seek(positionMs: Long)

    /**
     * Shows the loaded play's subtitle as [show] says: a track of the file
     * or a rendition of the stream selected, a subtitle file drawn, or no
     * subtitle track for [TrackShow.Off] and [TrackShow.Burned]. It comes
     * after each load, and again as the choice changes.
     */
    fun showSubtitle(show: TrackShow)

    /** Plays the loaded file's audio track with stream index [index]. */
    fun selectAudio(index: Int)

    /** What the engine tells the controller, on the controller's thread. */
    interface Events {
        /** Loaded at the play's start position: playback can begin. */
        fun onReady()

        fun onPlay()

        /** Paused by the user or the system; not on reaching the end of a part on the way to the next. */
        fun onPause()

        /** Stalled waiting for data. */
        fun onWaiting()

        /** Frames are moving again. */
        fun onPlaying()

        /** The first frame after a seek showed. */
        fun onSeeked()

        /** The position moved: about four times a second while playing. */
        fun onTime()

        /** The last part played to its end. */
        fun onEnded()

        /** Media failed; the loaded media is spent. Called at most once per load. */
        fun onFailure(failure: PlayerError)
    }
}

/**
 * What failed in the player, by its `PlayerErrorCode`: `mediaDecodeError`,
 * `mediaSourceError`, `networkError` or `unsupported`; [detail] is the
 * player's own error text, for the server log.
 */
data class PlayerError(val code: String, val detail: String)
