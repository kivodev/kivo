package com.mmcdole.kivo.core.session

import com.mmcdole.kivo.api.models.DownloadList
import com.mmcdole.kivo.api.models.ServerInfo
import com.mmcdole.kivo.core.Compatibility
import com.mmcdole.kivo.core.MIN_SERVER_VERSION
import com.mmcdole.kivo.core.auth.SavedServer
import com.mmcdole.kivo.core.downloads.db.DownloadDatabase
import com.mmcdole.kivo.core.json
import com.mmcdole.kivo.core.kivoJson
import com.mmcdole.kivo.core.problem
import com.mmcdole.kivo.core.proofOf
import io.ktor.http.ContentType
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.server.application.install
import io.ktor.server.cio.CIO
import io.ktor.server.engine.embeddedServer
import io.ktor.server.response.respondText
import io.ktor.server.routing.get
import io.ktor.server.routing.routing
import io.ktor.server.websocket.WebSockets
import io.ktor.server.websocket.webSocket
import io.ktor.websocket.Frame
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.isActive
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.currentTime
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.withTimeout
import java.io.IOException
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertIs
import kotlin.test.assertSame
import kotlin.test.assertTrue
import kotlin.time.Duration.Companion.minutes
import kotlin.time.Duration.Companion.seconds
import com.mmcdole.kivo.core.downloads.db.Download as Row

// Where the app is, as its root shows it from KivoClient.state: opening the
// server last signed in to, one that doesn't answer, a session that ends,
// leaving for another server, and what runs while the app is in the
// foreground. Against servers answered by Ktor's mock engine by host, on
// the test's virtual time.

private const val SERVER = "home.test GET /api/v1/server"
private const val DOWNLOADS = "home.test GET /api/v1/downloads"

/** A harness whose device was signed in to [home] before, at `home.test`, which answers as [info]. */
private suspend fun TestScope.signedInBefore(info: ServerInfo = info("server-1", "Basement"), withDownloads: Boolean = false, appVersion: String = "1.0"): Harness {
    val h = Harness(this, withDownloads, appVersion)
    h.servers.server("home.test", info)
    h.device.signedInTo("server-1")
    h.core.restore(home).close()
    return h
}

private fun Harness.unreachable() = servers.on("home.test", "GET", "/api/v1/server") { throw IOException("no route to host") }

private suspend fun KivoClient.started(): AppState = state.first { it != AppState.Starting }

private suspend fun KivoClient.signedIn(): KivoSession = assertIs<AppState.SignedIn>(started()).session

@OptIn(ExperimentalCoroutinesApi::class)
class AppStateTest {
    @Test
    fun withNoServerSignedInToTheAppStartsSignedOut() = runTest {
        val h = Harness(this)
        assertEquals(AppState.Starting, h.core.state.value)

        h.core.start()

        assertEquals(AppState.SignedOut(null), h.core.started())
    }

    @Test
    fun aSavedServerTheDeviceHoldsNoSessionOnIsNotOpened() = runTest {
        val h = Harness(this)
        h.servers.server("home.test", info("server-1", "Basement"))
        h.device.signedInTo("server-1")
        h.core.restore(home).close()
        h.device.secureStore.values.clear()

        h.core.start()

        assertEquals(AppState.SignedOut(null), h.core.started())
    }

    @Test
    fun theServerLastSignedInToOpensSignedInAndItsUserIsRead() = runTest {
        val h = signedInBefore()

        h.core.start()
        val session = h.core.signedIn()

        assertTrue(session.isSignedIn)
        assertEquals("server-1", session.savedServer.value.id)
        assertEquals("anna", session.user.first { it != null }?.username)
        assertTrue("home.test GET /api/v1/users/me" in h.servers.seen)
    }

    @Test
    fun startingTwiceOpensTheServerOnce() = runTest {
        val h = signedInBefore()

        h.core.start()
        val session = h.core.signedIn()
        h.core.start()
        testScheduler.runCurrent()

        assertSame(session, h.core.signedIn())
        assertTrue(session.scope.isActive)
    }

    @Test
    fun aServerThatAnswersNowhereIsUnreachableOnceItHasStayedSoAndRetryOpensIt() = runTest {
        val h = signedInBefore()
        val answering = h.servers.seen.size
        h.unreachable()

        h.core.start()

        assertEquals(AppState.Unreachable(home, hasDownloads = false), h.core.started())
        assertTrue(currentTime >= 3000, "it's tried twice more, a moment apart, before it's unreachable")
        assertTrue(h.servers.seen.drop(answering).count { it == SERVER } >= 3)

        h.core.retry()
        assertEquals(AppState.Starting, h.core.state.value)
        assertEquals(AppState.Unreachable(home, hasDownloads = false), h.core.started(), "it still doesn't answer")

        h.servers.server("home.test", info("server-1", "Basement"))
        h.core.retry()
        assertFalse(h.core.signedIn().unreachable.value)
    }

