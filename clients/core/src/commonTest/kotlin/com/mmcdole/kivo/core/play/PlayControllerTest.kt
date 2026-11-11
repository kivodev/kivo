package com.mmcdole.kivo.core.play

import com.mmcdole.kivo.api.models.Capabilities
import com.mmcdole.kivo.api.models.FileCapabilities
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.Play
import com.mmcdole.kivo.api.models.PlayDelivery
import com.mmcdole.kivo.api.models.PlayPart
import com.mmcdole.kivo.api.models.PlayTrack
import com.mmcdole.kivo.api.models.QualityRequest
import com.mmcdole.kivo.api.models.ServerInfo
import com.mmcdole.kivo.api.models.SubtitleEntry
import com.mmcdole.kivo.api.models.TrackDelivery
import com.mmcdole.kivo.api.models.TrackReason
import com.mmcdole.kivo.api.models.TrackType
import com.mmcdole.kivo.core.FakeServer
import com.mmcdole.kivo.core.auth.PICK_REST
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.caps.withoutExternal
import com.mmcdole.kivo.core.player.TrackShow
import com.mmcdole.kivo.core.empty
import com.mmcdole.kivo.core.json
import com.mmcdole.kivo.core.problem
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.io.IOException
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.int
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlin.time.Duration.Companion.seconds

// The web player controller's cases, against a fake engine and Ktor's mock
// engine.

private const val START = "/api/v1/plays"

private fun play(id: String, startMs: Int = 0, delivery: PlayDelivery = PlayDelivery.File, tracks: List<PlayTrack> = emptyList()) = PlayStarted(
    id = id,
    workId = "w1",
    versionId = "v1",
    startMs = startMs,
    durationMs = 3_600_000,
    local = true,
    delivery = delivery,
    url = "plays/$id/media",
    tracks = tracks,
    parts = listOf(PlayPart(3_600_000)),
    chapters = emptyList(),
)

/** A direct play's tracks: the file's two audio tracks, a subtitle in it, one drawn as a file, one only a new play shows, and off. */
private fun fileTracks(subtitle: String = "off") = listOf(
    PlayTrack("a1", TrackType.Audio, selected = true, delivery = TrackDelivery.File, index = 1),
    PlayTrack("a2", TrackType.Audio, delivery = TrackDelivery.File, index = 2),
    PlayTrack("a3", TrackType.Audio),
    PlayTrack("t4", TrackType.Subtitle, delivery = TrackDelivery.File, index = 3),
    PlayTrack("t5", TrackType.Subtitle, delivery = TrackDelivery.External, codec = "hdmv_pgs_subtitle", url = "plays/s1/subtitles/5.sup"),
    PlayTrack("t6", TrackType.Subtitle),
    PlayTrack("off", TrackType.Subtitle, delivery = TrackDelivery.None),
).map { if (it.trackId == subtitle) it.copy(selected = true) else it }

private val capabilities = Capabilities(
    video = emptyList(),
    audio = emptyList(),
    subtitles = listOf(SubtitleEntry("webvtt", listOf("stream", "external"))),
    file = FileCapabilities(containers = listOf("mkv")),
)

private fun body(text: String) = Json.parseToJsonElement(text).jsonObject

/** The deliveries of a play request's first subtitle entry, as JSON. */
private fun subtitleDelivery(body: JsonObject) =
    body["capabilities"]?.jsonObject?.get("subtitles")?.jsonArray?.first()?.jsonObject?.get("delivery").toString()

private fun FakeServer.plays(connection: ServerConnection = connection()) = ServerPlays(connection.plays, connection.reachability)

private class Harness(scope: TestScope, canFallBack: Boolean = true) {
    val server = FakeServer(StandardTestDispatcher(scope.testScheduler))
    val engine = FakeEngine()
    var endedCalls = 0
    val controller = PlayController(
        plays = server.plays(),
        engine = engine,
        capabilities = { external -> if (external) capabilities else withoutExternal(capabilities) },
        scope = scope.backgroundScope,
        canFallBack = canFallBack,
        onEnded = { endedCalls++ },
    )

    fun answerStarts(vararg plays: PlayStarted) {
        val queue = ArrayDeque(plays.toList())
        server.on("POST", START) { json(PlayStarted.serializer(), queue.removeFirst(), HttpStatusCode.Created) }
    }

    fun acceptBeats(id: String) {
        server.on("PATCH", "$START/$id") { empty() }
        server.on("POST", "$START/$id/stop") { empty() }
    }

