package com.mmcdole.kivo.android.mobile.library

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.DragInteraction
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyGridState
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.FilterList
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.InputChip
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.Stable
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.NestedScrollConnection
import androidx.compose.ui.input.nestedscroll.NestedScrollSource
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.library.GridValues
import com.mmcdole.kivo.android.library.filtered
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.app.BookSeriesRoute
import com.mmcdole.kivo.android.mobile.app.CollectionRoute
import com.mmcdole.kivo.android.mobile.app.MobileNav
import com.mmcdole.kivo.android.mobile.app.PersonRoute
import com.mmcdole.kivo.android.mobile.app.handle
import com.mmcdole.kivo.android.mobile.lists.smartListBuilderRoute
import com.mmcdole.kivo.android.mobile.lists.smartListQuery
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.android.mobile.ui.LocalWide
import com.mmcdole.kivo.android.mobile.ui.CardShape
import com.mmcdole.kivo.android.mobile.ui.CollectionCard
import com.mmcdole.kivo.android.mobile.ui.Failed
import com.mmcdole.kivo.android.mobile.ui.Message
import com.mmcdole.kivo.android.mobile.ui.NamedCard
import com.mmcdole.kivo.android.mobile.ui.PageTopBar
import com.mmcdole.kivo.android.mobile.ui.WorkCard
import com.mmcdole.kivo.android.mobile.ui.WorkMenu
import com.mmcdole.kivo.android.mobile.ui.WorkMenuHost
import com.mmcdole.kivo.android.mobile.cast.CastButton
import com.mmcdole.kivo.android.mobile.ui.display
import com.mmcdole.kivo.core.data.GridQuery
import kotlinx.coroutines.launch
import kotlin.math.roundToInt

/**
 * A library: its tabs, the filters on as chips, the count line that opens
 * Sort, and the grid, with the letter rail at the right in title order.
 * Filter, in the top bar, opens the filters; both in bottom sheets.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun LibraryScreen(session: ServerSession, libraryId: String, oneOffSort: String?, nav: MobileNav) {
    val model = viewModel {
        LibraryViewModel(session.session, session.graph.savedGrids, session.gridChoices.value, libraryId, oneOffSort)
    }
    val state by model.state.collectAsStateWithLifecycle()
    var sheet by rememberSaveable { mutableStateOf<String?>(null) }
    val scroll = TopAppBarDefaults.exitUntilCollapsedScrollBehavior()
    Scaffold(
        modifier = Modifier.nestedScroll(scroll.nestedScrollConnection),
        topBar = {
            PageTopBar(state.library?.name.orEmpty(), nav::back, scroll, large = true) {
                if (state.library?.kind != "audiobooks") CastButton()
                if (state.tab == LibraryTab.Titles) FilterButton(state.query.filtered) { sheet = "filter" }
            }
        },
    ) { padding ->
        // The header scrolls away with the grid, and comes back as soon as it scrolls back.
        val header = remember { QuickReturn() }
        LaunchedEffect(state.grid) { header.offset = 0f }
        Box(Modifier.padding(padding).fillMaxSize().clipToBounds().nestedScroll(header)) {
            val top = with(LocalDensity.current) { header.height.toDp() }
            WorkMenuHost({ action, menu -> nav.handle(action, menu, model.marks) }) { openMenu ->
                Grid(state, model, nav, openMenu, top)
            }
            Header(
                state,
                model,
                Modifier
                    .onSizeChanged { header.height = it.height }
                    .offset { IntOffset(0, header.offset.roundToInt()) }
                    .background(MaterialTheme.colorScheme.background),
            ) { sheet = "sort" }
        }
    }
    when (sheet) {
        "sort" -> SortSheet(state, onSort = { model.chooseQuery(state.query.copy(sort = it)) }, onDismiss = { sheet = null })
        "filter" -> FilterSheet(
            state,
            model::chooseQuery,
            model::clearFilters,
            onSaveSmart = { nav.open(smartListBuilderRoute(state.library?.name.orEmpty(), smartListQuery(state.query, state.library?.kind))) },
            onDismiss = { sheet = null },
        )
    }
}

@Composable
private fun FilterButton(filtered: Boolean, onClick: () -> Unit) {
    IconButton(onClick = onClick, modifier = Modifier.testTag("filter")) {
        Box {
            Icon(Icons.Default.FilterList, stringResource(R.string.mobile_filter))
            if (filtered) Box(Modifier.align(Alignment.TopEnd).offset(x = 3.dp, y = (-3).dp).size(8.dp).background(MaterialTheme.colorScheme.primary, CircleShape))
        }
    }
}

/**
 * A header over a grid that scrolls away as the grid scrolls on and comes
 * back as it scrolls back, [height] high and moved [offset] up: it takes
 * what the grid scrolls after the top bar takes its share, without
 * consuming any.
 */
