@file:OptIn(ExperimentalFoundationApi::class)

package com.mmcdole.kivo.android.ui

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.gestures.BringIntoViewSpec
import androidx.compose.foundation.gestures.LocalBringIntoViewSpec
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.relocation.BringIntoViewResponder
import androidx.compose.foundation.relocation.bringIntoViewResponder
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.Dp

// Rows hold their place: the focused row stays at one height on the screen
// while the rows move under it. Along a row, the cards move only as far as
// it takes to show the next one, so Up and Down land on the card above or
// below as the row stands.

/** Scrolls whatever takes focus to [pivot] from the start, as far as the list goes. */
private class Pivot(private val pivot: Float) : BringIntoViewSpec {
    override fun calculateScrollDistance(offset: Float, size: Float, containerSize: Float): Float = offset - pivot
}

/** Lists in [content] scroll only as far as it takes to show what takes focus, as a list of choices does. */
@Composable
fun KeepInView(content: @Composable () -> Unit) {
    CompositionLocalProvider(LocalBringIntoViewSpec provides JustEnough, content = content)
}

private val JustEnough = object : BringIntoViewSpec {}

/**
 * Lists in [content] keep a focused row's cards [pivot] from their top:
 * a screen's rows, where that's just under the row's title.
 */
@Composable
fun RowsHoldPlace(pivot: Dp, content: @Composable () -> Unit) {
    val px = with(LocalDensity.current) { pivot.toPx() }
    val spec = remember(px) { Pivot(px) }
    CompositionLocalProvider(LocalBringIntoViewSpec provides spec, content = content)
}

/** Where a row's cards sit under its title: the title's line, its gap and the room for focus. */
val RowCardsTop = Dp(22f) + Dp(12f) + FocusBleed

/** Scrolls only as far as it takes to keep what takes focus [margin] clear of the list's ends. */
private class Clear(private val margin: Float) : BringIntoViewSpec {
    override fun calculateScrollDistance(offset: Float, size: Float, containerSize: Float): Float = when {
        offset < margin -> offset - margin
        offset + size > containerSize - margin -> offset + size - (containerSize - margin)
        else -> 0f
    }
}

/**
 * A row of cards. It scrolls only when focus moves to a card not fully on
 * screen, and then only far enough to show it, so the cards it shows stay
 * where they are as focus moves into the row from above or below.
 */
@Composable
fun CardRow(
    modifier: Modifier = Modifier,
    spacing: Dp = CardGap,
    content: LazyListScope.() -> Unit,
) {
    val margin = with(LocalDensity.current) { FocusBleed.toPx() }
    val spec = remember(margin) { Clear(margin) }
    CompositionLocalProvider(LocalBringIntoViewSpec provides spec) {
        LazyRow(
            modifier = modifier.bleed(),
            horizontalArrangement = Arrangement.spacedBy(spacing),
            contentPadding = RowPadding,
            content = content,
        )
    }
}

/**
 * Focus anywhere in this part of a list, such as a detail page's buttons,
 * brings the whole part into view from its top, so the title above them
 * shows too.
 */
fun Modifier.bringsTopIntoView(): Modifier = bringIntoViewResponder(
    object : BringIntoViewResponder {
        override fun calculateRectForParent(localRect: Rect): Rect = Rect(0f, 0f, localRect.right, localRect.bottom)

        override suspend fun bringChildIntoView(localRect: () -> Rect?) = Unit
    },
)
