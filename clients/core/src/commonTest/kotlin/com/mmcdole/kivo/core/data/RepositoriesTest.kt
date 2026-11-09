package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.BookSeries
import com.mmcdole.kivo.api.models.BookSeriesSummaryPage
import com.mmcdole.kivo.api.models.Collection
import com.mmcdole.kivo.api.models.CollectionSummaryPage
import com.mmcdole.kivo.api.models.Home
import com.mmcdole.kivo.api.models.HomeRow
import com.mmcdole.kivo.api.models.LibraryFacets
import com.mmcdole.kivo.api.models.Person
import com.mmcdole.kivo.api.models.PersonSummaryPage
import com.mmcdole.kivo.api.models.WorkList
import com.mmcdole.kivo.api.models.WorkListList
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkSummaryList
import com.mmcdole.kivo.api.models.WorkSummaryPage
import com.mmcdole.kivo.core.FakeServer
import com.mmcdole.kivo.core.json
import com.mmcdole.kivo.core.live.Invalidation
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals

class RepositoriesTest {
    @Test
    fun homeIsRefetchedWhenTheLiveEventsSaySo() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        var fetches = 0
        server.on("GET", "/api/v1/home") {
            fetches++
            json(Home.serializer(), Home(listOf(HomeRow("recentlyAdded:l1", "recentlyAdded", emptyList()))))
        }
        val invalidations = MutableSharedFlow<Invalidation>()
        val homes = mutableListOf<Home>()
        backgroundScope.launch { HomeRepository(server.connection(), invalidations).home().collect { homes += it } }
        runCurrent()
        assertEquals(1, homes.size)

        invalidations.emit(Invalidation.LibraryChanged(listOf("l1")))
        runCurrent()
        invalidations.emit(Invalidation.PlayEnded("p1"))
        runCurrent()
        invalidations.emit(Invalidation.Everything)
        runCurrent()

