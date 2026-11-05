package com.mmcdole.kivo.core.books

import com.mmcdole.kivo.api.models.Chapter
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class BookChaptersTest {
    private val chapters = BookChapters(
        listOf(Chapter(0, 60_000, "Opening"), Chapter(60_000, 600_000, "One"), Chapter(600_000, 1_200_000, "Two")),
        listOf(1_200_000),
    )

    @Test
    fun previousGoesToTheChapterBeforeWithinThreeSecondsOfAStartElseToItsStart() {
        assertEquals(60_000, chapters.previousMs(65_000), "past 3 s: the chapter's own start")
        assertEquals(0, chapters.previousMs(62_000), "within 3 s: the chapter before")
        assertEquals(0, chapters.previousMs(63_000), "3 s exactly still counts as its start")
        assertEquals(60_000, chapters.previousMs(63_001))
        assertEquals(0, chapters.previousMs(1_000), "the first chapter has none before it")
        assertEquals(600_000, chapters.previousMs(1_199_000))
    }

    @Test
    fun nextGoesToTheNextChaptersStartAndNowhereInTheLast() {
        assertEquals(60_000, chapters.nextMs(0))
        assertEquals(600_000, chapters.nextMs(599_999))
        assertEquals(600_000, chapters.nextMs(60_000), "at a start, the one after it")
        assertNull(chapters.nextMs(600_000))
    }

    @Test
    fun progressIsWithinTheChapterAndAcrossTheBook() {
        val p = chapters.progress(90_000)
        assertEquals(1, p.chapterIndex)
        assertEquals(3, p.chapterCount)
        assertEquals("One", p.chapterTitle)
        assertEquals(30_000, p.chapterElapsedMs)
        assertEquals(510_000, p.chapterRemainingMs)
        assertEquals(1_110_000, p.remainingMs)
        assertEquals(0.075, p.fraction)
        assertTrue(p.hasNext)
        assertFalse(chapters.progress(700_000).hasNext)
        assertEquals(0, chapters.progress(5_000_000).remainingMs, "past the end, the end")
    }

    @Test
    fun aBookWithoutChaptersIsOneChapter() {
        val one = BookChapters(emptyList(), listOf(3_600_000))
        assertEquals(listOf(Chapter(0, 3_600_000)), one.list)
        assertEquals(0, one.previousMs(2_000_000))
        assertNull(one.nextMs(0))
        assertEquals(3_600_000, one.progress(0).chapterDurationMs)
    }

    @Test
    fun aMultiPartBookWithoutChaptersIsAChapterPerPart() {
        val parts = BookChapters(emptyList(), listOf(1_000_000, 2_000_000, 500_000))
        assertEquals(listOf(Chapter(0, 1_000_000), Chapter(1_000_000, 3_000_000), Chapter(3_000_000, 3_500_000)), parts.list)
        assertEquals(3_500_000, parts.durationMs)
        assertEquals(3_000_000, parts.nextMs(1_500_000), "into the third part")
        assertEquals(0, parts.previousMs(1_002_000), "back across a part's start within 3 s")
        assertEquals(2, parts.progress(3_200_000).chapterIndex)
        assertEquals(300_000, parts.progress(3_200_000).chapterRemainingMs)
    }

    @Test
    fun aMultiPartBooksChaptersAreTheServersAcrossItsParts() {
        // Two files, the second chapter running from the first into the second.
        val book = BookChapters(
            listOf(Chapter(0, 400_000, "1"), Chapter(400_000, 1_300_000, "2"), Chapter(1_300_000, 2_000_000, "3")),
            listOf(1_000_000, 1_000_000),
        )
        assertEquals(2_000_000, book.durationMs)
        assertEquals(1, book.indexAt(1_100_000), "in the second part, still the second chapter")
        assertEquals(400_000, book.previousMs(1_100_000))
        assertEquals(1_300_000, book.nextMs(500_000))
        assertEquals(900_000, book.progress(1_100_000).chapterDurationMs)
        assertEquals(2_000_000, book.endMs(2))
    }
}
