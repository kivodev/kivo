package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.BookInfo
import com.mmcdole.kivo.api.models.BookSeries
import com.mmcdole.kivo.api.models.BookSeriesRef
import com.mmcdole.kivo.api.models.BookSeriesSummary
import com.mmcdole.kivo.api.models.CollectionSummary
import com.mmcdole.kivo.api.models.Credit
import com.mmcdole.kivo.api.models.ImageRef
import com.mmcdole.kivo.api.models.Person
import com.mmcdole.kivo.api.models.PersonRef
import com.mmcdole.kivo.api.models.PersonSummary
import com.mmcdole.kivo.api.models.SearchResults
import com.mmcdole.kivo.api.models.UserState
import com.mmcdole.kivo.api.models.Version
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkSummary
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class KnownTest {
    private fun summary(id: String, title: String = id, kind: String = "movie", book: BookInfo? = null, poster: ImageRef? = null) =
        WorkSummary(id, kind, title, "matched", true, WorkImages(poster = poster), book = book)

    private fun work(id: String, title: String, credits: List<Credit>, extras: List<WorkSummary>) = WorkDetail(
        id, "movie", title, "matched", true, WorkImages(), emptyList(), emptyList(), credits, emptyList(), emptyList(), extras, emptyList(),
        libraryId = "l1",
    )

    @Test
    fun theCardsOfTheListsReadLatelyAreKnownTheOldestGoing() {
        val known = Known()
        known.read((0 until 1_000).map { summary("w$it") })
        known.read(listOf(summary("w0", "Newer"), summary("w1000")))
        assertEquals("Newer", known.summary("w0")?.title, "read again, it's the newest")
        assertNull(known.summary("w1"), "the oldest went")
        assertEquals("w999", known.summary("w999")?.id)
    }

    @Test
    fun aPageIsKnownAsItsCardWithWhatItLists() {
        val known = Known()
        val photo = ImageRef("i1")
        known.read(work("w1", "Heat", listOf(Credit("p1", "Al Pacino", "actor", photo = photo)), listOf(summary("x1", kind = "extra"))))
        assertEquals("Heat", known.work("w1")?.title)
        assertEquals("l1", known.summary("w1")?.libraryId)
        assertEquals("extra", known.summary("x1")?.kind)
        assertEquals(Heading("p1", "Al Pacino", photo), known.personHeading("p1"))
    }

    @Test
    fun peopleAndBookSeriesAreNamedByTheirCardsAndBooksKeepingAPhotoSeen() {
        val known = Known()
        val photo = ImageRef("i1")
        val cover = ImageRef("i2")
        known.read(SearchResults(emptyList(), listOf(PersonSummary("p1", "Ann Leckie", "author", 3, photo)), emptyList()))
        val book = BookInfo(
            listOf(PersonRef("p1", "Ann Leckie")),
            listOf(PersonRef("p2", "Adjoa Andoh")),
            listOf(BookSeriesRef("s1", "Imperial Radch", primary = true)),
        )
        known.read(listOf(summary("b1", kind = "book", book = book)))
        known.readBookSeries(listOf(BookSeriesSummary("s2", "Murderbot", 4, cover)))
        assertEquals(Heading("p1", "Ann Leckie", photo), known.personHeading("p1"), "a book's author keeps the photo search showed")
        assertEquals(Heading("p2", "Adjoa Andoh"), known.personHeading("p2"))
        assertEquals(Heading("s1", "Imperial Radch"), known.bookSeriesHeading("s1"))
        assertEquals(Heading("s2", "Murderbot", cover), known.bookSeriesHeading("s2"))
    }

    @Test
    fun aPersonsAndABookSeriesPageAreKnownAsRead() {
        val known = Known()
        val cover = ImageRef("i2")
        val series = BookSeries("s1", "Imperial Radch", listOf(summary("b1", kind = "book", poster = cover)))
        known.read(Person("p1", "Ann Leckie", emptyList(), listOf(series), emptyList(), emptyList()))
        assertEquals("Ann Leckie", known.person("p1")?.name)
        assertEquals(Heading("s1", "Imperial Radch", cover), known.bookSeriesHeading("s1"))
        assertEquals("b1", known.summary("b1")?.id)
        known.read(series)
        assertEquals(series, known.bookSeries("s1"))
    }

    @Test
    fun aWorksCollectionsAreKnownByTheirCards() {
        val known = Known()
        val dune = CollectionSummary("c1", "Dune Collection", 2, WorkImages(poster = ImageRef("i1")), "provider")
        known.read(work("w1", "Dune", emptyList(), emptyList()).copy(collections = listOf(dune)))
        assertEquals(dune, known.collectionCard("c1"))
        assertNull(known.summary("c1"), "a collection isn't a work")
    }

    @Test
    fun searchsCollectionsAreKnownByTheirCards() {
        val known = Known()
        val dune = CollectionSummary("c1", "Dune Collection", 2, WorkImages(poster = ImageRef("i1")), "provider")
        known.read(SearchResults(emptyList(), emptyList(), listOf(dune)))
        assertEquals(dune, known.collectionCard("c1"))
    }

    @Test
    fun aPositionResumesOnlyOnItsEditionsVersions() {
        fun version(id: String, edition: String? = null) = Version(id, true, 1, 1, emptyList(), emptyList(), emptyList(), "", edition = edition)
        val work = work("w1", "Blade Runner", emptyList(), emptyList()).copy(
            versions = listOf(version("theatrical"), version("theatrical4k"), version("final", "Final Cut")),
            userState = UserState(played = false, favorite = false, playCount = 0, positionMs = 600_000, versionId = "theatrical"),
        )
        assertEquals(600_000, work.resumeAt(null), "the default is the version played")
        assertEquals(600_000, work.resumeAt("theatrical4k"))
        assertNull(work.resumeAt("final"), "another edition starts from the beginning")
        val gone = work.copy(userState = work.userState?.copy(versionId = "gone"))
        assertEquals(600_000, gone.resumeAt("final"), "a position on a version that's gone resumes anywhere")
    }

    @Test
    fun thePageSelectsTheVersionLastPlayedElseTheFirst() {
        fun version(id: String) = Version(id, true, 1, 1, emptyList(), emptyList(), emptyList(), "")
        val fresh = work("w1", "Blade Runner", emptyList(), emptyList()).copy(versions = listOf(version("first"), version("second")))
        assertEquals("first", fresh.defaultVersion()?.id, "none played")
        val played = fresh.copy(userState = UserState(played = false, favorite = false, playCount = 1, positionMs = 0, versionId = "second"))
        assertEquals("second", played.defaultVersion()?.id, "the version last played")
        val gone = fresh.copy(userState = played.userState?.copy(versionId = "gone"))
        assertEquals("first", gone.defaultVersion()?.id, "the version played is gone")
        assertNull(fresh.copy(versions = emptyList()).defaultVersion())
        assertEquals("first", played.selectedVersion("first")?.id, "a version chosen")
        assertEquals("second", played.selectedVersion(null)?.id, "none chosen")
        assertEquals("second", played.selectedVersion("gone")?.id, "a choice that's gone")
    }
}
