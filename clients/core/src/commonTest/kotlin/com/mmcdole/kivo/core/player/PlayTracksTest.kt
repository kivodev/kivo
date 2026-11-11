package com.mmcdole.kivo.core.player

import com.mmcdole.kivo.api.models.PlayDelivery
import com.mmcdole.kivo.api.models.PlayPart
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.PlayTrack
import com.mmcdole.kivo.api.models.TrackDelivery
import com.mmcdole.kivo.api.models.TrackReason
import com.mmcdole.kivo.api.models.TrackType
import com.mmcdole.kivo.core.kivoJson
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

private fun play(vararg tracks: PlayTrack) = PlayStarted(
    id = "p1", workId = "w1", versionId = "v1", startMs = 0, durationMs = 1, local = true,
    delivery = PlayDelivery.Stream, url = "plays/p1/hls/index.m3u8", tracks = tracks.toList(), parts = listOf(PlayPart(1)), chapters = emptyList(),
)

/** A stream play: its audio, two streamed subtitles around a styled file, a PGS in the file, one only a new play shows, and off. */
private val stream = play(
    PlayTrack("v1", TrackType.Video, selected = true, delivery = TrackDelivery.Stream),
    PlayTrack("a1", TrackType.Audio, selected = true, delivery = TrackDelivery.Stream),
    PlayTrack("a2", TrackType.Audio),
    PlayTrack("t1", TrackType.Subtitle, delivery = TrackDelivery.Stream, codec = "webvtt"),
    PlayTrack("t2", TrackType.Subtitle, delivery = TrackDelivery.External, codec = "ass", url = "plays/p1/subtitles/2.ass", fonts = listOf("fonts/f1")),
    PlayTrack("t3", TrackType.Subtitle, selected = true, delivery = TrackDelivery.Stream, codec = "webvtt"),
    PlayTrack("t4", TrackType.Subtitle, delivery = TrackDelivery.File, codec = "hdmv_pgs_subtitle", index = 5),
    PlayTrack("t5", TrackType.Subtitle, reason = TrackReason.SubtitleCodecUnsupported),
    PlayTrack("off", TrackType.Subtitle, delivery = TrackDelivery.None),
)

class PlayTracksTest {
    @Test
    fun theSelectedTrackOfEachType() {
        assertEquals("v1", PlayTracks.selected(stream, TrackType.Video))
        assertEquals("a1", PlayTracks.selected(stream, TrackType.Audio))
        assertEquals("t3", PlayTracks.selected(stream, TrackType.Subtitle))
        assertEquals(SUBTITLES_OFF, PlayTracks.selected(play(), TrackType.Subtitle), "no subtitle selected is off")
        assertNull(PlayTracks.selected(play(), TrackType.Audio))
    }

    @Test
    fun aChoiceShowsAsItsDeliverySays() {
        assertEquals(TrackShow.Stream("t1", 0), PlayTracks.show(stream, "t1"))
        assertEquals(TrackShow.Stream("t3", 1), PlayTracks.show(stream, "t3"), "its place among the streamed subtitles, not among all")
        assertEquals(TrackShow.External("t2", "plays/p1/subtitles/2.ass", "ass", listOf("fonts/f1")), PlayTracks.show(stream, "t2"))
        assertEquals(TrackShow.File(5), PlayTracks.show(stream, "t4"))
        assertEquals(TrackShow.Off, PlayTracks.show(stream, SUBTITLES_OFF))
    }

    @Test
    fun aChoiceWithoutADeliveryOrUnknownTakesANewPlay() {
        assertEquals(TrackShow.NewPlay, PlayTracks.show(stream, "t5"))
        assertEquals(TrackShow.NewPlay, PlayTracks.show(stream, "a2"))
        assertEquals(TrackShow.NewPlay, PlayTracks.show(stream, "t9"), "a track the play doesn't list")
        val later = kivoJson.decodeFromString(PlayTrack.serializer(), """{"trackId":"t1","type":"lyrics","delivery":"hologram"}""")
        assertEquals(PlayTrack("t1", TrackType.Unknown, delivery = TrackDelivery.Unknown), later, "a later server's type and delivery")
        assertEquals(TrackShow.NewPlay, PlayTracks.show(play(later), "t1"), "a delivery this version doesn't know")
        assertEquals(TrackShow.NewPlay, PlayTracks.show(play(PlayTrack("t1", TrackType.Subtitle, delivery = TrackDelivery.File)), "t1"), "a file track without its index")
    }

    @Test
    fun aBurnedSubtitleIsInThePictureAndEveryOtherChoiceTakesANewPlay() {
        val burned = play(
            PlayTrack("t1", TrackType.Subtitle, selected = true, delivery = TrackDelivery.Burn),
            PlayTrack("t2", TrackType.Subtitle),
            PlayTrack("off", TrackType.Subtitle),
        )
        assertEquals(TrackShow.Burned, PlayTracks.show(burned, "t1"))
        assertEquals(TrackShow.NewPlay, PlayTracks.show(burned, "t2"))
        assertEquals(TrackShow.NewPlay, PlayTracks.show(burned, SUBTITLES_OFF))
    }
}

class PictureInPictureTest {
    @Test
    fun enteringTakesANewPlayOnlyForASubtitleFileTheAppDraws() {
        assertTrue(PictureInPicture.enteringNeedsNewPlay(stream, "t2"))
        assertFalse(PictureInPicture.enteringNeedsNewPlay(stream, "t3"), "streamed: the player shows it there")
        assertFalse(PictureInPicture.enteringNeedsNewPlay(stream, "t4"), "read from the file by the player")
        assertFalse(PictureInPicture.enteringNeedsNewPlay(stream, SUBTITLES_OFF))
        assertFalse(PictureInPicture.enteringNeedsNewPlay(play(PlayTrack("t1", TrackType.Subtitle, selected = true, delivery = TrackDelivery.Burn)), "t1"))
    }

    @Test
    fun leavingTakesANewPlayOnlyWhenThePlayBurnedTheSubtitleIn() {
        assertTrue(PictureInPicture.leavingNeedsNewPlay(play(PlayTrack("t1", TrackType.Subtitle, selected = true, delivery = TrackDelivery.Burn)), "t1"))
        assertFalse(PictureInPicture.leavingNeedsNewPlay(stream, "t3"))
        assertFalse(PictureInPicture.leavingNeedsNewPlay(stream, SUBTITLES_OFF))
    }
}
