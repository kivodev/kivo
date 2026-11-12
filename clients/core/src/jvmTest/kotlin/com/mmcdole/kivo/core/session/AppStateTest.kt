package com.mmcdole.kivo.core.session

import com.mmcdole.kivo.api.models.DownloadDelivery
import com.mmcdole.kivo.api.models.DownloadList
import com.mmcdole.kivo.api.models.ServerInfo
import com.mmcdole.kivo.core.Compatibility
import com.mmcdole.kivo.core.MIN_SERVER_VERSION
import com.mmcdole.kivo.core.auth.SavedServer
import com.mmcdole.kivo.core.downloads.db.DownloadDatabase
import com.mmcdole.kivo.core.json
import com.mmcdole.kivo.core.kivoJson
import com.mmcdole.kivo.core.problem
import com.mmcdole.kivo.core.TestServerKey
import com.mmcdole.kivo.core.auth.ServerConnection
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
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.isActive
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.currentTime
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeout
import java.io.IOException
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertIs
import kotlin.test.assertSame
import kotlin.test.assertTrue
import kotlin.time.Duration.Companion.seconds
import com.mmcdole.kivo.api.models.Download as ServerDownload
import com.mmcdole.kivo.core.downloads.db.Download as Row

// Where the app is, as its root shows it from KivoClient.state: opening the
// server last signed in to, one that doesn't answer, a session that ends,
// leaving for another server, and what runs while the app is in the
// foreground. Against servers answered by Ktor's mock engine by host, on
// the test's virtual time.

private const val SERVER = "home.test GET /api/v1/server"
private const val DOWNLOADS = "home.test GET /api/v1/downloads"

/**
 * Runs [test] on a device whose app, when [signedInBefore] says what its
 * server is, was signed in to [home] at `home.test` before.
 */
private fun appTest(
    signedInBefore: ServerInfo? = info("server-1", "Basement"),
    withDownloads: Boolean = false,
    appVersion: String = "1.0",
    test: suspend TestScope.(Harness) -> Unit,
) = runTest {
    val h = Harness(this, withDownloads, appVersion)
    if (signedInBefore != null) {
        h.servers.server("home.test", signedInBefore)
        h.device.signedInTo("server-1")
        h.core.restore(home).close()
    }
    test(h)
}

private fun Harness.unreachable() {
    for (path in listOf("/api/v1/server", "/api/v1/users/me")) servers.on("home.test", "GET", path) { throw IOException("no route to host") }
}

private suspend fun KivoClient.started(): AppState = state.first { it != AppState.Starting }

private suspend fun KivoClient.session(): KivoSession = assertIs<AppState.SignedIn>(started()).session

/** How many times the server was asked for the downloads, once the syncs under way are over; waits in real time, for the server's own threads. */
private suspend fun Harness.syncs(): Int = withContext(Dispatchers.Default) {
    var seen = -1
    while (seen != servers.seen.count { it == DOWNLOADS }) {
        seen = servers.seen.count { it == DOWNLOADS }
        delay(300)
    }
    seen
}

@OptIn(ExperimentalCoroutinesApi::class)
class AppStateTest {
    @Test
    fun withNoServerSignedInToTheAppStartsSignedOut() = appTest(signedInBefore = null) { h ->
        assertEquals(AppState.Starting, h.core.state.value)

        h.core.start()

        assertEquals(AppState.SignedOut(null), h.core.started())
    }

    @Test
    fun aSavedServerTheDeviceHoldsNoSessionOnIsNotOpened() = appTest { h ->
        h.device.secureStore.values.clear()

        h.core.start()

        assertEquals(AppState.SignedOut(null), h.core.started())
    }

    @Test
    fun theServerLastSignedInToOpensSignedInAndItsUserIsRead() = appTest { h ->
        h.core.start()
        val session = h.core.session()

        assertTrue(session.isSignedIn)
        assertEquals("server-1", session.savedServer.value.id)
        assertEquals("anna", session.user.first { it != null }?.username)
        assertTrue("home.test GET /api/v1/users/me" in h.servers.seen)
    }

