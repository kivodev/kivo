package com.mmcdole.kivo.core.auth

import com.mmcdole.kivo.core.MemoryPreferences
import com.mmcdole.kivo.core.MemoryStore
import com.mmcdole.kivo.core.RequestProblem
import io.ktor.client.plugins.websocket.webSocket
import io.ktor.client.request.get
import io.ktor.client.request.post
import io.ktor.http.HttpStatusCode
import io.ktor.server.application.install
import io.ktor.server.cio.CIO
import io.ktor.server.engine.embeddedServer
import io.ktor.server.response.respondText
import io.ktor.server.routing.get
import io.ktor.server.routing.post
import io.ktor.server.routing.routing
import io.ktor.server.websocket.WebSockets
import io.ktor.server.websocket.webSocket
import io.ktor.websocket.Frame
import io.ktor.websocket.readText
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import java.net.ServerSocket
import java.net.Socket
import java.util.Collections
import kotlin.concurrent.thread
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFails
import kotlin.test.assertTrue
import kotlin.time.Duration.Companion.milliseconds
import kotlin.time.Duration.Companion.seconds
import kotlin.time.TimeSource

// The connection's timeouts over the HTTP client the platform would use,
// shortened: a server that takes connections and never answers fails a read
// in about the idle wait, as no answer; one that answers slowly is waited
// for where its calls take long; the live events' socket stays open
// however quiet it is.

private val short = ServerTimeouts(connect = 300.milliseconds, idle = 300.milliseconds, request = 600.milliseconds, slow = 3.seconds)

class ServerTimeoutsTest {
    @Test
    fun aServerThatNeverAnswersFailsAReadInSecondsAsNoAnswer() = runBlocking {
        // Takes each connection and says nothing, as a server behind a dropped route never does.
        val silent = ServerSocket(0)
        val held = Collections.synchronizedList(mutableListOf<Socket>())
        thread(isDaemon = true) { runCatching { while (true) held += silent.accept() } }
        try {
            val c = ServerConnection("server-1", "http://127.0.0.1:${silent.localPort}", MemoryStore(), MemoryPreferences(), timeouts = short)
            val started = TimeSource.Monotonic.markNow()
            val e = assertFails { c.users.getCurrentUser() }
            assertTrue(started.elapsedNow() < 2.seconds, "failed after ${started.elapsedNow()}")
            assertEquals(RequestProblem.Unreachable, RequestProblem.of(e), "a timeout is no answer: $e")
        } finally {
            silent.close()
            held.forEach { it.close() }
        }
    }

    @Test
    fun aPlaysStartIsWaitedForWhereAReadIsNot() = runBlocking {
        val server = embeddedServer(CIO, port = 0) {
            install(WebSockets)
            routing {
                post("/api/v1/plays") {
                    delay(1_000)
                    call.respondText("{}", status = HttpStatusCode.Created)
                }
                get("/api/v1/users/me") {
                    delay(1_000)
                    call.respondText("{}")
                }
                webSocket("/api/v1/events") {
                    delay(1_500)
                    send(Frame.Text("hello"))
                }
            }
        }
        server.startSuspend()
        try {
            val port = server.engine.resolvedConnectors().first().port
            val base = "http://127.0.0.1:$port"
            val c = ServerConnection("server-1", base, MemoryStore(), MemoryPreferences(), timeouts = short)
            assertEquals(HttpStatusCode.Created, c.http.post("$base/api/v1/plays").status, "a play's start waits for its transcode")
            assertEquals(RequestProblem.Unreachable, RequestProblem.of(assertFails { c.http.get("$base/api/v1/users/me") }))
            var heard: String? = null
            c.http.webSocket("ws://127.0.0.1:$port/api/v1/events") {
                heard = (incoming.receive() as Frame.Text).readText()
            }
            assertEquals("hello", heard, "the live events outlast both waits")
        } finally {
            server.stop(0, 0)
        }
    }
}
