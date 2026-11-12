package com.mmcdole.kivo.core.play

import com.mmcdole.kivo.api.models.Marker
import com.mmcdole.kivo.api.models.PlayDelivery
import com.mmcdole.kivo.api.models.PlayPart
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.core.player.Autoplay
import com.mmcdole.kivo.core.player.MarkerKind
import com.mmcdole.kivo.core.player.UpNextPhase
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlin.coroutines.EmptyCoroutineContext
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlin.time.Duration
import kotlin.time.Duration.Companion.seconds

// The player's timed rules on the test's virtual clock: the controls shown,
// Skip Intro and Credits, up next's countdown, autoplay's run and the play's
// end.

/** 44 minutes, with an intro from 1:00 to 2:30 and credits over the last 90 seconds; up next at 43:25. */
private const val DURATION_MS = 2_640_000
private const val UP_NEXT_MS = DURATION_MS - 35_000L

/** Where final credits start, in [creditsToTheEnd]. */
private const val CREDITS_MS = 2_550_000L

private val episode = PlayStarted(
    id = "p1",
    workId = "w1",
    versionId = "v1",
    startMs = 0,
    durationMs = DURATION_MS,
    local = true,
    delivery = PlayDelivery.File,
    url = "plays/p1/media",
    tracks = emptyList(),
    parts = listOf(PlayPart(DURATION_MS)),
    chapters = emptyList(),
    markers = listOf(
        Marker("intro", 60_000, 150_000, "chapter"),
        Marker("credits", 2_550_000, DURATION_MS, "chapter"),
    ),
)

/** The episode with its credits running to the end, and a recap before its intro. */
private val creditsToTheEnd = episode.copy(
    markers = listOf(
        Marker("recap", 0, 45_000, "chapter"),
        Marker("intro", 60_000, 150_000, "chapter"),
        Marker("credits", 2_550_000, DURATION_MS, "chapter", final = true),
    ),
)

/** The rules over a play the test sets; their controls hide by themselves only when [hideAfter] says. */
private class UiHarness(
    private val test: TestScope,
    run: Int = 0,
    private val play: PlayStarted = episode,
    hideAfter: Duration = Duration.INFINITE,
) {
    val state = MutableStateFlow(PlayerState())
    val seeks = mutableListOf<Long>()
    val autoplay = Autoplay().apply { repeat(run) { playedOnItsOwn() } }
    var ui = PlayerUi()
        private set
    val rules = PlayerUiRules(state, { seeks += it }, autoplay, test.backgroundScope, EmptyCoroutineContext, hideAfter) { ui = it }
    val end get() = rules.end.value

    init {
        test.runCurrent()
    }

    /** The video playing at [positionMs], or paused there; or, with [status], loading or the like. */
    fun at(positionMs: Long, paused: Boolean = false, ended: Boolean = false, status: PlayerStatus = PlayerStatus.Playing) {
        state.value = PlayerState(status, play, paused = paused, positionMs = positionMs, ended = ended)
        test.runCurrent()
    }

    fun advance(ms: Long) {
        test.advanceTimeBy(ms)
        test.runCurrent()
    }
}

class PlayerUiTest {
    @Test
    fun theControlsShowUntilTheyHide() = runTest {
        val h = UiHarness(this)
        assertTrue(h.ui.controlsShown, "the controls show as the player opens")
        h.at(600_000)
        h.advance(60_000)
        assertTrue(h.ui.controlsShown, "with no wait, only hiding them hides them")
        h.rules.hideControls()
        assertFalse(h.ui.controlsShown)

        h.rules.input()
        assertFalse(h.ui.controlsShown, "input alone doesn't show them")
        h.rules.showControls()
        assertTrue(h.ui.controlsShown)
    }

    @Test
    fun pausedTheControlsShow() = runTest {
        val h = UiHarness(this)
        h.at(600_000)
        h.rules.hideControls()
        h.at(600_000, paused = true)
        assertTrue(h.ui.controlsShown, "a pause shows them")
    }

