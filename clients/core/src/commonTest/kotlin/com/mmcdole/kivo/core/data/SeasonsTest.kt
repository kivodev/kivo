package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.EpisodeInfo
import com.mmcdole.kivo.api.models.MatchState
import com.mmcdole.kivo.api.models.SeasonInfo
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkSummary
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class SeasonsTest {
    private fun season(id: String, number: Int) = WorkSummary(
        id = id, kind = WorkKind.Season, title = if (number == 0) "Specials" else "Season $number", matchState = MatchState.Matched,
        available = true, images = WorkImages(), season = SeasonInfo(number, "sg", "Steins;Gate", 0),
    )

    private fun episode(season: Int, seasonId: String?) = EpisodeInfo(season, 11, "sg", "Steins;Gate", seasonId = seasonId)

    // As the server lists Steins;Gate's: Season 1, then Specials.
    private val seasons = listOf(season("s1", 1), season("s0", 0))

    @Test
    fun goToSeriesOpensTheEpisodesSeasonBeforeTheSeriesIsRead() {
        assertEquals("s1", Seasons.openSeason(seasons, SeasonRef("s1", 1), null, seriesRead = false)?.id)
        assertEquals("s0", Seasons.openSeason(seasons, SeasonRef("s0", 0), null, seriesRead = false)?.id)
    }

    @Test
    fun anAskedSeasonIsFoundByNumberWhenItsIdIsMissingOrUnlisted() {
        assertEquals("s1", Seasons.openSeason(seasons, SeasonRef(null, 1), null, seriesRead = false)?.id)
        assertEquals("s0", Seasons.openSeason(seasons, SeasonRef("gone", 0), null, seriesRead = false)?.id)
    }

    @Test
    fun aSeasonAskedForButNotListedOpensTheFirstRegularSeasonAtOnce() {
        assertEquals("s1", Seasons.openSeason(seasons, SeasonRef("gone", 7), null, seriesRead = false)?.id)
        assertEquals("s1", Seasons.openSeason(listOf(season("s0", 0), season("s1", 1)), SeasonRef("gone", 4), null, seriesRead = false)?.id)
        assertEquals("s0", Seasons.openSeason(listOf(season("s0", 0)), SeasonRef("gone", 3), null, seriesRead = false)?.id, "Specials alone")
    }

    @Test
    fun aSeasonsCardOpensThatSeasonWhereverNextUpIs() {
        assertEquals("s0", Seasons.openSeason(seasons, SeasonRef("s0", 0), episode(1, "s1"), seriesRead = true)?.id)
    }

    @Test
    fun withNoneAskedItWaitsForTheSeriesThenOpensNextUpsSeason() {
        val nextUp = episode(0, "s0")
        assertNull(Seasons.openSeason(seasons, null, nextUp, seriesRead = false))
        assertEquals("s0", Seasons.openSeason(seasons, null, nextUp, seriesRead = true)?.id)
        assertEquals("s0", Seasons.openSeason(seasons, null, episode(0, null), seriesRead = true)?.id)
    }

    @Test
    fun withoutNextUpOrAReadSeriesItOpensTheFirstRegularSeason() {
        assertEquals("s1", Seasons.openSeason(listOf(season("s0", 0), season("s1", 1)), null, null, seriesRead = true)?.id)
        assertEquals("s1", Seasons.openSeason(seasons, null, episode(7, "gone"), seriesRead = true)?.id, "Next Up's season unlisted")
    }

    @Test
    fun noSeasonsOpenNone() {
        assertNull(Seasons.openSeason(emptyList(), SeasonRef("s1", 1), null, seriesRead = true))
    }
}
