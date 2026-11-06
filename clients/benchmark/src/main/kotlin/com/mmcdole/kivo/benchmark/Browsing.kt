package com.mmcdole.kivo.benchmark

import android.content.Intent
import android.os.SystemClock
import android.view.InputDevice
import android.view.KeyCharacterMap
import android.view.KeyEvent
import androidx.benchmark.macro.MacrobenchmarkScope
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.Until
import java.util.regex.Pattern

// Browsing with the D-pad, shared by BrowseBenchmark, which measures it, and
// BaselineProfileGenerator, which records the code it runs.

internal const val PACKAGE = "com.mmcdole.kivo"
private const val ACTIVITY = "com.mmcdole.kivo.android.MainActivity"
private val CARD: Pattern = Pattern.compile("card:.*")
private const val HOLD_MS = 3_000L
private const val FIRST_REPEAT_MS = 400L
private const val REPEAT_MS = 50L
private const val SETTLE_MS = 1_500L
private const val TIMEOUT_MS = 15_000L
private const val FOCUS_WAIT_MS = 1_000L
private const val PRESS_WAIT_MS = 4_000L
private const val RAIL_ITEMS = 20

/** The library named by the `library` instrumentation argument (its name on the rail), Movies when not given. */
internal fun libraryArgument(): String = InstrumentationRegistry.getArguments().getString("library") ?: "Movies"

/** Starts Home afresh, signed in to the device's server, and waits for its first card to take focus. */
internal fun MacrobenchmarkScope.openHome() {
    startActivityAndWait(
        Intent(Intent.ACTION_MAIN).setClassName(PACKAGE, ACTIVITY)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK),
    )
    check(device.wait(Until.hasObject(By.focused(true).res(CARD)), TIMEOUT_MS)) {
        device.executeShellCommand("screencap -p /data/local/tmp/kivo-home-failed.png")
        "Home showed no focused card"
    }
    settle()
}

/**
 * From Home's first card: Left opens the rail, then Down along it to [library] and OK opens it.
 * Focus can take over a second to show a press on a slow device, and pressing again then skips
 * an item, so each press waits for focus to move, and it goes back Up if it still overshoots.
 */
internal fun MacrobenchmarkScope.openLibrary(library: String) {
    val item = "rail:$library"
    device.executeShellCommand("input keyevent KEYCODE_DPAD_LEFT")
    check(device.wait(Until.hasObject(By.res(item)), TIMEOUT_MS)) { "The rail has no $library" }
    val seen = mutableListOf<String?>()
    for (key in listOf("KEYCODE_DPAD_DOWN", "KEYCODE_DPAD_UP", "KEYCODE_DPAD_DOWN")) {
        for (step in 0 until RAIL_ITEMS) {
            if (device.wait(Until.hasObject(By.focused(true).res(item)), FOCUS_WAIT_MS)) {
                device.pressDPadCenter()
                check(device.wait(Until.hasObject(By.focused(true).res(CARD)), TIMEOUT_MS)) { "$library showed no focused card" }
                settle()
                return
            }
            val now = device.findObject(By.focused(true))?.resourceName
            if (key == "KEYCODE_DPAD_DOWN" && now == "rail:Settings" || key == "KEYCODE_DPAD_UP" && now == "rail:Home") break
            seen += now
            device.executeShellCommand("input keyevent $key")
            if (now != null) device.wait(Until.gone(By.focused(true).res(now)), PRESS_WAIT_MS)
        }
    }
    error("Focus never reached $library on the rail; focus went $seen")
}

/** Holds [code] for a few seconds, repeating every 50 ms as a remote does, then lets go. */
internal fun hold(code: Int) {
    val automation = InstrumentationRegistry.getInstrumentation().uiAutomation
    fun send(action: Int, repeat: Int, down: Long) = automation.injectInputEvent(
        KeyEvent(
            down, SystemClock.uptimeMillis(), action, code, repeat, 0,
            KeyCharacterMap.VIRTUAL_KEYBOARD, 0, if (repeat == 1) KeyEvent.FLAG_LONG_PRESS else 0,
            InputDevice.SOURCE_DPAD,
        ),
        true,
    )
    val down = SystemClock.uptimeMillis()
    var repeat = 0
    while (SystemClock.uptimeMillis() - down < HOLD_MS) {
        send(KeyEvent.ACTION_DOWN, repeat++, down)
        SystemClock.sleep(if (repeat == 1) FIRST_REPEAT_MS else REPEAT_MS)
    }
    send(KeyEvent.ACTION_UP, 0, down)
}

/** Waits out a scroll's end and the artwork loading where it stopped. */
internal fun MacrobenchmarkScope.settle() {
    SystemClock.sleep(SETTLE_MS)
    device.waitForIdle()
}