    fun beats(id: String) = server.to("PATCH", "$START/$id").map { body(it.body) }

    fun stops(id: String) = server.to("POST", "$START/$id/stop").map { body(it.body) }
}

class PlayControllerTest {
    @Test
    fun startsWithTheReportAndHeartbeatsEveryTenSecondsAndOnPauseSeekAndEnd() = runTest {
        val h = Harness(this)
        h.answerStarts(play("s1", startMs = 5_000))
        h.acceptBeats("s1")

        h.controller.play(PlayRequest("w1", positionMs = 5_000))
        runCurrent()
        val start = body(h.server.to("POST", START).single().body)
        assertNull(start["device"])
        assertEquals(5_000, start["positionMs"]?.jsonPrimitive?.int)
        assertEquals("""["mkv"]""", start["capabilities"]?.jsonObject?.get("file")?.jsonObject?.get("containers").toString())
        assertEquals(listOf("s1"), h.engine.loads.map { it.id })
        assertEquals(PlayerStatus.Loading, h.controller.state.value.status)

        h.engine.current.onReady()
        assertEquals(PlayerStatus.Playing, h.controller.state.value.status)
        assertEquals(false, h.controller.state.value.paused)

        h.engine.positionMs = 15_000
        advanceTimeBy(HEARTBEAT_INTERVAL)
        runCurrent()
        assertEquals("playing", h.beats("s1").single()["state"]?.jsonPrimitive?.content)
        assertEquals(15_000, h.beats("s1").single()["positionMs"]?.jsonPrimitive?.int)

        h.engine.pause()
        runCurrent()
        assertEquals("paused", h.beats("s1").last()["state"]?.jsonPrimitive?.content)

        h.controller.seek(60_000)
        assertEquals(PlayerStatus.Seeking, h.controller.state.value.status)
        h.engine.current.onSeeked()
        runCurrent()
        assertNotNull(h.beats("s1").last()["seekToFirstFrameMs"])
        assertEquals(60_000, h.beats("s1").last()["positionMs"]?.jsonPrimitive?.int)

        h.engine.current.onEnded()
        runCurrent()
        assertEquals("ended", h.beats("s1").last()["state"]?.jsonPrimitive?.content)
        assertEquals(1, h.endedCalls)
        assertTrue(h.controller.state.value.ended)
    }

    @Test
    fun lateAnswersAreDropped() = runTest {
        val h = Harness(this)
        val firstAnswer = CompletableDeferred<Unit>()
        var starts = 0
        h.server.on("POST", START) {
            starts++
            if (starts == 1) {
                firstAnswer.await()
                json(PlayStarted.serializer(), play("s1"), HttpStatusCode.Created)
            } else {
                json(PlayStarted.serializer(), play("s2"), HttpStatusCode.Created)
            }
        }
        h.acceptBeats("s1")
        h.acceptBeats("s2")

        h.controller.play(PlayRequest("w1"))
        runCurrent()
        h.controller.play(PlayRequest("w1", audioTrackId = "a2"))
        runCurrent()
        firstAnswer.complete(Unit)
        runCurrent()

        // The superseded start's play is stopped at once and never loaded.
        assertEquals(listOf("s2"), h.engine.loads.map { it.id })
        assertEquals("s2", h.controller.current?.id)
        assertEquals(1, h.stops("s1").size)

        // Reports from a load that's been replaced are ignored too.
        val old = h.engine.current
        h.controller.switchTo { it.copy(subtitleTrackId = "off") }
        h.answerStarts(play("s3"))
        h.acceptBeats("s3")
        runCurrent()
        old.onEnded()
        old.onFailure(com.mmcdole.kivo.core.player.PlayerError("networkError", "late"))
        runCurrent()
        assertEquals(0, h.endedCalls)
        assertTrue(h.server.to("POST", "$START/s2/fallback").isEmpty())
    }

