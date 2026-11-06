package com.mmcdole.kivo.benchmark

import android.view.KeyEvent
import androidx.benchmark.macro.CompilationMode
import androidx.benchmark.macro.ExperimentalMacrobenchmarkApi
import androidx.benchmark.macro.FrameTimingMetric
import androidx.benchmark.macro.MacrobenchmarkScope
import androidx.benchmark.macro.junit4.MacrobenchmarkRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Browsing with a held D-pad, as someone looking for something to watch does:
 * Right along Home's first row, Down through Home's rows, and Down through a
 * library's grid, each held for a few seconds at a remote's 50 ms repeat and
 * measured until the screen settles, so the artwork loading where it stops
 * counts too. Each iteration starts Home afresh, signed in to the device's
 * server, and the library is the one named by
 *   -e library <its name on the rail>
 * (Movies when not given).
 */
@OptIn(ExperimentalMacrobenchmarkApi::class)
@RunWith(AndroidJUnit4::class)
class BrowseBenchmark {
    @get:Rule
    val rule = MacrobenchmarkRule()

    private val library = libraryArgument()

    @Test
    fun homeHoldRight() = measure(setup = { openHome() }) { hold(KeyEvent.KEYCODE_DPAD_RIGHT) }

    @Test
    fun homeHoldDown() = measure(setup = { openHome() }) { hold(KeyEvent.KEYCODE_DPAD_DOWN) }

    @Test
    fun libraryHoldDown() = measure(setup = { openHome(); openLibrary(library) }) { hold(KeyEvent.KEYCODE_DPAD_DOWN) }

    private fun measure(setup: MacrobenchmarkScope.() -> Unit, held: MacrobenchmarkScope.() -> Unit) = rule.measureRepeated(
        packageName = PACKAGE,
        metrics = listOf(FrameTimingMetric()),
        // Ignore: any other mode resets compilation by uninstalling the app, which signs it out.
        // Compile it once beforehand: adb shell cmd package compile -f -m speed-profile com.mmcdole.kivo
        compilationMode = CompilationMode.Ignore(),
        iterations = ITERATIONS,
        setupBlock = setup,
    ) {
        held()
        settle()
    }

    private companion object {
        const val ITERATIONS = 5
    }
}