    @Test
    fun aServerThatAnswersWhileStartingTriesAgainOpensSignedIn() = runTest {
        val h = signedInBefore()
        h.servers.on("home.test", "GET", "/api/v1/server") {
            if (currentTime < 1500) throw IOException("the network is waking")
            json(ServerInfo.serializer(), info("server-1", "Basement").copy(proof = proofOf(it, "server-1", h.device.secureStore.values.values)))
        }

        h.core.start()

        assertFalse(h.core.signedIn().unreachable.value)
    }

    @Test
    fun aSessionTheServerEndedSignsInToItsServerAgain() = runTest {
        val h = signedInBefore()
        h.servers.on("home.test", "GET", "/api/v1/users/me") { problem(HttpStatusCode.Unauthorized, "unauthorized") }
        h.core.start()
        h.core.signedIn()

        assertEquals(AppState.SignedOut(home), h.core.state.first { it is AppState.SignedOut })
    }

    @Test
    fun signingOutGoesBackToSigningInToTheServerAndClosesTheSession() = runTest {
        val h = signedInBefore()
        h.core.start()
        val session = h.core.signedIn()

        session.connection.signOut()

        assertEquals(AppState.SignedOut(home), h.core.state.first { it is AppState.SignedOut })
        testScheduler.runCurrent()
        assertFalse(session.scope.isActive)
    }

    @Test
    fun changingServerLeavesForSignInAndKeepsTheDeviceSignedIn() = runTest {
        val h = signedInBefore()
        h.core.start()
        val session = h.core.signedIn()

        h.core.changeServer()

        assertEquals(AppState.SignedOut(null), h.core.state.value)
        assertFalse(session.scope.isActive)
        assertEquals("server-1", h.core.lastSignedIn()?.id)
    }

    @Test
    fun changingServerFromOneThatCantBeReachedSignsIn() = runTest {
        val h = signedInBefore()
        h.unreachable()
        h.core.start()
        assertIs<AppState.Unreachable>(h.core.started())

        h.core.changeServer()

        assertEquals(AppState.SignedOut(null), h.core.state.value)
    }

    @Test
    fun aSignInShowsItsServerInPlaceOfTheOneOpen() = runTest {
        val h = signedInBefore()
        h.servers.server("b.test", info("b"))
        h.device.signedInTo("b")
        h.core.start()
        val first = h.core.signedIn()

        val other = h.core.restore(SavedServer("b", "b", listOf("http://b.test")))
        h.core.signedIn(other)

        assertEquals(AppState.SignedIn(other), h.core.state.value)
        assertFalse(first.scope.isActive)
        other.connection.signOut()
        assertEquals("b", assertIs<AppState.SignedOut>(h.core.state.first { it is AppState.SignedOut }).server?.id)
    }

    @Test
    fun anUnreachableServerWithDownloadsOnTheDeviceOpensWithoutIt() = runTest {
        val h = signedInBefore(withDownloads = true)
        DownloadDatabase(h.device.downloads!!.driver).downloadsQueries.insertDownload(
            Row("d1", "server-1", "u1", null, "w1", null, null, null, "standard", "ready", null, null, null, "{}", "{}", "Night Shift", "movie", null, null, null, null, 1, 1, 1, null),
        )
        h.unreachable()
        h.core.start()
        assertEquals(AppState.Unreachable(home, hasDownloads = true), h.core.started())

        h.core.openOffline()
        val session = h.core.signedIn()

        assertTrue(session.isSignedIn)
        assertTrue(session.unreachable.value)
        assertTrue(session.state.first { it.unreachable }.unreachable)
    }

    @Test
    fun theForegroundRunsTheLiveEventsAndSyncsTheDownloads() = runTest {
        val h = signedInBefore(withDownloads = true)
        h.servers.on("home.test", "GET", "/api/v1/downloads") { json(DownloadList.serializer(), DownloadList(emptyList())) }
        h.core.start()
        val session = h.core.signedIn()
        testScheduler.advanceTimeBy(10.minutes)
        testScheduler.runCurrent()
        val idle = h.servers.seen.size
        testScheduler.advanceTimeBy(10.minutes)
        assertEquals(idle, h.servers.seen.size, "in the background nothing asks the server")
        val synced = h.servers.seen.count { it == DOWNLOADS }

        h.servers.server("home.test", info("server-1", "Den"))
        h.core.setForeground(true)

        assertEquals("Den", session.savedServer.first { it.name == "Den" }.name)
        testScheduler.advanceTimeBy(10.minutes)
        assertTrue(h.servers.seen.count { it == DOWNLOADS } > synced, "coming to the foreground syncs the downloads")
        assertTrue(h.servers.seen.size > idle)

        h.core.setForeground(false)
        testScheduler.runCurrent()
        val left = h.servers.seen.size
        testScheduler.advanceTimeBy(10.minutes)
        assertEquals(left, h.servers.seen.size, "the live events stop as the app leaves the foreground")
        assertTrue(session.scope.isActive)
    }

