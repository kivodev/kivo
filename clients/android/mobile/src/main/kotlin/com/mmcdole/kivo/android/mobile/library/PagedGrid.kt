package com.mmcdole.kivo.android.mobile.library

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyGridState
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mmcdole.kivo.android.mobile.theme.KivoTokens
import com.mmcdole.kivo.android.mobile.ui.CardShape
import com.mmcdole.kivo.android.mobile.ui.LocalWide
import com.mmcdole.kivo.core.data.GridItem
import com.mmcdole.kivo.core.data.GridPager
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map

/** The gap between a grid's cards across, a [wide] window's the shared tokens' gap, and down. */
fun gridGap(wide: Boolean): Dp = if (wide) KivoTokens.Layout.gridGap else 10.dp
val GridLineGap = 16.dp

/**
 * Cards across [width] for [shape]: landscape at least 170 dp wide and two
 * across, the rest 104 dp and three; in a [wide] window, as many as fit at
 * the shared tokens' widths, so a tablet has more columns rather than bigger cards.
 */
fun gridColumns(width: Dp, shape: CardShape, wide: Boolean = false): Int {
    val gap = gridGap(wide)
    val (fewest, narrowest) = when {
        shape == CardShape.Landscape -> 2 to if (wide) KivoTokens.Layout.landscapeMinWidth else 170.dp
        else -> 3 to if (wide) KivoTokens.Layout.posterMinWidth else 104.dp
    }
    return maxOf(fewest, ((width + gap) / (narrowest + gap)).toInt())
}

/** A card's width in a grid of [columns] across [width]. */
fun gridCardWidth(width: Dp, columns: Int, wide: Boolean): Dp = (width - gridGap(wide) * (columns - 1)) / columns

/** How far past the screen the pages of a grid are read: lines ahead of a fling, and behind. */
private const val LinesAhead = 8
private const val LinesBehind = 4

/**
 * How long a phone's grid holds still before the pages where it rests are
 * read, so a fling or a letter jump reads none of the pages it passes.
 */
const val GridSettleMs = 150L

/**
 * A grid too long to read at once, paged by position: a cell for each of
 * the pager's items, [cell] drawing one, null while its page is read. The
 * pages of the lines in view and some either side are asked for as they
 * come into view, which the pager reads once the grid has held still. Cells
 * are keyed by position, which is what a page fills in, so a page arriving
 * redraws its own cells in place.
 */
@Composable
fun PagedGrid(
    pager: GridPager,
    state: LazyGridState,
    columns: Int,
    contentPadding: PaddingValues,
    modifier: Modifier = Modifier,
    cell: @Composable (index: Int, item: GridItem?) -> Unit,
) {
    val pages by pager.state.collectAsStateWithLifecycle()
    LaunchedEffect(pager, columns) {
        val visible = snapshotFlow {
            state.layoutInfo.visibleItemsInfo.let { (it.firstOrNull()?.index ?: 0)..(it.lastOrNull()?.index ?: 0) }
        }.distinctUntilChanged()
        combine(visible, pager.state.map { it.total }.distinctUntilChanged()) { range, _ -> range }.collect { range ->
            pager.load((range.first - LinesBehind * columns).coerceAtLeast(0), range.last + LinesAhead * columns)
        }
    }
    LazyVerticalGrid(
        columns = GridCells.Fixed(columns),
        state = state,
        modifier = modifier.testTag("grid"),
        contentPadding = contentPadding,
        horizontalArrangement = Arrangement.spacedBy(gridGap(LocalWide.current)),
        verticalArrangement = Arrangement.spacedBy(GridLineGap),
    ) {
        items(count = pages.total ?: 0, key = { it }, contentType = { 0 }) { index ->
            cell(index, pages.item(index))
        }
    }
}