    @Test
    fun failedMediaFallsBackTwiceThenShowsTheError() = runTest {
        val h = Harness(this)
        h.answerStarts(play("s1", delivery = PlayDelivery.Stream))
        h.acceptBeats("s1")
        h.acceptBeats("s2")
        h.acceptBeats("s3")
        h.server.on("POST", "$START/s1/fallback") { json(PlayStarted.serializer(), play("s2", startMs = 42_000), HttpStatusCode.Created) }
        h.server.on("POST", "$START/s2/fallback") { json(PlayStarted.serializer(), play("s3", startMs = 42_000), HttpStatusCode.Created) }

        h.controller.play(PlayRequest("w1"))
        runCurrent()
        h.engine.current.onReady()
        h.engine.positionMs = 42_000
        h.engine.fail("mediaDecodeError", "x".repeat(2_000))
        assertTrue(h.controller.state.value.adjusting)
        runCurrent()

        val fallback = body(h.server.to("POST", "$START/s1/fallback").single().body)
        assertEquals(42_000, fallback["positionMs"]?.jsonPrimitive?.int)
        assertEquals("mediaDecodeError", fallback["code"]?.jsonPrimitive?.content)
        assertEquals(1_000, fallback["detail"]?.jsonPrimitive?.content?.length)
        assertEquals(listOf("s1", "s2"), h.engine.loads.map { it.id })
        assertEquals(0, h.engine.unloads, "the transcode's media replaces the failed media")
        assertTrue(h.beats("s1").none { it["state"]?.jsonPrimitive?.content == "paused" }, "the failed media's pause isn't reported")

        h.engine.current.onReady()
        h.engine.current.onPlaying()
        assertEquals(false, h.controller.state.value.adjusting)

        // A second failure falls back again, to the CPU.
        h.engine.fail()
        runCurrent()
        assertEquals(1, h.server.to("POST", "$START/s2/fallback").size)
        assertEquals(listOf("s1", "s2", "s3"), h.engine.loads.map { it.id })
        h.engine.current.onReady()

        // A third failure in the same play shows the error and stops the play.
        h.engine.fail()
        runCurrent()
        assertEquals(1, h.server.to("POST", "$START/s1/fallback").size)
        assertEquals(1, h.server.to("POST", "$START/s2/fallback").size)
        assertTrue(h.server.to("POST", "$START/s3/fallback").isEmpty())
        assertEquals(1, h.stops("s3").size)
        assertEquals(PlayerStatus.Error, h.controller.state.value.status)
        assertEquals(PlayerFailure.Problem("noFallback"), h.controller.state.value.failure)
        assertEquals(1, h.engine.unloads)
    }

    @Test
    fun theServersNoFallbackEndsTheLadder() = runTest {
        val h = Harness(this)
        h.answerStarts(play("s1", delivery = PlayDelivery.Stream))
        h.acceptBeats("s1")
        h.server.on("POST", "$START/s1/fallback") { problem(HttpStatusCode.Conflict, "noFallback") }

        h.controller.play(PlayRequest("w1"))
        runCurrent()
        h.engine.current.onReady()
        h.engine.fail()
        runCurrent()

        assertEquals(1, h.server.to("POST", "$START/s1/fallback").size)
        assertEquals(PlayerStatus.Error, h.controller.state.value.status)
        assertEquals(PlayerFailure.Problem("noFallback"), h.controller.state.value.failure)
    }

    @Test
    fun aBookNeverFallsBack() = runTest {
        val h = Harness(this, canFallBack = false)
        h.answerStarts(play("s1"))
        h.acceptBeats("s1")

        h.controller.play(PlayRequest("book"))
        runCurrent()
        h.engine.fail("unsupported", "no decoder")
        runCurrent()

        assertTrue(h.server.to("POST", "$START/s1/fallback").isEmpty())
        assertEquals(1, h.stops("s1").size)
        assertEquals(PlayerFailure.Problem("notPlayable"), h.controller.state.value.failure)
        assertEquals("no decoder", h.controller.state.value.detail)
    }

    @Test
    fun aForgottenPlayStartsAgainAtThePosition() = runTest {
        val h = Harness(this)
        h.answerStarts(play("s1"), play("s2", startMs = 30_000), play("s3"))
        h.server.on("PATCH", "$START/s1") { problem(HttpStatusCode.NotFound, "notFound") }
        h.acceptBeats("s2")
        h.server.on("POST", "$START/s2/fallback") { problem(HttpStatusCode.NotFound, "notFound") }
        h.acceptBeats("s3")

        h.controller.play(PlayRequest("w1"))
        runCurrent()
        h.engine.current.onReady()
        h.engine.positionMs = 30_000
        advanceTimeBy(HEARTBEAT_INTERVAL)
        runCurrent()

        // The heartbeat's 404: a new play at the position, with no stop for the forgotten one.
        val restart = body(h.server.to("POST", START)[1].body)
        assertEquals(30_000, restart["positionMs"]?.jsonPrimitive?.int)
        assertTrue(h.stops("s1").isEmpty())
        assertEquals("s2", h.controller.current?.id)

        // The fallback's 404 does the same.
        h.engine.positionMs = 31_000
        h.engine.fail()
        runCurrent()
        assertEquals(31_000, body(h.server.to("POST", START)[2].body)["positionMs"]?.jsonPrimitive?.int)
        assertEquals("s3", h.controller.current?.id)
    }

