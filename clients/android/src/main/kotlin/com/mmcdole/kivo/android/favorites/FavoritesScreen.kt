package com.mmcdole.kivo.android.favorites

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.R
import com.mmcdole.kivo.android.ServerSession
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
import com.mmcdole.kivo.android.ui.RowCardsTop
import com.mmcdole.kivo.android.ui.RowTitle
import com.mmcdole.kivo.android.ui.RowsHoldPlace
import com.mmcdole.kivo.android.ui.SafeEdge
import com.mmcdole.kivo.android.ui.SafeTop
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.android.ui.SeeAllCard
import com.mmcdole.kivo.android.ui.SkeletonGate
import com.mmcdole.kivo.android.ui.SkeletonRow
import com.mmcdole.kivo.android.ui.WorkCard
import com.mmcdole.kivo.android.ui.cardWidth
import com.mmcdole.kivo.android.ui.focusKey
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

/** The screen's padding beside the rail. */
private val FavoritesPadding = PaddingValues(start = 24.dp, top = SafeTop)

/** The screen's title, at a library's title's height. */
@Composable
private fun FavoritesHeading() {
    Box(Modifier.height(44.dp).padding(end = SafeEdge), contentAlignment = Alignment.CenterStart) {
        Text(stringResource(R.string.favorites), style = TvType.heading, color = Kivo.colors.text)
    }
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
            Column(Modifier.fillMaxSize().padding(FavoritesPadding)) {
                FavoritesHeading()
                if (rows.isEmpty()) {
                    FavoritesEmpty()
                } else {
                    Rows(rows, navigator)
                }
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
        Column(Modifier.padding(top = RowsTop), verticalArrangement = Arrangement.spacedBy(RowGap)) {
            SkeletonRow(CardShape.Poster)
            SkeletonRow(CardShape.Poster)
        }
    }
}

/** The gap between rows, as Home's. */
private val RowGap = 28.dp

/** The gap between the screen's title and its first row's. */
private val RowsTop = 12.dp

@Composable
private fun Rows(rows: List<FavoriteRow>, navigator: Navigator) {
    val memory = rememberFocusMemory()
    val list = rememberLazyListState()
    val first = rows.first().let { "${it.kind}/${it.page.items.first().id}" }
    RowsHoldPlace(RowCardsTop) {
        LazyColumn(
            state = list,
            modifier = Modifier.fillMaxSize().padding(top = RowsTop),
            contentPadding = PaddingValues(end = SafeEdge, bottom = SafeEdge),
            verticalArrangement = Arrangement.spacedBy(RowGap),
        ) {
            items(rows, key = { it.kind }) { row ->
                val shape = favoriteShape(row.kind)
                val width = cardWidth(shape)
                val grid = Screen.FavoritesGrid(row.kind)
                Column {
                    RowTitle(favoritesTitle(row.kind), Modifier.padding(bottom = 12.dp))
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
