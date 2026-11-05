package com.mmcdole.kivo.core.player

import com.mmcdole.kivo.api.models.Chapter
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class UpNextTest {
    private val minute = 60_000L

    @Test
    fun appearsAFixedTimeBeforeTheEndByRuntime() {
        assertNull(upNextAtMs(9 * minute + 59_000))
        assertEquals(10 * minute - 30_000, upNextAtMs(10 * minute))
        assertEquals(24 * minute - 30_000, upNextAtMs(24 * minute))
        assertEquals(40 * minute - 35_000, upNextAtMs(40 * minute))
        assertEquals(50 * minute - 40_000, upNextAtMs(50 * minute))
        assertEquals(62 * minute - 40_000, upNextAtMs(62 * minute))
    }

    @Test
    fun ignoresChaptersThatPutTheEndingAtTheStart() {
        // Serial Experiments Lain's ordered chapters, each starting at 0:00,
        // read as a file's own: the server marks none of them, and the
        // card's time doesn't read chapters at all.
        val duration = 1_380_000L
        val lain = listOf(
            Chapter(0, 119_620, "Opening"),
            Chapter(0, duration.toInt(), "Episode"),
            Chapter(0, 98_599, "Ending"),
        )
        assertNull(SkipShow().at(markersOf(null), 0, controlsShowing = true, upNextShowing = false))
        var upNext = UpNext()
        for (position in listOf(0L, 1_000L, 98_000L, duration - 31_000)) {
            upNext = upNext.at(position, duration, appearsAs = UpNextPhase.CountingDown)
            assertEquals(UpNextPhase.Hidden, upNext.phase, "at $position")
        }
        upNext = upNext.at(duration - 30_000, duration, appearsAs = UpNextPhase.CountingDown)
        assertEquals(UpNextPhase.CountingDown, upNext.phase)
    }

    @Test
    fun appearsAtCreditsThatRunToTheEnd() {
        val duration = 24 * minute
        val credits = SkipMarker(MarkerKind.Credits, duration - 90_000, duration - 5_000, final = true)
        assertEquals(duration - 90_000, upNextAtMs(duration, credits.startMs))
        assertEquals(duration - 30_000, upNextAtMs(duration, duration - 15_000), "credits shorter than the lead don't delay it")
        var upNext = UpNext().at(duration - 91_000, duration, appearsAs = UpNextPhase.CountingDown, credits)
        assertEquals(UpNextPhase.Hidden, upNext.phase)
        upNext = upNext.at(duration - 90_000, duration, appearsAs = UpNextPhase.CountingDown, credits)
        assertEquals(UpNextPhase.CountingDown, upNext.phase)
        assertTrue(upNext.atCredits, "its second button is Watch credits")
        assertFalse(UpNext().at(duration - 30_000, duration, appearsAs = UpNextPhase.CountingDown).atCredits)
        // Watch credits: the card goes, and the credits play on.
        upNext = upNext.hide()
        assertEquals(UpNextPhase.Dismissed, upNext.at(duration - 60_000, duration, appearsAs = UpNextPhase.CountingDown, credits).phase)
    }

    @Test
    fun countsTheSecondsLeftAndNeverPlaysNextBeforeTheEnd() {
        val duration = 24 * minute
        var upNext = UpNext().at(duration - 30_000, duration, appearsAs = UpNextPhase.CountingDown)
        assertEquals(30, upNext.secondsLeft)
        assertEquals(upNext, upNext.tick(running = true), "a second passing doesn't count it; the position does")
        upNext = upNext.at(duration - 29_000, duration, appearsAs = UpNextPhase.CountingDown)
        assertEquals(29, upNext.secondsLeft)
        upNext = upNext.at(duration - 400, duration, appearsAs = UpNextPhase.CountingDown)
        assertEquals(1, upNext.secondsLeft)
        assertFalse(upNext.playsNext)
        upNext = upNext.at(duration, duration, appearsAs = UpNextPhase.CountingDown)
        assertEquals(0, upNext.secondsLeft)
        assertFalse(upNext.playsNext, "the episode's end plays the next one")
    }

    @Test
    fun finalCreditsShorterThanTheLeadCountTheSecondsLeftWithHide() {
        val duration = 24 * minute
        val credits = SkipMarker(MarkerKind.Credits, duration - 15_000, duration, final = true)
        var upNext = UpNext().at(duration - 30_000, duration, appearsAs = UpNextPhase.CountingDown, credits)
        assertEquals(UpNext(UpNextPhase.CountingDown, 30, atCredits = false), upNext)
        assertEquals(upNext, upNext.tick(running = true))
        upNext = upNext.at(duration - 15_000, duration, appearsAs = UpNextPhase.CountingDown, credits)
        assertEquals(UpNext(UpNextPhase.CountingDown, 15, atCredits = false), upNext, "reaching the credits leaves it as it is")
        assertFalse(upNext.at(duration, duration, appearsAs = UpNextPhase.CountingDown, credits).playsNext)
        assertFalse(UpNext().at(duration - 30_000, duration, appearsAs = UpNextPhase.Waiting, credits).atCredits)
    }

    @Test
    fun countsDownTenSecondsAtTheCreditsOnlyWhilePlayingThenPlaysNext() {
        val duration = 24 * minute
        val credits = SkipMarker(MarkerKind.Credits, duration - 90_000, duration, final = true)
        var upNext = UpNext().at(duration - 90_000, duration, appearsAs = UpNextPhase.CountingDown, credits)
        assertEquals(UP_NEXT_COUNTDOWN_SECONDS, upNext.secondsLeft)
        assertEquals(upNext, upNext.at(duration - 80_000, duration, appearsAs = UpNextPhase.CountingDown, credits))
        upNext = upNext.tick(running = false)
        assertEquals(10, upNext.secondsLeft)
        repeat(9) { upNext = upNext.tick(running = true) }
        assertFalse(upNext.playsNext)
        upNext = upNext.tick(running = true)
        assertTrue(upNext.playsNext)
        assertEquals(upNext, upNext.tick(running = true))
    }

    @Test
    fun aSeekPastItsTimeShowsItAndOneBackHidesAndRearmsIt() {
        val duration = 45 * minute
        var upNext = UpNext().at(10 * minute, duration, appearsAs = UpNextPhase.CountingDown)
        assertFalse(upNext.shows)
        upNext = upNext.at(duration - 25_000, duration, appearsAs = UpNextPhase.CountingDown)
        assertTrue(upNext.shows)
        assertEquals(25, upNext.secondsLeft)
        upNext = upNext.at(duration - 36_000, duration, appearsAs = UpNextPhase.CountingDown)
        assertEquals(UpNext(), upNext)
        upNext = upNext.at(duration - 35_000, duration, appearsAs = UpNextPhase.CountingDown)
        assertEquals(UpNext(UpNextPhase.CountingDown, 35), upNext)
    }

    @Test
    fun aSeekToTheLastMomentsDoesntBringIt() {
        val duration = 24 * minute
        var upNext = UpNext().at(duration - 19_000, duration, appearsAs = UpNextPhase.CountingDown)
        assertFalse(upNext.shows)
        // Once showing, it stays to the end.
        upNext = UpNext().at(duration - 30_000, duration, appearsAs = UpNextPhase.CountingDown).at(duration - 1_000, duration, appearsAs = UpNextPhase.CountingDown)
        assertTrue(upNext.shows)
    }

    @Test
    fun anEpisodeUnderTenMinutesHasNoCard() {
        val duration = 8 * minute
        assertFalse(UpNext().at(duration - 25_000, duration, appearsAs = UpNextPhase.CountingDown).shows)
    }

    @Test
    fun hideKeepsItAwayForTheEpisodeButNotThroughASeekBack() {
        val duration = 24 * minute
        var upNext = UpNext().at(duration - 30_000, duration, appearsAs = UpNextPhase.CountingDown).hide()
        assertEquals(UpNextPhase.Dismissed, upNext.phase)
        upNext = upNext.at(duration - 25_000, duration, appearsAs = UpNextPhase.CountingDown).tick(running = true)
        assertEquals(UpNextPhase.Dismissed, upNext.phase)
        assertFalse(upNext.playsNext)
        // Hidden, the episode still plays on at its end.
        assertEquals(PlayEnd.PlayNext, playEnd(episode = true, hasNext = true, autoplayAllowed = true))
        upNext = upNext.at(duration - 60_000, duration, appearsAs = UpNextPhase.CountingDown)
            .at(duration - 30_000, duration, appearsAs = UpNextPhase.CountingDown)
        assertEquals(UpNextPhase.CountingDown, upNext.phase)
    }

    @Test
    fun afterThreeEpisodesWithoutInputItAsksAndNothingPlaysOn() {
        val duration = 24 * minute
        val upNext = UpNext().at(duration - 30_000, duration, appearsAs = UpNextPhase.StillWatching)
        assertEquals(UpNextPhase.StillWatching, upNext.phase)
        repeat(20) { assertFalse(upNext.tick(running = true).playsNext) }
        assertEquals(PlayEnd.Wait, playEnd(episode = true, hasNext = true, autoplayAllowed = false))
        // After the end, the card stays until a seek back before its time.
        val ended = upNext.hide().waits(UpNextPhase.StillWatching)
        assertEquals(ended, ended.at(duration, duration, appearsAs = UpNextPhase.StillWatching))
        assertEquals(UpNext(), ended.at(duration - 60_000, duration, appearsAs = UpNextPhase.StillWatching))
        // An episode too short for the card still asks at its end.
        val short = UpNext().waits(UpNextPhase.StillWatching)
        assertEquals(short, short.at(5 * minute - 200, 5 * minute, appearsAs = UpNextPhase.StillWatching))
    }

    @Test
    fun withAutoplayOffItShowsWithoutCountingDownAndNothingPlaysOn() {
        val duration = 24 * minute
        val upNext = UpNext().at(duration - 30_000, duration, appearsAs = UpNextPhase.Waiting)
        assertEquals(UpNextPhase.Waiting, upNext.phase)
        assertTrue(upNext.shows)
        repeat(20) { assertEquals(upNext, upNext.tick(running = true)) }
        assertFalse(upNext.playsNext)
        assertEquals(PlayEnd.Wait, playEnd(episode = true, hasNext = true, autoplayAllowed = false))
        val ended = upNext.hide().waits(UpNextPhase.Waiting)
        assertTrue(ended.shows)
        assertEquals(ended, ended.at(duration, duration, appearsAs = UpNextPhase.Waiting))
    }

    @Test
    fun theLastEpisodeEndsOnItsSeriesAndAnythingElseCloses() {
        assertEquals(PlayEnd.Series, playEnd(episode = true, hasNext = false, autoplayAllowed = true))
        assertEquals(PlayEnd.Close, playEnd(episode = false, hasNext = false, autoplayAllowed = true))
    }

    @Test
    fun autoplayStopsAfterThreeEpisodesWithoutInput() {
        val autoplay = Autoplay()
        repeat(3) {
            assertTrue(autoplay.allowed)
            autoplay.playedOnItsOwn()
        }
        assertFalse(autoplay.allowed)
        autoplay.reset()
        assertTrue(autoplay.allowed)
        autoplay.playedOnItsOwn()
        autoplay.reset()
        autoplay.playedOnItsOwn()
        autoplay.playedOnItsOwn()
        assertTrue(autoplay.allowed)
    }

    @Test
    fun autoplayOffWaitsWhateverTheRun() {
        val autoplay = Autoplay()
        assertEquals(UpNextPhase.CountingDown, autoplay.phase)
        autoplay.enabled = false
        assertEquals(UpNextPhase.Waiting, autoplay.phase)
        assertFalse(autoplay.allowed)
        repeat(3) { autoplay.playedOnItsOwn() }
        assertEquals(UpNextPhase.Waiting, autoplay.phase)
        autoplay.enabled = true
        assertEquals(UpNextPhase.StillWatching, autoplay.phase)
        autoplay.reset()
        assertTrue(autoplay.allowed)
    }
}
