package com.mmcdole.kivo.core.play

import com.mmcdole.kivo.core.FakeServer
import com.mmcdole.kivo.core.empty
import com.mmcdole.kivo.core.tempDataStore
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpStatusCode
import io.ktor.utils.io.ByteChannel
import io.ktor.utils.io.writeFully
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlin.time.Duration.Companion.milliseconds

class BandwidthTest {
    private val path = "/api/v1/bandwidth-test"

    @Test
    fun measuresOnceBeforeTheFirstRemotePlayAndKeepsIt() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        server.on("GET", path) { respond(ByteArray(100_000), HttpStatusCode.OK) }
        val bandwidth = Bandwidth(backgroundScope.tempDataStore())

        val measured = assertNotNull(bandwidth.forAuto(server.connection()))
        assertTrue(measured > 0)
        assertEquals(listOf("size=5000000"), server.to("GET", path).map { it.query })
        assertEquals(measured, bandwidth.last())

        // The next play goes by it without measuring again, and by what the player measured once it has.
        assertEquals(measured, bandwidth.forAuto(server.connection()))
        bandwidth.remember(42_000_000)
        assertEquals(42_000_000, bandwidth.forAuto(server.connection()))
        assertEquals(1, server.to("GET", path).size)
    }

    @Test
    fun aMeasurementStandsForAnHour() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        server.on("GET", path) { respond(ByteArray(100_000), HttpStatusCode.OK) }
        var now = 0L
        val bandwidth = Bandwidth(backgroundScope.tempDataStore(), now = { now })

        bandwidth.remember(42_000_000)
        now += 59 * 60_000
        assertEquals(42_000_000, bandwidth.forAuto(server.connection()))
        assertEquals(0, server.to("GET", path).size)

        now += 2 * 60_000
        assertNull(bandwidth.last())
        val measured = assertNotNull(bandwidth.forAuto(server.connection()))
        assertEquals(1, server.to("GET", path).size)
        assertEquals(measured, bandwidth.last())
    }

    @Test
    fun aLocalClientPlaysWithoutAMeasurement() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        server.on("GET", path) { empty() }
        val bandwidth = Bandwidth(backgroundScope.tempDataStore())

        assertNull(bandwidth.forAuto(server.connection()))
        assertNull(bandwidth.last())
    }

    @Test
    fun countsWhatArrivedByTheDeadline() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        server.on("GET", path) {
            val body = ByteChannel()
            body.writeFully(ByteArray(1000))
            body.flush()
            respond(body, HttpStatusCode.OK)
        }

        val measured = assertNotNull(measureBandwidth(server.connection(), limit = 200.milliseconds))
        assertTrue(measured > 0)
    }
}
