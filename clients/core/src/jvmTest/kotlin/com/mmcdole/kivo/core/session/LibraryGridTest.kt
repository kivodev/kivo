package com.mmcdole.kivo.core.session

import androidx.datastore.preferences.core.stringPreferencesKey
import com.mmcdole.kivo.api.models.CollectionSummaryPage
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.LibraryKind
import com.mmcdole.kivo.api.models.LibraryList
import com.mmcdole.kivo.api.models.MatchState
import com.mmcdole.kivo.api.models.Resolution
import com.mmcdole.kivo.api.models.WorkFieldValue
import com.mmcdole.kivo.api.models.WorkFieldValueList
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkQueryRequest
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.api.models.WorkSummaryPage
import com.mmcdole.kivo.core.data.DynamicRange
import com.mmcdole.kivo.core.data.FilterGroup
import com.mmcdole.kivo.core.data.FilterGroupKind
import com.mmcdole.kivo.core.data.GridFilter
import com.mmcdole.kivo.core.data.GridFilters
import com.mmcdole.kivo.core.data.GridSort
import com.mmcdole.kivo.core.data.LibraryTab
import com.mmcdole.kivo.core.data.SavedGrid
import com.mmcdole.kivo.core.data.SavedGrids
import com.mmcdole.kivo.core.data.WorkPlayState
import com.mmcdole.kivo.core.json
import com.mmcdole.kivo.core.kivoJson
import io.ktor.http.content.OutgoingContent
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.currentTime
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import java.io.IOException
import java.util.concurrent.CopyOnWriteArrayList
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotSame
import kotlin.test.assertNull
import kotlin.test.assertSame
import kotlin.test.assertTrue

// A library's grid as its screen gets it from SessionLibrary.libraryGrid:
// what a library of each kind offers, the rules of choosing, what's kept on
// the device, against a server answered by Ktor's mock engine.

private val movies = Library("movies", "Movies", true, LibraryKind.Movies, 3)
private val shows = Library("shows", "Shows", true, LibraryKind.Shows, 3)
private val videos = Library("videos", "Home videos", true, LibraryKind.Videos, 3)
private val books = Library("books", "Books", true, LibraryKind.Audiobooks, 3)

private val action = GridFilter.Genre("Action")
private val drama = GridFilter.Genre("Drama")
private val unplayed = GridFilter.Played(WorkPlayState.Unplayed)
private val nineties = GridFilter.Decade(1990)
private val fourK = GridFilter.Resolution(Resolution.FourK)
private val hdr = GridFilter.DynamicRange(DynamicRange.Hdr)
private val ray = GridFilter.Narrator("p1", "Ray Porter")

private fun filters(vararg on: GridFilter) = on.fold(GridFilters.None) { chosen, filter -> chosen.choosing(filter) }

/** A device signed in to a server with a library of each kind, [collections] in the movies, and what its grids asked for. */
private class Grids(private val scope: TestScope, collections: Int = 2, val h: Harness = Harness(scope)) {
    /** Each read of a works grid, as the server was asked. */
    val asked: MutableList<WorkQueryRequest> = CopyOnWriteArrayList()

