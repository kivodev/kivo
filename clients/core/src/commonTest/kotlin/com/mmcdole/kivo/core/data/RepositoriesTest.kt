package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.BookSeries
import com.mmcdole.kivo.api.models.BookSeriesSummaryPage
import com.mmcdole.kivo.api.models.Collection
import com.mmcdole.kivo.api.models.CollectionSummaryPage
import com.mmcdole.kivo.api.models.Home
import com.mmcdole.kivo.api.models.HomeRow
import com.mmcdole.kivo.api.models.HomeRowChange
import com.mmcdole.kivo.api.models.HomeRowInfo
import com.mmcdole.kivo.api.models.HomeRowInfoList
import com.mmcdole.kivo.api.models.LibraryFacets
import com.mmcdole.kivo.api.models.Person
import com.mmcdole.kivo.api.models.PersonSummaryPage
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkSummaryList
import com.mmcdole.kivo.api.models.WorkSummaryPage
import com.mmcdole.kivo.core.FakeServer
import com.mmcdole.kivo.core.json
import com.mmcdole.kivo.core.live.Invalidation
import kotlinx.coroutines.CompletableDeferred
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
    fun aLayoutReadThatOverlapsASaveIsDroppedAndSavesRunInOrder() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        val row = { shown: Boolean -> HomeRowInfoList(listOf(HomeRowInfo("continueWatching", "continueWatching", shown))) }
        server.on("GET", "/api/v1/home/rows") { json(HomeRowInfoList.serializer(), row(true)) }
        val gate = CompletableDeferred<Unit>()
        var saved = 0
        val order = mutableListOf<Int>()
        server.on("PATCH", "/api/v1/home/rows") {
            val n = ++saved
            order += n
            if (n == 1) gate.await()
            json(HomeRowInfoList.serializer(), row(false))
        }
        val invalidations = MutableSharedFlow<Invalidation>()
        val home = HomeRepository(server.connection(), invalidations)
        val reads = mutableListOf<List<HomeRowInfo>>()
        backgroundScope.launch { home.rows().collect { reads += it } }
        runCurrent()
        assertEquals(1, reads.size)

        val change = listOf(HomeRowChange("continueWatching", false))
        backgroundScope.launch { home.setRows(change) }
        backgroundScope.launch { home.setRows(change) }
        runCurrent()
        invalidations.emit(Invalidation.HomeChanged)
        runCurrent()
        assertEquals(1, reads.size, "the echo of a save in flight doesn't overwrite the editor")
        assertEquals(1, saved, "the second save waits for the first")

        gate.complete(Unit)
        runCurrent()
        assertEquals(listOf(1, 2), order)
        invalidations.emit(Invalidation.HomeChanged)
        runCurrent()
        assertEquals(2, reads.size)
    }

    @Test
    fun favoritesAreReadAKindAtATimeAndRefetchedAsTheUsersStateChanges() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        server.on("GET", "/api/v1/favorites") { json(WorkSummaryPage.serializer(), WorkSummaryPage(emptyList(), total = 0)) }
        val invalidations = MutableSharedFlow<Invalidation>()
        val favorites = FavoritesRepository(server.connection(), invalidations)
        val pages = mutableListOf<WorkSummaryPage>()
        backgroundScope.launch { favorites.favorites("episode").collect { pages += it } }
        runCurrent()
        invalidations.emit(Invalidation.UserStateChanged(listOf("w1")))
        runCurrent()
        assertEquals(2, pages.size)

        val counts = mutableListOf<Int>()
        backgroundScope.launch { favorites.count().collect { counts += it } }
        runCurrent()
        assertEquals(listOf(0), counts)

        favorites.page("episode", offset = 20)
        favorites.page("movie", offset = 0, limit = 60)
        favorites.page("movie", offset = 120, limit = 60)
        assertEquals(
            listOf(
                setOf("kind=episode"),
                setOf("kind=episode"),
                setOf("limit=1"),
                setOf("kind=episode", "offset=20"),
                setOf("kind=movie", "limit=60"),
                setOf("kind=movie", "offset=120", "limit=60"),
            ),
            server.seen.map { it.query.split('&').toSet() },
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
        grid.page(GridQuery("l1", favorite = true), offset = 120, limit = 60)
        grid.collections("l1", offset = 60, limit = 60)
        grid.authors("l1", offset = 0, limit = 60)
        grid.bookSeries("l1", offset = 0, limit = 60)
        assertEquals(
            listOf(
                "/api/v1/libraries/l1/works" to setOf("limit=60"),
                "/api/v1/libraries/l1/works" to setOf("favorite=true", "offset=120", "limit=60"),
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
