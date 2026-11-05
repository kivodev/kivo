package com.mmcdole.kivo.android.player

import androidx.media3.common.MediaMetadata
import com.mmcdole.kivo.core.player.PlayerEngine

/** The [PlayerEngine] the video player plays through: [Media3Engine] on a device. */
interface VideoEngine : PlayerEngine {
    /** What Now Playing shows of the video: its title, and an episode's series and code. */
    var metadata: MediaMetadata

    /** How fast the video plays, 1 being normal, with the audio's pitch kept; it holds for every play the engine loads. */
    var speed: Float

    /** Plays the episode before, for the media session's previous; null when there's none, and the session doesn't offer previous. */
    var playPrevious: (() -> Unit)?

    /** Plays the next episode, for the media session's next; null when there's none, and the session doesn't offer next. */
    var playNext: (() -> Unit)?
}