    init {
        val servers = h.servers
        servers.server("home.test", info("server-1"))
        servers.on("home.test", "GET", "/api/v1/libraries") { json(LibraryList.serializer(), LibraryList(listOf(movies, shows, videos, books))) }
        val values = mapOf(
            "genre" to listOf("Action", "Comedy", "Drama"),
            "year" to listOf("1994", "1999", "2011", "2024"),
            "contentRating" to listOf("PG", "R"),
            "resolution" to listOf("4k", "1080p"),
            "dynamicRange" to listOf("hdr"),
        )
        for ((field, found) in values) {
            servers.on("home.test", "GET", "/api/v1/work-fields/$field/values") {
                json(WorkFieldValueList.serializer(), WorkFieldValueList(found.map { WorkFieldValue(it, it) }))
            }
        }
        servers.on("home.test", "GET", "/api/v1/work-fields/narrator/values") {
            json(WorkFieldValueList.serializer(), WorkFieldValueList(listOf(WorkFieldValue("p1", "Ray Porter"), WorkFieldValue("p2", "Kate Reading"))))
        }
        for (library in listOf(movies, shows, videos, books)) {
            val has = if (library == movies) collections else 0
            servers.on("home.test", "GET", "/api/v1/libraries/${library.id}/collections") { json(CollectionSummaryPage.serializer(), CollectionSummaryPage(emptyList(), has)) }
        }
        servers.on("home.test", "POST", "/api/v1/works/query") { request ->
            asked += kivoJson.decodeFromString(WorkQueryRequest.serializer(), (request.body as OutgoingContent.ByteArrayContent).bytes().decodeToString())
            val work = WorkSummary("w1", WorkKind.Movie, "Heat", MatchState.Matched, true, WorkImages())
            json(WorkSummaryPage.serializer(), WorkSummaryPage(listOf(work), 1))
        }
        h.device.signedInTo("server-1")
    }

    /** A session of the app, opened as it starts: one after another is the app opened again. */
    suspend fun session(): KivoSession = KivoClient(h.device, h.servers.engine, h.core.scope).restore(home)

    /** The filters of the grid's last read, by field. */
    fun rules(): Map<String, List<String>> = asked.last().query.where?.rules.orEmpty().associate { it.field to it.propertyValues.orEmpty() }

    /** What the device keeps for [libraryId], once what's on its way there is kept. */
    suspend fun kept(libraryId: String): String? {
        scope.runCurrent()
        return h.device.preferences.data.first()[stringPreferencesKey("kivo.grid.$libraryId")]
    }
}

/** The state once [holds] of it. */
private suspend fun LibraryGrid.await(holds: (LibraryGridState) -> Boolean): LibraryGridState = state.first(holds)

/** The grid's count, once its first page answers. */
private suspend fun LibraryGridState.total(): Int = pager.state.first { it.total != null }.total!!

private fun List<FilterGroup>.kinds() = map { it.kind }

@OptIn(ExperimentalCoroutinesApi::class)
class LibraryGridTest {
    @Test
    fun aVideoLibraryOpensOnItsWorksByTitleWithCollectionsOnceItSaysItHasThem() = runTest {
        val grids = Grids(this)
        val grid = grids.session().library.libraryGrid("movies")

        val first = grid.state.value
        assertEquals(LibraryTab.Titles, first.tab)
        assertEquals(GridSort.Title, first.sort)
        assertTrue(first.filters.isEmpty)
        assertEquals(listOf(LibraryTab.Titles), first.tabs)
        assertFalse(first.tabsKnown, "not until the library says whether it has collections")
        assertEquals(OfferRead.Pending, first.offerRead)
        assertEquals(listOf(FilterGroupKind.WatchStatus), first.offer.kinds(), "the watch status alone until the values are read")

        val read = grid.await { it.offerRead != OfferRead.Pending && it.library != null }
        assertEquals(OfferRead.Read, read.offerRead)
        assertEquals(movies, read.library)
        assertEquals(listOf(LibraryTab.Titles, LibraryTab.Collections), read.tabs)
        assertTrue(read.tabsKnown)
        assertEquals(1, read.total())
    }

    @Test
    fun aLibraryWhoseValuesCantBeReadHasItsWorksAlone() = runTest {
        val grids = Grids(this)
        for (field in listOf("genre", "year", "contentRating", "resolution", "dynamicRange", "narrator")) {
            grids.h.servers.on("home.test", "GET", "/api/v1/work-fields/$field/values") { throw IOException("offline") }
        }
        val grid = grids.session().library.libraryGrid("movies")

        val read = grid.await { it.offerRead != OfferRead.Pending }
        assertEquals(OfferRead.Failed, read.offerRead)
        assertTrue(read.tabsKnown)
        assertEquals(listOf(LibraryTab.Titles), read.tabs)
        assertEquals(listOf(FilterGroupKind.WatchStatus), read.offer.kinds())
    }

