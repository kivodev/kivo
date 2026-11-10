@file:OptIn(ExperimentalFoundationApi::class)

package com.mmcdole.kivo.android.lists

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.gestures.BringIntoViewSpec
import androidx.compose.foundation.gestures.LocalBringIntoViewSpec
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.home.RowCardsBelow
import com.mmcdole.kivo.android.home.RowGap
import com.mmcdole.kivo.android.home.RowTitlePadding
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.tv.R
import com.mmcdole.kivo.android.ui.CardRow
import com.mmcdole.kivo.android.ui.CardShape
import com.mmcdole.kivo.android.ui.CardSurface
import com.mmcdole.kivo.android.ui.EmptyState
import com.mmcdole.kivo.android.ui.Failed
import com.mmcdole.kivo.android.ui.FocusRail
import com.mmcdole.kivo.android.ui.KivoIcon
import com.mmcdole.kivo.android.ui.KivoIconView
import com.mmcdole.kivo.android.ui.Loader
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.SafeEdge
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.android.ui.SeeAllCard
import com.mmcdole.kivo.android.ui.SkeletonGate
import com.mmcdole.kivo.android.ui.SkeletonRow
import com.mmcdole.kivo.android.ui.WorkCard
import com.mmcdole.kivo.android.ui.cardShapeOf
import com.mmcdole.kivo.android.ui.cardWidth
import com.mmcdole.kivo.android.ui.focusKey
import com.mmcdole.kivo.android.ui.map
import com.mmcdole.kivo.android.ui.raisedOnFocus
import com.mmcdole.kivo.android.ui.rememberFocusMemory
import com.mmcdole.kivo.api.models.WorkList
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.Load
import kotlinx.coroutines.flow.map

/** A list's shelf: the list and its first works, books left out. */
data class Shelf(val list: WorkList, val works: List<WorkSummary>)

private const val SMART = "smart"

/** The lists as shelves, in their order: Watchlist only with works to show, and a list kind it doesn't know left out. */
internal fun shelves(lists: List<WorkList>): List<Shelf> = lists
    .filter { it.kind == "manual" || it.kind == SMART }
    .map { list -> Shelf(list, list.items.orEmpty().filter { it.kind in tvWorkKinds }) }
    .filter { !it.list.builtIn || it.works.isNotEmpty() }

/** Lists with their first works, read again as they change. */
class ListsViewModel(session: ServerSession) : ViewModel() {
    private val library = session.session.library

    val ui = Loader(viewModelScope) { library.lists(counted = true).map { it.map(::shelves) } }
}

/** Where the screen's title starts. */
private val TitleTop = 39.5.dp

/** The screen's padding beside the rail. */
private val ScreenPadding = PaddingValues(start = 24.dp, top = TitleTop, end = SafeEdge)

/** The gap between the screen's title and its first row. */
private val TitleGap = 37.5.dp

/** The screen's title. */
@Composable
private fun ListsHeading(modifier: Modifier = Modifier) {
    Text(stringResource(R.string.lists), style = TvType.title, color = Kivo.colors.text, modifier = modifier)
}

/** [title], with how to add to a list; with nothing here to focus, focus goes to the rail. */
@Composable
internal fun ListsEmpty(title: String) {
    EmptyState(title, stringResource(R.string.lists_empty_sentence))
    FocusRail()
}

/**
 * Lists: the title, then a shelf for each list, its name over a row of its
 * first works that ends in a See all card, which opens it. An empty list's
 * shelf is one card that opens it. Refreshes in place as lists change,
 * keeping focus and scroll.
 */
@Composable
fun ListsScreen(session: ServerSession, navigator: Navigator) {
    val model = viewModel { ListsViewModel(session) }
    val state by model.ui.state.collectAsStateWithLifecycle()

    when (val s = state) {
        is Load.Failed -> Failed(s.problem, session.session.baseUrl, model.ui::retry, session.graph::changeServer)
        else -> SkeletonGate(s.getOrNull(), { ListsSkeleton() }) { shelves ->
            if (shelves.isEmpty()) {
                Column(Modifier.fillMaxSize().padding(ScreenPadding)) {
                    ListsHeading()
                    ListsEmpty(stringResource(R.string.lists_empty))
                }
            } else {
                Shelves(shelves, navigator)
            }
        }
    }
}

/** Lists while they load: the title, then two rows of posters. */
@Composable
private fun ListsSkeleton() {
    Column(Modifier.fillMaxSize().padding(ScreenPadding)) {
        ListsHeading()
        Column(Modifier.padding(top = TitleGap), verticalArrangement = Arrangement.spacedBy(RowGap)) {
            SkeletonRow(CardShape.Poster, titlePadding = RowTitlePadding)
            SkeletonRow(CardShape.Poster, titlePadding = RowTitlePadding)
        }
    }
}

/** Where a focused row stays as the rows move under it, under the screen's top; as Home's. */
private val FocusedCardsTop = 37.dp + RowCardsBelow

/** Where the first row's cards stay while it has focus, with the title above them. */
private val FirstCardsTop = TitleTop + 30.dp + TitleGap + RowCardsBelow

