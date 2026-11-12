package com.mmcdole.kivo.core.session

import com.mmcdole.kivo.api.models.DownloadDelivery
import com.mmcdole.kivo.api.models.DownloadPart
import com.mmcdole.kivo.api.models.EpisodeInfo
import com.mmcdole.kivo.api.models.MatchState
import com.mmcdole.kivo.api.models.PlayDelivery
import com.mmcdole.kivo.api.models.PlayPart
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.PreviewManifest
import com.mmcdole.kivo.api.models.PreviewsDelivery
import com.mmcdole.kivo.api.models.Version
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.core.display.QualityOption
import com.mmcdole.kivo.core.downloads.db.DownloadDatabase
import com.mmcdole.kivo.core.empty
import com.mmcdole.kivo.core.json
import com.mmcdole.kivo.core.kivoJson
import com.mmcdole.kivo.core.play.FakeEngine
import com.mmcdole.kivo.core.play.PlayChoice
import com.mmcdole.kivo.core.play.PlayerStatus
import com.mmcdole.kivo.core.settings.Quality
import com.mmcdole.kivo.api.models.PlayCreate
import io.ktor.client.request.HttpRequestData
import io.ktor.http.HttpStatusCode
import io.ktor.http.content.OutgoingContent
import io.ktor.http.content.TextContent
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import java.io.IOException
import java.util.concurrent.CopyOnWriteArrayList
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue
import com.mmcdole.kivo.api.models.Download as ServerDownload
import com.mmcdole.kivo.core.downloads.db.Download as Row
import com.mmcdole.kivo.core.downloads.db.Download_file as FileRow

// The video player's screen as an app gets it from SessionPlayback.videoPlayer:
// the stream or the download it plays, the work and its next episode, the
// qualities, the scrub previews, where it goes at the end and the controls
// hiding by themselves, against a server answered by Ktor's mock engine.

/** A 10 Mb/s version, between the test ladder's 20 and 8 Mb/s rungs. */
private val version = Version("v1", true, 600_000, 1_000, emptyList(), emptyList(), emptyList(), "1080p", bitrateBps = 10_000_000)

private fun work(id: String, kind: WorkKind, episode: EpisodeInfo? = null) = WorkDetail(
    id, kind, "Title $id", MatchState.Matched, true, WorkImages(), emptyList(), emptyList(), emptyList(), emptyList(),
    listOf(version), emptyList(), emptyList(), episode = episode,
)

private fun episode(number: Int, nextId: String?) = EpisodeInfo(1, number, "s1", "Severance", nextId = nextId)

private val movie = work("m1", WorkKind.Movie)
private val first = work("e1", WorkKind.Episode, episode(1, nextId = "e2"))
private val last = work("e2", WorkKind.Episode, episode(2, nextId = null))

private fun play(id: String, workId: String, previews: String? = null) = PlayStarted(
    id = id, workId = workId, versionId = "v1", startMs = 0, durationMs = 600_000, local = true, delivery = PlayDelivery.File,
    url = "plays/$id/media", tracks = emptyList(), parts = listOf(PlayPart(600_000)), chapters = emptyList(),
    previews = previews?.let { PreviewsDelivery(ready = true, url = it) },
)

private fun manifest(sheet: String) = PreviewManifest(10_000, 160, 90, 10, 10, listOf(sheet))

/** A server with [works], whose plays each show the scrub previews [previews] names for it, in order. */
private class Players(private val scope: TestScope, withDownloads: Boolean = false, works: List<WorkDetail>) {
    val h = Harness(scope, withDownloads)
    val previews = mutableListOf<String?>()
    val starts: MutableList<String> = CopyOnWriteArrayList()

