package com.mmcdole.kivo.android.player

import android.view.KeyEvent
import androidx.compose.ui.test.assertIsFocused
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mmcdole.kivo.android.R
import com.mmcdole.kivo.android.ui.Screen
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.TestCoroutineScheduler
import org.junit.After
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

// The remote in the player, key by key, through the device's input: each
// press is owned from its first DOWN to its UP, and what it does follows
// the player's remote rules.

@RunWith(AndroidJUnit4::class)
class PlayerRemoteTest {
    /** The screen's clock, which the core's player waits on too. */
    private val clock = TestCoroutineScheduler()

    @get:Rule
    val compose = createComposeRule(clock)

    private val player by lazy { PlayerHarness(compose, clock) }

    @After
    fun close() = player.close()

    /** Lets the controls hide, as they do 3 seconds after the last input while playing. */
    private fun hideControls() {
        player.advance(5_100)
        assertFalse(player.controlsShown)
    }

    private fun playPause() = player.node(player.context.getString(R.string.pause))

    private fun tracksButton() = player.node(player.context.getString(R.string.audio_and_subtitles))

    /** Opens Audio & Subtitles from hidden controls and waits for its window to take the keys. */
    private fun openTracksPanel() {
        hideControls()
        player.press(KeyEvent.KEYCODE_DPAD_DOWN)
        player.press(KeyEvent.KEYCODE_DPAD_CENTER)
        player.waitFor { player.focused(player.text("English")) }
        player.advance(300)
    }

    private fun string(id: Int) = player.context.getString(id)

    private fun gear() = player.node(string(R.string.playback_settings))

    /**
     * Opens Playback Settings from hidden controls, OK held for [repeats],
     * and waits for its window to take the keys: Down lands on Audio &
     * Subtitles, and Right moves on to the gear.
     */
    private fun openSettings(repeats: Int = 0) {
        hideControls()
        player.press(KeyEvent.KEYCODE_DPAD_DOWN)
        tracksButton().assertIsFocused()
        player.press(KeyEvent.KEYCODE_DPAD_RIGHT)
        gear().assertIsFocused()
        val plays = player.plays.size
        player.press(KeyEvent.KEYCODE_DPAD_CENTER, repeats = repeats)
        player.waitFor { player.focused(player.row(string(R.string.audio_and_subtitles))) }
        player.advance(300)
        assertEquals(plays, player.plays.size)
    }

    @Test
    fun backWhileLoadingLeavesInOnePress() { // R1
        player.open(MIDDLE_MS, live = false)
        assertTrue(player.controlsShown)
        player.press(KeyEvent.KEYCODE_BACK)
        assertFalse(player.showing)
    }

    @Test
    fun okWithTheControlsHiddenShowsThemOnPlayOrPauseAndKeepsPlaying() { // R2, R3
        player.open(MIDDLE_MS)
        hideControls()
        player.press(KeyEvent.KEYCODE_DPAD_CENTER, repeats = 20)
        assertTrue(player.controlsShown)
        playPause().assertIsFocused()
        assertFalse(player.engine.paused)
        assertEquals(listOf("play"), player.engine.calls)
    }

    @Test
    fun okHeldOnTheScrubBarPlaysOrPausesOnce() { // R4
        player.open(MIDDLE_MS)
        player.press(KeyEvent.KEYCODE_MEDIA_PAUSE)
        player.node("scrub").assertIsFocused()
        player.press(KeyEvent.KEYCODE_DPAD_CENTER, repeats = 20)
        assertEquals(listOf("play", "pause", "play"), player.engine.calls)
    }

    @Test
    fun backWithAButtonFocusedHidesTheControlsAndPressesNothing() { // R5
        player.open(MIDDLE_MS)
        player.press(KeyEvent.KEYCODE_DPAD_DOWN)
        playPause().assertIsFocused()
        player.press(KeyEvent.KEYCODE_BACK)
        assertFalse(player.controlsShown)
        assertTrue(player.showing)
        assertEquals(listOf("play"), player.engine.calls)
    }

