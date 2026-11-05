package com.mmcdole.kivo.android.ui

import android.view.KeyEvent
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class KeyDispatcherTest {
    /** What the actions ran, as "key@repeatCount", and what each first DOWN was asked. */
    private val ran = mutableListOf<String>()
    private val asked = mutableListOf<Int>()

    /** A dispatcher that acts on [taken] keys and leaves the rest to the focused control. */
    private fun dispatcher(vararg taken: Int) = KeyDispatcher { key ->
        asked += key
        if (key in taken) KeyAction { ran += "$key@$it" } else null
    }

    private fun KeyDispatcher.hold(key: Int, repeats: Int) = (0..repeats).map { down(key, it) }

    @Test
    fun okAndBackActOnceAsTheyAreReleased() {
        val keys = dispatcher(KeyEvent.KEYCODE_DPAD_CENTER, KeyEvent.KEYCODE_BACK)
        for (key in listOf(KeyEvent.KEYCODE_DPAD_CENTER, KeyEvent.KEYCODE_BACK)) {
            assertTrue(keys.hold(key, repeats = 20).all { it })
            assertEquals(emptyList(), ran)
            assertTrue(keys.up(key))
            assertEquals(listOf("$key@0"), ran)
            ran.clear()
        }
    }

    @Test
    fun otherKeysActOnTheirFirstDownAndOnlyArrowsAndSkipsAgainOnRepeats() {
        val keys = dispatcher(KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE, KeyEvent.KEYCODE_MEDIA_NEXT, KeyEvent.KEYCODE_MEDIA_FAST_FORWARD, KeyEvent.KEYCODE_DPAD_RIGHT)
        keys.hold(KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE, repeats = 20)
        keys.up(KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE)
        keys.hold(KeyEvent.KEYCODE_MEDIA_NEXT, repeats = 10)
        keys.up(KeyEvent.KEYCODE_MEDIA_NEXT)
        assertEquals(listOf("${KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE}@0", "${KeyEvent.KEYCODE_MEDIA_NEXT}@0"), ran)
        ran.clear()

        keys.hold(KeyEvent.KEYCODE_MEDIA_FAST_FORWARD, repeats = 2)
        keys.up(KeyEvent.KEYCODE_MEDIA_FAST_FORWARD)
        keys.hold(KeyEvent.KEYCODE_DPAD_RIGHT, repeats = 2)
        keys.up(KeyEvent.KEYCODE_DPAD_RIGHT)
        val ff = KeyEvent.KEYCODE_MEDIA_FAST_FORWARD
        val right = KeyEvent.KEYCODE_DPAD_RIGHT
        assertEquals(listOf("$ff@0", "$ff@1", "$ff@2", "$right@0", "$right@1", "$right@2"), ran)
    }

    @Test
    fun aPressIsDecidedOnceAtItsFirstDown() {
        val keys = dispatcher(KeyEvent.KEYCODE_DPAD_LEFT)
        keys.hold(KeyEvent.KEYCODE_DPAD_LEFT, repeats = 5)
        keys.up(KeyEvent.KEYCODE_DPAD_LEFT)
        assertEquals(listOf(KeyEvent.KEYCODE_DPAD_LEFT), asked)
    }

    @Test
    fun aPressLeftToTheFocusedControlReachesItWhole() {
        val keys = dispatcher()
        assertEquals(listOf(false, false, false), keys.hold(KeyEvent.KEYCODE_DPAD_CENTER, repeats = 2))
        assertFalse(keys.up(KeyEvent.KEYCODE_DPAD_CENTER))
    }

    @Test
    fun aPressLeftToAControlStopsReachingItOnceFocusIsMovedFromUnderIt() {
        val keys = dispatcher()
        assertFalse(keys.down(KeyEvent.KEYCODE_DPAD_CENTER, 0))
        keys.focusMoved()
        assertTrue(keys.down(KeyEvent.KEYCODE_DPAD_CENTER, 1))
        assertTrue(keys.up(KeyEvent.KEYCODE_DPAD_CENTER))
        // The next press starts afresh.
        assertFalse(keys.down(KeyEvent.KEYCODE_DPAD_CENTER, 0))
        assertFalse(keys.up(KeyEvent.KEYCODE_DPAD_CENTER))
    }

    @Test
    fun eventsOfAPressWhoseFirstDownWasntSeenAreDropped() {
        val keys = dispatcher(KeyEvent.KEYCODE_BACK)
        assertTrue(keys.up(KeyEvent.KEYCODE_DPAD_CENTER))
        assertTrue(keys.down(KeyEvent.KEYCODE_DPAD_CENTER, 3))
        assertTrue(keys.up(KeyEvent.KEYCODE_BACK))
        assertEquals(emptyList(), ran)
        assertEquals(emptyList(), asked)
    }

    @Test
    fun aNewPressEndsOneWhoseReleaseWentElsewhere() {
        val keys = dispatcher(KeyEvent.KEYCODE_BACK)
        keys.down(KeyEvent.KEYCODE_BACK, 0)
        keys.down(KeyEvent.KEYCODE_DPAD_DOWN, 0)
        keys.up(KeyEvent.KEYCODE_BACK)
        assertEquals(emptyList(), ran)
    }
}
