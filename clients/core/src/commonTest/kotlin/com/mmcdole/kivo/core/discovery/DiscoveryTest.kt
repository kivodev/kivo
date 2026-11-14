package com.mmcdole.kivo.core.discovery

import com.mmcdole.kivo.api.models.ServerInfo
import com.mmcdole.kivo.core.FakeServer
import com.mmcdole.kivo.core.auth.SavedServer
import com.mmcdole.kivo.core.auth.probeClient
import com.mmcdole.kivo.core.json
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpHeaders
import io.ktor.http.headersOf
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFails
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.time.Duration.Companion.milliseconds

private val info = ServerInfo("server-1", "Basement", "1.0", localUrls = listOf("http://192.168.1.20:8484"))

/** A server per base URL, scheme and port included, answering `GET /api/v1/server` with its ID; any other is unreachable. */
private fun serversAtUrls(vararg bases: Pair<String, String>) = MockEngine { request ->
    val base = "${request.url.protocol.name}://${request.url.host}:${request.url.port}"
    val id = bases.toMap()[base] ?: error("unreachable")
    respond("""{"id":"$id","name":"Server $id","version":"1.0"}""", headers = headersOf(HttpHeaders.ContentType, "application/json"))
}

/** A server per host, answering `GET /api/v1/server` with its ID; any other host is unreachable. */
private fun serversAt(vararg hosts: Pair<String, String>) = MockEngine { request ->
    val id = hosts.toMap()[request.url.host] ?: error("unreachable")
    respond("""{"id":"$id","name":"Server $id","version":"1.0"}""", headers = headersOf(HttpHeaders.ContentType, "application/json"))
}

class DiscoveryTest {
    @Test
    fun confirmsAFoundServerAtTheFirstUrlThatAnswersAsIt() = runTest {
        val engine = serversAt("10.0.0.5" to "someone-else", "192.168.1.20" to "server-1", "192.168.1.30" to "server-2")
        val discovery = Discovery(probeClient(engine, null))

        val found = discovery.confirm(FoundAddress("server-1", listOf("http://10.0.0.9:8484", "http://10.0.0.5:8484", "http://192.168.1.20:8484")))

        assertEquals("http://192.168.1.20:8484" to "server-1", found?.let { it.baseUrl to it.info.id })
        assertNull(discovery.confirm(FoundAddress("server-3", listOf("http://192.168.1.30:8484"))), "a server answering with another ID isn't it")
    }

    @Test
    fun aResolvedServiceIsAUrl() {
        assertEquals("http://192.168.1.20:8484", serviceUrl("192.168.1.20", 8484, tls = false, path = ""))
        assertEquals("https://192.168.1.20:8443/kivo", serviceUrl("192.168.1.20", 8443, tls = true, path = "/kivo/"))
        assertEquals("http://[fd00::1]:8484", serviceUrl("fd00::1%wlan0", 8484, tls = false, path = ""))
    }

    @Test
    fun aTypedAddressWithItsSchemeIsThatUrlAlone() {
        assertEquals(listOf("https://kivo.example.com/kivo"), addressCandidates("https://kivo.example.com/kivo/"))
        assertEquals(listOf("http://192.168.1.20:8484"), addressCandidates("http://192.168.1.20"))
        assertEquals(listOf("http://[fd00::1]:80"), addressCandidates("http://[fd00::1]:80"))
    }

    @Test
    fun aTypedAddressWithoutItsSchemeIsTriedOverHttpsFirst() {
        assertEquals(
            listOf("https://kivo.example.com", "http://kivo.example.com:8484", "http://kivo.example.com"),
            addressCandidates("kivo.example.com"),
        )
        assertEquals(listOf("https://nas:9000", "http://nas:9000"), addressCandidates(" nas:9000/ "))
        assertEquals(listOf("https://[fd00::1]", "http://[fd00::1]:8484", "http://[fd00::1]"), addressCandidates("[fd00::1]"))
        assertEquals(listOf("https://nas/kivo", "http://nas:8484/kivo", "http://nas/kivo"), addressCandidates("nas/kivo"))
    }

    @Test
    fun namesTheServerAtATypedAddress() = runTest {
        val server = FakeServer()
        server.on("GET", "/api/v1/server") { json(ServerInfo.serializer(), info) }

        val found = Discovery(probeClient(server.engine, null)).serverAt("kivo.test")

        assertEquals(FoundServer("https://kivo.test", info), found)
    }

    @Test
    fun aTypedAddressOpensOverHttpsWhenItAnswersThere() = runTest {
        val engine = serversAtUrls("https://kivo.example.com:443" to "server-1", "http://kivo.example.com:8484" to "server-1")

        assertEquals("https://kivo.example.com", Discovery(probeClient(engine, null)).serverAt("kivo.example.com").baseUrl)
    }

    @Test
    fun aTypedAddressOpensOverPlainHttpWhenNothingAnswersOverHttps() = runTest {
        val discovery = Discovery(probeClient(serversAtUrls("http://192.168.1.20:8484" to "server-1"), null))

        assertEquals("http://192.168.1.20:8484", discovery.serverAt("192.168.1.20").baseUrl)
        assertEquals("http://192.168.1.20:8484", discovery.serverAt("192.168.1.20:8484").baseUrl)
    }

    @Test
    fun aTypedAddressWhereNothingAnswersFails() = runTest {
        val discovery = Discovery(probeClient(serversAtUrls(), null))

        assertFails { discovery.serverAt("kivo.example.com") }
    }

    @Test
    fun aTypedAddressWhereNothingAnswersInTimeFailsAsAnyOtherFailure() = runTest {
        val silent = MockEngine { awaitCancellation() }
        val discovery = Discovery(probeClient(silent, null), timeout = 50.milliseconds)

        val failure = assertFails { discovery.serverAt("kivo.example.com") }

        assertFalse(failure is CancellationException, "a caller would take it for its own cancellation")
    }

    @Test
    fun reachesASavedServerAtTheFirstAddressThatIsStillIt() = runTest {
        val engine = serversAt("192.168.1.20" to "someone-else", "kivo.example.com" to "server-1")
        val saved = SavedServer(info.copy(publicUrl = "https://kivo.example.com"), "http://10.0.0.5:8484")
        assertEquals(listOf("http://192.168.1.20:8484", "http://10.0.0.5:8484", "https://kivo.example.com"), saved.addresses)

        val discovery = Discovery(probeClient(engine, null))

        assertEquals("https://kivo.example.com", discovery.reach(saved)?.baseUrl)
        assertNull(discovery.reach(saved.copy(addresses = listOf("http://192.168.1.20:8484"))))
    }
}