/** Keeps a focused row's cards [rest] from the list's top, or [first] while the first row has focus. */
private class SectionsSpec(private val first: Float, private val rest: Float, private val inFirst: () -> Boolean) : BringIntoViewSpec {
    override fun calculateScrollDistance(offset: Float, size: Float, containerSize: Float): Float = offset - if (inFirst()) first else rest
}

/** A plain flag for where focus is, read as the list scrolls, so moving it recomposes nothing. */
private class FirstSection(var focused: Boolean = true)

@Composable
private fun Shelves(shelves: List<Shelf>, navigator: Navigator) {
    val memory = rememberFocusMemory()
    val list = rememberLazyListState()
    val firstSection = remember { FirstSection() }
    val density = LocalDensity.current
    val spec = remember(density) { with(density) { SectionsSpec(FirstCardsTop.toPx(), FocusedCardsTop.toPx()) { firstSection.focused } } }
    val first = shelves.first().let { "${it.list.id}/${it.works.firstOrNull()?.id ?: "all"}" }
    CompositionLocalProvider(LocalBringIntoViewSpec provides spec) {
        LazyColumn(
            state = list,
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(start = 24.dp, top = TitleTop, bottom = 250.dp),
            verticalArrangement = Arrangement.spacedBy(RowGap),
        ) {
            item(key = "title") { ListsHeading(Modifier.padding(bottom = TitleGap - RowGap)) }
            itemsIndexed(shelves, key = { _, shelf -> shelf.list.id }) { index, (shelfList, works) ->
                val open = { navigator.push(Screen.ListWorks(shelfList.id, shelfList.name)) }
                // Raised with its focused card, so the card's glow lies over what's below.
                Column(Modifier.raisedOnFocus().onFocusChanged { if (it.hasFocus) firstSection.focused = index == 0 }) {
                    ShelfTitle(shelfList, works.isEmpty())
                    CardRow {
                        items(works, key = { it.id }, contentType = { cardShapeOf(it) }) { work ->
                            val shape = cardShapeOf(work)
                            WorkCard(
                                work = work,
                                shape = shape,
                                width = cardWidth(shape),
                                onClick = { navigator.push(Screen.Work(work.id)) },
                                modifier = Modifier.focusKey(memory, "${shelfList.id}/${work.id}"),
                            )
                        }
                        item(key = "all", contentType = "all") {
                            val shape = CardShape.Poster
                            val modifier = Modifier.focusKey(memory, "${shelfList.id}/all")
                            if (works.isEmpty()) {
                                EmptyShelfCard(cardWidth(shape), open, modifier)
                            } else {
                                SeeAllCard(detail = null, shape = shape, width = cardWidth(shape), onClick = open, modifier = modifier)
                            }
                        }
                    }
                }
            }
        }
    }
    LaunchedEffect(Unit) { memory.restore(first) }
}

/** A shelf's heading: the list's name, a smart list's mark, and how many titles it has. */
@Composable
private fun ShelfTitle(list: WorkList, empty: Boolean) {
    val colors = Kivo.colors
    Row(
        Modifier.padding(RowTitlePadding).testTag("list:${list.name}"),
        horizontalArrangement = Arrangement.spacedBy(Kivo.space(3)),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(list.name, Modifier.weight(1f, fill = false), style = TvType.large, color = colors.text, maxLines = 1, overflow = TextOverflow.Ellipsis)
        if (list.kind == SMART) {
            Row(horizontalArrangement = Arrangement.spacedBy(Kivo.space(1)), verticalAlignment = Alignment.CenterVertically) {
                KivoIconView(KivoIcon.Sparkle, size = 16.dp, tint = colors.accent)
                Text(stringResource(R.string.smart_list), style = TvType.small, color = colors.accent, maxLines = 1)
            }
        }
        Text(
            if (empty) stringResource(R.string.list_no_titles) else pluralStringResource(R.plurals.titles, list.workCount, list.workCount),
            style = TvType.small,
            color = colors.textMuted,
            maxLines = 1,
        )
    }
}

/** The card in an empty list's shelf, a poster's size, which opens the list. */
@Composable
private fun EmptyShelfCard(width: Dp, onClick: () -> Unit, modifier: Modifier) {
    val colors = Kivo.colors
    CardSurface(
        onClick = onClick,
        onLongClick = null,
        modifier = modifier.raisedOnFocus().width(width).aspectRatio(CardShape.Poster.aspect).testTag("emptyShelf"),
    ) {
        Box(
            Modifier
                .fillMaxSize()
                .drawBehind {
                    val stroke = 1.dp.toPx()
                    val dash = 4.dp.toPx()
                    val radius = KivoTokens.Radius.md.toPx()
                    drawRoundRect(
                        color = colors.border,
                        topLeft = Offset(stroke / 2, stroke / 2),
                        size = Size(size.width - stroke, size.height - stroke),
                        cornerRadius = CornerRadius(radius, radius),
                        style = Stroke(stroke, pathEffect = PathEffect.dashPathEffect(floatArrayOf(dash, dash))),
                    )
                }
                .padding(Kivo.space(3)),
            contentAlignment = Alignment.Center,
        ) {
            Text(stringResource(R.string.list_empty_shelf), style = TvType.small, color = colors.textMuted, textAlign = TextAlign.Center)
        }
    }
}
