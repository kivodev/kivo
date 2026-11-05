package com.mmcdole.kivo.core.auth

import com.mmcdole.kivo.api.models.ServerInfo
import io.ktor.http.URLBuilder
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.delay
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.currentTime
import kotlinx.coroutines.test.runTest
import kotlin.coroutines.EmptyCoroutineContext
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlin.time.Duration
import kotlin.time.Duration.Companion.milliseconds
import kotlin.time.Duration.Companion.seconds

private const val LOCAL = "http://192.168.1.20:8484"
private const val OTHER_LOCAL = "http://10.0.0.5:8484"
private const val PUBLIC = "https://kivo.example.com"

/**
 * A network the tests set: what answers at each address, as which server,
 * after how long; an address it doesn't list never answers. It counts the
 * probes each address gets.
 */
private class Network {
    val answers = mutableMapOf<String, Pair<String, Duration>>()
    val probes = mutableMapOf<String, Int>()

    fun answer(address: String, after: Duration, id: String = "server-1") {
        answers[address] = id to after
    }

    fun gone(address: String) {
        answers.remove(address)
    }

    suspend fun probe(address: String): ServerInfo? {
        probes[address] = (probes[address] ?: 0) + 1
        val (id, after) = answers[address] ?: awaitCancellation()
        delay(after)
        return ServerInfo(id, "Basement", "1.0")
    }
}

private suspend fun pick(network: Network, addresses: List<String>, prefer: String? = null) =
    pickAddress("server-1", addresses, prefer, PICK_GRACE, PICK_WAIT, network::probe)?.first

private fun TestScope.address(network: Network, initial: String, addresses: List<String>) =
    ServerAddress("server-1", initial, addresses, network::probe, time = testScheduler.timeSource, context = EmptyCoroutineContext)

class ServerAddressTest {
    @Test
    fun aLocalAddressWinsWhenBothAnswerEvenAfterThePublicOne() = runTest {
        val network = Network()
        network.answer(PUBLIC, 50.milliseconds)
        network.answer(LOCAL, 300.milliseconds)

        assertEquals(LOCAL, pick(network, listOf(LOCAL, PUBLIC)))
        assertEquals(300, currentTime, "a local answer ends the pick at once")
    }

    @Test
    fun aLocalAddressAnsweringFirstEndsThePickAtOnce() = runTest {
        val network = Network()
        network.answer(LOCAL, 20.milliseconds)
        network.answer(PUBLIC, 100.milliseconds)

        assertEquals(LOCAL, pick(network, listOf(LOCAL, PUBLIC)))
        assertEquals(20, currentTime)
    }

    @Test
    fun thePublicAddressWinsWhenNoLocalOneAnswersWithinTheGrace() = runTest {
        val network = Network()
        network.answer(PUBLIC, 80.milliseconds)
        network.answer(LOCAL, 2.seconds)

        assertEquals(PUBLIC, pick(network, listOf(LOCAL, PUBLIC)))
        assertEquals(80 + PICK_GRACE.inWholeMilliseconds, currentTime, "it waits the grace, not every address's wait")

        network.gone(LOCAL)
        assertEquals(PUBLIC, pick(network, listOf(LOCAL, PUBLIC)), "nor for a local address that never answers")
    }

    @Test
    fun anAddressAnsweringAsAnotherServerDoesntCount() = runTest {
        val network = Network()
        network.answer(LOCAL, 10.milliseconds, id = "someone-else")
        network.answer(PUBLIC, 50.milliseconds)

        assertEquals(PUBLIC, pick(network, listOf(LOCAL, PUBLIC)))
        network.gone(PUBLIC)
        val before = currentTime
        assertNull(pick(network, listOf(LOCAL, PUBLIC)))
        assertEquals(PICK_WAIT.inWholeMilliseconds, currentTime - before, "none answering as it takes the wait")
    }

    @Test
    fun theAddressInUseStaysWhileItAnswersAsWellAsAnother() = runTest {
        val network = Network()
        network.answer(LOCAL, 30.milliseconds)
        network.answer(OTHER_LOCAL, 200.milliseconds)

        assertEquals(OTHER_LOCAL, pick(network, listOf(LOCAL, OTHER_LOCAL), prefer = OTHER_LOCAL))
        assertEquals(LOCAL, pick(network, listOf(LOCAL, OTHER_LOCAL)), "with none in use, the first to answer")
    }

    @Test
    fun aNetworkChangeMovesOffAnAddressThatNoLongerAnswersAndBackHome() = runTest {
        val network = Network()
        network.answer(PUBLIC, 50.milliseconds)
        val address = address(network, LOCAL, listOf(LOCAL, PUBLIC))

        assertTrue(address.repick(AddressChange.Network), "away from home: the public address")
        assertEquals(PUBLIC, address.current.value)

        network.answer(LOCAL, 10.milliseconds)
        assertTrue(address.repick(AddressChange.Network), "home again")
        assertEquals(LOCAL, address.current.value)

        network.gone(LOCAL)
        network.gone(PUBLIC)
        assertFalse(address.repick(AddressChange.Network), "with nothing answering it stays")
        assertEquals(LOCAL, address.current.value)
    }

