package com.mmcdole.kivo.android.mobile.ui

import com.mmcdole.kivo.android.mobile.work
import com.mmcdole.kivo.api.models.ListRef
import com.mmcdole.kivo.api.models.EpisodeInfo
import com.mmcdole.kivo.api.models.UserState
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse

class WorkActionsTest {
    private val started = work("m1").copy(durationMs = 100_000, userState = UserState(played = false, playCount = 0, positionMs = 40_000))

    @Test
    fun aStartedMovieInContinueWatchingOffersEverything() {
        assertEquals(
            listOf(WorkAction.Play, WorkAction.PlayFromStart, WorkAction.TogglePlayed, WorkAction.AddToList, WorkAction.RemoveFromContinue),
            workActions(WorkMenu(started, CONTINUE_WATCHING)),
        )
    }

    @Test
    fun anUnstartedWorkElsewhereHasNoStartOverOrRemove() {
        assertEquals(listOf(WorkAction.Play, WorkAction.TogglePlayed, WorkAction.AddToList), workActions(WorkMenu(work("m2"), "list")))
    }

    @Test
    fun aSeriesDoesntPlayAndAnEpisodeGoesToItsSeriesUnlessOnIt() {
        assertEquals(listOf(WorkAction.TogglePlayed, WorkAction.AddToList), workActions(WorkMenu(work("s", kind = "series"))))
        val episode = work("e1", kind = "episode").copy(episode = EpisodeInfo(1, 2, "show", "Show"))
        assertEquals(WorkAction.GoToSeries, workActions(WorkMenu(episode)).last())
        assertEquals(WorkAction.AddToList, workActions(WorkMenu(episode), seriesPage = "show").last())
    }

    @Test
    fun progressIsTheShareWatchedUntilPlayed() {
        assertEquals(0.4f, progressOf(started))
        assertEquals(null, progressOf(started.copy(userState = started.userState!!.copy(played = true))))
    }

    @Test
    fun artworkIsAskedForAtAStoredWidthCoveringTheCard() {
        assertEquals(320, artworkWidth(300))
        assertEquals(1920, artworkWidth(4000))
    }

    @Test
    fun aCardInAListOffersRemovingItFromTheList() {
        val inList = workActions(WorkMenu(work("m"), list = ListRef("l1", "Date Night")))
        assertEquals(WorkAction.RemoveFromList, inList[inList.indexOf(WorkAction.AddToList) + 1])
        assertFalse(WorkAction.RemoveFromList in workActions(WorkMenu(work("m"))))
    }
}