    init {
        val servers = h.servers
        servers.server("home.test", info("server-1"))
        h.device.signedInTo("server-1")
        for (w in works) servers.on("home.test", "GET", "/api/v1/works/${w.id}") { json(WorkDetail.serializer(), w) }
        servers.on("home.test", "POST", "/api/v1/plays") { request ->
            val workId = kivoJson.decodeFromString(PlayCreate.serializer(), request.text()).workId
            starts += workId
            val n = starts.size
            json(PlayStarted.serializer(), play("p$n", workId, previews.getOrNull(n - 1)), HttpStatusCode.Created)
        }
        for (n in 1..4) {
            servers.on("home.test", "PATCH", "/api/v1/plays/p$n") { empty() }
            servers.on("home.test", "POST", "/api/v1/plays/p$n/stop") { empty() }
        }
        for (sheet in listOf("a", "b")) {
            servers.on("home.test", "GET", "/api/v1/previews/$sheet") { json(PreviewManifest.serializer(), manifest(sheet)) }
        }
    }

    suspend fun session() = h.core.restore(home)

    /** The server answers nothing from now on. */
    fun unreachable() {
        for (path in listOf("/api/v1/server", "/api/v1/users/me", "/api/v1/plays") + listOf("m1", "e1", "e2").map { "/api/v1/works/$it" }) {
            h.servers.on("home.test", if (path == "/api/v1/plays") "POST" else "GET", path) { throw IOException("no route to host") }
        }
    }

    /** [work] downloaded as [id] and ready here, its one part on the device. */
    fun downloaded(id: String, work: WorkDetail) {
        val q = DownloadDatabase(h.device.downloads!!.driver).downloadsQueries
        val plan = ServerDownload(
            id = "r-$id", workId = work.id, versionId = "v1", quality = "high", state = "ready", delivery = DownloadDelivery.File,
            tracks = emptyList(), durationMs = 600_000, sizeBytes = 1, sizeExact = true,
            parts = listOf(DownloadPart(600_000, "downloads/r-$id/files/0", "video/mp4")), chapters = emptyList(),
        )
        q.insertDownload(
            Row(
                id, "server-1", "u1", null, work.id, null, null, null, "high", "ready", null, null, null,
                kivoJson.encodeToString(ServerDownload.serializer(), plan), kivoJson.encodeToString(WorkDetail.serializer(), work),
                work.title, "episode", "s1", "Severance", 1, work.episode?.number?.toLong(), 1, 1, 1, null,
            ),
        )
        q.insertFile(FileRow(id, "part-0.mp4", "part", "downloads/r-$id/files/0", 1, null, null, "done", 1, 0))
    }
}

private fun HttpRequestData.text(): String = when (val content = body) {
    is TextContent -> content.text
    is OutgoingContent.ByteArrayContent -> content.bytes().decodeToString()
    else -> ""
}

/** The player once its play is playing: ready on [engine], and the work read with the version playing. */
private suspend fun VideoPlayer.playing(engine: FakeEngine, playId: String? = null): VideoPlayer {
    controller.state.first { it.play != null && (playId == null || it.play?.id == playId) }
    engine.current.onReady()
    controller.state.first { it.status == PlayerStatus.Playing }
    screen.first { it.version != null }
    return this
}

/** Where the player goes once its play plays to the end. */
private suspend fun VideoPlayer.end(engine: FakeEngine): PlayerExit? {
    engine.current.onEnded()
    return screen.first { it.exit != null }.exit
}

class VideoPlayerTest {
    @Test
    fun aChoiceWithoutADownloadPlaysTheServersStream() = runTest {
        val p = Players(this, works = listOf(movie))
        val engine = FakeEngine()
        val player = p.session().playback.videoPlayer(engine, PlayChoice("m1"), hideControlsAfterMs = 3_000).playing(engine)

        assertEquals(listOf("m1"), p.starts)
        assertEquals("plays/p1/media", engine.loads.single().url)
        val screen = player.screen.value
        assertFalse(screen.offline)
        assertNull(screen.downloadSource)
        assertEquals("m1", screen.work?.id)
        assertEquals("v1", screen.version?.id)
        player.dispose()
    }