    @Test
    fun aMenuOrAHeldScrubShowsTheControls() = runTest {
        val h = UiHarness(this)
        h.at(600_000)
        h.rules.hideControls()
        h.rules.menu(open = true)
        assertTrue(h.ui.controlsShown)
        h.rules.menu(open = false)
        h.rules.hideControls()
        h.rules.scrubbing(held = true)
        assertTrue(h.ui.controlsShown)
    }

    @Test
    fun aTapHidesTheControlsOnlyWhenTheyWouldHideOnTheirOwn() = runTest {
        val h = UiHarness(this)
        h.at(600_000)
        h.rules.toggleControls()
        assertFalse(h.ui.controlsShown)
        h.rules.toggleControls()
        assertTrue(h.ui.controlsShown)
        h.at(600_000, paused = true)
        h.rules.toggleControls()
        assertTrue(h.ui.controlsShown, "paused, a tap keeps them")
        h.rules.hideControls()
        assertFalse(h.ui.controlsShown, "Back hides them, paused or not")
    }

    @Test
    fun skipShowsForTenSecondsOfWallClockAfterPlaybackEntersAMarker() = runTest {
        val h = UiHarness(this)
        h.at(30_000)
        h.rules.hideControls()
        assertNull(h.ui.skipMarker)
        h.at(60_000)
        assertEquals(MarkerKind.Intro, h.ui.skipMarker?.kind)
        h.advance(5_000)
        h.at(65_000)
        h.advance(4_999)
        assertEquals(MarkerKind.Intro, h.ui.skipMarker?.kind)
        h.advance(1)
        assertNull(h.ui.skipMarker)
    }

    @Test
    fun skipHidesOnTimeWhilePaused() = runTest {
        val h = UiHarness(this)
        h.at(100_000)
        h.rules.hideControls()
        h.at(100_000, paused = true)
        h.rules.hideControls()
        assertEquals(MarkerKind.Intro, h.ui.skipMarker?.kind)
        // The position doesn't move, but the clock does.
        h.advance(10_000)
        assertNull(h.ui.skipMarker)
    }

    @Test
    fun skipComesBackWithTheControlsInsideTheMarkerAndPausedShowsWhenTheyDo() = runTest {
        val h = UiHarness(this)
        h.at(60_000)
        h.advance(10_000)
        h.rules.hideControls()
        assertNull(h.ui.skipMarker)
        h.rules.showControls()
        assertEquals(MarkerKind.Intro, h.ui.skipMarker?.kind)
        h.rules.hideControls()
        assertNull(h.ui.skipMarker, "it hides with them")
        h.at(80_000, paused = true)
        assertTrue(h.ui.controlsShown)
        assertEquals(MarkerKind.Intro, h.ui.skipMarker?.kind, "paused, it shows with them")
        h.rules.hideControls()
        assertNull(h.ui.skipMarker)
    }

    @Test
    fun skipIsGoneAtTheMarkersEnd() = runTest {
        val h = UiHarness(this)
        h.at(140_000)
        assertEquals(MarkerKind.Intro, h.ui.skipMarker?.kind)
        h.rules.showControls()
        h.at(150_000)
        assertNull(h.ui.skipMarker)
    }

    @Test
    fun skipPressedSeeksPastTheMarkerAndStaysHiddenUntilASeekBackIntoIt() = runTest {
        val h = UiHarness(this)
        h.at(70_000)
        h.rules.skipMarker()
        assertEquals(listOf(150_000L), h.seeks)
        assertNull(h.ui.skipMarker)
        h.rules.showControls()
        // Until the seek lands, the position is still inside the marker.
        h.at(71_000)
        assertNull(h.ui.skipMarker)
        h.at(150_000)
        h.rules.hideControls()
        h.at(140_000)
        assertEquals(MarkerKind.Intro, h.ui.skipMarker?.kind, "a seek back shows it again")
        h.advance(10_000)
        assertNull(h.ui.skipMarker, "for 10 seconds")
    }