    @Test
    fun backHeldHidesTheControlsOnce() { // R6
        player.open(MIDDLE_MS)
        player.press(KeyEvent.KEYCODE_BACK, repeats = 10)
        assertFalse(player.controlsShown)
        assertTrue(player.showing)
    }

    @Test
    fun backWithTheControlsHiddenLeaves() { // R7
        player.open(MIDDLE_MS)
        hideControls()
        player.press(KeyEvent.KEYCODE_BACK)
        assertFalse(player.showing)
    }

    @Test
    fun backWhilePausedHidesTheControlsThenLeaves() { // R8
        player.open(MIDDLE_MS)
        player.press(KeyEvent.KEYCODE_MEDIA_PAUSE)
        player.press(KeyEvent.KEYCODE_BACK)
        assertFalse(player.controlsShown)
        assertTrue(player.showing)
        player.press(KeyEvent.KEYCODE_BACK)
        assertFalse(player.showing)
    }

    @Test
    fun rightWithTheControlsHiddenShowsThemAndScrubsTenSeconds() { // R9
        player.open(MIDDLE_MS)
        hideControls()
        player.press(KeyEvent.KEYCODE_DPAD_RIGHT)
        assertTrue(player.controlsShown)
        player.node("scrub").assertIsFocused()
        assertEquals(emptyList(), player.engine.seeks)
        player.advance(1_100)
        assertEquals(listOf(MIDDLE_MS + 10_000L), player.engine.seeks)
    }

    @Test
    fun rightHeldFromHiddenScrubsFasterAndSeeksOnce() { // R10
        player.open(MIDDLE_MS)
        hideControls()
        player.press(KeyEvent.KEYCODE_DPAD_RIGHT, repeats = 20)
        player.node("scrub").assertIsFocused()
        player.advance(1_100)
        assertEquals(listOf(MIDDLE_MS + (0..20).sumOf(::scrubStepMs)), player.engine.seeks)
    }

    @Test
    fun backDuringAScrubDropsItAndHidesTheControls() { // R11
        player.open(MIDDLE_MS)
        hideControls()
        player.press(KeyEvent.KEYCODE_DPAD_RIGHT)
        player.press(KeyEvent.KEYCODE_BACK)
        assertFalse(player.controlsShown)
        player.advance(1_500)
        assertEquals(emptyList(), player.engine.seeks)
    }

    @Test
    fun okDuringAScrubSeeksAtOnceAndStaysPaused() { // R12
        player.open(MIDDLE_MS)
        player.press(KeyEvent.KEYCODE_MEDIA_PAUSE)
        player.press(KeyEvent.KEYCODE_DPAD_RIGHT)
        player.press(KeyEvent.KEYCODE_DPAD_CENTER)
        assertEquals(listOf(MIDDLE_MS + 10_000L), player.engine.seeks)
        assertTrue(player.engine.paused)
    }

    @Test
    fun upAndDownOnTheScrubBarMoveFocusOnly() { // R13
        player.open(MIDDLE_MS)
        player.node("scrub").assertIsFocused()
        player.press(KeyEvent.KEYCODE_DPAD_UP)
        player.node("scrub").assertIsFocused()
        player.press(KeyEvent.KEYCODE_DPAD_DOWN)
        playPause().assertIsFocused()
        player.press(KeyEvent.KEYCODE_DPAD_UP)
        player.node("scrub").assertIsFocused()
        player.advance(1_100)
        assertEquals(emptyList(), player.engine.seeks)
    }

    @Test
    fun rightOverSkipShowsTheControlsAndScrubs() { // R14
        player.open(IN_INTRO_MS)
        hideControls()
        player.node("skip").assertIsFocused()
        player.press(KeyEvent.KEYCODE_DPAD_RIGHT)
        assertTrue(player.controlsShown)
        player.advance(1_100)
        assertEquals(listOf(IN_INTRO_MS + 10_000L), player.engine.seeks)
    }

