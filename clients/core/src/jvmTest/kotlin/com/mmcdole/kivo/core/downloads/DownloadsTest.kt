package com.mmcdole.kivo.core.downloads

import app.cash.sqldelight.db.SqlDriver
import app.cash.sqldelight.driver.jdbc.sqlite.JdbcSqliteDriver
import com.mmcdole.kivo.api.models.Capabilities
import com.mmcdole.kivo.api.models.FileCapabilities
import com.mmcdole.kivo.api.models.SubtitleEntry
import com.mmcdole.kivo.api.models.CurrentUser
import com.mmcdole.kivo.api.models.DownloadAvailability
import com.mmcdole.kivo.api.models.DownloadCheckResult
import com.mmcdole.kivo.api.models.DownloadEstimate
import com.mmcdole.kivo.api.models.DownloadEstimateList
import com.mmcdole.kivo.api.models.DownloadList
import com.mmcdole.kivo.api.models.DownloadOption
import com.mmcdole.kivo.api.models.DownloadPart
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.PlayTrack
import com.mmcdole.kivo.api.models.ServerInfo
import com.mmcdole.kivo.api.models.PlaybackPreferences
import com.mmcdole.kivo.api.models.TrackReason
import com.mmcdole.kivo.api.models.TrackTranscode
import com.mmcdole.kivo.api.models.UserLimits
import com.mmcdole.kivo.api.models.UserState
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.core.FakeServer
import com.mmcdole.kivo.core.auth.AddressChange
import com.mmcdole.kivo.core.auth.NetworkEpoch
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.player.PlayTracks
import com.mmcdole.kivo.core.proofOf
import com.mmcdole.kivo.core.display.Display
import com.mmcdole.kivo.core.downloads.db.DownloadDatabase
import com.mmcdole.kivo.core.empty
import com.mmcdole.kivo.core.json
import com.mmcdole.kivo.core.live.Invalidation
import com.mmcdole.kivo.core.player.TrackShow
import com.mmcdole.kivo.core.problem
import com.mmcdole.kivo.core.settings.DeviceSettings
import com.mmcdole.kivo.core.settings.DownloadQuality
import io.ktor.client.request.HttpRequestData
import io.ktor.client.engine.mock.MockRequestHandleScope
import io.ktor.client.engine.mock.respond
import io.ktor.client.request.get
import io.ktor.client.request.header
import io.ktor.client.statement.bodyAsBytes
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
import io.ktor.http.content.OutgoingContent
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.currentTime
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.flow.first
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.long
import java.io.File
import java.io.IOException
import java.security.MessageDigest
import kotlin.io.path.createTempDirectory
import kotlin.test.Test
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertIs
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue
import okio.FileSystem
import kotlin.time.Duration.Companion.days
import kotlin.time.Instant
import com.mmcdole.kivo.api.models.Download as ServerDownload
import com.mmcdole.kivo.api.models.DownloadDelivery
import com.mmcdole.kivo.api.models.PlayDelivery
import com.mmcdole.kivo.api.models.SubtitleMode
import com.mmcdole.kivo.api.models.TrackDelivery
import com.mmcdole.kivo.api.models.TrackType
import com.mmcdole.kivo.core.SERVER_KEY

// The download store and its state machine against Ktor's mock engine, a
// transport the tests drive, and a real SQLite store and folder: transfers
// resume, the server's restarts and forgetting are recovered from, the
// outbox's latest position wins, and a download the user may no longer have
// goes, while one they may plays however long the server has been away.

private val caps = Capabilities(
    video = emptyList(),
    audio = emptyList(),
    subtitles = listOf(SubtitleEntry("webvtt", listOf("stream", "external")), SubtitleEntry("ass", listOf("file", "external")), SubtitleEntry("subrip", listOf("file"))),
    file = FileCapabilities(containers = listOf("mp4")),
)

private val movie = "the movie's bytes, all of them".encodeToByteArray()
private val vtt = "WEBVTT\n\n00:00.000 --> 00:01.000\nHello\n".encodeToByteArray()

private fun sha(bytes: ByteArray) = MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it) }

private const val WORK = """{"id":"w1","kind":"movie","title":"Heat","matchState":"matched","available":true,
    "images":{"poster":{"id":"img1"}},"genres":[],"studios":[],"credits":[],"externalIds":[],"versions":[],"extras":[],"collections":[],
    "userState":{"played":false,"positionMs":60000}}"""

/** The server's download: a ready conversion of [movie] with a WebVTT subtitle, unless [state] says otherwise. */
private fun download(
    id: String = "d1",
    state: String = "ready",
    progress: Double? = null,
    remaining: Int? = null,
    delivery: DownloadDelivery = DownloadDelivery.Conversion,
) = ServerDownload(
    id = id, workId = "w1", versionId = "v1", quality = "high", state = state, delivery = delivery,
    durationMs = 7_200_000, sizeBytes = movie.size.toLong(), sizeExact = state == "ready" || delivery == DownloadDelivery.File,
    parts = listOf(
        DownloadPart(
            durationMs = 7_200_000, url = "downloads/$id/files/0", contentType = "video/mp4",
            sizeBytes = movie.size.toLong().takeIf { state == "ready" }, etag = "\"${sha(movie)}\"".takeIf { state == "ready" },
            sha256 = sha(movie).takeIf { state == "ready" && delivery == DownloadDelivery.Conversion },
        ),
    ),
    tracks = listOf(
        PlayTrack("t3", TrackType.Subtitle, selected = true, delivery = TrackDelivery.External, codec = "webvtt", url = "downloads/$id/subtitles/t3.vtt"),
        PlayTrack("off", TrackType.Subtitle, delivery = TrackDelivery.None),
    ),
    chapters = emptyList(),
    markers = listOf(com.mmcdole.kivo.api.models.Marker("credits", 6_600_000, 7_200_000, "chapter", final = true)),
    progress = progress, remainingMs = remaining,
)

/** A transport the tests drive: it records what's started, and the test says what arrives. */
private const val LOCAL = "http://192.168.1.20:8484"
private const val PUBLIC = "https://kivo.example.com"

/** A session token, as the server issues them, which a probe's proof is keyed by. */
private const val TOKEN = "kivo_s_AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"

private class FakeTransport : DownloadTransport {
    val started = mutableListOf<TransferRequest>()
    val running = mutableSetOf<String>()
    val stopped = mutableListOf<String>()
    val cancelled = mutableListOf<String>()

    override fun start(request: TransferRequest) {
        started += request
        running += request.id
    }

    override fun stop(id: String) {
        stopped += id
        running -= id
    }

    override fun cancel(id: String) {
        cancelled += id
        running -= id
    }

    override fun running() = running.toList()
}

private class Disk(override val folder: String, var free: Long = 100L * 1024 * 1024 * 1024) : DownloadDisk {
    override fun freeBytes() = free

    override fun sha256(path: String) = sha(File(path).readBytes())
}