    @Test
    fun upNextCountsTheTimeLeftAndTheNextEpisodePlaysAtTheEnd() = runTest {
        val h = UiHarness(this)
        h.at(UP_NEXT_MS - 1_000)
        h.rules.episode(hasNext = true)
        assertFalse(h.ui.upNext.shows)
        h.at(UP_NEXT_MS)
        assertEquals(UpNextPhase.CountingDown, h.ui.upNext.phase)
        assertEquals(35, h.ui.upNext.secondsLeft)
        h.at(UP_NEXT_MS + 10_000, paused = true)
        h.advance(60_000)
        assertEquals(25, h.ui.upNext.secondsLeft, "a pause holds it")
        assertNull(h.end)
        h.at(DURATION_MS - 500L)
        assertEquals(1, h.ui.upNext.secondsLeft)
        h.advance(10_000)
        assertNull(h.end, "nothing plays before the end")
        h.at(DURATION_MS.toLong(), paused = true, ended = true)
        h.rules.ended()
        assertEquals(PlayerEnd.PlayNext, h.end)
        assertEquals(1, h.autoplay.count, "it played on by itself")
    }

    @Test
    fun finalCreditsShorterThanTheLeadPlayToTheEnd() = runTest {
        val shortCredits = episode.copy(markers = listOf(Marker("credits", DURATION_MS - 15_000, DURATION_MS, "chapter", final = true)))
        val h = UiHarness(this, play = shortCredits)
        h.rules.episode(hasNext = true)
        h.at(UP_NEXT_MS)
        assertEquals(UpNextPhase.CountingDown, h.ui.upNext.phase)
        assertFalse(h.ui.upNext.atCredits, "its second button is Hide")
        assertEquals(35, h.ui.upNext.secondsLeft)
        h.advance(15_000)
        assertNull(h.end, "no 10-second countdown")
        h.at(DURATION_MS - 10_000L)
        assertEquals(10, h.ui.upNext.secondsLeft)
        h.advance(15_000)
        assertNull(h.end)
        h.at(DURATION_MS.toLong(), paused = true, ended = true)
        h.rules.ended()
        assertEquals(PlayerEnd.PlayNext, h.end)
    }

    @Test
    fun upNextAtTheCreditsCountsDownWhilePlayingWithNoMenuOpenThenPlaysTheNextEpisode() = runTest {
        val h = UiHarness(this, play = creditsToTheEnd)
        h.rules.episode(hasNext = true)
        h.at(CREDITS_MS)
        assertEquals(UpNextPhase.CountingDown, h.ui.upNext.phase)
        assertEquals(10, h.ui.upNext.secondsLeft)
        h.advance(1_000)
        assertEquals(9, h.ui.upNext.secondsLeft)
        h.rules.menu(open = true)
        h.advance(3_000)
        assertEquals(9, h.ui.upNext.secondsLeft, "a menu holds the countdown")
        h.rules.menu(open = false)
        h.at(CREDITS_MS + 4_000, paused = true)
        h.advance(3_000)
        assertEquals(9, h.ui.upNext.secondsLeft, "so does a pause")
        h.at(CREDITS_MS + 4_000)
        h.advance(8_000)
        assertEquals(1, h.ui.upNext.secondsLeft)
        assertNull(h.end)
        h.advance(1_000)
        assertEquals(PlayerEnd.PlayNext, h.end)
        assertEquals(1, h.autoplay.count, "it played on by itself")
        h.rules.ended()
        assertEquals(1, h.autoplay.count, "the end doesn't count it twice")
    }

    @Test
    fun hideKeepsTheCardAwayAndTheEpisodeStillPlaysOnAtItsEnd() = runTest {
        val h = UiHarness(this)
        h.rules.episode(hasNext = true)
        h.at(UP_NEXT_MS)
        h.rules.hideUpNext()
        assertFalse(h.ui.upNext.shows)
        h.advance(20_000)
        assertNull(h.end)
        h.at(DURATION_MS.toLong(), paused = true, ended = true)
        h.rules.ended()
        assertEquals(PlayerEnd.PlayNext, h.end)
    }

