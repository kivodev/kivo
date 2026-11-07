package com.mmcdole.kivo.android.ui

import android.view.KeyEvent
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class EpisodeStopsTest {
    @Test
    fun downMovesFromTheThumbnailToTheText() {
        assertEquals(true, stopAfterArrow(KeyEvent.KEYCODE_DPAD_DOWN, onText = false))
    }

    @Test
    fun upMovesFromTheTextToTheThumbnail() {
        assertEquals(false, stopAfterArrow(KeyEvent.KEYCODE_DPAD_UP, onText = true))
    }

    @Test
    fun theOtherMovesLeaveTheCardOrMoveAlongTheRow() {
        assertNull(stopAfterArrow(KeyEvent.KEYCODE_DPAD_UP, onText = false), "Up from a thumbnail leaves the row")
        assertNull(stopAfterArrow(KeyEvent.KEYCODE_DPAD_DOWN, onText = true), "Down from text leaves the row")
        for (onText in listOf(false, true)) {
            assertNull(stopAfterArrow(KeyEvent.KEYCODE_DPAD_LEFT, onText), "Left keeps the stop")
            assertNull(stopAfterArrow(KeyEvent.KEYCODE_DPAD_RIGHT, onText), "Right keeps the stop")
        }
    }

    @Test
    fun okPlaysFromTheThumbnailAndOpensFromTheText() {
        assertFalse(okOpensPage(onText = false, available = true))
        assertTrue(okOpensPage(onText = true, available = true))
    }

    @Test
    fun okOpensAnUnavailableEpisodesPageFromEitherStop() {
        assertTrue(okOpensPage(onText = false, available = false))
        assertTrue(okOpensPage(onText = true, available = false))
    }

    @Test
    fun aRowOpensAsFewCardsAlongAsItTakesToShowItsEntry() {
        val ids = (1..10).map { "e$it" }
        assertEquals(0, openingIndex(ids, null))
        assertEquals(0, openingIndex(ids, "gone"))
        assertEquals(0, openingIndex(ids, "e1"))
        assertEquals(0, openingIndex(ids, "e4"), "the fourth card fits as the row stands")
        assertEquals(1, openingIndex(ids, "e5"), "as on the Samsung TV app, the fifth card shows the second first")
        assertEquals(6, openingIndex(ids, "e10"))
    }

    @Test
    fun focusEntersARowOnTheCardLastFocusedThenItsEntryThenTheFirstShown() {
        val shown = listOf("e2", "e3", "e4", "e5")
        assertEquals("e3", rowEntry(shown, last = "e3", entry = "e5"))
        assertEquals("e5", rowEntry(shown, last = null, entry = "e5"))
        assertEquals("e5", rowEntry(shown, last = "e9", entry = "e5"), "a card scrolled away can't take focus")
        assertEquals("e2", rowEntry(shown, last = "e9", entry = "e1"))
        assertEquals("e2", rowEntry(shown, last = null, entry = null))
        assertNull(rowEntry(emptyList(), last = "e3", entry = "e5"))
    }

    @Test
    fun aSynopsisIsOneLineWithoutMarkup() {
        assertEquals("One two three", synopsisLine("One <b>two</b>\n\n  three"))
        assertNull(synopsisLine(null))
        assertNull(synopsisLine("  <br/> \n"))
    }

    @Test
    fun anOverviewsParagraphsRunTogether() {
        assertEquals("First. Second.", oneLine(" First.\n\nSecond.\n"))
    }
}
