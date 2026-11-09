package com.mmcdole.kivo.android.lists

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
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.library.GridTop
import com.mmcdole.kivo.android.library.PagedCards
import com.mmcdole.kivo.android.library.gridCardWidth
import com.mmcdole.kivo.android.library.gridColumns
import com.mmcdole.kivo.android.library.scrollsWith
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.tv.R
import com.mmcdole.kivo.android.ui.CardGap
import com.mmcdole.kivo.android.ui.CardShape
import com.mmcdole.kivo.android.ui.Failed
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.SafeEdge
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.android.ui.SkeletonCard
import com.mmcdole.kivo.android.ui.SkeletonGate
import com.mmcdole.kivo.android.ui.SkeletonLine
import com.mmcdole.kivo.android.ui.WorkCard
import com.mmcdole.kivo.android.ui.cardShapeOf
import com.mmcdole.kivo.android.ui.prepareCard
import kotlinx.coroutines.launch

/** The kinds the TV shows of a list: no books, which aren't on the TV. */
internal val tvWorkKinds = setOf("movie", "series", "episode", "season", "video", "extra")

/** An open list as a grid, its pages read again in place as the list changes. */
class ListWorksViewModel(session: ServerSession, listId: String) : ViewModel() {
    val paged = session.session.library.pagedListWorks(listId)

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
 * An open list, most recently added first: the title, the count line, and
 * the grid of a library, without its sort, filters or letter rail, each card
 * in its own work's shape; a book leaves its cell empty, as the pager has no
 * way to skip it. With nothing in it, the title and the empty state.
 */
@Composable
fun ListWorksScreen(session: ServerSession, listId: String, name: String, navigator: Navigator) {
    val model = viewModel(key = listId) { ListWorksViewModel(session, listId) }
    val pages by model.paged.state.collectAsStateWithLifecycle()
    val failed = pages.problem
    if (failed != null) {
        Failed(failed, session.session.baseUrl, model::retry, session.graph::changeServer)
        return
    }
    SkeletonGate(pages.total, { GridSkeleton(name) }) { total ->
        if (total == 0) {
            Column(Modifier.fillMaxSize().padding(HeadingPadding)) {
                Heading(name)
                ListsEmpty(stringResource(R.string.list_empty))
            }
        } else {
            val grid = rememberSaveable(saver = LazyListState.Saver) { LazyListState() }
            Box(Modifier.fillMaxSize()) {
                PagedCards(
                    paged = model.paged,
                    grid = grid,
                    shape = CardShape.Poster,
                    key = { it.id },
                    prepare = { prepareCard(it, cardShapeOf(it)) },
                ) { work, width, cardModifier ->
                    if (work == null || work.kind in tvWorkKinds) {
                        WorkCard(work, work?.let(::cardShapeOf) ?: CardShape.Poster, width, { if (work != null) navigator.push(Screen.Work(work.id)) }, cardModifier)
                    }
                }
                // The title and count line scroll away with the grid.
                Column(Modifier.scrollsWith(grid).padding(HeadingPadding)) {
                    Heading(name)
                    Text(pluralStringResource(R.plurals.titles, total, total), style = TvType.small, color = Kivo.colors.textMuted, modifier = Modifier.padding(top = 13.dp))
                }
            }
        }
    }
}

@Composable
private fun Heading(name: String) {
    Text(name, style = TvType.title, color = Kivo.colors.text)
}

/** The grid while its first page loads: the title, the count line and a line of cards. */
@Composable
private fun GridSkeleton(name: String) {
    val shape = CardShape.Poster
    Box(Modifier.fillMaxSize()) {
        Column(Modifier.padding(HeadingPadding)) {
            Heading(name)
            SkeletonLine(180.dp, TvType.small, Modifier.padding(top = 13.dp))
        }
        Row(Modifier.padding(start = 24.dp, top = GridTop), horizontalArrangement = Arrangement.spacedBy(CardGap)) {
            repeat(gridColumns(shape)) { SkeletonCard(shape, gridCardWidth(shape)) }
        }
    }
}
