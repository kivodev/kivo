package com.mmcdole.kivo.android.mobile.library

import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import com.mmcdole.kivo.android.library.SavedGrids
import com.mmcdole.kivo.android.library.Section
import com.mmcdole.kivo.android.mobile.CoreTest
import com.mmcdole.kivo.android.mobile.reply
import com.mmcdole.kivo.android.mobile.awaitValue
import com.mmcdole.kivo.android.mobile.ui.CardShape
import com.mmcdole.kivo.android.mobile.work
import com.mmcdole.kivo.api.models.LetterStart
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.infrastructure.ApiClient
import com.mmcdole.kivo.api.models.CollectionSummary
import com.mmcdole.kivo.api.models.CollectionSummaryPage
import com.mmcdole.kivo.api.models.LibraryKind
import com.mmcdole.kivo.api.models.LibraryList
import com.mmcdole.kivo.api.models.Resolution
import com.mmcdole.kivo.api.models.WorkFieldValue
import com.mmcdole.kivo.api.models.WorkFieldValueList
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkQueryRequest
import com.mmcdole.kivo.api.models.WorkSummaryPage
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import com.mmcdole.kivo.android.library.filtered
import com.mmcdole.kivo.android.library.unfiltered
import com.mmcdole.kivo.api.models.PersonRef
import com.mmcdole.kivo.core.data.DynamicRange
import com.mmcdole.kivo.core.data.GridQuery
import com.mmcdole.kivo.core.data.GridSort
import com.mmcdole.kivo.core.data.WorkPlayState
import io.ktor.http.content.OutgoingContent
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import java.util.concurrent.CopyOnWriteArrayList
import kotlin.io.path.createTempDirectory
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

class LibraryViewModelTest : CoreTest() {
    private val movies = Library("lib1", "Movies", pinned = true, kind = LibraryKind.Movies, workCount = 3)
    private val books = Library("lib2", "Books", pinned = true, kind = LibraryKind.Audiobooks, workCount = 0)
    /** The grid's reads, as the server was asked. */
    private val asked: MutableList<WorkQueryRequest> = CopyOnWriteArrayList()
    private val store = PreferenceDataStoreFactory.create { createTempDirectory("grids").resolve("grids.preferences_pb").toFile() }
    private val saved = SavedGrids(store)

    init {
        server.get("/api/v1/libraries", LibraryList.serializer()) { LibraryList(listOf(movies, books)) }
        val values = mapOf(
            "genre" to listOf("Action", "Comedy", "Drama"),
            "year" to listOf("1994", "1999", "2011", "2024"),
            "contentRating" to listOf("PG", "PG-13", "R"),
            "resolution" to listOf("1080p"),
            "dynamicRange" to listOf("hdr"),
        )
        for ((field, items) in values) {
            server.get("/api/v1/work-fields/$field/values", WorkFieldValueList.serializer()) { WorkFieldValueList(items.map { WorkFieldValue(it, it) }) }
        }
        server.get("/api/v1/work-fields/narrator/values", WorkFieldValueList.serializer()) {
            WorkFieldValueList(listOf(WorkFieldValue("p1", "Ray Porter"), WorkFieldValue("p2", "Kate Reading")))
        }
        server.get("/api/v1/libraries/lib1/collections", CollectionSummaryPage.serializer()) {
            CollectionSummaryPage(listOf(CollectionSummary("c1", "Dune Collection", 2, WorkImages())), 1)
        }
        server.get("/api/v1/libraries/lib2/collections", CollectionSummaryPage.serializer()) { CollectionSummaryPage(emptyList(), 0) }
        server.on("POST", "/api/v1/works/query") { request ->
            asked += ApiClient.JSON_DEFAULT.decodeFromString(WorkQueryRequest.serializer(), (request.body as OutgoingContent.ByteArrayContent).bytes().decodeToString())
            reply(
                WorkSummaryPage.serializer(),
                WorkSummaryPage(listOf(work("a", "Algiers"), work("b", "Battleship"), work("c", "Charade")), 3, listOf(LetterStart("A", 0), LetterStart("B", 1), LetterStart("C", 2))),
            )
        }
    }

