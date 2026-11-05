package com.mmcdole.kivo.android.player

import android.content.Context
import android.media.MediaCodecInfo
import android.media.MediaCodecList
import android.os.Build
import androidx.annotation.OptIn
import androidx.media3.common.AudioAttributes
import androidx.media3.common.C
import androidx.media3.common.Format
import androidx.media3.common.MimeTypes
import androidx.media3.common.util.UnstableApi
import androidx.media3.decoder.ffmpeg.FfmpegLibrary
import androidx.media3.exoplayer.audio.AudioCapabilities
import com.mmcdole.kivo.api.models.SubtitleEntry
import com.mmcdole.kivo.core.caps.AudioAnswer
import com.mmcdole.kivo.core.caps.AudioSink
import com.mmcdole.kivo.core.caps.CapabilityProbe
import com.mmcdole.kivo.core.caps.Measured
import com.mmcdole.kivo.core.caps.PassthroughFormat
import com.mmcdole.kivo.core.caps.VideoAnswer
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/**
 * The device's measurements: the video
 * decoders MediaCodec lists, with their profiles, levels, largest picture
 * and whether each is an accelerator; the HDR formats a decoder outputs
 * and its Dolby Vision profiles, whatever the display, as Jellyfin's
 * Android TV app decides them; the audio decoded here, by the platform or
 * Media3's FFmpeg extension; and the HDMI sink's formats
 * and the channels it takes of each from Media3's AudioCapabilities, read at
 * every start. Media3 plays any audio track of a file and draws the
 * subtitles listed. The core corrects and shapes the report.
 */
@OptIn(UnstableApi::class)
class AndroidCapabilityProbe(private val context: Context) : CapabilityProbe {
    override suspend fun measure(): Measured = withContext(Dispatchers.Default) {
        val decoders = MediaCodecList(MediaCodecList.REGULAR_CODECS).codecInfos.filter { !it.isEncoder }
        val video = mutableListOf<VideoAnswer>()
        val levels = mutableMapOf<String, Double>()
        val hdr10 = mutableSetOf<VideoCodec>()
        val hdr10Plus = mutableSetOf<VideoCodec>()
        for (codec in VideoCodec.entries) {
            for (info in decoders.filter { codec.mime in it.supportedTypes.map(String::lowercase) }) {
                val accelerated = isAccelerated(info)
                val type = info.supportedTypes.first { it.equals(codec.mime, ignoreCase = true) }
                val capabilities = info.getCapabilitiesForType(type)
                val sizes = capabilities.videoCapabilities
                for (pl in capabilities.profileLevels) {
                    val (profile, bitDepth) = profileOf(codec, pl.profile) ?: continue
                    video += VideoAnswer(codec.codec, profile, bitDepth, accelerated, sizes?.supportedWidths?.upper, sizes?.supportedHeights?.upper)
                    if (hdr10Profile(codec, pl.profile)) hdr10 += codec
                    if (hdr10PlusProfile(codec, pl.profile)) hdr10Plus += codec
                    levelOf(codec, pl.level)?.let { level -> levels[codec.codec] = maxOf(levels[codec.codec] ?: 0.0, level) }
                }
            }
        }

        // HDR: HDR10 and HDR10+ need a decoder's HDR profile, and HLG plays on
        // any 10-bit decoder. The display isn't asked: Android and the TV map
        // HDR to a screen that doesn't show it.
        val hdr = VideoCodec.entries.associate { codec ->
            codec.codec to buildList {
                if (codec in hdr10) add("hdr10")
                if (codec in hdr10Plus) add("hdr10plus")
                add("hlg")
            }
        }
        val dolbyVision = decoders
            .filter { info -> info.supportedTypes.any { it.equals(DOLBY_VISION, ignoreCase = true) } }
            .flatMap { info ->
                val type = info.supportedTypes.first { it.equals(DOLBY_VISION, ignoreCase = true) }
                info.getCapabilitiesForType(type).profileLevels.mapNotNull { dolbyVisionProfileOf(it.profile) }
            }
            .groupBy({ it.first.codec }, { it.second })
            .mapValues { (_, profiles) -> profiles.distinct().sorted() }

        val ffmpeg = FfmpegLibrary.isAvailable()
        val audio = audioCodecs.mapNotNull { (codec, mime) ->
            val platform = decoders
                .filter { info -> info.supportedTypes.any { it.equals(mime, ignoreCase = true) } }
                .maxOfOrNull { info ->
                    val type = info.supportedTypes.first { it.equals(mime, ignoreCase = true) }
                    info.getCapabilitiesForType(type).audioCapabilities?.maxInputChannelCount ?: 2
                }
            val extension = if (ffmpeg && FfmpegLibrary.supportsFormat(mime)) 8 else null
            val channels = listOfNotNull(platform, extension).maxOrNull() ?: return@mapNotNull null
            AudioAnswer(codec, minOf(channels, 8))
        }

        Measured(
            containers = listOf("mp4", "webm", "mkv", "mp3", "aac", "flac", "ogg"),
            hls = true,
            trackSelection = true,
            video = video.distinct(),
            levels = levels,
            hdr = hdr,
            dolbyVision = dolbyVision,
            audio = audio,
            subtitles = subtitles,
        )
    }

