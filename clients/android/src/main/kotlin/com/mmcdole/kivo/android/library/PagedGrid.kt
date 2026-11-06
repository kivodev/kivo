@file:OptIn(ExperimentalFoundationApi::class)

package com.mmcdole.kivo.android.library

import android.view.KeyEvent
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.gestures.BringIntoViewSpec
import androidx.compose.foundation.gestures.LocalBringIntoViewSpec
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.nativeKeyCode
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import com.mmcdole.kivo.android.ui.ArtworkDwellMs
import com.mmcdole.kivo.android.ui.CardGap
import com.mmcdole.kivo.android.ui.CaptionGap
import com.mmcdole.kivo.android.ui.CardShape
import com.mmcdole.kivo.android.ui.FocusMemory
import com.mmcdole.kivo.android.ui.GridPosterWidth
import com.mmcdole.kivo.android.ui.LandscapeColumns
import com.mmcdole.kivo.android.ui.LandscapeGridWidth
import com.mmcdole.kivo.android.ui.LocalArtworkShown
import com.mmcdole.kivo.android.ui.LocalCaptionsShown
import com.mmcdole.kivo.android.ui.LocalCardLight
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.PosterColumns
import com.mmcdole.kivo.android.ui.RowBleed
import com.mmcdole.kivo.android.ui.SafeEdge
import com.mmcdole.kivo.android.ui.SafeTop
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.android.ui.WorkCard
import com.mmcdole.kivo.android.ui.awaitSettled
import com.mmcdole.kivo.android.ui.focusKey
import com.mmcdole.kivo.android.ui.focusWhenPlaced
import com.mmcdole.kivo.android.ui.prepareCard
import com.mmcdole.kivo.android.ui.raisedOnFocus
import com.mmcdole.kivo.android.ui.rememberFocusMemory
import com.mmcdole.kivo.android.ui.rememberCardLight
import com.mmcdole.kivo.android.ui.rememberOneMovePerFrame
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.data.GridPager
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull

/** Cards across a grid of [shape]. */
fun gridColumns(shape: CardShape) = if (shape == CardShape.Landscape) LandscapeColumns else PosterColumns

/** A card's width in a grid of [shape]. */
fun gridCardWidth(shape: CardShape) = if (shape == CardShape.Landscape) LandscapeGridWidth else GridPosterWidth

/** Where a grid's first line sits when only a title and a count line are above it. */
val GridTop = 112.5.dp

/** How wide a grid's cards span, from the screen's content to beside the letter rail, which it keeps with no rail there. */
internal val GridWidth = GridPosterWidth * PosterColumns + CardGap * (PosterColumns - 1)

/** The gap between a grid's lines. */
private val LineGap = 20.dp

/** How far a line stops short of the screen's top and bottom, clear of overscan. */
private val LineMargin = SafeTop

/** The two lines of a card's caption, below the card, which a line brought into view shows too. */
private val CaptionLines = CaptionGap + 32.dp

/** Scrolls only as far as it takes to keep a focused card [above] from the list's top and [below] from its bottom. */
class GridScroll(private val above: Float, private val below: Float) : BringIntoViewSpec {
    override fun calculateScrollDistance(offset: Float, size: Float, containerSize: Float): Float = when {
        offset < above -> offset - above
        offset + size > containerSize - below -> offset + size - (containerSize - below)
        else -> 0f
    }
}

/**
 * What sits above a grid in its screen scrolls with it, read as it's placed, so scrolling recomposes nothing. It
 * stays out of the grid's list, which a new sort or filter replaces, so focus stays where it is in it.
 */
fun Modifier.scrollsWith(grid: LazyListState): Modifier =
    offset { IntOffset(0, if (grid.firstVisibleItemIndex == 0) -grid.firstVisibleItemScrollOffset else -OffScreen) }

/** Further up than anything above a grid reaches. */
private const val OffScreen = 100_000

/** Scrolls [line] to the top of the screen, clear of overscan, under a grid whose first line is [top] down; the first line, to where it starts. */
suspend fun LazyListState.scrollToLine(line: Int, top: Int, margin: Int) {
    if (line == 0) scrollToItem(0) else scrollToItem(line, top - margin)
}

/** A paged grid of works' cards, each opening its work's page (see [PagedCards]). */
@Composable
fun PagedGrid(
    paged: GridPager<WorkSummary>,
    grid: LazyListState,
    shape: CardShape,
    navigator: Navigator,
    modifier: Modifier = Modifier,
    top: Dp = GridTop,
    memory: FocusMemory = rememberFocusMemory(),
    takesFocus: () -> Boolean = { true },
) = PagedCards(paged, grid, shape, { it.id }, modifier, top, memory, takesFocus, prepare = { prepareCard(it, shape) }) { work, width, cardModifier ->
    WorkCard(work, shape, width, { if (work != null) navigator.push(Screen.Work(work.id)) }, cardModifier)
}

/**
 * A paged grid's cards, a line of them to each item of a list: the list
 * lays out a whole line as focus moves past those on screen, so Up and Down
 * find the card above or below even before its line has scrolled into view.
 * As it's shown it focuses the card focused last, else the first, unless
 * [takesFocus] says to leave focus where it is. [card] draws an item, null
 * while its page is read; [prepare] readies an item's card off the main
 * thread before it shows. It fills the screen's height, its first line [top]
 * down, leaving that room for a header that [scrollsWith] it; a focused card
 * scrolls only as far as shows its line.
 */
