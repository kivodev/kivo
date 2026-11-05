package com.mmcdole.kivo.core.display

import com.mmcdole.kivo.api.models.PlayDelivery
import com.mmcdole.kivo.api.models.PlayPart
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.PlayTrack
import com.mmcdole.kivo.api.models.QualityRung
import com.mmcdole.kivo.api.models.TrackDelivery
import com.mmcdole.kivo.api.models.TrackTranscode
import com.mmcdole.kivo.api.models.TrackType
import com.mmcdole.kivo.core.settings.Quality
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

class DisplayTest {
    private val en = Display.forLocale("en-US")

    @Test
    fun readsTheSharedTablesForEachLocale() {
        assertTrue("en" in Display.locales && "fr" in Display.locales)
        assertEquals("Direct Stream", en.method("directStream"))
        assertEquals("Flux direct", Display.forLocale("fr-CA").method("directStream"))
        assertEquals("Direct Stream", Display.forLocale("de").method("directStream"), "English for a locale without a table")
    }

    @Test
    fun fallsBackAsTheWebClientDoes() {
        assertEquals("somethingNew", en.method("somethingNew"))
        assertEquals("This version of Kivo doesn't know why.", en.reason("somethingNew"))
        assertEquals("The audio has more channels than this device plays.", en.reason("audioChannelsUnsupported"))
        assertEquals("TrueHD", en.codec("truehd"))
        assertEquals("PCM", en.codec("pcm_s16le"))
        assertEquals("WMAPRO", en.codec("wmapro"))
        assertEquals("MKV", en.container("matroska,webm"))
        assertEquals("5.1", en.channels(6))
        assertEquals("3 channels", en.channels(3))
    }

    @Test
    fun saysWhatADialogueBoostCosts() {
        assertEquals("Plays the audio as AAC while on", en.dialogueBoostCost("audioAsAac"))
        assertEquals("Converts the video and audio while on", en.dialogueBoostCost("videoTranscode"))
        assertNull(en.dialogueBoostCost("somethingNew"))
    }

    @Test
    fun namesSubtitleKindsByHowTheyreDeliveredAndSources() {
        assertEquals("Burned in", en.subtitleKind(PlayTrack("t1", TrackType.Subtitle, delivery = TrackDelivery.Burn, codec = "ass")))
        assertEquals("Styled", en.subtitleKind(PlayTrack("t1", TrackType.Subtitle, delivery = TrackDelivery.External, codec = "ass")))
        assertEquals("Styled", en.subtitleKind(PlayTrack("t1", TrackType.Subtitle, delivery = TrackDelivery.File, codec = "ass")))
        assertEquals("Image", en.subtitleKind(PlayTrack("t1", TrackType.Subtitle, delivery = TrackDelivery.External, codec = "hdmv_pgs_subtitle")))
        assertEquals("Image", en.subtitleKind(PlayTrack("t1", TrackType.Subtitle, delivery = TrackDelivery.File, codec = "dvd_subtitle")))
        assertEquals("Plain", en.subtitleKind(PlayTrack("t1", TrackType.Subtitle, delivery = TrackDelivery.Stream, codec = "webvtt")))
        assertEquals("Plain", en.subtitleKind(PlayTrack("t1", TrackType.Subtitle, delivery = TrackDelivery.File, codec = "subrip")))
        assertEquals("External", en.subtitleSource("external"))
        assertEquals("Unknown language", en.tracks.unknownLanguage)
    }

    @Test
    fun aPlaysMethodIsItsVideosElseItsAudios() {
        fun play(vararg tracks: PlayTrack) = PlayStarted(
            id = "p1", workId = "w1", versionId = "v1", startMs = 0, durationMs = 1, local = true,
            delivery = PlayDelivery.Stream, url = "plays/p1/hls/index.m3u8", tracks = tracks.toList(), parts = listOf(PlayPart(1)), chapters = emptyList(),
        )
        val transcoded = PlayTrack("a1", TrackType.Audio, selected = true, delivery = TrackDelivery.Stream, transcode = TrackTranscode(channels = 2))

        assertEquals("Direct Play", en.method(play(PlayTrack("v1", TrackType.Video, selected = true, delivery = TrackDelivery.File))))
        assertEquals("Direct Stream", en.method(play(PlayTrack("v1", TrackType.Video, selected = true, delivery = TrackDelivery.Stream), transcoded)))
        assertEquals("Transcode", en.method(play(PlayTrack("v1", TrackType.Video, selected = true, delivery = TrackDelivery.Stream, transcode = TrackTranscode(1280, 720)))))
        assertEquals("Transcode", en.method(play(transcoded)), "without video, the audio's")
        assertEquals("Direct Stream", en.method(play(PlayTrack("v2", TrackType.Video, delivery = TrackDelivery.Stream, transcode = TrackTranscode(1280, 720)))), "only a selected track counts")
    }

    /** English's formats are in the shared vectors (VectorsTest); another locale has its own words and separators. */
    @Test
    fun formatsInTheLocalesWordsAndSeparators() {
        val fr = Display.forLocale("fr")
        assertEquals("9,8 Mbit/s", fr.bitrate(9_800_000))
        assertEquals("1 234 Go", fr.size(1_234_000_000_000))
        assertEquals("2 h 46 min", fr.runtime(9_960_000))
        assertEquals("1 h 34 min restantes", fr.timeLeft(5_640_000))
        assertEquals("2 h restantes", fr.timeLeft(7_200_000))
    }

    @Test
    fun theQualityMenuIsAutoOriginalAndTheServersLadder() {
        val ladder = listOf(QualityRung(20_000_000, 1080), QualityRung(4_000_000, 720))

        val menu = qualityMenu(ladder)

        assertEquals(
            listOf(Quality.Auto, Quality.Original, Quality.Limit(20_000_000), Quality.Limit(4_000_000)),
            menu.map { it.quality },
        )
        assertEquals(listOf(null, null) + ladder, menu.map { it.rung })
    }
}
