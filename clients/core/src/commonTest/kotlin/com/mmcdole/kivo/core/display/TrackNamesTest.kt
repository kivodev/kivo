package com.mmcdole.kivo.core.display

import com.mmcdole.kivo.api.models.AudioTrackInfo
import com.mmcdole.kivo.api.models.PlayTrack
import com.mmcdole.kivo.api.models.SubtitleTrackInfo
import com.mmcdole.kivo.api.models.Track
import com.mmcdole.kivo.api.models.TrackTranscode
import com.mmcdole.kivo.api.models.Version
import com.mmcdole.kivo.api.models.VideoTrackInfo
import kotlin.test.Test
import kotlin.test.assertEquals

class TrackNamesTest {
    private val en = Display.forLocale("en")

    private fun audio(
        id: String,
        codec: String,
        language: String?,
        title: String?,
        channels: Int,
        layout: String? = null,
        commentary: Boolean = false,
        original: Boolean = false,
        default: Boolean = false,
        profile: String? = null,
    ) = Track(
        id, "audio", codec, default, language = language, title = title, commentary = commentary.takeIf { it },
        original = original.takeIf { it }, audio = AudioTrackInfo(channels, layout, profile),
    )

    private fun subtitle(
        id: String,
        codec: String,
        language: String?,
        title: String? = null,
        sdh: Boolean = false,
        forced: Boolean = false,
        source: String = "embedded",
    ) = Track(
        id, "subtitle", codec, false, language = language, title = title,
        hearingImpaired = sdh.takeIf { it }, forced = forced.takeIf { it }, subtitle = SubtitleTrackInfo(source),
    )

    // Tremors (1990) as a Blu-ray rip has it: five English audio tracks and
    // eighteen subtitle tracks.
    private val tremorsAudio = listOf(
        audio("a1", "eac3", "eng", "E-AC3 DD+5.1 1024kbs", 6, "5.1", default = true),
        audio("a2", "eac3", "eng", "E-AC3 DD+4.0 1024kbs Surround Mix", 4, "4.0"),
        audio("a3", "flac", "eng", "Flac 2.0 Original Stereo Audio 627 kb/s", 2, "2.0"),
        audio("a4", "ac3", "eng", "Commentary by director Ron Underwood, and writers/producers Brent Maddock and S.S. Wilson", 2, "2.0", commentary = true),
        audio("a5", "ac3", "eng", "Commentary by Jonathan Melville (author of Seeking Perfection: The Unofficial Guide to Tremors)", 2, "2.0", commentary = true),
    )

    private val tremorsSubtitles = listOf(
        subtitle("s01", "subrip", "eng", "Full .srt"),
        subtitle("s02", "subrip", "eng", "SDH .srt", sdh = true),
        subtitle("s03", "hdmv_pgs_subtitle", "jpn"),
        subtitle("s04", "hdmv_pgs_subtitle", "fre"),
        subtitle("s05", "hdmv_pgs_subtitle", "ita"),
        subtitle("s06", "hdmv_pgs_subtitle", "ger"),
        subtitle("s07", "hdmv_pgs_subtitle", "spa", "Castilian"),
        subtitle("s08", "hdmv_pgs_subtitle", "dut"),
        subtitle("s09", "hdmv_pgs_subtitle", "chi", "Simplified"),
        subtitle("s10", "hdmv_pgs_subtitle", "dan"),
        subtitle("s11", "hdmv_pgs_subtitle", "fin"),
        subtitle("s12", "hdmv_pgs_subtitle", "gre"),
        subtitle("s13", "hdmv_pgs_subtitle", "nor"),
        subtitle("s14", "hdmv_pgs_subtitle", "ice"),
        subtitle("s15", "hdmv_pgs_subtitle", "kor"),
        subtitle("s16", "hdmv_pgs_subtitle", "por", "Portuguese (Portugal)"),
        subtitle("s17", "hdmv_pgs_subtitle", "swe"),
        subtitle("s18", "hdmv_pgs_subtitle", "chi", "Traditional"),
    )

