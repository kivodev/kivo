package com.mmcdole.kivo.core.player

import com.mmcdole.kivo.api.models.Chapter
import com.mmcdole.kivo.api.models.Marker as ServerMarker
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class MarkersTest {
    private val chapters = listOf(
        Chapter(0, 60_000, "Cold open"),
        Chapter(60_000, 150_000, "Opening credits"),
        Chapter(150_000, 2_550_000, "Episode"),
        Chapter(2_550_000, 2_640_000, "End credits"),
    )
    private val markers = listOf(
        SkipMarker(MarkerKind.Intro, 60_000, 150_000),
        SkipMarker(MarkerKind.Credits, 2_550_000, 2_640_000),
    )

    @Test
    fun findsTheChapterAndTheMarkerAtAPosition() {
        assertEquals("Cold open", Markers.chapterAt(chapters, 30_000)?.title)
        assertNull(markerAt(markers, 30_000))
        assertEquals(MarkerKind.Intro, markerAt(markers, 60_000)?.kind)
        assertEquals(MarkerKind.Credits, markerAt(markers, 2_600_000)?.kind)
        assertNull(Markers.chapterAt(emptyList(), 5))
    }

    @Test
    fun aPlaysMarkersAreTheServers() {
        val server = listOf(
            ServerMarker("recap", 0, 40_000, "chapter"),
            ServerMarker("intro", 40_000, 42_000, "chapter"),
            ServerMarker("sponsor", 50_000, 80_000, "chapter"),
            ServerMarker("credits", 2_550_000, 2_640_000, "chapter", final = true),
        )
        assertEquals(
            listOf(SkipMarker(MarkerKind.Recap, 0, 40_000), SkipMarker(MarkerKind.Credits, 2_550_000, 2_640_000, final = true)),
            markersOf(server),
            "a kind it doesn't know, and one under 3 seconds, are left out",
        )
        assertEquals(emptyList(), markersOf(emptyList()), "none, from a server that says so")
    }

    @Test
    fun upNextComesAtCreditsOnlyWhenTheyRunToTheEnd() {
        assertNull(upNextCredits(markers))
        val finalCredits = SkipMarker(MarkerKind.Credits, 2_550_000, 2_640_000, final = true)
        assertEquals(finalCredits, upNextCredits(listOf(markers[0], finalCredits)))
    }

    /** Skip at [positionMs], with the controls hidden unless [controls]. */
    private fun SkipShow.shown(positionMs: Long, controls: Boolean = false, upNext: Boolean = false) =
        at(markers, positionMs, controlsShowing = controls, upNextShowing = upNext)?.kind

    @Test
    fun skipShowsFromEnteringAMarkerUntilItsTimeIsUp() {
        val skip = SkipShow()
        assertNull(skip.shown(30_000))
        assertFalse(skip.entered)
        assertEquals(MarkerKind.Intro, skip.shown(60_000))
        assertTrue(skip.entered, "entering starts its time")
        assertEquals(MarkerKind.Intro, skip.shown(69_000))
        assertFalse(skip.entered)
        skip.expire()
        assertNull(skip.shown(70_000))
    }

    @Test
    fun skipComesBackWithTheControlsInsideTheMarker() {
        val skip = SkipShow()
        skip.shown(60_000)
        skip.expire()
        assertNull(skip.shown(80_000))
        assertEquals(MarkerKind.Intro, skip.shown(85_000, controls = true))
        assertNull(skip.shown(90_000))
        // Gone at the marker's end, controls or not.
        assertNull(skip.shown(150_000, controls = true))
    }

    @Test
    fun skipHidesForTheMarkerOnceSkippedAndShowsAgainOnASeekBackIntoIt() {
        val skip = SkipShow()
        assertEquals(MarkerKind.Intro, skip.shown(60_000))
        skip.skip()
        // Until the seek lands, the position is still inside the marker.
        assertNull(skip.shown(61_000, controls = true))
        assertNull(skip.shown(150_000, controls = true))
        assertEquals(MarkerKind.Intro, skip.shown(140_000))
        assertTrue(skip.entered, "the seek back starts its time again")
    }

    @Test
    fun skipCreditsGivesWayToUpNext() {
        val skip = SkipShow()
        assertNull(skip.shown(2_551_000, controls = true, upNext = true))
        assertEquals(MarkerKind.Credits, skip.shown(2_552_000))
    }

    @Test
    fun aTimelineMapsPositionsToPartsAndBack() {
        val timeline = PartTimeline(listOf(60_000, 30_000, 45_000))
        assertEquals(135_000, timeline.durationMs)
        assertEquals(PartPosition(0, 0), timeline.locate(0))
        assertEquals(PartPosition(0, 59_999), timeline.locate(59_999))
        assertEquals(PartPosition(1, 0), timeline.locate(60_000))
        assertEquals(PartPosition(2, 10_000), timeline.locate(100_000))
        // Past the end, the last part's end; before the start, its start.
        assertEquals(PartPosition(2, 45_000), timeline.locate(200_000))
        assertEquals(PartPosition(0, 0), timeline.locate(-5))
        assertEquals(100_000, timeline.position(2, 10_000))
    }
}