    @Test
    fun theForegroundOpensTheLiveEventsSocketAndTheBackgroundClosesIt() = runBlocking {
        val opened = Channel<Unit>(Channel.UNLIMITED)
        val closed = Channel<Unit>(Channel.UNLIMITED)
        val server = embeddedServer(CIO, port = 0) {
            install(WebSockets)
            routing {
                get("/api/v1/server") {
                    val q = call.request.queryParameters
                    val proof = proofOf(q["challenge"], q["probeId"], call.request.headers[HttpHeaders.Host].orEmpty(), "server-1", listOf("tok"))
                    call.respondText(kivoJson.encodeToString(ServerInfo.serializer(), info("server-1", "Basement").copy(proof = proof)), ContentType.Application.Json)
                }
                webSocket("/api/v1/events") {
                    send(Frame.Text("""{"type":"snapshot","serverVersion":"1.0"}"""))
                    opened.send(Unit)
                    try {
                        for (frame in incoming) Unit
                    } finally {
                        closed.trySend(Unit)
                    }
                }
            }
        }
        server.startSuspend()
        val scope = CoroutineScope(SupervisorJob())
        try {
            val device = TestDevice(withDownloads = false)
            device.secureStore.values["kivo.token.server-1"] = "tok"
            val core = KivoClient(device, null, scope)
            val url = "http://127.0.0.1:${server.engine.resolvedConnectors().first().port}"
            core.restore(SavedServer("server-1", "Basement", listOf(url))).close()
            core.start()
            withTimeout(20.seconds) { core.signedIn() }
            assertTrue(opened.tryReceive().isFailure, "no socket before the app is in the foreground")

            core.setForeground(true)
            withTimeout(20.seconds) { opened.receive() }
            core.setForeground(false)
            withTimeout(20.seconds) { closed.receive() }
            core.setForeground(true)
            withTimeout(20.seconds) { opened.receive() }
            core.changeServer()
            withTimeout(20.seconds) { closed.receive() }
        } finally {
            scope.cancel()
            server.stop(0, 0)
        }
    }

    @Test
    fun aServerTheAppIsTooOldForOpensSignedInAndSaysSo() = runTest {
        val h = signedInBefore(release("0.9.0", minAppVersion = "0.6.0"), appVersion = "0.5.0")

        h.core.start()
        val session = h.core.signedIn()

        assertEquals(Compatibility.AppTooOld("0.5.0", "0.6.0"), session.compatibility.value)
        assertEquals(Compatibility.AppTooOld("0.5.0", "0.6.0"), session.state.value.compatibility)
    }

    @Test
    fun aServerTooOldForTheAppOpensSignedInAndSaysSo() = runTest {
        val h = signedInBefore(release("0.2.0"), appVersion = "0.5.0")

        h.core.start()

        assertEquals(Compatibility.ServerTooOld("0.2.0", MIN_SERVER_VERSION, "0.5.0"), h.core.signedIn().compatibility.value)
    }

    @Test
    fun comingToTheForegroundChecksTheServerAgainstTheAppAgain() = runTest {
        val h = signedInBefore(release("0.5.0"), appVersion = "0.5.0")
        h.core.start()
        val session = h.core.signedIn()
        assertEquals(Compatibility.Compatible, session.compatibility.value)
        h.servers.server("home.test", release("0.9.0", minAppVersion = "0.6.0"))

        h.core.setForeground(true)

        assertEquals(Compatibility.AppTooOld("0.5.0", "0.6.0"), session.state.first { it.compatibility != Compatibility.Compatible }.compatibility)
        h.core.setForeground(false)
    }

    @Test
    fun aSessionsStateFollowsItsServerUserAndReachability() = runTest {
        val h = signedInBefore()
        h.core.start()
        val session = h.core.signedIn()

        val state = session.state.first { it.user != null }

        assertEquals(SessionState(home, session.user.value, unreachable = false, Compatibility.Compatible, "http://home.test"), state)
        session.reachability.unanswered()
        assertTrue(session.state.first { it.unreachable }.unreachable)
    }
}