    private fun savedNow() = runBlocking { saved.all.first() }

    /** The filters of each of the grid's reads, by field. */
    private fun askedRules() = asked.map { request -> request.query.where?.rules.orEmpty().associate { it.field to it.propertyValues.orEmpty() } }

    @Test
    fun opensOnTitlesInTitleOrderWithCollectionsOnceTheLibrarySaysItHasThem() {
        val model = LibraryViewModel(signIn(), saved, emptyMap(), "lib1")
        val state = awaitValue(model.state) { it.library != null && it.filters != null }
        assertEquals(GridSort.Title, state.query.sort)
        assertEquals(listOf(LibraryTab.Titles, LibraryTab.Collections), state.tabs)
        assertEquals(listOf(Resolution.P1080), state.filters?.resolutions)
        assertEquals(CardShape.Poster, state.shape)
        val pages = awaitValue(state.grid.pager.state) { it.total != null }
        assertEquals(3, pages.total)
        assertEquals("B", pages.letterAt(1))
        assertTrue(asked.all { it.library == "lib1" && it.query.kinds == listOf(WorkKind.Movie) }, "read as the movies library's grid")
    }

    @Test
    fun aSortChosenIsKeptForTheLibrary() {
        val model = LibraryViewModel(signIn(), saved, emptyMap(), "lib1")
        val first = model.state.value.grid
        model.chooseQuery(model.state.value.query.copy(sort = GridSort.AddedAt))
        val state = awaitValue(model.state) { it.query.sort == GridSort.AddedAt }
        assertTrue(state.grid !== first, "a new grid for the new sort")
        val kept = awaitValue(saved.all) { it["lib1"] != null }
        assertEquals(GridSort.AddedAt, kept["lib1"]!!.query.sort)
        awaitValue(state.grid.pager.state) { it.total != null }
        assertTrue(asked.any { it.query.sort?.single()?.field == "addedAt" })
    }

    @Test
    fun aOneOffSortShowsNewestFirstAndKeepsNothing() {
        val model = LibraryViewModel(signIn(), saved, emptyMap(), "lib1", oneOffSort = GridSort.AddedAt)
        assertEquals(GridSort.AddedAt, model.state.value.query.sort)
        model.chooseTab(LibraryTab.Collections)
        awaitValue(model.state) { it.tab == LibraryTab.Collections }
        assertNull(savedNow()["lib1"])
    }

    @Test
    fun collectionsAreKeptAsTheLibrarysSection() {
        val model = LibraryViewModel(signIn(), saved, emptyMap(), "lib1")
        model.chooseTab(LibraryTab.Collections)
        assertEquals(Section.Collections, awaitValue(saved.all) { it["lib1"] != null }["lib1"]!!.section)
    }

    @Test
    fun anAudiobooksLibraryOffersAuthorsAndSeriesInSquares() {
        assertEquals(listOf(LibraryTab.Titles, LibraryTab.Authors, LibraryTab.Series), tabsOf(LibraryKind.Audiobooks, null, LibraryTab.Titles))
        assertEquals(CardShape.Square, gridShape(LibraryKind.Audiobooks, LibraryTab.Titles))
        assertEquals(CardShape.Landscape, gridShape(LibraryKind.Videos, LibraryTab.Titles))
        assertEquals(CardShape.Poster, gridShape(LibraryKind.Videos, LibraryTab.Collections))
        assertEquals(listOf(LibraryTab.Titles), tabsOf(LibraryKind.Movies, null, LibraryTab.Titles))
    }

    @Test
    fun theRailListsTheLettersTheGridHasInRailOrder() {
        assertEquals(listOf("#", "A", "Z"), railLetters(listOf("Z", "A", "#")))
    }

    @Test
    fun gridColumnsFitTheWidth() {
        assertEquals(3, gridColumns(androidx.compose.ui.unit.Dp(340f), CardShape.Poster))
        assertEquals(2, gridColumns(androidx.compose.ui.unit.Dp(340f), CardShape.Landscape))
        assertEquals(10, gridColumns(androidx.compose.ui.unit.Dp(1130f), CardShape.Poster))
        assertEquals(6, gridColumns(androidx.compose.ui.unit.Dp(1130f), CardShape.Poster, wide = true))
        assertEquals(4, gridColumns(androidx.compose.ui.unit.Dp(1130f), CardShape.Landscape, wide = true))
    }

