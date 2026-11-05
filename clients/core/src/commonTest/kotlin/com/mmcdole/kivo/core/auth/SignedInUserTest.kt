package com.mmcdole.kivo.core.auth

import androidx.datastore.preferences.core.stringPreferencesKey
import com.mmcdole.kivo.api.ProblemException
import com.mmcdole.kivo.api.models.CurrentUser
import com.mmcdole.kivo.api.models.PlaybackPreferences
import com.mmcdole.kivo.api.models.UserLimits
import com.mmcdole.kivo.api.models.Session
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.FakeServer
import com.mmcdole.kivo.core.MemoryPreferences
import com.mmcdole.kivo.core.MemoryStore
import com.mmcdole.kivo.core.empty
import com.mmcdole.kivo.core.json
import com.mmcdole.kivo.core.live.Invalidation
import com.mmcdole.kivo.core.problem
import com.mmcdole.kivo.core.settings.UserSettings
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestCoroutineScheduler
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlin.time.Instant

private val playback = PlaybackPreferences("forced", true, true, 1.0)
private val anna = CurrentUser("u1", "anna", admin = false, limits = UserLimits(), playback = playback)
private val at = Instant.fromEpochSeconds(1_800_000_000)
private val session = Session("d1", "app", "Anna's phone", at, at, current = true, token = "tok")

private const val TOKEN_KEY = "kivo.token.server-1"
private val USER_KEY = stringPreferencesKey("kivo.user.server-1")

/** The server signs [user] in by password and says who it is, until [answering] is false. */
private class Server(scheduler: TestCoroutineScheduler) {
    val fake = FakeServer(StandardTestDispatcher(scheduler))
    var user = anna
    var answering = true

    init {
        fake.on("POST", "/api/v1/sessions") { json(Session.serializer(), session, HttpStatusCode.Created) }
        fake.on("GET", "/api/v1/users/me") {
            if (!answering) problem(HttpStatusCode.BadGateway, "unreachable") else json(CurrentUser.serializer(), user)
        }
        fake.on("DELETE", "/api/v1/sessions/current") { empty() }
    }
}

class SignedInUserTest {
    @Test
    fun signingInKeepsWhoItIsForBesideTheToken() = runTest {
        val server = Server(testScheduler)
        val store = MemoryStore()
        val preferences = MemoryPreferences()
        val connection = server.fake.connection(store, preferences = preferences)

        SignIn(connection, "Anna's phone","client-1").withPassword("anna", "pw")

        assertEquals(SignedInUser("u1", "anna", admin = false), connection.user.value)
        assertEquals("A", connection.user.value?.initial)
        assertEquals("tok", store.values[TOKEN_KEY])
        assertEquals("""{"id":"u1","username":"anna","admin":false}""", preferences.data.first()[USER_KEY])
    }

    @Test
    fun aColdStartWithoutTheServerShowsTheStoredUser() = runTest {
        val server = Server(testScheduler)
        val store = MemoryStore()
        val preferences = MemoryPreferences()
        SignIn(server.fake.connection(store, preferences = preferences), "Anna's phone", "client-1").withPassword("anna", "pw")
        server.answering = false
        server.fake.seen.clear()

        val launched = server.fake.connection(store, preferences = preferences)

        assertTrue(launched.restore())
        assertEquals(SignedInUser("u1", "anna", admin = false), launched.user.value)
        assertTrue(server.fake.seen.isEmpty(), "nothing asked the server")
        assertFailsWith<ProblemException> { UserSettings(launched).current() }
        assertEquals(SignedInUser("u1", "anna", admin = false), launched.user.value, "a failed read keeps it")
    }

    @Test
    fun aReconnectReadsTheUserAgainAndKeepsTheChange() = runTest {
        val server = Server(testScheduler)
        val preferences = MemoryPreferences()
        val connection = server.fake.connection(preferences = preferences)
        SignIn(connection, "Anna's phone","client-1").withPassword("anna", "pw")
        val invalidations = MutableSharedFlow<Invalidation>()
        val following = launch { UserSettings(connection).refreshOn(invalidations) }
        runCurrent()

        server.answering = false
        invalidations.emit(Invalidation.Everything)
        runCurrent()
        assertEquals("anna", connection.user.value?.username, "an unanswered read keeps the stored user")

        server.answering = true
        server.user = anna.copy(username = "Annabel", admin = true)
        invalidations.emit(Invalidation.WorkChanged("w1"))
        runCurrent()
        assertEquals("anna", connection.user.value?.username, "only a reconnect reads it again")

        invalidations.emit(Invalidation.Everything)
        runCurrent()
        assertEquals(SignedInUser("u1", "Annabel", admin = true), connection.user.value)
        assertEquals("""{"id":"u1","username":"Annabel","admin":true}""", preferences.data.first()[USER_KEY])
        following.cancel()
    }

    @Test
    fun signingOutForgetsTheTokenAndTheUser() = runTest {
        val server = Server(testScheduler)
        val store = MemoryStore()
        val preferences = MemoryPreferences()
        val connection = server.fake.connection(store, preferences = preferences)
        val signIn = SignIn(connection, "Anna's phone","client-1")
        signIn.withPassword("anna", "pw")

        connection.endSession()

        assertNull(connection.user.value)
        assertTrue(store.values.isEmpty())
        assertTrue(preferences.data.first().asMap().isEmpty())
    }

    @Test
    fun aRevokedSessionForgetsTheTokenAndTheUser() = runTest {
        val server = Server(testScheduler)
        val store = MemoryStore()
        val preferences = MemoryPreferences()
        val connection = server.fake.connection(store, preferences = preferences)
        SignIn(connection, "Anna's phone","client-1").withPassword("anna", "pw")
        server.fake.on("GET", "/api/v1/users/me") { problem(HttpStatusCode.Unauthorized, "unauthorized") }

        assertFailsWith<ProblemException> { connection.users.getCurrentUser().result() }

        assertNull(connection.user.value)
        assertTrue(store.values.isEmpty())
        assertTrue(preferences.data.first().asMap().isEmpty())
        val relaunched = server.fake.connection(store, preferences = preferences)
        relaunched.restore()
        assertNull(relaunched.user.value)
    }

    @Test
    fun anAnswerAfterSigningOutKeepsNothing() = runTest {
        val server = Server(testScheduler)
        val preferences = MemoryPreferences()
        val connection = server.fake.connection(preferences = preferences)
        SignIn(connection, "Anna's phone","client-1").withPassword("anna", "pw")
        connection.signOut()

        connection.keep(anna)

        assertNull(connection.user.value)
        assertTrue(preferences.data.first().asMap().isEmpty())
    }
}
