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
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyGridState
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.FilterList
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.InputChip
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.material3.rememberModalBottomSheetState
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
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.app.BookSeriesRoute
import com.mmcdole.kivo.android.mobile.app.CollectionRoute
import com.mmcdole.kivo.android.mobile.app.MobileNav
import com.mmcdole.kivo.android.mobile.app.PersonRoute
import com.mmcdole.kivo.android.mobile.app.handle
import com.mmcdole.kivo.android.mobile.lists.smartListBuilderRoute
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
import com.mmcdole.kivo.api.models.LibraryKind
import com.mmcdole.kivo.api.models.Resolution
import com.mmcdole.kivo.api.models.WorkQuery
import com.mmcdole.kivo.core.data.DynamicRange
import com.mmcdole.kivo.core.data.FilterGroupKind
import com.mmcdole.kivo.core.data.GridFilter
import com.mmcdole.kivo.core.data.GridItem
import com.mmcdole.kivo.core.data.GridPager
import com.mmcdole.kivo.core.data.GridSort
import com.mmcdole.kivo.core.data.LibraryTab
import com.mmcdole.kivo.core.data.WorkPlayState
import com.mmcdole.kivo.core.session.GridEmptyCase
import com.mmcdole.kivo.core.session.LibraryGridState
import kotlinx.coroutines.launch
import kotlin.math.roundToInt

/**
 * A library: its tabs, the filters on as chips, the count line that opens
 * Sort, and the grid, with the letter rail at the right in title order.
 * Filter, in the top bar, opens the filters; both in bottom sheets.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun LibraryScreen(session: ServerSession, libraryId: String, oneOffSort: GridSort?, nav: MobileNav) {
    val model = viewModel { LibraryViewModel(session.session, libraryId, oneOffSort) }
    val state by model.state.collectAsStateWithLifecycle()
    var sheet by rememberSaveable { mutableStateOf<String?>(null) }
    val scroll = TopAppBarDefaults.exitUntilCollapsedScrollBehavior()
    Scaffold(
        modifier = Modifier.nestedScroll(scroll.nestedScrollConnection),
        topBar = {
            PageTopBar(state.library?.name.orEmpty(), nav::back, scroll, large = true) {
                if (state.library?.kind != LibraryKind.Audiobooks) CastButton()
                if (state.tab == LibraryTab.Titles) FilterButton(!state.filters.isEmpty) { sheet = "filter" }
            }
        },
    ) { padding ->
        // The header scrolls away with the grid, and comes back as soon as it scrolls back.
        val header = remember { QuickReturn() }
        LaunchedEffect(state.pager) { header.offset = 0f }
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
        "sort" -> SortSheet(
            state.sorts,
            state.sort,
            { it.value },
            { sortName(it) },
            { sortDirection(it) },
            onSort = model.grid::choose,
            onDismiss = { sheet = null },
        )
        "filter" -> FilterSheet(
            state,
            model,
            onSaveSmart = { nav.open(smartListBuilderRoute(state.library?.name.orEmpty(), it)) },
            onDismiss = { sheet = null },
        )
    }
}

@Composable
private fun FilterButton(filtered: Boolean, onClick: () -> Unit) {
    IconButton(onClick = onClick, modifier = Modifier.testTag("filter")) {
        Box {
            Icon(Icons.Default.FilterList, stringResource(if (filtered) R.string.mobile_filter_on else R.string.mobile_filter))
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
private fun Header(state: LibraryGridState, model: LibraryViewModel, modifier: Modifier, onSort: () -> Unit) = Column(modifier.fillMaxWidth()) {
    val books = state.library?.kind == LibraryKind.Audiobooks
    val chips = state.activeFilters
    if (state.tabs.size > 1 || chips.isNotEmpty()) {
        Row(
            Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = space(4)),
            horizontalArrangement = Arrangement.spacedBy(space(2)),
        ) {
            if (state.tabs.size > 1) {
                for (tab in state.tabs) {
                    FilterChip(
                        selected = tab == state.tab,
                        onClick = { model.grid.choose(tab) },
                        label = { Text(tabName(tab, state.library?.kind)) },
                        modifier = Modifier.testTag("tab:${tab.name}"),
                    )
                }
            }
            for (chip in chips) {
                InputChip(
                    selected = true,
                    onClick = { model.grid.choose(state.filters.toggling(chip)) },
                    label = { Text(chipName(chip, books)) },
                    trailingIcon = { Icon(Icons.Default.Close, stringResource(R.string.mobile_remove_filter), Modifier.size(16.dp)) },
                )
            }
        }
    }
    val total = state.pager.state.collectAsStateWithLifecycle().value.total
    CountSortLine(total?.let { countLine(it, state) }.orEmpty(), if (state.tab == LibraryTab.Titles) sortShort(state.sort) else null, onSort)
}

/** The count line: how many there are and, with [sort], the sort in a word or two, the line opening the sort sheet. */
@Composable
internal fun CountSortLine(count: String, sort: String?, onSort: () -> Unit) {
    Row(
        Modifier
            .padding(horizontal = space(2))
            .then(if (sort != null) Modifier.clickable(onClick = onSort).testTag("sort") else Modifier)
            .padding(horizontal = space(2), vertical = space(2)),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        val muted = MaterialTheme.colorScheme.onSurfaceVariant
        Text(count, style = MaterialTheme.typography.bodyMedium, color = muted)
        if (sort != null) {
            Text(" · ", style = MaterialTheme.typography.bodyMedium, color = muted)
            Text(sort, style = MaterialTheme.typography.labelLarge, color = muted)
            Icon(Icons.Default.ArrowDropDown, null, tint = muted)
        }
    }
}