    @Test
    fun aVideoLibraryOffersDecadesGenresRatingsAndQualityAndNoNarrators() {
        val model = LibraryViewModel(signIn(), saved, emptyMap(), "lib1")
        val offer = awaitValue(model.state) { it.library != null && it.filters != null }.offer
        assertEquals(
            FilterOffer(
                genres = listOf("Action", "Comedy", "Drama"),
                decades = listOf(2020, 2010, 1990),
                contentRatings = listOf("PG", "PG-13", "R"),
                resolutions = listOf(Resolution.P1080),
                dynamicRanges = listOf(DynamicRange.Hdr),
            ),
            offer,
        )
    }

    @Test
    fun aBooksLibraryOffersDecadesGenresAndNarratorsAndNoRatingsOrQuality() {
        val model = LibraryViewModel(signIn(), saved, emptyMap(), "lib2")
        val offer = awaitValue(model.state) { it.library != null && it.filters != null }.offer
        assertEquals(
            FilterOffer(
                genres = listOf("Action", "Comedy", "Drama"),
                decades = listOf(2020, 2010, 1990),
                narrators = listOf(PersonRef("p1", "Ray Porter"), PersonRef("p2", "Kate Reading")),
            ),
            offer,
        )
        assertEquals(FilterOffer(), filterOffer(LibraryKind.Movies, null), "nothing until the filters are read")
    }

    @Test
    fun aDecadeIsOneChoiceAndTappingItAgainClearsIt() {
        val query = GridQuery("lib1", sort = GridSort.Title)
        val nineties = query.toggleDecade(1990)
        assertEquals(1990 to 1999, nineties.yearFrom to nineties.yearTo)
        val twenties = nineties.toggleDecade(2020)
        assertEquals(2020 to 2029, twenties.yearFrom to twenties.yearTo)
        assertEquals(query, twenties.toggleDecade(2020))
    }

    @Test
    fun aNarratorIsOneChoiceAndTappingItAgainClearsIt() {
        val query = GridQuery("lib2", sort = GridSort.Title)
        assertEquals("p2", query.toggleNarrator("p1").toggleNarrator("p2").narratorId)
        assertEquals(query, query.toggleNarrator("p1").toggleNarrator("p1"))
    }

    @Test
    fun theChipsComeInTheSheetsOrderEachTurningItsFilterOff() {
        val offer = FilterOffer(
            genres = listOf("Action", "Comedy", "Drama"),
            contentRatings = listOf("PG", "R"),
            resolutions = listOf(Resolution.FourK, Resolution.P1080),
            dynamicRanges = listOf(DynamicRange.Hdr, DynamicRange.DolbyVision),
            narrators = listOf(PersonRef("p1", "Ray Porter")),
        )
        val query = GridQuery(
            "lib1",
            sort = GridSort.Rating,
            played = WorkPlayState.Unplayed,
            genres = listOf("Drama", "Gone", "Action"),
            yearFrom = 1990,
            yearTo = 1999,
            contentRatings = listOf("R", "PG"),
            resolutions = listOf(Resolution.P1080, Resolution.FourK),
            dynamicRanges = listOf(DynamicRange.DolbyVision),
            narratorId = "p1",
        )
        val chips = activeFilters(query, offer)
        assertEquals(
            listOf<Any>(
                WorkPlayState.Unplayed, "Action", "Drama", "Gone", "PG", "R", 1990, Resolution.FourK, Resolution.P1080, DynamicRange.DolbyVision, "Ray Porter",
            ),
            chips.map {
                when (it) {
                    is ActiveFilter.Played -> it.state
                    is ActiveFilter.Genre -> it.name
                    is ActiveFilter.ContentRating -> it.name
                    is ActiveFilter.Decade -> it.decade
                    is ActiveFilter.Quality -> it.resolution
                    is ActiveFilter.Range -> it.range
                    is ActiveFilter.Narrator -> it.name.orEmpty()
                }
            },
        )
        assertEquals(query.copy(genres = listOf("Gone", "Action")), chips[2].without)
        assertEquals(query.copy(yearFrom = null, yearTo = null), chips[6].without)
        assertEquals(query.copy(narratorId = null), chips.last().without)
        assertNull((activeFilters(query, FilterOffer()).last() as ActiveFilter.Narrator).name, "named once the narrators are read")
        assertEquals(emptyList(), activeFilters(query.unfiltered(), offer))
    }

