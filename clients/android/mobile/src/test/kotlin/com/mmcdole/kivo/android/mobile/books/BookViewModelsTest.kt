package com.mmcdole.kivo.android.mobile.books

import com.mmcdole.kivo.android.mobile.CoreTest
import com.mmcdole.kivo.android.mobile.awaitValue
import com.mmcdole.kivo.android.mobile.reply
import com.mmcdole.kivo.api.models.BookInfo
import com.mmcdole.kivo.api.models.Chapter
import com.mmcdole.kivo.api.models.MatchState
import com.mmcdole.kivo.api.models.PersonRef
import com.mmcdole.kivo.api.models.PlayDelivery
import com.mmcdole.kivo.api.models.PlayPart
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.UserState
import com.mmcdole.kivo.api.models.Version
import com.mmcdole.kivo.api.models.VersionPart
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.core.books.BookAudio
import com.mmcdole.kivo.core.books.SleepChoice
import com.mmcdole.kivo.core.books.SleepState
import com.mmcdole.kivo.core.play.PlayerStatus
import com.mmcdole.kivo.core.player.PlayerEngine
import com.mmcdole.kivo.core.player.TrackShow
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpStatusCode
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertIs
import kotlin.test.assertTrue

class BookViewModelsTest : CoreTest() {
    private val chapters = listOf(Chapter(0, 60_000, "Opening"), Chapter(60_000, 180_000, "Middle"), Chapter(180_000, 300_000))

    init {
        server.get("/api/v1/works/b1", WorkDetail.serializer()) {
            WorkDetail(
                id = "b1", kind = WorkKind.Book, title = "The Martian", matchState = MatchState.Matched, available = true, images = WorkImages(),
                genres = emptyList(), studios = emptyList(), credits = emptyList(), externalIds = emptyList(),
                versions = listOf(
                    Version(
                        id = "v1", available = true, durationMs = 300_000, sizeBytes = 1, parts = listOf(VersionPart("f1", "book.m4b", 300_000, 1)),
                        tracks = emptyList(), workIds = listOf("b1"), name = "M4B", chapters = chapters,
                    ),
                ),
                extras = emptyList(), collections = emptyList(),
                book = BookInfo(authors = listOf(PersonRef("p1", "Andy Weir")), narrators = listOf(PersonRef("p2", "Ray Porter")), series = emptyList()),
                userState = UserState(played = false, playCount = 0, positionMs = 90_000),
            )
        }
        server.on("POST", "/api/v1/plays") {
            reply(
                PlayStarted.serializer(),
                PlayStarted(
                    id = "p1", workId = "b1", versionId = "v1", startMs = 90_000, durationMs = 300_000, local = true,
                    delivery = PlayDelivery.File, url = "plays/p1/files/0", tracks = emptyList(), parts = listOf(PlayPart(300_000)), chapters = chapters,
                ),
                HttpStatusCode.Created,
            )
        }
        server.on("POST", "/api/v1/plays/p1/stop") { respond("", HttpStatusCode.NoContent) }
        server.on("PATCH", "/api/v1/plays/p1") { respond("", HttpStatusCode.NoContent) }
    }

    @Test
    fun theMiniPlayerShowsTheBookFromWhereItWasLeft() {
        val engine = AudioEngine()
        val books = signIn().playback.bookPlayer(engine, engine)
        val mini = held { MiniPlayerViewModel(books) }
        assertFalse(mini.state.value.shown)
        mini.play("b1", fromStart = false)
        val state = awaitValue(mini.state) { it.playing }
        assertEquals("The Martian", state.title)
        assertEquals(1, state.chapterIndex)
        assertEquals(0.3f, state.fraction)
        assertEquals(210_000L / 60_000 * 60_000, state.remainingMs)
        mini.togglePlay()
        assertFalse(awaitValue(mini.state) { !it.playing }.playing)
        assertTrue(engine.paused)
    }

    @Test
    fun nowPlayingMovesByChapterSetsTheSleepTimerAndStops() {
        val engine = AudioEngine()
        val books = signIn().playback.bookPlayer(engine, engine)
        val model = held { NowPlayingViewModel(books) }
        books.play("b1")
        val playing = awaitValue(model.state) { it.playing && it.progress != null }
        assertEquals("Middle", playing.progress?.chapterTitle)
        assertEquals(3, playing.chapters.size)
        model.nextChapter()
        assertEquals(180_000L, awaitValue(model.state) { it.progress?.chapterIndex == 2 }.progress?.chapterStartMs)
        model.setSleep(SleepChoice.Minutes(15))
        assertIs<SleepState.Running>(awaitValue(model.state) { it.sleep is SleepState.Running }.sleep)
        model.stop()
        assertFalse(awaitValue(model.state) { !it.shown }.shown)
    }
}

/** A book's engine that's ready as soon as it loads and plays at once. */
private class AudioEngine : PlayerEngine, BookAudio {
    override var positionMs = 0L
    override var paused = true
    override val bandwidthBps: Long? = null
    private var events: PlayerEngine.Events? = null

    override fun load(play: PlayStarted, events: PlayerEngine.Events) {
        this.events = events
        positionMs = play.startMs.toLong()
        events.onReady()
    }

    override fun unload() {
        events = null
        paused = true
    }

    override fun play() {
        paused = false
        events?.onPlay()
        events?.onPlaying()
    }

    override fun pause() {
        paused = true
        events?.onPause()
    }

    override fun seek(positionMs: Long) {
        this.positionMs = positionMs
        events?.onSeeked()
        events?.onTime()
    }

    override fun showSubtitle(show: TrackShow) = Unit

    override fun selectAudio(index: Int) = Unit

    override fun setSpeed(speed: Double) = Unit

    override fun fadeOut(durationMs: Long) = Unit

    override fun restoreVolume() = Unit
}
