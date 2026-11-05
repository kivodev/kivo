package com.mmcdole.kivo.core.downloads

import com.mmcdole.kivo.api.models.PlayFallback
import com.mmcdole.kivo.api.models.PlayUpdate
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.PlayCreate
import com.mmcdole.kivo.api.models.PlayPart
import com.mmcdole.kivo.api.models.PlayStop
import com.mmcdole.kivo.api.models.PlayTrack
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.core.display.Display
import com.mmcdole.kivo.core.display.trackDetail
import com.mmcdole.kivo.core.downloads.db.Download
import com.mmcdole.kivo.core.play.PlaySource
import com.mmcdole.kivo.core.player.PlayTracks
import com.mmcdole.kivo.core.player.Subtitles
import com.mmcdole.kivo.core.player.TrackShow
import com.mmcdole.kivo.api.models.Download as ServerDownload

/**
 * A download playing with no server: [plays] gives the player its files here,
 * through the same controller, engine and subtitle drawing as streaming, and
 * keeps each position it reports in the progress outbox. [work] is the work
 * as it was downloaded, to show.
 */
internal class OfflinePlay internal constructor(
    private val downloads: Downloads,
    private val id: String,
    play: PlayStarted,
    val work: WorkDetail,
    private val plan: ServerDownload,
) {
    val plays: PlaySource = Plays(play)

    /** Where the file playing came from, for the stream info's "Source" line. */
    fun source(display: Display): DownloadSource {
        if (plan.delivery != "conversion") return DownloadSource(null)
        val video = plan.tracks.firstOrNull { it.type == "video" && it.selected == true }
        val from = work.versions.firstOrNull { it.id == plan.versionId }?.tracks?.firstOrNull { it.id == video?.trackId }
        return DownloadSource(from?.let(display::trackDetail))
    }

    private inner class Plays(private val play: PlayStarted) : PlaySource {
        /**
         * The download's play, at the request's position. A subtitle the
         * request chooses, or off, shows where the play shows it as it is:
         * from a file the download has, or one its file carries. One burned
         * into the picture stays, as does the audio, the one track the
         * download has.
         */
        override suspend fun start(request: PlayCreate): PlayStarted {
            val started = request.positionMs?.let { play.copy(startMs = it) } ?: play
            val chosen = request.subtitleTrackId ?: return started
            return when (PlayTracks.show(play, chosen)) {
                TrackShow.NewPlay, TrackShow.Burned -> started
                else -> started.copy(
                    tracks = started.tracks.map { track ->
                        if (track.type != "subtitle") track else track.copy(selected = true.takeIf { track.trackId == chosen })
                    },
                )
            }
        }

        override suspend fun heartbeat(id: String, heartbeat: PlayUpdate) {
            downloads.record(this@OfflinePlay.id, heartbeat.positionMs.toLong())
        }

        /** The files are here: there's no server to wait for. */
        override suspend fun reconnect(id: String, heartbeat: PlayUpdate) = heartbeat(id, heartbeat)

        override suspend fun stop(id: String, stop: PlayStop) {
            stop.positionMs?.let { downloads.record(this@OfflinePlay.id, it.toLong()) }
            downloads.played()
        }

        override suspend fun fallback(id: String, request: PlayFallback): PlayStarted =
            throw UnsupportedOperationException("a download has no transcode to fall back to")

    }
}

/**
 * A downloaded file, as the stream info's "Source" line says it: for a
 * conversion the video it was [convertedFrom], "HEVC 1080p"; null for the
 * file as the server has it.
 */
data class DownloadSource(val convertedFrom: String?)

/**
 * A download's play as the player plays it offline: the server's [plan]
 * of it, every URL its file here, starting at [positionMs]. The file plays
 * directly: its video and audio are the file's, the source's tracks for an
 * original and, for a conversion, what the plan made of them. A subtitle
 * file shows at once where it arrived, and a subtitle the file carries as
 * the plan says; a subtitle file that didn't arrive, and anything the plan
 * streams, takes a new play, which offline is the same play.
 */
internal fun Downloads.startedOffline(row: Download, plan: ServerDownload, work: WorkDetail, positionMs: Long): PlayStarted {
    val files = files(row.id)
    val here = files.filter { it.state == Downloads.DONE }.associateBy { it.url }
    val parts = files.filter { it.role == Downloads.PART }.sortedBy { it.name.substringAfter('-').substringBefore('.').toInt() }
    val partUrls = parts.map { fileUrl(path(it).toString()) }
    val local = { url: String -> here[url]?.let { fileUrl(path(it).toString()) } }
    return PlayStarted(
        id = "download-${row.id}",
        workId = row.work_id,
        versionId = plan.versionId,
        startMs = positionMs.coerceIn(0, plan.durationMs.toLong()).toInt(),
        durationMs = plan.durationMs,
        local = true,
        delivery = "file",
        url = partUrls.first(),
        tracks = offlineTracks(plan.tracks, local),
        parts = plan.parts.mapIndexed { i, p -> PlayPart(durationMs = p.durationMs, url = partUrls[i]) },
        chapters = plan.chapters,
        markers = plan.markers,
    )
}

/**
 * A download's [tracks] as its file plays here: the selected video and
 * audio from the file, a conversion's saying what it became but not how;
 * a subtitle file at its [local] URL, or without a delivery where it didn't
 * arrive, with its fonts that did; and nothing streamed. Off shows when the
 * chosen subtitle can't.
 */
internal fun offlineTracks(tracks: List<PlayTrack>, local: (String) -> String?): List<PlayTrack> {
    val offline = tracks.map { t ->
        val media = t.type != SUBTITLE && t.selected == true
        when {
            media -> t.copy(delivery = "file", reason = null, mismatch = null, transcode = t.transcode?.copy(bitrateBps = null, encoder = null))
            t.delivery == "external" -> {
                val url = t.url?.let(local)
                if (url == null) {
                    t.copy(delivery = null, selected = null, url = null, fonts = null)
                } else {
                    t.copy(url = url, fonts = t.fonts?.mapNotNull(local))
                }
            }
            t.delivery == "stream" -> t.copy(delivery = null, selected = null)
            else -> t
        }
    }
    if (offline.any { it.type == SUBTITLE && it.selected == true }) return offline
    return offline.map { if (it.trackId == Subtitles.OFF && it.delivery != null) it.copy(selected = true) else it }
}

private const val SUBTITLE = "subtitle"

/** A `file://` URL for an absolute path, each segment escaped. */
internal fun fileUrl(path: String): String = "file://" + path.split('/').joinToString("/") { segment ->
    buildString {
        for (b in segment.encodeToByteArray()) {
            val c = b.toInt().toChar()
            if (c.isLetterOrDigit() && b >= 0 || c in "-._~") append(c) else append('%').append(((b.toInt() and 0xff) or 0x100).toString(16).substring(1).uppercase())
        }
    }
}
