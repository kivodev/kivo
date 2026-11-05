package com.mmcdole.kivo.benchmark

import android.content.Intent
import android.net.Uri
import android.os.SystemClock
import android.view.KeyEvent
import androidx.benchmark.macro.CompilationMode
import androidx.benchmark.macro.ExperimentalMetricApi
import androidx.benchmark.macro.FrameTimingMetric
import androidx.benchmark.macro.MacrobenchmarkScope
import androidx.benchmark.macro.TraceSectionMetric
import androidx.benchmark.macro.junit4.MacrobenchmarkRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.Until
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Opening Audio & Subtitles with OK, from the detail page's row and from the
 * player's button: how long from the press to the panel's first frame
 * ([PanelOpenMetric]), the frames it takes, and the app's own sections
 * within it. Each iteration closes the panel with Back and opens it again
 * in the same process, as someone choosing tracks does.
 */
@OptIn(ExperimentalMetricApi::class)
@RunWith(AndroidJUnit4::class)
class TracksPanelBenchmark {
    @get:Rule
    val rule = MacrobenchmarkRule()

    private val work: String = InstrumentationRegistry.getArguments().getString("work")
        ?: error("Pass the ID of a work with several tracks: -Pandroid.testInstrumentationRunnerArguments.work=<ID>")

    private val metrics = listOf(
        PanelOpenMetric(),
        FrameTimingMetric(),
        TraceSectionMetric("Tracks:names", TraceSectionMetric.Mode.Sum),
    )

    @Test
    fun openFromDetailPage() = rule.measureRepeated(
        packageName = PACKAGE,
        metrics = metrics,
        compilationMode = CompilationMode.Partial(),
        iterations = ITERATIONS,
        setupBlock = {
            if (opens == 0) openDetailPage() else closePanel()
            focus(DETAIL_PAGE_ROW, KeyEvent.KEYCODE_DPAD_DOWN)
        },
    ) { openPanel() }

    @Test
    fun openFromPlayer() = rule.measureRepeated(
        packageName = PACKAGE,
        metrics = metrics,
        compilationMode = CompilationMode.Partial(),
        iterations = ITERATIONS,
        setupBlock = {
            if (opens == 0) {
                openDetailPage()
                device.pressDPadCenter()
                // Paused, the controls stay up between iterations.
                check(device.wait(Until.hasObject(By.desc(PAUSE)), TIMEOUT_MS)) { "The play didn't start" }
                device.pressKeyCode(KeyEvent.KEYCODE_MEDIA_PAUSE)
                device.wait(Until.hasObject(By.desc(PLAY)), TIMEOUT_MS)
            } else {
                closePanel()
            }
            // Back hides the controls, and Down from hidden controls shows them on Audio & Subtitles.
            focus(PLAYER_BUTTON, KeyEvent.KEYCODE_BACK, KeyEvent.KEYCODE_DPAD_DOWN)
        },
    ) { openPanel() }

    // The panel's opens so far in this test.
    private var opens = 0

    private fun MacrobenchmarkScope.openDetailPage() {
        startActivityAndWait(Intent(Intent.ACTION_VIEW, Uri.parse("kivo://work/$work")).setPackage(PACKAGE))
        check(device.wait(Until.hasObject(By.text(DETAIL_PAGE_ROW)), TIMEOUT_MS)) { "The detail page has no $DETAIL_PAGE_ROW" }
    }

    private fun MacrobenchmarkScope.openPanel() {
        device.pressDPadCenter()
        check(device.wait(Until.hasObject(By.text(PANEL_HEADING)), TIMEOUT_MS)) { "Audio & Subtitles didn't open" }
        device.waitForIdle()
        opens++
    }

    private fun MacrobenchmarkScope.closePanel() {
        device.pressBack()
        device.wait(Until.gone(By.text(PANEL_HEADING)), TIMEOUT_MS)
    }

    /** Moves focus to what reads or is tagged [text], pressing [keys] in turn until it has it. */
    private fun MacrobenchmarkScope.focus(text: String, vararg keys: Int) {
        fun focused() = device.hasObject(By.focused(true).hasDescendant(By.text(text))) || device.hasObject(By.focused(true).res(text))
        if (focused()) return
        for (key in keys) {
            device.pressKeyCode(key)
            val until = SystemClock.uptimeMillis() + FOCUS_WAIT_MS
            while (SystemClock.uptimeMillis() < until) {
                if (focused()) return
                SystemClock.sleep(50)
            }
        }
        error("Nothing reading $text took focus")
    }

    private companion object {
        const val PACKAGE = "com.mmcdole.kivo"
        const val ITERATIONS = 10
        const val TIMEOUT_MS = 10_000L
        const val FOCUS_WAIT_MS = 1_000L
        const val DETAIL_PAGE_ROW = "Audio & subtitles"
        const val PLAYER_BUTTON = "Audio & Subtitles"
        const val PANEL_HEADING = "Audio"
        const val PLAY = "Play"
        const val PAUSE = "Pause"
    }
}