    @Test
    fun aDownloadPlaysFromItsFilesWithoutTheServer() = runTest {
        val p = Players(this, withDownloads = true, works = listOf(first, last))
        p.downloaded("d1", first)
        val session = p.session()
        p.unreachable()
        val engine = FakeEngine()
        val player = session.playback.videoPlayer(engine, PlayChoice("e1"), hideControlsAfterMs = 3_000, downloadId = "d1").playing(engine)

        assertEquals(emptyList(), p.starts, "the server isn't asked")
        assertTrue(engine.loads.single().url.startsWith("file://"), engine.loads.single().url)
        val screen = player.screen.value
        assertTrue(screen.offline)
        assertEquals("e1", screen.work?.id, "the work as it was downloaded")
        assertNull(screen.downloadSource?.convertedFrom, "a download of the file as it is")
        player.dispose()
    }

    @Test
    fun aDownloadThatCantPlayNowGoesBack() = runTest {
        val p = Players(this, withDownloads = true, works = listOf(movie))
        val engine = FakeEngine()
        val player = p.session().playback.videoPlayer(engine, PlayChoice("m1"), hideControlsAfterMs = 3_000, downloadId = "gone")

        assertEquals(PlayerExit.Back, player.screen.first { it.exit != null }.exit)
        assertTrue(engine.loads.isEmpty())
        assertEquals(emptyList(), p.starts)
        player.dispose()
    }

    @Test
    fun aMovieGoesBackAtItsEnd() = runTest {
        val p = Players(this, works = listOf(movie))
        val engine = FakeEngine()
        val player = p.session().playback.videoPlayer(engine, PlayChoice("m1"), hideControlsAfterMs = 3_000).playing(engine)

        assertNull(player.screen.value.next)
        assertEquals(PlayerExit.Back, player.end(engine))
        player.dispose()
    }

    @Test
    fun anEpisodeWithANextOneReadsItAndPlaysItAtItsEnd() = runTest {
        val p = Players(this, works = listOf(first, last))
        val engine = FakeEngine()
        val player = p.session().playback.videoPlayer(engine, PlayChoice("e1"), hideControlsAfterMs = 3_000).playing(engine)

        assertEquals("e2", player.screen.first { it.next != null }.next?.id)
        assertEquals(PlayerExit.Next(PlayChoice("e2"), downloadId = null), player.end(engine))
        player.dispose()
    }

    @Test
    fun theNextEpisodePlaysItsDownloadWhenOneIsReady() = runTest {
        val p = Players(this, withDownloads = true, works = listOf(first, last))
        p.downloaded("d2", last)
        val engine = FakeEngine()
        val player = p.session().playback.videoPlayer(engine, PlayChoice("e1"), hideControlsAfterMs = 3_000).playing(engine)

        player.screen.first { it.next != null }
        player.playNext()
        assertEquals(PlayerExit.Next(PlayChoice("e2"), downloadId = "d2"), player.screen.first { it.exit != null }.exit, "Play now, from the download")
        player.dispose()
    }

    @Test
    fun theLastEpisodeGivesWayToItsSeries() = runTest {
        val p = Players(this, works = listOf(first, last))
        val engine = FakeEngine()
        val player = p.session().playback.videoPlayer(engine, PlayChoice("e2"), hideControlsAfterMs = 3_000).playing(engine)

        assertNull(player.screen.value.next)
        assertEquals(PlayerExit.Series("s1"), player.end(engine))
        player.dispose()
    }

    @Test
    fun aDownloadedLastEpisodeGoesBackWhereItWasPlayedFrom() = runTest {
        val p = Players(this, withDownloads = true, works = listOf(first, last))
        p.downloaded("d2", last)
        val engine = FakeEngine()
        val player = p.session().playback.videoPlayer(engine, PlayChoice("e2"), hideControlsAfterMs = 3_000, downloadId = "d2").playing(engine)

        assertEquals(PlayerExit.Back, player.end(engine), "not its series' page, even with the server there")
        player.dispose()
    }

