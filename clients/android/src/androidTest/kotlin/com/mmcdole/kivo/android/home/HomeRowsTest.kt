package com.mmcdole.kivo.android.home

import android.os.SystemClock
import android.util.Log
import android.view.Choreographer
import android.view.KeyEvent
import android.view.View
import android.view.ViewGroup
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.runtime.CompositionLocalProvider
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
import com.mmcdole.kivo.android.theme.KivoTheme
import com.mmcdole.kivo.android.ui.LocalSession
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.api.infrastructure.ApiClient
import com.mmcdole.kivo.api.models.Home
import com.mmcdole.kivo.api.models.HomeRow
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.LibraryList
import com.mmcdole.kivo.api.models.LibraryRef
import com.mmcdole.kivo.api.models.ServerInfo
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.auth.SavedServer
import com.mmcdole.kivo.core.session.KivoClient
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockRequestHandleScope
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.KSerializer
import org.junit.After
import org.junit.Test
import org.junit.runner.RunWith
import java.io.File
import java.util.UUID
import kotlin.concurrent.thread
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

// Home with a dozen rows of posters, on an activity of its own with no
// compose test rule, so its frames come from the display as on the TV:
// holding Down keeps a card focused and on screen at every frame, moving
// focus at most a row a frame, and Right along a row does the same. The
// screen is read from its semantics on the main thread.

@RunWith(AndroidJUnit4::class)
class HomeRowsTest {
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val context = instrumentation.targetContext
    private val preferences = File(context.cacheDir, "home-test-${UUID.randomUUID()}.preferences_pb")
    private var scenario: ActivityScenario<ComponentActivity>? = null
    private var root: ViewRootForTest? = null

    private val server = MockEngine { request ->
        when (request.url.encodedPath) {
            "/api/v1/server" -> json(ServerInfo.serializer(), ServerInfo("home-test", "Home test", "1.0"))
            "/api/v1/libraries" -> json(LibraryList.serializer(), LibraryList(libraries))
            "/api/v1/home" -> json(Home.serializer(), Home(libraries.mapIndexed { r, library -> row(r, library) }))
            else -> respond("""{"status":404,"code":"notFound"}""", HttpStatusCode.NotFound, headersOf(HttpHeaders.ContentType, "application/problem+json"))
        }
    }

    @After
    fun close() {
        scenario?.close()
        preferences.delete()
    }

    @Test
    fun holdingDownKeepsACardFocusedAndMovesARowAFrameAtMost() {
        open()
        waitFor { focusedTag() == "card:${name(0, 0)}" }
        val held = hold(KeyEvent.KEYCODE_DPAD_DOWN)
        val row = assertNotNull(held.focused?.removePrefix("card:")?.substringBefore(" ")?.removePrefix("R")?.toIntOrNull(), "a row's card focused: ${held.focused}")
        assertTrue(row > 0, "focus moved down the rows")
        assertTrue(row <= held.frames, "focus moved $row rows in ${held.frames} frames")
    }

    @Test
    fun holdingRightKeepsACardFocusedAlongTheRow() {
        open()
        waitFor { focusedTag() == "card:${name(0, 0)}" }
        val held = hold(KeyEvent.KEYCODE_DPAD_RIGHT, ms = 1_000)
        val card = assertNotNull(held.focused?.substringAfterLast(" ")?.toIntOrNull(), "a card of the first row focused: ${held.focused}")
        assertTrue(held.focused.startsWith("card:${name(0, 0).substringBefore(" ")} "), "focus stayed in the first row")
        assertTrue(card > 0, "focus moved along the row")
    }

    /** Down from row to row, to the last: each row's focused card comes to rest at the first row's height. */
    @Test
    fun theFocusedRowHoldsOneHeight() {
        open()
        waitFor { focusedTag() == "card:${name(0, 0)}" }
        val first = restingTop()
        for (r in listOf(1, 2, ROWS - 1)) {
            while (focusedTag() != "card:${name(r, 0)}") {
                instrumentation.sendKeyDownUpSync(KeyEvent.KEYCODE_DPAD_DOWN)
                SystemClock.sleep(120)
            }
            assertEquals(first, restingTop(), "row $r's height")
        }
    }

