package com.mmcdole.kivo.core.display

import com.mmcdole.kivo.api.models.PlayTrack
import com.mmcdole.kivo.api.models.Track
import com.mmcdole.kivo.api.models.TrackTranscode
import com.mmcdole.kivo.api.models.TrackType
import kotlin.test.Test
import kotlin.test.assertEquals

class TrackNamesTest {
    private val en = Display()

    @Test
    fun aPickerShowsTheServersNameWithTheTitleUnderIt() {
        fun track(title: String?) = Track("s", TrackType.Subtitle, "subrip", false, "English · SDH", "English · SDH · SRT · Embedded", title = title)
        assertEquals(TrackName("English · SDH", "SDH .srt"), track(" SDH .srt ").pickerName)
        assertEquals(TrackName("English · SDH"), track("   ").pickerName)
        assertEquals(TrackName("English · SDH"), track(null).pickerName)
    }

    @Test
    fun namesWhatATrackIsDeliveredAs() {
        assertEquals("H.264 1080p", en.delivered(PlayTrack("v", TrackType.Video, codec = "h264", transcode = TrackTranscode(1920, 800))))
        assertEquals("HEVC 4K", en.delivered(PlayTrack("v", TrackType.Video, codec = "hevc", transcode = TrackTranscode(3840, 1600))))
        assertEquals("AAC 5.1", en.delivered(PlayTrack("a", TrackType.Audio, codec = "aac", transcode = TrackTranscode(channels = 6))))
        assertEquals("AAC", en.delivered(PlayTrack("a", TrackType.Audio, codec = "aac")), "not transcoded")
    }

    @Test
    fun namesLanguages() {
        assertEquals("Chinese (Traditional, Taiwan)", en.language("zh-Hant-TW"))
        assertEquals("American English", en.language("en-US"))
        assertEquals("Unknown language", en.language(null))
        assertEquals("Unknown language", en.language("und"))
    }
}