@Stable
private class QuickReturn : NestedScrollConnection {
    var height by mutableIntStateOf(0)
    var offset by mutableFloatStateOf(0f)

    override fun onPreScroll(available: Offset, source: NestedScrollSource): Offset {
        offset = (offset + available.y).coerceIn(-height.toFloat(), 0f)
        return Offset.Zero
    }
}

/** The tabs and the filters on, as chips, then the count line. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun Header(state: LibraryUiState, model: LibraryViewModel, modifier: Modifier, onSort: () -> Unit) = Column(modifier.fillMaxWidth()) {
    val query = state.query
    val chips = activeFilters(query)
    if (state.tabs.size > 1 || chips.isNotEmpty()) {
        Row(
            Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = space(4)),
            horizontalArrangement = Arrangement.spacedBy(space(2)),
        ) {
            if (state.tabs.size > 1) {
                for (tab in state.tabs) {
                    FilterChip(
                        selected = tab == state.tab,
                        onClick = { model.chooseTab(tab) },
                        label = { Text(tabName(tab, state.library?.kind)) },
                        modifier = Modifier.testTag("tab:${tab.name}"),
                    )
                }
            }
            for ((label, without) in chips) {
                InputChip(
                    selected = true,
                    onClick = { model.chooseQuery(without) },
                    label = { Text(label) },
                    trailingIcon = { Icon(Icons.Default.Close, stringResource(R.string.mobile_remove_filter), Modifier.size(16.dp)) },
                )
            }
        }
    }
    val total = state.grid.pager.state.collectAsStateWithLifecycle().value.total
    Row(
        Modifier
            .padding(horizontal = space(2))
            .then(if (state.tab == LibraryTab.Titles) Modifier.clickable(onClick = onSort).testTag("sort") else Modifier)
            .padding(horizontal = space(2), vertical = space(2)),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        val muted = MaterialTheme.colorScheme.onSurfaceVariant
        Text(total?.let { countLine(it, state) }.orEmpty(), style = MaterialTheme.typography.bodyMedium, color = muted)
        if (state.tab == LibraryTab.Titles) {
            Text(" · ", style = MaterialTheme.typography.bodyMedium, color = muted)
            Text(sortShort(query.sort), style = MaterialTheme.typography.labelLarge, color = muted)
            Icon(Icons.Default.ArrowDropDown, null, tint = muted)
        }
    }
}

/** The grid, its first row [top] down, under the header. */
@Composable
private fun Grid(state: LibraryUiState, model: LibraryViewModel, nav: MobileNav, openMenu: (WorkMenu) -> Unit, top: Dp) {
    val grid = state.grid
    val pages by grid.pager.state.collectAsStateWithLifecycle()
    val problem = pages.problem
    val total = pages.total
    val under = Modifier.padding(top = top)
    when {
        problem != null -> return Failed(problem, stringResource(R.string.mobile_couldnt_load_library), state.library?.name.orEmpty(), model::retry, under)
        total == 0 && state.tab == LibraryTab.Titles && state.query.filtered -> return Message(emptyFiltered(state), null, under) {
            Button(onClick = model::clearFilters) { Text(stringResource(R.string.mobile_clear_filters)) }
        }
        total == 0 -> return Message(stringResource(R.string.mobile_nothing_here), null, under)
    }
    // Each grid keeps its own scroll, across a page opened from it; a new tab, sort or filter starts at the top.
    val list = rememberSaveable(grid, saver = LazyGridState.Saver) { LazyGridState() }
    val scope = rememberCoroutineScope()
    var refreshing by remember { mutableStateOf(false) }
    val letters = remember(pages.letters) { railLetters(pages.letters.map { it.letter }) }
    val rail = letters.size > 1
    BoxWithConstraints(Modifier.fillMaxSize()) {
        val endPad = if (rail) LetterRailWidth + space(1) else space(4)
        val width = maxWidth - space(4) - endPad
        val wide = LocalWide.current
        val columns = gridColumns(width, state.shape, wide)
        val cardWidth = gridCardWidth(width, columns, wide)
        PullToRefreshBox(
            isRefreshing = refreshing,
            onRefresh = {
                refreshing = true
                scope.launch {
                    model.refresh()
                    refreshing = false
                }
            },
            modifier = Modifier.fillMaxSize(),
        ) {
            val padding = PaddingValues(start = space(4), end = endPad, top = top + space(1), bottom = space(6))
            if (total == null) {
                SkeletonGrid(columns, state.shape, cardWidth, padding)
            } else {
                Cells(grid, list, columns, padding, cardWidth, state.shape, nav, openMenu)
            }
        }
        if (rail && total != null) {
            // A letter tapped stays lit, wherever the grid can scroll to, until the grid is scrolled by hand.
            var tapped by remember(pages) { mutableStateOf<String?>(null) }
            LaunchedEffect(list) {
                list.interactionSource.interactions.collect { if (it is DragInteraction.Start) tapped = null }
            }
            val current by remember(pages) { derivedStateOf { tapped ?: pages.letterAt(list.firstVisibleItemIndex) } }
            LetterRail(
                letters,
                current,
                onLetter = { letter ->
                    tapped = letter
                    scope.launch { list.scrollToItem(pages.letterOffset(letter)) }
                },
                modifier = Modifier.align(Alignment.TopEnd).padding(top = top + space(2), bottom = space(2)),
            )
        }
    }
}