    @Test
    fun namesAudioByLanguageFlagsAndChannelsWithTheTitleUnderIt() {
        assertEquals(
            listOf(
                TrackName("English · 5.1", "E-AC3 DD+5.1 1024kbs"),
                TrackName("English · 4.0", "E-AC3 DD+4.0 1024kbs Surround Mix"),
                TrackName("English · 2.0 · FLAC", "Flac 2.0 Original Stereo Audio 627 kb/s"),
                TrackName("English · Commentary · 2.0", "Commentary by director Ron Underwood, and writers/producers Brent Maddock and S.S. Wilson"),
                TrackName("English · Commentary · 2.0", "Commentary by Jonathan Melville (author of Seeking Perfection: The Unofficial Guide to Tremors)"),
            ),
            en.trackNames(tremorsAudio).values.toList(),
        )
    }

    @Test
    fun namesSubtitlesByLanguageAndFlagsWithTheTitleUnderIt() {
        assertEquals(
            listOf(
                TrackName("English", "Full .srt"), TrackName("English · SDH", "SDH .srt"), TrackName("Japanese"), TrackName("French"),
                TrackName("Italian"), TrackName("German"), TrackName("Spanish", "Castilian"), TrackName("Dutch"),
                TrackName("Chinese", "Simplified"), TrackName("Danish"), TrackName("Finnish"), TrackName("Greek"), TrackName("Norwegian"),
                TrackName("Icelandic"), TrackName("Korean"), TrackName("Portuguese", "Portuguese (Portugal)"), TrackName("Swedish"),
                TrackName("Chinese", "Traditional"),
            ),
            en.trackNames(tremorsSubtitles).values.toList(),
        )
    }

    @Test
    fun alwaysNamesALosslessOrPremiumFormat() {
        val names = en.trackNames(
            listOf(
                audio("t", "truehd", "eng", null, 8, "7.1", profile = "atmos"),
                audio("m", "dts", "eng", "DTS-HD MA 5.1", 6, "5.1", profile = "dtsHdMa"),
                audio("x", "dts", "eng", null, 8, "7.1", profile = "dtsX"),
                audio("e", "eac3", "eng", null, 6, "5.1", profile = "atmos"),
                audio("d", "eac3", "eng", null, 6, "5.1"),
                audio("a", "ac3", "eng", null, 6, "5.1"),
            ),
        )
        assertEquals(
            listOf(
                "English · 7.1 · TrueHD Atmos", "English · 5.1 · DTS-HD MA", "English · 7.1 · DTS:X", "English · 5.1 · E-AC-3 Atmos",
                "English · 5.1 · E-AC-3", "English · 5.1 · AC-3",
            ),
            names.values.map { it.name },
        )
    }

    @Test
    fun marksTheOriginalBesideADub() {
        val names = en.trackNames(
            listOf(audio("j", "aac", "jpn", null, 2, "2.0", original = true), audio("e", "eac3", "eng", "English Dub", 6, "5.1")),
        )
        assertEquals(listOf(TrackName("Japanese [Original] · 2.0"), TrackName("English · 5.1", "English Dub")), names.values.toList())
        assertEquals("japonais [VO] · 2.0", Display.forLocale("fr").trackNames(listOf(audio("j", "aac", "ja", null, 2, "2.0", original = true)))["j"]?.name)
    }

    @Test
    fun namesATrackFromItsFactsAloneAndShowsItsTitleAsWritten() {
        fun shown(track: Track) = en.trackNames(listOf(track)).getValue(track.id)
        assertEquals(TrackName("English · Forced", "English [Forced]"), shown(subtitle("f", "subrip", "eng", "English [Forced]", forced = true)))
        assertEquals(
            TrackName("Spanish · SDH", "Español (Latinoamérica) (SDH)"),
            shown(subtitle("l", "subrip", "spa", "  Español (Latinoamérica) (SDH) ", sdh = true)),
        )
        assertEquals(TrackName("English · 5.1", "English DTS 5.1 @ 1509 Kbps"), shown(audio("a", "dts", "eng", "English DTS 5.1 @ 1509 Kbps", 6, "5.1")))
        assertEquals(TrackName("English", "Forced"), shown(subtitle("s", "subrip", "eng", "Forced")))
        assertEquals(TrackName("English"), shown(subtitle("w", "subrip", "eng", "   ")))
        assertEquals(TrackName("English · Mono"), shown(audio("m", "aac", "eng", null, 1, "1.0")))
        val names = en.trackNames(listOf(audio("x", "aac", "eng", null, 6), audio("y", "aac", "eng", null, 3)))
        assertEquals(listOf("English · 5.1", "English · 3 channels"), names.values.map { it.name })
    }