    @Test
    fun startingTwiceOpensTheServerOnce() = appTest { h ->
        h.core.start()
        val session = h.core.session()

        h.core.start()
        testScheduler.runCurrent()

        assertSame(session, h.core.session())
        assertTrue(session.scope.isActive)
    }

    @Test
    fun aServerThatAnswersNowhereIsUnreachableOnceItHasStayedSoAndRetryOpensIt() = appTest { h ->
        val before = h.servers.seen.size
        h.unreachable()

        h.core.start()

        assertEquals(AppState.Unreachable(home), h.core.started())
        assertTrue(currentTime >= 3000, "it's tried twice more, a moment apart, before it's unreachable")
        assertTrue(h.servers.seen.drop(before).count { it == SERVER } >= 3)

        h.core.retry()
        assertEquals(AppState.Starting, h.core.state.value)
        assertEquals(AppState.Unreachable(home), h.core.started(), "it still doesn't answer")

        h.servers.server("home.test", info("server-1", "Basement"))
        h.core.retry()
        assertFalse(h.core.session().unreachable.value)
    }

    @Test
    fun aServerThatAnswersAsStartingTriesAgainOpensSignedIn() = appTest { h ->
        h.servers.on("home.test", "GET", "/api/v1/server") {
            if (currentTime < 1500) throw IOException("the network is waking")
            json(ServerInfo.serializer(), info("server-1", "Basement").copy(proof = proofOf(it, "server-1")))
        }

        h.core.start()

        assertFalse(h.core.session().unreachable.value)
    }

    @Test
    fun aSessionTheServerEndedSignsInToItsServerAgain() = appTest { h ->
        h.servers.on("home.test", "GET", "/api/v1/users/me") { problem(HttpStatusCode.Unauthorized, "unauthorized") }

        h.core.start()

        assertEquals(AppState.SignedOut(home), h.core.state.first { it is AppState.SignedOut })
    }

    @Test
    fun aSessionRevokedWhileTheAppWasClosedSignsInToItsServerAgainAtOnce() = appTest { h ->
        // Revoked, the session is gone from the server: it still proves itself with its key, and refuses the token.
        h.servers.on("home.test", "GET", "/api/v1/users/me") { problem(HttpStatusCode.Unauthorized, "unauthorized") }
        val start = currentTime

        h.core.start()

        assertEquals(AppState.SignedOut(home), h.core.state.first { it is AppState.SignedOut || it is AppState.Unreachable })
        assertTrue(currentTime - start < 3000, "took ${currentTime - start} ms")
    }

    @Test
    fun aSessionRevokedWhileTheAppWasClosedSignsInToItsServerAgainAtOnceOverHttps() = appTest(signedInBefore = null) { h ->
        val secure = SavedServer("server-1", "Basement", listOf("https://home.test"))
        h.servers.server("home.test", info("server-1", "Basement"))
        h.servers.on("home.test", "GET", "/api/v1/users/me") { problem(HttpStatusCode.Unauthorized, "unauthorized") }
        h.device.signedInTo("server-1")
        h.core.restore(secure).close()
        val start = currentTime

        h.core.start()

        assertEquals(AppState.SignedOut(secure), h.core.state.first { it is AppState.SignedOut || it is AppState.Unreachable })
        assertTrue(currentTime - start < 3000, "took ${currentTime - start} ms")
    }

    @Test
    fun aServerThatProvesItselfWithAnotherKeyCantBeReached() = appTest { h ->
        // Set up again, or an impostor: it answers as the server, signed by a key that isn't the one the app keeps.
        val other = TestServerKey()
        h.servers.on("home.test", "GET", "/api/v1/server") { json(ServerInfo.serializer(), info("server-1", "Basement").copy(proof = proofOf(it, "server-1", other))) }
        val before = h.servers.seen.size
        val start = currentTime

        h.core.start()

        assertEquals(AppState.Unreachable(home), h.core.state.first { it is AppState.SignedOut || it is AppState.Unreachable })
        assertTrue(currentTime - start < 3000, "took ${currentTime - start} ms")
        assertEquals(listOf(SERVER), h.servers.seen.drop(before).distinct(), "nothing but probes, so nothing carries the token")
        assertTrue(ServerConnection.holdsSession(h.device.secureStore, "server-1"))
    }

