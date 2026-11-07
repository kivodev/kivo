package com.mmcdole.kivo.android.ui

import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.LayoutDirection
import androidx.compose.ui.unit.dp
import kotlin.test.Test
import kotlin.test.assertEquals

class MenuAboveTest {
    private val screen = DpSize(960.dp, 540.dp)

    @Test
    fun aMenuStandsClearOfTheButtonsRowAtTheButtonsEnd() {
        val margin = menuAbove(rowTop = 464.dp, end = 860.dp, screen = screen)
        assertEquals(84.dp, margin.calculateBottomPadding())
        assertEquals(100.dp, margin.calculateRightPadding(LayoutDirection.Ltr))
        assertEquals(SafeTop, margin.calculateTopPadding())
    }

    @Test
    fun aButtonAtTheScreensEdgeKeepsTheMenuInsideTheSafeMargin() {
        val margin = menuAbove(rowTop = 464.dp, end = 940.dp, screen = screen)
        assertEquals(SafeEdge, margin.calculateRightPadding(LayoutDirection.Ltr))
    }
}