    @Test
    fun cutsALongTitleAtAWord() {
        val credits = "Audio commentary by " + "Director James Cameron, Producer Gale Anne Hurd, ".repeat(4)
        val note = en.trackNames(listOf(audio("c", "aac", "eng", credits, 2, "2.0", commentary = true)))["c"]?.note.orEmpty()
        assertEquals("Audio commentary by Director James Cameron, Producer Gale Anne Hurd, Director James Cameron,…", note)
    }

    @Test
    fun tellsAnEmbeddedSubtitleFromAFileBesideItThenNumbersTheRest() {
        val names = en.trackNames(listOf(subtitle("e", "subrip", "eng"), subtitle("x", "subrip", "eng", source = "external")))
        assertEquals(listOf("English", "English · External"), names.values.map { it.name })
        val french = en.trackNames(listOf(subtitle("x", "subrip", "fre"), subtitle("y", "subrip", "fre"), subtitle("z", "ass", "fre")))
        assertEquals(listOf("French · SRT · 1", "French · SRT · 2", "French · ASS"), french.values.map { it.name })
    }

    @Test
    fun aDownloadedSubtitleNamesWhereItCameFromWithItsReleaseUnder() {
        val names = en.trackNames(
            listOf(
                subtitle("e", "subrip", "eng"),
                subtitle("d", "subrip", "eng", source = "downloaded"),
                subtitle("t", "subrip", "eng", "Modern.Family.S03E16.480p.BluRay.x264-mSD", source = "downloaded"),
            ),
        )
        assertEquals(TrackName("English"), names["e"])
        assertEquals(TrackName("English · OpenSubtitles"), names["d"])
        assertEquals(TrackName("English · OpenSubtitles", "Modern.Family.S03E16.480p.BluRay.x264-mSD"), names["t"])
        assertEquals(
            "français · OpenSubtitles",
            Display.forLocale("fr").trackNames(listOf(subtitle("d", "subrip", "fre", source = "downloaded")))["d"]?.name,
        )
    }

    @Test
    fun downloadedNamesMatchTheWebs() {
        assertEquals(
            listOf(TrackName("English", "Full"), TrackName("English · OpenSubtitles", "Modern.Family.S03E16.480p.BluRay.x264-mSD")),
            en.trackNames(
                listOf(
                    subtitle("e", "subrip", "eng", "Full"),
                    subtitle("d", "subrip", "eng", "Modern.Family.S03E16.480p.BluRay.x264-mSD", source = "downloaded"),
                ),
            ).values.toList(),
        )
        assertEquals(
            listOf(TrackName("English · SDH · OpenSubtitles", "Dune.2021.SDH"), TrackName("English · Forced · OpenSubtitles", "Dune.2021")),
            en.trackNames(
                listOf(
                    subtitle("s", "subrip", "eng", "Dune.2021.SDH", sdh = true, source = "downloaded"),
                    subtitle("f", "subrip", "eng", "Dune.2021", forced = true, source = "downloaded"),
                ),
            ).values.toList(),
        )
        assertEquals(
            listOf(TrackName("English · OpenSubtitles · 1"), TrackName("English · OpenSubtitles · 2")),
            en.trackNames(listOf(subtitle("a", "subrip", "eng", source = "downloaded"), subtitle("b", "subrip", "eng", source = "downloaded"))).values.toList(),
        )
        assertEquals(
            listOf("English", "English · OpenSubtitles"),
            en.trackNames(listOf(subtitle("e", "subrip", "eng"), subtitle("d", "subrip", "eng", source = "downloaded"))).values.map { it.name },
        )
    }

    @Test
    fun twoDownloadedSubtitlesAreToldApartByTheirFacts() {
        val names = en.trackNames(
            listOf(
                subtitle("a", "subrip", "eng", source = "downloaded"),
                subtitle("b", "subrip", "eng", sdh = true, source = "downloaded"),
                subtitle("c", "subrip", "eng", source = "downloaded"),
                subtitle("d", "ass", "eng", source = "downloaded"),
            ),
        )
        assertEquals(
            listOf(
                TrackName("English · OpenSubtitles · SRT · 1"),
                TrackName("English · SDH · OpenSubtitles"),
                TrackName("English · OpenSubtitles · SRT · 2"),
                TrackName("English · OpenSubtitles · ASS"),
            ),
            names.values.toList(),
        )
    }