/** The grid, its first row [top] down, under the header. */
@Composable
private fun Grid(state: LibraryGridState, model: LibraryViewModel, nav: MobileNav, openMenu: (WorkMenu) -> Unit, top: Dp) {
    val grid = state.pager
    val shape = state.cardShape.card
    val pages by grid.state.collectAsStateWithLifecycle()
    val problem = pages.problem
    val total = pages.total
    val under = Modifier.padding(top = top)
    when {
        problem != null -> return Failed(problem, stringResource(R.string.mobile_couldnt_load_library), state.library?.name.orEmpty(), model::retry, under)
        total == 0 && state.emptyCase != GridEmptyCase.Unfiltered -> return Message(emptyFiltered(state), null, under) {
            Button(onClick = { model.grid.clearFilters() }) { Text(stringResource(R.string.mobile_clear_filters)) }
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
        val columns = gridColumns(width, shape, wide)
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
                SkeletonGrid(columns, shape, cardWidth, padding)
            } else {
                Cells(grid, state.tab, list, columns, padding, cardWidth, shape, nav, openMenu)
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

/** The grid's cards, a [tab]'s own placeholder while a card's page is read. */
@Composable
private fun Cells(
    grid: GridPager,
    tab: LibraryTab,
    list: LazyGridState,
    columns: Int,
    padding: PaddingValues,
    cardWidth: Dp,
    shape: CardShape,
    nav: MobileNav,
    openMenu: (WorkMenu) -> Unit,
) {
    PagedGrid(grid, list, columns, padding) { _, item ->
        when (tab) {
            LibraryTab.Titles -> {
                val work = (item as? GridItem.Work)?.work
                WorkCard(
                    work,
                    shape,
                    cardWidth,
                    onClick = { if (work != null) nav.openWork(work) },
                    onLongClick = { if (work != null) openMenu(WorkMenu(work)) },
                    captionLines = if (shape == CardShape.Square) 2 else 1,
                )
            }
            LibraryTab.Collections -> {
                val collection = (item as? GridItem.Collection)?.collection
                CollectionCard(collection, cardWidth, { if (collection != null) nav.open(CollectionRoute(collection.id)) })
            }
            LibraryTab.Authors -> {
                val author = (item as? GridItem.Author)?.author
                NamedCard(author?.name, author?.workCount?.let { pluralStringResource(R.plurals.mobile_count_books, it, it) }, author?.photo, cardWidth, round = true, {
                    if (author != null) nav.open(PersonRoute(author.id))
                })
            }
            LibraryTab.BookSeries -> {
                val series = (item as? GridItem.BookSeries)?.series
                NamedCard(series?.name, series?.bookCount?.let { pluralStringResource(R.plurals.mobile_count_books, it, it) }, series?.cover, cardWidth, round = false, {
                    if (series != null) nav.open(BookSeriesRoute(series.id))
                })
            }
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

/** Sort by: [sorts], each tagged by its [value] and named by [name] with which way it runs, from [direction], under it, and [selected] ticked. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun <T> SortSheet(
    sorts: List<T>,
    selected: T,
    value: (T) -> String,
    name: @Composable (T) -> String,
    direction: @Composable (T) -> String?,
    onSort: (T) -> Unit,
    onDismiss: () -> Unit,
) {
    ModalBottomSheet(onDismissRequest = onDismiss, modifier = Modifier.testTag("sortSheet")) {
        Text(stringResource(R.string.mobile_sort_by), Modifier.padding(horizontal = space(6), vertical = space(2)), style = MaterialTheme.typography.titleMedium)
        for (sort in sorts) {
            val runs = direction(sort)
            ListItem(
                headlineContent = { Text(name(sort)) },
                supportingContent = runs?.let { { Text(it) } },
                trailingContent = { if (sort == selected) Icon(Icons.Default.Check, null, tint = MaterialTheme.colorScheme.primary) },
                colors = ListItemDefaults.colors(containerColor = Color.Transparent),
                modifier = Modifier.testTag("sort:${value(sort)}").clickable {
                    onSort(sort)
                    onDismiss()
                },
            )
        }
        Box(Modifier.navigationBarsPadding().padding(bottom = space(4)))
    }
}

/**
 * Filters: watch status, then a group of chips for each filter the library
 * offers, each shown only when it has something. The grid behind follows
 * the choices once they've held still, and Show gives its count, the last
 * one known while the next is read, and closes the sheet.
 */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
private fun FilterSheet(state: LibraryGridState, model: LibraryViewModel, onSaveSmart: (WorkQuery) -> Unit, onDismiss: () -> Unit) {
    val books = state.library?.kind == LibraryKind.Audiobooks
    val draft = state.filters
    val total = state.pager.state.collectAsStateWithLifecycle().value.total
    var count by remember { mutableStateOf(total) }
    LaunchedEffect(total) { if (total != null) count = total }
    var allGenres by rememberSaveable { mutableStateOf(false) }
    var allNarrators by rememberSaveable { mutableStateOf(false) }
    val close = {
        model.grid.choose(model.state.value.filters)
        onDismiss()
    }
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    val scope = rememberCoroutineScope()
    ModalBottomSheet(onDismissRequest = close, sheetState = sheetState, modifier = Modifier.testTag("filterSheet")) {
        Box(Modifier.fillMaxWidth().padding(horizontal = space(3))) {
            Text(stringResource(R.string.mobile_filters), Modifier.align(Alignment.Center), style = MaterialTheme.typography.titleMedium)
            TextButton(
                onClick = { model.grid.clearFilters(settle = true) },
                enabled = !draft.isEmpty,
                modifier = Modifier.align(Alignment.CenterEnd).testTag("filtersClear"),
            ) { Text(stringResource(R.string.mobile_clear)) }
        }
        Column(
            Modifier
                .weight(1f, fill = false)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = space(5))
                .padding(top = space(2), bottom = space(4)),
            verticalArrangement = Arrangement.spacedBy(space(6)),
        ) {
            for (group in state.offer) {
                when (group.kind) {
                    FilterGroupKind.WatchStatus -> FilterGroup(stringResource(R.string.mobile_watch_status)) {
                        WatchStatus(draft.played, books) { played ->
                            val next = if (played == null) draft.clearing(FilterGroupKind.WatchStatus) else draft.choosing(GridFilter.Played(played))
                            model.grid.choose(next, settle = true)
                        }
                    }
                    else -> {
                        val whole = if (group.kind == FilterGroupKind.Genre) allGenres else allNarrators
                        val onWhole = { all: Boolean -> if (group.kind == FilterGroupKind.Genre) allGenres = all else allNarrators = all }
                        FilterGroup(groupName(group.kind), group.values.size, whole, onWhole.takeIf { group.isLong }) {
                            ChipFlow {
                                for (value in group.shown(whole, draft)) {
                                    SheetChip(valueName(value, books), draft.has(value), value.key) { model.grid.choose(draft.toggling(value), settle = true) }
                                }
                            }
                        }
                    }
                }
            }
            if (!draft.isEmpty) {
                TextButton(
                    onClick = {
                        close()
                        onSaveSmart(model.state.value.smartListQuery)
                    },
                    contentPadding = PaddingValues(0.dp),
                    modifier = Modifier.testTag("saveAsSmartList"),
                ) {
                    Icon(Icons.Default.AutoAwesome, null, Modifier.size(18.dp))
                    Text(stringResource(R.string.mobile_smart_save_as), Modifier.padding(start = space(2)))
                }
            }
        }
        HorizontalDivider()
        Button(
            onClick = { scope.launch { sheetState.hide() }.invokeOnCompletion { close() } },
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = space(5))
                .padding(top = space(3), bottom = space(2))
                .heightIn(min = 48.dp)
                .testTag("filtersShow"),
        ) {
            Text(count?.let { stringResource(R.string.mobile_show_count, countLine(it, state)) } ?: stringResource(R.string.mobile_show), style = MaterialTheme.typography.titleSmall)
        }
    }
}

/**
 * All, Unwatched, In progress, Watched, or a books library's Not started and
 * Finished: segments, or chips that wrap whole words when a segment can't
 * hold its word, as at large text.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun WatchStatus(played: WorkPlayState?, books: Boolean, onPlayed: (WorkPlayState?) -> Unit) {
    val states = listOf(null) + WorkPlayState.entries
    var cut by remember(books, LocalDensity.current.fontScale) { mutableStateOf(false) }
    if (cut) {
        ChipFlow {
            for (state in states) {
                SheetChip(playedName(state, books), played == state, "played:${state?.value.orEmpty()}") { onPlayed(state) }
            }
        }
        return
    }
    SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
        states.forEachIndexed { i, state ->
            SegmentedButton(
                selected = played == state,
                onClick = { onPlayed(state) },
                shape = SegmentedButtonDefaults.itemShape(i, states.size),
                icon = {},
                modifier = Modifier.testTag("played:${state?.value.orEmpty()}"),
            ) {
                Text(playedName(state, books), maxLines = 1, overflow = TextOverflow.Ellipsis, onTextLayout = { if (it.hasVisualOverflow) cut = true })
            }
        }
    }
}

/** A group: its [title], with All or Fewer of its [count] values when [onWhole] is given, as for a long one, over its [content]. */
@Composable
private fun FilterGroup(
    title: String,
    count: Int = 0,
    whole: Boolean = false,
    onWhole: ((Boolean) -> Unit)? = null,
    content: @Composable () -> Unit,
) = Column(verticalArrangement = Arrangement.spacedBy(space(1))) {
    Row(Modifier.fillMaxWidth().heightIn(min = 32.dp), verticalAlignment = Alignment.CenterVertically) {
        Text(title, Modifier.weight(1f).semantics { heading() }, style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.onSurfaceVariant)
        if (onWhole != null) {
            TextButton(onClick = { onWhole(!whole) }, contentPadding = PaddingValues(horizontal = space(2))) {
                Text(if (whole) stringResource(R.string.mobile_fewer) else stringResource(R.string.mobile_all_count, count))
            }
        }
    }
    content()
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun ChipFlow(content: @Composable () -> Unit) = FlowRow(
    horizontalArrangement = Arrangement.spacedBy(space(2)),
    verticalArrangement = Arrangement.spacedBy(space(2)),
) { content() }

/** A choice in the filter sheet: neutral, or filled with the accent while chosen. */
@Composable
private fun SheetChip(label: String, selected: Boolean, tag: String, onClick: () -> Unit) {
    val colors = MaterialTheme.colorScheme
    FilterChip(
        selected = selected,
        onClick = onClick,
        // At least its touch target wide, its label centred, so a one-letter chip starts where the others do.
        label = { Text(label, Modifier.widthIn(min = 28.dp), style = MaterialTheme.typography.bodyMedium, textAlign = TextAlign.Center) },
        modifier = Modifier.heightIn(min = 36.dp).testTag(tag),
        shape = CircleShape,
        colors = FilterChipDefaults.filterChipColors(
            containerColor = colors.surfaceContainerHighest,
            labelColor = colors.onSurface,
            selectedContainerColor = colors.primary,
            selectedLabelColor = colors.onPrimary,
        ),
        border = null,
    )
}

/** A filter that's on, as its chip names it. */
@Composable
private fun chipName(filter: GridFilter, books: Boolean): String = when (filter) {
    is GridFilter.Narrator -> filter.name?.let { stringResource(R.string.mobile_narrated_by, it) } ?: stringResource(R.string.mobile_narrator)
    else -> valueName(filter, books)
}

/** A value to filter by, as the sheet names it. */
@Composable
private fun valueName(filter: GridFilter, books: Boolean): String = when (filter) {
    is GridFilter.Played -> playedName(filter.state, books)
    is GridFilter.Genre -> filter.name
    is GridFilter.ContentRating -> filter.name
    is GridFilter.Decade -> decadeName(filter.year)
    is GridFilter.Resolution -> resolutionName(filter.resolution)
    is GridFilter.DynamicRange -> dynamicRangeName(filter.range)
    is GridFilter.Narrator -> filter.name.orEmpty()
}

@Composable
private fun groupName(group: FilterGroupKind): String = stringResource(
    when (group) {
        FilterGroupKind.WatchStatus -> R.string.mobile_watch_status
        FilterGroupKind.Decade -> R.string.mobile_decade
        FilterGroupKind.Genre -> R.string.mobile_genre
        FilterGroupKind.ContentRating -> R.string.mobile_content_rating
        FilterGroupKind.Quality -> R.string.mobile_quality
        FilterGroupKind.Narrator -> R.string.mobile_narrator
    },
)

/** "1990s". */
@Composable
private fun decadeName(decade: Int) = stringResource(R.string.mobile_decade_label, decade)

@Composable
private fun tabName(tab: LibraryTab, kind: LibraryKind?): String = stringResource(
    when (tab) {
        LibraryTab.Titles -> when (kind) {
            LibraryKind.Movies -> R.string.mobile_tab_movies
            LibraryKind.Shows -> R.string.mobile_tab_shows
            LibraryKind.Audiobooks -> R.string.mobile_tab_books
            else -> R.string.mobile_tab_videos
        }
        LibraryTab.Collections -> R.string.mobile_collections
        LibraryTab.Authors -> R.string.mobile_authors
        LibraryTab.BookSeries -> R.string.mobile_series
    },
)

/** "1,996 movies", "5 collections". */
@Composable
private fun countLine(total: Int, state: LibraryGridState): String {
    val plural = when (state.tab) {
        LibraryTab.Collections -> R.plurals.mobile_count_collections
        LibraryTab.Authors -> R.plurals.mobile_count_authors
        LibraryTab.BookSeries -> R.plurals.mobile_count_series
        LibraryTab.Titles -> when (state.library?.kind) {
            LibraryKind.Movies -> R.plurals.mobile_count_movies
            LibraryKind.Shows -> R.plurals.mobile_count_shows
            LibraryKind.Audiobooks -> R.plurals.mobile_count_books
            else -> R.plurals.mobile_count_videos
        }
    }
    return pluralStringResource(plural, total, total)
}

/** "No unwatched movies" for the watch status alone, else "No movies match these filters". */
@Composable
private fun emptyFiltered(state: LibraryGridState): String {
    val noun = stringResource(
        when (state.library?.kind) {
            LibraryKind.Movies -> R.string.mobile_noun_movies
            LibraryKind.Shows -> R.string.mobile_noun_shows
            LibraryKind.Audiobooks -> R.string.mobile_noun_books
            else -> R.string.mobile_noun_videos
        },
    )
    val played = state.filters.played
    return if (played != null && state.emptyCase == GridEmptyCase.WatchStatus) {
        val books = state.library?.kind == LibraryKind.Audiobooks
        val name = stringResource(
            when (played) {
                WorkPlayState.Unplayed -> if (books) R.string.mobile_state_unstarted else R.string.mobile_state_unwatched
                WorkPlayState.InProgress -> R.string.mobile_state_in_progress
                else -> if (books) R.string.mobile_state_finished else R.string.mobile_state_watched
            },
        )
        stringResource(R.string.mobile_empty_played, name, noun)
    } else {
        stringResource(R.string.mobile_empty_filters, noun)
    }
}

@Composable
private fun playedName(played: WorkPlayState?, books: Boolean): String = stringResource(
    when (played) {
        null -> R.string.mobile_all
        WorkPlayState.Unplayed -> if (books) R.string.mobile_not_started else R.string.mobile_unwatched
        WorkPlayState.InProgress -> R.string.mobile_in_progress
        else -> if (books) R.string.mobile_finished else R.string.mobile_watched
    },
)

@Composable
private fun sortName(sort: GridSort?) = stringResource(
    when (sort) {
        GridSort.AddedAt -> R.string.mobile_sort_added
        GridSort.LastEpisodeAddedAt -> R.string.mobile_sort_last_episode_added
        GridSort.ReleaseDate -> R.string.mobile_sort_release
        GridSort.Rating -> R.string.mobile_sort_rating
        GridSort.Runtime -> R.string.mobile_sort_runtime
        GridSort.LastPlayedAt -> R.string.mobile_sort_last_played
        GridSort.Author -> R.string.mobile_sort_author
        else -> R.string.mobile_sort_title
    },
)

@Composable
private fun sortDirection(sort: GridSort?) = stringResource(
    when (sort) {
        GridSort.Rating -> R.string.mobile_highest_first
        GridSort.AddedAt, GridSort.LastEpisodeAddedAt, GridSort.ReleaseDate -> R.string.mobile_newest_first
        GridSort.Runtime -> R.string.mobile_shortest_first
        GridSort.LastPlayedAt -> R.string.mobile_most_recent_first
        else -> R.string.mobile_a_to_z
    },
)

/** The sort in a word or two, for the count line: "A–Z", "Recently added". */
@Composable
private fun sortShort(sort: GridSort?) = stringResource(
    when (sort) {
        GridSort.AddedAt -> R.string.mobile_sort_short_added
        GridSort.LastEpisodeAddedAt -> R.string.mobile_sort_last_episode_added
        GridSort.ReleaseDate -> R.string.mobile_sort_short_release
        GridSort.Rating -> R.string.mobile_sort_short_rating
        GridSort.Runtime -> R.string.mobile_sort_short_runtime
        GridSort.LastPlayedAt -> R.string.mobile_sort_short_last_played
        GridSort.Author -> R.string.mobile_sort_author
        else -> R.string.mobile_sort_short_title
    },
)

private fun resolutionName(r: Resolution) = display.resolution(r)

@Composable
private fun dynamicRangeName(range: DynamicRange) = stringResource(if (range == DynamicRange.DolbyVision) R.string.mobile_dolby_vision else R.string.mobile_hdr)
