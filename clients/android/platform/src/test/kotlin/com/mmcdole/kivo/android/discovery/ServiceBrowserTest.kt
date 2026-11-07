package com.mmcdole.kivo.android.discovery

import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.runBlocking
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue
import kotlin.time.Duration.Companion.milliseconds

/** A browse that finds [services] at once, resolving each to [resolved] after a short wait, and counts resolves running at once. */
private class FakeBrowser(
    private val services: List<String>,
    private val resolved: Map<String, ResolvedService?>,
) : ServiceBrowser<String> {
    var running = 0
    var mostAtOnce = 0
    val order = mutableListOf<String>()
    var stopped = false

    override fun start(onFound: (String) -> Unit) = services.forEach(onFound)

    override fun stop() {
        stopped = true
    }

    override suspend fun resolve(service: String): ResolvedService? {
        running++
        mostAtOnce = maxOf(mostAtOnce, running)
        order += service
        delay(10)
        running--
        return resolved[service]
    }
}

class ServiceBrowserTest {
    @Test
    fun resolvesOneAtATimeInTheOrderFoundThenStops() = runBlocking {
        val browser = FakeBrowser(
            listOf("Basement", "Printer", "Attic"),
            mapOf(
                "Basement" to ResolvedService(listOf("fe80::1", "192.168.1.20"), 8484, mapOf("id" to "s-1", "tls" to "0", "path" to "")),
                "Printer" to ResolvedService(listOf("192.168.1.9"), 631, emptyMap()),
                "Attic" to ResolvedService(listOf("10.0.0.5"), 8443, mapOf("id" to "s-2", "tls" to "1", "path" to "/kivo")),
            ),
        )

        val found = browser.find(500.milliseconds).toList()

        assertEquals(
            listOf(
                Advertised("s-1", listOf("192.168.1.20", "fe80::1"), 8484, tls = false, path = ""),
                Advertised("s-2", listOf("10.0.0.5"), 8443, tls = true, path = "/kivo"),
            ),
            found,
        )
        assertEquals(listOf("Basement", "Printer", "Attic"), browser.order)
        assertEquals(1, browser.mostAtOnce)
        assertTrue(browser.stopped)
    }

    @Test
    fun aServiceThatDoesntResolveIsSkipped() = runBlocking {
        val browser = FakeBrowser(
            listOf("Gone", "Basement"),
            mapOf("Gone" to null, "Basement" to ResolvedService(listOf("192.168.1.20"), 8484, mapOf("id" to "s-1"))),
        )

        assertEquals(listOf(Advertised("s-1", listOf("192.168.1.20"), 8484, tls = false, path = "")), browser.find(500.milliseconds).toList())
    }
}
