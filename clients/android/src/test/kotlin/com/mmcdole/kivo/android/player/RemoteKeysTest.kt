package com.mmcdole.kivo.android.player

import android.view.KeyEvent
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class RemoteKeysTest {
    private val hidden = RemoteState(controlsVisible = false)
    private val shown = RemoteState(controlsVisible = true)

    @Test
    fun withControlsHiddenOkShowsThemOnPlayAndTheArrowsShowThemAndScrub() {
        assertEquals(PlayerCommand.FocusPlay, playerCommand(KeyEvent.KEYCODE_DPAD_CENTER, hidden))
        assertEquals(PlayerCommand.FocusPlay, playerCommand(KeyEvent.KEYCODE_ENTER, hidden))
        assertEquals(PlayerCommand.ScrubBack, playerCommand(KeyEvent.KEYCODE_DPAD_LEFT, hidden))
        assertEquals(PlayerCommand.ScrubForward, playerCommand(KeyEvent.KEYCODE_DPAD_RIGHT, hidden))
        assertEquals(PlayerCommand.ShowControls, playerCommand(KeyEvent.KEYCODE_DPAD_UP, hidden))
        assertEquals(PlayerCommand.FocusTracks, playerCommand(KeyEvent.KEYCODE_DPAD_DOWN, hidden))
    }

    @Test
    fun withControlsShownOkAndTheArrowsAreTheFocusedControls() {
        for (key in listOf(KeyEvent.KEYCODE_DPAD_CENTER, KeyEvent.KEYCODE_DPAD_LEFT, KeyEvent.KEYCODE_DPAD_RIGHT, KeyEvent.KEYCODE_DPAD_UP, KeyEvent.KEYCODE_DPAD_DOWN)) {
            assertNull(playerCommand(key, shown))
        }
    }

    @Test
    fun overSkipOkSkipsLeftAndRightScrubAndUpAndDownShowTheControls() {
        val skip = hidden.copy(overlay = Overlay.Skip)
        assertEquals(PlayerCommand.Skip, playerCommand(KeyEvent.KEYCODE_DPAD_CENTER, skip))
        assertEquals(PlayerCommand.ScrubBack, playerCommand(KeyEvent.KEYCODE_DPAD_LEFT, skip))
        assertEquals(PlayerCommand.ScrubForward, playerCommand(KeyEvent.KEYCODE_DPAD_RIGHT, skip))
        assertEquals(PlayerCommand.ShowControls, playerCommand(KeyEvent.KEYCODE_DPAD_UP, skip))
        assertEquals(PlayerCommand.FocusTracks, playerCommand(KeyEvent.KEYCODE_DPAD_DOWN, skip))
    }

    @Test
    fun overUpNextOkLeftAndRightAreItsButtons() {
        val upNext = hidden.copy(overlay = Overlay.UpNext)
        assertNull(playerCommand(KeyEvent.KEYCODE_DPAD_CENTER, upNext))
        assertNull(playerCommand(KeyEvent.KEYCODE_DPAD_LEFT, upNext))
        assertNull(playerCommand(KeyEvent.KEYCODE_DPAD_RIGHT, upNext))
        assertEquals(PlayerCommand.ShowControls, playerCommand(KeyEvent.KEYCODE_DPAD_UP, upNext))
        assertEquals(PlayerCommand.FocusTracks, playerCommand(KeyEvent.KEYCODE_DPAD_DOWN, upNext))
    }

    @Test
    fun backClosesAMenuThenHidesTheControlsThenExits() {
        for (key in listOf(KeyEvent.KEYCODE_BACK, KeyEvent.KEYCODE_ESCAPE)) {
            assertEquals(PlayerCommand.CloseMenu, playerCommand(key, shown.copy(menuOpen = true)))
            assertEquals(PlayerCommand.HideControls, playerCommand(key, shown))
            assertEquals(PlayerCommand.Exit, playerCommand(key, hidden))
            assertEquals(PlayerCommand.Exit, playerCommand(key, hidden.copy(overlay = Overlay.Skip)))
            assertEquals(PlayerCommand.Exit, playerCommand(key, hidden.copy(overlay = Overlay.UpNext)))
        }
    }

    @Test
    fun backLeavesAtOnceWhileLoading() {
        assertEquals(PlayerCommand.Exit, playerCommand(KeyEvent.KEYCODE_BACK, shown.copy(loading = true)))
    }

    @Test
    fun aMenuTakesOkAndTheArrows() {
        val menu = shown.copy(menuOpen = true)
        for (key in listOf(KeyEvent.KEYCODE_DPAD_CENTER, KeyEvent.KEYCODE_DPAD_LEFT, KeyEvent.KEYCODE_DPAD_UP, KeyEvent.KEYCODE_DPAD_DOWN)) {
            assertNull(playerCommand(key, menu))
        }
    }

    @Test
    fun mediaKeysAndInfoAreThePlayersWhateverShows() {
        val states = listOf(hidden, shown, shown.copy(menuOpen = true), hidden.copy(overlay = Overlay.Skip), shown.copy(loading = true))
        for (state in states) {
            assertEquals(PlayerCommand.TogglePlay, playerCommand(KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE, state))
            assertEquals(PlayerCommand.Play, playerCommand(KeyEvent.KEYCODE_MEDIA_PLAY, state))
            assertEquals(PlayerCommand.Pause, playerCommand(KeyEvent.KEYCODE_MEDIA_PAUSE, state))
            assertEquals(PlayerCommand.SkipForward, playerCommand(KeyEvent.KEYCODE_MEDIA_FAST_FORWARD, state))
            assertEquals(PlayerCommand.SkipBack, playerCommand(KeyEvent.KEYCODE_MEDIA_REWIND, state))
            assertEquals(PlayerCommand.Previous, playerCommand(KeyEvent.KEYCODE_MEDIA_PREVIOUS, state))
            assertEquals(PlayerCommand.Next, playerCommand(KeyEvent.KEYCODE_MEDIA_NEXT, state))
            assertEquals(PlayerCommand.Info, playerCommand(KeyEvent.KEYCODE_INFO, state))
        }
    }

    @Test
    fun aScrubStepsTenSecondsAPressThenFasterWhenHeld() {
        assertEquals(10_000L, scrubStepMs(0))
        assertEquals(10_000L, scrubStepMs(4))
        assertEquals(30_000L, scrubStepMs(5))
        assertEquals(60_000L, scrubStepMs(15))
    }
}