    @Test
    fun okOverSkipSkipsWithTheControlsHidden() { // R15
        player.open(IN_INTRO_MS)
        hideControls()
        player.press(KeyEvent.KEYCODE_DPAD_CENTER)
        assertEquals(listOf(INTRO_END_MS), player.engine.seeks)
        assertFalse(player.controlsShown)
    }

    @Test
    fun skipTakesFocusAsTheControlsHideUnderItAndIsntPressed() { // R16
        player.open(IN_INTRO_MS)
        player.node("scrub").assertIsFocused()
        hideControls()
        player.node("skip").assertIsFocused()
        assertEquals(emptyList(), player.engine.seeks)
    }

    @Test
    fun upNextAppearingOverShownControlsLeavesFocusWhereItIs() { // R17
        player.open(NEAR_END_MS)
        player.node("playNow").assertExists()
        player.node("scrub").assertIsFocused()
    }

    @Test
    fun leftAndRightOverUpNextMoveBetweenItsButtonsWithTheControlsHidden() { // R18
        player.open(NEAR_END_MS)
        hideControls()
        player.node("playNow").assertIsFocused()
        player.press(KeyEvent.KEYCODE_DPAD_RIGHT)
        player.node("hideUpNext").assertIsFocused()
        player.press(KeyEvent.KEYCODE_DPAD_LEFT)
        player.node("playNow").assertIsFocused()
        assertFalse(player.controlsShown)
    }

    @Test
    fun backOverUpNextLeaves() { // R19
        player.open(NEAR_END_MS)
        hideControls()
        player.press(KeyEvent.KEYCODE_BACK)
        assertFalse(player.showing)
    }

    @Test
    fun upNextsCountdownStopsWhileAPanelIsOpen() { // R20
        player.open(NEAR_END_MS)
        openTracksPanel()
        val frozen = player.countdown()
        player.advance(3_000)
        assertEquals(frozen, player.countdown())
        player.press(KeyEvent.KEYCODE_BACK)
        player.advance(1_000)
        assertTrue(player.countdown() < frozen)
    }

    @Test
    fun okAtStillWatchingPlaysTheNextEpisodeWithTheControlsShown() { // R21
        player.playedOnTheirOwn()
        player.open(NEAR_END_MS)
        player.text(player.context.getString(R.string.still_watching).uppercase()).assertExists()
        player.onMain { player.engine.end() }
        player.advance(100)
        assertTrue(player.controlsShown)
        player.node("playNow").assertIsFocused()
        player.press(KeyEvent.KEYCODE_DPAD_CENTER)
        assertEquals(Screen.Player(NEXT), player.navigator.current.screen)
        assertFalse(0L in player.engine.seeks)
    }

    @Test
    fun mediaKeysInAPanelDoWhatTheyDoWithout() { // R22
        player.open(MIDDLE_MS)
        openTracksPanel()
        player.press(KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE)
        assertTrue(player.engine.paused)
        player.press(KeyEvent.KEYCODE_MEDIA_FAST_FORWARD)
        player.press(KeyEvent.KEYCODE_MEDIA_REWIND)
        assertEquals(listOf(MIDDLE_MS + 30_000L, MIDDLE_MS + 20_000L), player.engine.seeks)
        player.press(KeyEvent.KEYCODE_MEDIA_PREVIOUS)
        assertEquals(Screen.Player(PREVIOUS), player.navigator.current.screen)
    }

    @Test
    fun backInAPanelClosesItAndFocusGoesBackToItsButton() { // R23
        player.open(MIDDLE_MS)
        openTracksPanel()
        player.press(KeyEvent.KEYCODE_BACK)
        player.waitFor { player.focused(tracksButton()) }
        player.text("French").assertDoesNotExist()
        assertTrue(player.showing)
        assertTrue(player.controlsShown)
    }

    @Test
    fun infoOpensAndClosesStreamInfo() { // R24
        player.open(MIDDLE_MS)
        player.press(KeyEvent.KEYCODE_INFO)
        player.waitFor { player.focused(player.node("stream-info")) }
        player.advance(300)
        player.press(KeyEvent.KEYCODE_INFO)
        player.node("stream-info").assertDoesNotExist()
        assertTrue(player.showing)
    }

