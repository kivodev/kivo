package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.BookSeries
import com.mmcdole.kivo.api.models.BookSeriesSummaryPage
import com.mmcdole.kivo.api.models.Collection
import com.mmcdole.kivo.api.models.CollectionSource
import com.mmcdole.kivo.api.models.CollectionSummaryPage
import com.mmcdole.kivo.api.models.Home
import com.mmcdole.kivo.api.models.HomeRow
import com.mmcdole.kivo.api.infrastructure.ApiClient
import com.mmcdole.kivo.api.models.HomeRowKind
import com.mmcdole.kivo.api.models.LibraryKind
import com.mmcdole.kivo.api.models.ListKind
import com.mmcdole.kivo.api.models.ListSort
import com.mmcdole.kivo.api.models.MatchState
import com.mmcdole.kivo.api.models.Person
import com.mmcdole.kivo.api.models.PersonRef
import com.mmcdole.kivo.api.models.PersonSummaryPage
import com.mmcdole.kivo.api.models.Resolution
import com.mmcdole.kivo.api.models.Rule
import com.mmcdole.kivo.api.models.RuleGroup
import com.mmcdole.kivo.api.models.RuleGroupMatch
import com.mmcdole.kivo.api.models.SortKey
import com.mmcdole.kivo.api.models.WorkFieldValue
import com.mmcdole.kivo.api.models.WorkFieldValueList
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkList
import com.mmcdole.kivo.api.models.WorkListList
import com.mmcdole.kivo.api.models.WorkQuery
import com.mmcdole.kivo.api.models.WorkQueryRequest
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.api.models.WorkSummaryList
import com.mmcdole.kivo.api.models.WorkSummaryPage
import com.mmcdole.kivo.core.FakeServer
import com.mmcdole.kivo.core.empty
import com.mmcdole.kivo.core.json
import com.mmcdole.kivo.core.live.Invalidation
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
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
            json(Home.serializer(), Home(listOf(HomeRow("recentlyAdded:l1", HomeRowKind.RecentlyAdded, emptyList()))))
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
        server.on("GET", "/api/v1/collections/c1") { json(Collection.serializer(), Collection("c1", "Dune Collection", WorkImages(), emptyList(), CollectionSource.Provider)) }
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
        backgroundScope.launch { lists.lists(false).collect { read += it } }
        runCurrent()
        invalidations.emit(Invalidation.ListsChanged)
        runCurrent()
        assertEquals(2, read.size)
        invalidations.emit(Invalidation.HomeChanged)
        runCurrent()
        assertEquals(2, read.size)

        lists.page("l1", offset = 20)
        lists.page("l1", offset = 0, limit = 60)
        lists.page("l1", offset = 0, kinds = listOf(WorkKind.Movie, WorkKind.Series), sort = ListSort.DateAdded)
        assertEquals(
            listOf(setOf("offset=20"), setOf("limit=60"), setOf("kind=movie", "kind=series", "sort=dateAdded")),
            server.to("GET", "/api/v1/lists/l1/works").map { it.query.split('&').toSet() },
        )
    }

    @Test
    fun aListIsSortedMovedAndReadInItsCustomOrder() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        val list = WorkList("l1", "Later", ListKind.Manual, builtIn = false, workCount = 201, sort = ListSort.Title)
        server.on("PATCH", "/api/v1/lists/l1") { json(WorkList.serializer(), list) }
        server.on("POST", "/api/v1/lists/l1/move") { empty() }
        server.on("GET", "/api/v1/lists/l1/works") { request ->
            val offset = request.url.parameters["offset"]?.toInt() ?: 0
            val works = (offset until minOf(offset + 200, 201)).map { WorkSummary("w$it", WorkKind.Movie, "Movie $it", MatchState.Matched, true, WorkImages()) }
            json(WorkSummaryPage.serializer(), WorkSummaryPage(works, total = 201.takeIf { offset == 0 }))
        }
        val lists = ListsRepository(server.connection(), MutableSharedFlow())

        assertEquals(ListSort.Title, ListSorts.of(lists.setSort("l1", ListSort.Title)))
        assertEquals("""{"sort":"title"}""", server.to("PATCH", "/api/v1/lists/l1").single().body)
        assertEquals(ListSort.Custom, ListSorts.of(list.copy(sort = ListSort.Unknown)))
        assertEquals(null, ListSorts.of(list.copy(kind = ListKind.Smart, sort = null)))

        lists.move("l1", listOf("w3", "w1"), after = "w7")
        lists.move("l1", listOf("w2"), after = null)
        assertEquals(
            listOf("""{"workIds":["w3","w1"],"after":"w7"}""", """{"workIds":["w2"]}"""),
            server.to("POST", "/api/v1/lists/l1/move").map { it.body },
        )

        assertEquals((0..200).map { "w$it" }, lists.inCustomOrder("l1").map { it.id })
        assertEquals(
            listOf(setOf("sort=custom", "limit=200"), setOf("sort=custom", "limit=200", "offset=200")),
            server.to("GET", "/api/v1/lists/l1/works").map { it.query.split('&').toSet() },
        )
    }

    @Test
    fun aGridPagesByOffsetAndHearsOnlyItsLibrary() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        server.on("POST", "/api/v1/works/query") { json(WorkSummaryPage.serializer(), WorkSummaryPage(emptyList(), total = 0)) }
        server.on("GET", "/api/v1/libraries/l1/collections") { json(CollectionSummaryPage.serializer(), CollectionSummaryPage(emptyList(), 0)) }
        server.on("GET", "/api/v1/libraries/l1/authors") { json(PersonSummaryPage.serializer(), PersonSummaryPage(emptyList(), 0)) }
        server.on("GET", "/api/v1/libraries/l1/book-series") { json(BookSeriesSummaryPage.serializer(), BookSeriesSummaryPage(emptyList(), 0)) }
        val invalidations = MutableSharedFlow<Invalidation>()
        val grid = GridRepository(server.connection(), invalidations)

        grid.page(GridQuery("l1"), LibraryKind.Movies, offset = 0, limit = 60)
        grid.page(GridQuery("l1", sort = GridSort.Author, genres = listOf("Drama"), yearFrom = 1990, yearTo = 1999), LibraryKind.Audiobooks, offset = 120, limit = 60)
        grid.collections("l1", offset = 60, limit = 60)
        grid.authors("l1", offset = 0, limit = 60)
        grid.bookSeries("l1", offset = 0, limit = 60)
        assertEquals(
            listOf(
                "/api/v1/works/query" to setOf(""),
                "/api/v1/works/query" to setOf(""),
                "/api/v1/libraries/l1/collections" to setOf("offset=60", "limit=60"),
                "/api/v1/libraries/l1/authors" to setOf("limit=60"),
                "/api/v1/libraries/l1/book-series" to setOf("limit=60"),
            ),
            server.seen.map { it.path to it.query.split('&').toSet() },
            "the first page, which carries the total and letters, has no offset",
        )
        assertEquals(
            listOf(
                WorkQueryRequest(WorkQuery(listOf(WorkKind.Movie), sort = listOf(SortKey("title"))), library = "l1", limit = 60),
                WorkQueryRequest(
                    WorkQuery(
                        listOf(WorkKind.Book),
                        where = RuleGroup(RuleGroupMatch.All, listOf(Rule("genre", "anyOf", listOf("Drama")), Rule("year", "between", listOf("1990", "1999")))),
                        sort = listOf(SortKey("author")),
                    ),
                    library = "l1",
                    offset = 120,
                    limit = 60,
                ),
            ),
            server.to("POST", "/api/v1/works/query").map { ApiClient.JSON_DEFAULT.decodeFromString(WorkQueryRequest.serializer(), it.body) },
            "the library is the request's, not a rule",
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
    fun homesRowsOfOneKindAreAskedForByItsName() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        server.on("GET", "/api/v1/home") {
            respond(
                """{"rows":[{"id":"continueWatching","kind":"continueWatching","items":[]},{"id":"shelf:1","kind":"shelf","items":[]}]}""",
                HttpStatusCode.OK,
                headersOf(HttpHeaders.ContentType, "application/json"),
            )
        }
        val homes = mutableListOf<Home>()
        backgroundScope.launch { HomeRepository(server.connection(), MutableSharedFlow()).home(HomeRowKind.ContinueWatching).collect { homes += it } }
        runCurrent()
        assertEquals("kind=continueWatching", server.to("GET", "/api/v1/home").single().query)
        assertEquals(listOf(HomeRowKind.ContinueWatching, HomeRowKind.Unknown), homes.single().rows.map { it.kind }, "a row of a later server's kind is read, for the app to skip")
    }

    @Test
    fun aFilterValueALaterServerAddsIsLeftOut() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        val values = mapOf("genre" to emptyList(), "year" to emptyList(), "contentRating" to emptyList(), "resolution" to listOf("8k", "4k", "sd"), "dynamicRange" to listOf("hdr", "hdr10000", "dolbyVision"))
        for ((field, found) in values) {
            server.on("GET", "/api/v1/work-fields/$field/values") {
                json(WorkFieldValueList.serializer(), WorkFieldValueList(found.map { WorkFieldValue(it, it) }))
            }
        }
        server.on("GET", "/api/v1/libraries/l1/collections") { json(CollectionSummaryPage.serializer(), CollectionSummaryPage(emptyList(), 0)) }
        val read = mutableListOf<LibraryFilters>()
        backgroundScope.launch { GridRepository(server.connection(), MutableSharedFlow()).filters("l1").collect { read += it } }
        runCurrent()
        assertEquals(listOf(Resolution.FourK, Resolution.Sd), read.single().resolutions)
        assertEquals(listOf(DynamicRange.Hdr, DynamicRange.DolbyVision), read.single().dynamicRanges)
    }

    @Test
    fun aLibrarysFiltersAreItsWorksValuesReadAgainAsItChangesAlone() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        val values = mapOf(
            "genre" to listOf("Crime", "Drama"),
            "year" to listOf("1995", "2021"),
            "contentRating" to listOf("PG", "R"),
            "resolution" to listOf("4k"),
            "dynamicRange" to listOf("hdr"),
        )
        for ((field, found) in values) {
            server.on("GET", "/api/v1/work-fields/$field/values") {
                json(WorkFieldValueList.serializer(), WorkFieldValueList(found.map { WorkFieldValue(it, it) }))
            }
        }
        server.on("GET", "/api/v1/work-fields/narrator/values") {
            json(WorkFieldValueList.serializer(), WorkFieldValueList(listOf(WorkFieldValue("p1", "Ray Porter"))))
        }
        server.on("GET", "/api/v1/libraries/l1/collections") { json(CollectionSummaryPage.serializer(), CollectionSummaryPage(emptyList(), 2)) }
        val invalidations = MutableSharedFlow<Invalidation>()
        val grid = GridRepository(server.connection(), invalidations)

        val read = mutableListOf<LibraryFilters>()
        backgroundScope.launch { grid.filters("l1").collect { read += it } }
        runCurrent()
        assertEquals(
            LibraryFilters(
                listOf("Crime", "Drama"), listOf(1995, 2021), listOf("PG", "R"), listOf(Resolution.FourK), listOf(DynamicRange.Hdr),
                listOf(PersonRef("p1", "Ray Porter")), collections = true,
            ),
            read.single(),
        )
        assertEquals(
            (values.keys + "narrator").map { "/api/v1/work-fields/$it/values" to "library=l1" }.toSet() + ("/api/v1/libraries/l1/collections" to "limit=1"),
            server.seen.map { it.path to it.query }.toSet(),
        )
        invalidations.emit(Invalidation.LibraryChanged(listOf("l2")))
        invalidations.emit(Invalidation.UserStateChanged(listOf("w1")))
        runCurrent()
        assertEquals(1, read.size, "another library's change and a played mark leave them")
        invalidations.emit(Invalidation.LibraryChanged(listOf("l1")))
        runCurrent()
        assertEquals(2, read.size)
    }
}
