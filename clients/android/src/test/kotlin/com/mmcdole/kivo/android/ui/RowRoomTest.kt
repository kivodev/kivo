@file:OptIn(ExperimentalFoundationApi::class)

package com.mmcdole.kivo.android.ui

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.mmcdole.kivo.android.library.GridWidth
import com.mmcdole.kivo.android.library.LetterRailWidth
import com.mmcdole.kivo.android.main.RailWidth
import com.mmcdole.kivo.android.search.ResultLandscapeWidth
import com.mmcdole.kivo.android.search.ResultPosterWidth
import com.mmcdole.kivo.android.search.ResultsLeft
import com.mmcdole.kivo.android.theme.KivoTokens
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class RowRoomTest {
    private val scale = KivoTokens.Tv.cardFocusScale
    private val glow = KivoTokens.Tv.CardFocus.glow

    /** A screen's content starts here, and its cards stop at the screen's right margin. */
    private val contentStart = RailWidth + 24.dp
    private val contentEnd = 960.dp - SafeEdge

    /** How far a card's shadows reach past it as it rests: Compose draws each radius + spread past the card, then the layer scales it all. */
    private fun sideReach(width: Dp) = glow.maxOf { (it.radius + it.spread) * scale } + width * (scale - 1) / 2

    private fun topReach(height: Dp) = glow.maxOf { (it.radius + it.spread - it.offset.y) * scale } + height * (scale - 1) / 2

    /** How far past a row's top its scroll clip lets what's in it draw. */
    private val clipAboveRow = 30.dp

    @Test
    fun aRowHasRoomAtItsStartForTheGlowOfItsWidestCard() {
        for (width in listOf(PosterWidth, GridPosterWidth, LandscapeWidth, LandscapeGridWidth, EpisodeCardWidth, ResultPosterWidth, ResultLandscapeWidth)) {
            assertTrue(sideReach(width) <= RowBleed, "$width reaches ${sideReach(width)}")
        }
    }

    @Test
    fun aRowHasRoomAboveForTheGlowOfItsCards() {
        for (height in listOf(PosterWidth * 1.5f, LandscapeWidth * 9f / 16f)) {
            assertTrue(topReach(height) <= FocusBleed + clipAboveRow, "$height reaches ${topReach(height)}")
        }
    }

    @Test
    fun aRowsGlowStaysOnTheScreenAtItsEnd() {
        assertTrue(contentEnd + sideReach(LandscapeWidth) <= 960.dp)
    }

    @Test
    fun sixPostersOrThreeLandscapeCardsFitAcrossARow() {
        assertTrue(contentStart + PosterWidth * 6 + CardGap * 5 <= contentEnd)
        assertTrue(contentStart + LandscapeWidth * 3 + CardGap * 2 <= contentEnd)
    }

    @Test
    fun theGridAndItsLetterRailFitInsideTheMargin() {
        assertEquals(GridPosterWidth * 6 + CardGap * 5, GridWidth)
        assertEquals(LandscapeGridWidth * 3 + CardGap * 2, GridWidth)
        assertTrue(contentStart + GridWidth + LetterRailWidth <= contentEnd)
    }

    @Test
    fun fourPostersOrTwoLandscapeCardsFitAcrossSearchResults() {
        assertTrue(ResultsLeft + ResultPosterWidth * 4 + CardGap * 3 <= contentEnd)
        assertTrue(ResultsLeft + ResultLandscapeWidth * 2 + CardGap <= contentEnd)
    }

    @Test
    fun searchResultsGlowClearOfTheKeyboard() {
        assertTrue(ResultsLeft - RowBleed >= contentStart + 275.dp)
    }

    private val scroll = Clear(start = 37f, end = 48f)

    @Test
    fun aCardInViewStaysWhereItIs() {
        assertEquals(0f, scroll.calculateScrollDistance(offset = 37f, size = 110f, containerSize = 893f))
        assertEquals(0f, scroll.calculateScrollDistance(offset = 735f, size = 110f, containerSize = 893f))
    }

    @Test
    fun aCardPastTheStartLinesUpWithTheTitle() {
        assertEquals(-20f, scroll.calculateScrollDistance(offset = 17f, size = 110f, containerSize = 893f))
    }

    @Test
    fun aCardPastTheEndStopsAtTheScreensMargin() {
        assertEquals(20f, scroll.calculateScrollDistance(offset = 755f, size = 110f, containerSize = 893f))
    }
}
