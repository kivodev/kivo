package com.mmcdole.kivo.android.player

import android.os.SystemClock
import android.view.KeyEvent
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsNodeInteraction
import androidx.compose.ui.test.assertIsFocused
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.ComposeContentTestRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewmodel.compose.LocalViewModelStoreOwner
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import androidx.media3.common.MediaMetadata
import androidx.test.platform.app.InstrumentationRegistry
import com.mmcdole.kivo.android.HasAppGraph
import com.mmcdole.kivo.android.tv.R
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.TestDevice
import com.mmcdole.kivo.android.detail.DetailScreen
import com.mmcdole.kivo.android.theme.KivoTheme
import com.mmcdole.kivo.android.ui.LocalSession
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.api.infrastructure.ApiClient
import com.mmcdole.kivo.api.models.Chapter
import com.mmcdole.kivo.api.models.Marker
import com.mmcdole.kivo.api.models.DialogueBoost
import com.mmcdole.kivo.api.models.MatchState
import com.mmcdole.kivo.api.models.PlayCreate
import com.mmcdole.kivo.api.models.EpisodeInfo
import com.mmcdole.kivo.api.models.PlayDelivery
import com.mmcdole.kivo.api.models.PlayPart
import com.mmcdole.kivo.api.models.PlayTrack
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.Resolution
import com.mmcdole.kivo.api.models.ServerInfo
import com.mmcdole.kivo.api.models.Track
import com.mmcdole.kivo.api.models.TrackDelivery
import com.mmcdole.kivo.api.models.TrackType
import com.mmcdole.kivo.api.models.Version
import com.mmcdole.kivo.api.models.VersionPart
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.core.auth.SavedServer
import com.mmcdole.kivo.core.play.PlayChoice
import com.mmcdole.kivo.core.play.PlayerEnd
import com.mmcdole.kivo.core.session.KivoClient
import com.mmcdole.kivo.core.player.PlayerEngine
import com.mmcdole.kivo.core.player.TrackShow
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.respond
import io.ktor.client.engine.mock.toByteArray
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withContext
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestCoroutineScheduler
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
import java.io.File
import java.util.UUID
import java.util.concurrent.CopyOnWriteArrayList

/** The episode the player plays, and the ones before and after it. */
const val PREVIOUS = "episode-1"
const val EPISODE = "episode-2"
const val NEXT = "episode-3"

/** The episode's length: 44 minutes, with an intro from 1:00 to 2:30 and credits over the last 90 seconds. */
const val DURATION_MS = 2_640_000

/** Where the intro marker ends. */
const val INTRO_END_MS = 150_000L

/** A position past the intro and well before Up next. */
const val MIDDLE_MS = 600_000

/** A position in the intro, where Skip Intro shows. */
const val IN_INTRO_MS = 70_000

/** The fake server's ID. */
const val SERVER_ID = "player-test"

/** A position past Up next's time. */
const val NEAR_END_MS = DURATION_MS - 30_000

/** A video the test drives: it records what the player asks and reports what the test says. */
class FakeVideoEngine : VideoEngine {
    override var metadata: MediaMetadata = MediaMetadata.EMPTY
    override var speed = 1f
    override var playPrevious: (() -> Unit)? = null
    override var playNext: (() -> Unit)? = null
    override var positionMs = 0L
    override var paused = true
    override val bandwidthBps: Long? = null

    /** What the player asked, in order: "play", "pause" and "seek <ms>". */
    val calls: MutableList<String> = CopyOnWriteArrayList()
    private var events: PlayerEngine.Events? = null
    val loaded: Boolean get() = events != null

    override fun load(play: PlayStarted, events: PlayerEngine.Events) {
        this.events = events
        positionMs = play.startMs.toLong()
        paused = true
    }

    override fun unload() {
        events = null
    }

    override fun play() {
        paused = false
        calls += "play"
        events?.onPlay()
    }

    override fun pause() {
        paused = true
        calls += "pause"
        events?.onPause()
    }

    override fun seek(positionMs: Long) {
        this.positionMs = positionMs
        calls += "seek $positionMs"
        events?.onSeeked()
    }

    override fun showSubtitle(show: TrackShow) = Unit

    override fun selectAudio(index: Int) = Unit

    /** The media loaded: the player starts playing it. */
    fun ready() = events?.onReady()

    /** The video played to its end. */
    fun end() = events?.onEnded()

    val seeks: List<Long> get() = calls.filter { it.startsWith("seek ") }.map { it.removePrefix("seek ").toLong() }
}

/**
 * The player screen of episode 2 of 3, on a server answered by Ktor's mock
 * engine and a [FakeVideoEngine], composed by [compose] with its clock
 * paused: the test moves time on with [advance] and presses keys through the
 * device's input, so they reach the window that has focus, a panel's
 * included. The core's player waits on [clock] too, which [compose] runs on
 * (`createComposeRule(clock)`), so the controls, Skip and up next keep the
 * screen's time.
 */