@Composable
private fun Cells(
    grid: LibraryGrid,
    list: LazyGridState,
    columns: Int,
    padding: PaddingValues,
    cardWidth: Dp,
    shape: CardShape,
    nav: MobileNav,
    openMenu: (WorkMenu) -> Unit,
) {
    when (grid) {
        is LibraryGrid.Works -> PagedGrid(grid.pager, list, columns, padding) { _, work ->
            WorkCard(
                work,
                shape,
                cardWidth,
                onClick = { if (work != null) nav.openWork(work) },
                onLongClick = { if (work != null) openMenu(WorkMenu(work)) },
                captionLines = if (shape == CardShape.Square) 2 else 1,
            )
        }
        is LibraryGrid.Collections -> PagedGrid(grid.pager, list, columns, padding) { _, collection ->
            CollectionCard(collection, cardWidth, { if (collection != null) nav.open(CollectionRoute(collection.id)) })
        }
        is LibraryGrid.Authors -> PagedGrid(grid.pager, list, columns, padding) { _, author ->
            NamedCard(author?.name, author?.workCount?.let { pluralStringResource(R.plurals.mobile_count_books, it, it) }, author?.photo, cardWidth, round = true, {
                if (author != null) nav.open(PersonRoute(author.id))
            })
        }
        is LibraryGrid.Series -> PagedGrid(grid.pager, list, columns, padding) { _, series ->
            NamedCard(series?.name, series?.bookCount?.let { pluralStringResource(R.plurals.mobile_count_books, it, it) }, series?.cover, cardWidth, round = false, {
                if (series != null) nav.open(BookSeriesRoute(series.id))
            })
        }
    }
}