    @Test
    fun playPauseHeldTogglesOnce() { // R25
        player.open(MIDDLE_MS)
        hideControls()
        player.press(KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE, repeats = 20)
        assertEquals(listOf("play", "pause"), player.engine.calls)
    }

    @Test
    fun nextHeldGoesToOneEpisode() { // R26
        player.open(MIDDLE_MS)
        hideControls()
        val entry = player.navigator.current.id
        player.press(KeyEvent.KEYCODE_MEDIA_NEXT, repeats = 10)
        assertEquals(Screen.Player(NEXT), player.navigator.current.screen)
        assertEquals(entry + 1, player.navigator.current.id)
    }

    @Test
    fun fastForwardHeldSkipsOnEachRepeat() { // R27
        player.open(MIDDLE_MS)
        hideControls()
        player.press(KeyEvent.KEYCODE_MEDIA_FAST_FORWARD, repeats = 10)
        assertEquals((1..11).map { MIDDLE_MS + it * 30_000L }, player.engine.seeks)
    }

    @Test
    fun fastForwardAndRewindMoveAPendingScrubTarget() {
        player.open(MIDDLE_MS)
        hideControls()
        player.press(KeyEvent.KEYCODE_DPAD_RIGHT)
        player.press(KeyEvent.KEYCODE_MEDIA_FAST_FORWARD)
        player.press(KeyEvent.KEYCODE_MEDIA_REWIND)
        assertEquals(emptyList(), player.engine.seeks)
        player.advance(1_100)
        assertEquals(listOf(MIDDLE_MS + 30_000L), player.engine.seeks)
    }

    @Test
    fun okHeldOnTheGearOpensItAndChoosesNothingUntilAFreshPress() { // R28
        player.open(MIDDLE_MS)
        openSettings(repeats = 20)
        player.text(string(R.string.quality)).assertExists()
        player.text("English").assertDoesNotExist()
        player.press(KeyEvent.KEYCODE_DPAD_CENTER)
        player.waitFor { player.focused(player.text("English")) }
    }

    @Test
    fun theGearListsAudioAndSubtitlesQualitySpeedAndStreamInfo() {
        player.open(MIDDLE_MS)
        openSettings()
        val rows = listOf(R.string.audio_and_subtitles, R.string.quality, R.string.speed, R.string.stream_info).map(::string)
        val tops = rows.map { player.row(it).fetchSemanticsNode().boundsInWindow.top }
        assertEquals(tops.sorted(), tops, "rows in the order $rows")
        assertTrue(player.focused(player.row(rows.first())))
        // One version: no Version row. The rows say what's playing.
        player.text(string(R.string.version)).assertDoesNotExist()
        player.text(string(R.string.speed_normal)).assertExists()
        player.text("Auto").assertExists() // the quality, as the core names it
    }

    @Test
    fun theGearsAudioAndSubtitlesOpensTheTracksPanelAndBackReturnsStepByStep() {
        player.open(MIDDLE_MS)
        openSettings()
        player.press(KeyEvent.KEYCODE_DPAD_CENTER)
        player.waitFor { player.focused(player.text("English")) }
        player.press(KeyEvent.KEYCODE_BACK)
        player.waitFor { player.focused(player.row(string(R.string.audio_and_subtitles))) }
        player.press(KeyEvent.KEYCODE_BACK)
        player.waitFor { player.focused(gear()) }
        player.text(string(R.string.quality)).assertDoesNotExist()
        assertTrue(player.controlsShown)
    }

    @Test
    fun streamInfoFromTheGearTakesThePanelsPlace() {
        player.open(MIDDLE_MS)
        openSettings()
        repeat(3) { player.press(KeyEvent.KEYCODE_DPAD_DOWN) }
        player.press(KeyEvent.KEYCODE_DPAD_CENTER)
        player.waitFor { player.focused(player.node("stream-info")) }
        player.advance(300)
        player.press(KeyEvent.KEYCODE_BACK)
        player.node("stream-info").assertDoesNotExist()
        player.text(string(R.string.quality)).assertDoesNotExist()
    }

