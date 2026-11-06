package com.mmcdole.kivo.benchmark

import android.os.Build
import android.view.KeyEvent
import androidx.benchmark.macro.junit4.BaselineProfileRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * The release build's profiles, recorded from its nonMinifiedRelease build on
 * a device signed in to a server: [startup], the code from launch to Home's
 * first focused card, is the startup profile, by which R8 lays out the first
 * dex file; [browse], the journeys BrowseBenchmark measures, is the baseline
 * profile, compiled ahead of time on install.
 *
 * Each collection resets the app's compiled code first. From Android 14 that
 * clears its profiles and keeps its data; before, it uninstalls and reinstalls
 * the app, which signs it out, so it refuses to run there unless
 *   -e allowReinstall true
 * says the device's sign-in can go (an emulator, or a rooted device, where it
 * resets without reinstalling). Android 12L and older can't record a profile
 * without root at all.
 */
@RunWith(AndroidJUnit4::class)
class BaselineProfileGenerator {
    @get:Rule
    val rule = BaselineProfileRule()

    private val library = libraryArgument()

    @Before
    fun keepsTheSignIn() {
        val allowReinstall = InstrumentationRegistry.getArguments().getString("allowReinstall") == "true"
        check(Build.VERSION.SDK_INT >= 34 || allowReinstall) {
            "Below Android 14 recording a profile reinstalls the app, which signs it out; pass -e allowReinstall true to go ahead"
        }
    }

    @Test
    fun startup() = rule.collect(PACKAGE, maxIterations = MAX_ITERATIONS, includeInStartupProfile = true) {
        openHome()
    }

    @Test
    fun browse() = rule.collect(PACKAGE, maxIterations = MAX_ITERATIONS) {
        openHome()
        hold(KeyEvent.KEYCODE_DPAD_RIGHT)
        settle()
        openHome()
        hold(KeyEvent.KEYCODE_DPAD_DOWN)
        settle()
        openHome()
        openLibrary(library)
        hold(KeyEvent.KEYCODE_DPAD_DOWN)
        settle()
    }

    private companion object {
        const val MAX_ITERATIONS = 8
    }
}
