package com.mmcdole.kivo.android.favorites

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
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
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.ui.CardGap
import com.mmcdole.kivo.android.ui.Failed
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.SafeTop
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

/** Where the grid sits beside the rail, as a library's does. */
private val GridPadding = PaddingValues(start = 24.dp, top = SafeTop, end = 12.dp)

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
        Column(Modifier.fillMaxSize().padding(GridPadding)) {
            GridHeading(kind)
            if (total == 0) {
                FavoritesEmpty()
            } else {
                Text(
                    favoritesCount(kind, total),
                    style = TvType.body,
                    color = Kivo.colors.textMuted,
                    modifier = Modifier.padding(top = Kivo.space(2)),
                )
                val grid = rememberSaveable(saver = LazyListState.Saver) { LazyListState() }
                PagedGrid(model.paged, grid, shape, navigator)
            }
        }
    }
}

/** "Favorites · Movies", at a library's title's height. */
@Composable
private fun GridHeading(kind: String) {
    Box(Modifier.height(44.dp), contentAlignment = Alignment.CenterStart) {
        Text(
            stringResource(R.string.favorites_of, favoritesTitle(kind)),
            style = TvType.heading,
            color = Kivo.colors.text,
        )
    }
}

/** The grid while its first page loads: the title, the count line and a line of cards. */
@Composable
private fun GridSkeleton(kind: String) {
    val shape = favoriteShape(kind)
    Column(Modifier.fillMaxSize().padding(GridPadding)) {
        GridHeading(kind)
        SkeletonLine(180.dp, TvType.body, Modifier.padding(top = Kivo.space(2)))
        Row(Modifier.padding(top = GridTop), horizontalArrangement = Arrangement.spacedBy(CardGap)) {
            repeat(gridColumns(shape)) { SkeletonCard(shape, gridCardWidth(shape)) }
        }
    }
}
