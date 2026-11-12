package com.mmcdole.kivo.core.session

import com.mmcdole.kivo.api.models.EpisodeInfo
import com.mmcdole.kivo.api.models.ListKind
import com.mmcdole.kivo.api.models.MatchState
import com.mmcdole.kivo.api.models.SeasonInfo
import com.mmcdole.kivo.api.models.SeriesInfo
import com.mmcdole.kivo.api.models.UserState
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkList
import com.mmcdole.kivo.api.models.WorkListList
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.api.models.WorkSummaryList
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.data.SeasonRef
import com.mmcdole.kivo.core.data.summary
import com.mmcdole.kivo.core.json
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.onStart
import kotlinx.coroutines.flow.transformWhile
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import java.io.IOException
import java.util.concurrent.CopyOnWriteArrayList
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

// A work's page as its screen gets it from SessionLibrary.workPage: what
// shows at once, the season-to-series redirect, the season open and its
// episodes, the play choices, and My List, over reads a test answers.

private fun detail(id: String, kind: WorkKind, title: String = id, season: SeasonInfo? = null, series: SeriesInfo? = null, episode: EpisodeInfo? = null) = WorkDetail(
    id = id, kind = kind, title = title, matchState = MatchState.Matched, available = true, images = WorkImages(),
    genres = emptyList(), studios = emptyList(), credits = emptyList(), externalIds = emptyList(), versions = emptyList(),
    extras = emptyList(), collections = emptyList(), similar = emptyList(), season = season, series = series, episode = episode,
)

private fun card(id: String, kind: WorkKind, title: String = id, season: SeasonInfo? = null, episode: EpisodeInfo? = null) = WorkSummary(
    id = id, kind = kind, title = title, matchState = MatchState.Matched, available = true,
    images = WorkImages(backdrop = null), season = season, episode = episode,
)

private val season1 = card("s1", WorkKind.Season, "Season 1", season = SeasonInfo(1, "show", "Show", 1))
private val season2 = card("s2", WorkKind.Season, "Season 2", season = SeasonInfo(2, "show", "Show", 2))
private val specials = card("s0", WorkKind.Season, "Specials", season = SeasonInfo(0, "show", "Show", 0))
private fun episode(id: String, season: Int, seasonId: String) = card(id, WorkKind.Episode, season = null, episode = EpisodeInfo(season, 1, "show", "Show", seasonId = seasonId))

/** A series whose Next Up is [nextUp]. */
private fun show(nextUp: WorkSummary? = null) = detail("show", WorkKind.Series, "Show", series = SeriesInfo(3, seasonCount = 2, nextUp = nextUp))

/**
 * The library a page reads: each work's page and children as the server
 * has them, read again as a live event says ([changed]), a page that can't
 * be read ([refused]), reads held back until let go ([held]), and what the
 * session knows of a work before it reads it.
 */
private class Reads {
    val pages = mutableMapOf<String, WorkDetail>()
    val children = mutableMapOf<String, List<WorkSummary>>()
    val known = mutableMapOf<String, WorkSummary>()
    val refused = mutableSetOf<String>()
    val held = mutableMapOf<String, CompletableDeferred<Unit>>()
    val myList = MutableStateFlow<String?>(null)
    var retries = 0

    /** Every read started, as "work id" or "children id". */
    val started: MutableList<String> = CopyOnWriteArrayList()

    /** Each read still being followed. */
    val following: MutableList<String> = CopyOnWriteArrayList()

    /** A live event: every read follows it. */
    private val events = MutableStateFlow(0)

    fun changed() {
        events.value++
    }

    private fun <T : Any> read(key: String, id: String, answer: () -> T?): Flow<Load<T>> = events
        .onStart { started += key; following += key; held[id]?.await() }
        .map<Int, Load<T>> { _ ->
            if (id in refused) Load.Failed(RequestProblem.Refused(404, "notFound", null, null)) else answer()?.let { Load.Loaded(it) } ?: Load.Loading
        }
        // A refusal ends the read, as the session's do.
        .transformWhile { emit(it); it !is Load.Failed }
        .let { answers ->
            flow {
                try {
                    answers.collect { emit(it) }
                } finally {
                    following -= key
                }
            }
        }

