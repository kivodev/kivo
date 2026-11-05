package com.mmcdole.kivo.core.display

import com.mmcdole.kivo.api.models.PlayTrack
import com.mmcdole.kivo.api.models.Track

/** A track's name in a picker: [name] on one line, and [note], the file's own title, dim under it. */
data class TrackName(val name: String, val note: String? = null)

/** How a picker lists the track: the server's [Track.name], with the file's own title under it. */
val Track.pickerName: TrackName get() = TrackName(name, title?.trim()?.takeIf { it.isNotEmpty() })

/**
 * A track of a play as it's delivered, for stream info: its codec and,
 * transcoded, a video's resolution, classed by its width as the server
 * classes versions ("H.264 1080p"), or audio's channels ("AAC 5.1").
 */
fun Display.delivered(track: PlayTrack): String {
    val to = track.transcode
    val size = to?.width?.let { width ->
        resolution(
            when {
                width >= 3200 -> "4k"
                width >= 1700 -> "1080p"
                width >= 1200 -> "720p"
                else -> "sd"
            },
        )
    }
    return listOfNotNull(track.codec?.let(::codec), size ?: to?.channels?.let(::channels)).joinToString(" ")
}
