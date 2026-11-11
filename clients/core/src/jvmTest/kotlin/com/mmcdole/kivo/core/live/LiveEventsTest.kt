package com.mmcdole.kivo.core.live

import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import com.mmcdole.kivo.core.MemoryPreferences
import com.mmcdole.kivo.core.MemoryStore
import com.mmcdole.kivo.core.auth.AddressChange
import com.mmcdole.kivo.core.auth.NetworkEpoch
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.kivoJson
import com.mmcdole.kivo.core.proofOf
import com.mmcdole.kivo.api.models.ServerInfo
import io.ktor.http.ContentType
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.server.application.createApplicationPlugin
import io.ktor.server.application.install
import io.ktor.server.cio.CIO
import io.ktor.server.engine.embeddedServer
import io.ktor.server.request.path
import io.ktor.server.response.respond
import io.ktor.server.response.respondText
import io.ktor.server.routing.get
import io.ktor.server.routing.routing
import io.ktor.server.websocket.DefaultWebSocketServerSession
import io.ktor.server.websocket.WebSockets
import io.ktor.server.websocket.webSocket
import io.ktor.websocket.CloseReason
import io.ktor.websocket.Frame
import io.ktor.websocket.close
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.take
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlinx.coroutines.yield
import java.util.Collections
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlin.time.Duration.Companion.hours
import kotlin.time.Duration.Companion.milliseconds
import kotlin.time.Duration.Companion.seconds

// The live-events client against a real WebSocket server, over the HTTP
// client the platform would use: the bearer token on the upgrade,
// reconnecting after the server goes away (1001), and signing out without
// reconnecting on 4401 or a refused upgrade.

private const val TOKEN_KEY = "kivo.token.server-1"

private typealias Script = suspend DefaultWebSocketServerSession.() -> Unit

private suspend fun DefaultWebSocketServerSession.text(json: String) = send(Frame.Text(json))

private suspend fun DefaultWebSocketServerSession.snapshot() = text("""{"type":"snapshot","serverVersion":"1.0"}""")

/**
 * A server whose nth socket runs the nth script, recording each upgrade's
 * Authorization header; it answers who it is as [id].
 */
private class EventServer(private val scripts: List<Script>) : AutoCloseable {
    val upgrades: MutableList<String?> = Collections.synchronizedList(mutableListOf())

    @Volatile
    var id = "server-1"

    private val server = embeddedServer(CIO, port = 0) {
        install(WebSockets)
        install(
            createApplicationPlugin("Auth") {
                onCall { call ->
                    if (call.request.path() == "/api/v1/server") return@onCall
                    if (call.request.headers[HttpHeaders.Authorization] != "Bearer tok") call.respond(HttpStatusCode.Unauthorized)
                }
            },
        )
        routing {
            get("/api/v1/server") {
                val q = call.request.queryParameters
                val proof = proofOf(q["challenge"], call.request.headers[HttpHeaders.Host].orEmpty(), id)
                val info = ServerInfo(id, "Basement", "1.0", proof = proof)
                call.respondText(kivoJson.encodeToString(ServerInfo.serializer(), info), ContentType.Application.Json)
            }
            webSocket("/api/v1/events") {
                val n = synchronized(upgrades) { upgrades.size.also { upgrades += call.request.headers[HttpHeaders.Authorization] } }
                scripts.getOrElse(n) { { awaitCancellation() } }.invoke(this)
            }
        }
    }

    /** Starts the server and connects to it, with [token] stored, over the platform's HTTP client. */
    suspend fun connection(
        token: String,
        store: MemoryStore = MemoryStore(mutableMapOf(TOKEN_KEY to token)),
        preferences: MemoryPreferences = MemoryPreferences(),
        network: NetworkEpoch = NetworkEpoch(),
    ): ServerConnection {
        return ServerConnection("server-1", url(), store, preferences, network = network).also { it.restore() }
    }

    /** Starts the server; its base URL. */
    suspend fun url(): String {
        server.startSuspend()
        return "http://127.0.0.1:${server.engine.resolvedConnectors().first().port}"
    }

    override fun close() = server.stop(0, 0)
}

class LiveEventsTest {
    @Test
    fun mapsEventsAndReconnectsAfterTheServerGoesAway() = runBlocking {
        val first: Script = {
            snapshot()
            text("""{"type":"event","kind":"libraryChanged","libraryIds":["l1"]}""")
            text("""{"type":"event","kind":"tasksChanged"}""")
            text("""{"type":"somethingNew"}""")
            text("""{"type":"event","kind":"workChanged","workId":"w1"}""")
            text("""{"type":"event","kind":"playEnded","playId":"p1"}""")
            text("""{"type":"event","kind":"playEnded","playId":"p2","message":"Server maintenance"}""")
            close(CloseReason(CloseReason.Codes.GOING_AWAY, "stopping"))
        }
        val second: Script = {
            snapshot()
            text("""{"type":"resync"}""")
            snapshot()
            text("""{"type":"event","kind":"userStateChanged","workIds":["w2"]}""")
            awaitCancellation()
        }
        EventServer(listOf(first, second)).use { server ->
            val live = LiveEvents(server.connection("tok")) { 10.milliseconds }

            withTimeout(10.seconds) {
                val received = async { live.invalidations.take(7).toList() }
                yield()
                val running = launch { live.run() }
                assertEquals(
                    listOf(
                        Invalidation.LibraryChanged(listOf("l1")),
                        Invalidation.WorkChanged("w1"),
                        Invalidation.PlayEnded("p1"),
                        Invalidation.PlayEnded("p2", "Server maintenance"),
                        // The new socket's snapshot: events may have been missed while it was closed.
                        Invalidation.Everything,
                        // The resync; the snapshot after it adds nothing.
                        Invalidation.Everything,
                        Invalidation.UserStateChanged(listOf("w2")),
                    ),
                    received.await(),
                )
                running.cancel()
            }
            assertEquals(listOf<String?>("Bearer tok", "Bearer tok"), server.upgrades)
        }
    }