    val wiring = WorkPageReads(
        work = { id -> read("work $id", id) { pages[id] } },
        children = { id -> read("children $id", id) { children[id] } },
        known = { known[it] },
        myListId = myList,
        retryNow = { retries++ },
    )
}

private fun TestScope.page(reads: Reads, id: String, card: WorkSummary? = null, openOn: SeasonRef? = null) =
    WorkPage(id, card, openOn, reads.wiring, backgroundScope)

private suspend fun WorkPage.until(test: (WorkPageState) -> Boolean): WorkPageState = state.first(test)

private fun WorkPageState.episodeIds() = episodes?.getOrNull()?.map { it.id }

class WorkPageTest {
    private val reads = Reads().apply {
        pages["m1"] = detail("m1", WorkKind.Movie, "Charade")
        pages["show"] = show()
        pages["s1"] = detail("s1", WorkKind.Season, "Season 1", season = season1.season)
        pages["s2"] = detail("s2", WorkKind.Season, "Season 2", season = season2.season)
        pages["e3"] = detail("e3", WorkKind.Episode, "Pilot", episode = EpisodeInfo(2, 1, "show", "Show", seasonId = "s2"))
        children["show"] = listOf(season1, season2, specials)
        children["s1"] = listOf(episode("e1", 1, "s1"))
        children["s2"] = listOf(episode("e3", 2, "s2"), episode("e4", 2, "s2"))
        children["s0"] = listOf(episode("e0", 0, "s0"))
    }

    @Test
    fun aMoviesPageIsItsPageWithNoSeasonsOrEpisodes() = runTest {
        val page = page(reads, "m1")

        val state = page.until { it.work != null }

        assertEquals("Charade", state.head?.title)
        assertEquals("m1", state.choice.workId)
        assertNull(state.seasons)
        assertNull(state.episodes)
        assertNull(state.problem)
        assertEquals(listOf("work m1"), reads.started)
    }

    @Test
    fun aSeriesOpensOnItsFirstRegularSeasonAndReadsItsEpisodes() = runTest {
        val page = page(reads, "show")

        val state = page.until { it.episodeIds() != null }

        assertEquals("Show", state.head?.title)
        assertEquals(listOf("s1", "s2", "s0"), state.seasons?.getOrNull()?.map { it.id })
        assertEquals("s1", state.season?.id)
        assertEquals("s1", state.episodesOf)
        assertEquals(listOf("e1"), state.episodeIds())
    }

    @Test
    fun aSeriesOpensOnTheSeasonHoldingNextUp() = runTest {
        reads.pages["show"] = show(nextUp = episode("e4", 2, "s2"))
        val page = page(reads, "show")

        assertEquals(listOf("e3", "e4"), page.until { it.episodeIds()?.size == 2 }.episodeIds())
        assertEquals("s2", page.state.value.season?.id)
        assertTrue("children s1" !in reads.started, "it waits for the series to know which season")
    }

    @Test
    fun anotherSeasonCanBeChosenAndTheLastOnesEpisodesStayUntilItsAreRead() = runTest {
        reads.held["s2"] = CompletableDeferred()
        val page = page(reads, "show")
        page.until { it.episodeIds() == listOf("e1") }

        page.chooseSeason("s2")
        runCurrent()
        val switching = page.state.value
        assertEquals("s2", switching.season?.id)
        assertEquals("s1", switching.episodesOf)
        assertEquals(listOf("e1"), switching.episodeIds())

        reads.held.getValue("s2").complete(Unit)
        val switched = page.until { it.episodesOf == "s2" }
        assertEquals(listOf("e3", "e4"), switched.episodeIds())
        assertFalse("children s1" in reads.following, "the last season's episodes stop being read")
    }

    @Test
    fun aSeasonKnownAsOneOpensItsSeriesAtOnceOnThatSeason() = runTest {
        reads.known["s2"] = season2
        reads.known["show"] = show().summary()
        val page = page(reads, "s2")

        assertEquals("show", page.state.value.head?.id, "the series' card shows at once")
        val state = page.until { it.work != null && it.episodeIds() != null }
        assertEquals("show", state.work?.id)
        assertEquals("s2", state.season?.id)
        assertEquals(listOf("e3", "e4"), state.episodeIds())
        assertTrue("work s2" !in reads.started, "the season's own page isn't read")
    }

