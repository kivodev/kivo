package com.mmcdole.kivo.core.player

import com.mmcdole.kivo.api.models.PlayPart
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.PlayTrack
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

private fun play(vararg tracks: PlayTrack) = PlayStarted(
    id = "p1", workId = "w1", versionId = "v1", startMs = 0, durationMs = 1, local = true,
    delivery = "stream", url = "plays/p1/hls/index.m3u8", tracks = tracks.toList(), parts = listOf(PlayPart(1)), chapters = emptyList(),
)

/** A stream play: its audio, two streamed subtitles around a styled file, a PGS in the file, one only a new play shows, and off. */
private val stream = play(
    PlayTrack("v1", "video", selected = true, delivery = "stream"),
    PlayTrack("a1", "audio", selected = true, delivery = "stream"),
    PlayTrack("a2", "audio"),
    PlayTrack("t1", "subtitle", delivery = "stream", codec = "webvtt"),
    PlayTrack("t2", "subtitle", delivery = "external", codec = "ass", url = "plays/p1/subtitles/2.ass", fonts = listOf("fonts/f1")),
    PlayTrack("t3", "subtitle", selected = true, delivery = "stream", codec = "webvtt"),
    PlayTrack("t4", "subtitle", delivery = "file", codec = "hdmv_pgs_subtitle", index = 5),
    PlayTrack("t5", "subtitle", reason = "subtitleCodecUnsupported"),
    PlayTrack("off", "subtitle", delivery = "none"),
)

class PlayTracksTest {
    @Test
    fun theSelectedTrackOfEachType() {
        assertEquals("v1", PlayTracks.selected(stream, "video"))
        assertEquals("a1", PlayTracks.selected(stream, "audio"))
        assertEquals("t3", PlayTracks.selected(stream, "subtitle"))
        assertEquals(Subtitles.OFF, PlayTracks.selected(play(), "subtitle"), "no subtitle selected is off")
        assertNull(PlayTracks.selected(play(), "audio"))
    }

    @Test
    fun aChoiceShowsAsItsDeliverySays() {
        assertEquals(TrackShow.Stream("t1", 0), PlayTracks.show(stream, "t1"))
        assertEquals(TrackShow.Stream("t3", 1), PlayTracks.show(stream, "t3"), "its place among the streamed subtitles, not among all")
        assertEquals(TrackShow.External("t2", "plays/p1/subtitles/2.ass", "ass", listOf("fonts/f1")), PlayTracks.show(stream, "t2"))
        assertEquals(TrackShow.File(5), PlayTracks.show(stream, "t4"))
        assertEquals(TrackShow.Off, PlayTracks.show(stream, Subtitles.OFF))
    }

    @Test
    fun aChoiceWithoutADeliveryOrUnknownTakesANewPlay() {
        assertEquals(TrackShow.NewPlay, PlayTracks.show(stream, "t5"))
        assertEquals(TrackShow.NewPlay, PlayTracks.show(stream, "a2"))
        assertEquals(TrackShow.NewPlay, PlayTracks.show(stream, "t9"), "a track the play doesn't list")
        assertEquals(TrackShow.NewPlay, PlayTracks.show(play(PlayTrack("t1", "subtitle", delivery = "hologram")), "t1"), "a delivery this version doesn't know")
        assertEquals(TrackShow.NewPlay, PlayTracks.show(play(PlayTrack("t1", "subtitle", delivery = "file")), "t1"), "a file track without its index")
    }

    @Test
    fun aBurnedSubtitleIsInThePictureAndEveryOtherChoiceTakesANewPlay() {
        val burned = play(
            PlayTrack("t1", "subtitle", selected = true, delivery = "burn"),
            PlayTrack("t2", "subtitle"),
            PlayTrack("off", "subtitle"),
        )
        assertEquals(TrackShow.Burned, PlayTracks.show(burned, "t1"))
        assertEquals(TrackShow.NewPlay, PlayTracks.show(burned, "t2"))
        assertEquals(TrackShow.NewPlay, PlayTracks.show(burned, Subtitles.OFF))
    }
}

class PictureInPictureTest {
    @Test
    fun enteringTakesANewPlayOnlyForASubtitleFileTheAppDraws() {
        assertTrue(PictureInPicture.enteringNeedsNewPlay(stream, "t2"))
        assertFalse(PictureInPicture.enteringNeedsNewPlay(stream, "t3"), "streamed: the player shows it there")
        assertFalse(PictureInPicture.enteringNeedsNewPlay(stream, "t4"), "read from the file by the player")
        assertFalse(PictureInPicture.enteringNeedsNewPlay(stream, Subtitles.OFF))
        assertFalse(PictureInPicture.enteringNeedsNewPlay(play(PlayTrack("t1", "subtitle", selected = true, delivery = "burn")), "t1"))
    }

    @Test
    fun leavingTakesANewPlayOnlyWhenThePlayBurnedTheSubtitleIn() {
        assertTrue(PictureInPicture.leavingNeedsNewPlay(play(PlayTrack("t1", "subtitle", selected = true, delivery = "burn")), "t1"))
        assertFalse(PictureInPicture.leavingNeedsNewPlay(stream, "t3"))
        assertFalse(PictureInPicture.leavingNeedsNewPlay(stream, Subtitles.OFF))
    }
}
