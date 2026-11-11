package com.mmcdole.kivo.android.player

import com.mmcdole.kivo.api.models.AudioTrackInfo
import com.mmcdole.kivo.api.models.CapabilityMismatch
import com.mmcdole.kivo.api.models.PlayDelivery
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.PlayPart
import com.mmcdole.kivo.api.models.PlayTrack
import com.mmcdole.kivo.api.models.QualityRequest
import com.mmcdole.kivo.api.models.Resolution
import com.mmcdole.kivo.api.models.SubtitleTrackInfo
import com.mmcdole.kivo.api.models.Track
import com.mmcdole.kivo.api.models.TrackDelivery
import com.mmcdole.kivo.api.models.TrackReason
import com.mmcdole.kivo.api.models.TrackTranscode
import com.mmcdole.kivo.api.models.TrackType
import com.mmcdole.kivo.api.models.Version
import com.mmcdole.kivo.api.models.VersionPart
import com.mmcdole.kivo.api.models.VideoTrackInfo
import com.mmcdole.kivo.core.display.Display
import com.mmcdole.kivo.core.player.SUBTITLES_OFF
import kotlin.test.Test
import kotlin.test.assertEquals

class StreamInfoTest {
    private val display = Display()
    private val words = StreamInfoWords(
        playback = "Playback", container = "Container", video = "Video", audio = "Audio", subtitles = "Subtitles",
        quality = "QualityRequest", delivery = "Delivery", bitrate = "Bitrate", off = "Off", hls = "HLS", file = "File",
        local = "Local", remote = "Remote",
        toHls = { "$it → HLS stream" },
        cantPlayContainer = { "This device can't play $it files." },
        originalUnder = { file, cap -> "Original ($file is under the $cap limit)" },
        limitedTo = { "Limited to $it" },
        limit = { "${it.field} ${it.actual}; this device: ${it.declared}" },
    )

    private val version = Version(
        id = "v", available = true, durationMs = 60_000, sizeBytes = 1, parts = listOf(VersionPart("f", "film.mkv", 60_000, 1)),
        tracks = listOf(
            Track("tv", TrackType.Video, "hevc", true, "HEVC 4K HDR10", "HEVC Main 10 4K HDR10", video = VideoTrackInfo(3840, 2160, resolution = Resolution.FourK, profile = "Main 10", hdr = "hdr10")),
            Track("ta", TrackType.Audio, "truehd", true, "English · 7.1 · TrueHD", "English · TrueHD 7.1 · Default", language = "eng", audio = AudioTrackInfo(8, "7.1")),
            Track("ts", TrackType.Subtitle, "ass", false, "English", "English · ASS · Embedded", language = "eng", subtitle = SubtitleTrackInfo("embedded")),
        ),
        workIds = listOf("w"), name = "4K HDR", container = "matroska,webm", bitrateBps = 30_000_000, resolution = Resolution.FourK, hdr = "hdr10",
    )

    private val play = PlayStarted(
        id = "s", workId = "w", versionId = "v", startMs = 0, durationMs = 60_000, local = true,
        delivery = PlayDelivery.Stream,
        url = "plays/s/hls/index.m3u8",
        tracks = listOf(
            PlayTrack("tv", TrackType.Video, selected = true, delivery = TrackDelivery.Stream, codec = "hevc", hdr = "hdr10", reason = TrackReason.ContainerUnsupported),
            PlayTrack(
                "ta", TrackType.Audio, selected = true, delivery = TrackDelivery.Stream, codec = "aac", transcode = TrackTranscode(channels = 6),
                reason = TrackReason.AudioChannelsUnsupported, mismatch = CapabilityMismatch("maxChannels", "8", "6"),
            ),
            PlayTrack("ts", TrackType.Subtitle, selected = true, delivery = TrackDelivery.Stream, codec = "webvtt", reason = TrackReason.SubtitlePreparing),
            PlayTrack("off", TrackType.Subtitle, delivery = TrackDelivery.None),
        ),
        parts = listOf(PlayPart(60_000)), chapters = emptyList(), bitrateBps = 9_000_000,
    )

    @Test
    fun aDirectStreamSaysWhatEachTrackDoesAndWhy() {
        val rows = streamInfo(play, version, null, "ts", display, words)
        assertEquals(
            listOf(
                InfoRow("Playback", "Direct Stream"),
                InfoRow("Container", "MKV → HLS stream", "This device can't play MKV files."),
                InfoRow("Video", "Direct Stream · HEVC Main 10 4K HDR10"),
                InfoRow(
                    "Audio",
                    "Transcode · English · TrueHD 7.1 · Default → AAC 5.1",
                    "The audio has more channels than this device plays. (maxChannels 8; this device: 6)",
                ),
                InfoRow("Subtitles", "English · Plain · Embedded", "The file's subtitles are being prepared, so they're carried in the stream or burned in for now."),
                InfoRow("Delivery", "HLS · Local"),
                InfoRow("Bitrate", "9 Mb/s"),
            ),
            rows,
        )
    }

    @Test
    fun aLimitTheFileIsUnderPlaysTheOriginalAndSaysSo() {
        val direct = play.copy(
            delivery = PlayDelivery.File,
            url = "plays/s/files/0",
            tracks = listOf(PlayTrack("tv", TrackType.Video, selected = true, delivery = TrackDelivery.File, index = 0), PlayTrack("off", TrackType.Subtitle, selected = true, delivery = TrackDelivery.None)),
            bitrateBps = null,
        )
        val rows = streamInfo(direct, version.copy(bitrateBps = 4_000_000), QualityRequest("limit", maxBitrateBps = 8_000_000), SUBTITLES_OFF, display, words)
        assertEquals(
            listOf(
                InfoRow("Playback", "Direct Play"),
                InfoRow("Video", "Direct Play · HEVC Main 10 4K HDR10"),
                InfoRow("Subtitles", "Off"),
                InfoRow("QualityRequest", "Original (4 Mb/s is under the 8 Mb/s limit)"),
                InfoRow("Delivery", "File · Local"),
            ),
            rows,
        )
    }

    @Test
    fun aSubtitleChosenSinceSaysHowItShows() {
        val withFile = play.copy(
            tracks = play.tracks + PlayTrack("tx", TrackType.Subtitle, delivery = TrackDelivery.External, codec = "ass", url = "plays/s/subtitles/x.ass"),
        )
        val styled = version.copy(tracks = version.tracks + Track("tx", TrackType.Subtitle, "ass", false, "French", "French · ASS · External", language = "fre", subtitle = SubtitleTrackInfo("external")))
        val rows = streamInfo(withFile, styled, null, "tx", display, words)
        assertEquals(InfoRow("Subtitles", "French · Styled (ASS) · External"), rows.single { it.label == "Subtitles" })
    }
}