    @Test
    fun eachKindOfLibraryOffersItsOwnSortsTabsAndCards() = runTest {
        val session = Grids(this).session()
        suspend fun read(library: Library) = session.library.libraryGrid(library.id).let { grid -> grid.await { it.library != null && it.offerRead != OfferRead.Pending }.also { grid.close() } }

        val everyVideo = listOf(GridSort.Title, GridSort.AddedAt, GridSort.ReleaseDate, GridSort.Rating, GridSort.Runtime, GridSort.LastPlayedAt)
        assertEquals(everyVideo, read(movies).sorts)
        assertEquals(everyVideo, read(videos).sorts)
        assertEquals(
            listOf(GridSort.Title, GridSort.AddedAt, GridSort.LastEpisodeAddedAt, GridSort.ReleaseDate, GridSort.Rating, GridSort.Runtime, GridSort.LastPlayedAt),
            read(shows).sorts,
        )
        assertEquals(listOf(GridSort.Title, GridSort.Author, GridSort.AddedAt, GridSort.ReleaseDate), read(books).sorts)

        assertEquals(listOf(LibraryTab.Titles), read(shows).tabs)
        assertEquals(listOf(LibraryTab.Titles, LibraryTab.Authors, LibraryTab.BookSeries), read(books).tabs)

        assertEquals(GridCardShape.Poster, read(movies).cardShape)
        assertEquals(GridCardShape.Poster, read(shows).cardShape)
        assertEquals(GridCardShape.Landscape, read(videos).cardShape)
        assertEquals(GridCardShape.Square, read(books).cardShape)
    }

    @Test
    fun aTabsCardsAreItsOwnShape() = runTest {
        val session = Grids(this).session()
        val inMovies = session.library.libraryGrid("movies")
        inMovies.choose(LibraryTab.Collections)
        assertEquals(GridCardShape.Poster, inMovies.state.value.cardShape)

        val inVideos = session.library.libraryGrid("videos")
        inVideos.await { it.library != null }
        inVideos.choose(LibraryTab.Collections)
        assertEquals(GridCardShape.Poster, inVideos.state.value.cardShape, "collections are posters in a library of landscape cards")
        assertEquals(listOf(LibraryTab.Titles, LibraryTab.Collections), inVideos.state.value.tabs, "the tab showing is always offered")

        val inBooks = session.library.libraryGrid("books")
        for (tab in listOf(LibraryTab.Authors, LibraryTab.BookSeries)) {
            inBooks.choose(tab)
            assertEquals(GridCardShape.Square, inBooks.state.value.cardShape)
        }
    }

    @Test
    fun aLibraryOpenedAgainInTheSessionKnowsItsKindAtOnce() = runTest {
        val session = Grids(this).session()
        session.library.libraries().first { it.getOrNull() != null }

        val grid = session.library.libraryGrid("books")

        assertEquals(books, grid.state.value.library)
        assertEquals(listOf(LibraryTab.Titles, LibraryTab.Authors, LibraryTab.BookSeries), grid.state.value.tabs)
        assertEquals(GridCardShape.Square, grid.state.value.cardShape)
    }

    @Test
    fun aVideoLibraryOffersDecadesGenresRatingsAndQualityInTheGroupsOrder() = runTest {
        val grid = Grids(this).session().library.libraryGrid("movies")

        val offer = grid.await { it.offerRead != OfferRead.Pending && it.library != null }.offer

        assertEquals(
            listOf(FilterGroupKind.WatchStatus, FilterGroupKind.Decade, FilterGroupKind.Genre, FilterGroupKind.ContentRating, FilterGroupKind.Quality),
            offer.kinds(),
        )
        assertEquals(WorkPlayState.entries.map(GridFilter::Played), offer[0].values)
        assertEquals(listOf(2020, 2010, 1990).map(GridFilter::Decade), offer[1].values, "newest first")
        assertEquals(listOf("Action", "Comedy", "Drama").map(GridFilter::Genre), offer[2].values)
        assertEquals(listOf("PG", "R").map(GridFilter::ContentRating), offer[3].values)
        assertEquals(
            listOf(GridFilter.Resolution(Resolution.FourK), GridFilter.Resolution(Resolution.P1080), hdr),
            offer[4].values,
            "the resolutions, then the dynamic ranges",
        )
    }

