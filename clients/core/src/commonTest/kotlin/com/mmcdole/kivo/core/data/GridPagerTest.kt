package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.infrastructure.ApiClient
import com.mmcdole.kivo.api.models.LetterStart
import com.mmcdole.kivo.api.models.LibraryKind
import com.mmcdole.kivo.api.models.MatchState
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkQueryRequest
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.api.models.WorkSummaryPage
import com.mmcdole.kivo.core.FakeServer
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.Seen
import com.mmcdole.kivo.core.json
import com.mmcdole.kivo.core.live.Invalidation
import com.mmcdole.kivo.core.problem
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlin.coroutines.EmptyCoroutineContext
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertIs
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertSame
import kotlin.test.assertTrue
import kotlin.time.Duration.Companion.seconds

class GridPagerTest {
    private val total = 2_000

    private fun summary(i: Int, prefix: String = "T") = WorkSummary("w$i", WorkKind.Movie, "$prefix$i", MatchState.Matched, true, WorkImages())

    /** Letters every 100 works: A at 0, B at 100, … */
    private val letters = (0 until 20).map { LetterStart(('A' + it).toString(), it * 100) }

    /**
     * The library over the fake server: works by position, from `offset`,
     * the total and letters on the first page alone; [answer] may answer
     * otherwise, or hold, by the requested start; null answers as the
     * library does.
     */
    private class Library(val scope: TestScope, val total: Int, val letters: List<LetterStart>, val summary: (Int) -> WorkSummary) {
        val server = FakeServer(StandardTestDispatcher(scope.testScheduler))
        val invalidations = MutableSharedFlow<Invalidation>()
        val known = Known()
        val connection = server.connection()
        var prefix = "T"
        var answer: (suspend io.ktor.client.engine.mock.MockRequestHandleScope.(start: Int) -> io.ktor.client.request.HttpResponseData?) = { null }

        init {
            server.on("POST", QUERY) {
                val asked = asked(server.seen.last())
                val start = asked.offset ?: 0
                answer(start) ?: run {
                    val end = minOf(start + (asked.limit ?: 60), total)
                    json(
                        WorkSummaryPage.serializer(),
                        WorkSummaryPage(
                            (start until end).map { summary(it).copy(title = "$prefix$it") },
                            total = total.takeIf { start == 0 },
                            letters = letters.takeIf { start == 0 },
                        ),
                    )
                }
            }
        }

        fun paged(kept: Int = GridPager.KEPT_PAGES) = GridPager(
            read = { offset, limit -> GridRepository(connection, invalidations).page(GridQuery("l1"), LibraryKind.Movies, offset, limit).let { GridPage(it.items, it.total, it.letters) } },
            changes = GridRepository(connection, invalidations).changes("l1"),
            reachability = connection.reachability,
            known = known,
            key = "works l1",
            seen = known::read,
            scope = scope.backgroundScope,
            work = EmptyCoroutineContext,
            kept = kept,
        )

        /** The offsets of the pages asked for, in order. */
        fun asked() = server.to("POST", QUERY).map { asked(it).offset ?: 0 }

        private fun asked(seen: Seen) = ApiClient.JSON_DEFAULT.decodeFromString(WorkQueryRequest.serializer(), seen.body)

        private companion object {
            const val QUERY = "/api/v1/works/query"
        }
    }

    private fun TestScope.library() = Library(this, total, letters, ::summary)

    @Test
    fun theFirstPageBringsTheCountLettersAndItsWorks() = runTest {
        val library = library()
        val paged = library.paged()
        assertNull(paged.state.value.total)
        assertEquals(Load.Loading, paged.state.value.pages[0])
        runCurrent()
        val state = paged.state.value
        assertEquals(total, state.total)
        assertEquals(letters, state.letters)
        assertEquals("T59", state.item(59)?.title)
        assertNull(state.item(60), "a page isn't read until it's asked for")
        assertEquals("C", state.letterAt(250))
        assertEquals(1_900, state.letterOffset("T"), "a letter without works goes to the next that has some")
        assertEquals(total - 1, state.letterOffset("Z"), "or the last work")
        assertNull(state.problem)
    }

    @Test
    fun aGridReadLatelyShowsAtOnceAndIsReadAgain() = runTest {
        val library = library()
        library.paged().also { runCurrent() }.close()
        library.prefix = "U"
        val again = library.paged()
        assertEquals(total, again.state.value.total, "the count before any answer")
        assertEquals("T0", again.state.value.item(0)?.title)
        runCurrent()
        assertEquals("U0", again.state.value.item(0)?.title, "read again")
        assertEquals(2, library.asked().size)
    }

    @Test
    fun pagesAreReadOnceEachAtTheirOffset() = runTest {
        val library = library()
        val paged = library.paged()
        runCurrent()
        // Works 1,200 to 1,319: pages 20 and 21.
        paged.load(1_200, 1_319)
        paged.load(1_210, 1_300)
        runCurrent()
        assertEquals(listOf(0, 1_200, 1_260), library.asked(), "each page once")
        paged.load(1_300, 1_379)
        runCurrent()
        assertEquals(1_320, library.asked().last())
        assertEquals("T1379", paged.state.value.item(1_379)?.title)
        paged.load(1_300, 1_379)
        runCurrent()
        assertEquals(4, library.asked().size, "pages read are kept")
    }

    @Test
    fun askingAgainForPagesItHasPublishesNothing() = runTest {
        val library = library()
        val paged = library.paged()
        runCurrent()
        paged.load(0, 59)
        val shown = paged.state.value
        paged.load(10, 50)
        runCurrent()
        assertSame(shown, paged.state.value)
    }