class PlayerHarness(private val compose: ComposeContentTestRule, private val clock: TestCoroutineScheduler) {
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    val context = instrumentation.targetContext
    private val graph = (context.applicationContext as HasAppGraph).graph
    val engine = FakeVideoEngine()
    val navigator = Navigator()

    /** The requests the server saw, as "METHOD /path". */
    val requests: MutableList<String> = CopyOnWriteArrayList()

    /** The bodies of the plays the player started, in order. */
    val plays: MutableList<PlayCreate> = CopyOnWriteArrayList()
    private val preferences = File(context.cacheDir, "player-test-${UUID.randomUUID()}.preferences_pb")

    /** Where the server starts the next play. */
    private var startMs = 0

    private val server = MockEngine { request ->
        val path = request.url.encodedPath
        requests += "${request.method.value} $path"
        when {
            path == "/api/v1/server" -> json(ServerInfo.serializer(), ServerInfo(SERVER_ID, "Player test", "1.0"))
            path == "/api/v1/plays" -> {
                plays += ApiClient.JSON_DEFAULT.decodeFromString(PlayCreate.serializer(), request.body.toByteArray().decodeToString())
                json(PlayStarted.serializer(), play(startMs), HttpStatusCode.Created)
            }
            path.startsWith("/api/v1/plays/") -> respond("", HttpStatusCode.NoContent)
            path == "/api/v1/works/$EPISODE" -> json(WorkDetail.serializer(), episode(EPISODE, 2, previousId = PREVIOUS, nextId = NEXT))
            path == "/api/v1/works/$NEXT" -> json(WorkDetail.serializer(), episode(NEXT, 3, previousId = EPISODE, nextId = null))
            else -> respond("""{"status":404,"code":"notFound"}""", HttpStatusCode.NotFound, headersOf(HttpHeaders.ContentType, "application/problem+json"))
        }
    }

    /** The core on a test TV, whose settings start as defaults and with no episodes played on, over the fake server. */
    val core = KivoClient(TestDevice(context, preferences), server, StandardTestDispatcher(clock))

    /** Opens the player at [startMs] and waits for its play to load; [live] has the media load too. */
    fun open(startMs: Int, live: Boolean = true) {
        val session = session(startMs)
        val args = Screen.Player(EPISODE)
        navigator.push(args)
        val entry = navigator.current
        ViewModelProvider.create(entry, viewModelFactory { initializer { PlayerViewModel(session, args) { engine } } })[PlayerViewModel::class]
        compose.mainClock.autoAdvance = false
        compose.setContent {
            KivoTheme {
                if (navigator.current === entry) {
                    CompositionLocalProvider(LocalSession provides session, LocalViewModelStoreOwner provides entry) {
                        PlayerScreen(session, args, navigator)
                    }
                }
            }
        }
        waitFor { engine.loaded && "GET /api/v1/works/$NEXT" in requests }
        if (live) {
            onMain { engine.ready() }
            advance(100)
        }
    }

    /** Opens the episode's detail page and waits for its Audio & Subtitles row. */
    fun openDetailPage() {
        val session = session(0)
        compose.setContent {
            KivoTheme {
                CompositionLocalProvider(LocalSession provides session) {
                    DetailScreen(session, EPISODE, navigator)
                }
            }
        }
        waitFor { shows(node("tracks")) }
    }

    /** Plays three episodes in a row to their ends without input, each in a player of its own, so the next asks "Still watching?". */
    fun playedOnTheirOwn() {
        val session = runBlocking { core.restore(SavedServer(SERVER_ID, "Player test", listOf("http://kivo.test"))) }
        repeat(3) {
            val engine = FakeVideoEngine()
            val video = session.playback.videoPlayer(engine)
            onMain { video.setHasNextEpisode(hasNext = true) }
            runBlocking { withContext(Dispatchers.Main) { video.play(PlayChoice(EPISODE)) } }
            waitFor { engine.loaded }
            onMain {
                engine.ready()
                engine.end()
            }
            check(video.ui.value.end == PlayerEnd.PlayNext)
            onMain { video.dispose() }
        }
    }

    fun close() {
        preferences.delete()
    }

    /** Whether the player is still the screen on top. */
    val showing: Boolean get() = navigator.current.screen == Screen.Player(EPISODE)

    /** Whether the controls show, which the scrub bar's being there says. */
    val controlsShown: Boolean get() = shows(node("scrub"))

    fun node(tag: String): SemanticsNodeInteraction = compose.onNodeWithTag(tag)

    /** The node showing [text], in any window, a panel's too. */
    fun text(text: String): SemanticsNodeInteraction = compose.onNode(hasText(text))

    /** A panel's row showing [text], not the player's button of the same name, which its name tags. */
    fun row(text: String): SemanticsNodeInteraction = compose.onNode(hasText(text) and !hasTestTag(text))

    /** Whether [node] has focus. */
    fun focused(node: SemanticsNodeInteraction): Boolean = runCatching { node.assertIsFocused() }.isSuccess

    /** Whether [node] is on screen. */
    fun shows(node: SemanticsNodeInteraction): Boolean = runCatching { node.assertExists() }.isSuccess