    @Test
    fun aBooksLibraryOffersDecadesGenresAndNarratorsAndNoRatingsOrQuality() = runTest {
        val grid = Grids(this).session().library.libraryGrid("books")

        val offer = grid.await { it.offerRead != OfferRead.Pending && it.library != null }.offer

        assertEquals(listOf(FilterGroupKind.WatchStatus, FilterGroupKind.Decade, FilterGroupKind.Genre, FilterGroupKind.Narrator), offer.kinds())
        assertEquals(listOf(ray, GridFilter.Narrator("p2", "Kate Reading")), offer.last().values)
    }

    @Test
    fun aGroupThatTakesOneHasOneOnAndTheOthersAnyNumber() {
        assertEquals(
            listOf(FilterGroupKind.WatchStatus, FilterGroupKind.Decade, FilterGroupKind.Narrator),
            FilterGroupKind.entries.filter { it.single },
        )
        val chosen = filters(unplayed, nineties, action, drama, fourK, hdr, ray)
        assertEquals(listOf(unplayed, nineties, action, drama, fourK, hdr, ray), chosen.chosen, "in the groups' order")
        assertEquals(WorkPlayState.Unplayed, chosen.played)

        val played = GridFilter.Played(WorkPlayState.Played)
        assertEquals(listOf(played, nineties, action, drama, fourK, hdr, ray), chosen.toggling(played).chosen, "another watch status takes its place")
        assertEquals(listOf(unplayed, GridFilter.Decade(2010), action, drama, fourK, hdr, ray), chosen.toggling(GridFilter.Decade(2010)).chosen)
        assertEquals(listOf(unplayed, nineties, action, drama, fourK, hdr, GridFilter.Narrator("p2", null)), chosen.toggling(GridFilter.Narrator("p2", null)).chosen)

        assertEquals(listOf(unplayed, action, drama, fourK, hdr, ray), chosen.toggling(nineties).chosen, "the one on, chosen again, goes off")
        assertEquals(listOf(unplayed, nineties, action, drama, fourK, hdr), chosen.toggling(GridFilter.Narrator("p1", null)).chosen, "a narrator is the same by any name")
        assertEquals(listOf(unplayed, nineties, drama, fourK, hdr, ray), chosen.toggling(action).chosen)
        assertEquals(listOf(unplayed, nineties, action, drama, hdr, ray), chosen.toggling(fourK).chosen)

        assertEquals(chosen, chosen.choosing(nineties), "choosing the one on leaves it on")
        assertEquals(chosen, chosen.choosing(action))
        assertEquals(listOf(nineties, action, drama, fourK, hdr, ray), chosen.clearing(FilterGroupKind.WatchStatus).chosen)
        assertEquals(listOf(unplayed, nineties, action, drama, ray), chosen.clearing(FilterGroupKind.Quality).chosen)
        assertEquals(filters(drama, action), filters(action, drama), "the same choice in whatever order it was made")
        assertTrue(GridFilters.None.isEmpty)
        assertFalse(chosen.isEmpty)
        assertTrue(chosen.has(drama))
        assertFalse(chosen.has(GridFilter.Genre("Comedy")))
    }

    @Test
    fun theFiltersOnComeInTheGroupsOrderAndEachGroupsAsOffered() = runTest {
        val grid = Grids(this).session().library.libraryGrid("movies")
        val comedy = GridFilter.Genre("Comedy")
        val unknown = GridFilter.Genre("Zombies")
        val p1080 = GridFilter.Resolution(Resolution.P1080)
        grid.choose(filters(hdr, p1080, fourK, unknown, drama, action, comedy, GridFilter.ContentRating("R"), nineties, unplayed))

        val read = grid.await { it.offerRead != OfferRead.Pending && it.library != null }

        assertEquals(
            listOf(unplayed, nineties, action, comedy, drama, unknown, GridFilter.ContentRating("R"), fourK, p1080, hdr),
            read.activeFilters,
            "a value the library doesn't offer after its group's others",
        )

        grid.choose(LibraryTab.Collections)
        assertEquals(emptyList(), grid.state.value.activeFilters, "none over a tab they don't filter")
        assertFalse(grid.state.value.filters.isEmpty, "kept for the works")
        grid.choose(LibraryTab.Titles)
        assertEquals(10, grid.state.value.activeFilters.size)
    }