    @Test
    fun aSeasonsCardWithItsSeriesUnknownShowsTheSeriesAsTheSeasonNamesIt() = runTest {
        reads.held["show"] = CompletableDeferred()
        val page = page(reads, "s2", card = season2)

        val head = page.state.value.head
        assertEquals("show", head?.id)
        assertEquals(WorkKind.Series, head?.kind)
        assertEquals("Show", head?.title)
        assertNull(page.state.value.work)
    }

    @Test
    fun aSeasonReadWithoutItsCardRedirectsToItsSeriesOnThatSeason() = runTest {
        val page = page(reads, "s2")
        assertNull(page.state.value.head, "nothing is known until the season is read")

        val state = page.until { it.work?.id == "show" && it.episodeIds() != null }

        assertEquals("show", state.head?.id)
        assertEquals("s2", state.season?.id)
        assertEquals(listOf("e3", "e4"), state.episodeIds())
        assertEquals("show", state.choice.workId)
    }

    @Test
    fun goToSeriesOpensTheEpisodesSeasonAndReadsItsEpisodesBesideTheSeasons() = runTest {
        reads.held["show"] = CompletableDeferred()
        val page = page(reads, "show", openOn = SeasonRef("s2", 2))
        runCurrent()

        assertTrue("children s2" in reads.started, "the season asked for is read before the seasons say it's there")
        reads.held.getValue("show").complete(Unit)
        val state = page.until { it.season != null && it.episodeIds() != null }
        assertEquals("s2", state.season?.id)
        assertEquals(listOf("e3", "e4"), state.episodeIds())
    }

    @Test
    fun goToSeriesFindsTheSeasonByNumberWithoutItsId() = runTest {
        val page = page(reads, "show", openOn = SeasonRef(null, 0))

        assertEquals("s0", page.until { it.season != null }.season?.id)
    }

    @Test
    fun theSeasonItOpenedOnStaysAsNextUpMoves() = runTest {
        reads.pages["show"] = show(nextUp = episode("e1", 1, "s1"))
        val page = page(reads, "show")
        page.until { it.season?.id == "s1" && it.episodeIds() != null }

        reads.pages["show"] = show(nextUp = episode("e3", 2, "s2"))
        reads.changed()
        page.until { it.work?.series?.nextUp?.id == "e3" }

        assertEquals("s1", page.state.value.season?.id)
    }

    @Test
    fun anEpisodesPageReadsItsSeasonsEpisodes() = runTest {
        val page = page(reads, "e3")

        val state = page.until { it.episodeIds() != null }

        assertEquals("s2", state.episodesOf)
        assertEquals(listOf("e3", "e4"), state.episodeIds())
        assertNull(state.seasons)
        assertNull(state.season)
    }

    @Test
    fun anEpisodesCardStartsItsSeasonsEpisodesBesideItsPage() = runTest {
        reads.held["e3"] = CompletableDeferred()
        val page = page(reads, "e3", card = episode("e3", 2, "s2"))

        assertEquals(listOf("e3", "e4"), page.until { it.episodeIds() != null }.episodeIds())
        assertNull(page.state.value.work, "before the episode's page answers")
    }

    @Test
    fun aPageShowsItsCardAtOnceThenTheRead() = runTest {
        reads.held["m1"] = CompletableDeferred()
        val page = page(reads, "m1", card = card("m1", WorkKind.Movie, "Charade (card)"))

        assertEquals("Charade (card)", page.state.value.head?.title)
        assertNull(page.state.value.work)

        reads.held.getValue("m1").complete(Unit)
        assertEquals("Charade", page.until { it.work != null }.head?.title)
    }

    @Test
    fun withoutACardItShowsWhatTheSessionKnows() = runTest {
        reads.known["m1"] = card("m1", WorkKind.Movie, "Charade (known)")
        reads.held["m1"] = CompletableDeferred()

        assertEquals("Charade (known)", page(reads, "m1").state.value.head?.title)
    }

    @Test
    fun aRefusedPageSaysWhyAndRetryReadsItAgain() = runTest {
        reads.refused += "m1"
        val page = page(reads, "m1")

        val failed = page.until { it.problem != null }
        assertEquals("notFound", failed.problem?.code)
        assertNull(failed.head)

        reads.refused -= "m1"
        page.retry()
        val read = page.until { it.work != null }
        assertNull(read.problem)
        assertEquals(1, reads.retries, "the session is asked to retry its waiting reads too")
    }

