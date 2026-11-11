package com.mmcdole.kivo.android.mobile.downloads

import com.mmcdole.kivo.android.mobile.CoreTest
import com.mmcdole.kivo.android.mobile.app.PlayerRoute
import com.mmcdole.kivo.android.mobile.awaitValue
import com.mmcdole.kivo.android.mobile.detail.DetailViewModel
import com.mmcdole.kivo.android.mobile.player.FakeEngine
import com.mmcdole.kivo.android.mobile.player.PlayerViewModel
import com.mmcdole.kivo.android.mobile.reply
import com.mmcdole.kivo.api.models.Download
import com.mmcdole.kivo.api.models.DownloadDelivery
import com.mmcdole.kivo.api.models.DownloadEstimate
import com.mmcdole.kivo.api.models.DownloadEstimateList
import com.mmcdole.kivo.api.models.DownloadList
import com.mmcdole.kivo.api.models.DownloadOption
import com.mmcdole.kivo.api.models.DownloadPart
import com.mmcdole.kivo.api.models.EpisodeInfo
import com.mmcdole.kivo.api.models.MatchState
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.core.downloads.DownloadStatus
import com.mmcdole.kivo.core.downloads.DownloadTransport
import com.mmcdole.kivo.core.downloads.TransferEvent
import com.mmcdole.kivo.core.downloads.TransferRequest
import com.mmcdole.kivo.core.play.PlayerStatus
import com.mmcdole.kivo.core.settings.DownloadQuality
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpStatusCode
import io.ktor.http.content.OutgoingContent
import io.ktor.http.content.TextContent
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import java.io.File
import java.security.MessageDigest
import java.util.concurrent.CopyOnWriteArrayList
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertIs
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

private val movie = "the movie's bytes, all of them".encodeToByteArray()

private fun sha(bytes: ByteArray) = MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it) }

/** A transport that fetches at once, with what [bytes] gives each request, unless it's [holding]. */
class Arriving(private val finished: (TransferEvent) -> Unit, var bytes: (TransferRequest) -> ByteArray) : DownloadTransport {
    val started = CopyOnWriteArrayList<TransferRequest>()
    private val running = mutableSetOf<String>()
    var holding = false

    override fun start(request: TransferRequest) {
        started += request
        running += request.id
        if (!holding) arrive(request)
    }

    fun arrive(request: TransferRequest) {
        File(request.destination).apply { parentFile?.mkdirs() }.writeBytes(bytes(request))
        running -= request.id
        finished(TransferEvent.Finished(request.id))
    }

    override fun stop(id: String) {
        running -= id
    }

    override fun cancel(id: String) {
        running -= id
    }

    override fun running() = running.toList()
}

/**
 * Downloads on the phone through the real core: a detail page's button
 * waits as the server is asked, a ready download lists with its words and
 * plays from its file, a file that keeps failing its check fails the
 * download until Retry, and deleting takes it off the list.
 */
class DownloadsViewModelTest : CoreTest() {
    override val transport = Arriving({ client.transferChanged(it) }) { movie }

    /** What the server says of the download it makes; the episodes' are by work. */
    private val asked = CopyOnWriteArrayList<String>()
    private var gate: CompletableDeferred<Unit>? = null

    private fun detail(id: String, kind: WorkKind = WorkKind.Movie, episode: EpisodeInfo? = null) = WorkDetail(
        id = id, kind = kind, title = if (kind == WorkKind.Movie) "Charade" else "Episode $id", matchState = MatchState.Matched, available = true, images = WorkImages(),
        genres = emptyList(), studios = emptyList(), credits = emptyList(), externalIds = emptyList(), versions = emptyList(),
        extras = emptyList(), collections = emptyList(), episode = episode, durationMs = 7_200_000,
    )

    private fun download(workId: String, id: String = "d-$workId") = Download(
        id = id, workId = workId, versionId = "v1", quality = "standard", state = "ready", delivery = DownloadDelivery.Conversion,
        durationMs = 7_200_000, sizeBytes = movie.size.toLong(), sizeExact = true,
        parts = listOf(DownloadPart(durationMs = 7_200_000, url = "downloads/$id/files/0", contentType = "video/mp4", sizeBytes = movie.size.toLong(), sha256 = sha(movie))),
        tracks = emptyList(), chapters = emptyList(), markers = emptyList(),
    )

    init {
        server.get("/api/v1/works/m1", WorkDetail.serializer()) { detail("m1") }
        for (n in 1..2) {
            server.get("/api/v1/works/e$n", WorkDetail.serializer()) {
                detail("e$n", WorkKind.Episode, EpisodeInfo(seasonNumber = 1, number = n, seriesId = "show", seriesTitle = "Show"))
            }
        }
        server.on("POST", "/api/v1/downloads/estimate") {
            reply(
                DownloadEstimateList.serializer(),
                DownloadEstimateList(
                    listOf("m1", "e1", "e2").map { work ->
                        DownloadEstimate(
                            work, playable = true,
                            options = listOf(
                                DownloadOption("original", 4_000_000_000, true, DownloadDelivery.File),
                                DownloadOption("high", 2_000_000_000, false, DownloadDelivery.Conversion),
                                DownloadOption("standard", movie.size.toLong(), false, DownloadDelivery.Conversion),
                            ),
                        )
                    }.take(1),
                ),
            )
        }
        server.on("POST", "/api/v1/downloads") { request ->
            gate?.await()
            val body = when (val content = request.body) {
                is TextContent -> content.text
                is OutgoingContent.ByteArrayContent -> content.bytes().decodeToString()
                else -> ""
            }
            val work = Regex("\"workId\":\"([^\"]+)\"").find(body)!!.groupValues[1]
            asked += body
            reply(Download.serializer(), download(work), HttpStatusCode.Created)
        }
        server.get("/api/v1/downloads", DownloadList.serializer()) { DownloadList(emptyList()) }
        server.on("POST", "/api/v1/downloads/d-m1/retry") { reply(Download.serializer(), download("m1")) }
        for (id in listOf("d-m1", "d-e1", "d-e2")) server.on("DELETE", "/api/v1/downloads/$id") { respond("", HttpStatusCode.NoContent) }
    }