    @Test
    fun aSeekBackBeforeItsTimeHidesTheCardAndItCountsFromTenAgain() = runTest {
        val h = UiHarness(this, play = creditsToTheEnd)
        h.rules.episode(hasNext = true)
        h.at(CREDITS_MS)
        h.advance(4_000)
        assertEquals(6, h.ui.upNext.secondsLeft)
        h.at(CREDITS_MS - 60_000)
        assertFalse(h.ui.upNext.shows)
        h.at(CREDITS_MS)
        assertEquals(10, h.ui.upNext.secondsLeft)
    }

    @Test
    fun skipCreditsGivesWayToUpNext() = runTest {
        val h = UiHarness(this)
        h.rules.episode(hasNext = true)
        h.at(2_560_000)
        assertEquals(MarkerKind.Credits, h.ui.skipMarker?.kind)
        h.at(UP_NEXT_MS)
        assertTrue(h.ui.upNext.shows)
        assertNull(h.ui.skipMarker)
    }

    @Test
    fun upNextAppearsAtCreditsThatRunToTheEndAndWatchCreditsPlaysThemOn() = runTest {
        val h = UiHarness(this, play = creditsToTheEnd)
        h.rules.episode(hasNext = true)
        h.at(1_000)
        assertEquals(MarkerKind.Recap, h.ui.skipMarker?.kind)
        h.at(2_549_000)
        assertFalse(h.ui.upNext.shows)
        h.at(2_550_000)
        assertEquals(UpNextPhase.CountingDown, h.ui.upNext.phase)
        assertTrue(h.ui.upNext.atCredits)
        assertNull(h.ui.skipMarker, "Skip Credits gives way to it")
        h.rules.hideUpNext()
        h.advance(15_000)
        assertFalse(h.ui.upNext.shows)
        assertNull(h.end, "the credits play on")
    }

    @Test
    fun afterThreeEpisodesWithoutInputUpNextAsksAndNothingPlaysOn() = runTest {
        val h = UiHarness(this, run = 3)
        h.rules.episode(hasNext = true)
        h.at(UP_NEXT_MS)
        assertEquals(UpNextPhase.StillWatching, h.ui.upNext.phase)
        h.advance(30_000)
        assertNull(h.end, "no countdown")
        h.at(DURATION_MS.toLong(), paused = true, ended = true)
        h.rules.ended()
        assertNull(h.end, "nothing plays on")
        assertEquals(UpNextPhase.StillWatching, h.ui.upNext.phase)
        h.rules.playNext()
        assertEquals(PlayerEnd.PlayNext, h.end)
        assertEquals(0, h.autoplay.count, "choosing it starts the run again")
    }

    @Test
    fun withAutoplayOffUpNextShowsWithoutCountingDownAndTheEpisodeStopsAtItsEnd() = runTest {
        val h = UiHarness(this)
        h.autoplay.enabled = false
        h.rules.episode(hasNext = true)
        h.at(UP_NEXT_MS)
        assertEquals(UpNextPhase.Waiting, h.ui.upNext.phase)
        assertTrue(h.ui.upNext.shows)
        h.advance(30_000)
        assertNull(h.end, "no countdown")
        h.rules.hideUpNext()
        h.at(DURATION_MS.toLong(), paused = true, ended = true)
        h.rules.ended()
        assertNull(h.end, "nothing plays on")
        assertEquals(UpNextPhase.Waiting, h.ui.upNext.phase, "the card waits at the end")
        assertEquals(0, h.autoplay.count)
        h.rules.playNext()
        assertEquals(PlayerEnd.PlayNext, h.end)
    }

    @Test
    fun theRunCountsEpisodesPlayedOnByThemselvesAndAnyInputStartsItAgain() = runTest {
        val h = UiHarness(this, run = 2)
        h.rules.episode(hasNext = true)
        h.at(DURATION_MS.toLong(), paused = true, ended = true)
        h.rules.ended()
        assertEquals(PlayerEnd.PlayNext, h.end)
        assertEquals(3, h.autoplay.count)
        assertFalse(h.autoplay.allowed)
        h.rules.input()
        assertEquals(0, h.autoplay.count)
    }

