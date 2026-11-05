package com.mmcdole.kivo.core.auth

import com.mmcdole.kivo.api.ProblemException
import com.mmcdole.kivo.api.models.CurrentUser
import com.mmcdole.kivo.api.models.PlaybackPreferences
import com.mmcdole.kivo.api.models.DeviceCode
import com.mmcdole.kivo.api.models.Session
import com.mmcdole.kivo.api.models.SubtitleMode
import com.mmcdole.kivo.api.models.UserLimits
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.FakeServer
import com.mmcdole.kivo.core.MemoryStore
import com.mmcdole.kivo.core.empty
import com.mmcdole.kivo.core.json
import com.mmcdole.kivo.core.problem
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.currentTime
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlin.time.Instant

private val anna = CurrentUser("u1", "anna", admin = false, limits = UserLimits(), playback = PlaybackPreferences(SubtitleMode.Forced, true, true, 1.0))

private val code = DeviceCode("ABC234", "secret-1", Instant.fromEpochSeconds(1_800_000_000), 2, "http://kivo.test/link?code=ABC234&server=server-1")

private const val TOKEN_KEY = "kivo.token.server-1"

private val at = Instant.fromEpochSeconds(1_800_000_000)

private val session = Session("d1", "app", "Living Room", at, at, current = true, token = "tok")

/** Who the server says is signed in, once the token is the app's. */
private fun FakeServer.signedInAs(user: CurrentUser) = on("GET", "/api/v1/users/me") { json(CurrentUser.serializer(), user) }

class SignInTest {
    @Test
    fun aDeviceCodePollsUntilApprovedThenSignsIn() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        val store = MemoryStore()
        val connection = server.connection(store)
        server.on("POST", "/api/v1/device-codes") { json(DeviceCode.serializer(), code, HttpStatusCode.Created) }
        var polls = 0
        server.on("POST", "/api/v1/device-codes/poll") {
            if (++polls < 3) empty(HttpStatusCode.Accepted) else json(Session.serializer(), session, HttpStatusCode.Created)
        }
        server.signedInAs(anna)

        val states = SignIn(connection, "Living Room","client-1").link().toList()

        assertEquals(
            listOf(LinkState.Waiting("ABC234", code.linkUrl, code.expiresAt), LinkState.SignedIn(anna)),
            states,
        )
        assertEquals(
            """{"name":"Living Room","clientId":"client-1"}""",
            server.to("POST", "/api/v1/device-codes").single().body,
        )
        assertEquals("Bearer tok", server.to("GET", "/api/v1/users/me").single().authorization)
        assertTrue(server.to("POST", "/api/v1/device-codes/poll").all { it.body == """{"secret":"secret-1"}""" })
        assertEquals(4_000, currentTime, "two waits of the server's two seconds")
        assertEquals("tok", store.values[TOKEN_KEY])
        assertTrue(connection.signedIn.value)
    }

    @Test
    fun anExpiredCodeEnds() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        server.on("POST", "/api/v1/device-codes") { json(DeviceCode.serializer(), code, HttpStatusCode.Created) }
        server.on("POST", "/api/v1/device-codes/poll") { problem(HttpStatusCode.Gone, "expired") }

        val states = SignIn(server.connection(), "Living Room","client-1").link().toList()

        assertEquals(LinkState.Expired, states.last())
    }

    @Test
    fun aDeniedCodeEnds() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        server.on("POST", "/api/v1/device-codes") { json(DeviceCode.serializer(), code, HttpStatusCode.Created) }
        var polls = 0
        server.on("POST", "/api/v1/device-codes/poll") {
            if (++polls < 2) empty(HttpStatusCode.Accepted) else problem(HttpStatusCode.Forbidden, "denied")
        }

        val states = SignIn(server.connection(), "Living Room","client-1").link().toList()

        assertEquals(LinkState.Denied, states.last())
        assertEquals(2, polls)
    }

    @Test
    fun aThrottledPollWaitsAsTheServerSays() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        server.on("POST", "/api/v1/device-codes") { json(DeviceCode.serializer(), code, HttpStatusCode.Created) }
        var polls = 0
        server.on("POST", "/api/v1/device-codes/poll") {
            if (++polls == 1) {
                problem(HttpStatusCode.TooManyRequests, "throttled", extra = ""","retryAfterSeconds":30""")
            } else {
                json(Session.serializer(), session, HttpStatusCode.Created)
            }
        }
        server.signedInAs(anna)

        SignIn(server.connection(), "Living Room","client-1").link().toList()

        assertEquals(30_000, currentTime)
    }

    @Test
    fun aPasswordSignsInAsAnApp() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        val store = MemoryStore()
        server.on("POST", "/api/v1/sessions") { json(Session.serializer(), session, HttpStatusCode.Created) }
        server.signedInAs(anna)

        assertEquals(anna, SignIn(server.connection(store), "Anna's phone","client-1").withPassword("anna", "pw"))

        assertEquals(
            """{"username":"anna","password":"pw","kind":"app","name":"Anna's phone","clientId":"client-1"}""",
            server.to("POST", "/api/v1/sessions").single().body,
        )
        assertEquals("tok", store.values[TOKEN_KEY])
    }

    @Test
    fun aWrongPasswordIsTheServersProblem() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        server.on("POST", "/api/v1/sessions") { problem(HttpStatusCode.Unauthorized, "invalidCredentials") }

        val error = assertFailsWith<ProblemException> { SignIn(server.connection(), "TV","client-1").withPassword("anna", "nope") }
        assertEquals("invalidCredentials", error.code)
    }

    @Test
    fun everyRequestCarriesTheTokenAndA401SignsOut() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        val store = MemoryStore(mutableMapOf(TOKEN_KEY to "tok"))
        val connection = server.connection(store)
        var revoked = false
        server.on("GET", "/api/v1/users/me") {
            if (revoked) problem(HttpStatusCode.Unauthorized, "unauthorized") else json(CurrentUser.serializer(), anna)
        }

        assertTrue(connection.restore())
        connection.users.getCurrentUser().result()
        assertEquals("Bearer tok", server.seen.last().authorization)

        revoked = true
        assertFailsWith<ProblemException> { connection.users.getCurrentUser().result() }
        assertFalse(connection.signedIn.value)
        assertNull(store.values[TOKEN_KEY])

        server.on("GET", "/api/v1/server") { empty(HttpStatusCode.OK) }
        connection.setup.getServer()
        assertNull(server.seen.last().authorization)
    }

    @Test
    fun signingOutEndsTheSessionAndForgetsTheToken() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        val store = MemoryStore(mutableMapOf(TOKEN_KEY to "tok"))
        val connection = server.connection(store)
        server.on("DELETE", "/api/v1/sessions/current") { empty() }
        connection.restore()

        connection.endSession()

        assertEquals("Bearer tok", server.to("DELETE", "/api/v1/sessions/current").single().authorization)
        assertFalse(connection.signedIn.value)
        assertNull(store.values[TOKEN_KEY])
    }
}
