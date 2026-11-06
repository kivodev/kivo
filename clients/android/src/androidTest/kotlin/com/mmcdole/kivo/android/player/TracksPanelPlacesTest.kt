package com.mmcdole.kivo.android.player

import android.view.KeyEvent
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.requestFocus
import androidx.test.ext.junit.runners.AndroidJUnit4
import kotlinx.coroutines.test.TestCoroutineScheduler
import org.junit.After
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import kotlin.test.assertFalse
import kotlin.test.assertTrue

// The title page's Audio & Subtitles row opens its panel, and the player's
// button its menu, both of TrackColumns, the audio column marking them: with
// Automatic heading the column before a play, and Dialogue boost above the
// tracks in a play.

@RunWith(AndroidJUnit4::class)
class TracksPanelPlacesTest {
    /** The screen's clock, which the core's player waits on too. */
    private val clock = TestCoroutineScheduler()

    @get:Rule
    val compose = createComposeRule(clock)

    private val harness by lazy { PlayerHarness(compose, clock) }

    @After
    fun close() = harness.close()

    private fun panelOpens() = harness.waitFor { harness.shows(harness.node("audio-column")) }

    @Test
    fun theDetailPageOpensItsPanelWithAutomatic() {
        harness.openDetailPage()
        harness.node("tracks").requestFocus()
        harness.press(KeyEvent.KEYCODE_DPAD_CENTER)
        panelOpens()
        assertTrue(harness.shows(harness.node("track:automatic-audio")))
        assertTrue(harness.shows(harness.node("track:english")))
        assertFalse(harness.shows(harness.node("dialogue-boost")))
    }

    @Test
    fun thePlayerOpensItsMenuWithDialogueBoost() {
        harness.open(MIDDLE_MS)
        // Down from hidden controls lands on Audio & Subtitles.
        harness.advance(5_100)
        harness.press(KeyEvent.KEYCODE_DPAD_DOWN)
        harness.press(KeyEvent.KEYCODE_DPAD_CENTER)
        panelOpens()
        assertTrue(harness.shows(harness.node("track:english")))
        assertTrue(harness.shows(harness.node("dialogue-boost")))
        assertFalse(harness.shows(harness.node("track:automatic-audio")))
    }
}
