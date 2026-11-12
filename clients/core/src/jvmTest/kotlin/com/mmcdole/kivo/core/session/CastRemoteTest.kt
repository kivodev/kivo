package com.mmcdole.kivo.core.session

import com.mmcdole.kivo.api.models.CastToken
import com.mmcdole.kivo.api.models.EpisodeInfo
import com.mmcdole.kivo.api.models.MatchState
import com.mmcdole.kivo.api.models.Version
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.cast.CastLink
import com.mmcdole.kivo.core.cast.CastLinkState
import com.mmcdole.kivo.core.cast.CastLoad
import com.mmcdole.kivo.core.cast.CastMedia
import com.mmcdole.kivo.core.json
import com.mmcdole.kivo.core.play.PlayChoice
import com.mmcdole.kivo.core.problem
import com.mmcdole.kivo.core.settings.Quality
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertIs
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlin.time.Instant

// The cast remote as an app gets it from SessionCast.remote: casting with a
// cast token, the receiver's media and status mirrored, the position kept
// moving, the work and its next episode read, problems and Try again, and
// stopping back to here, over a Cast session the test drives and a server
// answered by Ktor's mock engine.

private val version = Version("v1", true, 6_000_000, 1_000, emptyList(), emptyList(), emptyList(), "1080p")

private fun work(id: String, kind: WorkKind, title: String, episode: EpisodeInfo? = null) = WorkDetail(
    id, kind, title, MatchState.Matched, true, WorkImages(), emptyList(), emptyList(), emptyList(), emptyList(),
    listOf(version), emptyList(), emptyList(), durationMs = 6_000_000, episode = episode,
)

private val movie = work("m1", WorkKind.Movie, "Charade")
private val first = work("e1", WorkKind.Episode, "Pilot", EpisodeInfo(1, 1, "s1", "Severance", nextId = "e2"))
private val second = work("e2", WorkKind.Episode, "Half Loop", EpisodeInfo(1, 2, "s1", "Severance"))

/** A Cast session the test drives: what's loaded, sent and sought, and the receiver as the test says. */
private class FakeLink : CastLink {
    lateinit var events: CastLink.Events
    override var positionMs = 0L
    val loads = MutableStateFlow<List<CastLoad>>(emptyList())
    val sent = MutableStateFlow<List<String>>(emptyList())
    val seeks = mutableListOf<Long>()
    var ended: Boolean? = null
    var closed = false

    override fun start(events: CastLink.Events) {
        this.events = events
        events.onLink(CastLinkState.Disconnected)
    }

    override fun load(load: CastLoad, title: String, imageUrl: String?) {
        loads.value += load
    }

    override fun play() = Unit

    override fun pause() = Unit

    override fun seek(positionMs: Long) {
        seeks += positionMs
    }

    override fun setSpeed(speed: Float) = Unit

    override fun setVolume(volume: Float) = Unit

    override fun send(message: String) {
        sent.value += message
    }

    override fun end(stopCasting: Boolean) {
        ended = stopCasting
    }

    override fun close() {
        closed = true
    }

    fun connect(resumed: Boolean = false) = events.onLink(CastLinkState.Connected("Living Room TV", resumed))

    fun media(workId: String, state: CastMedia.State, positionMs: Long, status: String? = null) =
        events.onMedia(CastMedia(workId, state, positionMs, 6_000_000, 1f, status))

    fun types(): List<String?> = sent.value.map { Json.parseToJsonElement(it).jsonObject["type"]?.jsonPrimitive?.content }
}

/** A signed-in session whose server gives cast tokens, unless [refuseTokens], and has the test's works. */
private suspend fun TestScope.session(refuseTokens: Boolean = false): KivoSession {
    val h = Harness(this)
    h.servers.server("home.test", info("server-1"))
    h.device.signedInTo("server-1")
    h.servers.on("home.test", "POST", "/api/v1/cast-tokens") {
        if (refuseTokens) {
            problem(HttpStatusCode.Forbidden, "forbidden")
        } else {
            json(CastToken.serializer(), CastToken("ct", Instant.fromEpochMilliseconds(0), "server-1", listOf("http://home.test")), HttpStatusCode.Created)
        }
    }
    for (w in listOf(movie, first, second)) h.servers.on("home.test", "GET", "/api/v1/works/${w.id}") { json(WorkDetail.serializer(), w) }
    return h.core.restore(home)
}

private suspend fun CastRemote.until(test: (CastRemoteState) -> Boolean): CastRemoteState = state.first(test)

