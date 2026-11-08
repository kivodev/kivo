package com.mmcdole.kivo.android.mobile.favorites

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.grid.LazyGridState
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.app.FavoritesGridRoute
import com.mmcdole.kivo.android.mobile.app.MobileNav
import com.mmcdole.kivo.android.mobile.app.handle
import com.mmcdole.kivo.android.mobile.library.gridCardWidth
import com.mmcdole.kivo.android.mobile.library.PagedGrid
import com.mmcdole.kivo.android.mobile.library.gridColumns
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.android.mobile.ui.LocalWide
import com.mmcdole.kivo.android.mobile.ui.CardShape
import com.mmcdole.kivo.android.mobile.ui.Failed
import com.mmcdole.kivo.android.mobile.ui.Loading
import com.mmcdole.kivo.android.mobile.ui.Message
import com.mmcdole.kivo.android.mobile.ui.PageTopBar
import com.mmcdole.kivo.android.mobile.ui.TabTopBar
import com.mmcdole.kivo.android.mobile.ui.WorkCard
import com.mmcdole.kivo.android.mobile.ui.WorkMenu
import com.mmcdole.kivo.android.mobile.ui.WorkMenuHost
import com.mmcdole.kivo.android.mobile.ui.rowCardWidth
import kotlinx.coroutines.launch

/** A kind's cards: posters for movies, shows, seasons and episodes, square books, landscape videos. */
fun favoriteShape(kind: String): CardShape = when (kind) {
    "video", "extra" -> CardShape.Landscape
    "book" -> CardShape.Square
    else -> CardShape.Poster
}

/** Favorites: a row of each kind with favorites, each heading opening the kind's grid. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun FavoritesScreen(session: ServerSession, nav: MobileNav) {
    val model = viewModel { FavoritesViewModel(session.session) }
    val state by model.state.collectAsStateWithLifecycle()
    val scroll = TopAppBarDefaults.exitUntilCollapsedScrollBehavior()
    Scaffold(
        modifier = Modifier.nestedScroll(scroll.nestedScrollConnection),
        topBar = { TabTopBar(stringResource(R.string.mobile_tab_favorites), scroll) },
    ) { padding ->
        val rows = state.rows
        val problem = state.problem
        Box(Modifier.padding(padding).fillMaxSize()) {
            when {
                rows == null && problem != null -> Failed(problem, stringResource(R.string.mobile_couldnt_load_favorites), state.serverName, model::refresh)
                rows == null -> Loading()
                else -> PullToRefreshBox(state.refreshing, model::refresh, Modifier.fillMaxSize()) {
                    WorkMenuHost({ action, work -> nav.handle(action, work, model.marks) }) { openMenu ->
                        LazyColumn(Modifier.fillMaxSize().testTag("favorites"), contentPadding = PaddingValues(bottom = space(6))) {
                            if (rows.isEmpty()) {
                                item { Message(stringResource(R.string.mobile_no_favorites), stringResource(R.string.mobile_no_favorites_sentence), Modifier.height(400.dp)) }
                            }
                            items(rows, key = { it.kind }) { row ->
                                val shape = favoriteShape(row.kind)
                                Column(Modifier.padding(top = space(4))) {
                                    Row(
                                        Modifier
                                            .fillMaxWidth()
                                            .clickable { nav.open(FavoritesGridRoute(row.kind)) }
                                            .heightIn(min = 48.dp)
                                            .padding(horizontal = space(4), vertical = space(2))
                                            .semantics(mergeDescendants = true) { heading() },
                                        verticalAlignment = Alignment.CenterVertically,
                                    ) {
                                        Text(favoritesTitle(row.kind), style = MaterialTheme.typography.titleMedium)
                                        Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, null, tint = MaterialTheme.colorScheme.onSurfaceVariant)
                                    }
                                    LazyRow(contentPadding = PaddingValues(horizontal = space(4)), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                                        items(row.items, key = { it.id }) { work ->
                                            WorkCard(work, shape, rowCardWidth(shape), { nav.openWork(work) }, { openMenu(WorkMenu(work)) })
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

/** One kind's favorites as a grid, most recently marked first. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun FavoritesGridScreen(session: ServerSession, kind: String, nav: MobileNav) {
    val model = viewModel { FavoritesGridViewModel(session.session, kind) }
    val pages by model.pager.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var refreshing by remember { mutableStateOf(false) }
    val list = rememberSaveable(saver = LazyGridState.Saver) { LazyGridState() }
    Scaffold(topBar = { PageTopBar(favoritesTitle(kind), nav::back) }) { padding ->
        val shape = favoriteShape(kind)
        BoxWithConstraints(Modifier.padding(padding).fillMaxSize()) {
            val width = maxWidth - space(8)
            val wide = LocalWide.current
            val columns = gridColumns(width, shape, wide)
            val cardWidth = gridCardWidth(width, columns, wide)
            val problem = pages.problem
            when {
                problem != null -> Failed(problem, stringResource(R.string.mobile_couldnt_load_favorites), "", { scope.launch { model.refresh() } })
                pages.total == null -> Loading()
                else -> PullToRefreshBox(refreshing, { scope.launch { refreshing = true; model.refresh(); refreshing = false } }, Modifier.fillMaxSize()) {
                    WorkMenuHost({ action, work -> nav.handle(action, work, model.marks) }) { openMenu ->
                        PagedGrid(model.pager, list, columns, PaddingValues(horizontal = space(4), vertical = space(2))) { _, work ->
                            WorkCard(work, shape, cardWidth, { if (work != null) nav.openWork(work) }, { if (work != null) openMenu(WorkMenu(work)) }, captionLines = 1)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun favoritesTitle(kind: String): String = stringResource(
    when (kind) {
        "movie" -> R.string.mobile_movies
        "series" -> R.string.mobile_shows
        "episode" -> R.string.mobile_episodes
        "book" -> R.string.mobile_books
        "season" -> R.string.mobile_seasons
        "extra" -> R.string.mobile_extras
        else -> R.string.mobile_videos
    },
)
