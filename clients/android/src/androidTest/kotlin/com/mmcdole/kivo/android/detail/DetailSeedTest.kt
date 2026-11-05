package com.mmcdole.kivo.android.detail

import android.os.SystemClock
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.SemanticsNodeInteraction
import androidx.compose.ui.test.assertIsFocused
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.mmcdole.kivo.android.KivoApp
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.TestDevice
import com.mmcdole.kivo.android.theme.KivoTheme
import com.mmcdole.kivo.android.ui.LocalSession
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.api.infrastructure.ApiClient
import com.mmcdole.kivo.api.models.EpisodeInfo
import com.mmcdole.kivo.api.models.ImageRef
import com.mmcdole.kivo.api.models.SeasonInfo
import com.mmcdole.kivo.api.models.SeriesInfo
import com.mmcdole.kivo.api.models.ServerInfo
import com.mmcdole.kivo.api.models.Version
import com.mmcdole.kivo.api.models.VersionPart
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.api.models.WorkSummaryList
import com.mmcdole.kivo.api.models.WorkSummaryPage
import com.mmcdole.kivo.core.auth.SavedServer
import com.mmcdole.kivo.core.data.GridQuery
import com.mmcdole.kivo.core.session.KivoClient
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockRequestHandleScope
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.KSerializer
import org.junit.After
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import java.io.File
import java.util.UUID
import java.util.concurrent.CopyOnWriteArrayList
import kotlin.test.assertFalse
import kotlin.test.assertTrue

// A detail page opened from a card shows what the card had at once, its
// artwork, title and details line, with Play, while the page is read, and
// fills in around them: Play keeps focus throughout.

@RunWith(AndroidJUnit4::class)
class DetailSeedTest {
    @get:Rule
    val compose = createComposeRule()

    private val context = InstrumentationRegistry.getInstrumentation().targetContext
    private val preferences = File(context.cacheDir, "detail-test-${UUID.randomUUID()}.preferences_pb")

    /** The requests the server saw, as "METHOD /path". */
    private val requests: MutableList<String> = CopyOnWriteArrayList()

    /** Let the server answer for the movie's and the series' pages, which it holds back until then. */
    private val movieAnswers = CompletableDeferred<Unit>()
    private val seriesAnswers = CompletableDeferred<Unit>()

    private val server = MockEngine { request ->
        val path = request.url.encodedPath
        requests += "${request.method.value} $path"
        when (path) {
            "/api/v1/server" -> json(ServerInfo.serializer(), ServerInfo("detail-test", "Detail test", "1.0"))
            "/api/v1/libraries/library/works" ->
                json(WorkSummaryPage.serializer(), WorkSummaryPage(listOf(card, season, card.copy(id = "gone", title = "Ronin")), 3))
            "/api/v1/works/series/children" -> json(WorkSummaryList.serializer(), WorkSummaryList(listOf(season, season2)))
            "/api/v1/works/season/children" -> json(WorkSummaryList.serializer(), WorkSummaryList(listOf(episode)))
            "/api/v1/works/movie" -> {
                movieAnswers.await()
                json(WorkDetail.serializer(), movie)
            }
            "/api/v1/works/gone" -> respond(
                """{"status":403,"code":"forbidden","title":"Forbidden"}""",
                HttpStatusCode.Forbidden,
                headersOf(HttpHeaders.ContentType, "application/problem+json"),
            )
            "/api/v1/works/series" -> {
                seriesAnswers.await()
                json(WorkDetail.serializer(), series)
            }
            else -> respond("""{"status":404,"code":"notFound"}""", HttpStatusCode.NotFound, headersOf(HttpHeaders.ContentType, "application/problem+json"))
        }
    }

    @After
    fun close() {
        movieAnswers.complete(Unit)
        seriesAnswers.complete(Unit)
        preferences.delete()
    }

    @Test
    fun aCardsPageShowsAtOnceAndPlayKeepsFocusAsItFillsIn() {
        open("movie")
        waitFor { shows(text("Heat")) && focused(node("play")) }
        assertTrue(shows(text("1995 · ", substring = true)), "the card's details line")
        assertFalse(shows(text("A heist.")), "no overview before the page is read")

        movieAnswers.complete(Unit)
        waitFor { shows(text("A heist.")) }
        assertTrue(shows(text(" · Crime", substring = true)), "genres join the line")
        assertTrue(focused(node("play")), "Play kept focus")
    }

    @Test
    fun aSeasonsCardOpensItsSeriesOnItWithoutReadingTheSeasonFirst() {
        open("season")
        waitFor { shows(text("The Wire")) && focused(node("play")) && shows(text("The Target", substring = true)) }
        assertFalse("GET /api/v1/works/season" in requests, "the season's card says which series")

        seriesAnswers.complete(Unit)
        waitFor { shows(text("Baltimore.")) }
        assertTrue(focused(node("play")), "Play kept focus")
        assertFalse("GET /api/v1/works/season" in requests)
    }