    /**
     * The HDMI sink's passthrough formats, each at the channels the sink
     * says it takes of it, and its PCM channels.
     */
    override fun audioSink(): AudioSink {
        val sink = AudioCapabilities.getCapabilities(context, AudioAttributes.DEFAULT, null, emptyList())
        val passthrough = PassthroughFormat.entries
            .filter { format -> sink.supportsEncoding(encodingOf(format)) }
            .associateWith { format ->
                val probe = Format.Builder().setSampleMimeType(mimeOf(format)).setSampleRate(48_000).build()
                val channels = sink.getEncodingAndChannelConfigForPassthrough(probe, AudioAttributes.DEFAULT)?.second?.let(Integer::bitCount) ?: 0
                if (channels > 0) minOf(channels, format.channels) else format.channels
            }
        return AudioSink(passthrough, sink.maxChannelCount)
    }

    private fun isAccelerated(info: MediaCodecInfo): Boolean? =
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            info.isHardwareAccelerated
        } else {
            val name = info.name.lowercase()
            !(name.startsWith("omx.google.") || name.startsWith("c2.android.") || ".sw." in name)
        }
}

private const val DOLBY_VISION = "video/dolby-vision"

/** The encoding that carries a passthrough format to the sink. */
private fun encodingOf(format: PassthroughFormat): Int = when (format) {
    PassthroughFormat.Ac3 -> C.ENCODING_AC3
    PassthroughFormat.Eac3 -> C.ENCODING_E_AC3
    PassthroughFormat.Dts -> C.ENCODING_DTS
    PassthroughFormat.DtsHd -> C.ENCODING_DTS_HD
    PassthroughFormat.TrueHd -> C.ENCODING_DOLBY_TRUEHD
}

/** A passthrough format by the MIME type Media3 reads its sink's channels for. */
private fun mimeOf(format: PassthroughFormat): String = when (format) {
    PassthroughFormat.Ac3 -> MimeTypes.AUDIO_AC3
    PassthroughFormat.Eac3 -> MimeTypes.AUDIO_E_AC3
    PassthroughFormat.Dts -> MimeTypes.AUDIO_DTS
    PassthroughFormat.DtsHd -> MimeTypes.AUDIO_DTS_HD
    PassthroughFormat.TrueHd -> MimeTypes.AUDIO_TRUEHD
}

/** The containers Matroska files are named by. */
private val MATROSKA = listOf("mkv", "webm")

/**
 * The subtitles Media3 shows: every text and picture format it reads from a
 * file (styled ASS through libass), WebVTT renditions of a stream, and the
 * files the app draws, WebVTT, ASS and PGS (through [SupParser]). Each
 * embedded format names the containers Media3's extractors read it from:
 * Matroska for all of them, and MP4 for WebVTT and mov_text. Its MPEG-TS
 * extractor reads only DVB subtitles.
 */
private val subtitles = listOf(
    SubtitleEntry("webvtt", listOf("file", "stream", "external"), containers = listOf("mkv", "webm", "mp4")),
    SubtitleEntry("subrip", listOf("file"), containers = MATROSKA),
    SubtitleEntry("mov_text", listOf("file"), containers = listOf("mp4")),
    SubtitleEntry("ass", listOf("file", "external"), containers = MATROSKA),
    SubtitleEntry("hdmv_pgs_subtitle", listOf("file", "external"), containers = MATROSKA),
    SubtitleEntry("dvd_subtitle", listOf("file"), containers = MATROSKA),
)
