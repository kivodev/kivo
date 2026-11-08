package com.mmcdole.kivo.android.mobile.library

import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import com.mmcdole.kivo.android.library.GridValues
import com.mmcdole.kivo.android.library.SavedGrids
import com.mmcdole.kivo.android.library.Section
import com.mmcdole.kivo.android.mobile.CoreTest
import com.mmcdole.kivo.android.mobile.awaitValue
import com.mmcdole.kivo.android.mobile.ui.CardShape
import com.mmcdole.kivo.android.mobile.work
import com.mmcdole.kivo.api.models.LetterStart
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.LibraryFacets
import com.mmcdole.kivo.api.models.LibraryList
import com.mmcdole.kivo.api.models.WorkSummaryPage
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlin.io.path.createTempDirectory
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

class LibraryViewModelTest : CoreTest() {
    private val movies = Library("lib1", "Movies", pinned = true, kind = "movies", workCount = 3)
    private val books = Library("lib2", "Books", pinned = true, kind = "audiobooks", workCount = 0)
    private val saved = SavedGrids(PreferenceDataStoreFactory.create { createTempDirectory("grids").resolve("grids.preferences_pb").toFile() })

    init {
        server.get("/api/v1/libraries", LibraryList.serializer()) { LibraryList(listOf(movies, books)) }
        server.get("/api/v1/libraries/lib1/facets", LibraryFacets.serializer()) {
            LibraryFacets(emptyList(), emptyList(), emptyList(), emptyList(), listOf("1080p"), emptyList(), collections = true)
        }
        server.get("/api/v1/libraries/lib1/works", WorkSummaryPage.serializer()) {
            WorkSummaryPage(listOf(work("a", "Algiers"), work("b", "Battleship"), work("c", "Charade")), 3, listOf(LetterStart("A", 0), LetterStart("B", 1), LetterStart("C", 2)))
        }
    }

    private fun savedNow() = runBlocking { saved.all.first() }

    @Test
    fun opensOnTitlesInTitleOrderWithCollectionsOnceTheLibrarySaysItHasThem() {
        val model = LibraryViewModel(signIn(), saved, emptyMap(), "lib1")
        val state = awaitValue(model.state) { it.library != null && it.facets != null }
        assertEquals(GridValues.TITLE, state.query.sort)
        assertEquals(listOf(LibraryTab.Titles, LibraryTab.Collections), state.tabs)
        assertEquals(CardShape.Poster, state.shape)
        val pages = awaitValue(state.grid.pager.state) { it.total != null }
        assertEquals(3, pages.total)
        assertEquals("B", pages.letterAt(1))
    }

    @Test
    fun aSortChosenIsKeptForTheLibrary() {
        val model = LibraryViewModel(signIn(), saved, emptyMap(), "lib1")
        val first = model.state.value.grid
        model.chooseQuery(model.state.value.query.copy(sort = GridValues.ADDED_AT))
        val state = awaitValue(model.state) { it.query.sort == GridValues.ADDED_AT }
        assertTrue(state.grid !== first, "a new grid for the new sort")
        val kept = awaitValue(saved.all) { it["lib1"] != null }
        assertEquals(GridValues.ADDED_AT, kept["lib1"]!!.query.sort)
        assertTrue(server.seen.any { it.startsWith("GET /api/v1/libraries/lib1/works") && "sort=addedAt" in it })
    }

    @Test
    fun aOneOffSortShowsNewestFirstAndKeepsNothing() {
        val model = LibraryViewModel(signIn(), saved, emptyMap(), "lib1", oneOffSort = GridValues.ADDED_AT)
        assertEquals(GridValues.ADDED_AT, model.state.value.query.sort)
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
        assertEquals(listOf(LibraryTab.Titles, LibraryTab.Authors, LibraryTab.Series), tabsOf("audiobooks", null, LibraryTab.Titles))
        assertEquals(CardShape.Square, gridShape("audiobooks", LibraryTab.Titles))
        assertEquals(CardShape.Landscape, gridShape("videos", LibraryTab.Titles))
        assertEquals(CardShape.Poster, gridShape("videos", LibraryTab.Collections))
        assertEquals(listOf(LibraryTab.Titles), tabsOf("movies", null, LibraryTab.Titles))
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