    @Test
    fun aRefusedSeasonListIsReadAgainOnRetry() = runTest {
        reads.refused += "show"
        reads.known["show"] = show().summary()
        val page = page(reads, "show")
        assertTrue(page.until { it.seasons is Load.Failed }.problem != null)

        reads.refused -= "show"
        page.retry()

        assertEquals("s1", page.until { it.season != null }.season?.id)
    }

    @Test
    fun myListAndPlayedFollowThePageAsTheLiveEventsReadItAgain() = runTest {
        val page = page(reads, "m1")
        page.until { it.work != null }
        assertFalse(page.state.value.onMyList, "not before the lists are read")

        reads.myList.value = "my"
        assertFalse(page.until { it.myListId == "my" }.onMyList)

        // A mark the server made, then the live event it sends.
        reads.pages["m1"] = detail("m1", WorkKind.Movie, "Charade").copy(listIds = listOf("my"), userState = UserState(played = true))
        reads.changed()

        val marked = page.until { it.onMyList }
        assertTrue(marked.played)
    }

    @Test
    fun anotherVersionStartsItsTracksAtTheServersChoice() = runTest {
        val page = page(reads, "m1")
        page.chooseAudio("a2")
        page.chooseSubtitle("t1")
        assertEquals("a2", page.state.value.choice.audioTrackId)
        assertEquals("t1", page.state.value.choice.subtitleTrackId)

        page.chooseVersion("v2")

        val choice = page.state.value.choice
        assertEquals("v2", choice.versionId)
        assertNull(choice.audioTrackId)
        assertNull(choice.subtitleTrackId)
        assertEquals("m1", choice.workId)
    }

    @Test
    fun closingStopsEveryRead() = runTest {
        val page = page(reads, "show")
        page.until { it.episodeIds() != null }
        assertTrue(reads.following.isNotEmpty())

        page.close()
        runCurrent()

        assertEquals(emptyList(), reads.following.toList())
    }

    @Test
    fun theSessionsPageReadsTheServer() = runTest {
        val h = Harness(this)
        h.servers.server("home.test", info("server-1", "Basement"))
        h.servers.on("home.test", "GET", "/api/v1/works/s2") { json(WorkDetail.serializer(), reads.pages.getValue("s2")) }
        h.servers.on("home.test", "GET", "/api/v1/works/show") { json(WorkDetail.serializer(), show()) }
        h.servers.on("home.test", "GET", "/api/v1/works/show/children") { json(WorkSummaryList.serializer(), WorkSummaryList(listOf(season1, season2))) }
        h.servers.on("home.test", "GET", "/api/v1/works/s2/children") { json(WorkSummaryList.serializer(), WorkSummaryList(listOf(episode("e3", 2, "s2")))) }
        h.servers.on("home.test", "GET", "/api/v1/lists") {
            json(WorkListList.serializer(), WorkListList(listOf(WorkList("my", "Watchlist", ListKind.Manual, builtIn = true, workCount = 0))))
        }
        h.device.signedInTo("server-1")
        val session = h.core.restore(home)

        val page = session.library.workPage("s2")
        val state = page.until { it.work != null && it.episodeIds() != null && it.myListId != null }
        page.close()

        assertEquals("show", state.work?.id)
        assertEquals("s2", state.season?.id)
        assertEquals(listOf("e3"), state.episodeIds())
        assertEquals("my", state.myListId)
    }

    @Test
    fun aPageOpenedFromItsCardSaysTheServerDoesntAnswer() = runTest {
        val h = Harness(this)
        h.servers.server("home.test", info("server-1", "Basement"))
        h.servers.on("home.test", "GET", "/api/v1/works/m1") { throw IOException("no route to host") }
        h.device.signedInTo("server-1")
        val session = h.core.restore(home)

        val page = session.library.workPage("m1", card = card("m1", WorkKind.Movie, "Sintel"))
        val state = page.until { it.problem != null }
        page.close()

        assertEquals("Sintel", state.head?.title)
        assertNull(state.work)
        assertEquals(RequestProblem.Unreachable, state.problem)
    }
}
