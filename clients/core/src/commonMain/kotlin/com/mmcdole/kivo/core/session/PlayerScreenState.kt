package com.mmcdole.kivo.core.session

import com.mmcdole.kivo.api.models.Version
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.core.display.QualityOption
import com.mmcdole.kivo.core.downloads.DownloadSource
import com.mmcdole.kivo.core.play.PlayChoice
import com.mmcdole.kivo.core.play.PlayerUi

/**
 * What a video player's screen shows around the video ([VideoPlayer.screen]),
 * apart from the play itself, which is the controller's state.
 */
data class PlayerScreenState(
    /** The controls, Skip Intro, Recap or Credits, and up next. */
    val ui: PlayerUi = PlayerUi(),
    /** The work playing: as it was downloaded at once when it is, then as the server has it. */
    val work: WorkDetail? = null,
    /** An episode's next one, once read: without the server, only when it's downloaded. */
    val next: WorkDetail? = null,
    /** The version the play plays, once the work is read. */
    val version: Version? = null,
    /** Auto, Original and the server's ladder below [version]'s own bitrate. */
    val qualities: List<QualityOption> = emptyList(),
    /** The play's scrub previews, once they're ready. */
    val previews: ScrubPreviews? = null,
    /** Playing a download from its files, with no server. */
    val offline: Boolean = false,
    /** Where a download playing came from, for the stream info's "Source" line. */
    val downloadSource: DownloadSource? = null,
    /** Where the player goes now, for the app to take it there; null while it plays on. */
    val exit: PlayerExit? = null,
)

/** Where a video player goes when it's done. */
sealed interface PlayerExit {
    /** The next episode, from its download [downloadId] when one is ready here. */
    data class Next(val choice: PlayChoice, val downloadId: String?) : PlayerExit

    /** The last episode ended, played from the server: its series' page. */
    data class Series(val seriesId: String) : PlayerExit

    /** Anything else ended, a download among them, or a download couldn't play: back where playback started. */
    data object Back : PlayerExit
}
