package com.mmcdole.kivo.core.caps

import com.mmcdole.kivo.api.models.AudioEntry
import com.mmcdole.kivo.api.models.Capabilities
import com.mmcdole.kivo.api.models.DolbyVisionEntry
import com.mmcdole.kivo.api.models.FileCapabilities
import com.mmcdole.kivo.api.models.SubtitleEntry
import com.mmcdole.kivo.api.models.VideoEntry
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

// The capability corrections, with the native audio rules.

private val both = listOf("file", "stream")

private val subtitles = listOf(
    SubtitleEntry("webvtt", listOf("file", "stream", "external")),
    SubtitleEntry("ass", listOf("file", "external")),
    SubtitleEntry("dvd_subtitle", listOf("file")),
)

private fun measured(accelerated: Boolean?) = Measured(
    containers = listOf("mkv", "mp4"),
    hls = true,
    trackSelection = true,
    video = listOf(
        VideoAnswer("h264", "High", 8, accelerated = true, maxWidth = 4096, maxHeight = 2304),
        VideoAnswer("hevc", "Main", 8, accelerated, maxWidth = 3840, maxHeight = 2160),
        VideoAnswer("hevc", "Main 10", 10, accelerated, maxWidth = 3840, maxHeight = 2160),
    ),
    levels = mapOf("h264" to 5.1, "hevc" to 5.1),
    hdr = mapOf("hevc" to listOf("hdr10", "hlg")),
    dolbyVision = mapOf("hevc" to listOf(8)),
    audio = listOf(AudioAnswer("aac", 6)),
    subtitles = subtitles,
)

class CorrectTest {
    @Test
    fun dropsHevcOnlySoftwareDecodes() {
        assertEquals(listOf("h264"), correct(measured(accelerated = false)).video.map { it.codec })
    }

    @Test
    fun entriesCarryTheirProfilesDecimalLevelSizeAndHdrInBothDeliveries() {
        assertEquals(
            Capabilities(
                video = listOf(
                    VideoEntry("h264", both, profiles = listOf("High"), maxLevel = 5.1, maxBitDepth = 8, maxWidth = 4096, maxHeight = 2304),
                    VideoEntry(
                        "hevc",
                        both,
                        profiles = listOf("Main", "Main 10"),
                        maxLevel = 5.1,
                        maxBitDepth = 10,
                        maxWidth = 3840,
                        maxHeight = 2160,
                        hdr = listOf("hdr10", "hlg"),
                        dolbyVision = listOf(DolbyVisionEntry(8)),
                        doviFallback = true,
                    ),
                ),
                audio = emptyList(),
                subtitles = subtitles,
                file = FileCapabilities(containers = listOf("mkv", "mp4"), trackSelection = true),
            ),
            correct(measured(accelerated = true)),
        )
    }

    @Test
    fun aPlayerWithoutHlsDecodesFilesOnly() {
        assertEquals(listOf(listOf("file")), correct(measured(accelerated = true).copy(hls = false)).video.map { it.delivery }.distinct())
    }

    @Test
    fun trustsHevcWhenThePlatformCantSayHowItDecodes() {
        assertEquals(listOf("h264", "hevc"), correct(measured(accelerated = null)).video.map { it.codec })
    }

    @Test
    fun reportsHdrOnlyWithATenBitProfile() {
        val eightBit = measured(accelerated = true).let { m -> m.copy(video = m.video.filter { it.bitDepth == 8 }) }
        val hevc = correct(eightBit).video.single { it.codec == "hevc" }
        assertNull(hevc.hdr)
        assertNull(hevc.dolbyVision)
        assertNull(hevc.doviFallback)
    }

    @Test
    fun aDolbyVisionBaseLayerPlaysOnlyWhereHdr10Shows() {
        val hlgOnly = measured(accelerated = true).copy(hdr = mapOf("hevc" to listOf("hlg")))
        assertNull(correct(hlgOnly).video.single { it.codec == "hevc" }.doviFallback)
    }

    @Test
    fun aPlayersTagAndFrameRateLimitsGoOnItsEntry() {
        val m = measured(accelerated = true)
        val apple = m.copy(video = m.video.map { if (it.codec == "hevc") it.copy(tags = listOf("hvc1", "dvh1"), maxFrameRate = 60.0) else it })
        val hevc = correct(apple).video.single { it.codec == "hevc" }
        assertEquals(listOf("hvc1", "dvh1"), hevc.tags)
        assertEquals(60.0, hevc.maxFrameRate)
        assertNull(correct(m).video.single { it.codec == "hevc" }.tags)
    }

    @Test
    fun aSizeIsALimitOnlyWhenEveryDecoderSaysIt() {
        val m = measured(accelerated = true)
        val unknown = m.copy(video = m.video.map { if (it.profile == "Main") it.copy(maxWidth = null) else it })
        val hevc = correct(unknown).video.single { it.codec == "hevc" }
        assertNull(hevc.maxWidth)
        assertEquals(2160, hevc.maxHeight)
    }

    @Test
    fun aPlayerWithoutContainersPlaysNoFiles() {
        assertNull(correct(measured(accelerated = true).copy(containers = emptyList())).file)
    }