@Composable
fun <T : Any> PagedCards(
    paged: GridPager<T>,
    grid: LazyListState,
    shape: CardShape,
    key: (T) -> String,
    modifier: Modifier = Modifier,
    top: Dp = GridTop,
    memory: FocusMemory = rememberFocusMemory(),
    takesFocus: () -> Boolean = { true },
    prepare: (T) -> Unit = {},
    card: @Composable (item: T?, width: Dp, modifier: Modifier) -> Unit,
) {
    val pages by paged.state.collectAsState()
    val total = pages.total ?: 0
    val columns = gridColumns(shape)
    val width = gridCardWidth(shape)
    val lines = (total + columns - 1) / columns
    val scope = rememberCoroutineScope()
    var focused by remember { mutableIntStateOf(0) }
    val density = LocalDensity.current
    val topPx = with(density) { top.roundToPx() }
    val marginPx = with(density) { LineMargin.roundToPx() }
    val scrolling = remember(density) { with(density) { GridScroll(LineMargin.toPx(), (LineMargin + CaptionLines).toPx()) } }

    // Channel Up and Down page the grid, and focus goes as far, to the card in the same place on the next page.
    fun page(step: Int) {
        val page = grid.layoutInfo.visibleItemsInfo.size
        val column = focused % columns
        val target = ((focused / columns + step * page).coerceIn(0, lines - 1) * columns + column).coerceAtMost(total - 1)
        scope.launch {
            grid.scrollToLine((grid.firstVisibleItemIndex + step * page).coerceIn(0, lines - 1), topPx, marginPx)
            focusWhenPlaced { paged.state.value.item(target)?.let(key)?.let(memory::requester) }
        }
    }

    // Up and Down move focus a line at most each frame, so a held key draws every line it passes and focus never
    // runs ahead of the screen: a press or repeat that comes while the last move waits to be drawn is dropped.
    val dropMove = rememberOneMovePerFrame()
    // A held key's glow would be drawn again for every card it passes, which a slow TV can't keep up with.
    val cardLight = rememberCardLight { focused }

    // The pages of the lines in view and a few either side are read as they come into view, a held key's too: a
    // page is read once, and one that scrolls away before it's read stops.
    LaunchedEffect(paged) {
        snapshotFlow { grid.layoutInfo.visibleItemsInfo.let { (it.firstOrNull()?.index ?: 0)..(it.lastOrNull()?.index ?: 0) } }
            .distinctUntilChanged()
            .collect { visible ->
                val count = paged.state.value.total ?: 0
                val near = ((visible.first - 4) * columns).coerceAtLeast(0)..((visible.last + 5) * columns - 1).coerceAtMost(count - 1)
                paged.load(near.first, near.last)
                val read = paged.state.value
                withContext(Dispatchers.Default) { near.forEach { i -> read.item(i)?.let(prepare) } }
            }
    }

    CompositionLocalProvider(LocalBringIntoViewSpec provides scrolling) {
        LazyColumn(
            state = grid,
            modifier = modifier
                .padding(start = 24.dp)
                .offset(x = -RowBleed)
                .width(GridWidth + RowBleed * 2)
                .fillMaxHeight()
                .onPreviewKeyEvent { event ->
                    if (event.type != KeyEventType.KeyDown) return@onPreviewKeyEvent false
                    when (event.key.nativeKeyCode) {
                        KeyEvent.KEYCODE_DPAD_UP, KeyEvent.KEYCODE_DPAD_DOWN -> return@onPreviewKeyEvent dropMove()
                        KeyEvent.KEYCODE_CHANNEL_DOWN, KeyEvent.KEYCODE_PAGE_DOWN -> page(1)
                        KeyEvent.KEYCODE_CHANNEL_UP, KeyEvent.KEYCODE_PAGE_UP -> page(-1)
                        else -> return@onPreviewKeyEvent false
                    }
                    true
                },
            contentPadding = PaddingValues(start = RowBleed, top = top, bottom = SafeEdge, end = RowBleed),
            verticalArrangement = Arrangement.spacedBy(LineGap),
        ) {
            items(count = lines) { line ->
                // A line's artwork loads once it's on screen and the grid has settled, or it has stayed on screen a
                // while, so lines that scroll past aren't fetched and decoded. A line built while the grid scrolls
                // has its captions then too, as measuring them costs a held key more than the rest of the line.
                val settled = remember { mutableStateOf(false) }
                val captions = remember { mutableStateOf(!grid.isScrollInProgress) }
                LaunchedEffect(Unit) {
                    snapshotFlow { grid.layoutInfo.visibleItemsInfo.any { it.index == line } }.first { it }
                    withTimeoutOrNull(ArtworkDwellMs) { grid.awaitSettled() }
                    settled.value = true
                    captions.value = true
                }
                CompositionLocalProvider(LocalArtworkShown provides settled, LocalCaptionsShown provides captions, LocalCardLight provides cardLight) {
                    // The line and the column of the focused card are raised too, so its glow lies over the lines and cards around it.
                    Row(Modifier.raisedOnFocus(), horizontalArrangement = Arrangement.spacedBy(CardGap)) {
                        for (index in line * columns until line * columns + columns) {
                            val item = pages.item(index)
                            val itemKey = item?.let(key)
                            // The same modifier while the card's item is, so a page arriving recomposes only its own cards.
                            val cardModifier = remember(index, itemKey) {
                                Modifier
                                    .onFocusChanged { if (it.isFocused) focused = index }
                                    .then(if (itemKey != null) Modifier.focusKey(memory, itemKey) else Modifier)
                            }
                            Box(Modifier.weight(1f).raisedOnFocus(), contentAlignment = Alignment.TopCenter) {
                                if (index < total) card(item, width, cardModifier)
                            }
                        }
                    }
                }
            }
        }
    }
    LaunchedEffect(Unit) {
        if (takesFocus()) memory.restore(paged.state.value.item(0)?.let(key))
    }
}
