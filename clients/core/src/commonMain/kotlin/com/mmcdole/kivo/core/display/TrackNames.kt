package com.mmcdole.kivo.core.display

import com.mmcdole.kivo.api.models.PlayTrack
import com.mmcdole.kivo.api.models.Track
import com.mmcdole.kivo.api.models.Version

/** A track's name in a picker: [name] on one line, and [note], the file's own title, dim under it. */
data class TrackName(val name: String, val note: String? = null)

private const val SEPARATOR = " · "

/** The longest note shown whole; a longer title is cut at a word and ends in an ellipsis. */
private const val NOTE_LENGTH = 100

/**
 * Names for one kind's tracks, as a picker lists them, by ID. A name is
 * built from the track's facts alone, never its title: the language,
 * marked when it's the original beside a dub ("Japanese [Original]"), the
 * flags ("English · SDH"), and for audio its channel layout and any
 * lossless or premium format ("English · 7.1 · TrueHD Atmos", "English ·
 * Commentary · 2.0"), and for a downloaded subtitle where it came from
 * ("English · OpenSubtitles"). The file's own title is the
 * [TrackName.note], as written, which for a downloaded subtitle is its
 * release. Tracks whose name and note are both the same add their codec,
 * then External or Downloaded for a subtitle file, and are numbered last.
 */
fun Display.trackNames(tracks: List<Track>): Map<String, TrackName> {
    val parts = mutableMapOf<String, MutableList<String>>()
    val notes = mutableMapOf<String, String>()
    for (t in tracks) {
        parts[t.id] = nameParts(t)
        note(t)?.let { notes[t.id] = it }
    }
    fun key(t: Track) = parts.getValue(t.id).joinToString(SEPARATOR) + "\n" + notes[t.id].orEmpty()
    fun alike() = tracks.groupBy(::key).values.filter { it.size > 1 }

    val telling = listOf<(Track) -> String?>(
        { t -> if (premiumAudio(t) != null || t.codec.isEmpty()) null else codec(t.codec) },
        { t -> t.subtitle?.source?.takeIf { it != "embedded" }?.let(::subtitleSource) },
    )
    for (fact in telling) {
        for (group in alike()) {
            if (group.map(fact).distinct().size > 1) group.forEach { t -> fact(t)?.let { parts.getValue(t.id) += it } }
        }
    }
    for (group in alike()) group.forEachIndexed { i, t -> parts.getValue(t.id) += (i + 1).toString() }
    return tracks.associate { it.id to TrackName(parts.getValue(it.id).joinToString(SEPARATOR), notes[it.id]) }
}

/**
 * A track in full, for stream info: a video's codec, profile, resolution
 * and HDR ("HEVC Main 10 4K Dolby Vision"); audio's language, flags, codec
 * or premium format and channels and Default ("English · E-AC-3 5.1 ·
 * Default", "English · TrueHD Atmos 7.1"); a subtitle's language, flags,
 * codec and where it lives ("English · SDH · SRT · External"); then the
 * file's title as written.
 */
fun Display.trackDetail(track: Track): String {
    val video = track.video
    if (video != null) {
        return listOfNotNull(codec(track.codec), video.profile, video.resolution?.let(::resolution), video.hdr?.let(::hdr)).joinToString(" ")
    }
    val parts = baseName(track).toMutableList()
    track.audio?.let { parts += (premiumAudio(track) ?: codec(track.codec)) + " " + channels(it.layout, it.channels) }
    track.subtitle?.let { s ->
        parts += codec(track.codec)
        subtitleSource(s.source)?.let { parts += it }
    }
    if (track.default) parts += tracks.default
    track.title?.takeIf { it.isNotBlank() }?.let { parts += "“${it.trim()}”" }
    return parts.joinToString(SEPARATOR)
}

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

/**
 * A version's name in the version picker: its edition label, then its
 * video's resolution and HDR ("Final Cut · 4K HDR", "1080p", "4K Dolby
 * Vision"), or Audio without video; empty for an untagged version saved
 * before the server sent its resolution.
 */
fun Display.versionName(version: Version): String =
    listOfNotNull(version.edition, quality(version).ifEmpty { null }).joinToString(" · ")

/** A version's resolution and HDR, or Audio without video. */
private fun Display.quality(version: Version): String {
    val resolution = version.resolution ?: return if (version.tracks.none { it.type == "video" }) tracks.audioOnly else ""
    return listOfNotNull(resolution(resolution), version.hdr?.let(::versionHdr)).joinToString(" ")
}

/** The language, marked as the original beside a dub, and the flags that matter: "English", "Japanese [Original]", "English · SDH". */
private fun Display.baseName(t: Track): List<String> = buildList {
    add(if (t.original == true) "${language(t.language)} [${tracks.flags.original}]" else language(t.language))
    if (t.hearingImpaired == true) add(tracks.flags.hearingImpaired)
    if (t.forced == true) add(tracks.flags.forced)
    if (t.commentary == true) add(tracks.flags.commentary)
}

/**
 * A name's parts before anything tells it from another track's: the
 * language, the flags, then audio's channels and premium format, or where a
 * downloaded subtitle came from.
 */
private fun Display.nameParts(t: Track): MutableList<String> {
    val parts = baseName(t).toMutableList()
    t.audio?.let { audio ->
        parts += channels(audio.layout, audio.channels)
        premiumAudio(t)?.let { parts += it }
    }
    if (t.subtitle?.source == "downloaded") parts += tracks.downloadedFrom
    return parts
}

/** A lossless or premium audio format, which a name always shows: "TrueHD Atmos", "DTS-HD MA", "FLAC". */
private fun premiumAudio(t: Track): String? {
    val audio = t.audio ?: return null
    return audio.profile?.let { media.premiumAudio[t.codec + " " + it] } ?: media.premiumAudio[t.codec]
}

/** The note under a track's name: the file's own title, capped. */
private fun note(t: Track): String? = t.title?.trim()?.takeIf { it.isNotEmpty() }?.let(::capped)

/** A title cut to the note's length at a word, ending in an ellipsis. */
private fun capped(title: String): String {
    if (title.length <= NOTE_LENGTH) return title
    val cut = title.substring(0, NOTE_LENGTH)
    val space = cut.lastIndexOf(' ')
    return (if (space > NOTE_LENGTH / 2) cut.substring(0, space) else cut).trimEnd() + "…"
}
