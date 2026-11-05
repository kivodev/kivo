package com.mmcdole.kivo.android.library

import android.view.KeyEvent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
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
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.nativeKeyCode
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.mmcdole.kivo.android.ui.CardGap
import com.mmcdole.kivo.android.ui.CardShape
import com.mmcdole.kivo.android.ui.FocusBleed
import com.mmcdole.kivo.android.ui.LandscapeColumns
import com.mmcdole.kivo.android.ui.LandscapeGridWidth
import com.mmcdole.kivo.android.ui.LocalArtworkShown
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.PosterColumns
import com.mmcdole.kivo.android.ui.PosterWidth
import com.mmcdole.kivo.android.ui.SafeEdge
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.android.ui.WorkCard
import com.mmcdole.kivo.android.ui.bleed
import com.mmcdole.kivo.android.ui.focusKey
import com.mmcdole.kivo.android.ui.focusWhenPlaced
import com.mmcdole.kivo.android.ui.prepareCard
import com.mmcdole.kivo.android.ui.rememberFocusMemory
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
fun gridCardWidth(shape: CardShape) = if (shape == CardShape.Landscape) LandscapeGridWidth else PosterWidth

/** Where a grid's first line sits under what's above it. */
val GridTop = 20.dp

/** A paged grid of works' cards, each opening its work's page (see [PagedCards]). */
@Composable
fun PagedGrid(
    paged: GridPager<WorkSummary>,
    grid: LazyListState,
    shape: CardShape,
    navigator: Navigator,
    modifier: Modifier = Modifier,
    takesFocus: () -> Boolean = { true },
) = PagedCards(paged, grid, shape, { it.id }, modifier, takesFocus, prepare = { prepareCard(it, shape) }) { work, width, cardModifier ->
    WorkCard(work, shape, width, { if (work != null) navigator.push(Screen.Work(work.id)) }, cardModifier)
}

/**
 * A paged grid's cards, a line of them to each item of a list: the list
 * lays out a whole line as focus moves past those on screen, so Up and Down
 * find the card above or below even before its line has scrolled into view.
 * As it's shown it focuses the card focused last, else the first, unless
 * [takesFocus] says to leave focus where it is. [card] draws an item, null
 * while its page is read; [prepare] readies an item's card off the main
 * thread before it shows.
 */
@Composable
fun <T : Any> PagedCards(
    paged: GridPager<T>,
    grid: LazyListState,
    shape: CardShape,
    key: (T) -> String,
    modifier: Modifier = Modifier,
    takesFocus: () -> Boolean = { true },
    prepare: (T) -> Unit = {},
    card: @Composable (item: T?, width: Dp, modifier: Modifier) -> Unit,
) {
    val pages by paged.state.collectAsState()
    val total = pages.total ?: 0
    val columns = gridColumns(shape)
    val width = gridCardWidth(shape)
    val lines = (total + columns - 1) / columns
    val memory = rememberFocusMemory()
    val scope = rememberCoroutineScope()
    var focused by remember { mutableIntStateOf(0) }

    // Channel Up and Down page the grid, and focus goes as far, to the card in the same place on the next page.
    fun page(step: Int) {
        val page = grid.layoutInfo.visibleItemsInfo.size
        val column = focused % columns
        val target = ((focused / columns + step * page).coerceIn(0, lines - 1) * columns + column).coerceAtMost(total - 1)
        scope.launch {
            grid.scrollToItem((grid.firstVisibleItemIndex + step * page).coerceIn(0, lines - 1))
            focusWhenPlaced { paged.state.value.item(target)?.let(key)?.let(memory::requester) }
        }
    }

    // Up and Down move focus a line at most each frame, so a held key draws every line it passes and focus never
    // runs ahead of the screen: a press or repeat that comes while the last move waits to be drawn is dropped.
    var drawing by remember { mutableStateOf(false) }
    fun dropMove(): Boolean {
        if (drawing) return true
        drawing = true
        scope.launch {
            withFrameNanos {}
            drawing = false
        }
        return false
    }

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

    LazyColumn(
        state = grid,
        modifier = modifier
            .fillMaxSize()
            .bleed()
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
        contentPadding = PaddingValues(start = FocusBleed, top = GridTop, bottom = SafeEdge, end = FocusBleed),
        verticalArrangement = Arrangement.spacedBy(24.dp),
    ) {
        items(count = lines) { line ->
            // A line's artwork loads once it's on screen and the grid has settled, or it has stayed on screen a
            // while, so lines that scroll past aren't fetched and decoded.
            val settled = remember { mutableStateOf(false) }
            LaunchedEffect(Unit) {
                snapshotFlow { grid.layoutInfo.visibleItemsInfo.any { it.index == line } }.first { it }
                withTimeoutOrNull(ArtworkDwellMs) { grid.awaitSettled() }
                settled.value = true
            }
            CompositionLocalProvider(LocalArtworkShown provides settled) {
                Row(horizontalArrangement = Arrangement.spacedBy(CardGap)) {
                    for (index in line * columns until line * columns + columns) {
                        val item = pages.item(index)
                        // A card keeps its width whether or not the letter rail narrows the grid, centred in its column.
                        Box(Modifier.weight(1f), contentAlignment = Alignment.TopCenter) {
                            if (index < total) {
                                card(
                                    item,
                                    width,
                                    Modifier
                                        .onFocusChanged { if (it.isFocused) focused = index }
                                        .then(if (item != null) Modifier.focusKey(memory, key(item)) else Modifier),
                                )
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

/** How long a line stays on screen while the grid scrolls before its artwork loads. */
private const val ArtworkDwellMs = 300L

/** Waits until the list has stopped scrolling for a moment, rather than between one move and the next. */
private suspend fun LazyListState.awaitSettled() {
    while (true) {
        snapshotFlow { isScrollInProgress }.first { !it }
        withTimeoutOrNull(50) { snapshotFlow { isScrollInProgress }.first { it } } ?: return
    }
}
