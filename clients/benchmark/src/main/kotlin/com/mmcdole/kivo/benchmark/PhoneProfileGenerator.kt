package com.mmcdole.kivo.benchmark

import android.content.Intent
import androidx.benchmark.macro.MacrobenchmarkScope
import androidx.benchmark.macro.junit4.BaselineProfileRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.uiautomator.By
import androidx.test.uiautomator.Direction
import androidx.test.uiautomator.Until
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import java.util.regex.Pattern

/**
 * The phone's share of the release build's profiles, recorded as
 * BaselineProfileGenerator records the TV's, on a phone or phone emulator
 * signed in to a server, Android 14 or later: [startup], from launch to
 * Home's first card, and [browse], flinging along Home's first row and down
 * Home, then opening the library named by
 *   -e library <its name on Libraries>
 * (Movies when not given), flinging down its grid and opening a work's page.
 * Both are merged into the TV's profiles rather than replacing them; see
 * build.gradle.kts.
 */
@RunWith(AndroidJUnit4::class)
class PhoneProfileGenerator {
    @get:Rule
    val rule = BaselineProfileRule()

    private val library = libraryArgument()

    @Test
    fun startup() = rule.collect(PACKAGE, maxIterations = MAX_ITERATIONS, includeInStartupProfile = true) {
        openPhoneHome()
    }

    @Test
    fun browse() = rule.collect(PACKAGE, maxIterations = MAX_ITERATIONS) {
        openPhoneHome()
        // Along the first row and back, by its first card's middle, then down Home.
        val y = device.findObject(By.res(CARD)).visibleCenter.y
        val width = device.displayWidth
        repeat(2) {
            device.swipe(width * 9 / 10, y, width / 10, y, SWIPE_STEPS)
            settle()
        }
        device.swipe(width / 10, y, width * 9 / 10, y, SWIPE_STEPS)
        settle()
        device.findObject(By.res("homeRows")).fling(Direction.DOWN)
        settle()
        device.findObject(By.res("tab:Libraries")).click()
        check(device.wait(Until.hasObject(By.res("library:$library")), TIMEOUT_MS)) { "Libraries has no $library" }
        device.findObject(By.res("library:$library")).click()
        check(device.wait(Until.hasObject(By.res(CARD)), TIMEOUT_MS)) { "$library showed no card" }
        settle()
        val grid = device.findObject(By.res("grid"))
        repeat(2) {
            grid.fling(Direction.DOWN)
            settle()
        }
        grid.fling(Direction.UP)
        settle()
        device.findObject(By.res(CARD)).click()
        check(device.wait(Until.hasObject(By.res("detail")), TIMEOUT_MS)) { "The work's page didn't open" }
        settle()
        device.findObject(By.res("detail")).fling(Direction.DOWN)
        settle()
    }

    private companion object {
        const val MAX_ITERATIONS = 8
        const val TIMEOUT_MS = 15_000L
        const val SWIPE_STEPS = 8
    }
}

private val CARD: Pattern = Pattern.compile("card:.*")

/** Starts the phone's Home afresh, signed in to the device's server, and waits for its first card. */
private fun MacrobenchmarkScope.openPhoneHome() {
    startActivityAndWait(
        Intent(Intent.ACTION_MAIN).setClassName(PACKAGE, "com.mmcdole.kivo.android.MainActivity")
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK),
    )
    check(device.wait(Until.hasObject(By.res(CARD)), 15_000L)) {
        device.executeShellCommand("screencap -p /data/local/tmp/kivo-home-failed.png")
        "Home showed no card"
    }
    settle()
}
