@file:OptIn(ExperimentalFoundationApi::class)

package com.mmcdole.kivo.android.ui

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.gestures.BringIntoViewSpec
import androidx.compose.foundation.gestures.LocalBringIntoViewSpec
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.relocation.BringIntoViewResponder
import androidx.compose.foundation.relocation.bringIntoViewResponder
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.snapshotFlow
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.Dp
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull

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

/** Scrolls only as far as it takes to keep what takes focus [start] and [end] clear of the list's ends. */
internal class Clear(private val start: Float, private val end: Float) : BringIntoViewSpec {
    override fun calculateScrollDistance(offset: Float, size: Float, containerSize: Float): Float = when {
        offset < start -> offset - start
        offset + size > containerSize - end -> offset + size - (containerSize - end)
        else -> 0f
    }
}

/**
 * A row of cards. It scrolls only when focus moves to a card not fully on
 * screen, and then only far enough to show it, so the cards it shows stay
 * where they are as focus moves into the row from above or below. Give it
 * a place that ends at the screen's edge: the cards rest inside the screen's
 * margin and run on past it to show there's more.
 */
@Composable
fun CardRow(
    modifier: Modifier = Modifier,
    spacing: Dp = CardGap,
    state: LazyListState = rememberLazyListState(),
    content: LazyListScope.() -> Unit,
) {
    val density = LocalDensity.current
    val spec = remember(density) { with(density) { Clear(RowBleed.toPx(), SafeEdge.toPx()) } }
    CompositionLocalProvider(LocalBringIntoViewSpec provides spec) {
        LazyRow(
            state = state,
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
 * shows too. With [above], in pixels, its top stops that far lower down, so
 * what's above it shows as well; a negative [above] stops it higher up.
 */
fun Modifier.bringsTopIntoView(above: Float = 0f): Modifier = bringIntoViewResponder(
    object : BringIntoViewResponder {
        override fun calculateRectForParent(localRect: Rect): Rect = Rect(0f, -above, localRect.right, maxOf(localRect.bottom, -above))

        override suspend fun bringChildIntoView(localRect: () -> Rect?) = Unit
    },
)

/**
 * Lets a held key move focus at most once a frame, so it draws every card
 * or row it passes and focus never runs ahead of the screen. The function
 * it gives says whether to drop a press or repeat: one that comes while the
 * last move waits to be drawn is dropped.
 */
@Composable
fun rememberOneMovePerFrame(): () -> Boolean {
    val scope = rememberCoroutineScope()
    return remember(scope) {
        var drawing = false
        drop@{
            if (drawing) return@drop true
            drawing = true
            scope.launch {
                withFrameNanos {}
                drawing = false
            }
            false
        }
    }
}

/** How long cards stay on screen while a list scrolls before their artwork loads. */
const val ArtworkDwellMs = 300L

/** Waits until the list has stopped scrolling for a moment, rather than between one move and the next. */
suspend fun LazyListState.awaitSettled() = awaitSettled { isScrollInProgress }

/** Waits until [scrolling] has been false for a moment, rather than between one move and the next. */
suspend fun awaitSettled(scrolling: () -> Boolean) {
    while (true) {
        snapshotFlow { scrolling() }.first { !it }
        withTimeoutOrNull(50) { snapshotFlow { scrolling() }.first { it } } ?: return
    }
}
