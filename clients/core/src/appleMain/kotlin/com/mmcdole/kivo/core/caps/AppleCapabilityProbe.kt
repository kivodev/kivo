package com.mmcdole.kivo.core.caps

import com.mmcdole.kivo.api.models.SubtitleEntry
import kotlinx.cinterop.ExperimentalForeignApi
import platform.AVFAudio.AVAudioSession
import platform.AVFAudio.maximumOutputNumberOfChannels
import platform.AVFoundation.AVURLAsset
import platform.CoreMedia.CMVideoCodecType
import platform.CoreMedia.kCMVideoCodecType_AV1
import platform.CoreMedia.kCMVideoCodecType_HEVC
import platform.CoreMedia.kCMVideoCodecType_VP9
import platform.VideoToolbox.VTIsHardwareDecodeSupported

/**
 * The Apple [CapabilityProbe]: what AVPlayer plays on this device, each
 * codec, profile and level as AVFoundation answers for its MP4 codec
 * string. HEVC, AV1 and VP9 count only where VideoToolbox decodes them in
 * hardware. AVPlayer plays HEVC only tagged `hvc1` or `dvh1`, at 60 frames
 * a second at most. HDR10 and HLG count with the codec, and Dolby Vision
 * profiles 5 and 8 where AVFoundation plays them, whatever the display:
 * AVPlayer maps HDR to an SDR display itself, as Swiftfin relies on. Audio
 * is decoded on the device: MP3, and AAC, FLAC, ALAC, AC-3 and E-AC-3 (with
 * Atmos) as AVFoundation plays them; DTS and TrueHD are left to the server.
 * AVPlayer plays no MKV, so the containers are MP4's and books'. The app
 * says which [subtitles] it shows, in which deliveries, and whether its
 * player plays any audio track of a file ([trackSelection]).
 */
@OptIn(ExperimentalForeignApi::class)
internal class AppleCapabilityProbe(
    private val subtitles: List<SubtitleEntry>,
    private val trackSelection: Boolean,
) : CapabilityProbe {
    override suspend fun measure(): Measured {
        val hardware = { type: CMVideoCodecType -> VTIsHardwareDecodeSupported(type) }
        val hevc = hardware(kCMVideoCodecType_HEVC)
        val av1 = hardware(kCMVideoCodecType_AV1)
        val vp9 = hardware(kCMVideoCodecType_VP9)
        val video = buildList {
            for ((profile, string) in H264_PROFILES) {
                if (plays(string)) add(VideoAnswer("h264", profile, 8, accelerated = true))
            }
            if (hevc && plays(HEVC_MAIN)) add(hevcAnswer("Main", 8))
            if (hevc && plays(HEVC_MAIN_10)) add(hevcAnswer("Main 10", 10))
            if (av1 && plays(AV1_MAIN_8)) add(VideoAnswer("av1", "Main", 8, accelerated = true))
            if (av1 && plays(AV1_MAIN_10)) add(VideoAnswer("av1", "Main", 10, accelerated = true))
            if (vp9 && plays(VP9_PROFILE_0)) add(VideoAnswer("vp9", "Profile 0", 8, accelerated = true))
            if (vp9 && plays(VP9_PROFILE_2)) add(VideoAnswer("vp9", "Profile 2", 10, accelerated = true))
        }
        val codecs = video.map { it.codec }.toSet()
        return Measured(
            containers = listOf("mp4", "mp3", "aac", "flac"),
            hls = true,
            trackSelection = trackSelection,
            video = video,
            levels = buildMap {
                highestLevel(H264_LEVELS)?.let { put("h264", it) }
                if ("hevc" in codecs) highestLevel(HEVC_LEVELS)?.let { put("hevc", it) }
            },
            hdr = buildMap {
                if ("hevc" in codecs) put("hevc", listOf("hdr10", "hlg"))
                if ("av1" in codecs) put("av1", listOf("hdr10", "hlg"))
                if ("vp9" in codecs) put("vp9", listOf("hdr10", "hlg"))
            },
            dolbyVision = buildMap {
                val profiles = DOLBY_VISION.filter { (_, string) -> "hevc" in codecs && plays(string) }.map { it.first }
                if (profiles.isNotEmpty()) put("hevc", profiles)
            },
            audio = listOf(AudioAnswer("mp3", 2)) + AUDIO.filter { (_, string) -> plays(string, audio = true) }.map { it.first },
            subtitles = subtitles,
        )
    }

    /** The current route's output channels: 2 on the speaker and headphones, more over HDMI or AirPlay to a receiver. */
    override fun audioSink(): AudioSink =
        AudioSink(channels = AVAudioSession.sharedInstance().maximumOutputNumberOfChannels().toInt())

    /** Whether AVFoundation plays MP4 media of the RFC 6381 [codecs] string. */
    private fun plays(codecs: String, audio: Boolean = false): Boolean =
        AVURLAsset.isPlayableExtendedMIMEType("${if (audio) "audio" else "video"}/mp4; codecs=\"$codecs\"")

    /** The highest of [levels] AVFoundation plays, each a level with its codec string, highest first. */
    private fun highestLevel(levels: List<Pair<Double, String>>): Double? = levels.firstOrNull { (_, string) -> plays(string) }?.first

    private fun hevcAnswer(profile: String, bitDepth: Int) =
        VideoAnswer("hevc", profile, bitDepth, accelerated = true, tags = listOf("hvc1", "dvh1"), maxFrameRate = 60.0)

    private companion object {
        val H264_PROFILES = listOf(
            "Constrained Baseline" to "avc1.42E01E",
            "Baseline" to "avc1.42001E",
            "Main" to "avc1.4D401E",
            "High" to "avc1.64001E",
        )
        const val HEVC_MAIN = "hvc1.1.6.L93.B0"
        const val HEVC_MAIN_10 = "hvc1.2.4.L93.B0"
        const val AV1_MAIN_8 = "av01.0.08M.08"
        const val AV1_MAIN_10 = "av01.0.08M.10"
        const val VP9_PROFILE_0 = "vp09.00.40.08"
        const val VP9_PROFILE_2 = "vp09.02.40.10"

        /** High profile from 5.2 down; the hardware decodes no higher. */
        val H264_LEVELS = listOf(5.2 to "avc1.640034", 5.1 to "avc1.640033", 5.0 to "avc1.640032", 4.2 to "avc1.64002A", 4.1 to "avc1.640029", 4.0 to "avc1.640028")

        /** Main 10, main tier, from 5.1 down; the hardware decodes no higher. */
        val HEVC_LEVELS = listOf(5.1 to "hvc1.2.4.L153.B0", 5.0 to "hvc1.2.4.L150.B0", 4.1 to "hvc1.2.4.L123.B0", 4.0 to "hvc1.2.4.L120.B0")

        /** Dolby Vision profiles 5 and 8 at level 6 (4K at 24 frames a second). */
        val DOLBY_VISION = listOf(5 to "dvh1.05.06", 8 to "dvh1.08.06")

        /**
         * Each codec AVPlayer may decode, with the most channels its decoder
         * outputs. MP3 always plays, and AVFoundation answers for no MP4
         * codec string of it, so it isn't asked.
         */
        val AUDIO = listOf(
            AudioAnswer("aac", 8) to "mp4a.40.2",
            AudioAnswer("flac", 8) to "fLaC",
            AudioAnswer("alac", 8) to "alac",
            AudioAnswer("ac3", 6) to "ac-3",
            AudioAnswer("eac3", 8) to "ec-3",
        )
    }
}