    @Test
    fun stopsOnExitAndStartsAgainOnReturn() = runTest {
        val h = Harness(this)
        h.answerStarts(play("s1"), play("s2", startMs = 90_000))
        h.acceptBeats("s1")
        h.acceptBeats("s2")

        h.controller.play(PlayRequest("w1"))
        runCurrent()
        h.engine.positionMs = 90_000
        h.controller.stopOnExit()
        runCurrent()
        assertEquals(90_000, h.stops("s1").single()["positionMs"]?.jsonPrimitive?.int)
        assertEquals(1, h.engine.unloads)
        assertNull(h.controller.current)

        h.controller.restore()
        runCurrent()
        assertEquals(90_000, body(h.server.to("POST", START)[1].body)["positionMs"]?.jsonPrimitive?.int)

        h.controller.dispose()
        runCurrent()
        assertEquals(1, h.stops("s2").size)
    }

    @Test
    fun anAdminStopEndsThePlayerWithAMessage() = runTest {
        val h = Harness(this)
        h.answerStarts(play("s1"))
        h.acceptBeats("s1")

        h.controller.play(PlayRequest("w1"))
        runCurrent()
        h.controller.playEnded("another")
        assertEquals(PlayerStatus.Loading, h.controller.state.value.status)

        h.controller.playEnded("s1", "Server maintenance")
        runCurrent()
        assertEquals(PlayerStatus.Error, h.controller.state.value.status)
        assertEquals(PlayerFailure.Ended("Server maintenance"), h.controller.state.value.failure)
        assertTrue(h.stops("s1").isEmpty())
        assertEquals(1, h.engine.unloads)

        // Nothing more is sent for it.
        advanceTimeBy(HEARTBEAT_INTERVAL * 2)
        runCurrent()
        assertTrue(h.beats("s1").isEmpty())
    }

    @Test
    fun aPlayShowsItsOwnTracksAndAsksWithItsQualitiesAsSettings() = runTest {
        val h = Harness(this)
        h.answerStarts(play("s1", tracks = fileTracks(subtitle = "t5")))
        h.acceptBeats("s1")
        h.controller.play(PlayRequest("w1", localQuality = QualityRequest("original")))
        runCurrent()

        val start = body(h.server.to("POST", START).single().body)
        assertEquals("original", start["settings"]?.jsonObject?.get("localQuality")?.jsonObject?.get("mode")?.jsonPrimitive?.content)
        assertNull(start["quality"])
        assertNull(start["overrides"])
        val state = h.controller.state.value
        assertEquals("a1", state.audioTrackId)
        assertEquals("t5", state.subtitleTrackId)
        val external = TrackShow.External("t5", "plays/s1/subtitles/5.sup", "hdmv_pgs_subtitle", emptyList())
        assertEquals(external, state.subtitle)
        assertEquals(listOf<TrackShow>(external), h.engine.subtitles)
        assertEquals(listOf(1), h.engine.audio, "the file's audio track the play selected")
    }

