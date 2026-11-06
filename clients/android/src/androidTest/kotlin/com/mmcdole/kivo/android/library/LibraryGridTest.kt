package com.mmcdole.kivo.android.library

import android.os.SystemClock
import android.util.Log
import android.view.Choreographer
import android.view.KeyEvent
import android.view.View
import android.view.ViewGroup
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.saveable.rememberSaveableStateHolder
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.platform.ViewRootForTest
import androidx.compose.ui.semantics.SemanticsNode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getAllSemanticsNodes
import androidx.compose.ui.semantics.getOrNull
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.mmcdole.kivo.android.KivoApp
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.TestDevice
import com.mmcdole.kivo.android.favorites.FavoritesGridScreen
import com.mmcdole.kivo.android.theme.KivoTheme
import com.mmcdole.kivo.android.ui.LocalSession
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.PosterColumns
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.api.infrastructure.ApiClient
import com.mmcdole.kivo.api.models.LetterStart
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.LibraryList
import com.mmcdole.kivo.api.models.ServerInfo
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.api.models.CollectionSummaryPage
import com.mmcdole.kivo.api.models.WorkSummaryPage
import com.mmcdole.kivo.core.auth.SavedServer
import com.mmcdole.kivo.core.data.GridQuery
import com.mmcdole.kivo.core.data.GridPages.Companion.RAIL_LETTERS
import com.mmcdole.kivo.core.session.KivoClient
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockRequestHandleScope
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.KSerializer
import org.junit.After
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import java.io.File
import java.util.UUID
import java.util.concurrent.CopyOnWriteArrayList
import kotlin.concurrent.thread
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

// A library grid of 3,000 works over a server answering each page 300 ms
// late, on an activity of its own with no compose test rule, so its frames
// come from the display as on the TV rather than a test clock: it opens on
// its first cards with the first focused; holding Down keeps a card focused
// and on screen at every frame, placeholder or not, and once let go the
// cards on screen fill in, focus staying on its card; the letter rail jumps
// to a letter's works; Back from a work's page returns focus to its card. The
// screen is read from its semantics on the main thread.

@RunWith(AndroidJUnit4::class)
class LibraryGridTest {
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val context = instrumentation.targetContext
    private val preferences = File(context.cacheDir, "grid-test-${UUID.randomUUID()}.preferences_pb")
    private var scenario: ActivityScenario<ComponentActivity>? = null
    private var root: ViewRootForTest? = null

    /** The pages the server was asked for, as (when, query). */
    private val pages: MutableList<Pair<Long, String>> = CopyOnWriteArrayList()

    private val server = MockEngine { request ->
        when (request.url.encodedPath) {
            "/api/v1/server" -> json(ServerInfo.serializer(), ServerInfo("grid-test", "Grid test", "1.0"))
            // 130 favorite movies, the works from 1,000 on.
            "/api/v1/favorites" -> {
                val params = request.url.parameters
                val start = params["offset"]?.toInt() ?: 0
                val end = minOf(start + (params["limit"]?.toInt() ?: 60), FAVORITES)
                json(WorkSummaryPage.serializer(), WorkSummaryPage((start until end).map { work(1_000 + it) }, total = FAVORITES))
            }
            "/api/v1/libraries" -> json(LibraryList.serializer(), LibraryList(listOf(Library("lib", "Movies", true, "movies", TOTAL))))
            // The library has no collections.
            "/api/v1/libraries/lib/collections" -> json(CollectionSummaryPage.serializer(), CollectionSummaryPage(emptyList(), 0))
            "/api/v1/libraries/lib/works" -> {
                val params = request.url.parameters
                val limit = params["limit"]?.toInt() ?: 60
                // Whether the library has works of a resolution or HDR format: none.
                if (params["resolution"] != null || params["hdr"] != null) {
                    json(WorkSummaryPage.serializer(), WorkSummaryPage(emptyList(), total = 0))
                } else {
                    pages += SystemClock.uptimeMillis() to request.url.encodedQuery
                    val start = params["offset"]?.toInt() ?: 0
                    delay(LATENCY_MS)
                    val end = minOf(start + limit, TOTAL)
                    json(
                        WorkSummaryPage.serializer(),
                        WorkSummaryPage(
                            // Date added is the works last to first.
                            items = (start until end).map { if (params["sort"] == "addedAt") work(TOTAL - 1 - it) else work(it) },
                            total = TOTAL,
                            letters = letters.takeIf { start == 0 && params["sort"] != "addedAt" },
                        ),
                    )
                }
            }
            else -> respond("""{"status":404,"code":"notFound"}""", HttpStatusCode.NotFound, headersOf(HttpHeaders.ContentType, "application/problem+json"))
        }
    }