class CastRemoteTest {
    @Test
    fun aNewConnectionIsSaidAndACastLoadsTheChoiceWithACastToken() = runTest {
        val link = FakeLink()
        val remote = session().cast.remote(link)
        assertFalse(remote.cast(PlayChoice("m1")), "nothing to cast to yet")
        val connections = mutableListOf<String>()
        backgroundScope.launch { remote.connections.collect { connections += it } }
        runCurrent()

        link.connect()
        runCurrent()

        assertEquals(listOf("Living Room TV"), connections)
        assertEquals("Living Room TV", remote.state.value.device)
        assertEquals(listOf("kivo.getStatus"), link.types())
        assertTrue(remote.cast(PlayChoice("m1", subtitleTrackId = "t1"), positionMs = 90_000))
        assertEquals("m1", remote.state.value.loading)
        assertTrue(remote.state.value.active, "a cast on its way shows the remote")
        val load = link.loads.first { it.isNotEmpty() }.single()
        assertEquals("m1", load.contentId)
        assertEquals(90.0, load.currentTimeSec)
        val custom = Json.parseToJsonElement(load.customData).jsonObject
        assertEquals("ct", custom["token"]?.jsonPrimitive?.content)
        assertEquals("t1", custom["subtitleTrackId"]?.jsonPrimitive?.content)
        remote.close()
        assertTrue(link.closed)
    }

    @Test
    fun theReceiversMediaAndStatusShowWithTheWorkRead() = runTest {
        val link = FakeLink()
        val remote = session().cast.remote(link)
        link.connect()
        remote.cast(PlayChoice("m1"))
        link.loads.first { it.isNotEmpty() }

        link.media("m1", CastMedia.State.Paused, 95_000, """{"status":{"workId":"m1","versionId":"v1","audioTrackId":"a1","subtitleTrackId":"t1","quality":{"mode":"original"}}}""")
        val shown = remote.until { it.work != null && it.qualities.isNotEmpty() }

        assertNull(shown.loading)
        assertTrue(shown.active)
        assertFalse(shown.playing)
        assertEquals(95_000, shown.positionMs)
        assertEquals("Charade", shown.work?.title)
        assertEquals("v1", shown.version?.id)
        assertEquals("t1", shown.status?.subtitleTrackId)
        assertEquals(Quality.Original, shown.quality)

        remote.chooseQuality(Quality.Auto)
        assertEquals(Quality.Auto, remote.state.value.quality, "the choice shows before the receiver says so")
        assertEquals("kivo.setQuality", link.types().last())
    }

    @Test
    fun aSeekStopsShortOfTheEndAndBeforeTheStart() = runTest {
        val link = FakeLink()
        val remote = session().cast.remote(link)
        link.connect()
        link.media("m1", CastMedia.State.Paused, 60_000)

        remote.seek(10_000_000)
        assertEquals(5_999_000, remote.state.value.positionMs)
        remote.skip(-10_000_000)
        assertEquals(0, remote.state.value.positionMs)
        remote.skip(30_000)
        assertEquals(listOf(5_999_000L, 0L, 30_000L), link.seeks)
    }

    @Test
    fun thePositionMovesWhileTheTvPlaysAndStopsWhenItPauses() = runTest {
        val link = FakeLink()
        val remote = session().cast.remote(link)
        link.connect()
        link.media("m1", CastMedia.State.Playing, 10_000)
        link.positionMs = 10_600

        advanceTimeBy(501)
        assertEquals(10_600, remote.state.value.positionMs)

        link.media("m1", CastMedia.State.Paused, 11_000)
        link.positionMs = 20_000
        advanceTimeBy(2_000)
        assertEquals(11_000, remote.state.value.positionMs)
        remote.close()
    }

    @Test
    fun anEpisodesNextIsReadForUpNext() = runTest {
        val link = FakeLink()
        val remote = session().cast.remote(link)
        link.connect()

        link.events.onMessage("""{"type":"kivo.status","workId":"e1","nextWorkId":"e2","versionId":"v1","upNext":{"phase":"countingDown","secondsLeft":7}}""")
        val shown = remote.until { it.work != null && it.next != null }

        assertEquals("e1", shown.work?.id)
        assertEquals("Half Loop", shown.next?.title)
        assertTrue(shown.status?.upNext?.shows == true)
        remote.playNext()
        remote.hideUpNext()
        assertEquals(listOf("kivo.playNext", "kivo.hideUpNext"), link.types().takeLast(2))
    }

