package com.mmcdole.kivo.android.mobile.home

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.horizontalScroll
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
import androidx.compose.runtime.remember
import androidx.compose.runtime.snapshots.Snapshot
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
import com.mmcdole.kivo.android.library.GridValues
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.app.LibraryRoute
import com.mmcdole.kivo.android.mobile.app.MobileNav
import com.mmcdole.kivo.android.mobile.app.SettingsRoute
import com.mmcdole.kivo.android.mobile.app.handle
import com.mmcdole.kivo.android.mobile.theme.LocalKivo
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.android.mobile.ui.CONTINUE_WATCHING
import com.mmcdole.kivo.android.mobile.ui.CardCorners
import com.mmcdole.kivo.android.mobile.ui.CardShape
import com.mmcdole.kivo.android.mobile.ui.Failed
import com.mmcdole.kivo.android.mobile.ui.Message
import com.mmcdole.kivo.android.mobile.ui.TabTopBar
import com.mmcdole.kivo.android.mobile.ui.WorkCard
import com.mmcdole.kivo.android.mobile.ui.WorkMenu
import com.mmcdole.kivo.android.mobile.ui.WorkMenuHost
import com.mmcdole.kivo.android.mobile.ui.rowCardWidth
import com.mmcdole.kivo.android.mobile.ui.shapeOf
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.ui.draw.clip

/** The gap between a row's cards, and the page's side margin. */
private val CardGap = 12.dp
private val SideMargin = 16.dp

/**
 * Home: the server's rows in its order, each a row of cards. Continue
 * Watching is the frames where you stopped; a Recently Added heading opens
 * its library newest first, and Favorites' the Favorites tab. A long press
 * on a card opens its actions. Pull to refresh; the live events refresh it too.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HomeScreen(session: ServerSession, nav: MobileNav, onFavorites: () -> Unit) {
    val model = viewModel { HomeViewModel(session.session) }
    val state by model.state.collectAsStateWithLifecycle()
    val scroll = TopAppBarDefaults.exitUntilCollapsedScrollBehavior()
    Scaffold(
        modifier = Modifier.nestedScroll(scroll.nestedScrollConnection),
        topBar = { TabTopBar(state.serverName, scroll, initial = state.initial, onAvatar = { nav.open(SettingsRoute) }) },
    ) { padding ->
        val rows = state.rows
        val problem = state.problem
        Box(Modifier.padding(padding).fillMaxSize()) {
            when {
                rows == null && problem != null ->
                    Failed(problem, stringResource(R.string.mobile_couldnt_load_home), state.serverName, model::refresh)
                rows == null -> HomeSkeleton()
                else -> PullToRefreshBox(isRefreshing = state.refreshing, onRefresh = model::refresh, modifier = Modifier.fillMaxSize()) {
                    if (rows.isEmpty()) {
                        // Scrollable, so a pull still refreshes it.
                        LazyColumn(Modifier.fillMaxSize()) {
                            item { Message(stringResource(R.string.mobile_nothing_yet), stringResource(R.string.mobile_nothing_yet_sentence), Modifier.height(400.dp)) }
                        }
                    } else {
                        WorkMenuHost({ action, work -> nav.handle(action, work, model.marks) }) { openMenu ->
                            Rows(rows, nav, onFavorites, openMenu)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun Rows(rows: List<HomeRowUi>, nav: MobileNav, onFavorites: () -> Unit, openMenu: (WorkMenu) -> Unit) {
    LazyColumn(
        Modifier.fillMaxSize().testTag("homeRows"),
        contentPadding = PaddingValues(bottom = space(6)),
        verticalArrangement = Arrangement.spacedBy(space(7)),
    ) {
        items(rows, key = { it.id }, contentType = { it.kind }) { row ->
            val library = row.library
            val opens: (() -> Unit)? = when {
                row.kind == "favorites" -> onFavorites
                row.kind == "recentlyAdded" && library != null -> { { nav.open(LibraryRoute(library.id, GridValues.ADDED_AT)) } }
                else -> null
            }
            Column {
                RowHeading(rowTitle(row), opens)
                LazyRow(
                    state = rowState(row.items.firstOrNull()?.id),
                    contentPadding = PaddingValues(horizontal = SideMargin),
                    horizontalArrangement = Arrangement.spacedBy(CardGap),
                ) {
                    items(row.items, key = { it.id }, contentType = { if (row.kind == CONTINUE_WATCHING) CardShape.Landscape else shapeOf(it) }) { work ->
                        // Continue Watching is the frames where you stopped.
                        val shape = if (row.kind == CONTINUE_WATCHING) CardShape.Landscape else shapeOf(work)
                        WorkCard(
                            work = work,
                            shape = shape,
                            width = rowCardWidth(shape),
                            onClick = { nav.openWork(work) },
                            onLongClick = { openMenu(WorkMenu(work, row.kind)) },
                        )
                    }
                }
            }
        }
    }
}

/**
 * A row's scroll, which keeps a card that arrives at the row's start in
 * view while the row sits at its start, rather than holding on to the card
 * that was first, as a keyed row otherwise does; [first] is the first card's ID.
 */