    /** A library opens as it was left, and that's kept on the device between tests and runs, as the sort test leaves it. */
    @Before
    fun forgetTheGrid() = runBlocking {
        (context.applicationContext as KivoApp).graph.savedGrids.save(GridChoice(GridQuery("lib")))
    }

    @After
    fun close() {
        scenario?.close()
        preferences.delete()
    }

    @Test
    fun aLibraryOpensOnItsFirstCardsWithTheFirstFocused() {
        open()
        waitFor { focusedTag() == "card:${name(0)}" }
        assertTrue(shows("card:${name(PosterColumns - 1)}"), "the first line's cards")
    }

    /**
     * Down held for four seconds, repeating every 50 ms as a remote does: at
     * every frame a card is focused and on screen, placeholder or not; once
     * let go, the cards on screen fill in shortly after, and focus stays on
     * its card as they do.
     */
    @Test
    fun holdingDownKeepsACardFocusedAndFillsInWhereItStops() {
        open()
        waitFor { focusedTag() == "card:${name(0)}" }
        val window = onMain { Rect(0f, 0f, root!!.view.width.toFloat(), root!!.view.height.toFloat()) }

        val lost = mutableListOf<String>()
        var frames = 0
        var slowFrames = 0
        var longestFrameMs = 0L
        var lastFrame = 0L
        var holding = true
        var lastProblem: String? = null
        val everyFrame = object : Choreographer.FrameCallback {
            override fun doFrame(nanos: Long) {
                val ms = nanos / 1_000_000
                if (lastFrame > 0) {
                    longestFrameMs = maxOf(longestFrameMs, ms - lastFrame)
                    if (ms - lastFrame > 50) slowFrames++
                }
                lastFrame = ms
                frames++
                // Read as the frame starts, so from the last frame's layout: a card focused by a key since is
                // brought on screen by this frame's, so it counts as lost only when it's still off a frame later.
                val focused = nodes().firstOrNull(::isFocused)
                val problem = when {
                    focused == null -> "nothing focused"
                    !focused.boundsInWindow.overlaps(window) -> "${tag(focused)} off screen"
                    tag(focused)?.let { it.startsWith("card:") || it == "placeholder" } != true -> "${tag(focused)} focused"
                    else -> null
                }
                if (problem != null && problem == lastProblem) lost += "frame $frames: $problem"
                lastProblem = problem
                if (holding) Choreographer.getInstance().postFrameCallback(this)
            }
        }
        instrumentation.runOnMainSync { Choreographer.getInstance().postFrameCallback(everyFrame) }

        val started = SystemClock.uptimeMillis()
        thread {
            var repeat = 0
            while (SystemClock.uptimeMillis() - started < HOLD_MS) {
                instrumentation.sendKeySync(KeyEvent(started, SystemClock.uptimeMillis(), KeyEvent.ACTION_DOWN, KeyEvent.KEYCODE_DPAD_DOWN, repeat++))
                SystemClock.sleep(50)
            }
            instrumentation.sendKeySync(KeyEvent(started, SystemClock.uptimeMillis(), KeyEvent.ACTION_UP, KeyEvent.KEYCODE_DPAD_DOWN, 0))
        }.join()
        val released = SystemClock.uptimeMillis()
        holding = false
        val duringHold = pages.count { it.first in started..released }
        // Where focus came to rest, once the grid's last scroll ends.
        var atRelease = onMain { nodes().firstOrNull(::isFocused)?.let { tag(it) to it.boundsInWindow } }
        waitFor {
            SystemClock.sleep(50)
            val now = onMain { nodes().firstOrNull(::isFocused)?.let { tag(it) to it.boundsInWindow } }
            (now?.second == atRelease?.second).also { atRelease = now }
        }

        waitFor(10_000) { placeholdersOnScreen(window) == 0 }
        val filledMs = SystemClock.uptimeMillis() - released
        val after = onMain { nodes().firstOrNull(::isFocused)?.let { tag(it) to it.boundsInWindow } }
        SystemClock.sleep(1_000)
        val afterQuiet = pages.size
        Log.i(
            "LibraryGridTest",
            "hold: $duringHold page requests during the hold, ${pages.count { it.first > released }} after; filled $filledMs ms after release; " +
                "$frames frames, $slowFrames over 50 ms, longest $longestFrameMs ms; focus at release $atRelease, filled $after",
        )
        assertEquals(emptyList(), lost.take(5), "a card stayed focused and on screen")
        assertNotNull(after)
        assertTrue(after.first != "card:${name(0)}", "focus moved down the grid")
        assertTrue(after.first!!.startsWith("card:"), "the focused card filled in")
        assertEquals(atRelease?.second, after.second, "focus stayed in its place as its page arrived")
        val rested = atRelease
        if (rested?.first?.startsWith("card:") == true) assertEquals(rested.first, after.first)
        assertTrue(filledMs < 3_000, "the cards filled in $filledMs ms after release")
        assertEquals(afterQuiet, pages.size, "no requests once filled in")
    }

