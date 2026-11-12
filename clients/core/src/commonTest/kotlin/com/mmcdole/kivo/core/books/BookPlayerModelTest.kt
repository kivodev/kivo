package com.mmcdole.kivo.core.books

import com.mmcdole.kivo.api.models.Capabilities
import com.mmcdole.kivo.api.models.FileCapabilities
import com.mmcdole.kivo.api.models.Chapter
import com.mmcdole.kivo.api.models.CurrentUser
import com.mmcdole.kivo.api.models.MatchState
import com.mmcdole.kivo.api.models.PlayDelivery
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.PlayPart
import com.mmcdole.kivo.api.models.PlaybackPreferences
import com.mmcdole.kivo.api.models.SubtitleMode
import com.mmcdole.kivo.api.models.UserLimits
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.core.FakeServer
import com.mmcdole.kivo.core.empty
import com.mmcdole.kivo.core.json
import com.mmcdole.kivo.core.play.FakeEngine
import com.mmcdole.kivo.core.play.PlayerStatus
import com.mmcdole.kivo.core.play.ServerPlays
import com.mmcdole.kivo.core.settings.UserSettings
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.double
import kotlinx.serialization.json.int
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertIs
import kotlin.test.assertNull
import kotlin.test.assertTrue

private const val START = "/api/v1/plays"

// A book of two 10-minute files, its second chapter running across them.
private val chapters = listOf(Chapter(0, 300_000, "One"), Chapter(300_000, 900_000, "Two"), Chapter(900_000, 1_200_000, "Three"))

private fun play(id: String, startMs: Int) = PlayStarted(
    id = id,
    workId = "b1",
    versionId = "v1",
    startMs = startMs,
    durationMs = 1_200_000,
    local = true,
    delivery = PlayDelivery.File,
    url = "plays/$id/media",
    tracks = emptyList(),
    parts = listOf(PlayPart(600_000, "plays/$id/parts/0"), PlayPart(600_000, "plays/$id/parts/1")),
    chapters = chapters,
)

private val book = WorkDetail(
    id = "b1", kind = WorkKind.Book, title = "Project Hail Mary", matchState = MatchState.Matched, available = true,
    images = WorkImages(), genres = emptyList(), studios = emptyList(), credits = emptyList(), externalIds = emptyList(),
    versions = emptyList(), extras = emptyList(), collections = emptyList(), similar = emptyList(),
)

private val capabilities = Capabilities(
    video = emptyList(),
    audio = emptyList(),
    subtitles = emptyList(),
    file = FileCapabilities(containers = listOf("mp4")),
)

private class FakeAudio : BookAudio {
    val speeds = mutableListOf<Double>()
    val fades = mutableListOf<Long>()
    var restores = 0

    override fun setSpeed(speed: Double) {
        speeds += speed
    }

    override fun fadeOut(durationMs: Long) {
        fades += durationMs
    }

    override fun restoreVolume() {
        restores++
    }
}

private class Harness(val scope: TestScope, startMs: Int = 0) {
    val server = FakeServer(StandardTestDispatcher(scope.testScheduler))
    val engine = FakeEngine()
    val audio = FakeAudio()
    val connection = server.connection()
    val model = BookPlayerModel(
        engine = engine,
        audio = audio,
        open = { BookSource(book, ServerPlays(connection.plays, connection.reachability), downloaded = false) },
        capabilities = { capabilities },
        userSettings = UserSettings(connection),
        scope = scope.backgroundScope,
        now = { scope.testScheduler.currentTime },
    )

    init {
        server.on("GET", "/api/v1/users/me") { json(CurrentUser.serializer(), CurrentUser("u1", "anna", false, UserLimits(), PlaybackPreferences(SubtitleMode.Smart, true, true, 1.25))) }
        server.on("PATCH", "/api/v1/users/me") { json(CurrentUser.serializer(), CurrentUser("u1", "anna", false, UserLimits(), PlaybackPreferences(SubtitleMode.Smart, true, true, 1.5))) }
        server.on("POST", START) { json(PlayStarted.serializer(), play("s1", startMs), HttpStatusCode.Created) }
        server.on("PATCH", "$START/s1") { empty() }
        server.on("POST", "$START/s1/stop") { empty() }
    }