    @Test
    fun aChoiceWithADeliveryShowsAtOnceAndTheHeartbeatsReportIt() = runTest {
        val h = Harness(this)
        h.answerStarts(play("s1", tracks = fileTracks()), play("s2"))
        h.acceptBeats("s1")
        h.acceptBeats("s2")
        h.controller.play(PlayRequest("w1"))
        runCurrent()
        h.engine.current.onReady()

        // A subtitle inside the file: selected by its index, no new play.
        h.controller.chooseSubtitle("t4")
        runCurrent()
        assertEquals(TrackShow.File(3), h.engine.subtitles.last())
        assertEquals("t4", h.controller.state.value.subtitleTrackId)
        assertEquals("t4", h.beats("s1").last()["subtitleTrackId"]?.jsonPrimitive?.content)
        advanceTimeBy(HEARTBEAT_INTERVAL)
        runCurrent()
        assertEquals("t4", h.beats("s1").last()["subtitleTrackId"]?.jsonPrimitive?.content)

        // Another audio track in the file, the same way.
        h.controller.chooseAudio("a2")
        runCurrent()
        assertEquals(listOf(1, 2), h.engine.audio, "the play's own, then the one chosen")
        assertEquals("a2", h.beats("s1").last()["audioTrackId"]?.jsonPrimitive?.content)

        // Off shows none at once.
        h.controller.chooseSubtitle("off")
        runCurrent()
        assertEquals(TrackShow.Off, h.engine.subtitles.last())
        assertEquals("off", h.beats("s1").last()["subtitleTrackId"]?.jsonPrimitive?.content)
        assertEquals(1, h.server.to("POST", START).size)

        // Another audio track, asked for with no version named, plays on
        // the play's version, keeping the tracks shown.
        h.controller.switchTracks { it.copy(audioTrackId = "audio-fr") }
        runCurrent()
        val switch = body(h.server.to("POST", START).last().body)
        assertEquals("v1", switch["versionId"]?.jsonPrimitive?.content)
        assertEquals("audio-fr", switch["audioTrackId"]?.jsonPrimitive?.content)
        assertEquals("off", switch["subtitleTrackId"]?.jsonPrimitive?.content)

        // The new play shows it as its own, so its heartbeats don't report it.
        h.engine.current.onReady()
        advanceTimeBy(HEARTBEAT_INTERVAL)
        runCurrent()
        assertNull(h.beats("s2").last()["subtitleTrackId"]?.jsonPrimitive?.contentOrNull)
        assertNull(h.beats("s2").last()["audioTrackId"]?.jsonPrimitive?.contentOrNull)
    }

    @Test
    fun aChoiceWithoutADeliveryStartsANewPlayAtThePosition() = runTest {
        val h = Harness(this)
        h.answerStarts(play("s1", tracks = fileTracks(subtitle = "t4")), play("s2"), play("s3"))
        h.acceptBeats("s1")
        h.acceptBeats("s2")
        h.controller.play(PlayRequest("w1"))
        runCurrent()
        h.engine.current.onReady()
        h.engine.positionMs = 120_000

        h.controller.chooseSubtitle("t6")
        runCurrent()
        val subtitle = body(h.server.to("POST", START).last().body)
        assertEquals("t6", subtitle["subtitleTrackId"]?.jsonPrimitive?.content)
        assertEquals("a1", subtitle["audioTrackId"]?.jsonPrimitive?.content, "with the audio playing")
        assertEquals(120_000, subtitle["positionMs"]?.jsonPrimitive?.int)
        assertTrue(h.beats("s1").none { it["subtitleTrackId"] != null }, "a new play isn't reported as a switch")
    }

    @Test
    fun anAudioTrackOutsideTheFileStartsANewPlayKeepingTheSubtitle() = runTest {
        val h = Harness(this)
        h.answerStarts(play("s1", tracks = fileTracks(subtitle = "t4")), play("s2"))
        h.acceptBeats("s1")
        h.controller.play(PlayRequest("w1"))
        runCurrent()
        h.engine.current.onReady()

        h.controller.chooseAudio("a3")
        runCurrent()
        val audio = body(h.server.to("POST", START).last().body)
        assertEquals("a3", audio["audioTrackId"]?.jsonPrimitive?.content)
        assertEquals("t4", audio["subtitleTrackId"]?.jsonPrimitive?.content)
        assertEquals(listOf(1), h.engine.audio, "only the play's own")
    }

    @Test
    fun whileASubtitleIsBurnedInEveryOtherChoiceStartsANewPlay() = runTest {
        val h = Harness(this)
        val burned = listOf(
            PlayTrack("t4", TrackType.Subtitle, selected = true, delivery = TrackDelivery.Burn, reason = TrackReason.BurnSubtitlesEnabled),
            PlayTrack("t5", TrackType.Subtitle, reason = TrackReason.BurnSubtitlesEnabled),
            PlayTrack("off", TrackType.Subtitle),
        )
        h.answerStarts(play("s1", delivery = PlayDelivery.Stream, tracks = burned), play("s2"))
        h.acceptBeats("s1")
        h.controller.play(PlayRequest("w1"))
        runCurrent()
        h.engine.current.onReady()
        assertEquals(TrackShow.Burned, h.controller.state.value.subtitle)

        h.controller.chooseSubtitle("off")
        runCurrent()
        assertEquals("off", body(h.server.to("POST", START).last().body)["subtitleTrackId"]?.jsonPrimitive?.content)
    }