private class Harness(
    val scope: TestScope,
    dir: File = createTempDirectory("kivo-downloads").toFile(),
    driver: SqlDriver? = null,
    at: String = "https://kivo.test",
    addresses: List<String> = listOf(at),
    val network: NetworkEpoch = NetworkEpoch(),
) {
    val server = FakeServer(StandardTestDispatcher(scope.testScheduler))
    val connection: ServerConnection = server.connection(at = at, addresses = addresses, network = network)
    val transport = FakeTransport()
    val disk = Disk(File(dir, "downloads").path)
    val driver = driver ?: JdbcSqliteDriver("jdbc:sqlite:${File(dir, "downloads.db").path}").also { DownloadDatabase.Schema.create(it) }
    var settings = DeviceSettings()
    var clock = 1_000_000_000_000L
    // On the test's scheduler, so advanceUntilIdle runs the manager's work,
    // which backgroundScope's it wouldn't wait for.
    val downloads = Downloads(
        DownloadDatabase(this.driver), disk, transport, { settings }, CoroutineScope(StandardTestDispatcher(scope.testScheduler) + SupervisorJob()),
        now = { clock }, background = StandardTestDispatcher(scope.testScheduler),
    )

    /** What the server holds: the download it answers with, and the files it serves. */
    var remote = download()

    /** Whether the server still has it, in its list of the device's downloads. */
    var listed = true
    val served = mutableMapOf("downloads/d1/files/0" to movie, "downloads/d1/subtitles/t3.vtt" to vtt, "images/img1?width=480" to "art".encodeToByteArray())

    init {
        server.on("POST", "/api/v1/downloads/estimate") {
            json(
                DownloadEstimateList.serializer(),
                DownloadEstimateList(
                    listOf(
                        DownloadEstimate(
                            "w1", playable = true,
                            options = listOf(DownloadOption("high", movie.size.toLong(), false, DownloadDelivery.Conversion), DownloadOption("original", 20_000_000_000, true, DownloadDelivery.File)),
                        ),
                    ),
                ),
            )
        }
        server.on("GET", "/api/v1/users/me") { json(CurrentUser.serializer(), CurrentUser("u1", "anna", false, UserLimits(), PlaybackPreferences(SubtitleMode.Forced, true, true, 1.0))) }
        server.on("GET", "/api/v1/works/w1") { respond(WORK, HttpStatusCode.OK, headersOf(HttpHeaders.ContentType, "application/json")) }
        server.on("POST", "/api/v1/downloads") { json(ServerDownload.serializer(), remote, HttpStatusCode.Created) }
        server.on("GET", "/api/v1/downloads/d1") { json(ServerDownload.serializer(), remote) }
        server.on("GET", "/api/v1/downloads/d2") { json(ServerDownload.serializer(), remote) }
        server.on("GET", "/api/v1/downloads") { json(DownloadList.serializer(), DownloadList(listOfNotNull(remote.takeIf { listed }))) }
        server.on("POST", "/api/v1/downloads/d1/retry") { json(ServerDownload.serializer(), remote) }
        server.on("DELETE", "/api/v1/downloads/d1") { empty() }
        server.on("DELETE", "/api/v1/downloads/d2") { empty() }
        for ((url, bytes) in served) server.on("GET", "/api/v1/" + url.substringBefore('?')) { serve(it, bytes) }
    }

    /** Whether the app's reads are getting no answer, and what returns once they are again. */
    val unreachable = MutableStateFlow(false)
    var awaitReachable: suspend () -> Unit = {}

    suspend fun connect(token: String = "token") {
        connection.signIn(token, SERVER_KEY.encoded)
        downloads.attach(DownloadServer(connection, { caps }, unreachable, { awaitReachable() }))
        scope.advanceUntilIdle()
    }

    suspend fun item(): DownloadItem? = downloads.items.first().singleOrNull()

    /** The transfer of a file, by its name, as last started. */
    fun transfer(name: String) = transport.started.last { it.id.endsWith("/$name") }

    /** The transport fetched [bytes] for a transfer. */
    fun arrive(request: TransferRequest, bytes: ByteArray) {
        File(request.destination).apply { parentFile.mkdirs() }.writeBytes(bytes)
        transport.running -= request.id
        downloads.transferred(TransferEvent.Finished(request.id))
        scope.advanceUntilIdle()
    }

    /** The transport's transfer stopped short, with no answer from the server. */
    fun fail(request: TransferRequest, message: String) {
        transport.running -= request.id
        downloads.transferred(TransferEvent.Failed(request.id, 0, message))
    }

    /** Every started transfer fetches what the server serves. */
    fun arriveAll() {
        for (r in transport.started.filter { it.id in transport.running }) {
            arrive(r, served.entries.first { r.url.endsWith(it.key) }.value)
        }
    }
}

/** A file, by range when the request asks for one. */
private fun MockRequestHandleScope.serve(request: HttpRequestData, bytes: ByteArray) = request.headers[HttpHeaders.Range]?.let { range ->
    val from = range.removePrefix("bytes=").substringBefore('-').toInt()
    respond(bytes.copyOfRange(from, bytes.size), HttpStatusCode.PartialContent, headersOf(HttpHeaders.ContentRange, "bytes $from-${bytes.size - 1}/${bytes.size}"))
} ?: respond(bytes, HttpStatusCode.OK)

class DownloadsTest {
    @Test
    fun aDownloadIsFetchedCheckedAndTheServerForgetsItsCopy() = runTest {
        val h = Harness(this)
        h.connect()
        val id = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()

        val asked = Json.parseToJsonElement(h.server.to("POST", "/api/v1/downloads").single().body).jsonObject
        assertEquals("high", asked["quality"]?.jsonPrimitive?.content, "the device's quality")
        assertEquals(setOf("$id/part-0.mp4", "$id/subtitle-t3.vtt", "$id/artwork"), h.transport.running, "each file is handed over")
        val part = h.transfer("part-0.mp4")
        assertEquals("https://kivo.test/api/v1/downloads/d1/files/0", part.url)
        assertEquals("Bearer token", part.authorization)
        assertFalse(part.cellular, "Wi-Fi only, by default")
        assertIs<DownloadStatus.Downloading>(h.item()?.status)

        h.arriveAll()
        val item = assertNotNull(h.item())
        assertEquals(DownloadStatus.Ready, item.status)
        assertNotNull(item.artworkPath)
        assertEquals(1, h.server.to("DELETE", "/api/v1/downloads/d1").size, "the device has it: the server forgets its copy")
        assertContentEquals(movie, File(h.disk.folder, "$id/part-0.mp4").readBytes())
    }

    @Test
    fun tappingDownloadAgainWhileTheFirstIsAskedForKeepsOneDownload() = runTest {
        val h = Harness(this)
        h.connect()
        // Three taps before the server has answered the first: each asks while
        // the others are still waiting on the estimate, the user and the work.
        val created = CompletableDeferred<Unit>()
        h.server.on("POST", "/api/v1/downloads") {
            created.await()
            json(ServerDownload.serializer(), h.remote, HttpStatusCode.Created)
        }
        val taps = List(3) { async { h.downloads.download(DownloadRequest("w1")) } }
        runCurrent()
        assertEquals(setOf("w1"), h.downloads.asking.first(), "the button shows it's waiting at once")
        assertNull(h.item(), "nothing stored before the server answers")
        created.complete(Unit)
        val ids = taps.awaitAll()
        advanceUntilIdle()

        val rows = h.downloads.items.first()
        assertEquals(
            listOf(1, 1, 1),
            listOf(rows.size, ids.toSet().size, h.server.to("POST", "/api/v1/downloads").size),
            "one download of the work, every tap given it, the server asked once",
        )
        assertEquals(emptySet(), h.downloads.asking.first())

        // With one download of it here, the server forgets its copy once, and nothing asks for it again.
        h.arriveAll()
        assertEquals(DownloadStatus.Ready, h.item()?.status)
        assertEquals(1, h.server.to("DELETE", "/api/v1/downloads/d1").size)
        assertEquals(1, h.server.to("POST", "/api/v1/downloads").size)
    }

    @Test
    fun aFileThatArrivesWrongIsFetchedAgainThenFails() = runTest {
        val h = Harness(this)
        h.connect()
        val id = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        h.arrive(h.transfer("subtitle-t3.vtt"), vtt)
        h.arrive(h.transfer("artwork"), "art".encodeToByteArray())
        h.arrive(h.transfer("part-0.mp4"), "the movie's bytes, some of them".encodeToByteArray())
        assertEquals(2, h.transport.started.count { it.id == "$id/part-0.mp4" }, "a file that fails its check is fetched again")
        assertIs<DownloadStatus.Downloading>(h.item()?.status)

        h.arrive(h.transfer("part-0.mp4"), movie.reversedArray()) // the right size, the wrong bytes
        h.arrive(h.transfer("part-0.mp4"), movie.reversedArray())
        assertEquals(DownloadStatus.Failed(DownloadStatus.CHECK_FAILED), h.item()?.status)
        assertTrue(h.server.to("DELETE", "/api/v1/downloads/d1").isEmpty(), "the server keeps its copy for a retry")

        h.downloads.retry(id)
        advanceUntilIdle()
        h.arrive(h.transfer("part-0.mp4"), movie)
        assertEquals(DownloadStatus.Ready, h.item()?.status)
    }

    @Test
    fun anInterruptedTransferResumesWhereItStopped() = runTest {
        val h = Harness(this)
        h.connect()
        h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        h.arrive(h.transfer("subtitle-t3.vtt"), vtt)
        h.arrive(h.transfer("artwork"), "art".encodeToByteArray())

        // The network drops part way through: the transfer starts again after
        // a wait, as the same transfer, so the platform resumes it from the
        // bytes it kept.
        val part = h.transfer("part-0.mp4")
        h.downloads.transferred(TransferEvent.Progress(part.id, 10, movie.size.toLong()))
        advanceUntilIdle()
        assertEquals(DownloadStatus.Downloading(10, movie.size.toLong(), 0), h.item()?.status)
        h.downloads.transferred(TransferEvent.Failed(part.id, 0, "The network connection was lost."))
        advanceTimeBy(Downloads.backoff(0).inWholeMilliseconds - 1)
        assertEquals(1, h.transport.started.count { it.id == part.id }, "it waits before starting again")
        assertFalse(part.id in h.transport.cancelled, "what arrived is kept")
        advanceUntilIdle()
        assertEquals(listOf(part, part), h.transport.started.filter { it.id == part.id })

        // A second drop waits longer.
        h.downloads.transferred(TransferEvent.Failed(part.id, 0, "The network connection was lost."))
        advanceTimeBy(Downloads.backoff(1).inWholeMilliseconds - 1)
        assertEquals(2, h.transport.started.count { it.id == part.id })
        advanceUntilIdle()

        // The platform resumes it by range from where it stopped.
        val rest = h.connection.http.get(part.url) { header(HttpHeaders.Range, "bytes=10-") }.bodyAsBytes()
        h.arrive(part, movie.copyOf(10) + rest)
        assertEquals(DownloadStatus.Ready, h.item()?.status)
        assertContentEquals(movie, File(part.destination).readBytes())
    }