/** A grid's placeholders while its first page is read: a screen of them, still. */
@Composable
private fun SkeletonGrid(columns: Int, shape: CardShape, cardWidth: Dp, padding: PaddingValues) {
    LazyVerticalGrid(
        columns = GridCells.Fixed(columns),
        contentPadding = padding,
        horizontalArrangement = Arrangement.spacedBy(gridGap(LocalWide.current)),
        verticalArrangement = Arrangement.spacedBy(GridLineGap),
        userScrollEnabled = false,
    ) {
        items(columns * 4) { WorkCard(null, shape, cardWidth, {}, null, captionLines = 1) }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SortSheet(state: LibraryUiState, onSort: (String) -> Unit, onDismiss: () -> Unit) {
    ModalBottomSheet(onDismissRequest = onDismiss, modifier = Modifier.testTag("sortSheet")) {
        Text(stringResource(R.string.mobile_sort_by), Modifier.padding(horizontal = space(6), vertical = space(2)), style = MaterialTheme.typography.titleMedium)
        for (sort in GridValues.sortsOf(state.library?.kind)) {
            ListItem(
                headlineContent = { Text(sortName(sort)) },
                supportingContent = { Text(sortDirection(sort)) },
                trailingContent = { if (sort == (state.query.sort ?: GridValues.TITLE)) Icon(Icons.Default.Check, null, tint = MaterialTheme.colorScheme.primary) },
                colors = ListItemDefaults.colors(containerColor = Color.Transparent),
                modifier = Modifier.clickable {
                    onSort(sort)
                    onDismiss()
                },
            )
        }
        Box(Modifier.navigationBarsPadding().padding(bottom = space(4)))
    }
}

@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
private fun FilterSheet(state: LibraryUiState, onQuery: (GridQuery) -> Unit, onClear: () -> Unit, onSaveSmart: () -> Unit, onDismiss: () -> Unit) {
    val query = state.query
    val facets = state.facets
    val books = state.library?.kind == "audiobooks"
    ModalBottomSheet(onDismissRequest = onDismiss, modifier = Modifier.testTag("filterSheet")) {
        Column(Modifier.padding(horizontal = space(6)).padding(bottom = space(6)), verticalArrangement = Arrangement.spacedBy(space(2))) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(stringResource(R.string.mobile_filter), Modifier.weight(1f), style = MaterialTheme.typography.titleMedium)
                if (query.filtered) TextButton(onClick = onClear) { Text(stringResource(R.string.mobile_clear_filters)) }
            }
            SheetLabel(stringResource(R.string.mobile_show))
            FlowRow(horizontalArrangement = Arrangement.spacedBy(space(2))) {
                for (played in listOf(null) + GridValues.playedStates) {
                    FilterChip(selected = query.played == played, onClick = { onQuery(query.copy(played = played)) }, label = { Text(playedName(played, books)) })
                }
            }
            val resolutions = GridValues.resolutions.filter { facets?.resolutions?.contains(it) == true }
            if (resolutions.isNotEmpty()) {
                SheetLabel(stringResource(R.string.mobile_resolution))
                FlowRow(horizontalArrangement = Arrangement.spacedBy(space(2))) {
                    for (r in resolutions) {
                        val on = query.resolutions.orEmpty()
                        FilterChip(selected = r in on, onClick = { onQuery(query.copy(resolutions = toggled(on, r))) }, label = { Text(resolutionName(r)) })
                    }
                }
            }
            val ranges = GridValues.dynamicRanges.filter { facets?.dynamicRanges?.contains(it) == true }
            if (ranges.isNotEmpty()) {
                SheetLabel(stringResource(R.string.mobile_dynamic_range))
                FlowRow(horizontalArrangement = Arrangement.spacedBy(space(2))) {
                    for (r in ranges) {
                        val on = query.dynamicRanges.orEmpty()
                        FilterChip(selected = r in on, onClick = { onQuery(query.copy(dynamicRanges = toggled(on, r))) }, label = { Text(dynamicRangeName(r)) })
                    }
                }
            }
            if (query.filtered) {
                TextButton(
                    onClick = {
                        onDismiss()
                        onSaveSmart()
                    },
                    modifier = Modifier.testTag("saveAsSmartList"),
                ) { Text(stringResource(R.string.mobile_smart_save_as)) }
            }
        }
    }
}

private fun toggled(values: List<String>, value: String) = (if (value in values) values - value else values + value).ifEmpty { null }

@Composable
private fun SheetLabel(text: String) {
    Text(text, Modifier.padding(top = space(2)), style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.onSurfaceVariant)
}

/** The filters on, each as a chip's label and the query without it. */
@Composable
private fun activeFilters(query: GridQuery): List<Pair<String, GridQuery>> = buildList {
    query.played?.let { add(playedName(it, false) to query.copy(played = null)) }
    query.resolutions?.forEach { add(resolutionName(it) to query.copy(resolutions = toggled(query.resolutions.orEmpty(), it))) }
    query.dynamicRanges?.forEach { add(dynamicRangeName(it) to query.copy(dynamicRanges = toggled(query.dynamicRanges.orEmpty(), it))) }
}

@Composable
private fun tabName(tab: LibraryTab, kind: String?): String = stringResource(
    when (tab) {
        LibraryTab.Titles -> when (kind) {
            "movies" -> R.string.mobile_tab_movies
            "shows" -> R.string.mobile_tab_shows
            "audiobooks" -> R.string.mobile_tab_books
            else -> R.string.mobile_tab_videos
        }
        LibraryTab.Collections -> R.string.mobile_collections
        LibraryTab.Authors -> R.string.mobile_authors
        LibraryTab.Series -> R.string.mobile_series
    },
)