    @Test
    fun withoutExternalLeavesOutSubtitleFilesAndEntriesLeftEmpty() {
        val caps = Capabilities(
            video = emptyList(),
            audio = emptyList(),
            subtitles = subtitles + SubtitleEntry("hdmv_pgs_subtitle", listOf("external")),
        )
        assertEquals(
            listOf(
                SubtitleEntry("webvtt", listOf("file", "stream")),
                SubtitleEntry("ass", listOf("file")),
                SubtitleEntry("dvd_subtitle", listOf("file")),
            ),
            withoutExternal(caps).subtitles,
        )
    }
}

class DeviceChannelsTest {
    @Test
    fun takesThisDevicesSettingFirst() {
        assertEquals(2, deviceChannels(2, AudioSink(channels = 8)))
        assertEquals(6, deviceChannels(6, AudioSink(channels = 2)))
    }

    @Test
    fun thenTheSinksChannelsElseStereo() {
        assertEquals(8, deviceChannels(null, AudioSink(channels = 8)))
        assertEquals(2, deviceChannels(null, AudioSink(channels = 0)))
    }
}

class AudioCapabilitiesTest {
    private val decoders = listOf(
        AudioAnswer("aac", 8),
        AudioAnswer("ac3", 6),
        AudioAnswer("eac3", 8),
        AudioAnswer("mp3", 2),
        AudioAnswer("truehd", 8),
    )

    /** A receiver that takes Dolby, not DTS, with 5.1 PCM. */
    private val receiver = AudioSink(
        passthrough = mapOf(PassthroughFormat.Ac3 to 6, PassthroughFormat.Eac3 to 8, PassthroughFormat.TrueHd to 8),
        channels = 6,
    )

    private fun report(sink: AudioSink, settings: AudioSettings) =
        audioEntries(decoders, sink, settings, both).associate { it.codec to it.maxChannels }

    @Test
    fun capsEveryDecodedCodecAtTheDevicesChannelsNeverRaisingOne() {
        assertEquals(
            mapOf("aac" to 2, "ac3" to 2, "eac3" to 2, "mp3" to 2, "truehd" to 2),
            report(AudioSink(channels = 2), AudioSettings()),
        )
        assertEquals(
            mapOf("aac" to 6, "ac3" to 6, "eac3" to 6, "mp3" to 2, "truehd" to 6),
            report(AudioSink(channels = 2), AudioSettings(maxChannels = 6)),
        )
        assertEquals(
            mapOf("aac" to 8, "ac3" to 6, "eac3" to 8, "mp3" to 2, "truehd" to 8),
            report(AudioSink(channels = 2), AudioSettings(maxChannels = 8)),
        )
    }

    @Test
    fun autoPassesThroughWhatTheSinkReportsAtItsChannels() {
        assertEquals(
            mapOf("aac" to 6, "mp3" to 2, "ac3" to 6, "eac3" to 8, "truehd" to 8),
            report(receiver, AudioSettings()),
        )
    }

    @Test
    fun dtsWithoutDtsHdPassesItsCoreAndDtsHdPassesItWhole() {
        assertEquals(6, report(AudioSink(passthrough = mapOf(PassthroughFormat.Dts to 6)), AudioSettings())["dts"])
        val both = AudioSink(passthrough = mapOf(PassthroughFormat.Dts to 6, PassthroughFormat.DtsHd to 8))
        assertEquals(8, report(both, AudioSettings())["dts"])
    }

    @Test
    fun offDecodesEverythingOnTheDevice() {
        assertEquals(
            mapOf("aac" to 6, "ac3" to 6, "eac3" to 6, "mp3" to 2, "truehd" to 6),
            report(receiver, AudioSettings(passthrough = PassthroughMode.Off)),
        )
    }

    @Test
    fun aCodecNeitherPassedNorDecodedIsLeftToTheServer() {
        assertEquals(null, report(receiver, AudioSettings())["dts"])
    }
}

class CapabilityReportTest {
    @Test
    fun measuresOnceAndAppliesTheAudioRulesAtEachStart() = runTest {
        var measures = 0
        var sink = AudioSink(channels = 2)
        var settings = AudioSettings()
        val probe = object : CapabilityProbe {
            override suspend fun measure(): Measured {
                measures++
                return measured(accelerated = true)
            }

            override fun audioSink() = sink
        }
        val report = CapabilityReport(probe) { settings }

        assertEquals(listOf(AudioEntry("aac", both, 2)), report.report().audio)
        sink = AudioSink(channels = 8)
        assertEquals(listOf(AudioEntry("aac", both, 6)), report.report().audio)
        settings = AudioSettings(maxChannels = 2)
        assertEquals(listOf(AudioEntry("aac", both, 2)), report.report().audio)
        assertEquals(1, measures)
    }

    @Test
    fun withoutExternalTheReportAsksForNoSubtitleFiles() = runTest {
        val probe = object : CapabilityProbe {
            override suspend fun measure() = measured(accelerated = true)

            override fun audioSink() = AudioSink()
        }
        val report = CapabilityReport(probe) { AudioSettings() }

        assertEquals(subtitles, report.report().subtitles)
        assertEquals(listOf("file", "stream"), report.report(external = false).subtitles.first().delivery)
    }
}