    @Test
    fun aRelaunchPicksUpWhereEachTransferWas() = runTest {
        val dir = createTempDirectory("kivo-relaunch").toFile()
        val first = Harness(this, dir)
        first.connect()
        val id = first.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        // While the app was away, the subtitle arrived and the transport kept
        // the part going; the artwork's transfer was lost.
        val subtitle = first.transfer("subtitle-t3.vtt")
        File(subtitle.destination).apply { parentFile.mkdirs() }.writeBytes(vtt)

        val again = Harness(this, dir, first.driver)
        again.transport.running += "$id/part-0.mp4"
        again.connect()
        assertEquals(listOf("$id/artwork"), again.transport.started.map { it.id }, "only what isn't under way starts again")
        again.arrive(again.transfer("artwork"), "art".encodeToByteArray())
        again.arrive(first.transfer("part-0.mp4"), movie)
        assertEquals(DownloadStatus.Ready, again.item()?.status, "the subtitle that arrived while away was checked and kept")
    }

    @Test
    fun aServerRestartMidConversionIsRecoveredFrom() = runTest {
        val h = Harness(this)
        h.remote = download(state = "queued")
        h.connect()
        val id = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        assertEquals(DownloadStatus.Queued, h.item()?.status)
        assertEquals(listOf("$id/artwork"), h.transport.started.map { it.id }, "only its artwork comes while it waits")

        h.remote = download(state = "preparing", progress = 0.5, remaining = 240_000)
        h.downloads.invalidated(Invalidation.DownloadsChanged)
        advanceUntilIdle()
        assertEquals(DownloadStatus.Converting(0.5, 240_000), h.item()?.status)
        assertEquals(true, h.item()?.prepared)

        // The server restarts: the conversion is back in line, then starts over.
        h.remote = download(state = "queued")
        h.downloads.sync()
        advanceUntilIdle()
        assertEquals(DownloadStatus.Queued, h.item()?.status)

        // A server that forgot it altogether is asked again.
        h.listed = false
        h.remote = download(id = "d2", state = "ready")
        h.downloads.sync()
        advanceUntilIdle()
        h.listed = true
        assertEquals(2, h.server.to("POST", "/api/v1/downloads").size)
        assertEquals("https://kivo.test/api/v1/downloads/d2/files/0", h.transfer("part-0.mp4").url)
        h.arrive(h.transfer("artwork"), "art".encodeToByteArray())
        h.arrive(h.transfer("part-0.mp4"), movie)
        h.arrive(h.transfer("subtitle-t3.vtt"), vtt)
        assertEquals(DownloadStatus.Ready, h.item()?.status)
        assertEquals(1, h.server.to("DELETE", "/api/v1/downloads/d2").size)
    }

    @Test
    fun aDownloadThatWouldntFitDoesntStart() = runTest {
        val h = Harness(this)
        h.disk.free = movie.size + HEADROOM_BYTES - 1
        h.connect()
        val e = assertFailsWith<NotEnoughSpaceException> { h.downloads.download(DownloadRequest("w1")) }
        assertEquals(1, e.shortBytes)
        assertTrue(h.server.to("POST", "/api/v1/downloads").isEmpty())
    }

    @Test
    fun aReadyDownloadKeepsItsWorkForOfflineUse() = runTest {
        val h = Harness(this)
        h.connect()
        h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        assertNull(h.downloads.storedWork("w1"), "not ready yet")
        h.arriveAll()
        h.server.on("GET", "/api/v1/works/w1") { problem(HttpStatusCode.ServiceUnavailable, "unavailable") }
        assertEquals("w1", h.downloads.storedWork("w1")?.id)
        assertNull(h.downloads.storedWork("w2"), "not downloaded")
    }

    @Test
    fun theOutboxSendsEachWorksLatestPosition() = runTest {
        val h = Harness(this)
        val sent = mutableListOf<String>()
        h.connect()
        val id = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        h.arriveAll()

        // Offline: the server can't be reached, so where play got to waits.
        h.server.on("GET", "/api/v1/works/w1") { problem(HttpStatusCode.ServiceUnavailable, "unavailable") }
        h.server.on("PATCH", "/api/v1/works/w1/user-state") { problem(HttpStatusCode.ServiceUnavailable, "unavailable") }
        val play = h.downloads.offline(id)
        assertEquals(60_000, play.plays.start(com.mmcdole.kivo.api.models.PlayCreate("w1", caps)).startMs, "where the user left it")
        h.downloads.record(id, 1_000_000)
        h.clock += 60_000
        h.downloads.record(id, 2_000_000)
        h.clock -= 120_000
        h.downloads.record(id, 500_000) // an older report, arriving late, doesn't win
        h.clock += 120_000

        // Back online: one PATCH, the latest.
        h.server.on("PATCH", "/api/v1/works/w1/user-state") {
            sent += h.server.seen.last().body
            json(UserState.serializer(), UserState(played = false, positionMs = 2_000_000))
        }
        h.downloads.sendOutbox()
        advanceUntilIdle()
        val body = Json.parseToJsonElement(sent.single()).jsonObject
        assertEquals(2_000_000, body["positionMs"]!!.jsonPrimitive.long)
        assertEquals("v1", body["versionId"]!!.jsonPrimitive.content)
        assertEquals(Instant.fromEpochMilliseconds(h.clock), Instant.parse(body["playedAt"]!!.jsonPrimitive.content))
        h.downloads.sendOutbox()
        assertEquals(1, sent.size, "sent once")

        // Finishing it offline keeps it: only the user deletes a download.
        h.server.on("PATCH", "/api/v1/works/w1/user-state") {
            json(UserState.serializer(), UserState(played = true, positionMs = 0))
        }
        play.plays.stop("download-$id", com.mmcdole.kivo.api.models.PlayStop(7_200_000))
        advanceUntilIdle()
        assertEquals(DownloadStatus.Ready, h.item()?.status, "a finished download stays")
        assertTrue(File(h.disk.folder, id).exists())
    }

    @Test
    fun aDownloadPlaysOfflineFromItsFiles() = runTest {
        val h = Harness(this)
        h.connect()
        val id = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        h.arriveAll()
        h.server.on("GET", "/api/v1/works/w1") { problem(HttpStatusCode.ServiceUnavailable, "unavailable") }
        val started = h.downloads.offline(id).plays.start(com.mmcdole.kivo.api.models.PlayCreate("w1", caps))
        assertEquals(PlayDelivery.File, started.delivery)
        assertEquals("file://" + File(h.disk.folder, "$id/part-0.mp4").path, started.url)
        assertEquals("file://" + File(h.disk.folder, "$id/part-0.mp4").path, started.parts.single().url)
        val subtitle = TrackShow.External("t3", "file://" + File(h.disk.folder, "$id/subtitle-t3.vtt").path, "webvtt", emptyList())
        assertEquals(subtitle, PlayTracks.show(started, "t3"))
        assertEquals("t3", PlayTracks.selected(started, TrackType.Subtitle))
        assertEquals(6_600_000, started.markers?.single()?.startMs, "its markers are the plan's, as saved at download")

        // A subtitle chosen on the detail page shows where the download has
        // it: off does, a track that didn't come with it doesn't.
        val plays = h.downloads.offline(id).plays
        assertEquals("off", PlayTracks.selected(plays.start(com.mmcdole.kivo.api.models.PlayCreate("w1", caps, subtitleTrackId = "off")), TrackType.Subtitle))
        assertEquals("t3", PlayTracks.selected(plays.start(com.mmcdole.kivo.api.models.PlayCreate("w1", caps, subtitleTrackId = "t9")), TrackType.Subtitle))
    }

    @Test
    fun everySubtitleComesDownAndShowsOffline() = runTest {
        val h = Harness(this)
        val files = mapOf(
            "downloads/d1/subtitles/t4.ass" to "[Script Info]\n".encodeToByteArray(),
            "fonts/f1" to "a font".encodeToByteArray(),
            "downloads/d1/subtitles/t5.sup" to "PG".encodeToByteArray(),
            "downloads/d1/subtitles/t6.vtt" to vtt,
        )
        h.served += files
        for ((url, bytes) in files) h.server.on("GET", "/api/v1/$url") { serve(it, bytes) }
        h.remote = download().copy(
            tracks = listOf(
                PlayTrack("t3", TrackType.Subtitle, selected = true, delivery = TrackDelivery.External, codec = "webvtt", url = "downloads/d1/subtitles/t3.vtt"),
                PlayTrack("t4", TrackType.Subtitle, delivery = TrackDelivery.External, codec = "ass", url = "downloads/d1/subtitles/t4.ass", fonts = listOf("fonts/f1")),
                PlayTrack("t5", TrackType.Subtitle, delivery = TrackDelivery.External, codec = "hdmv_pgs_subtitle", url = "downloads/d1/subtitles/t5.sup"),
                PlayTrack("t6", TrackType.Subtitle, delivery = TrackDelivery.External, codec = "webvtt", url = "downloads/d1/subtitles/t6.vtt"),
                PlayTrack("off", TrackType.Subtitle, delivery = TrackDelivery.None),
            ),
        )
        h.connect()
        val id = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        assertEquals(
            setOf("part-0.mp4", "subtitle-t3.vtt", "subtitle-t4.ass", "font-f1", "subtitle-t5.sup", "subtitle-t6.vtt", "artwork"),
            h.transport.running.map { it.substringAfter('/') }.toSet(),
            "every subtitle the server lists comes down, with its fonts",
        )
        h.arriveAll()
        assertEquals(DownloadStatus.Ready, h.item()?.status)

        val started = h.downloads.offline(id).plays.start(com.mmcdole.kivo.api.models.PlayCreate("w1", caps))
        val shown = listOf("t3", "t4", "t5", "t6").map { PlayTracks.show(started, it) as TrackShow.External }
        assertEquals(listOf("webvtt", "ass", "hdmv_pgs_subtitle", "webvtt"), shown.map { it.codec })
        assertTrue(shown.all { it.url.startsWith("file://" + File(h.disk.folder, id).path) }, "each from its file here")
        assertEquals(listOf("file://" + File(h.disk.folder, "$id/font-f1").path), shown[1].fonts)
        val plays = h.downloads.offline(id).plays
        assertEquals("t5", PlayTracks.selected(plays.start(com.mmcdole.kivo.api.models.PlayCreate("w1", caps, subtitleTrackId = "t5")), TrackType.Subtitle), "the PGS track")
        assertEquals("t4", PlayTracks.selected(plays.start(com.mmcdole.kivo.api.models.PlayCreate("w1", caps, subtitleTrackId = "t4")), TrackType.Subtitle), "the styled one")
    }