    @Test
    fun offlineTheNextEpisodeIsOnlyOneDownloaded() = runTest {
        val p = Players(this, withDownloads = true, works = listOf(first, last))
        p.downloaded("d1", first)
        val session = p.session()
        p.unreachable()
        val engine = FakeEngine()
        val alone = session.playback.videoPlayer(engine, PlayChoice("e1"), hideControlsAfterMs = 3_000, downloadId = "d1").playing(engine)

        assertNull(alone.screen.value.next, "its next isn't downloaded, and the server doesn't answer")
        assertEquals(PlayerExit.Back, alone.end(engine), "so it ends as the last one does: back, as its series needs the server")
        alone.dispose()

        p.downloaded("d2", last)
        val engine2 = FakeEngine()
        val player = session.playback.videoPlayer(engine2, PlayChoice("e1"), hideControlsAfterMs = 3_000, downloadId = "d1").playing(engine2)
        assertEquals("e2", player.screen.first { it.next != null }.next?.id, "as it was downloaded")
        assertEquals(PlayerExit.Next(PlayChoice("e2"), downloadId = "d2"), player.end(engine2))
        player.dispose()
    }

    @Test
    fun theQualitiesAreTheLadderBelowTheVersionsBitrate() = runTest {
        val p = Players(this, works = listOf(movie))
        val engine = FakeEngine()
        val player = p.session().playback.videoPlayer(engine, PlayChoice("m1"), hideControlsAfterMs = 3_000).playing(engine)

        val qualities = player.screen.first { it.qualities.size > 2 }.qualities
        assertEquals(listOf(Quality.Auto, Quality.Original, Quality.Limit(8_000_000)), qualities.map(QualityOption::quality))
        player.dispose()
    }

    @Test
    fun thePreviewsAreReadAgainAsANewPlayBringsOthers() = runTest {
        val p = Players(this, works = listOf(movie))
        p.previews += listOf("previews/a", "previews/a", "previews/b")
        val engine = FakeEngine()
        val player = p.session().playback.videoPlayer(engine, PlayChoice("m1"), hideControlsAfterMs = 3_000).playing(engine)
        assertEquals(listOf("a"), player.screen.first { it.previews != null }.previews?.manifest?.sheets)

        player.chooseQuality(Quality.Limit(8_000_000))
        player.playing(engine, "p2")
        assertEquals(listOf("a"), player.screen.value.previews?.manifest?.sheets, "the same previews stay")
        assertEquals(1, p.h.servers.seen.count { it == "home.test GET /api/v1/previews/a" }, "and aren't read again")

        player.chooseQuality(Quality.Original)
        player.playing(engine, "p3")
        assertEquals(listOf("b"), player.screen.first { it.previews?.manifest?.sheets == listOf("b") }.previews?.manifest?.sheets)
        player.dispose()
    }

    @Test
    fun theControlsHideByThemselvesAfterTheWaitGiven() = runTest {
        val p = Players(this, works = listOf(movie))
        val engine = FakeEngine()
        val player = p.session().playback.videoPlayer(engine, PlayChoice("m1"), hideControlsAfterMs = 5_000).playing(engine)

        player.showControls()
        advanceTimeBy(4_999)
        runCurrent()
        assertTrue(player.screen.value.ui.controlsShown)
        advanceTimeBy(1)
        runCurrent()
        assertFalse(player.screen.value.ui.controlsShown, "5 seconds after the last input")

        player.setMenuOpen(true)
        advanceTimeBy(60_000)
        runCurrent()
        assertTrue(player.screen.value.ui.controlsShown, "a menu holds them")
        player.setMenuOpen(false)
        engine.pause()
        runCurrent()
        advanceTimeBy(60_000)
        runCurrent()
        assertTrue(player.screen.value.ui.controlsShown, "and so does a pause")
        player.dispose()
    }
}