    @Test
    fun aNarratorOnGoesByTheNameTheLibraryGives() = runTest {
        val grid = Grids(this).session().library.libraryGrid("books")
        grid.choose(filters(GridFilter.Narrator("p1", null)))
        assertEquals(listOf(GridFilter.Narrator("p1", null)), grid.state.value.activeFilters)

        assertEquals(listOf(ray), grid.await { it.offerRead != OfferRead.Pending && it.library != null }.activeFilters)
    }

    @Test
    fun anEmptyGridSaysWhetherTheWatchStatusAloneOrTheFiltersLeftItEmpty() = runTest {
        val grid = Grids(this).session().library.libraryGrid("movies")
        assertEquals(GridEmptyCase.Unfiltered, grid.state.value.emptyCase)

        grid.choose(filters(unplayed))
        assertEquals(GridEmptyCase.WatchStatus, grid.state.value.emptyCase)

        grid.choose(filters(unplayed, action))
        assertEquals(GridEmptyCase.Filtered, grid.state.value.emptyCase)
        grid.choose(filters(action))
        assertEquals(GridEmptyCase.Filtered, grid.state.value.emptyCase)

        grid.choose(LibraryTab.Collections)
        assertEquals(GridEmptyCase.Unfiltered, grid.state.value.emptyCase, "a tab the filters don't narrow")

        grid.choose(LibraryTab.Titles)
        grid.clearFilters()
        assertEquals(GridEmptyCase.Unfiltered, grid.state.value.emptyCase)
        assertTrue(grid.state.value.filters.isEmpty)
    }

    @Test
    fun aLongGroupShowsItsFirstFewAndAnyChosenUntilOpenedWhole() {
        val genres = (1..20).map { GridFilter.Genre("G$it") }
        val long = FilterGroup(FilterGroupKind.Genre, genres)
        assertTrue(long.isLong)
        assertEquals(genres.take(9), long.shown(whole = false, chosen = GridFilters.None))
        assertEquals(genres.take(9) + genres[14], long.shown(whole = false, chosen = filters(genres[14])))
        assertEquals(genres, long.shown(whole = true, chosen = GridFilters.None))

        val short = FilterGroup(FilterGroupKind.Genre, genres.take(12))
        assertFalse(short.isLong)
        assertEquals(genres.take(12), short.shown(whole = false, chosen = GridFilters.None))

        assertTrue(FilterGroup(FilterGroupKind.Narrator, (1..13).map { GridFilter.Narrator("p$it", "N$it") }).isLong)
        val ratings = FilterGroup(FilterGroupKind.ContentRating, (1..20).map { GridFilter.ContentRating("R$it") })
        assertFalse(ratings.isLong, "only the genres and narrators are shortened")
        assertEquals(20, ratings.shown(whole = false, chosen = GridFilters.None).size)
    }

