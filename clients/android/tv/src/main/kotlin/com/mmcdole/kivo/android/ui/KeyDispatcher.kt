package com.mmcdole.kivo.android.ui

import android.view.KeyEvent
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.window.DialogProperties

/** What a key press does, run with the press's repeat count (0 for its first DOWN and its release). */
fun interface KeyAction {
    fun run(repeatCount: Int)
}

/**
 * KeyDispatcher owns each key press on a screen, from its first DOWN to its
 * UP, in every window the screen opens: its panels and dialogs forward their
 * keys to it. At the first DOWN it asks [decide], once, what the press does:
 * an action, or null to leave the press to the focused control. The rest of
 * the press follows that answer and reaches nothing else.
 *
 * An action runs as OK or Back is released and as any other key goes down;
 * held, only the arrows, fast forward and rewind run it again on each
 * repeat. A press left to a control stops reaching it once the screen moves
 * focus from under it ([focusMoved]). Events of a press whose first DOWN the
 * dispatcher didn't see are dropped.
 */
class KeyDispatcher(private val decide: (keyCode: Int) -> KeyAction?) {
    private class Press(val keyCode: Int, val action: KeyAction?, val focusMoves: Int)

    private var press: Press? = null
    private var focusMoves = 0

    /** Handles a key event; whether it's taken from the focused control. */
    fun dispatch(event: KeyEvent): Boolean = when (event.action) {
        KeyEvent.ACTION_DOWN -> down(event.keyCode, event.repeatCount)
        KeyEvent.ACTION_UP -> up(event.keyCode)
        else -> false
    }

    fun down(keyCode: Int, repeatCount: Int): Boolean {
        if (repeatCount == 0) {
            val action = decide(keyCode)
            press = Press(keyCode, action, focusMoves)
            if (action != null && !actsOnRelease(keyCode)) action.run(0)
            return action != null
        }
        val current = press?.takeIf { it.keyCode == keyCode } ?: return true
        val action = current.action ?: return current.focusMoves != focusMoves
        if (actsOnRepeat(keyCode)) action.run(repeatCount)
        return true
    }

    fun up(keyCode: Int): Boolean {
        val current = press?.takeIf { it.keyCode == keyCode } ?: return true
        press = null
        val action = current.action ?: return current.focusMoves != focusMoves
        if (actsOnRelease(keyCode)) action.run(0)
        return true
    }

    /** The screen moved focus itself: the press under way no longer reaches the control it began on. */
    fun focusMoved() {
        focusMoves++
    }
}

/** A [KeyDispatcher] that asks the latest [decide] passed to it. */
@Composable
fun rememberKeyDispatcher(decide: (keyCode: Int) -> KeyAction?): KeyDispatcher {
    val current by rememberUpdatedState(decide)
    return remember { KeyDispatcher { current(it) } }
}

/** The dispatcher of the screen a panel opens over, which takes the panel's keys too; null where a panel keeps its own. */
val LocalKeyDispatcher = staticCompositionLocalOf<KeyDispatcher?> { null }

/** The dispatcher a panel's or dialog's window hands its keys to: the screen's it opens over, else its own. */
@Composable
fun rememberPanelKeys(): KeyDispatcher {
    val own = remember { KeyDispatcher { null } }
    return LocalKeyDispatcher.current ?: own
}

/**
 * A panel's or dialog's window properties: full width, closed by Back
 * only where it keeps its own keys, as the screen's dispatcher decides
 * what Back does with the keys it takes.
 */
@Composable
fun panelProperties(): DialogProperties =
    DialogProperties(usePlatformDefaultWidth = false, dismissOnBackPress = LocalKeyDispatcher.current == null)

/** OK on a remote, and Enter on a keyboard. */
val OkKeys = setOf(KeyEvent.KEYCODE_DPAD_CENTER, KeyEvent.KEYCODE_ENTER, KeyEvent.KEYCODE_NUMPAD_ENTER)

/** The remote's arrows and OK, which move or use focus. */
val RemoteKeys = OkKeys + setOf(
    KeyEvent.KEYCODE_DPAD_UP,
    KeyEvent.KEYCODE_DPAD_DOWN,
    KeyEvent.KEYCODE_DPAD_LEFT,
    KeyEvent.KEYCODE_DPAD_RIGHT,
)

/** Back on a remote, and Escape on a keyboard. */
val BackKeys = setOf(KeyEvent.KEYCODE_BACK, KeyEvent.KEYCODE_ESCAPE)

private fun actsOnRelease(keyCode: Int) = keyCode in OkKeys || keyCode in BackKeys

private fun actsOnRepeat(keyCode: Int) = when (keyCode) {
    KeyEvent.KEYCODE_DPAD_LEFT, KeyEvent.KEYCODE_DPAD_RIGHT, KeyEvent.KEYCODE_DPAD_UP, KeyEvent.KEYCODE_DPAD_DOWN,
    KeyEvent.KEYCODE_MEDIA_FAST_FORWARD, KeyEvent.KEYCODE_MEDIA_REWIND,
    -> true
    else -> false
}
