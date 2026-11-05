package com.mmcdole.kivo.android.player

import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.api.models.EpisodeInfo
import com.mmcdole.kivo.core.play.PlayerEnd
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

// Where the player screen goes when the core's player is done: the core
// decides the end, and the screen only takes it there.

class PlayerExitTest {
    private val episode = EpisodeInfo(seasonNumber = 1, number = 2, seriesId = "series", seriesTitle = "Series", nextId = "episode-3")

    @Test
    fun theNextEpisodeReplacesThePlayer() {
        assertEquals(PlayerExit.Replace(Screen.Player("episode-3")), exitAt(PlayerEnd.PlayNext, episode))
        assertNull(exitAt(PlayerEnd.PlayNext, episode.copy(nextId = null)), "with none known, the player stays")
    }

    @Test
    fun theLastEpisodeGivesWayToItsSeriesAndAnythingElseGoesBack() {
        assertEquals(PlayerExit.Series("series"), exitAt(PlayerEnd.Series, episode.copy(nextId = null)))
        assertEquals(PlayerExit.Back, exitAt(PlayerEnd.Close, null))
    }

    @Test
    fun playingOnGoesNowhere() {
        assertNull(exitAt(null, episode))
    }
}
