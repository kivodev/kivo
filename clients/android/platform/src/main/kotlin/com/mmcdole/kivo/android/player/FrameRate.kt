package com.mmcdole.kivo.android.player

import android.app.Activity
import android.hardware.display.DisplayManager
import android.os.Build
import androidx.annotation.ChecksSdkIntAtLeast
import com.mmcdole.kivo.android.settings.ScreenSettings
import kotlin.math.abs
import kotlin.math.roundToInt

/**
 * Whether the system's Match content frame rate decides frame-rate
 * matching: on Android 12 and later, where the app has no setting of its
 * own.
 */
@get:ChecksSdkIntAtLeast(api = Build.VERSION_CODES.S)
val systemMatchesFrameRate: Boolean get() = Build.VERSION.SDK_INT >= Build.VERSION_CODES.S

/** Whether the player matches the display to the video: as the system says from Android 12, else by this TV's setting. */
val ScreenSettings.matchesFrameRate: Boolean get() = systemMatchesFrameRate || frameRateMatching

/** A display mode: its ID, size and refresh rate. */
data class DisplayModeInfo(val id: Int, val width: Int, val height: Int, val refreshRate: Float)

/**
 * The display mode to switch to for video at [frameRate]: one at the
 * current resolution refreshing at the frame rate or a whole multiple of it,
 * the smallest multiple first (23.976 fps prefers 23.976 Hz, then 47.952);
 * null when the current mode already refreshes at a whole multiple (25 fps
 * at 50 Hz plays smoothly), or none does.
 */
fun matchingMode(modes: List<DisplayModeInfo>, current: DisplayModeInfo, frameRate: Float): DisplayModeInfo? {
    if (frameRate <= 0f) return null
    fun multiple(mode: DisplayModeInfo): Int? {
        val ratio = mode.refreshRate / frameRate
        val whole = ratio.roundToInt()
        return whole.takeIf { it >= 1 && abs(ratio - it) < 0.0005f * it }
    }
    if (multiple(current) != null) return null
    return modes
        .filter { it.width == current.width && it.height == current.height }
        .mapNotNull { mode -> multiple(mode)?.let { mode to it } }
        .minByOrNull { it.second }
        ?.first
}

/**
 * Frame-rate matching: Media3 asks the surface for the
 * video's rate, which switches where the switch is seamless; where it isn't,
 * the window asks for a matching display mode, as the system's own
 * match-content setting allows (Android 12 and later: Always; Never and
 * Seamless only leave it to the surface). The mode is let go as the player
 * closes.
 */
class FrameRateMatcher(private val activity: Activity) {
    private var switched = false

    fun match(frameRate: Float) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            val preference = activity.getSystemService(DisplayManager::class.java).matchContentFrameRateUserPreference
            if (preference != DisplayManager.MATCH_CONTENT_FRAMERATE_ALWAYS) return
        }
        @Suppress("DEPRECATION")
        val display = activity.windowManager.defaultDisplay ?: return
        val modes = display.supportedModes.map { DisplayModeInfo(it.modeId, it.physicalWidth, it.physicalHeight, it.refreshRate) }
        val current = display.mode.let { DisplayModeInfo(it.modeId, it.physicalWidth, it.physicalHeight, it.refreshRate) }
        val mode = matchingMode(modes, current, frameRate) ?: return
        activity.window.attributes = activity.window.attributes.apply { preferredDisplayModeId = mode.id }
        switched = true
    }

    fun release() {
        if (!switched) return
        activity.window.attributes = activity.window.attributes.apply { preferredDisplayModeId = 0 }
        switched = false
    }
}
