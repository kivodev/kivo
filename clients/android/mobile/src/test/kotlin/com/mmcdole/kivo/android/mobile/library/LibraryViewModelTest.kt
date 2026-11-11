package com.mmcdole.kivo.android.mobile.library

import com.mmcdole.kivo.android.mobile.CoreTest
import com.mmcdole.kivo.android.mobile.awaitValue
import com.mmcdole.kivo.android.mobile.reply
import com.mmcdole.kivo.android.mobile.ui.CardShape
import com.mmcdole.kivo.android.mobile.work
import com.mmcdole.kivo.api.infrastructure.ApiClient
import com.mmcdole.kivo.api.models.CollectionSummary
import com.mmcdole.kivo.api.models.CollectionSummaryPage
import com.mmcdole.kivo.api.models.LetterStart
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.LibraryKind
import com.mmcdole.kivo.api.models.LibraryList
import com.mmcdole.kivo.api.models.WorkFieldValue
import com.mmcdole.kivo.api.models.WorkFieldValueList
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkQueryRequest
import com.mmcdole.kivo.api.models.WorkSummaryPage
import com.mmcdole.kivo.core.data.FilterGroupKind
import com.mmcdole.kivo.core.data.GridFilter
import com.mmcdole.kivo.core.data.GridSort
import com.mmcdole.kivo.core.data.LibraryTab
import com.mmcdole.kivo.core.session.GridCardShape
import com.mmcdole.kivo.core.session.OfferRead
import io.ktor.http.content.OutgoingContent
import java.util.concurrent.CopyOnWriteArrayList
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue
import kotlin.time.Duration.Companion.milliseconds
import kotlin.time.TimeSource

class LibraryViewModelTest : CoreTest() {
    private val movies = Library("lib1", "Movies", pinned = true, kind = LibraryKind.Movies, workCount = 3)
    private val books = Library("lib2", "Books", pinned = true, kind = LibraryKind.Audiobooks, workCount = 0)
    /** The grid's reads, as the server was asked. */
    private val asked: MutableList<WorkQueryRequest> = CopyOnWriteArrayList()

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

    @Test
    fun opensOnTitlesInTitleOrderWithCollectionsOnceTheLibrarySaysItHasThem() {
        val model = held { LibraryViewModel(signIn(), "lib1") }
        val state = awaitValue(model.state) { it.library != null && it.offerRead == OfferRead.Read }
        assertEquals(GridSort.Title, state.sort)
        assertEquals(listOf(LibraryTab.Titles, LibraryTab.Collections), state.tabs)
        assertEquals(
            listOf(FilterGroupKind.WatchStatus, FilterGroupKind.Decade, FilterGroupKind.Genre, FilterGroupKind.ContentRating, FilterGroupKind.Quality),
            state.offer.map { it.kind },
        )
        assertEquals(CardShape.Poster, state.cardShape.card)
        val pages = awaitValue(state.pager.state) { it.total != null }
        assertEquals(3, pages.total)
        assertEquals("B", pages.letterAt(1))
        assertTrue(asked.all { it.library == "lib1" && it.query.kinds == listOf(WorkKind.Movie) }, "read as the movies library's grid")
    }

    @Test
    fun theGridReadsThePagesAskedForOnceItHasHeldStill() {
        server.on("POST", "/api/v1/works/query") { request ->
            val read = ApiClient.JSON_DEFAULT.decodeFromString(WorkQueryRequest.serializer(), (request.body as OutgoingContent.ByteArrayContent).bytes().decodeToString())
            asked += read
            val start = read.offset ?: 0
            reply(WorkSummaryPage.serializer(), WorkSummaryPage((start until minOf(start + 60, 200)).map { work("w$it") }, 200.takeIf { start == 0 }))
        }
        val grid = held { LibraryViewModel(signIn(), "lib1") }.state.value.pager
        awaitValue(grid.state) { it.total != null }
        val asking = TimeSource.Monotonic.markNow()
        grid.load(60, 119)
        grid.load(120, 179)
        awaitValue(grid.state) { it.item(120) != null }
        assertTrue(asking.elapsedNow() >= GridSettleMs.milliseconds, "not while the grid moves")
        assertEquals(listOf(0, 120), asked.map { it.offset ?: 0 }, "none of the pages it passed")
    }

    @Test
    fun whatsChosenShowsANewGridAndOpensTheLibraryNextTime() {
        val session = signIn()
        val model = LibraryViewModel(session, "lib2")
        val first = model.state.value.pager
        model.grid.choose(GridSort.AddedAt)
        model.grid.choose(model.state.value.filters.toggling(GridFilter.Genre("Drama")), settle = true)
        model.grid.choose(LibraryTab.BookSeries)
        assertTrue(model.state.value.pager !== first, "a new grid for what's chosen")
        awaitValue(model.state) { it.offerRead == OfferRead.Read }

        val reopened = LibraryViewModel(session, "lib2").state.value
        assertEquals(LibraryTab.BookSeries, reopened.tab)
        assertEquals(GridSort.AddedAt, reopened.sort)
        assertEquals(listOf(GridFilter.Genre("Drama")), reopened.filters.chosen)
        assertEquals(CardShape.Square, reopened.cardShape.card)

        val once = LibraryViewModel(session, "lib2", oneOffSort = GridSort.ReleaseDate).state.value
        assertEquals(LibraryTab.Titles, once.tab)
        assertEquals(GridSort.ReleaseDate, once.sort)
        assertTrue(once.filters.isEmpty)
    }

    @Test
    fun theGridsCardsAreThePhonesShapes() {
        assertEquals(listOf(CardShape.Poster, CardShape.Landscape, CardShape.Square), GridCardShape.entries.map { it.card })
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