    @Test
    fun theLastEpisodeEndsOnItsSeriesAndAnythingElseCloses() = runTest {
        val last = UiHarness(this)
        last.rules.episode(hasNext = false)
        last.at(UP_NEXT_MS)
        assertFalse(last.ui.upNext.shows, "the last episode has no card")
        last.rules.playNext()
        assertNull(last.end, "nor a next one to choose")
        last.at(DURATION_MS.toLong(), paused = true, ended = true)
        last.rules.ended()
        assertEquals(PlayerEnd.Series, last.end)

        val movie = UiHarness(this)
        movie.at(DURATION_MS.toLong())
        movie.advance(3_000)
        movie.at(DURATION_MS.toLong(), paused = true, ended = true)
        movie.rules.hideControls()
        movie.rules.ended()
        assertEquals(PlayerEnd.Close, movie.end)
        assertTrue(movie.ui.controlsShown)
        movie.at(0)
        assertNull(movie.end, "playing again from the start")
    }

    @Test
    fun theControlsHideAWhileAfterTheLastInputWhilePlaying() = runTest {
        val h = UiHarness(this, hideAfter = 3.seconds)
        h.at(600_000, status = PlayerStatus.Loading)
        h.advance(10_000)
        assertTrue(h.ui.controlsShown, "loading, they stay")
        h.at(600_000)
        h.advance(2_999)
        assertTrue(h.ui.controlsShown)
        h.advance(1)
        assertFalse(h.ui.controlsShown, "3 seconds after they showed, playing")

        h.rules.showControls()
        h.advance(2_000)
        h.rules.input()
        h.advance(2_000)
        assertTrue(h.ui.controlsShown, "each input starts the wait again")
        h.advance(1_000)
        assertFalse(h.ui.controlsShown)

        h.rules.toggleControls()
        h.advance(2_000)
        h.at(602_000)
        h.advance(1_000)
        assertFalse(h.ui.controlsShown, "the position moving on doesn't start it again")
    }

    @Test
    fun theControlsStayWhilePausedOrWithAMenuOpenOrAScrubHeld() = runTest {
        val h = UiHarness(this, hideAfter = 5.seconds)
        h.at(600_000, paused = true)
        h.advance(60_000)
        assertTrue(h.ui.controlsShown, "paused")
        h.at(600_000)
        h.advance(4_000)
        h.at(604_000, paused = true)
        h.advance(60_000)
        h.at(604_000)
        h.advance(4_999)
        assertTrue(h.ui.controlsShown, "a pause stops the wait, and playing again starts it over")
        h.advance(1)
        assertFalse(h.ui.controlsShown)

        h.rules.menu(open = true)
        h.advance(60_000)
        assertTrue(h.ui.controlsShown, "a menu open")
        h.rules.menu(open = false)
        h.advance(4_999)
        assertTrue(h.ui.controlsShown, "closing it starts the wait over")
        h.advance(1)
        assertFalse(h.ui.controlsShown)

        h.rules.scrubbing(held = true)
        h.advance(60_000)
        assertTrue(h.ui.controlsShown, "a scrub held")
        h.rules.scrubbing(held = false)
        h.advance(5_000)
        assertFalse(h.ui.controlsShown)
    }

    @Test
    fun theWaitsStopWithThePlayer() = runTest {
        val h = UiHarness(this)
        h.at(60_000)
        h.rules.hideControls()
        assertEquals(MarkerKind.Intro, h.ui.skipMarker?.kind)
        h.rules.dispose()
        h.advance(10_000)
        assertEquals(MarkerKind.Intro, h.ui.skipMarker?.kind, "Skip's wait stopped with the player")

        val hiding = UiHarness(this, hideAfter = 3.seconds)
        hiding.at(600_000)
        hiding.rules.dispose()
        hiding.advance(10_000)
        assertTrue(hiding.ui.controlsShown, "and so did the controls'")
    }
}
