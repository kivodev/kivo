// The newer constants are compile-time ints compared with what a decoder lists, never passed to the platform.
@file:SuppressLint("InlinedApi")

package com.mmcdole.kivo.android.player

import android.annotation.SuppressLint
import android.media.MediaCodecInfo.CodecProfileLevel as P

// MediaCodec's profile and level constants as the server compares them:
// profile names as ffprobe reports them, and levels as the codec's
// specification writes them (4.1, 5.1).

/** A video codec by the server's name and MediaCodec's MIME type. */
enum class VideoCodec(val codec: String, val mime: String) {
    H264("h264", "video/avc"),
    Hevc("hevc", "video/hevc"),
    Av1("av1", "video/av01"),
    Vp9("vp9", "video/x-vnd.on2.vp9"),
}

/** A profile's ffprobe name and bit depth; null for one the server never asks about. */
fun profileOf(codec: VideoCodec, profile: Int): Pair<String, Int>? = when (codec) {
    VideoCodec.H264 -> when (profile) {
        P.AVCProfileConstrainedBaseline -> "Constrained Baseline" to 8
        P.AVCProfileBaseline -> "Baseline" to 8
        P.AVCProfileMain -> "Main" to 8
        P.AVCProfileHigh, P.AVCProfileConstrainedHigh -> "High" to 8
        P.AVCProfileHigh10 -> "High 10" to 10
        else -> null
    }
    VideoCodec.Hevc -> when (profile) {
        P.HEVCProfileMain -> "Main" to 8
        P.HEVCProfileMain10, P.HEVCProfileMain10HDR10, P.HEVCProfileMain10HDR10Plus -> "Main 10" to 10
        else -> null
    }
    VideoCodec.Av1 -> when (profile) {
        P.AV1ProfileMain8 -> "Main" to 8
        P.AV1ProfileMain10, P.AV1ProfileMain10HDR10, P.AV1ProfileMain10HDR10Plus -> "Main" to 10
        else -> null
    }
    VideoCodec.Vp9 -> when (profile) {
        P.VP9Profile0 -> "Profile 0" to 8
        P.VP9Profile1 -> "Profile 1" to 8
        P.VP9Profile2, P.VP9Profile2HDR, P.VP9Profile2HDR10Plus -> "Profile 2" to 10
        P.VP9Profile3, P.VP9Profile3HDR, P.VP9Profile3HDR10Plus -> "Profile 3" to 10
        else -> null
    }
}

/** A level as the specification writes it; null where the server has no levels (AV1, VP9) or for one it doesn't know. */
fun levelOf(codec: VideoCodec, level: Int): Double? = when (codec) {
    VideoCodec.H264 -> avcLevels[level]
    VideoCodec.Hevc -> hevcLevels[level]
    else -> null
}

private val avcLevels = mapOf(
    P.AVCLevel1 to 1.0, P.AVCLevel1b to 1.0, P.AVCLevel11 to 1.1, P.AVCLevel12 to 1.2, P.AVCLevel13 to 1.3,
    P.AVCLevel2 to 2.0, P.AVCLevel21 to 2.1, P.AVCLevel22 to 2.2,
    P.AVCLevel3 to 3.0, P.AVCLevel31 to 3.1, P.AVCLevel32 to 3.2,
    P.AVCLevel4 to 4.0, P.AVCLevel41 to 4.1, P.AVCLevel42 to 4.2,
    P.AVCLevel5 to 5.0, P.AVCLevel51 to 5.1, P.AVCLevel52 to 5.2,
    P.AVCLevel6 to 6.0, P.AVCLevel61 to 6.1, P.AVCLevel62 to 6.2,
)

/** HEVC's levels, main and high tier alike. */
private val hevcLevels = mapOf(
    P.HEVCMainTierLevel1 to 1.0, P.HEVCHighTierLevel1 to 1.0,
    P.HEVCMainTierLevel2 to 2.0, P.HEVCHighTierLevel2 to 2.0,
    P.HEVCMainTierLevel21 to 2.1, P.HEVCHighTierLevel21 to 2.1,
    P.HEVCMainTierLevel3 to 3.0, P.HEVCHighTierLevel3 to 3.0,
    P.HEVCMainTierLevel31 to 3.1, P.HEVCHighTierLevel31 to 3.1,
    P.HEVCMainTierLevel4 to 4.0, P.HEVCHighTierLevel4 to 4.0,
    P.HEVCMainTierLevel41 to 4.1, P.HEVCHighTierLevel41 to 4.1,
    P.HEVCMainTierLevel5 to 5.0, P.HEVCHighTierLevel5 to 5.0,
    P.HEVCMainTierLevel51 to 5.1, P.HEVCHighTierLevel51 to 5.1,
    P.HEVCMainTierLevel52 to 5.2, P.HEVCHighTierLevel52 to 5.2,
    P.HEVCMainTierLevel6 to 6.0, P.HEVCHighTierLevel6 to 6.0,
    P.HEVCMainTierLevel61 to 6.1, P.HEVCHighTierLevel61 to 6.1,
    P.HEVCMainTierLevel62 to 6.2, P.HEVCHighTierLevel62 to 6.2,
)

/** The 10-bit HDR profiles, which a decoder lists when it outputs HDR10 or HDR10+. */
fun hdr10Profile(codec: VideoCodec, profile: Int): Boolean = when (codec) {
    VideoCodec.Hevc -> profile == P.HEVCProfileMain10HDR10 || profile == P.HEVCProfileMain10HDR10Plus
    VideoCodec.Av1 -> profile == P.AV1ProfileMain10HDR10 || profile == P.AV1ProfileMain10HDR10Plus
    VideoCodec.Vp9 -> profile == P.VP9Profile2HDR || profile == P.VP9Profile3HDR || profile == P.VP9Profile2HDR10Plus
    VideoCodec.H264 -> false
}

fun hdr10PlusProfile(codec: VideoCodec, profile: Int): Boolean = when (codec) {
    VideoCodec.Hevc -> profile == P.HEVCProfileMain10HDR10Plus
    VideoCodec.Av1 -> profile == P.AV1ProfileMain10HDR10Plus
    VideoCodec.Vp9 -> profile == P.VP9Profile2HDR10Plus || profile == P.VP9Profile3HDR10Plus
    VideoCodec.H264 -> false
}

/**
 * A Dolby Vision decoder profile as the spec numbers it, with the codec of
 * its base layer: 5, 7 and 8 on HEVC, 10 on AV1; null for one the server
 * never asks about.
 */
fun dolbyVisionProfileOf(profile: Int): Pair<VideoCodec, Int>? = when (profile) {
    P.DolbyVisionProfileDvheStn -> VideoCodec.Hevc to 5
    P.DolbyVisionProfileDvheDtb -> VideoCodec.Hevc to 7
    P.DolbyVisionProfileDvheSt -> VideoCodec.Hevc to 8
    P.DolbyVisionProfileDvav110 -> VideoCodec.Av1 to 10
    else -> null
}

/** Audio codecs by the server's names and their MIME types, for the platform's decoders and the FFmpeg extension. */
val audioCodecs = listOf(
    "aac" to "audio/mp4a-latm",
    "mp3" to "audio/mpeg",
    "opus" to "audio/opus",
    "vorbis" to "audio/vorbis",
    "flac" to "audio/flac",
    "alac" to "audio/alac",
    "ac3" to "audio/ac3",
    "eac3" to "audio/eac3",
    "dts" to "audio/vnd.dts",
    "truehd" to "audio/true-hd",
)
