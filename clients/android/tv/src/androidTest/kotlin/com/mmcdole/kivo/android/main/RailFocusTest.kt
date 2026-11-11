package com.mmcdole.kivo.android.main

import android.os.SystemClock
import android.view.KeyEvent
import android.view.View
import android.view.ViewGroup
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.ViewRootForTest
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsNode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getAllSemanticsNodes
import androidx.compose.ui.semantics.getOrNull
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.mmcdole.kivo.android.HasAppGraph
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.TestDevice
import com.mmcdole.kivo.android.theme.KivoTheme
import com.mmcdole.kivo.android.ui.LocalSession
import com.mmcdole.kivo.api.infrastructure.ApiClient
import com.mmcdole.kivo.api.models.Home
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.LibraryKind
import com.mmcdole.kivo.api.models.LibraryList
import com.mmcdole.kivo.api.models.ServerInfo
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
import kotlin.test.assertEquals

// The rail over an empty Home, with more pinned libraries than fit, so the
// libraries scroll: Down from Lists lands on the first library and Up
// from Settings on the last, however the libraries were left scrolled.
// Focus moves by where items are laid out, in view or not, so it passed
// over the libraries scrolled level with Lists or Settings.

@RunWith(AndroidJUnit4::class)
class RailFocusTest {
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val context = instrumentation.targetContext
    private val preferences = File(context.cacheDir, "rail-test-${UUID.randomUUID()}.preferences_pb")
    private var scenario: ActivityScenario<ComponentActivity>? = null
    private var root: ViewRootForTest? = null

    private val libraries = (0 until LIBRARIES).map { Library("lib$it", "Library $it", true, LibraryKind.Movies, 10) }

    private val server = MockEngine { request ->
        when (request.url.encodedPath) {
            "/api/v1/server" -> json(ServerInfo.serializer(), ServerInfo("rail-test", "Rail test", "1.0"))
            "/api/v1/libraries" -> json(LibraryList.serializer(), LibraryList(libraries))
            // An empty Home hands focus to the rail.
            "/api/v1/home" -> json(Home.serializer(), Home(emptyList()))
            else -> respond("""{"status":404,"code":"notFound"}""", HttpStatusCode.NotFound, headersOf(HttpHeaders.ContentType, "application/problem+json"))
        }
    }

    @After
    fun close() {
        scenario?.close()
        preferences.delete()
    }

    @Test
    fun downFromListsLandsOnTheFirstLibraryWithTheLibrariesScrolledToTheirEnd() {
        open()
        waitFor { focusedTag() == "rail:Home" }
        scrollLibraries(10_000f)
        press(KeyEvent.KEYCODE_DPAD_DOWN)
        waitFor { focusedTag() == "rail:Lists" }
        press(KeyEvent.KEYCODE_DPAD_DOWN)
        assertEquals("rail:${libraries.first().name}", focusedTag())
    }

    @Test
    fun upFromSettingsLandsOnTheLastLibraryWithTheLibrariesScrolledToTheirStart() {
        open()
        waitFor { focusedTag() == "rail:Home" }
        repeat(LIBRARIES + 2) { press(KeyEvent.KEYCODE_DPAD_DOWN) }
        waitFor { focusedTag() == "rail:Settings" }
        scrollLibraries(-10_000f)
        press(KeyEvent.KEYCODE_DPAD_UP)
        assertEquals("rail:${libraries.last().name}", focusedTag())
    }

    /** Opens the signed-in app over the mock server. */
    private fun open() {
        val graph = (context.applicationContext as HasAppGraph).graph
        val core = KivoClient(TestDevice(context, preferences), server)
        val kivo = runBlocking { core.restore(SavedServer("rail-test", "Rail test", listOf("http://kivo.test"))) }
        val session = ServerSession(graph, kivo)
        scenario = ActivityScenario.launch(ComponentActivity::class.java).onActivity {
            it.setContent {
                KivoTheme {
                    CompositionLocalProvider(LocalSession provides session) {
                        MainScreen(session, onExit = {})
                    }
                }
            }
        }
        waitFor { onMain { scenario!!.onActivity { root = find(it.window.decorView) } }.let { root != null } }
    }

    /** Scrolls the rail's libraries by [by] pixels, as far as they go, and waits for them to stop. */
    private fun scrollLibraries(by: Float) {
        val list = { nodes().first { node -> node.config.getOrNull(SemanticsActions.ScrollBy) != null && node.children.any { tag(it)?.startsWith("rail:") == true } } }
        onMain { list().config[SemanticsActions.ScrollBy].action!!.invoke(0f, by) }
        var last = onMain { list().children.first().boundsInWindow }
        waitFor {
            SystemClock.sleep(100)
            val now = onMain { list().children.first().boundsInWindow }
            (now == last).also { last = now }
        }
    }

    private fun find(view: View): ViewRootForTest? = view as? ViewRootForTest
        ?: (view as? ViewGroup)?.let { group -> (0 until group.childCount).firstNotNullOfOrNull { find(group.getChildAt(it)) } }

    private fun press(key: Int) {
        instrumentation.sendKeyDownUpSync(key)
        SystemClock.sleep(300)
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
        /** More than the rail's room for libraries on a 1080p TV. */
        const val LIBRARIES = 12
    }
}
