@file:OptIn(ExperimentalFoundationApi::class)

package com.mmcdole.kivo.android.lists

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.gestures.BringIntoViewSpec
import androidx.compose.foundation.gestures.LocalBringIntoViewSpec
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
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
import com.mmcdole.kivo.android.library.GridWidth
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.tv.R
import com.mmcdole.kivo.android.ui.CardGap
import com.mmcdole.kivo.android.ui.CardRow
import com.mmcdole.kivo.android.ui.CardShape
import com.mmcdole.kivo.android.ui.CardSurface
import com.mmcdole.kivo.android.ui.EmptyState
import com.mmcdole.kivo.android.ui.Failed
import com.mmcdole.kivo.android.ui.FocusRail
import com.mmcdole.kivo.android.ui.Loader
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.RowTitle
import com.mmcdole.kivo.android.ui.SafeEdge
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.android.ui.SeeAllCard
import com.mmcdole.kivo.android.ui.SkeletonGate
import com.mmcdole.kivo.android.ui.SkeletonRow
import com.mmcdole.kivo.android.ui.WorkCard
import com.mmcdole.kivo.android.ui.cardShapeOf
import com.mmcdole.kivo.android.ui.cardWidth
import com.mmcdole.kivo.android.ui.focusKey
import com.mmcdole.kivo.android.ui.raisedOnFocus
import com.mmcdole.kivo.android.ui.rememberFocusMemory
import com.mmcdole.kivo.android.ui.with
import com.mmcdole.kivo.api.models.WorkList
import com.mmcdole.kivo.api.models.WorkSummaryPage
import com.mmcdole.kivo.core.Load
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.onStart
import kotlinx.coroutines.flow.shareIn

/** What the Lists screen shows: My List and its first works, books left out, and the lists the user made. */
data class ListsUi(val myList: WorkList?, val works: WorkSummaryPage, val lists: List<WorkList>)

/** The most cards My List's row shows before its See all card. */
private const val ROW_LIMIT = 20

/** Lists as the screen shows them, from the lists and My List's first works; a list kind it doesn't know is left out. */
internal fun listsUi(lists: List<WorkList>, works: WorkSummaryPage): ListsUi = ListsUi(
    myList = lists.firstOrNull { it.builtIn },
    works = works.copy(items = works.items.filter { it.kind in tvWorkKinds }),
    lists = lists.filter { !it.builtIn && it.kind == "manual" },
)

/** Lists, read again as they change: My List's works follow its id, which the lists give. */
@OptIn(ExperimentalCoroutinesApi::class)
class ListsViewModel(session: ServerSession) : ViewModel() {
    private val library = session.session.library

    val ui = Loader(viewModelScope) {
        val lists: Flow<Load<List<WorkList>>> = library.lists().shareIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), replay = 1)
        val works: Flow<Load<WorkSummaryPage>> = lists
            .map { load -> load.getOrNull()?.firstOrNull { it.builtIn }?.id }
            .distinctUntilChanged()
            .flatMapLatest { id -> if (id == null) flowOf(Load.Loaded(WorkSummaryPage(emptyList(), total = 0))) else library.listWorks(id, ROW_LIMIT).onStart { emit(Load.Loading) } }
        combine(lists, works) { lists, works -> lists.with(works, ::listsUi) }
    }
}

/** Where the screen's title starts. */
private val TitleTop = 39.5.dp

/** The screen's padding beside the rail. */
private val ScreenPadding = PaddingValues(start = 24.dp, top = TitleTop, end = SafeEdge)

/** The gap between the screen's title and its first row. */
private val TitleGap = 37.5.dp

/** A list's tile: as tall as a line of a grid's name and count, in thirds of a grid's width. */
private val TileHeight = 96.dp
private val TileWidth = (GridWidth - CardGap * 2) / 3

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
 * Lists: the title, My List's row with its See all card that opens it, then
 * the lists the user made as tiles with their counts. A section with nothing
 * in it is left out. Refreshes in place as lists change, keeping focus and scroll.
 */