    @Test
    fun theTextTracksAConversionCarriesInItsFileShowOfflineByTheirIndex() = runTest {
        val h = Harness(this)
        val sup = "PG".encodeToByteArray()
        h.served += mapOf("downloads/d1/subtitles/t5.sup" to sup)
        h.server.on("GET", "/api/v1/downloads/d1/subtitles/t5.sup") { serve(it, sup) }
        // For a client that draws PGS but no WebVTT: the text tracks in the file, the PGS a file of its own.
        h.remote = download().copy(
            tracks = listOf(
                PlayTrack("t1", TrackType.Video, selected = true, delivery = TrackDelivery.Stream, codec = "h264", transcode = TrackTranscode(1920, 1080, 8_000_000, encoder = "CPU")),
                PlayTrack("t3", TrackType.Subtitle, selected = true, delivery = TrackDelivery.File, codec = "mov_text", index = 2),
                PlayTrack("t4", TrackType.Subtitle, delivery = TrackDelivery.File, codec = "mov_text", index = 3),
                PlayTrack("t5", TrackType.Subtitle, delivery = TrackDelivery.External, codec = "hdmv_pgs_subtitle", url = "downloads/d1/subtitles/t5.sup"),
                PlayTrack("off", TrackType.Subtitle, delivery = TrackDelivery.None),
            ),
        )
        h.connect()
        val id = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        h.arriveAll()

        val plays = h.downloads.offline(id).plays
        val started = plays.start(com.mmcdole.kivo.api.models.PlayCreate("w1", caps))
        assertEquals(listOf<TrackShow>(TrackShow.File(2), TrackShow.File(3)), listOf("t3", "t4").map { PlayTracks.show(started, it) })
        assertTrue(PlayTracks.show(started, "t5") is TrackShow.External)
        assertEquals(TrackTranscode(1920, 1080), started.tracks.first().transcode, "what the video became, without how")
        assertEquals(TrackDelivery.File, started.tracks.first().delivery)
        assertEquals("t4", PlayTracks.selected(plays.start(com.mmcdole.kivo.api.models.PlayCreate("w1", caps, subtitleTrackId = "t4")), TrackType.Subtitle))
    }

    @Test
    fun aSubtitleBurnedIntoADownloadStaysAndNothingElseShows() {
        val tracks = offlineTracks(
            listOf(
                PlayTrack("t3", TrackType.Subtitle, selected = true, delivery = TrackDelivery.Burn),
                PlayTrack("t4", TrackType.Subtitle),
                PlayTrack("off", TrackType.Subtitle),
            ),
        ) { it }
        val play = PlayStarted("p", "w1", "v1", 0, 1, true, PlayDelivery.File, "file:///0.mp4", tracks, emptyList(), emptyList())
        assertEquals(TrackShow.Burned, PlayTracks.show(play, "t3"))
        assertEquals(TrackShow.NewPlay, PlayTracks.show(play, "t4"))
        assertEquals(TrackShow.NewPlay, PlayTracks.show(play, "off"))
    }

    @Test
    fun offShowsWhenTheChosenSubtitleFileDidntCome() {
        val tracks = offlineTracks(
            listOf(
                PlayTrack("t3", TrackType.Subtitle, selected = true, delivery = TrackDelivery.External, codec = "webvtt", url = "downloads/d1/subtitles/t3.vtt"),
                PlayTrack("off", TrackType.Subtitle, delivery = TrackDelivery.None),
            ),
        ) { null }
        assertEquals(listOf<Boolean?>(null, true), tracks.map { it.selected })
    }

    /** A ready download, played offline once at [position], with the server's answers to its position held until [gate] opens. */
    private suspend fun TestScope.heldBack(gate: CompletableDeferred<Unit>, serverPosition: Long, position: Long = 1_500_000): Pair<Harness, String> {
        val h = Harness(this)
        h.connect()
        val id = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        h.arriveAll()
        h.downloads.record(id, position)
        h.server.on("PATCH", "/api/v1/works/w1/user-state") {
            gate.await()
            json(UserState.serializer(), UserState(played = false, positionMs = position.toInt()))
        }
        h.server.on("GET", "/api/v1/works/w1") {
            gate.await()
            val work = WORK.replace(""""positionMs":60000""", """"positionMs":$serverPosition""")
            respond(work, HttpStatusCode.OK, headersOf(HttpHeaders.ContentType, "application/json"))
        }
        return h to id
    }

    private suspend fun Harness.startMs(id: String) = downloads.offline(id).plays.start(com.mmcdole.kivo.api.models.PlayCreate("w1", caps)).startMs

    @Test
    fun aDownloadPlaysWithinASecondWhenTheServerDoesntAnswerAndTheNextPlayTakesItsPosition() = runTest {
        val gate = CompletableDeferred<Unit>()
        val (h, id) = heldBack(gate, serverPosition = 3_000_000)
        val asked = currentTime
        assertEquals(1_500_000, h.startMs(id), "where offline play left it")
        assertEquals(1_000, currentTime - asked, "a server that seemed reachable is waited for a second")

        // The server answers once play is under way: the next play starts there.
        gate.complete(Unit)
        advanceUntilIdle()
        assertEquals(1, h.server.to("PATCH", "/api/v1/works/w1/user-state").size, "the outbox went first")
        h.unreachable.value = true
        assertEquals(3_000_000, h.startMs(id))
    }

    @Test
    fun theServersLatePositionDoesntReplaceOfflinePlaysOwn() = runTest {
        val gate = CompletableDeferred<Unit>()
        val (h, id) = heldBack(gate, serverPosition = 3_000_000)
        assertEquals(1_500_000, h.startMs(id))
        h.downloads.record(id, 1_600_000)
        gate.complete(Unit)
        advanceUntilIdle()
        h.unreachable.value = true
        assertEquals(1_600_000, h.startMs(id), "played here since: this device's position stands")
    }

    @Test
    fun aServerAnsweringAtOnceGivesItsPositionToThisPlay() = runTest {
        val gate = CompletableDeferred(Unit)
        val (h, id) = heldBack(gate, serverPosition = 3_000_000)
        assertEquals(3_000_000, h.startMs(id), "continued on another device")
    }

    @Test
    fun whileTheServerIsUnreachableADownloadPlaysAtOnceAndCatchesUpWhenItAnswers() = runTest {
        val gate = CompletableDeferred<Unit>()
        val (h, id) = heldBack(gate, serverPosition = 3_000_000)
        val reachable = CompletableDeferred<Unit>()
        h.awaitReachable = { reachable.await() }
        h.unreachable.value = true
        val asked = currentTime
        assertEquals(1_500_000, h.startMs(id))
        assertEquals(asked, currentTime, "no wait at all")
        advanceTimeBy(10_000)
        assertTrue(h.server.to("PATCH", "/api/v1/works/w1/user-state").isEmpty(), "nothing is sent until the server answers")

        // It answers again: the outbox goes, and the next play starts where the server has it.
        reachable.complete(Unit)
        gate.complete(Unit)
        advanceUntilIdle()
        assertEquals(1, h.server.to("PATCH", "/api/v1/works/w1/user-state").size)
        assertEquals(3_000_000, h.startMs(id))
    }

    /** The plan's tracks for an HEVC 1080p source with 5.1 AC-3, as the server makes them for [delivery]. */
    private fun plannedTracks(delivery: DownloadDelivery) = if (delivery == DownloadDelivery.Conversion) {
        listOf(
            PlayTrack(
                "t1", TrackType.Video, selected = true, delivery = TrackDelivery.Stream, codec = "h264", reason = TrackReason.DownloadQuality,
                transcode = TrackTranscode(1280, 720, 4_000_000, encoder = "CPU"),
            ),
            PlayTrack("t2", TrackType.Audio, selected = true, delivery = TrackDelivery.Stream, codec = "aac", reason = TrackReason.DownloadQuality, transcode = TrackTranscode(channels = 2)),
            PlayTrack("t3", TrackType.Subtitle, selected = true, delivery = TrackDelivery.External, codec = "webvtt", url = "downloads/d1/subtitles/t3.vtt"),
        )
    } else {
        listOf(
            PlayTrack("t1", TrackType.Video, selected = true, delivery = TrackDelivery.File, codec = "hevc", index = 0),
            PlayTrack("t2", TrackType.Audio, selected = true, delivery = TrackDelivery.File, codec = "ac3", index = 1),
            PlayTrack("t3", TrackType.Subtitle, selected = true, delivery = TrackDelivery.External, codec = "webvtt", url = "downloads/d1/subtitles/t3.vtt"),
        )
    }