    @Test
    fun theLetterRailJumpsToALettersWorks() {
        open()
        waitFor { focusedTag() == "card:${name(0)}" }
        val m = letters.first { it.letter == "M" }
        // Right from the last card of the first line reaches the rail, at the top's letter; Down to M and OK.
        repeat(PosterColumns) { press(KeyEvent.KEYCODE_DPAD_RIGHT) }
        waitFor { focusedTag() == "letters" }
        repeat(RAIL_LETTERS.indexOf("M") - RAIL_LETTERS.indexOf("A")) { press(KeyEvent.KEYCODE_DPAD_DOWN) }
        press(KeyEvent.KEYCODE_DPAD_CENTER)
        waitFor(10_000) { shows("card:${name(m.offset)}") }
        // Left goes back to the letter's first item rather than the nearest card.
        press(KeyEvent.KEYCODE_DPAD_LEFT)
        waitFor(10_000) { focusedTag() == "card:${name(m.offset)}" }
    }

    /** Another sort from Sort: its works show in their order, and focus stays on Sort, the grid taking none. */
    @Test
    fun anotherSortShowsItsOrderAndFocusStaysOnSort() {
        open()
        waitFor { focusedTag() == "card:${name(0)}" }
        press(KeyEvent.KEYCODE_DPAD_UP)
        waitFor { focusedTag() in setOf("sort", "filter") }
        if (focusedTag() == "filter") press(KeyEvent.KEYCODE_DPAD_LEFT)
        waitFor { focusedTag() == "sort" }
        // The panel opens on Title; Date added is next.
        press(KeyEvent.KEYCODE_DPAD_CENTER)
        SystemClock.sleep(500)
        press(KeyEvent.KEYCODE_DPAD_DOWN)
        press(KeyEvent.KEYCODE_DPAD_CENTER)
        waitFor(10_000) { shows("card:${name(TOTAL - 1)}") }
        SystemClock.sleep(500)
        assertEquals("sort", focusedTag(), "focus stayed on Sort")
        assertTrue(!shows("letters"), "no letter rail by date added")
    }

    /** Back from a work's page focuses the card it opened from, with its line scrolled well past the first. */
    @Test
    fun backFromAWorkFocusesItsCard() {
        lateinit var shown: Navigator
        open { session, navigator ->
            shown = navigator
            navigator.Host(rememberSaveableStateHolder()) { target ->
                if (target !is Screen.Work) LibraryScreen(session, "lib", navigator)
            }
        }
        waitFor { focusedTag()?.startsWith("card:") == true }
        val first = focusedTag()
        repeat(8) { press(KeyEvent.KEYCODE_DPAD_DOWN) }
        SystemClock.sleep(1_000)
        val card = assertNotNull(focusedTag())
        assertTrue(card != first, "focus moved down the grid")
        press(KeyEvent.KEYCODE_DPAD_CENTER)
        waitFor { !shows(card) }
        onMain { shown.pop() }
        waitFor { focusedTag() == card }
    }

    /** Down from the count line, which stands over the grid's list, goes back to the grid's cards. */
    @Test
    fun downFromTheCountLineFocusesACard() {
        open()
        waitFor { focusedTag() == "card:${name(0)}" }
        press(KeyEvent.KEYCODE_DPAD_UP)
        waitFor { focusedTag() == "sort" }
        press(KeyEvent.KEYCODE_DPAD_DOWN)
        waitFor { focusedTag() == "card:${name(0)}" }
    }