    @Test
    fun aRequestWithoutAnAnswerPicksAgainButNotInABurst() = runTest {
        val network = Network()
        network.answer(PUBLIC, 50.milliseconds)
        val address = address(network, LOCAL, listOf(LOCAL, PUBLIC))
        delay(PICK_REST)

        // Many reads fail at once: they share one pick.
        val burst = List(5) { async { address.repick(AddressChange.NoAnswer) } }.map { it.await() }

        assertEquals(List(5) { true }, burst)
        assertEquals(PUBLIC, address.current.value)
        assertEquals(1, network.probes[PUBLIC])

        // Another soon after is skipped; a network change never is.
        network.answer(LOCAL, 10.milliseconds)
        assertFalse(address.repick(AddressChange.NoAnswer))
        assertFalse(address.repick(AddressChange.Foreground))
        assertEquals(1, network.probes[PUBLIC])
        delay(PICK_REST)
        assertTrue(address.repick(AddressChange.Foreground))
        assertEquals(LOCAL, address.current.value)
    }

    @Test
    fun aPickRightAfterTheSessionOpenedIsSkipped() = runTest {
        val network = Network()
        network.answer(PUBLIC, 50.milliseconds)
        val address = address(network, LOCAL, listOf(LOCAL, PUBLIC))

        assertFalse(address.repick(AddressChange.Foreground), "the session just picked")
        assertEquals(emptyMap(), network.probes)
    }

    @Test
    fun onAPublicAddressALocalOneIsLookedForAlone() = runTest {
        val network = Network()
        network.answer(PUBLIC, 50.milliseconds)
        val address = address(network, PUBLIC, listOf(LOCAL, PUBLIC))

        assertFalse(address.preferLocal())
        assertNull(network.probes[PUBLIC], "only the local addresses are asked")

        network.answer(LOCAL, 10.milliseconds)
        assertTrue(address.preferLocal())
        assertEquals(LOCAL, address.current.value)
        assertFalse(address.preferLocal(), "on a local address there's nothing to look for")
        assertEquals(2, network.probes[LOCAL])
    }

    @Test
    fun aStreamPlayingOnThePublicAddressStaysThereUntilItStops() = runTest {
        val network = Network()
        network.answer(PUBLIC, 50.milliseconds)
        val address = address(network, PUBLIC, listOf(LOCAL, PUBLIC))
        val stream = Any()
        address.streaming(stream, true)
        delay(PICK_REST)

        network.answer(LOCAL, 10.milliseconds)
        assertFalse(address.preferLocal())
        assertFalse(address.repick(AddressChange.Network), "the public address still answers")
        assertEquals(PUBLIC, address.current.value)

        address.streaming(stream, false)
        assertTrue(address.preferLocal(), "once it stops")
        assertEquals(LOCAL, address.current.value)
    }

    @Test
    fun aStreamPlayingMovesOffAnAddressThatNoLongerAnswers() = runTest {
        val network = Network()
        network.answer(LOCAL, 10.milliseconds)
        val address = address(network, PUBLIC, listOf(LOCAL, PUBLIC))
        address.streaming(Any(), true)

        assertTrue(address.repick(AddressChange.Network))
        assertEquals(LOCAL, address.current.value)
    }

    @Test
    fun aUrlForAnyKnownAddressGoesToTheCurrentOne() = runTest {
        val network = Network()
        network.answer(PUBLIC, 50.milliseconds)
        val address = address(network, LOCAL, listOf(LOCAL, "https://kivo.example.com/kivo"))
        address.remember(listOf(LOCAL, PUBLIC))

        fun rebased(url: String) = URLBuilder(url).also(address::rebase).buildString()

        assertEquals("$LOCAL/api/v1/works/w1?x=1", rebased("$LOCAL/api/v1/works/w1?x=1"), "already there")
        address.repick(AddressChange.Network)
        assertEquals("$PUBLIC/api/v1/works/w1?x=1", rebased("$LOCAL/api/v1/works/w1?x=1"))
        assertEquals("$PUBLIC/api/v1/images/i?width=480", rebased("https://kivo.example.com/kivo/api/v1/images/i?width=480"), "a base path is swapped")
        assertEquals("wss://kivo.example.com/api/v1/events", rebased("ws://192.168.1.20:8484/api/v1/events"), "the socket's scheme follows")
        assertEquals("http://elsewhere.test/api/v1/x", rebased("http://elsewhere.test/api/v1/x"), "a URL of no known address stays")
        assertEquals("http://192.168.1.20:9999/x", rebased("http://192.168.1.20:9999/x"), "nor one on another port")
    }

    @Test
    fun localAddressesAreTheLocalNetworksAndPrivateOnes() {
        val local = listOf(
            "http://192.168.1.20:8484", "http://10.1.2.3", "http://172.20.0.1", "http://169.254.3.4", "http://127.0.0.1:8484",
            "http://100.101.102.103", "http://[fd00::1]:8484", "http://[fe80::1]", "http://nas:8484", "http://media.local:8484",
            "https://media.tail1234.ts.net", "http://kivo.home.arpa",
        )
        val public = listOf("https://kivo.example.com", "http://8.8.8.8", "http://172.32.0.1", "http://100.128.0.1", "http://[2001:db8::1]")

        assertEquals(local, local.filter(::isLocal))
        assertEquals(emptyList(), public.filter(::isLocal))
    }
}
