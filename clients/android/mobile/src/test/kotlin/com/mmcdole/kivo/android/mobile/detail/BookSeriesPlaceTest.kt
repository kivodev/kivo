package com.mmcdole.kivo.android.mobile.detail

import com.mmcdole.kivo.android.mobile.work
import com.mmcdole.kivo.api.models.BookInfo
import com.mmcdole.kivo.api.models.BookSeries
import com.mmcdole.kivo.api.models.BookSeriesRef
import com.mmcdole.kivo.api.models.PersonRef
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkSummary
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class BookSeriesPlaceTest {
    private val law = BookSeriesRef("s1", "The First Law", primary = true, position = "2")
    private val world = BookSeriesRef("s2", "The World of the First Law", primary = false, position = "5")

    private fun book(id: String, author: String, vararg series: BookSeriesRef): WorkSummary =
        work(id, kind = WorkKind.Book).copy(book = BookInfo(authors = listOf(PersonRef(author, author)), narrators = emptyList(), series = series.toList()))

    @Test
    fun theMarkedSeriesIsTheBooksPrimaryEvenWhenListedSecond() {
        val info = BookInfo(authors = emptyList(), narrators = emptyList(), series = listOf(world, law))
        assertEquals("s1", info.primarySeries()?.id)
        assertEquals("5", info.positionIn("s2"))
        assertNull(BookInfo(authors = emptyList(), narrators = emptyList(), series = emptyList()).primarySeries())
    }

    @Test
    fun withoutAMarkTheFirstSeriesIsPrimary() {
        val info = BookInfo(authors = emptyList(), narrators = emptyList(), series = listOf(world.copy(primary = false), law.copy(primary = false)))
        assertEquals("s2", info.primarySeries()?.id)
    }

    @Test
    fun aSeriesByOneAuthorNamesNone() {
        val series = BookSeries("s1", "The First Law", listOf(book("b1", "abercrombie", law), book("b2", "abercrombie", law)))
        assertEquals("abercrombie", series.commonAuthorId())
        assertFalse(series.namesAuthors())
        assertTrue(series.hasPositions())
    }

    @Test
    fun aGuestAuthorIsNamedAndTheOthersKeepABlankLine() {
        val guest = book("b3", "guest", law.copy(position = null))
        val series = BookSeries("s1", "The First Law", listOf(book("b1", "abercrombie", law), book("b2", "abercrombie", law), guest))
        assertTrue(series.namesAuthors())
        assertTrue(series.byCommonAuthor(series.items[0]))
        assertFalse(series.byCommonAuthor(guest))
    }

    @Test
    fun withNoAuthorMostBooksShareEveryCardNamesItsOwn() {
        val series = BookSeries("s9", "Anthology", listOf(book("b1", "a"), book("b2", "b")))
        assertNull(series.commonAuthorId())
        assertTrue(series.namesAuthors())
        assertFalse(series.byCommonAuthor(series.items[0]))
        assertFalse(series.hasPositions())
    }
}
