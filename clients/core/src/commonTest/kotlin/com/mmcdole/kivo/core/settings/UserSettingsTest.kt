package com.mmcdole.kivo.core.settings

import com.mmcdole.kivo.api.ProblemException
import com.mmcdole.kivo.api.models.Session
import com.mmcdole.kivo.api.models.SessionList
import com.mmcdole.kivo.core.FakeServer
import com.mmcdole.kivo.core.MemoryStore
import com.mmcdole.kivo.core.empty
import com.mmcdole.kivo.core.json
import com.mmcdole.kivo.core.problem
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue
import kotlin.time.Instant

class UserSettingsTest {
    @Test
    fun listsWhereTheUserIsSignedInAndSignsOneOut() = runTest {
        val server = FakeServer()
        val at = Instant.fromEpochSeconds(1_800_000_000)
        val sessions = listOf(
            Session("d1", "app", "Anna's iPhone", at, at, current = true),
            Session("d2", "app", "Living Room", at, at, current = false),
        )
        server.on("GET", "/api/v1/sessions") { json(SessionList.serializer(), SessionList(sessions)) }
        server.on("DELETE", "/api/v1/sessions/d2") { empty() }
        server.on("DELETE", "/api/v1/sessions") { empty() }
        val user = UserSettings(server.connection())

        assertEquals(sessions, user.sessions())
        user.signOut("d2")
        user.signOutEverywhereElse()

        assertEquals(1, server.to("DELETE", "/api/v1/sessions/d2").size)
        assertEquals("except=current", server.to("DELETE", "/api/v1/sessions").single().query)
    }

    @Test
    fun aWrongCurrentPasswordKeepsThisSessionSignedIn() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        val connection = server.connection(MemoryStore(mutableMapOf("kivo.token.server-1" to "tok")))
        assertTrue(connection.restore())
        val user = UserSettings(connection)

        for (status in listOf(HttpStatusCode.Unauthorized, HttpStatusCode.BadRequest)) {
            server.on("POST", "/api/v1/users/me/change-password") { problem(status, "wrongPassword") }
            val e = assertFailsWith<ProblemException> { user.changePassword("wrong", "a new password", signOutOthers = true) }
            assertEquals(status.value, e.status)
            assertTrue(connection.signedIn.value)
        }

        server.on("POST", "/api/v1/users/me/change-password") { empty() }
        user.changePassword("right", "a new password", signOutOthers = false)
        assertEquals(
            """{"currentPassword":"right","password":"a new password","signOutOthers":false}""",
            server.to("POST", "/api/v1/users/me/change-password").last().body,
        )
    }
}