    @Test
    fun aStreamedSubtitleIsTheRenditionAtItsPlaceAmongTheStreamedOnes() = runTest {
        val h = Harness(this)
        val streamed = listOf(
            PlayTrack("v1", TrackType.Video, selected = true, delivery = TrackDelivery.Stream),
            PlayTrack("t4", TrackType.Subtitle, delivery = TrackDelivery.Stream, codec = "webvtt"),
            PlayTrack("t5", TrackType.Subtitle, delivery = TrackDelivery.External, codec = "ass", url = "plays/s1/subtitles/5.ass", fonts = listOf("fonts/f1")),
            PlayTrack("t6", TrackType.Subtitle, selected = true, delivery = TrackDelivery.Stream, codec = "webvtt"),
            PlayTrack("off", TrackType.Subtitle, delivery = TrackDelivery.None),
        )
        h.answerStarts(play("s1", delivery = PlayDelivery.Stream, tracks = streamed))
        h.acceptBeats("s1")
        h.controller.play(PlayRequest("w1"))
        runCurrent()

        assertEquals(TrackShow.Stream("t6", 1), h.engine.subtitles.single())
        h.controller.chooseSubtitle("t5")
        assertEquals(TrackShow.External("t5", "plays/s1/subtitles/5.ass", "ass", listOf("fonts/f1")), h.engine.subtitles.last())
    }

    @Test
    fun aSubtitleTheAppDrawsPlaysAgainWithoutFilesInPictureInPictureAndAgainWithThemOnLeaving() = runTest {
        val h = Harness(this)
        val inPip = listOf(
            PlayTrack("t5", TrackType.Subtitle, selected = true, delivery = TrackDelivery.Burn, reason = TrackReason.SubtitleCodecUnsupported),
            PlayTrack("off", TrackType.Subtitle),
        )
        h.answerStarts(play("s1", tracks = fileTracks(subtitle = "t5")), play("s2", startMs = 90_000, tracks = inPip), play("s3"))
        h.acceptBeats("s1")
        h.acceptBeats("s2")
        h.controller.play(PlayRequest("w1"))
        runCurrent()
        h.engine.current.onReady()
        val first = body(h.server.to("POST", START).last().body)
        assertEquals("""["stream","external"]""", subtitleDelivery(first))

        // Entering: the PGS the app draws plays again from the position, asked for without files.
        h.engine.positionMs = 90_000
        h.controller.setOverlayVisible(false)
        runCurrent()
        val pip = body(h.server.to("POST", START).last().body)
        assertEquals("""["stream"]""", subtitleDelivery(pip))
        assertEquals("t5", pip["subtitleTrackId"]?.jsonPrimitive?.content)
        assertEquals(90_000, pip["positionMs"]?.jsonPrimitive?.int)

        // Leaving: the play started there burned it in, so it plays again with files.
        h.engine.current.onReady()
        h.controller.setOverlayVisible(true)
        runCurrent()
        assertEquals(3, h.server.to("POST", START).size)
        val back = body(h.server.to("POST", START).last().body)
        assertEquals("""["stream","external"]""", subtitleDelivery(back))
    }

    @Test
    fun aSubtitleInTheStreamNeedsNoNewPlayInPictureInPictureNorOnLeaving() = runTest {
        val h = Harness(this)
        h.answerStarts(play("s1", tracks = listOf(PlayTrack("t4", TrackType.Subtitle, selected = true, delivery = TrackDelivery.Stream))))
        h.acceptBeats("s1")
        h.controller.play(PlayRequest("w1"))
        runCurrent()
        h.engine.current.onReady()

        h.controller.setOverlayVisible(false)
        h.controller.setOverlayVisible(true)
        runCurrent()
        assertEquals(1, h.server.to("POST", START).size)
    }

    @Test
    fun aBurnedSubtitleThePlayDidntBurnForPictureInPictureStaysOnLeaving() = runTest {
        val h = Harness(this)
        h.answerStarts(play("s1", tracks = listOf(PlayTrack("t4", TrackType.Subtitle, selected = true, delivery = TrackDelivery.Burn))))
        h.acceptBeats("s1")
        h.controller.play(PlayRequest("w1"))
        runCurrent()
        h.engine.current.onReady()

        h.controller.setOverlayVisible(false)
        h.controller.setOverlayVisible(true)
        runCurrent()
        assertEquals(1, h.server.to("POST", START).size)
    }