    /** A download of [delivery] played offline: the play and its Source line. */
    private suspend fun TestScope.playOffline(delivery: DownloadDelivery, quality: String): Pair<PlayStarted, DownloadSource> {
        val h = Harness(this)
        h.remote = download(delivery = delivery).copy(quality = quality, tracks = plannedTracks(delivery))
        val version = """{"id":"v1","name":"1080p","available":true,"durationMs":7200000,"sizeBytes":20000000000,"parts":[],"workIds":["w1"],
            "tracks":[{"id":"t1","type":"video","codec":"hevc","default":true,"name":"HEVC","fullName":"HEVC","video":{"width":1920,"height":1080}},
            {"id":"t2","type":"audio","codec":"ac3","default":true,"name":"English · 5.1","fullName":"English · AC-3 5.1 · Default","audio":{"channels":6}}]}"""
        val work = WORK.replace(""""versions":[]""", """"versions":[$version]""")
        h.server.on("GET", "/api/v1/works/w1") { respond(work, HttpStatusCode.OK, headersOf(HttpHeaders.ContentType, "application/json")) }
        h.connect()
        val id = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        h.arriveAll()
        h.server.on("GET", "/api/v1/works/w1") { problem(HttpStatusCode.ServiceUnavailable, "unavailable") }
        val play = h.downloads.offline(id)
        return play.plays.start(com.mmcdole.kivo.api.models.PlayCreate("w1", caps)) to play.source()
    }

    @Test
    fun aDownloadedOriginalPlaysDirectlyAsItsSource() = runTest {
        val (started, source) = playOffline(DownloadDelivery.File, "original")
        val en = Display()
        assertEquals("Direct Play", en.method(started))
        val (video, audio) = started.tracks
        assertEquals(listOf(TrackDelivery.File, TrackDelivery.File), listOf(video.delivery, audio.delivery))
        assertEquals(listOf("hevc", "ac3"), listOf(video.codec, audio.codec), "the source's tracks are the file's")
        assertEquals(listOf(0, 1), listOf(video.index, audio.index))
        assertEquals(listOf(null, null), listOf(video.reason, audio.reason))
        assertNull(video.transcode)
        assertEquals(DownloadSource(null), source)
    }

    @Test
    fun aDownloadedConversionPlaysDirectlyAsWhatItBecame() = runTest {
        val (started, source) = playOffline(DownloadDelivery.Conversion, "standard")
        val en = Display()
        assertEquals("Direct Play", en.method(started))
        val (video, audio) = started.tracks
        assertEquals("Direct Play", en.method(video))
        assertNull(video.reason)
        assertEquals("h264", video.codec)
        assertEquals(TrackTranscode(width = 1280, height = 720), video.transcode, "what the file is now, without how it was made")
        assertEquals("aac", audio.codec)
        assertEquals(TrackTranscode(channels = 2), audio.transcode)
        assertEquals(DownloadSource("HEVC"), source)
    }

    @Test
    fun aDownloadKeptInAnOlderShapeFailsAloneAndRetryAsksAfresh() = runTest {
        val h = Harness(this)
        h.connect()
        val id = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        h.arriveAll()
        h.driver.execute(null, "UPDATE download SET plan = '{\"decision\":{}}' WHERE id = '$id'", 0)
        // The old plan's file under a name the new plan doesn't give.
        val part = h.downloads.files(id).first { it.role == Downloads.PART }
        val old = h.downloads.path(part).parent!! / "part-0.m4v"
        FileSystem.SYSTEM.atomicMove(h.downloads.path(part), old)
        h.driver.execute(null, "UPDATE download_file SET name = 'part-0.m4v' WHERE download_id = '$id' AND name = '${part.name}'", 0)

        assertEquals(DownloadStatus.Failed(Downloads.PLAN_UNREADABLE), h.item()?.status)
        assertNull(h.downloads.playable("w1"), "it streams instead")
        val asked = h.server.to("POST", "/api/v1/downloads").size
        h.downloads.retry(id)
        advanceUntilIdle()
        assertEquals(asked + 1, h.server.to("POST", "/api/v1/downloads").size)
        assertTrue(h.item()?.status !is DownloadStatus.Failed)
        assertFalse(FileSystem.SYSTEM.exists(old), "the old file goes")
        assertTrue(h.downloads.files(id).none { it.name == "part-0.m4v" })
    }

    @Test
    fun aDownloadsKindIsTheWorksAsStored() = runTest {
        val h = Harness(this)
        h.connect()
        val id = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        assertEquals(WorkKind.Movie, h.item()?.kind)
        assertEquals(DownloadQuality.High, h.item()?.quality)
        h.driver.execute(null, "UPDATE download SET kind = 'podcast', quality = 'lossless' WHERE id = '$id'", 0)
        h.downloads.sync()
        advanceUntilIdle()
        assertEquals(WorkKind.Unknown, h.item()?.kind, "a kind this version doesn't know still lists")
        assertNull(h.item()?.quality)
    }

    // Migration (downloads): remove after every client has updated past this release.
    @Test
    fun anEarlierVersionsDownloadPlaysOnAtItsQualityNow() = runTest {
        val h = Harness(this)
        h.connect()
        val id = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        h.arriveAll()
        h.driver.execute(
            null,
            "UPDATE download SET quality = '720p', plan = json_remove(replace(plan, '\"quality\":\"high\"', '\"quality\":\"720p\"'), '$.sizeExact') WHERE id = '$id'",
            0,
        )
        val item = h.item()
        assertEquals(DownloadStatus.Ready, item?.status)
        assertEquals(DownloadQuality.Standard, item?.quality)
        assertEquals(true, item?.sizeExact)
        assertNotNull(h.downloads.playable("w1"), "it plays from its files")
        assertEquals("""{"state":"queued","delivery":"conversion","sizeExact":false}""", withSizeExact("""{"state":"queued","delivery":"conversion"}"""))
        assertEquals("""{"delivery":"file","sizeExact":true}""", withSizeExact("""{"delivery":"file"}"""))
    }

    @Test
    fun aWorkStoredBeforeTracksWereNamedReadsWithPlainNames() {
        val stored = """{"versions":[{"id":"v1","edition":"Final Cut","tracks":[{"id":"t1","type":"video","codec":"hevc"},
            {"id":"t2","type":"audio","codec":"ac3","language":"eng"}]},{"id":"v2","resolution":"4k","tracks":[]}]}"""
        assertEquals(
            """{"versions":[{"id":"v1","edition":"Final Cut","tracks":[{"id":"t1","type":"video","codec":"hevc","name":"HEVC","fullName":"HEVC"},""" +
                """{"id":"t2","type":"audio","codec":"ac3","language":"eng","name":"English","fullName":"English"}],"name":"Final Cut"},""" +
                """{"id":"v2","resolution":"4k","tracks":[],"name":"4K"}]}""",
            withTrackNames(stored),
        )
        val named = """{"versions":[{"id":"v1","name":"1080p","tracks":[]}]}"""
        assertEquals(named, withTrackNames(named))
    }

    @Test
    fun aReadyDownloadPlaysInPlaceOfStreaming() = runTest {
        val h = Harness(this)
        h.connect()
        assertNull(h.downloads.playable("w1"), "nothing downloaded: it streams")
        val id = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        assertNull(h.downloads.playable("w1"), "not ready yet")
        h.arriveAll()

        val play = assertNotNull(h.downloads.playable("w1"))
        assertEquals("w1", play.work.id)
        assertEquals("download-$id", play.plays.start(com.mmcdole.kivo.api.models.PlayCreate("w1", caps)).id)
    }

    @Test
    fun aDownloadTheUserMayNoLongerHaveGoes() = runTest {
        val h = Harness(this)
        h.connect()
        val id = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        h.arriveAll()
        h.server.on("POST", "/api/v1/downloads/check") {
            json(DownloadCheckResult.serializer(), DownloadCheckResult(listOf(DownloadAvailability("w1", "v1", available = false))))
        }
        h.clock += 2 * 60 * 60 * 1000
        h.downloads.sync()
        advanceUntilIdle()
        assertNull(h.item())
        assertFalse(File(h.disk.folder, id).exists())
        assertEquals(1, h.downloads.deleted.first(), "the list says so once")
        h.downloads.deletionsSeen()
        assertEquals(0, h.downloads.deleted.first())
    }

    @Test
    fun aDownloadPlaysHoweverLongTheServerHasBeenAway() = runTest {
        val h = Harness(this)
        h.connect()
        val id = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        h.arriveAll()

        h.clock += 365.days.inWholeMilliseconds
        h.server.on("POST", "/api/v1/downloads/check") { problem(HttpStatusCode.ServiceUnavailable, "unavailable") }
        h.server.on("GET", "/api/v1/works/w1") { problem(HttpStatusCode.ServiceUnavailable, "unavailable") }
        assertEquals("download-$id", h.downloads.offline(id).plays.start(com.mmcdole.kivo.api.models.PlayCreate("w1", caps)).id)
        assertNotNull(h.downloads.playable("w1"))
    }

