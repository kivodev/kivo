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
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

private val info = ServerInfo("server-1", "Basement", "1.0", localUrls = listOf("http://192.168.1.20:8484"))

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
    fun aTypedAddressIsAUrl() {
        assertEquals("http://192.168.1.20:8484", normalizeAddress("192.168.1.20"))
        assertEquals("http://nas:9000", normalizeAddress(" nas:9000/ "))
        assertEquals("https://kivo.example.com/kivo", normalizeAddress("https://kivo.example.com/kivo/"))
        assertEquals("http://[fd00::1]:8484", normalizeAddress("[fd00::1]"))
        assertEquals("http://[fd00::1]:80", normalizeAddress("http://[fd00::1]:80"))
    }

    @Test
    fun namesTheServerAtATypedAddress() = runTest {
        val server = FakeServer()
        server.on("GET", "/api/v1/server") { json(ServerInfo.serializer(), info) }

        val found = Discovery(probeClient(server.engine, null)).serverAt("kivo.test")

        assertEquals(FoundServer("http://kivo.test:8484", info), found)
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
