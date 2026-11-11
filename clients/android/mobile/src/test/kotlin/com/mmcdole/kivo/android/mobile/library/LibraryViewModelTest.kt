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
import com.mmcdole.kivo.core.data.GridSort
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
    private val saved = SavedGrids(PreferenceDataStoreFactory.create { createTempDirectory("grids").resolve("grids.preferences_pb").toFile() })

    init {
        server.get("/api/v1/libraries", LibraryList.serializer()) { LibraryList(listOf(movies, books)) }
        for (field in listOf("genre", "year", "contentRating", "resolution", "dynamicRange", "narrator")) {
            server.get("/api/v1/work-fields/$field/values", WorkFieldValueList.serializer()) {
                WorkFieldValueList(if (field == "resolution") listOf(WorkFieldValue("1080p", "1080p")) else emptyList())
            }
        }
        server.get("/api/v1/libraries/lib1/collections", CollectionSummaryPage.serializer()) {
            CollectionSummaryPage(listOf(CollectionSummary("c1", "Dune Collection", 2, WorkImages())), 1)
        }
        server.on("POST", "/api/v1/works/query") { request ->
            asked += ApiClient.JSON_DEFAULT.decodeFromString(WorkQueryRequest.serializer(), (request.body as OutgoingContent.ByteArrayContent).bytes().decodeToString())
            reply(
                WorkSummaryPage.serializer(),
                WorkSummaryPage(listOf(work("a", "Algiers"), work("b", "Battleship"), work("c", "Charade")), 3, listOf(LetterStart("A", 0), LetterStart("B", 1), LetterStart("C", 2))),
            )
        }
    }

    private fun savedNow() = runBlocking { saved.all.first() }

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
}