    /** The seconds Up next's countdown shows. */
    fun countdown(): Int {
        val words = context.getString(R.string.playing_in, 0).substringBefore("0")
        val node = compose.onNode(hasText(words, substring = true), useUnmergedTree = true).fetchSemanticsNode()
        return node.config[SemanticsProperties.Text].joinToString().filter(Char::isDigit).toInt()
    }

    /** Runs [block] on the main thread, where the engine reports. */
    fun onMain(block: () -> Unit) = instrumentation.runOnMainSync(block)

    /** Moves the screen's clock on by [ms], letting the main thread run what's due first. */
    fun advance(ms: Long) {
        instrumentation.waitForIdleSync()
        compose.mainClock.advanceTimeBy(ms)
        instrumentation.waitForIdleSync()
    }

    /** Moves the clock a frame at a time until [condition] holds, for up to five seconds. */
    fun waitFor(condition: () -> Boolean) {
        val until = SystemClock.uptimeMillis() + 5_000
        while (!condition()) {
            check(SystemClock.uptimeMillis() < until) { "timed out" }
            compose.mainClock.advanceTimeByFrame()
            SystemClock.sleep(16)
        }
    }

    /** A first DOWN and [repeats] repeats of [key], then its UP, and a moment for the screen to follow. */
    fun press(key: Int, repeats: Int = 0) {
        down(key)
        for (n in 1..repeats) down(key, n)
        up(key)
        advance(100)
    }

    fun down(key: Int, repeatCount: Int = 0) = send(KeyEvent.ACTION_DOWN, key, repeatCount)

    fun up(key: Int) = send(KeyEvent.ACTION_UP, key, 0)

    private fun send(action: Int, key: Int, repeatCount: Int) {
        val now = SystemClock.uptimeMillis()
        instrumentation.sendKeySync(KeyEvent(now, now, action, key, repeatCount))
    }

    private fun session(startMs: Int): ServerSession {
        this.startMs = startMs
        val session = runBlocking { core.restore(SavedServer(SERVER_ID, "Player test", listOf("http://kivo.test"))) }
        return ServerSession(graph, session, emptyMap())
    }

    private fun <T> io.ktor.client.engine.mock.MockRequestHandleScope.json(
        serializer: kotlinx.serialization.KSerializer<T>,
        value: T,
        status: HttpStatusCode = HttpStatusCode.OK,
    ) = respond(ApiClient.JSON_DEFAULT.encodeToString(serializer, value), status, headersOf(HttpHeaders.ContentType, "application/json"))

    private companion object {
        val chapters = listOf(
            Chapter(0, 60_000, "Cold open"),
            Chapter(60_000, INTRO_END_MS.toInt(), "Opening credits"),
            Chapter(INTRO_END_MS.toInt(), DURATION_MS - 90_000, "Episode"),
            Chapter(DURATION_MS - 90_000, DURATION_MS, "End credits"),
        )
        val markers = listOf(
            Marker("intro", 60_000, INTRO_END_MS.toInt(), "chapter"),
            Marker("credits", DURATION_MS - 90_000, DURATION_MS, "chapter"),
        )

        /** Two audio tracks, so Audio & Subtitles has a choice. */
        val tracks = listOf(
            Track("video", TrackType.Video, "h264", true, "H.264 1080p", "H.264 1080p · Default"),
            Track("english", TrackType.Audio, "aac", true, "English", "English · Default"),
            Track("french", TrackType.Audio, "aac", false, "French", "French"),
        )

        fun episode(id: String, number: Int, previousId: String?, nextId: String?) = WorkDetail(
            id = id,
            kind = WorkKind.Episode,
            title = "Episode $number",
            matchState = MatchState.Matched,
            available = true,
            images = WorkImages(),
            genres = emptyList(),
            studios = emptyList(),
            credits = emptyList(),
            externalIds = emptyList(),
            versions = listOf(
                Version(
                    id = "$id-version",
                    resolution = Resolution.P1080,
                    available = true,
                    durationMs = DURATION_MS,
                    sizeBytes = 1_000_000_000,
                    parts = listOf(VersionPart("$id-file", "$id.mkv", DURATION_MS, 1_000_000_000)),
                    tracks = tracks,
                    workIds = listOf(id),
                    name = "1080p",
                ),
            ),
            extras = emptyList(),
            collections = emptyList(),
            durationMs = DURATION_MS,
            episode = EpisodeInfo(seasonNumber = 1, number = number, seriesId = "series", seriesTitle = "The Series", previousId = previousId, nextId = nextId),
        )

        fun play(startMs: Int) = PlayStarted(
            id = "play",
            workId = EPISODE,
            versionId = "$EPISODE-version",
            startMs = startMs,
            durationMs = DURATION_MS,
            local = true,
            delivery = PlayDelivery.File,
            url = "plays/play/files/0",
            tracks = listOf(PlayTrack("english", TrackType.Audio, selected = true, delivery = TrackDelivery.File, index = 1)),
            parts = listOf(PlayPart(DURATION_MS)),
            chapters = chapters,
            markers = markers,
            dialogueBoost = DialogueBoost(enabled = false, cost = "audioAsAac"),
        )
    }
}