    @Test
    fun theSocketOpensAgainAtOnceWhereTheServerMoved() = runBlocking {
        val first: Script = {
            snapshot()
            text("""{"type":"event","kind":"workChanged","workId":"w1"}""")
            awaitCancellation()
        }
        val second: Script = {
            snapshot()
            text("""{"type":"event","kind":"workChanged","workId":"w2"}""")
            awaitCancellation()
        }
        EventServer(listOf(first)).use { home ->
            EventServer(listOf(second)).use { away ->
                val connection = home.connection("tok")
                connection.address.remember(listOf(connection.baseUrl, away.url()))
                // Never waits to reconnect: a move opens the socket again at once.
                val live = LiveEvents(connection) { 1.hours }

                withTimeout(10.seconds) {
                    val received = async { live.invalidations.take(3).toList() }
                    val delivered = async { live.invalidations.first() }
                    val running = launch { live.run() }
                    // The first socket's snapshot came before its event, so the next one is a reconnect.
                    delivered.await()

                    // The address in use answers as another server now: the session moves.
                    home.id = "someone-else"
                    assertTrue(connection.address.repick(AddressChange.Network))
                    assertEquals(
                        listOf(Invalidation.WorkChanged("w1"), Invalidation.Everything, Invalidation.WorkChanged("w2")),
                        received.await(),
                    )
                    running.cancel()
                }
                assertEquals(1, home.upgrades.size)
                assertEquals(listOf<String?>("Bearer tok"), away.upgrades)
            }
        }
    }

    @Test
    fun aNewNetworkClosesTheSocketUntilItsAddressProvesItselfAgain() = runBlocking {
        val first: Script = {
            snapshot()
            text("""{"type":"event","kind":"workChanged","workId":"w1"}""")
            awaitCancellation()
        }
        val second: Script = {
            snapshot()
            text("""{"type":"event","kind":"workChanged","workId":"w3"}""")
            awaitCancellation()
        }
        EventServer(listOf(first, second)).use { server ->
            val network = NetworkEpoch()
            val connection = server.connection("tok", network = network)
            val live = LiveEvents(connection) { 10.milliseconds }

            withTimeout(10.seconds) {
                val received = async { live.invalidations.take(3).toList() }
                val delivered = async { live.invalidations.first() }
                val running = launch { live.run() }
                // The first socket's snapshot came before its event, so the next one is a reconnect.
                delivered.await()

                network.changed()
                assertEquals(
                    listOf(Invalidation.WorkChanged("w1"), Invalidation.Everything, Invalidation.WorkChanged("w3")),
                    received.await(),
                )
                running.cancel()
            }
            assertEquals(listOf<String?>("Bearer tok", "Bearer tok"), server.upgrades)
        }
    }

    @Test
    fun closing4401SignsOutWithoutReconnecting() = runBlocking {
        val script: Script = {
            snapshot()
            close(CloseReason(CLOSE_UNAUTHORIZED, "signed out"))
        }
        EventServer(listOf(script)).use { server ->
            val store = MemoryStore(mutableMapOf(TOKEN_KEY to "tok"))
            val preferences = MemoryPreferences()
            preferences.edit { it[stringPreferencesKey("kivo.user.server-1")] = """{"id":"u1","username":"anna","admin":false}""" }
            val connection = server.connection("tok", store, preferences)
            assertEquals("anna", connection.user.value?.username)

            withTimeout(10.seconds) { LiveEvents(connection) { 10.milliseconds }.run() }

            assertEquals(1, server.upgrades.size)
            assertFalse(connection.signedIn.value)
            assertNull(store.values[TOKEN_KEY])
            assertNull(connection.user.value)
            assertNull(preferences.data.first()[stringPreferencesKey("kivo.user.server-1")])
        }
    }

    @Test
    fun aRefusedUpgradeSignsOut() = runBlocking {
        EventServer(emptyList()).use { server ->
            // An expired session: the server still proves itself to it, and refuses it.
            val connection = server.connection("expired0")

            withTimeout(10.seconds) { LiveEvents(connection) { 10.milliseconds }.run() }

            assertEquals(0, server.upgrades.size)
            assertFalse(connection.signedIn.value)
        }
    }
}