    @Test
    fun signingOutGoesBackToSigningInToTheServerAndClosesTheSession() = appTest { h ->
        h.core.start()
        val session = h.core.session()

        session.connection.signOut()

        assertEquals(AppState.SignedOut(home), h.core.state.first { it is AppState.SignedOut })
        testScheduler.runCurrent()
        assertFalse(session.scope.isActive)
    }

    @Test
    fun changingServerLeavesForSignInAndKeepsTheDeviceSignedIn() = appTest { h ->
        h.core.start()
        val session = h.core.session()

        h.core.changeServer()

        assertEquals(AppState.SignedOut(null), h.core.state.value)
        assertFalse(session.scope.isActive)
        assertEquals("server-1", h.core.lastSignedIn()?.id)
    }

    @Test
    fun changingServerFromOneThatCantBeReachedSignsIn() = appTest { h ->
        h.unreachable()
        h.core.start()
        assertIs<AppState.Unreachable>(h.core.started())

        h.core.changeServer()

        assertEquals(AppState.SignedOut(null), h.core.state.value)
    }

    @Test
    fun aSignInShowsItsServerInPlaceOfTheOneOpen() = appTest { h ->
        h.servers.server("b.test", info("b"))
        h.device.signedInTo("b")
        h.core.start()
        val first = h.core.session()

        val other = h.core.restore(SavedServer("b", "b", listOf("http://b.test")))
        h.core.signedIn(other)

        assertEquals(AppState.SignedIn(other), h.core.state.value)
        assertFalse(first.scope.isActive)
        other.connection.signOut()
        assertEquals("b", assertIs<AppState.SignedOut>(h.core.state.first { it is AppState.SignedOut }).server?.id)
    }

    @Test
    fun anUnreachableServerWithDownloadsOnTheDeviceOpensWithoutIt() = appTest(withDownloads = true) { h ->
        DownloadDatabase(h.device.downloads!!.driver).downloadsQueries.insertDownload(
            Row("d1", "server-1", "u1", null, "w1", null, null, null, "standard", "ready", null, null, null, "{}", "{}", "Night Shift", "movie", null, null, null, null, 1, 1, 1, null),
        )
        h.unreachable()

        h.core.start()
        val session = h.core.session()

        assertTrue(currentTime < 1500, "it opens at once, without trying again first")
        assertTrue(session.isSignedIn)
        assertTrue(session.unreachable.value)
        assertTrue(session.state.first { it.unreachable }.unreachable)

        // Its reads ask until the server answers.
        h.servers.server("home.test", info("server-1", "Basement"))
        assertFalse(session.unreachable.first { !it })
        assertEquals("anna", session.user.first { it != null }?.username)
    }

    @Test
    fun anUnreachableServerWithDownloadsOfAnotherServerOnlyIsUnreachable() = appTest(withDownloads = true) { h ->
        DownloadDatabase(h.device.downloads!!.driver).downloadsQueries.insertDownload(
            Row("d1", "server-2", "u1", null, "w1", null, null, null, "standard", "ready", null, null, null, "{}", "{}", "Night Shift", "movie", null, null, null, null, 1, 1, 1, null),
        )
        h.unreachable()

        h.core.start()

        assertEquals(AppState.Unreachable(home), h.core.started())
    }