    @Test
    fun aStoreFromBeforeLosesWhatItNoLongerKeepsAndKeepsItsDownloads() {
        val driver = JdbcSqliteDriver("jdbc:sqlite:${File(createTempDirectory("kivo-store").toFile(), "downloads.db").path}")
        DownloadDatabase.Schema.create(driver)
        // As the first version kept a download, with when the server last said the user may have it, whether to remove it when
        // finished, a conversion's time left in seconds, and its place in the server's line.
        driver.execute(null, "ALTER TABLE download RENAME COLUMN remaining_ms TO remaining_seconds", 0)
        driver.execute(null, "ALTER TABLE download ADD COLUMN queue_position INTEGER", 0)
        driver.execute(null, "ALTER TABLE download ADD COLUMN remove_when_finished INTEGER NOT NULL DEFAULT 1", 0)
        driver.execute(null, "ALTER TABLE download ADD COLUMN checked_at INTEGER NOT NULL DEFAULT 0", 0)
        driver.execute(
            null,
            "INSERT INTO download (id, server_id, account_id, work_id, quality, state, queue_position, remaining_seconds, plan, work, title, kind, size_bytes, remove_when_finished, created_at, checked_at) " +
                "VALUES ('d', 's', 'a', 'w1', '1080p', 'converting', 2, 90, '{}', '{}', 'Title', 'movie', 1, 1, 1, 1)",
            0,
        )
        DownloadDatabase.Schema.migrate(driver, 1, DownloadDatabase.Schema.version)
        val q = DownloadDatabase(driver).downloadsQueries
        val row = q.download("d").executeAsOne()
        assertEquals("Title", row.title)
        assertEquals(90_000L, row.remaining_ms)
    }

    @Test
    fun aGroupIsDeletedWithItsDownloadsUnderWayCancelled() = runTest {
        val h = Harness(this)
        for (w in listOf("w2", "w3")) {
            h.server.on("GET", "/api/v1/works/$w") { respond(WORK.replace("\"w1\"", "\"$w\""), HttpStatusCode.OK, headersOf(HttpHeaders.ContentType, "application/json")) }
        }
        h.server.on("DELETE", "/api/v1/downloads/d3") { empty() }
        h.connect()
        // A show's episodes: one here, one converting on the server, one on its way.
        val ready = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        h.arriveAll()
        h.remote = download(id = "d2", state = "preparing", progress = 0.4, remaining = 120_000)
        val converting = h.downloads.download(DownloadRequest("w2"))
        h.remote = download(id = "d3")
        val coming = h.downloads.download(DownloadRequest("w3"))
        advanceUntilIdle()
        assertEquals(listOf(DownloadStatus.Ready::class, DownloadStatus.Converting::class, DownloadStatus.Downloading::class), listOf(ready, converting, coming).map { id -> h.downloads.items.first().single { it.id == id }.status::class })

        h.downloads.delete(listOf(ready, converting, coming))
        advanceUntilIdle()
        assertTrue(h.downloads.items.first().isEmpty())
        assertTrue("$coming/part-0.mp4" in h.transport.cancelled, "the transfer under way stops")
        assertEquals(1, h.server.to("DELETE", "/api/v1/downloads/d2").size, "the server stops converting it")
        assertEquals(1, h.server.to("DELETE", "/api/v1/downloads/d3").size, "the server forgets the copy it was sending")
        for (id in listOf(ready, converting, coming)) assertFalse(File(h.disk.folder, id).exists())
    }

    @Test
    fun signingOutDeletesTheServersDownloads() = runTest {
        val h = Harness(this)
        h.connect()
        val id = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        h.downloads.forget("server-1")
        advanceUntilIdle()
        assertNull(h.item())
        assertTrue(h.transport.cancelled.contains("$id/part-0.mp4"))
        assertFalse(File(h.disk.folder, id).exists())
    }

    @Test
    fun allowingCellularStartsWhatWaitedForWifiAtOnce() = runTest {
        val h = Harness(this)
        h.connect()
        h.downloads.network(NetworkPath(online = true, expensive = true))
        val id = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        h.arrive(h.transfer("artwork"), "art".encodeToByteArray())
        assertFalse(h.transfer("part-0.mp4").cellular)
        assertEquals(DownloadStatus.Waiting(WaitReason.Wifi, 0, movie.size.toLong()), h.item()?.status)

        h.settings = h.settings.copy(cellularDownloads = true)
        h.downloads.settingsChanged()
        advanceUntilIdle()
        assertEquals(
            setOf("$id/part-0.mp4", "$id/subtitle-t3.vtt"), h.transport.started.filter { it.cellular }.map { it.id }.toSet(),
            "each file still to come goes on under the new rule, and the artwork that's here doesn't",
        )
        assertTrue(h.transport.cancelled.isEmpty(), "what arrived is kept")
        assertEquals(DownloadStatus.Downloading(0, movie.size.toLong(), 0), h.item()?.status)

        h.downloads.settingsChanged()
        advanceUntilIdle()
        assertEquals(2, h.transport.started.count { it.cellular }, "an unchanged rule moves nothing")
        h.arriveAll()
        assertEquals(DownloadStatus.Ready, h.item()?.status)
    }

    @Test
    fun disallowingCellularOnCellularMakesTransfersWaitForWifi() = runTest {
        val h = Harness(this)
        h.settings = h.settings.copy(cellularDownloads = true)
        h.connect()
        h.downloads.network(NetworkPath(online = true, expensive = true))
        val id = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        val part = h.transfer("part-0.mp4")
        assertTrue(part.cellular)
        h.downloads.transferred(TransferEvent.Progress(part.id, 10, movie.size.toLong()))
        h.clock += 1000
        h.downloads.transferred(TransferEvent.Progress(part.id, 20, movie.size.toLong()))
        assertEquals(DownloadStatus.Downloading(20, movie.size.toLong(), 10), h.item()?.status)

        h.settings = h.settings.copy(cellularDownloads = false)
        h.downloads.settingsChanged()
        advanceUntilIdle()
        assertFalse(h.transfer("part-0.mp4").cellular, "it goes on under the Wi-Fi rule, resuming what arrived")
        assertTrue(h.transport.cancelled.isEmpty())
        assertEquals(DownloadStatus.Waiting(WaitReason.Wifi, 20, movie.size.toLong()), h.item()?.status)

        // On Wi-Fi it comes again, its speed measured afresh.
        h.downloads.network(NetworkPath(online = true, expensive = false))
        advanceUntilIdle()
        assertEquals(DownloadStatus.Downloading(20, movie.size.toLong(), 0), h.item()?.status)
        assertEquals(2, h.transport.started.count { it.id == "$id/part-0.mp4" }, "the network changing doesn't start what's under way again")
    }

    @Test
    fun transfersComeFromTheHttpsAddressEvenOnAProvenLocalOne() = runTest {
        val h = Harness(this, at = LOCAL, addresses = listOf(LOCAL, PUBLIC))
        h.server.on("GET", "/api/v1/server") { json(ServerInfo.serializer(), ServerInfo("server-1", "Basement", "1.0", proof = proofOf(it, "server-1"))) }
        h.connect(TOKEN)
        h.connection.address.repick(AddressChange.Network)
        assertEquals(LOCAL, h.connection.baseUrl)

        h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()

        assertEquals("$PUBLIC/api/v1/downloads/d1/files/0", h.transfer("part-0.mp4").url)
        assertEquals("Bearer $TOKEN", h.transfer("part-0.mp4").authorization)
        assertTrue(h.transport.started.none { it.url.startsWith(LOCAL) })
    }

    @Test
    fun withoutHttpsTransfersStopOnANewNetworkAndGoOnOnceTheAddressIsProvenAgain() = runTest {
        val h = Harness(this, at = LOCAL)
        var impostor = false
        h.server.on("GET", "/api/v1/server") {
            val proof = proofOf(it, "server-1").takeUnless { impostor }
            json(ServerInfo.serializer(), ServerInfo("server-1", "Basement", "1.0", proof = proof))
        }
        h.connect(TOKEN)
        h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        val part = h.transfer("part-0.mp4")
        assertEquals("$LOCAL/api/v1/downloads/d1/files/0", part.url)

        impostor = true
        h.network.changed()
        advanceUntilIdle()
        assertTrue(part.id in h.transport.stopped, "stopped at once, keeping what it fetched")
        assertTrue(part.id !in h.transport.running)
        assertFalse(h.connection.address.repick(AddressChange.Network))
        advanceUntilIdle()
        assertEquals(1, h.transport.started.count { it.id == part.id }, "not started again where it's unproven")

        impostor = false
        h.connection.address.repick(AddressChange.Network)
        advanceUntilIdle()
        assertEquals(2, h.transport.started.count { it.id == part.id }, "proven again: it goes on")
        assertTrue(h.transport.cancelled.isEmpty())
    }

