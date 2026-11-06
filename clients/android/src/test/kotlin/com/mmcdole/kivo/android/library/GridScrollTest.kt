@file:OptIn(ExperimentalFoundationApi::class)

package com.mmcdole.kivo.android.library

import androidx.compose.foundation.ExperimentalFoundationApi
import kotlin.test.Test
import kotlin.test.assertEquals

class GridScrollTest {
    private val scroll = GridScroll(above = 16f, below = 64f)

    @Test
    fun aCardInViewStaysWhereItIs() {
        assertEquals(0f, scroll.calculateScrollDistance(offset = 151f, size = 165f, containerSize = 540f))
        assertEquals(0f, scroll.calculateScrollDistance(offset = 16f, size = 165f, containerSize = 540f))
        assertEquals(0f, scroll.calculateScrollDistance(offset = 311f, size = 165f, containerSize = 540f))
    }

    @Test
    fun aCardPastTheBottomComesUpWithItsCaptionClear() {
        assertEquals(73f, scroll.calculateScrollDistance(offset = 384f, size = 165f, containerSize = 540f))
    }

    @Test
    fun aCardPastTheTopComesDownClearOfOverscan() {
        assertEquals(-66f, scroll.calculateScrollDistance(offset = -50f, size = 165f, containerSize = 540f))
    }
}
