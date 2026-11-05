package com.mmcdole.kivo.android.player

import com.mmcdole.kivo.api.models.CapabilityMismatch
import com.mmcdole.kivo.api.models.PlayDelivery
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.PlayTrack
import com.mmcdole.kivo.api.models.QualityRequest
import com.mmcdole.kivo.api.models.TrackDelivery
import com.mmcdole.kivo.api.models.TrackType
import com.mmcdole.kivo.api.models.Version
import com.mmcdole.kivo.core.display.Display
import com.mmcdole.kivo.core.display.bitrate
import com.mmcdole.kivo.core.display.delivered
import com.mmcdole.kivo.core.player.Subtitles

/** One line of the stream info overlay, with why it isn't direct. */
data class InfoRow(val label: String, val value: String, val why: String? = null)

/** The overlay's own words, from the app's strings; the rest come from the display tables. */
data class StreamInfoWords(
    val playback: String,
    val container: String,
    val video: String,
    val audio: String,
    val subtitles: String,
    val quality: String,
    val delivery: String,
    val bitrate: String,
    val off: String,
    val hls: String,
    val file: String,
    val local: String,
    val remote: String,
    /** "MKV → HLS stream". */
    val toHls: (container: String) -> String,
    /** "This device can't play MKV files." */
    val cantPlayContainer: (container: String) -> String,
    /** "Original: the file (1.9 Mb/s) is under your 8 Mb/s limit". */
    val originalUnder: (file: String, cap: String) -> String,
    /** "Limited to 8 Mb/s". */
    val limitedTo: (cap: String) -> String,
    /** The capability a track failed: "maxLevel 5.1; this device: 4.1". */
    val limit: (CapabilityMismatch) -> String,
)

private val styledCodecs = setOf("ass", "ssa")

/**
 * The stream info overlay's lines, as the web player's StreamInfo builds
 * them, from the play's tracks: the headline is how its video is
 * delivered; each selected track says its own way, and why when it isn't
 * from the file, with the capability it failed; the subtitle line follows
 * [subtitleTrackId], the subtitle showing, chosen during playback or not;
 * the quality line says when a limit the file is already under plays the
 * original.
 */
fun streamInfo(
    play: PlayStarted,
    version: Version?,
    quality: QualityRequest?,
    subtitleTrackId: String,
    display: Display,
    words: StreamInfoWords,
): List<InfoRow> {
    fun sourceLabel(t: PlayTrack): String? = version?.tracks?.firstOrNull { it.id == t.trackId }?.fullName
    fun outputLabel(t: PlayTrack): String? {
        if (t.transcode == null) return null
        val out = display.delivered(t)
        return listOfNotNull(out, t.transcode?.encoder.takeIf { t.type == TrackType.Video }).joinToString(" · ")
    }
    fun why(t: PlayTrack): String? {
        val reason = t.reason?.takeIf { it != "containerUnsupported" } ?: return null
        return display.reason(reason) + (t.mismatch?.let { " (${words.limit(it)})" } ?: "")
    }
    val names = mapOf(TrackType.Video to words.video, TrackType.Audio to words.audio)
    val selected = play.tracks.filter { it.selected == true }

    val rows = mutableListOf(InfoRow(words.playback, display.method(play)))
    // Repackaging is the container's doing, said once on its own row.
    val container = version?.container?.let(display::container)
    if (play.delivery == PlayDelivery.Stream && container != null) {
        val repackaged = selected.any { it.reason == "containerUnsupported" }
        rows += InfoRow(words.container, words.toHls(container), if (repackaged) words.cantPlayContainer(container) else null)
    }
    for (t in selected) {
        if (t.type == TrackType.Subtitle) continue
        // A file's track with what it became is a download's conversion, which is what the file holds.
        val what = if (t.delivery == TrackDelivery.File && t.transcode != null) outputLabel(t).orEmpty()
        else listOfNotNull(sourceLabel(t), outputLabel(t)).joinToString(" → ")
        val method = display.method(t)
        rows += InfoRow(
            names[t.type] ?: t.type.value,
            if (what.isEmpty()) method else "$method · $what",
            why(t).takeIf { t.delivery != TrackDelivery.File },
        )
    }

    val shown = play.tracks.firstOrNull { it.type == TrackType.Subtitle && it.trackId == subtitleTrackId }
    if (subtitleTrackId == Subtitles.OFF || shown == null) {
        rows += InfoRow(words.subtitles, words.off)
    } else {
        val track = version?.tracks?.firstOrNull { it.id == subtitleTrackId }
        // A styled track shown plain doesn't name its codec: "Plain (ASS)" would read as styled.
        val plainStyled = track != null && track.codec in styledCodecs && shown.delivery != TrackDelivery.Burn && shown.codec !in styledCodecs
        val kind = display.subtitleKind(shown) + (if (track != null && !plainStyled) " (${display.codec(track.codec)})" else "")
        val value = listOfNotNull(track?.name, kind, track?.subtitle?.source?.let(display::subtitleSource)).joinToString(" · ")
        rows += InfoRow(words.subtitles, value, why(shown))
    }

    val cap = quality?.takeIf { it.mode == "limit" }?.maxBitrateBps
    if (cap != null) {
        val limited = selected.any { it.reason == "clientQualityLimit" }
        val source = version?.bitrateBps
        rows += InfoRow(
            words.quality,
            if (!limited && source != null && source <= cap) words.originalUnder(display.bitrate(source), display.bitrate(cap)) else words.limitedTo(display.bitrate(cap)),
        )
    }
    rows += InfoRow(words.delivery, listOf(if (play.delivery == PlayDelivery.Stream) words.hls else words.file, if (play.local) words.local else words.remote).joinToString(" · "))
    play.bitrateBps?.let { rows += InfoRow(words.bitrate, display.bitrate(it)) }
    return rows
}
