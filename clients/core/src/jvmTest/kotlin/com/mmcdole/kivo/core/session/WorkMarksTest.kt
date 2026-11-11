package com.mmcdole.kivo.core.session

import com.mmcdole.kivo.api.models.UserState
import com.mmcdole.kivo.core.Done
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.display.Display
import com.mmcdole.kivo.core.display.problem
import com.mmcdole.kivo.core.downloads.DownloadStart
import com.mmcdole.kivo.core.downloads.downloadProblem
import com.mmcdole.kivo.core.empty
import com.mmcdole.kivo.core.json
import com.mmcdole.kivo.core.problem
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.take
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.async
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.withContext
import java.io.IOException
import java.util.concurrent.CopyOnWriteArrayList
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertIs
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

// Changes answer with Done rather than throwing, and the marks a card makes
// tell each failure once, against a server answered by Ktor's mock engine.

class WorkMarksTest {
    private val played = UserState(played = true, playCount = 1)

    private suspend fun TestScope.signedIn(): Pair<Harness, KivoSession> {
        val h = Harness(this)
        h.servers.server("home.test", info("server-1", "Basement"))
        h.device.signedInTo("server-1")
        return h to h.core.restore(home)
    }

    @Test
    fun aChangeTheServerMakesIsOkWithWhatItGivesBack() = runTest {
        val (h, session) = signedIn()
        h.servers.on("home.test", "PATCH", "/api/v1/works/m1/user-state") { json(UserState.serializer(), played) }

        val done = session.library.setPlayed("m1", true)

        assertEquals(Done.Ok(played), done)
        assertEquals(played, done.getOrNull())
        assertNull(done.problemOrNull())
    }

    @Test
    fun aRefusedChangeFailsWithTheServersProblem() = runTest {
        val (h, session) = signedIn()
        h.servers.on("home.test", "POST", "/api/v1/lists") { problem(HttpStatusCode.Conflict, "nameTaken") }

        val done = session.library.createList("Favourites")

        val problem = assertIs<RequestProblem.Refused>(assertIs<Done.Failed>(done).problem)
        assertEquals("nameTaken", problem.code)
        assertNull(done.getOrNull())
    }

    @Test
    fun aChangeThatGetsNoAnswerFailsAsUnreachable() = runTest {
        val (h, session) = signedIn()
        h.servers.on("home.test", "POST", "/api/v1/device-codes/K7Q2M9/approve") { throw IOException("no route to host") }

        assertEquals(Done.Failed(RequestProblem.Unreachable), session.link.approve("K7Q2M9"))
    }

    @Test
    fun aChangeWithNothingToGiveBackIsOkWithUnit() = runTest {
        val (h, session) = signedIn()
        h.servers.on("home.test", "POST", "/api/v1/works/e1/user-state/remove-from-continue") { empty() }

        assertEquals(Done.Ok(Unit), session.library.removeFromContinue("e1"))
    }

    @Test
    fun aFailedMarkIsToldOnceAndOneThatWorksIsntToldAtAll() = runTest {
        val (h, session) = signedIn()
        h.servers.on("home.test", "PATCH", "/api/v1/works/m1/user-state") { problem(HttpStatusCode.NotFound, "notFound") }
        h.servers.on("home.test", "PATCH", "/api/v1/works/m2/user-state") { json(UserState.serializer(), played) }
        h.servers.on("home.test", "POST", "/api/v1/lists/my/works") { throw IOException("no route to host") }
        val told = CopyOnWriteArrayList<RequestProblem>()
        val listening = launch(start = CoroutineStart.UNDISPATCHED) { session.library.marks.problems.collect { told += it } }

        session.library.marks.setPlayed("m1", true)
        session.library.marks.setPlayed("m2", true)
        session.library.marks.setOnList("my", "m2", on = true)
        withContext(Dispatchers.Default) {
            while (told.size < 2) delay(10)
            // Long enough for a third, were one coming.
            delay(200)
        }
        listening.cancel()

        assertEquals(2, told.size)
        assertTrue(told.any { it is RequestProblem.Refused && it.code == "notFound" })
        assertTrue(RequestProblem.Unreachable in told)
        assertTrue("PATCH /api/v1/works/m2/user-state" in h.servers.seen.map { it.substringAfter(' ') })
    }

    @Test
    fun aFailedMarkWithNoOneListeningIsntToldLater() = runTest {
        val (h, session) = signedIn()
        val answered = CompletableDeferred<Unit>()
        h.servers.on("home.test", "POST", "/api/v1/works/e1/user-state/remove-from-continue") {
            answered.complete(Unit)
            problem(HttpStatusCode.NotFound, "notFound")
        }
        session.library.marks.removeFromContinue("e1")
        answered.await()
        withContext(Dispatchers.Default) { delay(100) }

        h.servers.on("home.test", "PATCH", "/api/v1/works/m1/user-state") { throw IOException("no route to host") }
        val next = async(start = CoroutineStart.UNDISPATCHED) { session.library.marks.problems.take(1).toList() }
        session.library.marks.setPlayed("m1", false)

        assertEquals(listOf<RequestProblem>(RequestProblem.Unreachable), next.await())
    }

    @Test
    fun aDownloadThatCantStartSaysWhy() {
        val display = Display()
        assertNull(display.downloadProblem(DownloadStart.Started("d1")))
        assertNotNull(display.downloadProblem(DownloadStart.NoRoom(1_000_000_000)))
        assertNotNull(display.downloadProblem(DownloadStart.Refused("notPlayable")))
        assertEquals(display.problem(RequestProblem.Unreachable), display.downloadProblem(DownloadStart.Failed(RequestProblem.Unreachable)))
    }

    @Test
    fun theFirstProblemsCollectorGetsIt() = runTest {
        val (h, session) = signedIn()
        h.servers.on("home.test", "DELETE", "/api/v1/lists/my/works/m1") { problem(HttpStatusCode.Forbidden, "forbidden") }
        val first = async(start = CoroutineStart.UNDISPATCHED) { session.library.marks.problems.first() }

        session.library.marks.setOnList("my", "m1", on = false)

        assertEquals("forbidden", first.await().code)
    }
}