    @Test
    fun aLongGroupShowsItsFirstFewAndAnyChosenUntilOpenedWhole() {
        val genres = (1..13).map { "g$it" }
        assertEquals(genres.take(9) + "g12", shortened(genres, whole = false) { it == "g12" })
        assertEquals(genres, shortened(genres, whole = true) { false })
        assertEquals(genres.take(12), shortened(genres.take(12), whole = false) { false }, "twelve is short enough to show whole")
    }

    @Test
    fun theSheetsChoicesReachTheGridOnceTheyHoldStillAndAreKept() {
        val model = LibraryViewModel(signIn(), saved, emptyMap(), "lib2")
        awaitValue(model.state.value.grid.pager.state) { it.total != null }
        val start = model.state.value.query
        val unplayed = start.copy(played = WorkPlayState.Unplayed)
        val chosen = unplayed.toggleDecade(1990).toggleGenre("Drama").toggleGenre("Action").toggleNarrator("p1")
        model.settle(unplayed)
        model.settle(unplayed.toggleDecade(1990))
        model.settle(chosen)
        assertEquals(start, model.state.value.query, "not before they hold still")
        val state = awaitValue(model.state) { it.query == chosen }
        awaitValue(state.grid.pager.state) { it.total != null }
        val rules = askedRules().last()
        assertEquals(listOf("unplayed"), rules["playState"])
        assertEquals(listOf("Drama", "Action"), rules["genre"])
        assertEquals(listOf("1990", "1999"), rules["year"])
        assertEquals(listOf("p1"), rules["narrator"])
        assertTrue(askedRules().none { it.keys == setOf("playState") || it.keys == setOf("playState", "year") }, "the choices on the way read nothing")
        assertEquals(chosen, awaitValue(saved.all) { it["lib2"]?.query == chosen }["lib2"]!!.query)
        val reopened = LibraryViewModel(signIn(), saved, savedNow(), "lib2")
        assertEquals(chosen, reopened.state.value.query)
        reopened.clearFilters()
        assertEquals(GridQuery("lib2", sort = GridSort.Title), awaitValue(reopened.state) { !it.query.filtered }.query)
    }

    @Test
    fun aChoiceMadeAtOnceOverridesOneOnItsWay() {
        val model = LibraryViewModel(signIn(), saved, emptyMap(), "lib1")
        val start = model.state.value.query
        model.settle(start.toggleGenre("Drama"))
        model.chooseQuery(start.copy(sort = GridSort.AddedAt))
        Thread.sleep(600)
        assertEquals(start.copy(sort = GridSort.AddedAt), savedNow()["lib1"]?.query, "the genre on its way never reached the grid")
    }

    @Test
    fun aGridKeptByTheBuildBeforeNarratorsOpensAsItWasLeft() {
        runBlocking {
            store.edit {
                it[stringPreferencesKey("grid.lib1")] = "sort=addedAt&played=unplayed&decade=1990&genre=Action+%26+Adventure,Drama" +
                    "&contentRating=PG-13&resolution=4k&dynamicRange=hdr"
            }
        }
        val model = LibraryViewModel(signIn(), saved, savedNow(), "lib1")
        assertEquals(
            GridQuery(
                "lib1",
                sort = GridSort.AddedAt,
                played = WorkPlayState.Unplayed,
                genres = listOf("Action & Adventure", "Drama"),
                yearFrom = 1990,
                yearTo = 1999,
                contentRatings = listOf("PG-13"),
                resolutions = listOf(Resolution.FourK),
                dynamicRanges = listOf(DynamicRange.Hdr),
            ),
            model.state.value.query,
        )
    }
}