    @Test
    fun theDialogueBoostSwitchesWithANewPlayAtThePosition() = runTest {
        val h = Harness(this)
        h.answerStarts(play("s1"), play("s2", startMs = 60_000))
        h.acceptBeats("s1")
        h.controller.play(PlayRequest("w1", dialogueBoost = false))
        runCurrent()
        assertEquals("false", body(h.server.to("POST", START).last().body)["dialogueBoost"]?.jsonPrimitive?.content)
        h.engine.current.onReady()
        h.engine.positionMs = 60_000

        h.controller.boostDialogue(true)
        // The play it replaces shows until the server answers.
        assertEquals(PlayerStatus.Loading, h.controller.state.value.status)
        assertEquals("s1", h.controller.state.value.play?.id)
        runCurrent()
        assertEquals("s2", h.controller.state.value.play?.id)
        // The server says the replaced play ended, as the switch stopped it: the new one plays on.
        h.controller.playEnded("s1")
        assertNull(h.controller.state.value.failure)
        val switch = body(h.server.to("POST", START).last().body)
        assertEquals("true", switch["dialogueBoost"]?.jsonPrimitive?.content)
        assertEquals("v1", switch["versionId"]?.jsonPrimitive?.content)
        assertEquals("60000", switch["positionMs"]?.jsonPrimitive?.content)
        assertEquals(60_000, h.stops("s1").single()["positionMs"]?.jsonPrimitive?.int)
    }

    @Test
    fun aRefusalShowsItsProblem() = runTest {
        val h = Harness(this)
        h.server.on("POST", START) {
            problem(
                HttpStatusCode.TooManyRequests,
                "streamLimit",
                extra = ""","plays":[{"id":"x","workId":"w2","title":"Other film","startedAt":"2026-10-12T10:00:00Z","clientName":"Bedroom"}]""",
            )
        }

        h.controller.play(PlayRequest("w1"))
        runCurrent()
        val failure = h.controller.state.value.failure as PlayerFailure.Problem
        assertEquals("streamLimit", failure.code)
        assertEquals("Bedroom", failure.plays?.single()?.clientName)
        assertTrue(h.engine.loads.isEmpty())
    }

    @Test
    fun aPlayAfterAnotherKeepsItsMediaPausedUntilItsOwnLoads() = runTest {
        val h = Harness(this)
        h.answerStarts(play("s1"), play("s2"))
        h.acceptBeats("s1")
        h.acceptBeats("s2")
        h.controller.play(PlayRequest("w1"))
        runCurrent()
        h.engine.current.onReady()

        h.controller.play(PlayRequest("w2"))
        assertEquals(0, h.engine.unloads)
        assertEquals(true, h.engine.paused)
        runCurrent()
        assertTrue(h.beats("s1").isEmpty(), "the ended play's pause isn't reported")
        assertEquals(listOf("s1", "s2"), h.engine.loads.map { it.id })
        assertEquals(0, h.engine.unloads, "the new media replaces the old")
        assertEquals(1, h.stops("s1").size)

        h.controller.dispose()
        assertEquals(1, h.engine.unloads)
    }

    @Test
    fun aHandedOverEngineKeepsTheMediaForTheNextControllerToReplace() = runTest {
        val h = Harness(this)
        h.answerStarts(play("s1"), play("s2"))
        h.acceptBeats("s1")
        h.acceptBeats("s2")
        h.controller.play(PlayRequest("w1"))
        runCurrent()
        h.engine.current.onReady()

        h.controller.handOver()
        runCurrent()
        assertEquals(1, h.stops("s1").size)
        assertEquals(0, h.engine.unloads)
        assertEquals(true, h.engine.paused)

        val next = PlayController(h.server.plays(), h.engine, { capabilities }, backgroundScope)
        next.play(PlayRequest("w2"))
        runCurrent()
        assertEquals(listOf("s1", "s2"), h.engine.loads.map { it.id })
        assertEquals(0, h.engine.unloads)

        // A start that fails releases the media handed over.
        h.server.on("POST", START) { problem(HttpStatusCode.NotFound, "notFound") }
        next.handOver()
        val failing = PlayController(h.server.plays(), h.engine, { capabilities }, backgroundScope)
        failing.play(PlayRequest("w3"))
        runCurrent()
        assertEquals(PlayerStatus.Error, failing.state.value.status)
        assertEquals(1, h.engine.unloads)
    }