    @Test
    fun theButtonWaitsWhileTheServerIsAskedThenTheDownloadComesAtTheChosenQuality() {
        val gate = CompletableDeferred<Unit>().also { this.gate = it }
        val session = signIn()
        val model = DetailViewModel(session, "m1", setting = flowOf(DownloadQuality.Standard))
        val ready = awaitValue(model.state) { it.download?.estimate != null }
        assertEquals(listOf("original", "high", "standard"), ready.download?.estimate?.options?.map { it.quality })
        assertEquals(DownloadQuality.Standard, ready.download?.setting)

        model.downloads!!.download(DownloadQuality.High)
        val asking = awaitValue(model.state) { it.download?.asking == true }
        assertEquals(null, asking.download?.item, "nothing listed yet: the button shows it's waiting")
        gate.complete(Unit)

        val done = awaitValue(model.state) { it.download?.item?.status == DownloadStatus.Ready }
        assertEquals(false, done.download?.asking)
        assertTrue(asked.single().contains("\"quality\":\"high\""), asked.single())
        assertEquals(DownloadWords.done(done.download!!.item!!), 100)
    }

    @Test
    fun downloadsListByKindWithTheirStorageAndDeletingTakesThemOff() {
        val session = signIn()
        val downloads = session.downloads!!
        runBlocking {
            downloads.download("m1", null, null, null, null)
            downloads.download("e1", null, null, null, null)
            downloads.download("e2", null, null, null, null)
        }
        val model = DownloadsViewModel(downloads)
        val state = awaitValue(model.state) { s -> s.movies.size == 1 && s.shows.singleOrNull()?.episodes?.all { it.status == DownloadStatus.Ready } == true }
        assertEquals("Charade", state.movies.single().title)
        assertEquals("Show", state.shows.single().title)
        assertEquals(null, DownloadWords.progress(state.shows.single().episodes), "every episode is here")
        assertTrue(awaitValue(model.state) { it.storage != null }.storage!!.usedBytes >= movie.size * 3L)

        val show = DownloadsViewModel(downloads, "show")
        assertEquals(listOf(1, 2), awaitValue(show.state) { it.seasons.isNotEmpty() }.seasons.single().episodes.map { it.episode })

        model.delete(state.shows.single().episodes.map { it.id })
        val after = awaitValue(model.state) { it.shows.isEmpty() }
        assertEquals(1, after.movies.size)
        assertTrue(show.state.value.seasons.isEmpty() || awaitValue(show.state) { it.empty }.empty)
    }

    @Test
    fun aFileThatKeepsFailingItsCheckFailsTheDownloadUntilRetry() {
        transport.bytes = { "something else entirely".encodeToByteArray() }
        val session = signIn()
        val model = DownloadsViewModel(session.downloads!!)
        runBlocking { session.downloads!!.download("m1", null, null, null, null) }
        val failed = awaitValue(model.state) { it.movies.singleOrNull()?.status is DownloadStatus.Failed }.movies.single()
        assertEquals(DownloadStatus.CHECK_FAILED, (failed.status as DownloadStatus.Failed).reason)
        assertEquals(3, transport.started.count { it.id.endsWith("part-0.mp4") }, "fetched again until the checks run out")

        transport.bytes = { movie }
        model.retry(failed.id)
        assertEquals(DownloadStatus.Ready, awaitValue(model.state) { it.movies.single().status == DownloadStatus.Ready }.movies.single().status)
    }

    @Test
    fun aReadyDownloadPlaysFromItsFileAndAnUnknownOneClosesThePlayer() {
        val session = signIn()
        val id = runBlocking { session.downloads!!.download("m1", null, null, null, null) }
        val downloads = DownloadsViewModel(session.downloads!!)
        awaitValue(downloads.state) { it.movies.singleOrNull()?.status == DownloadStatus.Ready }

        val engine = FakeEngine()
        val player = held { PlayerViewModel(session, PlayerRoute("m1", downloadId = id)) { engine } }
        val playing = awaitValue(player.state) { it.playback.status == PlayerStatus.Playing && it.work != null }
        val play = assertNotNull(playing.playback.play)
        assertTrue(play.url.startsWith("file://"), play.url)
        assertTrue(play.url.endsWith("/$id/part-0.mp4"), play.url)
        assertEquals(true, playing.offline)
        assertEquals("Charade", playing.work?.title)

        // Made alone: held would hand back the player above, its kind's one.
        val gone = PlayerViewModel(session, PlayerRoute("m1", downloadId = "nothing")) { FakeEngine() }
        assertIs<com.mmcdole.kivo.android.mobile.player.PlayerExit.Back>(runBlocking { withTimeout(5_000) { gone.exits.receive() } })
    }
}
