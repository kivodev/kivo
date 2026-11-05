package com.mmcdole.kivo.android.player

import android.view.KeyEvent
import com.mmcdole.kivo.android.ui.BackKeys
import com.mmcdole.kivo.android.ui.OkKeys

/** What a remote key does in the player. */
enum class PlayerCommand {
    TogglePlay,
    Play,
    Pause,

    /** Back 10 seconds, or the scrub target back as far when there is one. */
    SkipBack,

    /** Forward 30 seconds, or the scrub target forward as far when there is one. */
    SkipForward,

    /** Show the controls with focus on the scrub bar and move its target back a step. */
    ScrubBack,

    /** Show the controls with focus on the scrub bar and move its target forward a step. */
    ScrubForward,
    Previous,
    Next,

    /** Show the controls with focus on the scrub bar. */
    ShowControls,

    /** Show the controls with focus on play or pause. */
    FocusPlay,

    /** Show the controls with focus on the track buttons. */
    FocusTracks,

    /** Skip to the end of the intro or credits, leaving the controls hidden. */
    Skip,
    HideControls,
    CloseMenu,
    Exit,

    /** Open the stream info, or close it when it's open. */
    Info,
}

/** The button over the picture while the controls are hidden. */
enum class Overlay { None, Skip, UpNext }

/** What the player shows as a key goes down, which [playerCommand] maps the key by. */
data class RemoteState(
    val controlsVisible: Boolean,
    val overlay: Overlay = Overlay.None,
    val menuOpen: Boolean = false,
    val loading: Boolean = false,
)

/**
 * The remote map. Media keys, Info and Back are always the player's: Back
 * closes an open menu, leaves while the video loads, hides shown controls,
 * and leaves when they're hidden. A menu takes OK and the arrows, as do the
 * shown controls' focused control. With the controls hidden, OK shows them
 * on play or pause, Left and Right show them and scrub, Up shows them and
 * Down shows them on the track buttons; over a Skip button OK skips and
 * Left and Right still scrub, and over Up next OK, Left and Right are its
 * buttons'. Null leaves the key to the focused control.
 */
fun playerCommand(keyCode: Int, state: RemoteState): PlayerCommand? = when (keyCode) {
    KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE, KeyEvent.KEYCODE_SPACE -> PlayerCommand.TogglePlay
    KeyEvent.KEYCODE_MEDIA_PLAY -> PlayerCommand.Play
    KeyEvent.KEYCODE_MEDIA_PAUSE, KeyEvent.KEYCODE_MEDIA_STOP -> PlayerCommand.Pause
    KeyEvent.KEYCODE_MEDIA_REWIND -> PlayerCommand.SkipBack
    KeyEvent.KEYCODE_MEDIA_FAST_FORWARD -> PlayerCommand.SkipForward
    KeyEvent.KEYCODE_MEDIA_PREVIOUS -> PlayerCommand.Previous
    KeyEvent.KEYCODE_MEDIA_NEXT -> PlayerCommand.Next
    KeyEvent.KEYCODE_INFO, KeyEvent.KEYCODE_GUIDE -> PlayerCommand.Info
    in BackKeys -> when {
        state.menuOpen -> PlayerCommand.CloseMenu
        state.loading || !state.controlsVisible -> PlayerCommand.Exit
        else -> PlayerCommand.HideControls
    }
    else -> if (state.menuOpen || state.controlsVisible) null else hiddenCommand(keyCode, state.overlay)
}

/** OK and the arrows with the controls hidden, under [overlay]. */
private fun hiddenCommand(keyCode: Int, overlay: Overlay): PlayerCommand? = when (keyCode) {
    in OkKeys -> when (overlay) {
        Overlay.None -> PlayerCommand.FocusPlay
        Overlay.Skip -> PlayerCommand.Skip
        Overlay.UpNext -> null
    }
    KeyEvent.KEYCODE_DPAD_LEFT -> if (overlay == Overlay.UpNext) null else PlayerCommand.ScrubBack
    KeyEvent.KEYCODE_DPAD_RIGHT -> if (overlay == Overlay.UpNext) null else PlayerCommand.ScrubForward
    KeyEvent.KEYCODE_DPAD_UP -> PlayerCommand.ShowControls
    KeyEvent.KEYCODE_DPAD_DOWN -> PlayerCommand.FocusTracks
    else -> null
}

/** How far one Left or Right moves the scrub target: 10 seconds a press, 30 and then 60 as the key is held. */
fun scrubStepMs(repeatCount: Int): Long = when {
    repeatCount >= 15 -> 60_000L
    repeatCount >= 5 -> 30_000L
    else -> 10_000L
}