    @Test
    fun theServerMovingToAnotherAddressMovesTheTransfersThere() = runTest {
        val h = Harness(this)
        h.connect()
        val id = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        h.arrive(h.transfer("artwork"), "art".encodeToByteArray())
        val subtitle = h.transfer("subtitle-t3.vtt")
        h.fail(subtitle, "Could not connect to the server.")
        runCurrent()

        // The phone leaves home: only the server's public address answers.
        h.server.on("GET", "/api/v1/server") { request ->
            if (request.url.host == "kivo.test") throw IOException("no route to host")
            json(ServerInfo.serializer(), ServerInfo("server-1", "Basement", "1.0"))
        }
        h.connection.address.remember(listOf("https://kivo.test", "https://kivo.example.com"))
        assertTrue(h.connection.address.repick(AddressChange.Network))
        runCurrent()

        val moved = h.transport.started.filter { it.url.startsWith("https://kivo.example.com/") }
        assertEquals(setOf("$id/part-0.mp4", "$id/subtitle-t3.vtt"), moved.map { it.id }.toSet(), "the one under way and the one waiting to start again")
        assertEquals("https://kivo.example.com/api/v1/downloads/d1/files/0", h.transfer("part-0.mp4").url)
        assertTrue(h.transport.cancelled.isEmpty(), "what arrived is kept")
        advanceTimeBy(Downloads.backoff(0).inWholeMilliseconds)
        assertEquals(1, h.transport.started.count { it.id == subtitle.id && it.url.startsWith("https://kivo.example.com/") }, "the retry it waited for is the move")

        h.arriveAll()
        assertEquals(DownloadStatus.Ready, h.item()?.status)
    }

    @Test
    fun aTransferThatCantReachTheServerWaitsAndStartsOnceTheServerOrTheNetworkIsBack() = runTest {
        val h = Harness(this)
        h.connect()
        h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        h.arrive(h.transfer("subtitle-t3.vtt"), vtt)
        h.arrive(h.transfer("artwork"), "art".encodeToByteArray())
        val part = h.transfer("part-0.mp4")
        val size = movie.size.toLong()

        h.fail(part, "Could not connect to the server.")
        runCurrent()
        assertEquals(DownloadStatus.Waiting(WaitReason.Server, 0, size), h.item()?.status)
        // The app reaches the server before the wait is out: it starts now.
        val failedAt = currentTime
        h.downloads.sync()
        assertTrue(currentTime - failedAt < Downloads.backoff(0).inWholeMilliseconds)
        assertEquals(2, h.transport.started.count { it.id == part.id })
        assertEquals(DownloadStatus.Downloading(0, size, 0), h.item()?.status)

        // With no network it waits for one, and starts once there is.
        h.downloads.network(NetworkPath(online = false, expensive = false))
        h.fail(part, "The Internet connection appears to be offline.")
        runCurrent()
        assertEquals(DownloadStatus.Waiting(WaitReason.Network, 0, size), h.item()?.status)
        h.downloads.network(NetworkPath(online = true, expensive = false))
        runCurrent()
        assertEquals(3, h.transport.started.count { it.id == part.id }, "back on a network, it starts without waiting out the backoff")
        h.arrive(h.transfer("part-0.mp4"), movie)
        assertEquals(DownloadStatus.Ready, h.item()?.status)
    }

    @Test
    fun aPausedDownloadKeepsWhatArrivedAndResumesFromThere() = runTest {
        val h = Harness(this)
        h.connect()
        val id = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        h.arrive(h.transfer("artwork"), "art".encodeToByteArray())
        val part = h.transfer("part-0.mp4")
        val size = movie.size.toLong()
        h.downloads.transferred(TransferEvent.Progress(part.id, 10, size))

        h.downloads.pause(id)
        advanceUntilIdle()
        assertEquals(setOf(part.id, "$id/subtitle-t3.vtt"), h.transport.stopped.toSet(), "its transfers stop")
        assertTrue(h.transport.cancelled.isEmpty(), "keeping what they fetched")
        assertEquals(DownloadStatus.Paused(10, size), h.item()?.status)

        // A transfer's late failure, a relaunch's pick-up and a sync leave it paused.
        val started = h.transport.started.size
        h.downloads.transferred(TransferEvent.Failed(part.id, 0, "cancelled"))
        h.downloads.pickUp()
        h.downloads.sync()
        advanceUntilIdle()
        assertEquals(started, h.transport.started.size)
        assertEquals(DownloadStatus.Paused(10, size), h.item()?.status)

        h.downloads.resume(id)
        advanceUntilIdle()
        assertEquals(setOf(part.id, "$id/subtitle-t3.vtt"), h.transport.started.drop(started).map { it.id }.toSet(), "each file goes on from where it stopped")
        assertEquals(DownloadStatus.Downloading(10, size, 0), h.item()?.status)
        h.arriveAll()
        assertEquals(DownloadStatus.Ready, h.item()?.status)
    }

    @Test
    fun aDiskThatFillsMidTransferFailsTheDownloadForWantOfSpace() = runTest {
        val h = Harness(this)
        h.connect()
        val id = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        val part = h.transfer("part-0.mp4")
        h.disk.free = 5
        h.downloads.transferred(TransferEvent.Failed(part.id, 0, "No space left on device"))
        advanceUntilIdle()
        assertEquals(DownloadStatus.Failed(DownloadStatus.NOT_ENOUGH_SPACE), h.item()?.status)
        assertTrue("$id/subtitle-t3.vtt" in h.transport.stopped, "its other transfers stop, keeping what they fetched")
        assertEquals(1, h.transport.started.count { it.id == part.id }, "it isn't tried again by itself")

        h.disk.free = 100L * 1024 * 1024 * 1024
        h.downloads.retry(id)
        advanceUntilIdle()
        h.arriveAll()
        assertEquals(DownloadStatus.Ready, h.item()?.status)
    }

    @Test
    fun aDownloadWhoseWorkWasDeletedOnTheServerGoes() = runTest {
        val h = Harness(this)
        h.connect()
        val id = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        // The server forgot it as the work went: asking again finds no work, and the check says so.
        h.server.on("POST", "/api/v1/downloads") { problem(HttpStatusCode.NotFound, "notFound") }
        h.server.on("POST", "/api/v1/downloads/check") {
            json(DownloadCheckResult.serializer(), DownloadCheckResult(listOf(DownloadAvailability("w1", "v1", available = false))))
        }
        h.downloads.transferred(TransferEvent.Failed(h.transfer("part-0.mp4").id, 404, "Not Found"))
        advanceUntilIdle()
        assertNull(h.item())
        assertFalse(File(h.disk.folder, id).exists())
        assertEquals(1, h.downloads.deleted.first(), "the list says so once")
    }

    @Test
    fun aRefusedTransferAsksWhetherTheUserMayStillHaveIt() = runTest {
        val h = Harness(this)
        h.connect()
        h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        h.server.on("POST", "/api/v1/downloads/check") {
            json(DownloadCheckResult.serializer(), DownloadCheckResult(listOf(DownloadAvailability("w1", "v1", available = false))))
        }
        h.downloads.transferred(TransferEvent.Failed(h.transfer("part-0.mp4").id, 403, "Forbidden"))
        advanceUntilIdle()
        assertNull(h.item(), "downloads are no longer allowed: it goes")
    }

    @Test
    fun aBackgroundRefreshStartsWhatTheServerFinishedPreparing() = runTest {
        val h = Harness(this)
        h.remote = download(state = "preparing", progress = 0.5, remaining = 240_000)
        h.connect()
        val id = h.downloads.download(DownloadRequest("w1"))
        advanceUntilIdle()
        assertTrue(h.downloads.startReady(), "still being prepared: the refresh is asked for again")
        assertTrue(h.transport.started.none { it.id == "$id/part-0.mp4" })

        h.remote = download()
        assertFalse(h.downloads.startReady(), "nothing left on the server")
        assertEquals("https://kivo.test/api/v1/downloads/d1/files/0", h.transfer("part-0.mp4").url, "handed to the transport")
        assertIs<DownloadStatus.Downloading>(h.item()?.status)
    }

    @Test
    fun atMostTwoDownloadsComeAtOnceAndTheNextStartsAsOneFinishes() = runTest {
        val h = Harness(this)
        val shelf = Shelf(h)
        for (w in listOf("w2", "w3", "w4")) shelf.add(w)
        h.connect()
        for (w in listOf("w2", "w3", "w4")) {
            h.downloads.download(DownloadRequest(w))
            h.clock += 5 * 60_000
        }
        advanceUntilIdle()
        assertEquals(setOf("w2", "w3"), shelf.underWay(), "the first two asked for")
        assertEquals(DownloadStatus.Waiting(WaitReason.Turn, 0, movie.size.toLong()), shelf.status("w4"), "the third waits its turn")
        assertTrue("${shelf.id("w4")}/artwork" in h.transport.running, "its artwork comes at once")

        // Allowing cellular, or the server moving, hands over only the two under way again.
        h.settings = h.settings.copy(cellularDownloads = true)
        h.downloads.settingsChanged()
        advanceUntilIdle()
        assertEquals(setOf("w2", "w3"), shelf.underWay())
        assertTrue(h.transport.started.none { it.id.startsWith("${shelf.id("w4")}/part") })

        // A real blocker says so over the turn.
        h.downloads.network(NetworkPath(online = false, expensive = false))
        assertEquals(DownloadStatus.Waiting(WaitReason.Network, 0, movie.size.toLong()), shelf.status("w4"))
        h.downloads.network(NetworkPath(online = true, expensive = false))
        advanceUntilIdle()
        assertEquals(setOf("w2", "w3"), shelf.underWay(), "back on a network, still only two")

        shelf.finish("w2")
        assertEquals(DownloadStatus.Ready, shelf.status("w2"))
        assertEquals(setOf("w3", "w4"), shelf.underWay(), "the next starts as one finishes")
        assertIs<DownloadStatus.Downloading>(shelf.status("w4"))

        h.downloads.delete(shelf.id("w3"))
        advanceUntilIdle()
        assertEquals(setOf("w4"), shelf.underWay())
        shelf.finish("w4")
        assertTrue(shelf.underWay().isEmpty())
    }