        assertEquals(3, homes.size)
        assertEquals(3, fetches)
    }

    @Test
    fun aCollectionAndAWorksChildrenAreTheirOwnReads() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        server.on("GET", "/api/v1/collections/c1") { json(Collection.serializer(), Collection("c1", "Dune Collection", WorkImages(), emptyList(), "provider")) }
        server.on("GET", "/api/v1/works/s1/children") { json(WorkSummaryList.serializer(), WorkSummaryList(emptyList())) }
        val invalidations = MutableSharedFlow<Invalidation>()
        val works = WorkRepository(server.connection(), invalidations)
        val collections = mutableListOf<Collection>()
        backgroundScope.launch { works.collection("c1").collect { collections += it } }
        backgroundScope.launch { works.children("s1").collect { } }
        runCurrent()
        assertEquals("Dune Collection", collections.single().name)
        assertEquals(listOf("/api/v1/collections/c1", "/api/v1/works/s1/children"), server.seen.map { it.path }.sorted())

        invalidations.emit(Invalidation.UserStateChanged(listOf("w1")))
        runCurrent()
        assertEquals(2, collections.size, "a played mark refetches the page")
    }

    @Test
    fun anAuthorsAndABookSeriesPageAreTheirOwnReadsNotWorks() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        server.on("GET", "/api/v1/people/p1") { json(Person.serializer(), Person("p1", "Ann Leckie", emptyList(), emptyList(), emptyList(), emptyList())) }
        server.on("GET", "/api/v1/book-series/s1") { json(BookSeries.serializer(), BookSeries("s1", "Imperial Radch", emptyList())) }
        val invalidations = MutableSharedFlow<Invalidation>()
        val works = WorkRepository(server.connection(), invalidations)
        val people = mutableListOf<Person>()
        val series = mutableListOf<BookSeries>()
        backgroundScope.launch { works.person("p1").collect { people += it } }
        backgroundScope.launch { works.bookSeries("s1").collect { series += it } }
        runCurrent()
        assertEquals("Ann Leckie", people.single().name)
        assertEquals("Imperial Radch", series.single().name)

        invalidations.emit(Invalidation.PlayEnded("x"))
        runCurrent()
        invalidations.emit(Invalidation.UserStateChanged(listOf("b1")))
        runCurrent()
        assertEquals(2, people.size, "a book's played mark refetches the page")
        assertEquals(2, series.size)
    }

    @Test
    fun listsAreReadAndRefetchedAsTheyChange() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        server.on("GET", "/api/v1/lists") { json(WorkListList.serializer(), WorkListList(emptyList())) }
        server.on("GET", "/api/v1/lists/l1/works") { json(WorkSummaryPage.serializer(), WorkSummaryPage(emptyList(), total = 0)) }
        val invalidations = MutableSharedFlow<Invalidation>()
        val lists = ListsRepository(server.connection(), invalidations)
        val read = mutableListOf<List<WorkList>>()
        backgroundScope.launch { lists.lists().collect { read += it } }
        runCurrent()
        invalidations.emit(Invalidation.ListsChanged)
        runCurrent()
        assertEquals(2, read.size)
        invalidations.emit(Invalidation.HomeChanged)
        runCurrent()
        assertEquals(2, read.size)

        lists.page("l1", offset = 20)
        lists.page("l1", offset = 0, limit = 60)
        assertEquals(
            listOf(setOf("offset=20"), setOf("limit=60")),
            server.to("GET", "/api/v1/lists/l1/works").map { it.query.split('&').toSet() },
        )
    }

    @Test
    fun aGridPagesByOffsetAndHearsOnlyItsLibrary() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        server.on("GET", "/api/v1/libraries/l1/works") { json(WorkSummaryPage.serializer(), WorkSummaryPage(emptyList(), total = 0)) }
        server.on("GET", "/api/v1/libraries/l1/collections") { json(CollectionSummaryPage.serializer(), CollectionSummaryPage(emptyList(), 0)) }
        server.on("GET", "/api/v1/libraries/l1/authors") { json(PersonSummaryPage.serializer(), PersonSummaryPage(emptyList(), 0)) }
        server.on("GET", "/api/v1/libraries/l1/book-series") { json(BookSeriesSummaryPage.serializer(), BookSeriesSummaryPage(emptyList(), 0)) }
        val invalidations = MutableSharedFlow<Invalidation>()
        val grid = GridRepository(server.connection(), invalidations)

        grid.page(GridQuery("l1"), offset = 0, limit = 60)
        grid.page(GridQuery("l1", genres = listOf("Drama")), offset = 120, limit = 60)
        grid.collections("l1", offset = 60, limit = 60)
        grid.authors("l1", offset = 0, limit = 60)
        grid.bookSeries("l1", offset = 0, limit = 60)
        assertEquals(
            listOf(
                "/api/v1/libraries/l1/works" to setOf("limit=60"),
                "/api/v1/libraries/l1/works" to setOf("genre=Drama", "offset=120", "limit=60"),
                "/api/v1/libraries/l1/collections" to setOf("offset=60", "limit=60"),
                "/api/v1/libraries/l1/authors" to setOf("limit=60"),
                "/api/v1/libraries/l1/book-series" to setOf("limit=60"),
            ),
            server.seen.map { it.path to it.query.split('&').toSet() },
            "the first page, which carries the total and letters, has no offset",
        )

        var changes = 0
        backgroundScope.launch { grid.changes("l1").collect { changes++ } }
        runCurrent()
        invalidations.emit(Invalidation.LibraryChanged(listOf("l2")))
        invalidations.emit(Invalidation.LibraryChanged(listOf("l1", "l2")))
        invalidations.emit(Invalidation.UserStateChanged(listOf("w1")))
        runCurrent()
        assertEquals(2, changes)
    }

    @Test
    fun aLibrarysFacetsAreReadAgainAsItChangesAlone() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        server.on("GET", "/api/v1/libraries/l1/facets") {
            json(LibraryFacets.serializer(), LibraryFacets(emptyList(), emptyList(), emptyList(), emptyList(), listOf("4k"), listOf("hdr"), collections = true))
        }
        val invalidations = MutableSharedFlow<Invalidation>()
        val grid = GridRepository(server.connection(), invalidations)

        val read = mutableListOf<LibraryFacets>()
        backgroundScope.launch { grid.facets("l1").collect { read += it } }
        runCurrent()
        assertEquals(listOf("4k"), read.single().resolutions)
        invalidations.emit(Invalidation.LibraryChanged(listOf("l2")))
        invalidations.emit(Invalidation.UserStateChanged(listOf("w1")))
        runCurrent()
        assertEquals(1, read.size, "another library's change and a played mark leave them")
        invalidations.emit(Invalidation.LibraryChanged(listOf("l1")))
        runCurrent()
        assertEquals(2, read.size)
    }
}