    /** Plays the book and lets the engine start it. */
    fun start(positionMs: Long? = null) {
        model.play("b1", positionMs)
        scope.runCurrent()
        engine.current.onReady()
        scope.runCurrent()
    }

    /** The engine played to [positionMs]. */
    fun at(positionMs: Long) {
        engine.positionMs = positionMs
        engine.current.onTime()
    }
}

private fun body(text: String) = Json.parseToJsonElement(text).jsonObject

class BookPlayerModelTest {
    @Test
    fun streamsThroughThePlayAtTheUsersSpeed() = runTest {
        val h = Harness(this, startMs = 420_000)
        h.start()

        val state = h.model.state.value
        assertEquals("b1", state.workId)
        assertEquals(book, state.work)
        assertEquals(PlayerStatus.Playing, state.player.status)
        assertEquals(false, state.player.paused)
        assertEquals(1.25, state.speed, "the user's speed")
        assertEquals(1.25, h.audio.speeds.last())
        val progress = assertIs<BookProgress>(state.progress)
        assertEquals(1, progress.chapterIndex, "the play's chapters")
        assertEquals("Two", progress.chapterTitle)
        assertEquals(120_000, progress.chapterElapsedMs)
        assertEquals(780_000, progress.remainingMs)
        assertEquals(1, h.server.to("POST", START).size)
    }

    @Test
    fun chaptersMoveByThePreviousRuleAndNextAcrossParts() = runTest {
        val h = Harness(this)
        h.start()

        h.at(450_000)
        h.model.nextChapter()
        assertEquals(900_000, h.engine.seeks.last(), "into the second file")
        h.model.nextChapter()
        assertEquals(1, h.engine.seeks.size, "none after the last")

        h.at(902_000)
        h.model.previousChapter()
        assertEquals(300_000, h.engine.seeks.last(), "within 3 s of a start: the chapter before")
        h.at(320_000)
        h.model.previousChapter()
        assertEquals(300_000, h.engine.seeks.last(), "later: its own start")

        h.at(320_000)
        h.model.seekInChapter(1, 500_000)
        assertEquals(800_000, h.engine.seeks.last(), "the seek bar spans the chapter")
        h.model.seekInChapter(1, 10_000_000)
        assertEquals(900_000, h.engine.seeks.last(), "and no further")

        h.model.skipBack()
        assertEquals(890_000, h.engine.seeks.last())
        h.model.playChapter(0)
        assertEquals(0, h.engine.seeks.last())
    }

    @Test
    fun skipsMoveTenBackAndThirtyForwardAcrossChaptersAndNowPlayingFollows() = runTest {
        val h = Harness(this)
        h.start()

        h.at(290_000)
        h.model.skipForward()
        assertEquals(320_000, h.engine.seeks.last(), "30 s on, into the next chapter")
        runCurrent()
        var progress = assertIs<BookProgress>(h.model.state.value.progress)
        assertEquals(1, progress.chapterIndex, "the chapter at once, before the engine plays there")
        assertEquals(20_000, progress.chapterElapsedMs)
        assertEquals(600_000, progress.chapterDurationMs)

        h.engine.current.onSeeked()
        h.at(305_000)
        h.model.skipBack()
        assertEquals(295_000, h.engine.seeks.last(), "10 s back, into the chapter before")
        runCurrent()
        progress = assertIs<BookProgress>(h.model.state.value.progress)
        assertEquals(0, progress.chapterIndex)
        assertEquals(295_000, progress.chapterElapsedMs)

        h.at(5_000)
        h.model.skipBack()
        assertEquals(0, h.engine.seeks.last(), "no further back than the start")
        h.at(1_190_000)
        h.model.skipForward()
        assertEquals(1_200_000, h.engine.seeks.last(), "nor on than the end")
    }

