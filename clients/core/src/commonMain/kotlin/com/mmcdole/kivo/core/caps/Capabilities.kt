package com.mmcdole.kivo.core.caps

import com.mmcdole.kivo.api.models.AudioEntry
import com.mmcdole.kivo.api.models.Capabilities
import com.mmcdole.kivo.api.models.DolbyVisionEntry
import com.mmcdole.kivo.api.models.FileCapabilities
import com.mmcdole.kivo.api.models.SubtitleEntry
import com.mmcdole.kivo.api.models.VideoEntry
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

// The capability report: what this device plays, measured by the platform's
// CapabilityProbe (MediaCodec and the HDMI sink's AudioCapabilities on
// Android, VideoToolbox and AVAudioSession on Apple), then corrected for
// answers platforms are known to get wrong by the pure functions here. It
// holds measured facts only; what the user chose goes in the play's
// PlaybackSettings.

/** One decodable video configuration, profile names as ffprobe reports them. */
data class VideoAnswer(
    val codec: String,
    val profile: String,
    val bitDepth: Int,
    /** Whether an accelerator (a hardware decoder) plays it; null when the platform can't say. */
    val accelerated: Boolean? = null,
    /** The largest picture the decoder takes; null when the platform can't say. */
    val maxWidth: Int? = null,
    val maxHeight: Int? = null,
    /** The codec tags the player takes in a file, such as `hvc1`; null means any. */
    val tags: List<String>? = null,
    /** The most frames a second the player shows; null when it sets no limit. */
    val maxFrameRate: Double? = null,
)

/** One audio codec the device decodes itself, with the most channels its decoder outputs. */
data class AudioAnswer(val codec: String, val channels: Int)

/** What measuring found, before corrections. */
data class Measured(
    /** The containers the player plays as files. */
    val containers: List<String>,
    /** Whether the player plays HLS streams; with it, every codec plays in a stream as in a file. */
    val hls: Boolean,
    /** Whether the player plays any audio track inside a file, selected by its stream index. */
    val trackSelection: Boolean = false,
    val video: List<VideoAnswer>,
    /** Highest level per codec, as the codec's specification writes it (4.1 is H.264 level 4.1, 5.1 HEVC level 5.1). */
    val levels: Map<String, Double> = emptyMap(),
    /** HDR formats per codec that the player shows: `hdr10`, `hdr10plus`, `hlg`. */
    val hdr: Map<String, List<String>> = emptyMap(),
    /** Dolby Vision profiles per codec that the player shows (5, 7, 8 on HEVC; 10 on AV1). */
    val dolbyVision: Map<String, List<Int>> = emptyMap(),
    /** The codecs decoded on the device. */
    val audio: List<AudioAnswer>,
    /** The subtitles the player shows, each with the deliveries it shows it in. */
    val subtitles: List<SubtitleEntry>,
)

/** The device's measurements, implemented by each platform. */
interface CapabilityProbe {
    /** Measures the decoders and the display; slow, so the report measures once. */
    suspend fun measure(): Measured

    /** The audio output as it is now; read at every start, since a receiver can be switched on or off. */
    fun audioSink(): AudioSink
}

/**
 * Corrects the measurement for answers platforms are known to get wrong, then
 * shapes it as the spec's report, without audio. HEVC decoded in software
 * can't keep up, and Android lists its software decoder beside any hardware
 * one, so HEVC stays only where an accelerator plays it, or the platform
 * can't say how it decodes. Media3 and AVPlayer decode a file and a stream
 * alike, so each entry lists both deliveries. HDR comes with a 10-bit
 * profile only, and a Dolby Vision video plays by its base layer where
 * HDR10 shows.
 */
internal fun correct(measured: Measured): Capabilities {
    val video = measured.video.filter { it.codec != "hevc" || it.accelerated != false }
    val delivery = deliveries(measured)
    val entries = video.map { it.codec }.distinct().map { codec ->
        val of = video.filter { it.codec == codec }
        val tenBit = of.any { it.bitDepth >= 10 }
        val hdr = measured.hdr[codec]?.takeIf { tenBit && it.isNotEmpty() }
        VideoEntry(
            codec = codec,
            delivery = delivery,
            profiles = of.map { it.profile }.distinct(),
            maxLevel = measured.levels[codec],
            maxBitDepth = of.maxOf { it.bitDepth },
            maxWidth = of.largest { it.maxWidth },
            maxHeight = of.largest { it.maxHeight },
            tags = of.firstNotNullOfOrNull { it.tags },
            maxFrameRate = of.firstNotNullOfOrNull { it.maxFrameRate },
            hdr = hdr,
            dolbyVision = measured.dolbyVision[codec]?.takeIf { tenBit && it.isNotEmpty() }?.map { DolbyVisionEntry(it) },
            doviFallback = true.takeIf { hdr != null && "hdr10" in hdr },
        )
    }
    return Capabilities(
        video = entries,
        audio = emptyList(),
        subtitles = measured.subtitles,
        file = FileCapabilities(measured.containers, trackSelection = measured.trackSelection).takeIf { measured.containers.isNotEmpty() },
    )
}

/** The largest of [size] among the answers; null when any can't say, as no limit is known then. */
private fun List<VideoAnswer>.largest(size: (VideoAnswer) -> Int?): Int? =
    map(size).takeIf { sizes -> sizes.none { it == null } }?.maxOfOrNull { it!! }

/** Where the player decodes what it decodes: files, and streams when it plays HLS. */
internal fun deliveries(measured: Measured): List<String> = if (measured.hls) listOf("file", "stream") else listOf("file")

/**
 * [capabilities] for a play whose video shows where the app can't draw
 * over it, as in picture-in-picture or on AirPlay: its subtitle entries
 * without `external`, and an entry with nothing left dropped.
 */
internal fun withoutExternal(capabilities: Capabilities): Capabilities = capabilities.copy(
    subtitles = capabilities.subtitles.mapNotNull { entry ->
        entry.copy(delivery = entry.delivery - "external").takeIf { it.delivery.isNotEmpty() }
    },
)

/**
 * This device's capability report: measured once and corrected, then given
 * its audio at each start by [audioEntries], since the audio settings and
 * the sink can change between plays.
 */
internal class CapabilityReport(
    private val probe: CapabilityProbe,
    private val audioSettings: suspend () -> AudioSettings,
) {
    private val lock = Mutex()
    private var measured: Measured? = null

    /** The report; with [external] false, without subtitle files ([withoutExternal]). */
    suspend fun report(external: Boolean = true): Capabilities {
        val measured = lock.withLock { measured ?: probe.measure().also { measured = it } }
        val audio: List<AudioEntry> = audioEntries(measured.audio, probe.audioSink(), audioSettings(), deliveries(measured))
        val report = correct(measured).copy(audio = audio)
        return if (external) report else withoutExternal(report)
    }
}