@Composable
private fun rowState(first: String?): LazyListState {
    val state = rememberLazyListState()
    val seen = remember { FirstCard(first) }
    if (first != seen.id) {
        seen.id = first
        val atStart = Snapshot.withoutReadObservation { state.firstVisibleItemIndex == 0 && state.firstVisibleItemScrollOffset == 0 }
        if (atStart) state.requestScrollToItem(0)
    }
    return state
}

/** The first card a row last showed, kept across compositions without drawing anything when it changes. */
private class FirstCard(var id: String?)

/** A row's heading; a chevron after it when it opens a page. */
@Composable
private fun RowHeading(title: String, onClick: (() -> Unit)?) {
    Row(
        Modifier
            .fillMaxWidth()
            .then(if (onClick != null) Modifier.clickable(onClick = onClick) else Modifier)
            .heightIn(min = 48.dp)
            .padding(horizontal = SideMargin, vertical = space(2))
            .semantics(mergeDescendants = true) { heading() },
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(title, style = MaterialTheme.typography.titleMedium)
        if (onClick != null) Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, null, tint = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
private fun rowTitle(row: HomeRowUi): String = when (row.kind) {
    CONTINUE_WATCHING -> stringResource(R.string.mobile_row_continue_watching)
    "continueListening" -> stringResource(R.string.mobile_row_continue_listening)
    "continueSeries" -> stringResource(R.string.mobile_row_continue_series)
    "favorites" -> stringResource(R.string.mobile_row_favorites)
    else -> row.libraryName?.let { stringResource(R.string.mobile_row_recently_added_in, it) } ?: stringResource(R.string.mobile_row_recently_added)
}

/** Home while it loads: a row of landscape cards, then two of posters. */
@Composable
private fun HomeSkeleton() {
    val placeholder = LocalKivo.current.placeholder
    Column(Modifier.fillMaxSize().padding(top = space(2)), verticalArrangement = Arrangement.spacedBy(space(7))) {
        for (shape in listOf(CardShape.Landscape, CardShape.Poster, CardShape.Poster)) {
            Column(verticalArrangement = Arrangement.spacedBy(space(3))) {
                Box(Modifier.padding(horizontal = SideMargin).width(140.dp).height(16.dp).clip(CardCorners).background(placeholder))
                Row(Modifier.horizontalScroll(rememberScrollState(), enabled = false).padding(horizontal = SideMargin), horizontalArrangement = Arrangement.spacedBy(CardGap)) {
                    repeat(6) { Box(Modifier.width(rowCardWidth(shape)).aspectRatio(shape.aspect).clip(CardCorners).background(placeholder)) }
                }
            }
        }
    }
}
