@file:OptIn(ExperimentalFoundationApi::class)

package com.mmcdole.kivo.android.favorites

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.gestures.BringIntoViewSpec
import androidx.compose.foundation.gestures.LocalBringIntoViewSpec
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.tv.R
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.home.RowCardsBelow
import com.mmcdole.kivo.android.home.RowGap
import com.mmcdole.kivo.android.home.RowTitlePadding
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.ui.CardRow
import com.mmcdole.kivo.android.ui.CardShape
import com.mmcdole.kivo.android.ui.EmptyState
import com.mmcdole.kivo.android.ui.Failed
import com.mmcdole.kivo.android.ui.FocusRail
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.android.ui.Loader
import com.mmcdole.kivo.android.ui.all
import com.mmcdole.kivo.android.ui.map
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.RowTitle
import com.mmcdole.kivo.android.ui.SafeEdge
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.android.ui.SeeAllCard
import com.mmcdole.kivo.android.ui.SkeletonGate
import com.mmcdole.kivo.android.ui.SkeletonRow
import com.mmcdole.kivo.android.ui.WorkCard
import com.mmcdole.kivo.android.ui.cardWidth
import com.mmcdole.kivo.android.ui.focusKey
import com.mmcdole.kivo.android.ui.raisedOnFocus
import com.mmcdole.kivo.android.ui.rememberFocusMemory
import com.mmcdole.kivo.api.models.WorkSummaryPage
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.map
import java.text.NumberFormat

/** The kinds the TV shows favorites of, in their rows' order: no books, which aren't on the TV. */
val tvFavoriteKinds = listOf(
    "movie",
    "series",
    "episode",
    "season",
    "video",
    "extra",
)

/** A kind's row: the first page of its favorites. */
data class FavoriteRow(val kind: String, val page: WorkSummaryPage)

/** The most cards a row shows before its See all card. */
private const val ROW_LIMIT = 20

class FavoritesViewModel(session: ServerSession) : ViewModel() {
    /** A row for each kind that has favorites, refetched as the user's state changes. */
    val rows = Loader(viewModelScope) {
        combine(tvFavoriteKinds.map { kind -> session.session.library.favorites(kind, ROW_LIMIT) }) { loads ->
            loads.toList().all().map { pages -> tvFavoriteKinds.zip(pages, ::FavoriteRow).filter { it.page.items.isNotEmpty() } }
        }
    }
}

/** A favorite's card shape: an episode by its series' poster; other videos landscape. */
fun favoriteShape(kind: String): CardShape = when (kind) {
    "video", "extra" -> CardShape.Landscape
    "book" -> CardShape.Square
    else -> CardShape.Poster
}

/** A kind's row title: "Movies", "Shows". */
@Composable
fun favoritesTitle(kind: String): String = stringResource(
    when (kind) {
        "movie" -> R.string.favorites_movies
        "series" -> R.string.favorites_shows
        "episode" -> R.string.favorites_episodes
        "season" -> R.string.favorites_seasons
        "extra" -> R.string.favorites_extras
        else -> R.string.favorites_videos
    },
)

/** "12 movies", "1 episode": how many favorites of [kind]. */
@Composable
fun favoritesCount(kind: String, count: Int): String {
    val plural = when (kind) {
        "movie" -> R.plurals.count_movies
        "series" -> R.plurals.count_shows
        "episode" -> R.plurals.count_episodes
        "season" -> R.plurals.count_seasons
        "extra" -> R.plurals.count_extras
        else -> R.plurals.count_videos
    }
    return pluralStringResource(plural, count, NumberFormat.getIntegerInstance().format(count))
}

/** Where the screen's title starts. */
private val TitleTop = 39.5.dp

/** The screen's padding beside the rail. */
private val FavoritesPadding = PaddingValues(start = 24.dp, top = TitleTop, end = SafeEdge)

/** The gap between the screen's title and its first row. */
private val TitleGap = 37.5.dp

/** The screen's title. */
@Composable
private fun FavoritesHeading(modifier: Modifier = Modifier) {
    Text(stringResource(R.string.favorites), style = TvType.title, color = Kivo.colors.text, modifier = modifier)
}

/**
 * Favorites: the title, then a row for each kind that has
 * any, most recently marked first, each ending in a See all card that opens
 * the kind's grid. Rows hold their place as Home's do, and refresh in place
 * as the user's state changes, keeping focus and scroll.
 */