    @Test
    fun aPageNoLongerAskedForStopsAndItsAnswerIsDropped() = runTest {
        val library = library()
        val paged = library.paged()
        runCurrent()
        val hold = CompletableDeferred<Unit>()
        library.answer = { start -> if (start == 600) hold.await().let { null } else null }
        paged.load(600, 659)
        runCurrent()
        assertEquals(Load.Loading, paged.state.value.pages[10])
        paged.load(1_200, 1_259)
        runCurrent()
        assertNull(paged.state.value.pages[10], "page 10 is no longer read")
        hold.complete(Unit)
        runCurrent()
        assertNull(paged.state.value.pages[10], "nor its answer kept")
        assertEquals("T1200", paged.state.value.item(1_200)?.title)
    }

    @Test
    fun aChangeReadsThePagesAskedForAgainInPlaceDroppingAnAnswerForTheGridBefore() = runTest {
        val library = library()
        val paged = library.paged()
        runCurrent()
        paged.load(0, 119)
        runCurrent()
        paged.load(600, 659)
        val hold = CompletableDeferred<Unit>()
        var held = false
        library.answer = { start ->
            if (start == 600 && !held) {
                held = true
                hold.await()
            }
            null
        }
        runCurrent()
        library.prefix = "U"
        library.invalidations.emit(Invalidation.LibraryChanged(listOf("l1")))
        runCurrent()
        val state = paged.state.value
        assertEquals("U0", state.item(0)?.title, "the first page is read again")
        assertNull(state.pages[1], "a page no longer asked for goes")
        assertEquals("U600", state.item(600)?.title)
        // The answer held back was for the grid before; it's dropped.
        library.prefix = "T"
        hold.complete(Unit)
        runCurrent()
        assertEquals("U600", paged.state.value.item(600)?.title)
    }

    @Test
    fun aPageShowsWhatItHadWhileItsReadAgain() = runTest {
        val library = library()
        val paged = library.paged()
        runCurrent()
        val hold = CompletableDeferred<Unit>()
        library.answer = { if (!hold.isCompleted) hold.await().let { null } else null }
        library.invalidations.emit(Invalidation.Everything)
        runCurrent()
        assertEquals("T0", paged.state.value.item(0)?.title, "no placeholders while it's read")
        hold.complete(Unit)
    }

    @Test
    fun aPageWithoutAnAnswerIsAskedAgainFiveTimesThenWhenNextAskedFor() = runTest {
        val library = library()
        val paged = library.paged()
        runCurrent()
        library.answer = { start -> if (start == 600) respond("", HttpStatusCode.BadGateway) else null }
        paged.load(600, 659)
        runCurrent()
        assertEquals(Load.Failed(RequestProblem.Unreachable), paged.state.value.pages[10])
        for (wait in listOf(1, 2, 4, 8, 16)) {
            val before = library.asked().size
            advanceTimeBy(wait.seconds - 1.seconds / 1000)
            runCurrent()
            assertEquals(before, library.asked().size, "waits $wait s")
            advanceTimeBy(1.seconds / 1000)
            runCurrent()
            assertEquals(before + 1, library.asked().size)
        }
        advanceTimeBy(60.seconds)
        assertEquals(7, library.asked().size, "the first page, then page 10 six times")
        assertIs<Load.Failed>(paged.state.value.pages[10])

        library.answer = { null }
        paged.load(600, 659)
        runCurrent()
        assertEquals("T600", paged.state.value.item(600)?.title, "asked for again, it's read again")
    }

    @Test
    fun aRefusalIsFinal() = runTest {
        val library = library()
        library.answer = { problem(HttpStatusCode.Forbidden, "forbidden") }
        val paged = library.paged()
        runCurrent()
        advanceTimeBy(60.seconds)
        assertEquals(1, library.asked().size)
        val problem = assertNotNull(paged.state.value.problem)
        assertEquals("forbidden", problem.code)
    }

    @Test
    fun aFirstPageThatFailsIsNoProblemWhileOneReadLatelyShows() = runTest {
        val library = library()
        library.paged().also { runCurrent() }.close()
        library.answer = { problem(HttpStatusCode.Forbidden, "forbidden") }
        val again = library.paged()
        runCurrent()
        assertNull(again.state.value.problem)
        assertEquals("T0", again.state.value.item(0)?.title)
    }

    @Test
    fun refreshReturnsOnceTheFirstPageAnswers() = runTest {
        val library = library()
        val paged = library.paged()
        runCurrent()
        library.prefix = "U"
        paged.refresh()
        assertEquals("U0", paged.state.value.item(0)?.title)
    }

    @Test
    fun pagesFarFromThoseAskedForAreLetGo() = runTest {
        val library = library()
        val paged = library.paged(kept = 4)
        runCurrent()
        for (page in 1..5) {
            paged.load(page * 60, page * 60 + 59)
            runCurrent()
        }
        assertEquals(setOf(0, 3, 4, 5), paged.state.value.pages.keys, "the first and the nearest")
        paged.load(60, 119)
        runCurrent()
        assertEquals("T60", paged.state.value.item(60)?.title, "read again once asked for")
    }

    @Test
    fun aClosedGridReadsNothingMore() = runTest {
        val library = library()
        val paged = library.paged()
        runCurrent()
        paged.close()
        library.invalidations.emit(Invalidation.Everything)
        runCurrent()
        assertEquals(1, library.asked().size)
        assertTrue(paged.state.value.total == total)
    }
}
