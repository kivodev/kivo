package com.mmcdole.kivo.android.favorites

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.R
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.library.GridTop
import com.mmcdole.kivo.android.library.PagedGrid
import com.mmcdole.kivo.android.library.gridCardWidth
import com.mmcdole.kivo.android.library.gridColumns
import com.mmcdole.kivo.android.library.scrollsWith
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.ui.CardGap
import com.mmcdole.kivo.android.ui.Failed
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.SafeEdge
import com.mmcdole.kivo.android.ui.SkeletonCard
import com.mmcdole.kivo.android.ui.SkeletonGate
import com.mmcdole.kivo.android.ui.SkeletonLine
import kotlinx.coroutines.launch

/** One kind's favorites as a grid, its pages read again in place as the user's state changes. */
class FavoritesGridViewModel(session: ServerSession, kind: String) : ViewModel() {
    val paged = session.session.library.pagedFavorites(kind)

    /** Reads the grid again, after its first page failed. */
    fun retry() {
        viewModelScope.launch { paged.refresh() }
    }

    override fun onCleared() {
        paged.close()
    }
}

/** Where the title and count line sit, as a library's do. */
private val HeadingPadding = PaddingValues(start = 24.dp, top = 39.5.dp, end = SafeEdge)

/**
 * Every favorite of one kind, most recently marked first: the
 * title, the count line, "12 movies", and the grid of a library, without
 * its sort, filters or letter rail; with none left, the title and the
 * empty state.
 */
@Composable
fun FavoritesGridScreen(session: ServerSession, kind: String, navigator: Navigator) {
    val model = viewModel(key = kind) { FavoritesGridViewModel(session, kind) }
    val pages by model.paged.state.collectAsStateWithLifecycle()
    val failed = pages.problem
    if (failed != null) {
        Failed(failed, session.session.baseUrl, model::retry, session.graph::changeServer)
        return
    }
    val shape = favoriteShape(kind)
    SkeletonGate(pages.total, { GridSkeleton(kind) }) { total ->
        if (total == 0) {
            Column(Modifier.fillMaxSize().padding(HeadingPadding)) {
                GridHeading(kind)
                FavoritesEmpty()
            }
        } else {
            val grid = rememberSaveable(saver = LazyListState.Saver) { LazyListState() }
            Box(Modifier.fillMaxSize()) {
                PagedGrid(model.paged, grid, shape, navigator)
                // The title and count line scroll away with the grid.
                Column(Modifier.scrollsWith(grid).padding(HeadingPadding)) {
                    GridHeading(kind)
                    Text(favoritesCount(kind, total), style = TvType.small, color = Kivo.colors.textMuted, modifier = Modifier.padding(top = 13.dp))
                }
            }
        }
    }
}

/** "Favorites · Movies". */
@Composable
private fun GridHeading(kind: String) {
    Text(stringResource(R.string.favorites_of, favoritesTitle(kind)), style = TvType.title, color = Kivo.colors.text)
}

/** The grid while its first page loads: the title, the count line and a line of cards. */
@Composable
private fun GridSkeleton(kind: String) {
    val shape = favoriteShape(kind)
    Box(Modifier.fillMaxSize()) {
        Column(Modifier.padding(HeadingPadding)) {
            GridHeading(kind)
            SkeletonLine(180.dp, TvType.small, Modifier.padding(top = 13.dp))
        }
        Row(Modifier.padding(start = 24.dp, top = GridTop), horizontalArrangement = Arrangement.spacedBy(CardGap)) {
            repeat(gridColumns(shape)) { SkeletonCard(shape, gridCardWidth(shape)) }
        }
    }
}
