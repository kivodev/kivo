package com.mmcdole.kivo.android.mobile.player

import com.mmcdole.kivo.android.mobile.CoreTest
import com.mmcdole.kivo.android.mobile.app.PlayerRoute
import com.mmcdole.kivo.android.mobile.awaitValue
import com.mmcdole.kivo.android.mobile.reply
import com.mmcdole.kivo.api.models.MatchState
import com.mmcdole.kivo.api.models.PlayCreate
import com.mmcdole.kivo.api.models.PlayDelivery
import com.mmcdole.kivo.api.models.PlayPart
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.PlayTrack
import com.mmcdole.kivo.api.models.TrackDelivery
import com.mmcdole.kivo.api.models.TrackType
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.core.play.PlayerStatus
import com.mmcdole.kivo.core.player.TrackShow
import io.ktor.client.engine.mock.respond
import io.ktor.client.request.HttpRequestData
import io.ktor.http.HttpStatusCode
import io.ktor.http.content.OutgoingContent
import io.ktor.http.content.TextContent
import kotlinx.serialization.json.Json
import java.util.concurrent.CopyOnWriteArrayList
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertIs

class PlayerViewModelTest : CoreTest() {
    private val json = Json { ignoreUnknownKeys = true }

    /** Every play asked for, in order. */
    private val starts = CopyOnWriteArrayList<PlayCreate>()

    private fun play(id: String, subtitle: PlayTrack) = PlayStarted(
        id = id,
        workId = "m1",
        versionId = "v2",
        startMs = 0,
        durationMs = 600_000,
        local = true,
        delivery = PlayDelivery.File,
        url = "plays/$id/files/0",
        tracks = listOf(PlayTrack("a2", TrackType.Audio, selected = true, delivery = TrackDelivery.File, index = 2), subtitle),
        parts = listOf(PlayPart(600_000)),
        chapters = emptyList(),
    )

    private val drawn = PlayTrack("t1", TrackType.Subtitle, selected = true, delivery = TrackDelivery.External, url = "plays/p1/subtitles/t1", codec = "ass")
    private val burned = PlayTrack("t1", TrackType.Subtitle, selected = true, delivery = TrackDelivery.Burn)

    init {
        server.get("/api/v1/works/m1", WorkDetail.serializer()) {
            WorkDetail(
                id = "m1", kind = WorkKind.Movie, title = "Charade", matchState = MatchState.Matched, available = true, images = WorkImages(),
                genres = emptyList(), studios = emptyList(), credits = emptyList(), externalIds = emptyList(), versions = emptyList(),
                extras = emptyList(), collections = emptyList(), similar = emptyList(),
            )
        }
        server.on("POST", "/api/v1/plays") { request ->
            val body = json.decodeFromString(PlayCreate.serializer(), request.text())
            starts += body
            // Asked without subtitle files, as in picture in picture, the server burns the subtitle in.
            val pip = starts.size == 2
            reply(PlayStarted.serializer(), play("p${starts.size}", if (pip) burned else drawn), HttpStatusCode.Created)
        }
        for (id in listOf("p1", "p2", "p3")) {
            server.on("POST", "/api/v1/plays/$id/stop") { respond("", HttpStatusCode.NoContent) }
            server.on("PATCH", "/api/v1/plays/$id") { respond("", HttpStatusCode.NoContent) }
        }
    }

    private fun HttpRequestData.text(): String = when (val content = body) {
        is TextContent -> content.text
        is OutgoingContent.ByteArrayContent -> content.bytes().decodeToString()
        else -> ""
    }

    private fun model(engine: FakeEngine, route: PlayerRoute = PlayerRoute("m1", versionId = "v2", audioTrackId = "a2", subtitleTrackId = "t1")) =
        held { PlayerViewModel(signIn(), route) { engine } }

    @Test
    fun thePagesChoicesStartThePlay() {
        val engine = FakeEngine()
        val model = model(engine)
        val state = awaitValue(model.state) { it.playback.status == PlayerStatus.Playing && it.screen.work != null }
        val start = starts.single()
        assertEquals("v2", start.versionId)
        assertEquals("a2", start.audioTrackId)
        assertEquals("t1", start.subtitleTrackId)
        assertEquals("Charade", state.screen.work?.title)
        assertIs<TrackShow.External>(engine.shown.last())
    }

    @Test
    fun pictureInPictureBurnsInASubtitleTheAppDrawsAndLeavingDrawsItAgain() {
        val engine = FakeEngine()
        val model = model(engine)
        awaitValue(model.state) { it.playback.play?.id == "p1" && it.playback.status == PlayerStatus.Playing }
        model.setPictureInPicture(true)
        val inPip = awaitValue(model.state) { it.playback.play?.id == "p2" }
        assertEquals(true, inPip.pictureInPicture)
        assertEquals("t1", starts[1].subtitleTrackId)
        awaitValue(model.state) { it.playback.status == PlayerStatus.Playing }
        model.setPictureInPicture(false)
        awaitValue(model.state) { it.playback.play?.id == "p3" }
        assertEquals(3, starts.size)
    }

    @Test
    fun speedHoldsOnTheEngine() {
        val engine = FakeEngine()
        val model = model(engine)
        model.chooseSpeed(1.5f)
        assertEquals(1.5f, awaitValue(model.state) { it.speed == 1.5f }.speed)
        assertEquals(1.5f, engine.speed)
    }
}