    /** The focused card's top once it has stopped moving, its focus scale settled. */
    private fun restingTop(): Float {
        var top = Float.NaN
        waitFor {
            SystemClock.sleep(400)
            val now = onMain { nodes().first(::isFocused).boundsInWindow.top }
            (now == top).also { top = now }
        }
        return top
    }

    private class Held(val focused: String?, val frames: Int)

    /**
     * [key] held for [ms], repeating every 50 ms as a remote does: at every
     * frame a card is focused and on screen. Gives the card focused once the
     * list comes to rest, and how many frames were drawn while it was held.
     */
    private fun hold(key: Int, ms: Long = HOLD_MS): Held {
        val window = onMain { Rect(0f, 0f, root!!.view.width.toFloat(), root!!.view.height.toFloat()) }
        val lost = mutableListOf<String>()
        var frames = 0
        var slowFrames = 0
        var lastFrame = 0L
        var holding = true
        var lastProblem: String? = null
        val everyFrame = object : Choreographer.FrameCallback {
            override fun doFrame(nanos: Long) {
                val now = nanos / 1_000_000
                if (lastFrame > 0 && now - lastFrame > 50) slowFrames++
                lastFrame = now
                frames++
                // Read from the last frame's layout: a card focused since is brought on screen by this frame's.
                val focused = nodes().firstOrNull(::isFocused)
                val problem = when {
                    focused == null -> "nothing focused"
                    !focused.boundsInWindow.overlaps(window) -> "${tag(focused)} off screen"
                    tag(focused)?.startsWith("card:") != true -> "${tag(focused)} focused"
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
            while (SystemClock.uptimeMillis() - started < ms) {
                instrumentation.sendKeySync(KeyEvent(started, SystemClock.uptimeMillis(), KeyEvent.ACTION_DOWN, key, repeat++))
                SystemClock.sleep(50)
            }
            instrumentation.sendKeySync(KeyEvent(started, SystemClock.uptimeMillis(), KeyEvent.ACTION_UP, key, 0))
        }.join()
        holding = false
        var rest = onMain { nodes().firstOrNull(::isFocused)?.let { tag(it) to it.boundsInWindow } }
        waitFor {
            SystemClock.sleep(50)
            val now = onMain { nodes().firstOrNull(::isFocused)?.let { tag(it) to it.boundsInWindow } }
            (now?.second == rest?.second).also { rest = now }
        }
        Log.i("HomeRowsTest", "hold $key: $frames frames, $slowFrames over 50 ms; focus at rest ${rest?.first}")
        assertEquals(emptyList(), lost.take(5), "a card stayed focused and on screen")
        return Held(rest?.first, frames)
    }

    private fun open() {
        val graph = (context.applicationContext as KivoApp).graph
        val core = KivoClient(TestDevice(context, preferences), server)
        val kivo = runBlocking { core.restore(SavedServer("home-test", "Home test", listOf("http://kivo.test"))) }
        val session = ServerSession(graph, kivo, emptyMap())
        val navigator = Navigator()
        scenario = ActivityScenario.launch(ComponentActivity::class.java).onActivity {
            it.setContent {
                KivoTheme {
                    CompositionLocalProvider(LocalSession provides session) {
                        HomeScreen(session, navigator)
                    }
                }
            }
        }
        waitFor { onMain { scenario!!.onActivity { root = find(it.window.decorView) } }.let { root != null } }
    }

    private fun find(view: View): ViewRootForTest? = view as? ViewRootForTest
        ?: (view as? ViewGroup)?.let { group -> (0 until group.childCount).firstNotNullOfOrNull { find(group.getChildAt(it)) } }

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
        const val ROWS = 12
        const val CARDS = 20
        const val HOLD_MS = 3_000L

        val libraries = (0 until ROWS).map { Library("lib$it", "Movies $it", true, "movies", CARDS) }

        /** Row [r]'s card [i]: its row first, so a tag says where it is. */
        fun name(r: Int, i: Int) = "R$r Title $i"

        fun row(r: Int, library: Library) = HomeRow(
            id = "recentlyAdded:${library.id}",
            kind = "recentlyAdded",
            items = (0 until CARDS).map { i ->
                WorkSummary(id = "w$r-$i", kind = "movie", title = name(r, i), matchState = "matched", available = true, images = WorkImages(), libraryId = library.id)
            },
            library = LibraryRef(library.id, library.name, library.kind),
        )
    }
}
