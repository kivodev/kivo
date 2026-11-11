package com.mmcdole.kivo.android.mobile.cast

import com.mmcdole.kivo.android.mobile.CoreTest
import com.mmcdole.kivo.android.mobile.app.PlayerRoute
import com.mmcdole.kivo.android.mobile.awaitValue
import com.mmcdole.kivo.android.mobile.waitFor
import com.mmcdole.kivo.android.mobile.reply
import com.mmcdole.kivo.api.models.CastToken
import com.mmcdole.kivo.api.models.MatchState
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.core.cast.CastLoad
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.util.concurrent.CopyOnWriteArrayList
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlin.time.Instant

/** A Cast session the test drives: what's loaded and sent, and the receiver's media as the test says. */
private class FakeLink : CastLink {
    override val state = MutableStateFlow<LinkState>(LinkState.Disconnected)
    override val media = MutableStateFlow<CastMedia?>(null)
    override val volume = MutableStateFlow(0.5f)
    override val messages = MutableSharedFlow<String>(extraBufferCapacity = 8)
    override var positionMs = 0L
    val loads = CopyOnWriteArrayList<CastLoad>()
    val sent = CopyOnWriteArrayList<String>()
    var ended: Boolean? = null

    override fun load(load: CastLoad, title: String, imageUrl: String?) {
        loads += load
    }

    override fun play() = Unit

    override fun pause() = Unit

    override fun seek(positionMs: Long) {
        this.positionMs = positionMs
    }

    override fun setVolume(volume: Float) {
        this.volume.value = volume
    }

    override fun send(message: String) {
        sent += message
    }

    override fun end(stopCasting: Boolean) {
        ended = stopCasting
        state.value = LinkState.Disconnected
    }

    override fun close() = Unit
}

/** Casting through the real core: a LOAD with a cast token and the server, the receiver's status mirrored, and stopping back to here. */
class CastViewModelTest : CoreTest() {
    init {
        server.on("POST", "/api/v1/cast-tokens") {
            reply(CastToken.serializer(), CastToken("ct", Instant.fromEpochMilliseconds(0), "s1", listOf("https://kivo.test")), HttpStatusCode.Created)
        }
        server.get("/api/v1/works/m1", WorkDetail.serializer()) {
            WorkDetail(
                id = "m1", kind = WorkKind.Movie, title = "Charade", matchState = MatchState.Matched, available = true, images = WorkImages(),
                genres = emptyList(), studios = emptyList(), credits = emptyList(), externalIds = emptyList(), versions = emptyList(),
                extras = emptyList(), collections = emptyList(), durationMs = 6_000_000,
            )
        }
    }

    @Test
    fun aNewSessionIsSaidSoThePlayHereCarriesOnAndItsLoadCarriesTheToken() {
        val link = FakeLink()
        val model = CastViewModel(signIn(), link)
        assertFalse(model.cast(PlayerRoute("m1")), "nothing to cast to yet")
        link.state.value = LinkState.Connected("Living Room TV", resumed = false)
        runBlocking { withTimeout(5_000) { model.connections.first() } }
        assertTrue(model.cast(PlayerRoute("m1", subtitleTrackId = "t1", positionMs = 90_000)))
        assertEquals("m1", model.state.value.loading)
        waitFor { link.loads.isNotEmpty() }
        val load = link.loads.single()
        assertEquals("m1", load.contentId)
        assertEquals(90.0, load.currentTimeSec)
        val custom = Json.parseToJsonElement(load.customData).jsonObject
        assertEquals("ct", custom["token"]?.jsonPrimitive?.content)
        assertEquals("s1", custom["serverId"]?.jsonPrimitive?.content)
        assertEquals("t1", custom["subtitleTrackId"]?.jsonPrimitive?.content)
        assertTrue(link.sent.any { it.contains("kivo.getStatus") })

        link.media.value = CastMedia("m1", CastMedia.State.Playing, 95_000, 6_000_000, """{"status":{"workId":"m1","versionId":"v1","audioTrackId":"a1","subtitleTrackId":"t1"}}""")
        val playing = awaitValue(model.state) { it.status != null && it.work != null }
        assertTrue(playing.active)
        assertNull(playing.loading)
        assertEquals("Charade", playing.work?.title)

        val here = model.stopCasting()
        assertEquals(PlayerRoute("m1", versionId = "v1", audioTrackId = "a1", subtitleTrackId = "t1", positionMs = 95_000), here)
        assertEquals(true, link.ended)
        assertFalse(awaitValue(model.state) { !it.connected }.active)
    }

    @Test
    fun aReceiverErrorSaysWhyAndASessionJoinedAgainSendsAFreshToken() {
        val link = FakeLink()
        val model = CastViewModel(signIn(), link)
        link.state.value = LinkState.Connected("Living Room TV", resumed = true)
        waitFor { link.sent.any { it.contains("kivo.token") } }
        link.messages.tryEmit("""{"type":"kivo.status","workId":"m1","error":"signedOut"}""")
        assertEquals(CastProblem.Receiver("signedOut", null), awaitValue(model.state) { it.problem != null }.problem)
    }

    @Test
    fun withoutGooglePlayServicesNothingCasts() {
        val model = CastViewModel(signIn(), null)
        assertFalse(model.offered)
        assertFalse(model.cast(PlayerRoute("m1")))
    }
}
