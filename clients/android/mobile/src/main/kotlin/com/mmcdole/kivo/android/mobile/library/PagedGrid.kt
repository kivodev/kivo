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
import com.mmcdole.kivo.android.mobile.ui.CardShape
import com.mmcdole.kivo.core.data.GridPager
import com.mmcdole.kivo.core.data.GridPages
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map

/** The gap between a grid's cards, across and down. */
val GridGap = 10.dp
val GridLineGap = 16.dp

/** Cards across [width] for [shape]: landscape at least 170 dp wide and two across, the rest 104 dp and three. */
fun gridColumns(width: Dp, shape: CardShape): Int =
    if (shape == CardShape.Landscape) maxOf(2, ((width + GridGap) / (170.dp + GridGap)).toInt()) else maxOf(3, ((width + GridGap) / (104.dp + GridGap)).toInt())

/** How far past the screen the pages of a grid are read: lines ahead of a fling, and behind. */
private const val LinesAhead = 8
private const val LinesBehind = 4

/**
 * A grid too long to read at once, paged by position: a cell for each of
 * the pager's items, [cell] drawing one, null while its page is read. The
 * pages of the lines in view and some either side are asked for as they
 * come into view. Cells are keyed by position, which is what a page fills
 * in, so a page arriving redraws its own cells in place.
 */
@Composable
fun <T : Any> PagedGrid(
    pager: GridPager<T>,
    state: LazyGridState,
    columns: Int,
    contentPadding: PaddingValues,
    modifier: Modifier = Modifier,
    cell: @Composable (index: Int, item: T?) -> Unit,
) {
    val pages: GridPages<T> by pager.state.collectAsStateWithLifecycle()
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
        horizontalArrangement = Arrangement.spacedBy(GridGap),
        verticalArrangement = Arrangement.spacedBy(GridLineGap),
    ) {
        items(count = pages.total ?: 0, key = { it }, contentType = { 0 }) { index ->
            cell(index, pages.item(index))
        }
    }
}
