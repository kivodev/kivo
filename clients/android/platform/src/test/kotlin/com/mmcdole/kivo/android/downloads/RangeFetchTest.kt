package com.mmcdole.kivo.android.downloads

import com.sun.net.httpserver.HttpExchange
import com.sun.net.httpserver.HttpServer
import kotlinx.coroutines.runBlocking
import java.io.File
import java.net.InetSocketAddress
import java.util.concurrent.CopyOnWriteArrayList
import kotlin.io.path.createTempDirectory
import kotlin.test.AfterTest
import kotlin.test.Test
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertIs
import kotlin.test.assertTrue

/** The transfer's fetch against a real HTTP server: resuming by range, starting over, drops and refusals. */
class RangeFetchTest {
    private val body = ByteArray(300_000) { (it % 251).toByte() }
    private val dir = createTempDirectory("fetch").toFile()
    private val destination = File(dir, "d1/part-0.mp4")
    private val requests = CopyOnWriteArrayList<Map<String, String?>>()

    /** How the server answers: honouring ranges, with [etag], and cutting the body short after [cutAt] bytes once. */
    private var etag = "\"v1\""
    private var honourRanges = true
    private var cutAt: Int? = null
    private var status: Int? = null

    private val server = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0).apply {
        createContext("/file") { exchange -> answer(exchange) }
        start()
    }
    private val url = "http://127.0.0.1:${server.address.port}/file"

    private fun answer(exchange: HttpExchange) {
        val range = exchange.requestHeaders.getFirst("Range")
        val ifRange = exchange.requestHeaders.getFirst("If-Range")
        requests += mapOf("Range" to range, "If-Range" to ifRange, "Authorization" to exchange.requestHeaders.getFirst("Authorization"))
        status?.let {
            exchange.sendResponseHeaders(it, -1)
            exchange.close()
            return
        }
        val start = range?.takeIf { honourRanges && (ifRange == null || ifRange == etag) }?.removePrefix("bytes=")?.substringBefore('-')?.toInt()
        exchange.responseHeaders.add("ETag", etag)
        val from = start ?: 0
        if (start != null) exchange.responseHeaders.add("Content-Range", "bytes $from-${body.size - 1}/${body.size}")
        exchange.sendResponseHeaders(if (start != null) 206 else 200, (body.size - from).toLong())
        val cut = cutAt
        exchange.responseBody.use { out ->
            if (cut != null) {
                cutAt = null
                out.write(body, from, cut)
                out.flush()
                // The connection drops partway through.
                throw java.io.IOException("dropped")
            }
            out.write(body, from, body.size - from)
        }
    }

    private fun fetch(authorization: String? = "Bearer t") = runBlocking { RangeFetch(url, authorization, destination) { _, _ -> }.run() }

    @AfterTest
    fun stop() {
        server.stop(0)
        dir.deleteRecursively()
    }

    @Test
    fun aWholeFileArrivesAtItsDestinationWithTheToken() {
        assertEquals(FetchEnd.Whole, fetch())
        assertContentEquals(body, destination.readBytes())
        assertFalse(RangeFetch.partOf(destination).exists())
        assertEquals("Bearer t", requests.single()["Authorization"])
    }

    @Test
    fun aFetchGoesOnFromWhatAnEarlierOneLeft() {
        val part = RangeFetch.partOf(destination)
        part.parentFile.mkdirs()
        part.writeBytes(body.copyOf(100_000))
        File(destination.path + ".etag").writeText(etag)
        assertEquals(FetchEnd.Whole, fetch())
        assertEquals("bytes=100000-", requests.single()["Range"])
        assertEquals(etag, requests.single()["If-Range"])
        assertContentEquals(body, destination.readBytes())
    }

    @Test
    fun aFileThatChangedOnTheServerStartsOver() {
        val part = RangeFetch.partOf(destination)
        part.parentFile.mkdirs()
        part.writeBytes(ByteArray(100_000) { 7 })
        File(destination.path + ".etag").writeText("\"old\"")
        assertEquals(FetchEnd.Whole, fetch())
        assertContentEquals(body, destination.readBytes())
    }

    @Test
    fun aServerThatIgnoresRangesSendsItAllAgain() {
        honourRanges = false
        val part = RangeFetch.partOf(destination)
        part.parentFile.mkdirs()
        part.writeBytes(body.copyOf(50_000))
        assertEquals(FetchEnd.Whole, fetch())
        assertContentEquals(body, destination.readBytes())
    }

    @Test
    fun aDropKeepsWhatArrivedAndTheRetryResumes() {
        cutAt = 120_000
        val first = fetch()
        assertIs<FetchEnd.Stopped>(first)
        assertEquals(0, first.status)
        assertFalse(destination.exists())
        val kept = RangeFetch.partOf(destination).length()
        assertTrue(kept in 1..120_000, "kept $kept bytes")
        assertEquals(FetchEnd.Whole, fetch())
        assertEquals("bytes=$kept-", requests.last()["Range"])
        assertContentEquals(body, destination.readBytes())
    }

    @Test
    fun aRefusalSaysItsStatusAndKeepsNothingNew() {
        status = 416
        assertEquals(FetchEnd.Stopped(416, ""), fetch().let { if (it is FetchEnd.Stopped) it.copy(message = "") else it })
        assertFalse(destination.exists())
        RangeFetch.discard(destination)
        assertFalse(RangeFetch.partOf(destination).exists())
    }
}