    @Test
    fun dialogueBoostSwitchesWithANewPlayAtThePositionAndIsKept() {
        player.open(MIDDLE_MS)
        assertTrue(player.plays.all { it.dialogueBoost == false })
        openTracksPanel()
        val plays = player.plays.size
        player.text(string(R.string.dialogue_boost)).assertExists()
        // What the boost costs is in Stream info, not under the switch.
        player.text("Plays the audio as AAC while on").assertDoesNotExist()
        // The switch sits above the audio tracks, where focus starts.
        player.press(KeyEvent.KEYCODE_DPAD_UP)
        assertTrue(player.focused(player.text(string(R.string.dialogue_boost))))
        player.press(KeyEvent.KEYCODE_DPAD_CENTER)
        player.waitFor { player.plays.size == plays + 1 }
        val switched = player.plays.last()
        assertEquals(true, switched.dialogueBoost)
        assertEquals(MIDDLE_MS, switched.positionMs)
        assertEquals("$EPISODE-version", switched.versionId)
        // The panel stays open on the switch, and the device keeps it for the next play.
        player.advance(300)
        assertTrue(player.focused(player.text(string(R.string.dialogue_boost))))
        player.waitFor { runBlocking { player.core.settings.first().dialogueBoost } }
    }

    @Test
    fun speedFromTheGearAppliesToTheVideo() {
        player.open(MIDDLE_MS)
        openSettings()
        val plays = player.plays.size
        player.press(KeyEvent.KEYCODE_DPAD_DOWN)
        player.press(KeyEvent.KEYCODE_DPAD_DOWN)
        player.press(KeyEvent.KEYCODE_DPAD_CENTER)
        player.waitFor { player.focused(player.text(string(R.string.speed_normal))) }
        player.advance(300)
        player.press(KeyEvent.KEYCODE_DPAD_DOWN)
        player.press(KeyEvent.KEYCODE_DPAD_CENTER)
        assertEquals(1.25f, player.engine.speed)
        player.text(string(R.string.speed)).assertDoesNotExist()
        assertEquals(plays, player.plays.size, "a speed needs no new play")
    }

    @Test
    fun controlsHideAfterFiveSecondsWhilePlayingOnly() { // R29
        player.open(MIDDLE_MS)
        player.advance(4_900)
        assertTrue(player.controlsShown)
        player.advance(200)
        assertFalse(player.controlsShown)
        player.press(KeyEvent.KEYCODE_MEDIA_PAUSE)
        player.advance(6_000)
        assertTrue(player.controlsShown)
        player.press(KeyEvent.KEYCODE_MEDIA_PLAY)
        player.press(KeyEvent.KEYCODE_INFO)
        player.advance(6_000)
        assertTrue(player.controlsShown)
    }

    @Test
    fun aReleaseWhosePressWasntSeenDoesNothing() { // R33
        player.open(MIDDLE_MS)
        hideControls()
        player.up(KeyEvent.KEYCODE_DPAD_CENTER)
        player.down(KeyEvent.KEYCODE_DPAD_CENTER, repeatCount = 3)
        player.up(KeyEvent.KEYCODE_DPAD_CENTER)
        player.advance(100)
        assertFalse(player.controlsShown)
        assertEquals(listOf("play"), player.engine.calls)
    }

    @Test
    fun downWithTheControlsHiddenFocusesTheTrackButtonsOverSkipToo() { // R34
        player.open(MIDDLE_MS)
        hideControls()
        player.press(KeyEvent.KEYCODE_DPAD_DOWN)
        tracksButton().assertIsFocused()
    }

    @Test
    fun downOverSkipFocusesTheTrackButtons() { // R34
        player.open(IN_INTRO_MS)
        hideControls()
        player.press(KeyEvent.KEYCODE_DPAD_DOWN)
        tracksButton().assertIsFocused()
    }
}