@Composable
fun FavoritesScreen(session: ServerSession, navigator: Navigator) {
    val model = viewModel { FavoritesViewModel(session) }
    val state by model.rows.state.collectAsStateWithLifecycle()

    when (val s = state) {
        is Load.Failed -> Failed(s.problem, session.session.baseUrl, model.rows::retry, session.graph::changeServer)
        else -> SkeletonGate(s.getOrNull(), { FavoritesSkeleton() }) { rows ->
            if (rows.isEmpty()) {
                Column(Modifier.fillMaxSize().padding(FavoritesPadding)) {
                    FavoritesHeading()
                    FavoritesEmpty()
                }
            } else {
                Rows(rows, navigator)
            }
        }
    }
}

/** "No favorites yet.", with how to add one; with nothing here to focus, focus goes to the rail. */
@Composable
fun FavoritesEmpty() {
    EmptyState(stringResource(R.string.favorites_empty), stringResource(R.string.favorites_empty_sentence))
    FocusRail()
}

/** Favorites while they load: the title, then two rows of posters. */
@Composable
private fun FavoritesSkeleton() {
    Column(Modifier.fillMaxSize().padding(FavoritesPadding)) {
        FavoritesHeading()
        Column(Modifier.padding(top = TitleGap), verticalArrangement = Arrangement.spacedBy(RowGap)) {
            SkeletonRow(CardShape.Poster, titlePadding = RowTitlePadding)
            SkeletonRow(CardShape.Poster, titlePadding = RowTitlePadding)
        }
    }
}

/** Where a focused row's cards stay as the rows move under it, under the screen's top; as Home's. */
private val FocusedCardsTop = 37.dp + RowCardsBelow

/** Where the first row's cards stay while it has focus, with the title above them. */
private val FirstCardsTop = TitleTop + 30.dp + TitleGap + RowCardsBelow

/** Keeps a focused row's cards [rest] from the list's top, or [first] while the first row has focus. */
private class RowsSpec(private val first: Float, private val rest: Float, private val inFirst: () -> Boolean) : BringIntoViewSpec {
    override fun calculateScrollDistance(offset: Float, size: Float, containerSize: Float): Float = offset - if (inFirst()) first else rest
}

/** A plain flag for where focus is, read as the list scrolls, so moving it recomposes nothing. */
private class FirstRow(var focused: Boolean = true)

@Composable
private fun Rows(rows: List<FavoriteRow>, navigator: Navigator) {
    val memory = rememberFocusMemory()
    val list = rememberLazyListState()
    val first = rows.first().let { "${it.kind}/${it.page.items.first().id}" }
    val firstKind = rows.first().kind
    val firstRow = remember { FirstRow() }
    val density = LocalDensity.current
    val spec = remember(density) { with(density) { RowsSpec(FirstCardsTop.toPx(), FocusedCardsTop.toPx()) { firstRow.focused } } }
    CompositionLocalProvider(LocalBringIntoViewSpec provides spec) {
        LazyColumn(
            state = list,
            modifier = Modifier.fillMaxSize(),
            // The rows run to the screen's edge. Below them is room for the last to come up to where a focused row stays.
            contentPadding = PaddingValues(start = 24.dp, top = TitleTop, bottom = 250.dp),
            verticalArrangement = Arrangement.spacedBy(RowGap),
        ) {
            item(key = "title") { FavoritesHeading(Modifier.padding(bottom = TitleGap - RowGap)) }
            items(rows, key = { it.kind }) { row ->
                val shape = favoriteShape(row.kind)
                val width = cardWidth(shape)
                val grid = Screen.FavoritesGrid(row.kind)
                // Raised with its focused card, so the card's glow lies over the row below.
                Column(Modifier.raisedOnFocus().onFocusChanged { if (it.hasFocus) firstRow.focused = row.kind == firstKind }) {
                    RowTitle(favoritesTitle(row.kind), Modifier.padding(RowTitlePadding))
                    CardRow {
                        items(row.page.items, key = { it.id }) { work ->
                            WorkCard(
                                work = work,
                                shape = shape,
                                width = width,
                                onClick = { navigator.push(Screen.Work(work.id)) },
                                modifier = Modifier.focusKey(memory, "${row.kind}/${work.id}"),
                            )
                        }
                        item(key = "all") {
                            val total = row.page.total ?: row.page.items.size
                            SeeAllCard(
                                detail = favoritesCount(row.kind, total),
                                shape = shape,
                                width = width,
                                onClick = { navigator.push(grid) },
                                modifier = Modifier.focusKey(memory, "${row.kind}/all"),
                            )
                        }
                    }
                }
            }
        }
    }
    LaunchedEffect(Unit) { memory.restore(first) }
}