    @Test
    fun aRefusedTokenSaysWhyAndTryAgainCastsTheSameAgain() = runTest {
        val link = FakeLink()
        val remote = session(refuseTokens = true).cast.remote(link)
        link.connect()
        remote.cast(PlayChoice("m1", versionId = "v1"), positionMs = 0)

        val refused = remote.until { it.problem != null }
        assertNull(refused.loading)
        assertTrue(refused.active, "the remote stays to say why")
        assertIs<RequestProblem.Refused>(assertIs<CastProblem.Refused>(refused.problem).problem)

        remote.retry()
        assertEquals("m1", remote.state.value.loading)
        assertNull(remote.state.value.problem)
        remote.until { it.problem != null }
    }

    @Test
    fun aReceiverErrorStopsTheLoadAndAnUnreachableServerClearsOnceItAnswers() = runTest {
        val link = FakeLink()
        val remote = session().cast.remote(link)
        link.connect()
        remote.cast(PlayChoice("m1"))

        link.events.onMessage("""{"type":"kivo.status","workId":"m1","error":"signedOut"}""")
        assertEquals(CastProblem.Receiver("signedOut", null), remote.state.value.problem)
        assertNull(remote.state.value.loading)

        link.events.onMessage("""{"type":"kivo.status","workId":"m1","versionId":"v1","error":"serverUnreachable"}""")
        assertEquals(CastProblem.Unreachable, remote.state.value.problem)
        link.events.onMessage("""{"type":"kivo.status","workId":"m1","versionId":"v1"}""")
        assertNull(remote.state.value.problem)
    }

    @Test
    fun aDroppedSessionClearsTheCastAndTryAgainResumesFromTheTvsPosition() = runTest {
        val link = FakeLink()
        val remote = session().cast.remote(link)
        link.connect()
        link.media("m1", CastMedia.State.Paused, 120_000, """{"status":{"workId":"m1","versionId":"v1","audioTrackId":"a1"}}""")
        link.events.onMessage("""{"type":"kivo.status","workId":"m1","versionId":"v1","audioTrackId":"a1","error":"playbackFailed"}""")
        assertIs<CastProblem.Receiver>(remote.state.value.problem)

        remote.retry()
        val load = link.loads.first { it.isNotEmpty() }.single()
        assertEquals(120.0, load.currentTimeSec)
        val custom = Json.parseToJsonElement(load.customData).jsonObject
        assertEquals("v1", custom["versionId"]?.jsonPrimitive?.content)
        assertEquals("a1", custom["audioTrackId"]?.jsonPrimitive?.content)

        link.events.onLink(CastLinkState.Connecting)
        val dropped = remote.state.value
        assertFalse(dropped.active)
        assertNull(dropped.media)
        assertNull(dropped.status)
        assertNull(dropped.loading)
    }

    @Test
    fun aSessionJoinedAgainSendsAFreshTokenAndIsNotANewConnection() = runTest {
        val link = FakeLink()
        val remote = session().cast.remote(link)
        val connections = mutableListOf<String>()
        backgroundScope.launch { remote.connections.collect { connections += it } }
        runCurrent()

        link.connect(resumed = true)
        link.sent.first { "kivo.token" in link.types() }

        assertEquals(emptyList(), connections)
        assertTrue("kivo.getStatus" in link.types())
    }

    @Test
    fun stopCastingEndsTheSessionAndSaysWhereToCarryOnHere() = runTest {
        val link = FakeLink()
        val remote = session().cast.remote(link)
        link.connect()
        link.media("e1", CastMedia.State.Paused, 300_000, """{"status":{"workId":"e1","versionId":"v1","audioTrackId":"a1","subtitleTrackId":"t1"}}""")

        assertEquals(ResumePoint(PlayChoice("e1", "v1", "a1", "t1"), 300_000), remote.stopCasting())
        assertEquals(true, link.ended)
        assertEquals(CastLinkState.Disconnected, remote.state.value.link)
        assertFalse(remote.state.value.active)
        assertNull(remote.stopCasting(), "nothing to carry on with once it's stopped")
    }

    @Test
    fun disconnectLeavesTheTvPlaying() = runTest {
        val link = FakeLink()
        val remote = session().cast.remote(link)
        link.connect()
        link.media("m1", CastMedia.State.Paused, 1_000)

        remote.disconnect()
        assertEquals(false, link.ended)
        assertFalse(remote.state.value.connected)
    }
}
