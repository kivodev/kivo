package com.mmcdole.kivo.core.player

import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.PlayTrack

// How the player shows each choice of a play. The server says it per track
// in the play's tracks: a choice with a delivery shows at once as that
// delivery says, and one without takes a new play at the same position with
// it. The apps keep no rules of their own about which is which.

/** How the player shows a track chosen of a play. */
sealed interface TrackShow {
    /** Select the track inside the played file by its stream [index]. */
    data class File(val index: Int) : TrackShow

    /**
     * Select the HLS stream's rendition whose `STABLE-RENDITION-ID` is
     * [trackId]. A player that can't read that ID takes the rendition at
     * [position] (from 0) among the play's tracks of the type delivered in
     * the stream, as the playlist lists them in the same order.
     */
    data class Stream(val trackId: String, val position: Int) : TrackShow

    /**
     * Draw track [trackId]'s subtitle file at [url], relative to the API
     * root, which is [codec]: `webvtt`, `ass` or `hdmv_pgs_subtitle`. An
     * ASS file's [fonts], relative to the API root too, load before it's
     * drawn.
     */
    data class External(val trackId: String, val url: String, val codec: String, val fonts: List<String>) : TrackShow

    /** Burned into the picture: nothing shows over it. */
    data object Burned : TrackShow

    /** No subtitle. */
    data object Off : TrackShow

    /** Only a new play at the same position, with this track, shows it. */
    data object NewPlay : TrackShow
}

object Subtitles {
    /** The subtitle choice that shows none, as plays say it. */
    const val OFF = "off"
}

/** What a play's tracks say: which show, and how a choice shows. */
object PlayTracks {
    /**
     * The track of [type] (`video`, `audio` or `subtitle`) that [play]
     * shows: for subtitles a track's ID or [Subtitles.OFF]; null when it
     * has no track of the type.
     */
    fun selected(play: PlayStarted, type: String): String? =
        play.tracks.firstOrNull { it.type == type && it.selected == true }?.trackId
            ?: Subtitles.OFF.takeIf { type == SUBTITLE }

    /** How the player shows [trackId] of [play], a track's ID or [Subtitles.OFF]. */
    fun show(play: PlayStarted, trackId: String): TrackShow = show(play.tracks, trackId)

    /** How the player shows [trackId] of a play's [tracks], such as a cast's ([com.mmcdole.kivo.core.cast.CastStatus.tracks]). */
    fun show(tracks: List<PlayTrack>, trackId: String): TrackShow {
        val track = tracks.firstOrNull { it.trackId == trackId } ?: return TrackShow.NewPlay
        return when (track.delivery) {
            "file" -> track.index?.let { TrackShow.File(it) } ?: TrackShow.NewPlay
            "stream" -> TrackShow.Stream(trackId, tracks.filter { it.type == track.type && it.delivery == "stream" }.indexOf(track))
            "external" -> track.url?.let { TrackShow.External(trackId, it, track.codec.orEmpty(), track.fonts.orEmpty()) } ?: TrackShow.NewPlay
            "burn" -> TrackShow.Burned
            "none" -> TrackShow.Off
            else -> TrackShow.NewPlay
        }
    }

    internal fun delivery(play: PlayStarted, trackId: String): String? = play.tracks.firstOrNull { it.trackId == trackId }?.delivery

    private const val SUBTITLE = "subtitle"
}

/**
 * Picture-in-picture and AirPlay, where only the video goes, so the app
 * can't draw a subtitle file over it: a play there asks without subtitle
 * files ([com.mmcdole.kivo.core.caps.withoutExternal]), entering takes a new play only when the subtitle
 * showing is a file the app draws, and leaving takes one only when the play
 * started there burned it in. The video player follows these rules itself
 * ([com.mmcdole.kivo.core.session.VideoPlayer.overlayShows]).
 */
object PictureInPicture {
    /** Whether entering takes a new play: the subtitle showing, [subtitleTrackId] of [play], is a file the app draws. */
    fun enteringNeedsNewPlay(play: PlayStarted, subtitleTrackId: String): Boolean =
        PlayTracks.delivery(play, subtitleTrackId) == "external"

    /** Whether leaving takes a new play: [play], started in picture-in-picture, burned in [subtitleTrackId]. */
    fun leavingNeedsNewPlay(play: PlayStarted, subtitleTrackId: String): Boolean =
        PlayTracks.delivery(play, subtitleTrackId) == "burn"
}
