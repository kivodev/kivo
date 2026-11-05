package com.mmcdole.kivo.core.auth

import com.mmcdole.kivo.api.ProblemException
import com.mmcdole.kivo.api.models.PendingDevice
import com.mmcdole.kivo.core.FakeServer
import com.mmcdole.kivo.core.MemoryStore
import com.mmcdole.kivo.core.empty
import com.mmcdole.kivo.core.json
import com.mmcdole.kivo.core.problem
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull
import kotlin.time.Instant

class LinkDeviceTest {
    private val asking = PendingDevice("Living Room", platform = "androidTv", nearby = false, startedAt = Instant.parse("2026-10-17T12:00:00Z"))

    @Test
    fun showsWhoIsAskingThenApprovesWithThisAppsToken() = runTest {
        val server = FakeServer()
        val connection = server.connection(MemoryStore(mutableMapOf("kivo.token.server-1" to "tok")))
        connection.restore()
        server.on("GET", "/api/v1/device-codes/K7Q2M9") { json(PendingDevice.serializer(), asking) }
        server.on("POST", "/api/v1/device-codes/K7Q2M9/approve") { empty() }
        val link = LinkDevice(connection)

        assertEquals(asking, link.pendingDevice("K7Q2M9"))
        link.approve("K7Q2M9")

        assertEquals("Bearer tok", server.to("POST", "/api/v1/device-codes/K7Q2M9/approve").single().authorization)
    }

    @Test
    fun deniesWithThisAppsToken() = runTest {
        val server = FakeServer()
        val connection = server.connection(MemoryStore(mutableMapOf("kivo.token.server-1" to "tok")))
        connection.restore()
        server.on("POST", "/api/v1/device-codes/K7Q2M9/deny") { empty() }

        LinkDevice(connection).deny("K7Q2M9")

        assertEquals("Bearer tok", server.to("POST", "/api/v1/device-codes/K7Q2M9/deny").single().authorization)
    }

    @Test
    fun anUnknownCodeIsANotFoundProblem() = runTest {
        val server = FakeServer()
        server.on("GET", "/api/v1/device-codes/AAAAAA") { problem(HttpStatusCode.NotFound, "notFound") }

        val error = assertFailsWith<ProblemException> { LinkDevice(server.connection()).pendingDevice("AAAAAA") }

        assertEquals(404, error.status)
    }

    @Test
    fun theCodeComesFromTheQrAddressOrWhatWasTyped() {
        assertEquals(LinkCode("K7Q2M9", "s-1"), LinkCode.parseOrNull("http://kivo.local:8484/link?code=K7Q2M9&server=s-1"))
        assertEquals(LinkCode("K7Q2M9", "s-1"), LinkCode.parseOrNull("kivo://link?code=k7q2m9&server=s-1"))
        assertEquals(LinkCode("K7Q2M9", null), LinkCode.parseOrNull("https://kivo.example.com/link?x=1&code=k7q2m9"))
        assertEquals(LinkCode("K7Q2M9", null), LinkCode.parseOrNull("k7q 2m9"))
        assertEquals(LinkCode("K7Q2M9", null), LinkCode.parseOrNull("K7Q-2M9"))
        assertNull(LinkCode.parseOrNull("K7Q2"))
        assertNull(LinkCode.parseOrNull("http://kivo.local/link"))
        assertNull(LinkCode.parseOrNull("http://kivo.local/link?server=s-1"))
    }
}