/** "1,996 movies", "5 collections". */
@Composable
private fun countLine(total: Int, state: LibraryUiState): String {
    val plural = when (state.tab) {
        LibraryTab.Collections -> R.plurals.mobile_count_collections
        LibraryTab.Authors -> R.plurals.mobile_count_authors
        LibraryTab.Series -> R.plurals.mobile_count_series
        LibraryTab.Titles -> when (state.library?.kind) {
            "movies" -> R.plurals.mobile_count_movies
            "shows" -> R.plurals.mobile_count_shows
            "audiobooks" -> R.plurals.mobile_count_books
            else -> R.plurals.mobile_count_videos
        }
    }
    return pluralStringResource(plural, total, total)
}

/** "No unwatched movies" for the watch status alone, else "No movies match these filters". */
@Composable
private fun emptyFiltered(state: LibraryUiState): String {
    val query = state.query
    val noun = stringResource(
        when (state.library?.kind) {
            "movies" -> R.string.mobile_noun_movies
            "shows" -> R.string.mobile_noun_shows
            "audiobooks" -> R.string.mobile_noun_books
            else -> R.string.mobile_noun_videos
        },
    )
    val played = query.played
    return if (played != null && query.copy(played = null).filtered.not()) {
        stringResource(R.string.mobile_empty_played, playedName(played, state.library?.kind == "audiobooks").lowercase(), noun)
    } else {
        stringResource(R.string.mobile_empty_filters, noun)
    }
}

@Composable
private fun playedName(played: String?, books: Boolean): String = stringResource(
    when (played) {
        null -> R.string.mobile_all
        GridValues.UNPLAYED -> if (books) R.string.mobile_not_started else R.string.mobile_unwatched
        GridValues.IN_PROGRESS -> R.string.mobile_in_progress
        else -> if (books) R.string.mobile_finished else R.string.mobile_watched
    },
)

@Composable
private fun sortName(sort: String?) = stringResource(
    when (sort) {
        GridValues.ADDED_AT -> R.string.mobile_sort_added
        GridValues.LAST_EPISODE_ADDED_AT -> R.string.mobile_sort_last_episode_added
        GridValues.RELEASE_DATE -> R.string.mobile_sort_release
        GridValues.RATING -> R.string.mobile_sort_rating
        GridValues.RUNTIME -> R.string.mobile_sort_runtime
        GridValues.LAST_PLAYED_AT -> R.string.mobile_sort_last_played
        GridValues.AUTHOR -> R.string.mobile_sort_author
        else -> R.string.mobile_sort_title
    },
)

@Composable
private fun sortDirection(sort: String?) = stringResource(
    when (sort) {
        GridValues.RATING -> R.string.mobile_highest_first
        GridValues.ADDED_AT, GridValues.LAST_EPISODE_ADDED_AT, GridValues.RELEASE_DATE -> R.string.mobile_newest_first
        GridValues.RUNTIME -> R.string.mobile_shortest_first
        GridValues.LAST_PLAYED_AT -> R.string.mobile_most_recent_first
        else -> R.string.mobile_a_to_z
    },
)

/** The sort in a word or two, for the count line: "A–Z", "Recently added". */
@Composable
private fun sortShort(sort: String?) = stringResource(
    when (sort) {
        GridValues.ADDED_AT -> R.string.mobile_sort_short_added
        GridValues.LAST_EPISODE_ADDED_AT -> R.string.mobile_sort_last_episode_added
        GridValues.RELEASE_DATE -> R.string.mobile_sort_short_release
        GridValues.RATING -> R.string.mobile_sort_short_rating
        GridValues.RUNTIME -> R.string.mobile_sort_short_runtime
        GridValues.LAST_PLAYED_AT -> R.string.mobile_sort_short_last_played
        GridValues.AUTHOR -> R.string.mobile_sort_author
        else -> R.string.mobile_sort_short_title
    },
)

private fun resolutionName(r: String) = display.resolution(r)

@Composable
private fun dynamicRangeName(range: String) = stringResource(if (range == GridValues.DOLBY_VISION) R.string.mobile_dolby_vision else R.string.mobile_hdr)