    @Test
    fun aNewSortTabOrFilterShowsANewGridAndTheOldOneStopsReading() = runTest {
        val grids = Grids(this)
        val grid = grids.session().library.libraryGrid("movies")
        val first = grid.state.value.pager
        assertEquals(1, grid.state.value.total())

        grid.choose(GridSort.AddedAt)
        val sorted = grid.state.value.pager
        assertNotSame(first, sorted)
        assertEquals(GridSort.AddedAt, grid.state.value.sort)
        assertEquals(1, grid.state.value.total())
        assertEquals("addedAt", grids.asked.last().query.sort?.single()?.field)

        grid.choose(GridSort.AddedAt)
        assertSame(sorted, grid.state.value.pager, "the sort it has")

        grid.choose(filters(unplayed, nineties, action, drama, fourK, hdr))
        assertNotSame(sorted, grid.state.value.pager)
        assertEquals(1, grid.state.value.total())
        assertEquals(
            mapOf(
                "playState" to listOf("unplayed"),
                "genre" to listOf("Action", "Drama"),
                "year" to listOf("1990", "1999"),
                "resolution" to listOf("4k"),
                "dynamicRange" to listOf("hdr"),
            ),
            grids.rules(),
            "a decade is its ten years",
        )

        val filtered = grid.state.value.pager
        grid.choose(LibraryTab.Collections)
        val collections = grid.state.value.pager
        assertNotSame(filtered, collections)
        assertEquals(2, grid.state.value.total(), "the collections' own count")
        grid.choose(GridSort.Rating)
        assertSame(collections, grid.state.value.pager, "the works' sort doesn't change the collections")

        grid.choose(LibraryTab.Titles)
        assertEquals(1, grid.state.value.total())
        assertEquals("rating", grids.asked.last().query.sort?.single()?.field)
    }

    @Test
    fun aSheetsChoicesReachTheGridOnceTheyHoldStill() = runTest {
        val grids = Grids(this)
        val grid = grids.session().library.libraryGrid("movies")
        val first = grid.state.value.pager
        val start = currentTime

        grid.choose(filters(action), settle = true)
        grid.choose(filters(action, drama), settle = true)
        assertEquals(filters(action, drama), grid.state.value.filters, "chosen at once")
        assertSame(first, grid.state.value.pager, "not shown while they're being chosen")

        val shown = grid.await { it.pager !== first }
        assertEquals(300, currentTime - start)
        shown.total()
        assertEquals(mapOf("genre" to listOf("Action", "Drama")), grids.rules())
        assertEquals(1, grids.asked.count { it.query.where != null }, "the choices in between aren't read")
    }

    @Test
    fun aChoiceMadeAtOnceOverridesOneOnItsWay() = runTest {
        val grids = Grids(this)
        val grid = grids.session().library.libraryGrid("movies")
        val start = currentTime

        grid.choose(filters(action), settle = true)
        val waiting = grid.state.value.pager
        grid.choose(grid.state.value.filters)
        assertNotSame(waiting, grid.state.value.pager, "as the sheet closes")
        assertEquals(start, currentTime)

        grid.clearFilters(settle = true)
        grid.choose(filters(drama))
        val shown = grid.state.value.pager
        shown.state.first { it.total != null }
        assertEquals(mapOf("genre" to listOf("Drama")), grids.rules())
        delay(400)
        assertSame(shown, grid.state.value.pager, "the one on its way never arrives")
    }

    @Test
    fun whatsChosenIsKeptForTheLibraryAndOpensItNextTime() = runTest {
        val grids = Grids(this)
        val grid = grids.session().library.libraryGrid("movies")
        grid.choose(GridSort.Rating)
        grid.choose(filters(unplayed, nineties, action, drama, GridFilter.ContentRating("PG"), fourK, hdr))
        grid.choose(LibraryTab.Collections)
        grid.await { it.offerRead != OfferRead.Pending }
        grid.close()

        val again = grids.session()
        val reopened = again.library.libraryGrid("movies").state.value
        assertEquals(LibraryTab.Collections, reopened.tab)
        assertEquals(GridSort.Rating, reopened.sort)
        assertEquals(filters(unplayed, nineties, action, drama, GridFilter.ContentRating("PG"), fourK, hdr), reopened.filters)
        assertTrue(reopened.tabsKnown, "its collections are known from the first frame")
        assertEquals(listOf(LibraryTab.Titles, LibraryTab.Collections), reopened.tabs)

        assertTrue(again.library.libraryGrid("shows").state.value.filters.isEmpty, "each library its own")
    }