    @Test
    fun episodesAskedForTogetherComeInSeasonAndEpisodeOrder() = runTest {
        val h = Harness(this)
        val shelf = Shelf(h)
        shelf.add("w2", season = 1, episode = 3, state = "preparing")
        shelf.add("w3", season = 2, episode = 1, state = "preparing")
        shelf.add("w4", season = 1, episode = 1, state = "preparing")
        shelf.add("w5", season = 1, episode = 2, state = "preparing")
        h.connect()
        for (w in listOf("w2", "w3", "w4", "w5")) {
            h.downloads.download(DownloadRequest(w))
            h.clock += 2_000
        }
        advanceUntilIdle()
        for (w in listOf("w2", "w3", "w4", "w5")) shelf.ready(w)
        h.downloads.invalidated(Invalidation.DownloadsChanged)
        advanceUntilIdle()

        assertEquals(setOf("w4", "w5"), shelf.underWay(), "S1 E1 and S1 E2 first")
        shelf.finish("w4")
        assertEquals(setOf("w5", "w2"), shelf.underWay(), "then S1 E3")
        shelf.finish("w5")
        assertEquals(setOf("w2", "w3"), shelf.underWay(), "then S2 E1")
    }

    @Test
    fun aPausedOrFailedDownloadDoesntHoldASlot() = runTest {
        val h = Harness(this)
        val shelf = Shelf(h)
        for (w in listOf("w2", "w3", "w4")) shelf.add(w)
        h.connect()
        for (w in listOf("w2", "w3", "w4")) {
            h.downloads.download(DownloadRequest(w))
            h.clock += 1_000
        }
        advanceUntilIdle()

        h.downloads.pause(shelf.id("w2"))
        advanceUntilIdle()
        assertEquals(setOf("w3", "w4"), shelf.underWay(), "its slot goes to the next")
        assertEquals(DownloadStatus.Paused(0, movie.size.toLong()), shelf.status("w2"))

        // Resumed, it waits its turn.
        h.downloads.resume(shelf.id("w2"))
        advanceUntilIdle()
        assertEquals(setOf("w3", "w4"), shelf.underWay())
        assertEquals(DownloadStatus.Waiting(WaitReason.Turn, 0, movie.size.toLong()), shelf.status("w2"))

        // A failed one doesn't hold one either.
        h.server.on("GET", "/api/v1/downloads/d-w3") { json(ServerDownload.serializer(), shelf.remotes.getValue("w3")) }
        h.arrive(h.transport.started.last { it.id == "${shelf.id("w3")}/part-0.mp4" }, movie.reversedArray())
        h.arrive(h.transport.started.last { it.id == "${shelf.id("w3")}/part-0.mp4" }, movie.reversedArray())
        h.arrive(h.transport.started.last { it.id == "${shelf.id("w3")}/part-0.mp4" }, movie.reversedArray())
        assertEquals(DownloadStatus.Failed(DownloadStatus.CHECK_FAILED), shelf.status("w3"))
        assertEquals(setOf("w2", "w4"), shelf.underWay(), "its slot goes to the next")
    }

    @Test
    fun aBackgroundRefreshStartsOnlyTwoOfWhatTheServerFinishedPreparing() = runTest {
        val h = Harness(this)
        val shelf = Shelf(h)
        for (w in listOf("w2", "w3", "w4")) shelf.add(w, state = "preparing")
        h.connect()
        for (w in listOf("w2", "w3", "w4")) {
            h.downloads.download(DownloadRequest(w))
            h.clock += 1_000
        }
        advanceUntilIdle()
        assertTrue(shelf.underWay().isEmpty())

        for (w in listOf("w2", "w3", "w4")) shelf.ready(w)
        assertFalse(h.downloads.startReady())
        assertEquals(setOf("w2", "w3"), shelf.underWay())
        assertEquals(DownloadStatus.Waiting(WaitReason.Turn, 0, movie.size.toLong()), shelf.status("w4"))
    }

    @Test
    fun aRelaunchKeepsTwoTransfersUnderWayAndStopsTheRest() = runTest {
        val dir = createTempDirectory("kivo-relaunch").toFile()
        val first = Harness(this, dir)
        val shelf = Shelf(first)
        for (w in listOf("w2", "w3", "w4")) shelf.add(w)
        first.connect()
        for (w in listOf("w2", "w3", "w4")) {
            first.downloads.download(DownloadRequest(w))
            first.clock += 1_000
        }
        advanceUntilIdle()
        val ids = listOf("w2", "w3", "w4").map { shelf.id(it) }

        // The platform kept all three going, as an app from before the limit left them.
        val again = Harness(this, dir, first.driver)
        for (id in ids) again.transport.running += listOf("$id/part-0.mp4", "$id/subtitle-t3.vtt")
        again.connect()
        assertEquals(setOf("${ids[2]}/part-0.mp4", "${ids[2]}/subtitle-t3.vtt"), again.transport.stopped.toSet(), "the last asked for stops, keeping what it fetched")
        assertTrue(again.transport.started.all { it.id.endsWith("/artwork") }, "the two under way go on as they are")
    }

    @Test
    fun qualitiesAreNamedByPurpose() {
        val en = Display()
        assertEquals("High", en.downloadQuality(DownloadQuality.High))
        assertEquals("Standard", en.downloadQuality(DownloadQuality.Standard))
        assertEquals("Original", en.downloadQuality(DownloadQuality.Original))
    }

    // Migration (downloads): remove after every client has updated past this release.
    @Test
    fun anEarlierVersionsQualityIsNamedAsItsQualityNow() {
        val en = Display()
        assertEquals("High", en.downloadQuality(DownloadQuality.parseOrNull("1080p-high")!!))
        assertEquals("Standard", en.downloadQuality(DownloadQuality.parseOrNull("480p")!!))
    }
}

/**
 * More works on the server, each made a download, `d-<work>`, when asked
 * for: a movie, or an episode of one show, ready at once unless [add]'s
 * state says otherwise.
 */
private class Shelf(val h: Harness) {
    val remotes = mutableMapOf<String, ServerDownload>()

    init {
        h.server.on("POST", "/api/v1/downloads") { request ->
            val asked = Json.parseToJsonElement((request.body as OutgoingContent.ByteArrayContent).bytes().decodeToString())
            val work = asked.jsonObject.getValue("workId").jsonPrimitive.content
            json(ServerDownload.serializer(), remotes[work] ?: h.remote, HttpStatusCode.Created)
        }
        h.server.on("GET", "/api/v1/downloads") { json(DownloadList.serializer(), DownloadList(remotes.values.toList())) }
    }

    fun add(work: String, season: Int? = null, episode: Int? = null, state: String = "ready") {
        val kind = if (season == null) "\"kind\":\"movie\"," else "\"kind\":\"episode\",\"episode\":{\"seasonNumber\":$season,\"number\":$episode,\"seriesId\":\"s1\",\"seriesTitle\":\"Severance\"},"
        val body = WORK.replace("\"w1\"", "\"$work\"").replace("\"kind\":\"movie\",", kind)
        h.server.on("GET", "/api/v1/works/$work") { respond(body, HttpStatusCode.OK, headersOf(HttpHeaders.ContentType, "application/json")) }
        h.server.on("DELETE", "/api/v1/downloads/d-$work") { empty() }
        remotes[work] = download(id = "d-$work", state = state, progress = 0.5.takeIf { state == "preparing" })
    }

    /** The server finished preparing it. */
    fun ready(work: String) {
        remotes[work] = download(id = "d-$work")
    }

    suspend fun items() = h.downloads.items.first()

    suspend fun id(work: String) = items().single { it.workId == work }.id

    suspend fun status(work: String) = items().single { it.workId == work }.status

    /** The works whose parts the transport is fetching. */
    suspend fun underWay(): Set<String> {
        val works = items().associate { it.id to it.workId }
        return h.transport.running.filter { it.endsWith("/part-0.mp4") }.mapNotNull { works[it.substringBefore('/')] }.toSet()
    }

    /** Each of its files under way arrives whole. */
    suspend fun finish(work: String) {
        val id = id(work)
        for (r in h.transport.started.filter { it.id.startsWith("$id/") && it.id in h.transport.running }.distinctBy { it.id }) {
            h.arrive(r, if (r.id.endsWith("/part-0.mp4")) movie else if (r.id.endsWith(".vtt")) vtt else "art".encodeToByteArray())
        }
    }
}
