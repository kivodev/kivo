package com.mmcdole.kivo.core.auth

import com.mmcdole.kivo.api.models.CurrentUser
import com.mmcdole.kivo.api.models.PlaybackPreferences
import com.mmcdole.kivo.api.models.ServerInfo
import com.mmcdole.kivo.api.models.SubtitleMode
import com.mmcdole.kivo.api.models.UserLimits
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.MemoryPreferences
import com.mmcdole.kivo.core.MemoryStore
import com.mmcdole.kivo.core.json
import com.mmcdole.kivo.core.problem
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockEngineConfig
import io.ktor.client.engine.mock.respond
import io.ktor.client.request.get
import io.ktor.client.statement.bodyAsText
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.delay
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import java.io.IOException
import java.util.concurrent.CopyOnWriteArrayList
import kotlin.coroutines.EmptyCoroutineContext
import kotlin.test.Test
import kotlin.test.assertEquals

// A connection whose server moves between its addresses: every request,
// however it was made, goes to the address in use, and a read with no
// answer picks again before it waits.

private const val LOCAL = "http://192.168.1.20:8484"
private const val PUBLIC = "https://kivo.example.com"

private val anna = CurrentUser("u1", "anna", admin = false, limits = UserLimits(), playback = PlaybackPreferences(SubtitleMode.Smart, true, true, 1.0))

/** The server at each address that's up, on the test's clock, and every request as "host path". */
private class Addresses(scope: TestScope) {
    val up = mutableSetOf<String>()
    val seen: MutableList<String> = CopyOnWriteArrayList()

    val engine = MockEngine(
        MockEngineConfig().apply {
            dispatcher = StandardTestDispatcher(scope.testScheduler)
            addHandler { request ->
                seen += "${request.url.host} ${request.url.encodedPath}"
                if (up.none { it.contains(request.url.host) }) throw IOException("no route to host")
                when (request.url.encodedPath) {
                    "/api/v1/server" -> json(ServerInfo.serializer(), ServerInfo("server-1", "Basement", "1.0"))
                    "/api/v1/users/me" -> json(CurrentUser.serializer(), anna)
                    "/api/v1/images/i1" -> respond("image from ${request.url.host}", headers = io.ktor.http.headersOf(HttpHeaders.ContentType, "image/webp"))
                    else -> problem(HttpStatusCode.NotFound, "notFound")
                }
            }
        },
    )

    private val time = scope.testScheduler.timeSource

    fun connection(at: String) = ServerConnection(
        "server-1", at, MemoryStore(), MemoryPreferences(), engine,
        addresses = listOf(LOCAL, PUBLIC), background = EmptyCoroutineContext, pickTime = time,
    )
}

class ConnectionAddressTest {
    @Test
    fun aReadWithNoAnswerPicksAgainAndEveryRequestFollows() = runTest {
        val net = Addresses(this)
        net.up += LOCAL
        val c = net.connection(LOCAL)
        val oldImage = c.imageUrl("i1", 480)
        assertEquals("anna", c.users.getCurrentUser().result().username)

        // Away from home: the local address goes quiet.
        net.up.clear()
        net.up += PUBLIC
        delay(PICK_REST)
        net.seen.clear()
        val user = c.reachability.untilAnswered { c.users.getCurrentUser().result() }

        assertEquals("anna", user.username)
        assertEquals(PUBLIC, c.baseUrl)
        assertEquals(
            listOf("192.168.1.20 /api/v1/users/me", "kivo.example.com /api/v1/users/me"),
            net.seen.filter { it.endsWith("/users/me") },
            "asked again at once at the new address, without the wait",
        )
        assertEquals("$PUBLIC/api/v1/images/i1?width=480", c.imageUrl("i1", 480))
        assertEquals("$PUBLIC/api/v1/plays/p1/hls/main.m3u8", c.apiUrl("plays/p1/hls/main.m3u8"))
        assertEquals("image from kivo.example.com", c.http.get(oldImage).bodyAsText(), "a URL made before the move arrives too")
    }

    @Test
    fun aRequestAtTheAddressInUseIsSentAsItIs() = runTest {
        val net = Addresses(this)
        net.up += PUBLIC
        val c = net.connection(PUBLIC)

        assertEquals("image from kivo.example.com", c.http.get(c.imageUrl("i1", 480)).bodyAsText())
        assertEquals(listOf("kivo.example.com /api/v1/images/i1"), net.seen)
    }
}
