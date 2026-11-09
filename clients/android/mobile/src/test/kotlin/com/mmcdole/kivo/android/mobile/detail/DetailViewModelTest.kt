package com.mmcdole.kivo.android.mobile.detail

import com.mmcdole.kivo.android.mobile.CoreTest
import com.mmcdole.kivo.android.mobile.awaitValue
import com.mmcdole.kivo.android.mobile.work
import com.mmcdole.kivo.api.models.EpisodeInfo
import com.mmcdole.kivo.api.models.SeasonInfo
import com.mmcdole.kivo.api.models.SeriesInfo
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkList
import com.mmcdole.kivo.api.models.WorkListList
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkSummaryList
import com.mmcdole.kivo.core.Load
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue
import kotlin.test.assertNull

class DetailViewModelTest : CoreTest() {
    private fun detail(id: String, kind: String, title: String = id, season: SeasonInfo? = null, series: SeriesInfo? = null, episode: EpisodeInfo? = null) = WorkDetail(
        id = id, kind = kind, title = title, matchState = "matched", available = true, images = WorkImages(),
        genres = emptyList(), studios = emptyList(), credits = emptyList(), externalIds = emptyList(), versions = emptyList(),
        extras = emptyList(), collections = emptyList(), season = season, series = series, episode = episode,
    )

    private val season1 = work("s1", "Season 1", "season").copy(season = SeasonInfo(1, "show", "Show", 0))
    private val season2 = work("s2", "Season 2", "season").copy(season = SeasonInfo(2, "show", "Show", 2))

    init {
        server.get("/api/v1/works/show", WorkDetail.serializer()) { detail("show", "series", "Show", series = SeriesInfo(2)) }
        server.get("/api/v1/works/s2", WorkDetail.serializer()) { detail("s2", "season", "Season 2", season = SeasonInfo(2, "show", "Show", 2)) }
        server.get("/api/v1/works/show/children", WorkSummaryList.serializer()) { WorkSummaryList(listOf(season1, season2)) }
        server.get("/api/v1/works/s1/children", WorkSummaryList.serializer()) { WorkSummaryList(listOf(work("e1", kind = "episode"))) }
        server.get("/api/v1/works/s2/children", WorkSummaryList.serializer()) { WorkSummaryList(listOf(work("e3", kind = "episode"), work("e4", kind = "episode"))) }
        server.get("/api/v1/works/m1", WorkDetail.serializer()) { detail("m1", "movie", "Charade") }
    }

    @Test
    fun aWorkOnMyListSaysSoOnceBothAreRead() {
        server.get("/api/v1/lists", WorkListList.serializer()) { WorkListList(listOf(WorkList("my", "My List", "manual", builtIn = true, workCount = 1))) }
        server.get("/api/v1/works/m1", WorkDetail.serializer()) { detail("m1", "movie", "Charade").copy(listIds = listOf("my")) }
        val model = DetailViewModel(signIn(), "m1")
        assertTrue(awaitValue(model.state) { it.work != null && it.myListId != null }.onMyList)
    }

    private fun episodes(state: DetailUiState) = (state.episodes as? Load.Loaded)?.value?.map { it.id }

    @Test
    fun aSeriesOpensOnItsFirstSeasonAndAnotherCanBeChosen() {
        val model = DetailViewModel(signIn(), "show")
        val state = awaitValue(model.state) { episodes(it) != null }
        assertEquals("Show", state.head?.title)
        assertEquals("s1", state.season?.id)
        assertEquals(listOf("e1"), episodes(state))
        model.chooseSeason("s2")
        assertEquals(listOf("e3", "e4"), episodes(awaitValue(model.state) { episodes(it)?.size == 2 }))
    }

    @Test
    fun aSeasonsPageIsItsSeriesOpenOnThatSeason() {
        val model = DetailViewModel(signIn(), "s2")
        val state = awaitValue(model.state) { it.work?.id == "show" && episodes(it) != null }
        assertEquals("s2", state.season?.id)
        assertEquals(listOf("e3", "e4"), episodes(state))
    }

    @Test
    fun goToSeriesOpensTheEpisodesSeason() {
        val model = DetailViewModel(signIn(), "show", seasonId = "s2", seasonNumber = 2)
        assertEquals("s2", awaitValue(model.state) { it.season != null }.season?.id)
    }

    @Test
    fun anotherVersionStartsItsTracksAtTheServersChoice() {
        val model = DetailViewModel(signIn(), "m1")
        awaitValue(model.state) { it.work != null }
        model.chooseAudio("a2")
        model.chooseSubtitle("t1")
        assertEquals("a2", awaitValue(model.state) { it.choices.subtitleTrackId == "t1" }.choices.audioTrackId)
        model.chooseVersion("v2")
        val choices = awaitValue(model.state) { it.choices.versionId == "v2" }.choices
        assertNull(choices.audioTrackId)
        assertNull(choices.subtitleTrackId)
    }
}
