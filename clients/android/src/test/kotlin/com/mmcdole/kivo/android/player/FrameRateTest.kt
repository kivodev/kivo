package com.mmcdole.kivo.android.player

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class FrameRateTest {
    private val hz60 = DisplayModeInfo(1, 3840, 2160, 60f)
    private val hz50 = DisplayModeInfo(2, 3840, 2160, 50f)
    private val hz23 = DisplayModeInfo(3, 3840, 2160, 23.976025f)
    private val hz24 = DisplayModeInfo(4, 3840, 2160, 24f)
    private val hz47 = DisplayModeInfo(5, 3840, 2160, 47.95205f)
    private val hd23 = DisplayModeInfo(6, 1920, 1080, 23.976025f)
    private val modes = listOf(hz60, hz50, hz23, hz24, hz47, hd23)

    @Test
    fun filmSwitchesToItsOwnRateAtTheSameResolution() {
        assertEquals(hz23, matchingMode(modes, hz60, 24000f / 1001))
        assertEquals(hz24, matchingMode(modes, hz60, 24f))
    }

    @Test
    fun aWholeMultipleWinsWhenTheRateItselfIsMissing() {
        assertEquals(hz47, matchingMode(listOf(hz60, hz47), hz60, 24000f / 1001))
    }

    @Test
    fun aModeThatAlreadyMatchesStays() {
        assertNull(matchingMode(modes, hz50, 25f))
        assertNull(matchingMode(modes, hz60, 30f))
        assertNull(matchingMode(modes, hz23, 24000f / 1001))
    }

    @Test
    fun nothingMatchingOrNoRateLeavesTheMode() {
        assertNull(matchingMode(listOf(hz60, hd23), hz60, 24000f / 1001))
        assertNull(matchingMode(modes, hz60, 0f))
    }
}