    @Test
    fun tellsTracksApartOnlyWhenTheirTitlesAreTheSameToo() {
        val differ = en.trackNames(listOf(subtitle("a", "subrip", "eng", "Signs"), subtitle("b", "ass", "eng", "Dialogue")))
        assertEquals(listOf(TrackName("English", "Signs"), TrackName("English", "Dialogue")), differ.values.toList())
        val same = en.trackNames(listOf(subtitle("a", "subrip", "eng", "Signs"), subtitle("b", "ass", "eng", "Signs")))
        assertEquals(listOf(TrackName("English · SRT", "Signs"), TrackName("English · ASS", "Signs")), same.values.toList())
    }

    @Test
    fun namesWithoutALanguage() {
        assertEquals("Unknown language", en.trackNames(listOf(subtitle("u", "subrip", null)))["u"]?.name)
        assertEquals("Unknown language", en.trackNames(listOf(subtitle("n", "subrip", "null")))["n"]?.name)
        assertEquals(TrackName("Unknown language", "English"), en.trackNames(listOf(subtitle("t", "subrip", null, "English")))["t"])
        assertEquals(
            "Langue inconnue · Commentaire · 2.0",
            Display.forLocale("fr").trackNames(listOf(audio("c", "aac", "und", "Commentary", 2, "2.0", commentary = true)))["c"]?.name,
        )
    }

    @Test
    fun describesTracksInFull() {
        assertEquals("English · E-AC-3 5.1 · Default · “E-AC3 DD+5.1 1024kbs”", en.trackDetail(tremorsAudio[0]))
        assertEquals("English · FLAC 2.0 · “Flac 2.0 Original Stereo Audio 627 kb/s”", en.trackDetail(tremorsAudio[2]))
        assertEquals("English · TrueHD Atmos 7.1", en.trackDetail(audio("t", "truehd", "eng", null, 8, "7.1", profile = "atmos")))
        assertEquals("English · SDH · SRT · Embedded · “SDH .srt”", en.trackDetail(tremorsSubtitles[1]))
        assertEquals("Japanese · PGS · Embedded", en.trackDetail(tremorsSubtitles[2]))
        val video = Track("v", "video", "hevc", true, video = VideoTrackInfo(3840, 1600, resolution = "4k", profile = "Main 10", hdr = "dolbyVision"))
        assertEquals("HEVC Main 10 4K Dolby Vision", en.trackDetail(video))
        assertEquals("Chinese (Traditional, Taiwan)", en.language("zh-Hant-TW"))
        assertEquals("American English", en.language("en-US"))
    }

    @Test
    fun namesOutputsAndVersions() {
        assertEquals("H.264 1080p", en.delivered(PlayTrack("v", "video", codec = "h264", transcode = TrackTranscode(1920, 800))))
        assertEquals("HEVC 4K", en.delivered(PlayTrack("v", "video", codec = "hevc", transcode = TrackTranscode(3840, 1600))))
        assertEquals("AAC 5.1", en.delivered(PlayTrack("a", "audio", codec = "aac", transcode = TrackTranscode(channels = 6))))
        assertEquals("AAC", en.delivered(PlayTrack("a", "audio", codec = "aac")), "not transcoded")
        fun version(resolution: String?, hdr: String?, video: Boolean = true) = Version(
            "v", true, 1, 1, emptyList(), if (video) listOf(Track("t", "video", "hevc", true)) else emptyList(), emptyList(),
            resolution = resolution, hdr = hdr,
        )
        assertEquals("4K HDR", en.versionName(version("4k", "hdr10")))
        assertEquals("4K Dolby Vision", en.versionName(version("4k", "dolbyVision")))
        assertEquals("1080p", en.versionName(version("1080p", null)))
        assertEquals("SD", en.versionName(version("sd", null)))
        assertEquals("Audio", en.versionName(version(null, null, video = false)))
        assertEquals("", en.versionName(version(null, null)))
    }
}