    @Test
    fun comingToTheForegroundReadsTheServerAgainAndSyncsTheDownloads() = appTest(withDownloads = true) { h ->
        // A download the server is still preparing, which a sync asks the server about.
        val preparing = ServerDownload("r1", "w1", "v1", "high", "queued", DownloadDelivery.Conversion, emptyList(), 1, 1, false, emptyList(), emptyList())
        h.servers.on("home.test", "GET", "/api/v1/downloads") { json(DownloadList.serializer(), DownloadList(listOf(preparing))) }
        DownloadDatabase(h.device.downloads!!.driver).downloadsQueries.insertDownload(
            Row(
                "d1", "server-1", "u1", "r1", "w1", null, null, null, "high", "queued", null, null, null,
                kivoJson.encodeToString(ServerDownload.serializer(), preparing), "{}", "Night Shift", "movie", null, null, null, null, 1, 1, null, null,
            ),
        )
        h.core.start()
        val session = h.core.session()
        val opened = h.syncs()
        h.servers.server("home.test", info("server-1", "Den"))

        h.core.setForeground(true)

        assertEquals("Den", session.savedServer.first { it.name == "Den" }.name)
        assertEquals(opened + 1, h.syncs(), "the downloads sync as the app comes to the foreground")

        h.core.setForeground(false)
        assertEquals(opened + 1, h.syncs(), "and not as it leaves it")
        h.core.setForeground(true)
        assertEquals(opened + 2, h.syncs(), "each time it comes back")
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
                    val proof = proofOf(q["challenge"], call.request.headers[HttpHeaders.Host].orEmpty(), "server-1")
                    call.respondText(kivoJson.encodeToString(ServerInfo.serializer(), info("server-1", "Basement").copy(proof = proof)), ContentType.Application.Json)
                }
                webSocket("/api/v1/events") {
                    send(Frame.Text("""{"type":"snapshot","serverVersion":"1.0"}"""))
                    opened.send(Unit)
                    try {
                        for (frame in incoming) continue
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
            withTimeout(20.seconds) { core.session() }
            delay(500)
            assertTrue(opened.tryReceive().isFailure, "no socket before the app is in the foreground")

            core.setForeground(true)
            withTimeout(20.seconds) { opened.receive() }
            core.setForeground(false)
            withTimeout(20.seconds) { closed.receive() }
            delay(500)
            assertTrue(opened.tryReceive().isFailure, "no socket in the background")

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
    fun aServerTheAppIsTooOldForOpensSignedInAndSaysSo() = appTest(release("0.9.0", minAppVersion = "0.6.0"), appVersion = "0.5.0") { h ->
        h.core.start()
        val session = h.core.session()

        assertEquals(Compatibility.AppTooOld("0.5.0", "0.6.0"), session.compatibility.value)
        assertEquals(Compatibility.AppTooOld("0.5.0", "0.6.0"), session.state.value.compatibility)
    }

    @Test
    fun aServerTooOldForTheAppOpensSignedInAndSaysSo() = appTest(release("0.2.0"), appVersion = "0.5.0") { h ->
        h.core.start()

        assertEquals(Compatibility.ServerTooOld("0.2.0", MIN_SERVER_VERSION, "0.5.0"), h.core.session().compatibility.value)
    }

    @Test
    fun comingToTheForegroundChecksTheServerAgainstTheAppAgain() = appTest(release("0.5.0"), appVersion = "0.5.0") { h ->
        h.core.start()
        val session = h.core.session()
        assertEquals(Compatibility.Compatible, session.compatibility.value)
        h.servers.server("home.test", release("0.9.0", minAppVersion = "0.6.0"))

        h.core.setForeground(true)

        assertEquals(Compatibility.AppTooOld("0.5.0", "0.6.0"), session.state.first { it.compatibility != Compatibility.Compatible }.compatibility)
    }

    @Test
    fun aSessionsStateFollowsItsServerUserAndReachability() = appTest { h ->
        h.core.start()
        val session = h.core.session()

        val state = session.state.first { it.user != null }

        assertEquals(SessionState(home, session.user.value, unreachable = false, Compatibility.Compatible, "http://home.test"), state)
        session.reachability.unanswered()
        assertTrue(session.state.first { it.unreachable }.unreachable)
    }
}