    @Test
    fun aPageThatCantBeReadSaysSoUnderWhatTheCardShowed() {
        open("gone")
        waitFor { shows(text("Ronin")) && shows(text("Try again")) }
        assertTrue(focused(node("play")), "Play still has focus")
    }

    /** Reads the library's grid, as a library screen does, then opens [id]'s page. */
    private fun open(id: String) {
        val graph = (context.applicationContext as KivoApp).graph
        val core = KivoClient(TestDevice(context, preferences), server)
        val kivo = runBlocking { core.restore(SavedServer("detail-test", "Detail test", listOf("http://kivo.test"))) }
        runBlocking { kivo.library.gridPage(GridQuery("library"), 0, 50) }
        val session = ServerSession(graph, kivo, emptyMap())
        val navigator = Navigator()
        compose.setContent {
            KivoTheme {
                CompositionLocalProvider(LocalSession provides session) {
                    DetailScreen(session, id, navigator)
                }
            }
        }
    }

    private fun node(tag: String): SemanticsNodeInteraction = compose.onNodeWithTag(tag)

    private fun text(text: String, substring: Boolean = false): SemanticsMatcher = hasText(text, substring)

    private fun focused(node: SemanticsNodeInteraction): Boolean = runCatching { node.assertIsFocused() }.isSuccess

    private fun shows(matcher: SemanticsMatcher): Boolean = compose.onAllNodes(matcher).fetchSemanticsNodes().isNotEmpty()

    /** Waits for [condition], for up to five seconds. */
    private fun waitFor(condition: () -> Boolean) {
        val until = SystemClock.uptimeMillis() + 5_000
        while (!condition()) {
            check(SystemClock.uptimeMillis() < until) { "timed out" }
            compose.waitForIdle()
            SystemClock.sleep(16)
        }
    }

    private fun <T> MockRequestHandleScope.json(serializer: KSerializer<T>, value: T) =
        respond(ApiClient.JSON_DEFAULT.encodeToString(serializer, value), HttpStatusCode.OK, headersOf(HttpHeaders.ContentType, "application/json"))

    private companion object {
        val backdrop = ImageRef("backdrop", blurhash = "LEHV6nWB2yk8pyo0adR*.7kCMdnj")

        val card = WorkSummary(
            id = "movie", kind = "movie", title = "Heat", matchState = "matched", available = true,
            images = WorkImages(backdrop = backdrop), year = 1995, durationMs = 10_200_000, communityRating = 8.3,
            communityVotes = 9000, contentRating = "R", libraryId = "library",
        )

        val movie = WorkDetail(
            id = "movie", kind = "movie", title = "Heat", matchState = "matched", available = true,
            images = WorkImages(backdrop = backdrop), genres = listOf("Crime"), studios = emptyList(), credits = emptyList(),
            externalIds = emptyList(),
            versions = listOf(Version("version", true, 10_200_000, 1_000, listOf(VersionPart("file", "heat.mkv", 10_200_000, 1_000)), emptyList(), listOf("movie"), "1080p")),
            extras = emptyList(), collections = emptyList(), year = 1995, durationMs = 10_200_000, communityRating = 8.3,
            communityVotes = 9000, contentRating = "R", overview = "A heist.", libraryId = "library",
        )

        val season = WorkSummary(
            id = "season", kind = "season", title = "Season 1", matchState = "matched", available = true,
            images = WorkImages(backdrop = backdrop), season = SeasonInfo(1, "series", "The Wire", 1, episodeCount = 1),
        )

        val season2 = WorkSummary(
            id = "season-2", kind = "season", title = "Season 2", matchState = "matched", available = true,
            images = WorkImages(backdrop = backdrop), season = SeasonInfo(2, "series", "The Wire", 0, episodeCount = 1),
        )

        val episode = WorkSummary(
            id = "episode", kind = "episode", title = "The Target", matchState = "matched", available = true,
            images = WorkImages(), durationMs = 3_600_000,
            episode = EpisodeInfo(1, 1, "series", "The Wire", seasonId = "season"),
        )

        val series = WorkDetail(
            id = "series", kind = "series", title = "The Wire", matchState = "matched", available = true,
            images = WorkImages(backdrop = backdrop), genres = emptyList(), studios = emptyList(), credits = emptyList(),
            externalIds = emptyList(), versions = emptyList(), extras = emptyList(), collections = emptyList(), year = 2002,
            series = SeriesInfo(1, seasonCount = 2, nextUp = episode), overview = "Baltimore.",
        )
    }
}