    /** A kind's favorites open on their first cards with the first focused, and Down to the last line fills it in. */
    @Test
    fun favoritesOpenOnTheirFirstCardsAndFillInToTheLast() {
        open { session, navigator -> FavoritesGridScreen(session, "movie", navigator) }
        waitFor { focusedTag() == "card:${name(1_000)}" }
        repeat((FAVORITES - 1) / PosterColumns) { press(KeyEvent.KEYCODE_DPAD_DOWN) }
        waitFor(10_000) { shows("card:${name(1_000 + FAVORITES - 1)}") }
    }

    /** Opens [screen] over the mock server: the library's, unless another. */
    private fun open(screen: @Composable (ServerSession, Navigator) -> Unit = { session, navigator -> LibraryScreen(session, "lib", navigator) }) {
        val graph = (context.applicationContext as KivoApp).graph
        val core = KivoClient(TestDevice(context, preferences), server)
        val kivo = runBlocking { core.restore(SavedServer("grid-test", "Grid test", listOf("http://kivo.test"))) }
        val session = ServerSession(graph, kivo, emptyMap())
        val navigator = Navigator()
        scenario = ActivityScenario.launch(ComponentActivity::class.java).onActivity {
            it.setContent {
                KivoTheme {
                    CompositionLocalProvider(LocalSession provides session) {
                        screen(session, navigator)
                    }
                }
            }
        }
        waitFor { onMain { scenario!!.onActivity { root = find(it.window.decorView) } }.let { root != null } }
    }

    private fun find(view: View): ViewRootForTest? = view as? ViewRootForTest
        ?: (view as? ViewGroup)?.let { group -> (0 until group.childCount).firstNotNullOfOrNull { find(group.getChildAt(it)) } }

    private fun press(key: Int) {
        instrumentation.sendKeyDownUpSync(key)
        SystemClock.sleep(120)
    }

    private fun <T> onMain(block: () -> T): T {
        var result: Result<T>? = null
        instrumentation.runOnMainSync { result = runCatching(block) }
        return result!!.getOrThrow()
    }

    /** The screen's semantics, unmerged; on the main thread. */
    private fun nodes(): List<SemanticsNode> = root?.semanticsOwner?.getAllSemanticsNodes(mergingEnabled = false).orEmpty()

    private fun tag(node: SemanticsNode): String? = node.config.getOrNull(SemanticsProperties.TestTag)

    private fun isFocused(node: SemanticsNode) = node.config.getOrNull(SemanticsProperties.Focused) == true

    private fun focusedTag(): String? = onMain { nodes().firstOrNull(::isFocused)?.let(::tag) }

    private fun shows(tag: String) = onMain { nodes().any { tag(it) == tag } }

    /** Placeholder cards inside [window]. */
    private fun placeholdersOnScreen(window: Rect) = onMain {
        nodes().count { tag(it) == "placeholder" && it.boundsInWindow.overlaps(window) && it.boundsInWindow.height > 0 }
    }

    /** Waits for [condition], for up to [ms]. */
    private fun waitFor(ms: Long = 5_000, condition: () -> Boolean) {
        val until = SystemClock.uptimeMillis() + ms
        while (!condition()) {
            check(SystemClock.uptimeMillis() < until) { "timed out" }
            SystemClock.sleep(16)
        }
    }

    private fun <T> MockRequestHandleScope.json(serializer: KSerializer<T>, value: T) =
        respond(ApiClient.JSON_DEFAULT.encodeToString(serializer, value), HttpStatusCode.OK, headersOf(HttpHeaders.ContentType, "application/json"))

    private companion object {
        const val TOTAL = 3_000
        const val LATENCY_MS = 300L
        const val HOLD_MS = 4_000L
        const val FAVORITES = 130

        /** Title [i]'s letter: A to Z in turn through the library. */
        fun letter(i: Int) = 'A' + i * 26 / TOTAL

        fun name(i: Int) = "${letter(i)} Title ${i.toString().padStart(4, '0')}"

        fun work(i: Int) = WorkSummary(
            id = "w$i", kind = "movie", title = name(i), matchState = "matched", available = true,
            images = WorkImages(), libraryId = "lib",
        )

        val letters = (0 until TOTAL).groupBy { letter(it).toString() }.map { (l, items) -> LetterStart(l, items.first()) }
    }
}