    @Test
    fun aChoiceOfSpeedIsKeptForTheUser() = runTest {
        val h = Harness(this)
        h.start()
        h.model.setSpeed(1.5)
        runCurrent()

        assertEquals(1.5, h.model.state.value.speed)
        assertEquals(1.5, h.audio.speeds.last())
        val patch = body(h.server.to("PATCH", "/api/v1/users/me").single().body)
        assertEquals(1.5, patch["playback"]?.jsonObject?.get("bookSpeed")?.jsonPrimitive?.double)
    }

    @Test
    fun theSleepTimerFadesOverTheLastTenSecondsThenPauses() = runTest {
        val h = Harness(this)
        h.start()
        h.model.setSleep(SleepChoice.Minutes(5))
        assertEquals(SleepState.Running(SleepChoice.Minutes(5), 300_000, fading = false), h.model.state.value.sleep)

        advanceTimeBy(290_001)
        assertEquals(listOf(10_000L), h.audio.fades)
        assertEquals(SleepState.Running(SleepChoice.Minutes(5), 10_000, fading = true), h.model.state.value.sleep)
        assertEquals(false, h.engine.paused)

        advanceTimeBy(10_000)
        assertTrue(h.engine.paused, "paused at the end")
        assertEquals(1, h.audio.restores, "with the volume back for next time")
        assertEquals(SleepState.Off, h.model.state.value.sleep)
        assertEquals(SleepChoice.Minutes(5), h.model.lastSleep)
    }

    @Test
    fun aPausedBookDoesntSpendTheTimerAndOffUndoesTheFade() = runTest {
        val h = Harness(this)
        h.start()
        h.model.setSleep(SleepChoice.Minutes(10))
        h.model.pause()
        advanceTimeBy(600_000)
        assertEquals(SleepState.Running(SleepChoice.Minutes(10), 600_000, fading = false), h.model.state.value.sleep)

        h.model.resume()
        advanceTimeBy(595_001)
        assertEquals(listOf(10_000L), h.audio.fades)
        h.model.stopSleep()
        assertEquals(1, h.audio.restores)
        assertEquals(SleepState.Off, h.model.state.value.sleep)
        advanceTimeBy(60_000)
        assertEquals(false, h.engine.paused, "cancelled: it plays on")
    }

    @Test
    fun endOfChapterPausesAtTheChaptersEndAndFollowsAChapterMove() = runTest {
        val h = Harness(this)
        h.start()
        h.model.setSpeed(1.0)
        h.at(100_000)
        h.model.setSleep(SleepChoice.EndOfChapter)
        assertEquals(200_000, (h.model.state.value.sleep as SleepState.Running).remainingMs)

        // Moved to the second chapter: it ends at 15:00 now.
        h.model.nextChapter()
        h.engine.current.onSeeked()
        h.at(892_000)
        advanceTimeBy(251)
        assertEquals(listOf(8_000L), h.audio.fades)

        h.at(900_200)
        advanceTimeBy(250)
        assertTrue(h.engine.paused)
        assertEquals(900_000, h.engine.seeks.last(), "kept at the chapter's end, the next one's start")
        assertEquals(SleepState.Off, h.model.state.value.sleep)
    }

    @Test
    fun stopSavesThePlaceAndUnloadsTheBook() = runTest {
        val h = Harness(this)
        h.start()
        h.at(450_000)
        h.model.stop()
        runCurrent()

        assertEquals(450_000, body(h.server.to("POST", "$START/s1/stop").single().body)["positionMs"]?.jsonPrimitive?.int)
        assertNull(h.model.state.value.workId, "the mini player goes")
        assertEquals(1, h.engine.unloads)
    }

    @Test
    fun theSameBookSeeksRatherThanStartingAgain() = runTest {
        val h = Harness(this)
        h.start()
        h.model.pause()
        h.model.play("b1", positionMs = 900_000)
        runCurrent()

        assertEquals(1, h.server.to("POST", START).size)
        assertEquals(900_000, h.engine.seeks.last())
        assertEquals(false, h.engine.paused)
    }
}