    @Test
    fun everyTabOfABooksLibraryIsKeptWithItsNarrator() = runTest {
        val grids = Grids(this)
        val grid = grids.session().library.libraryGrid("books")
        grid.choose(GridSort.Author)
        grid.choose(filters(ray))
        for (tab in listOf(LibraryTab.Authors, LibraryTab.BookSeries)) {
            grid.choose(tab)
            assertEquals(tab, grids.session().library.libraryGrid("books").state.value.tab)
        }

        val reopened = grids.session().library.libraryGrid("books").state.value
        assertEquals(GridSort.Author, reopened.sort)
        assertEquals(listOf(ray), reopened.filters.chosen, "with the name it had, to show before the narrators are read")
    }

    @Test
    fun aLibraryOpenedSortedForOnceStartsFromThatSortAloneAndKeepsNothing() = runTest {
        val grids = Grids(this)
        val session = grids.session()
        val usual = session.library.libraryGrid("movies")
        usual.choose(GridSort.Rating)
        usual.choose(filters(action))
        usual.choose(LibraryTab.Collections)
        usual.await { it.offerRead != OfferRead.Pending }
        val kept = grids.kept("movies")

        val once = session.library.libraryGrid("movies", GridSort.AddedAt)
        assertEquals(LibraryTab.Titles, once.state.value.tab)
        assertEquals(GridSort.AddedAt, once.state.value.sort)
        assertTrue(once.state.value.filters.isEmpty, "none of the kept filters")
        assertTrue(once.state.value.tabsKnown)

        once.choose(GridSort.Runtime)
        once.choose(filters(drama, unplayed))
        once.choose(filters(drama), settle = true)
        delay(400)
        assertEquals(1, once.state.value.total())
        once.await { it.offerRead != OfferRead.Pending }

        assertEquals(kept, grids.kept("movies"))
        val reopened = grids.session().library.libraryGrid("movies").state.value
        assertEquals(LibraryTab.Collections, reopened.tab)
        assertEquals(GridSort.Rating, reopened.sort)
        assertEquals(filters(action), reopened.filters)

        val never = session.library.libraryGrid("shows", GridSort.AddedAt)
        never.choose(filters(action))
        never.await { it.offerRead != OfferRead.Pending }
        assertEquals(SavedGrid(collections = false), grids.kept("shows")?.let(SavedGrids::decode), "only whether it has collections, for a library only ever opened for once")
    }

    @Test
    fun theQueryForASmartListIsTheLibrarysWorksAsChosen() = runTest {
        val grid = Grids(this).session().library.libraryGrid("shows")
        grid.await { it.library != null }
        grid.choose(GridSort.LastEpisodeAddedAt)
        grid.choose(filters(action), settle = true)

        val query = grid.state.value.smartListQuery

        assertEquals(listOf(WorkKind.Series), query.kinds)
        assertEquals("lastEpisodeAddedAt", query.sort?.single()?.field)
        assertEquals(mapOf("library" to listOf("shows"), "genre" to listOf("Action")), query.where?.rules.orEmpty().associate { it.field to it.propertyValues.orEmpty() })
    }

    @Test
    fun aSavedGridWithValuesThisVersionDoesntKnowOpensWithoutThem() {
        val stored = """{"tab":"shelves","sort":"shoeSize","played":"maybe","decade":1990,"genres":["Drama"],"resolutions":["8k","1080p"],"dynamicRanges":["hdr10000","dolbyVision"],"view":"list"}"""

        assertEquals(
            SavedGrid(
                LibraryTab.Titles,
                GridSort.Title,
                filters(nineties, drama, GridFilter.Resolution(Resolution.P1080), GridFilter.DynamicRange(DynamicRange.DolbyVision)),
            ),
            SavedGrids.decode(stored),
        )
        assertNull(SavedGrids.decode("sort=title"), "not a saved grid")
        val whole = SavedGrid(LibraryTab.BookSeries, GridSort.Author, filters(unplayed, nineties, action, GridFilter.ContentRating("PG-13"), fourK, hdr, ray), collections = false)
        assertEquals(whole, SavedGrids.decode(SavedGrids.encode(whole)))
        assertEquals(SavedGrid(), SavedGrids.decode(SavedGrids.encode(SavedGrid())))
    }
}