@Composable
fun ListsScreen(session: ServerSession, navigator: Navigator) {
    val model = viewModel { ListsViewModel(session) }
    val state by model.ui.state.collectAsStateWithLifecycle()

    when (val s = state) {
        is Load.Failed -> Failed(s.problem, session.session.baseUrl, model.ui::retry, session.graph::changeServer)
        else -> SkeletonGate(s.getOrNull(), { ListsSkeleton() }) { ui ->
            if (ui.works.items.isEmpty() && ui.lists.isEmpty()) {
                Column(Modifier.fillMaxSize().padding(ScreenPadding)) {
                    ListsHeading()
                    ListsEmpty(stringResource(R.string.lists_empty))
                }
            } else {
                Sections(ui, navigator)
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
private fun Sections(ui: ListsUi, navigator: Navigator) {
    val memory = rememberFocusMemory()
    val list = rememberLazyListState()
    val firstSection = remember { FirstSection() }
    val density = LocalDensity.current
    val spec = remember(density) { with(density) { SectionsSpec(FirstCardsTop.toPx(), FocusedCardsTop.toPx()) { firstSection.focused } } }
    val works = ui.works.items
    val myList = ui.myList?.takeIf { works.isNotEmpty() }
    val tiles = ui.lists.chunked(3)
    val first = if (myList != null) "myList/${works.first().id}" else ui.lists.first().id
    CompositionLocalProvider(LocalBringIntoViewSpec provides spec) {
        LazyColumn(
            state = list,
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(start = 24.dp, top = TitleTop, bottom = 250.dp),
            verticalArrangement = Arrangement.spacedBy(RowGap),
        ) {
            item(key = "title") { ListsHeading(Modifier.padding(bottom = TitleGap - RowGap)) }
            if (myList != null) {
                item(key = "myList") {
                    // Raised with its focused card, so the card's glow lies over what's below.
                    Column(Modifier.raisedOnFocus().onFocusChanged { if (it.hasFocus) firstSection.focused = true }) {
                        RowTitle(myList.name, Modifier.padding(RowTitlePadding))
                        CardRow {
                            items(works, key = { it.id }) { work ->
                                val shape = cardShapeOf(work)
                                WorkCard(
                                    work = work,
                                    shape = shape,
                                    width = cardWidth(shape),
                                    onClick = { navigator.push(Screen.Work(work.id)) },
                                    modifier = Modifier.focusKey(memory, "myList/${work.id}"),
                                )
                            }
                            item(key = "all") {
                                val shape = CardShape.Poster
                                SeeAllCard(
                                    detail = pluralStringResource(R.plurals.titles, myList.workCount, myList.workCount),
                                    shape = shape,
                                    width = cardWidth(shape),
                                    onClick = { navigator.push(Screen.ListWorks(myList.id, myList.name)) },
                                    modifier = Modifier.focusKey(memory, "myList/all"),
                                )
                            }
                        }
                    }
                }
            }
            if (tiles.isNotEmpty()) {
                item(key = "yourLists") { RowTitle(stringResource(R.string.your_lists), Modifier.padding(RowTitlePadding)) }
                items(tiles, key = { line -> line.first().id }) { line ->
                    Row(
                        Modifier.onFocusChanged { if (it.hasFocus) firstSection.focused = false },
                        horizontalArrangement = Arrangement.spacedBy(CardGap),
                    ) {
                        for (tile in line) {
                            ListTile(tile, Modifier.focusKey(memory, tile.id)) { navigator.push(Screen.ListWorks(tile.id, tile.name)) }
                        }
                    }
                }
            }
        }
    }
    LaunchedEffect(Unit) { memory.restore(first) }
}

/** A list the user made: its name and count on a tile. */
@Composable
private fun ListTile(list: WorkList, modifier: Modifier, onClick: () -> Unit) {
    val colors = Kivo.colors
    CardSurface(
        onClick = onClick,
        onLongClick = null,
        modifier = modifier.raisedOnFocus().width(TileWidth).height(TileHeight).testTag("list:${list.name}"),
    ) {
        Column(Modifier.padding(Kivo.space(3)), horizontalAlignment = Alignment.CenterHorizontally) {
            Text(list.name, style = TvType.bodyStrong, color = colors.text, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Text(pluralStringResource(R.plurals.titles, list.workCount, list.workCount), style = TvType.small, color = colors.textMuted, maxLines = 1)
        }
    }
}