    @Test
    fun mediaThatGetsNoAnswerLoadsAgainWhereItWasAtTheAddressPickedAgain() = runTest {
        val server = FakeServer(StandardTestDispatcher(testScheduler))
        var away = false
        fun answer(host: String) {
            if (away && host == "home.test") throw IOException("no answer")
        }
        server.on("GET", "/api/v1/server") { answer(it.url.host); json(ServerInfo.serializer(), ServerInfo("server-1", "Basement", "1.0")) }
        server.on("POST", START) { json(PlayStarted.serializer(), play("s1"), HttpStatusCode.Created) }
        server.on("PATCH", "$START/s1") { answer(it.url.host); empty() }
        server.on("POST", "$START/s1/stop") { empty() }
        val connection = server.connection(
            at = "https://home.test",
            addresses = listOf("https://home.test", "https://kivo.test"),
            pickTime = testScheduler.timeSource,
        )
        val engine = FakeEngine()
        val controller = PlayController(server.plays(connection), engine, { capabilities }, backgroundScope)
        controller.play(PlayRequest("w1"))
        runCurrent()
        engine.current.onReady()

        // The device left home: the media gets no answer, and the play waits for the server.
        away = true
        engine.positionMs = 42_000
        engine.fail("networkError", "The network connection was lost.")
        assertEquals(PlayerStatus.Buffering, controller.state.value.status)
        // Asked again after each wait, the address is picked again once the last pick, as the session opened, has rested.
        advanceTimeBy(PICK_REST + 8.seconds)
        runCurrent()
        assertEquals("https://kivo.test", connection.address.current.value)
        assertEquals(listOf("s1", "s1"), engine.loads.map { it.id }, "the same play, no new one")
        assertEquals(42_000, engine.loads.last().startMs)
        assertEquals(0, engine.unloads, "the media loaded again replaces the media")
        assertEquals(1, server.to("POST", START).size)
        assertTrue(server.to("POST", "$START/s1/fallback").isEmpty())
        engine.current.onReady()
        assertEquals(false, engine.paused, "it plays on")
        assertEquals(PlayerStatus.Playing, controller.state.value.status)

        // A paused play stays paused.
        engine.pause()
        engine.positionMs = 50_000
        engine.fail("networkError", "The request timed out.")
        runCurrent()
        assertEquals(50_000, engine.loads.last().startMs)
        engine.current.onReady()
        assertEquals(true, engine.paused)
        assertEquals(true, controller.state.value.paused)
        assertEquals(0, engine.unloads)
        assertTrue(server.to("POST", "$START/s1/stop").isEmpty(), "the play goes on")
    }

    @Test
    fun mediaThatGetsNoAnswerAgainBeforeItsReadyFallsBack() = runTest {
        val h = Harness(this)
        h.answerStarts(play("s1"))
        h.acceptBeats("s1")
        h.acceptBeats("s2")
        h.server.on("POST", "$START/s1/fallback") { json(PlayStarted.serializer(), play("s2", startMs = 42_000), HttpStatusCode.Created) }
        h.controller.play(PlayRequest("w1"))
        runCurrent()
        h.engine.current.onReady()
        h.engine.positionMs = 42_000

        h.engine.fail("networkError", "no answer")
        runCurrent()
        assertEquals(listOf("s1", "s1"), h.engine.loads.map { it.id })
        h.engine.fail("networkError", "no answer")
        runCurrent()
        assertEquals(1, h.server.to("POST", "$START/s1/fallback").size)
        assertEquals(listOf("s1", "s1", "s2"), h.engine.loads.map { it.id })
        assertEquals(0, h.engine.unloads)
    }

    @Test
    fun aPlayForgottenWhileItsMediaGotNoAnswerStartsAgainAtThePosition() = runTest {
        val h = Harness(this)
        h.answerStarts(play("s1"), play("s2", startMs = 42_000))
        h.server.on("PATCH", "$START/s1") { problem(HttpStatusCode.NotFound, "notFound") }
        h.acceptBeats("s2")
        h.controller.play(PlayRequest("w1"))
        runCurrent()
        h.engine.current.onReady()
        h.engine.positionMs = 42_000

        h.engine.fail("networkError", "no answer")
        runCurrent()
        assertEquals(42_000, body(h.server.to("POST", START)[1].body)["positionMs"]?.jsonPrimitive?.int)
        assertEquals(listOf("s1", "s2"), h.engine.loads.map { it.id })
        assertEquals(0, h.engine.unloads)
        assertTrue(h.server.to("POST", "$START/s1/fallback").isEmpty())
    }
}
