package com.mmcdole.kivo.android.mobile.lists

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.clickable
import androidx.compose.foundation.focusable
import androidx.compose.foundation.gestures.detectDragGestures
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.filled.DragHandle
import androidx.compose.material3.Surface
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.semantics.CustomAccessibilityAction
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.customActions
import androidx.compose.ui.zIndex
import com.mmcdole.kivo.android.mobile.library.CountSortLine
import com.mmcdole.kivo.android.mobile.library.SortSheet
import com.mmcdole.kivo.android.mobile.ui.Artwork
import com.mmcdole.kivo.android.mobile.ui.cardCaption
import com.mmcdole.kivo.android.mobile.ui.cardTitle
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.data.ListSorts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.grid.LazyGridState
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.automirrored.filled.List
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.app.ListRoute
import com.mmcdole.kivo.android.mobile.app.MobileNav
import com.mmcdole.kivo.android.mobile.app.handle
import com.mmcdole.kivo.android.mobile.library.PagedGrid
import com.mmcdole.kivo.android.mobile.library.gridCardWidth
import com.mmcdole.kivo.android.mobile.library.gridColumns
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.android.mobile.ui.CardShape
import com.mmcdole.kivo.android.mobile.ui.Failed
import com.mmcdole.kivo.android.mobile.ui.Loading
import com.mmcdole.kivo.android.mobile.ui.LocalWide
import com.mmcdole.kivo.android.mobile.ui.Message
import com.mmcdole.kivo.android.mobile.ui.PageTopBar
import com.mmcdole.kivo.android.mobile.ui.TabTopBar
import com.mmcdole.kivo.android.mobile.ui.WorkCard
import com.mmcdole.kivo.android.mobile.ui.WorkMenu
import com.mmcdole.kivo.android.mobile.ui.WorkMenuHost
import com.mmcdole.kivo.android.mobile.ui.rowCardWidth
import com.mmcdole.kivo.android.mobile.ui.sentence
import com.mmcdole.kivo.android.mobile.ui.listShapeOf
import com.mmcdole.kivo.api.models.ListRef
import com.mmcdole.kivo.api.models.WorkList
import com.mmcdole.kivo.api.models.WorkListSmart
import com.mmcdole.kivo.api.models.WorkQuery
import com.mmcdole.kivo.core.RequestProblem
import kotlinx.coroutines.launch
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.text.style.TextAlign

/** The gap between a shelf's cards, and the page's side margin, as Home's. */
private val CardGap = 12.dp
private val SideMargin = 16.dp

/**
 * Lists: a shelf for each list, My List first and the others by name, its
 * name and count over a row of its first titles; the name opens the list.
 * The + makes a list, or opens the smart list builder. Pull to refresh; the
 * live events refresh it too.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ListsScreen(session: ServerSession, nav: MobileNav) {
    val model = viewModel { ListsViewModel(session.session) }
    val state by model.state.collectAsStateWithLifecycle()
    val scroll = TopAppBarDefaults.exitUntilCollapsedScrollBehavior()
    var naming by rememberSaveable { mutableStateOf(false) }
    Scaffold(
        modifier = Modifier.nestedScroll(scroll.nestedScrollConnection),
        topBar = {
            TabTopBar(stringResource(R.string.mobile_tab_lists), scroll, actions = {
                NewListMenu(onNewList = { naming = true }, onNewSmart = { nav.open(smartListBuilderRoute("", null)) })
            })
        },
    ) { padding ->
        val lists = state.lists
        val problem = state.problem
        Box(Modifier.padding(padding).fillMaxSize()) {
            when {
                lists == null && problem != null -> Failed(problem, stringResource(R.string.mobile_couldnt_load_lists), state.serverName, model::refresh)
                lists == null -> Loading()
                else -> PullToRefreshBox(state.refreshing, model::refresh, Modifier.fillMaxSize()) {
                    ListsContent(state, nav, model)
                }
            }
        }
    }
    if (naming) {
        ListNameDialog(
            title = stringResource(R.string.mobile_new_list),
            initial = "",
            confirm = stringResource(R.string.mobile_create),
            onSubmit = model::create,
            onDone = { naming = false },
        )
    }
}

@Composable
private fun NewListMenu(onNewList: () -> Unit, onNewSmart: () -> Unit) {
    var open by remember { mutableStateOf(false) }
    Box {
        IconButton(onClick = { open = true }, modifier = Modifier.testTag("newListMenu")) {
            Icon(Icons.Default.Add, stringResource(R.string.mobile_new_list_menu))
        }
        DropdownMenu(open, onDismissRequest = { open = false }) {
            DropdownMenuItem(text = { Text(stringResource(R.string.mobile_new_list)) }, onClick = { open = false; onNewList() }, leadingIcon = { Icon(Icons.AutoMirrored.Filled.List, null) }, modifier = Modifier.testTag("newList"))
            DropdownMenuItem(text = { Text(stringResource(R.string.mobile_smart_new)) }, onClick = { open = false; onNewSmart() }, leadingIcon = { Icon(Icons.Default.AutoAwesome, null, tint = MaterialTheme.colorScheme.primary) }, modifier = Modifier.testTag("newSmartList"))
        }
    }
}

@Composable
private fun ListsContent(state: ListsUiState, nav: MobileNav, model: ListsViewModel) {
    val shelves = state.shelves
    WorkMenuHost({ action, menu -> nav.handle(action, menu, model.marks) }) { openMenu ->
        LazyColumn(
            Modifier.fillMaxSize().testTag("lists"),
            contentPadding = PaddingValues(bottom = space(6)),
            verticalArrangement = Arrangement.spacedBy(space(7)),
        ) {
            if (shelves.isEmpty()) {
                item { Message(stringResource(R.string.mobile_no_lists), stringResource(R.string.mobile_no_lists_sentence), Modifier.height(400.dp)) }
            }
            items(shelves, key = { it.id }) { list -> Shelf(list, nav, openMenu) }
        }
    }
}

/** A list's shelf: its heading, which opens it, over its first titles as cards, or a placeholder for an empty list. */
@Composable
private fun Shelf(list: WorkList, nav: MobileNav, openMenu: (WorkMenu) -> Unit) {
    val works = list.items.orEmpty()
    val open = { nav.open(ListRoute(list.id, list.name)) }
    Column {
        ShelfHeading(list, open)
        if (works.isEmpty()) {
            EmptyShelf(Modifier.padding(horizontal = SideMargin).clickable(onClick = open))
        } else {
            LazyRow(contentPadding = PaddingValues(horizontal = SideMargin), horizontalArrangement = Arrangement.spacedBy(CardGap)) {
                items(works, key = { it.id }, contentType = { listShapeOf(it) }) { work ->
                    val shape = listShapeOf(work)
                    WorkCard(
                        work = work,
                        shape = shape,
                        width = rowCardWidth(shape),
                        onClick = { nav.openWork(work) },
                        onLongClick = { openMenu(WorkMenu(work, list = list.takeIf { it.isManual }?.let { ListRef(it.id, it.name, it.kind) })) },
                    )
                }
            }
        }
    }
}

/** A shelf's heading: the list's name and chevron, a smart list's mark, and its count. */
@Composable
private fun ShelfHeading(list: WorkList, onClick: () -> Unit) {
    Row(
        Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .heightIn(min = 48.dp)
            .padding(horizontal = SideMargin, vertical = space(2))
            .semantics(mergeDescendants = true) { heading() }
            .testTag("list:${list.name}"),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(list.name, Modifier.weight(1f, fill = false), style = MaterialTheme.typography.titleMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
        Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, null, tint = MaterialTheme.colorScheme.onSurfaceVariant)
        if (list.smart != null) {
            Icon(Icons.Default.AutoAwesome, null, Modifier.padding(start = space(1)).size(16.dp), tint = MaterialTheme.colorScheme.primary)
            Text(
                stringResource(R.string.mobile_smart_list),
                Modifier.padding(start = space(1), end = space(2)),
                style = MaterialTheme.typography.labelLarge,
                color = MaterialTheme.colorScheme.primary,
                maxLines = 1,
            )
        }
        Text(
            if (list.workCount == 0) stringResource(R.string.mobile_list_no_titles) else pluralStringResource(R.plurals.mobile_count_titles, list.workCount, list.workCount),
            Modifier.padding(start = space(1)),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
        )
    }
}

/** An empty list's shelf: a dashed outline the size of a poster, saying so. */
@Composable
private fun EmptyShelf(modifier: Modifier) {
    val outline = MaterialTheme.colorScheme.outlineVariant
    Box(
        modifier
            .width(rowCardWidth(CardShape.Poster))
            .aspectRatio(CardShape.Poster.aspect)
            .drawBehind {
                val stroke = 1.dp.toPx()
                val dash = 4.dp.toPx()
                val radius = 10.dp.toPx()
                drawRoundRect(
                    color = outline,
                    topLeft = Offset(stroke / 2, stroke / 2),
                    size = Size(size.width - stroke, size.height - stroke),
                    cornerRadius = CornerRadius(radius, radius),
                    style = Stroke(stroke, pathEffect = PathEffect.dashPathEffect(floatArrayOf(dash, dash))),
                )
            }
            .padding(space(3))
            .testTag("emptyShelf"),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            stringResource(R.string.mobile_list_empty_shelf),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center,
        )
    }
}

/**
 * An open list: its works in a grid, each card its own work's shape. A
 * manual list's are in its sort, which the count line chooses, and its
 * overflow has Reorder, which shows them as rows to move until Done; a
 * list the user made also has Rename and Delete there. A smart list has
 * its count and a line of its rules at the top, and Edit, which opens the
 * builder on its query.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ListScreen(session: ServerSession, id: String, name: String, nav: MobileNav) {
    val model = viewModel { ListViewModel(session.session, id) }
    val pages by model.pager.state.collectAsStateWithLifecycle()
    val list by model.list.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var refreshing by remember { mutableStateOf(false) }
    val grid = rememberSaveable(saver = LazyGridState.Saver) { LazyGridState() }
    var renaming by rememberSaveable { mutableStateOf(false) }
    var deleting by rememberSaveable { mutableStateOf(false) }
    var sorting by rememberSaveable { mutableStateOf(false) }
    val title = list?.name ?: name
    val smart = list?.smart
    val sort by model.sort.collectAsStateWithLifecycle()
    val reorder by model.reorder.collectAsStateWithLifecycle()
    BackHandler(reorder != null, model::endReorder)
    val rulesWords by model.rulesWords.collectAsStateWithLifecycle()
    val rulesProblem by model.problem.collectAsStateWithLifecycle()
    Scaffold(
        topBar = {
            PageTopBar(title, { if (reorder != null) model.endReorder() else nav.back() }) {
                val open = list
                when {
                    reorder != null -> TextButton(onClick = model::endReorder, modifier = Modifier.testTag("reorderDone")) {
                        Text(stringResource(R.string.mobile_done))
                    }
                    smart != null -> TextButton(onClick = { nav.open(smartListBuilderRoute(title, smart.query, id)) }, modifier = Modifier.testTag("smartListEdit")) {
                        Text(stringResource(R.string.mobile_smart_edit_short))
                    }
                }
                if (reorder == null && open != null && (open.isManual || !open.builtIn)) {
                    ListMenu(
                        onReorder = model::startReorder.takeIf { open.isManual },
                        onRename = { renaming = true }.takeIf { !open.builtIn },
                        onDelete = { deleting = true }.takeIf { !open.builtIn },
                    )
                }
            }
        },
    ) { padding ->
        Column(Modifier.padding(padding).fillMaxSize()) {
            val rows = reorder
            if (rows != null) {
                ReorderRows(rows.works, model::moveRow, model::saveMove, model::step)
                return@Column
            }
            pages.total?.let { total ->
                if (smart != null) SmartListLine(smart.query, total, rulesWords)
                sort?.let { CountSortLine(pluralStringResource(R.plurals.mobile_count_titles, total, total), listSortShort(it)) { sorting = true } }
            }
            BoxWithConstraints(Modifier.fillMaxSize()) {
                val width = maxWidth - space(8)
                val wide = LocalWide.current
                val columns = gridColumns(width, CardShape.Poster, wide)
                val cardWidth = gridCardWidth(width, columns, wide)
                val problem = pages.problem
                val total = pages.total
                when {
                    problem != null -> Failed(problem, stringResource(R.string.mobile_couldnt_load_lists), "", { scope.launch { model.refresh() } })
                    total == null -> Loading()
                    else -> PullToRefreshBox(refreshing, { scope.launch { refreshing = true; model.refresh(); refreshing = false } }, Modifier.fillMaxSize()) {
                        if (total == 0) {
                            // Scrollable, so a pull still refreshes it.
                            LazyColumn(Modifier.fillMaxSize()) {
                                item { Message(emptyMessage(smart), null, Modifier.height(400.dp)) }
                            }
                        } else {
                            WorkMenuHost({ action, menu -> nav.handle(action, menu, model.marks) }) { openMenu ->
                                PagedGrid(model.pager, grid, columns, PaddingValues(horizontal = space(4), vertical = space(2))) { _, work ->
                                    WorkCard(
                                        work,
                                        work?.let(::listShapeOf) ?: CardShape.Poster,
                                        cardWidth,
                                        { if (work != null) nav.openWork(work) },
                                        { if (work != null) openMenu(WorkMenu(work, list = list?.takeIf { it.isManual }?.let { ListRef(id, title, it.kind) })) },
                                        captionLines = 1,
                                    )
                                }
                            }
                        }
                    }
                }
            }
        }
    }
    rulesProblem?.let { failure ->
        AlertDialog(
            onDismissRequest = model::dismissProblem,
            text = { Text(sentence(failure)) },
            confirmButton = { TextButton(onClick = model::dismissProblem) { Text(stringResource(R.string.mobile_ok)) } },
        )
    }
    if (sorting) {
        SortSheet(ListSorts.all, sort ?: ListSorts.CUSTOM, { listSortName(it) }, { listSortDirection(it) }, model::chooseSort) { sorting = false }
    }
    if (renaming) {
        ListNameDialog(
            title = stringResource(R.string.mobile_rename_list),
            initial = title,
            confirm = stringResource(R.string.mobile_rename),
            onSubmit = model::rename,
            onDone = { renaming = false },
        )
    }
    if (deleting) {
        var problem by remember { mutableStateOf<RequestProblem?>(null) }
        AlertDialog(
            onDismissRequest = { deleting = false },
            title = { Text(stringResource(R.string.mobile_delete_named, title)) },
            text = { Text(problem?.let { sentence(it) } ?: stringResource(R.string.mobile_delete_list_note)) },
            confirmButton = {
                TextButton(onClick = {
                    scope.launch {
                        problem = model.delete()
                        if (problem == null) {
                            deleting = false
                            nav.back()
                        }
                    }
                }, Modifier.testTag("confirmDelete")) { Text(stringResource(R.string.mobile_delete), color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = { TextButton(onClick = { deleting = false }) { Text(stringResource(R.string.mobile_cancel)) } },
        )
    }
}

/** A smart list's count and rules: "42 movies. Comedy, no Animation". */
@Composable
private fun SmartListLine(query: WorkQuery, total: Int, words: SmartRulesWords?) {
    val count = smartCount(total, query.kinds)
    val rules = words?.let { rulesSummary(query, it.fields, it.names) }.orEmpty()
    Row(Modifier.fillMaxWidth().padding(horizontal = space(4)), horizontalArrangement = Arrangement.spacedBy(space(2))) {
        Icon(Icons.Default.AutoAwesome, null, Modifier.size(18.dp), tint = MaterialTheme.colorScheme.primary)
        Text(
            if (rules.isEmpty()) count else stringResource(R.string.mobile_smart_count_rules, count, rules),
            Modifier.testTag("smartListSummary"),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/** Why a list shows nothing: rules that no longer run, rules matching nothing, or nothing added. */
@Composable
private fun emptyMessage(smart: WorkListSmart?): String = when {
    smart == null -> stringResource(R.string.mobile_list_empty)
    !smart.errors.isNullOrEmpty() -> stringResource(R.string.mobile_smart_rules_failed)
    else -> stringResource(R.string.mobile_smart_nothing_matches)
}

/** A list's overflow: those of Reorder, Rename and Delete it has. */
@Composable
private fun ListMenu(onReorder: (() -> Unit)?, onRename: (() -> Unit)?, onDelete: (() -> Unit)?) {
    var open by remember { mutableStateOf(false) }
    Box {
        IconButton(onClick = { open = true }, modifier = Modifier.testTag("listMenu")) { Icon(Icons.Default.MoreVert, stringResource(R.string.mobile_more)) }
        DropdownMenu(open, onDismissRequest = { open = false }) {
            for ((label, tag, action) in listOf(
                Triple(R.string.mobile_reorder, "reorderList", onReorder),
                Triple(R.string.mobile_rename, "renameList", onRename),
                Triple(R.string.mobile_delete, "deleteList", onDelete),
            )) {
                if (action != null) DropdownMenuItem(text = { Text(stringResource(label)) }, onClick = { open = false; action() }, modifier = Modifier.testTag(tag))
            }
        }
    }
}

@Composable
private fun listSortName(sort: String) = stringResource(
    when (sort) {
        ListSorts.TITLE -> R.string.mobile_sort_title
        ListSorts.RELEASE_DATE -> R.string.mobile_sort_release
        ListSorts.RATING -> R.string.mobile_sort_rating
        ListSorts.DATE_ADDED -> R.string.mobile_sort_added_to_list
        else -> R.string.mobile_sort_custom
    },
)

/** Which way a sort runs; the custom order runs as the user put it. */
@Composable
private fun listSortDirection(sort: String): String? = when (sort) {
    ListSorts.TITLE -> R.string.mobile_a_to_z
    ListSorts.RELEASE_DATE, ListSorts.DATE_ADDED -> R.string.mobile_newest_first
    ListSorts.RATING -> R.string.mobile_highest_first
    else -> null
}?.let { stringResource(it) }

/** The sort in a word or two, for the count line. */
@Composable
private fun listSortShort(sort: String) = stringResource(
    when (sort) {
        ListSorts.TITLE -> R.string.mobile_sort_short_title
        ListSorts.RELEASE_DATE -> R.string.mobile_sort_short_release
        ListSorts.RATING -> R.string.mobile_sort_short_rating
        ListSorts.DATE_ADDED -> R.string.mobile_sort_short_added
        else -> R.string.mobile_sort_custom
    },
)

/** A reorder row's height, and its poster's width. */
private val ReorderRowHeight = 76.dp
private val ReorderPosterWidth = 40.dp

/**
 * Reorder: the list's works in its custom order as rows, each a small
 * poster, its title and the line under it, and a handle at the right. A
 * drag by the handle moves the row past the rows it crosses, [onMove], and
 * letting go saves its place, [onDrop]. The handle also takes Move up and
 * Move down from a screen reader, and the up and down keys, [onStep].
 */
@Composable
private fun ReorderRows(works: List<WorkSummary>?, onMove: (Int, Int) -> Unit, onDrop: (String) -> Unit, onStep: (Int, Int) -> Unit) {
    if (works == null) {
        Loading()
        return
    }
    val state = rememberLazyListState()
    val shown by rememberUpdatedState(works)
    var dragging by remember { mutableStateOf<String?>(null) }
    // How far the dragged row is from where the list lays it out.
    var lift by remember { mutableFloatStateOf(0f) }
    LazyColumn(Modifier.fillMaxSize().testTag("reorderRows"), state, contentPadding = PaddingValues(vertical = space(2))) {
        itemsIndexed(works, key = { _, work -> work.id }) { index, work ->
            val held = dragging == work.id
            val drag = Modifier.pointerInput(work.id) {
                detectDragGestures(
                    onDragStart = {
                        dragging = work.id
                        lift = 0f
                    },
                    onDragEnd = {
                        dragging = null
                        onDrop(work.id)
                    },
                    onDragCancel = {
                        dragging = null
                        onDrop(work.id)
                    },
                ) { change, amount ->
                    change.consume()
                    lift += amount.y
                    val layout = state.layoutInfo
                    val row = layout.visibleItemsInfo.firstOrNull { it.key == work.id } ?: return@detectDragGestures
                    // The list hasn't laid out the last move yet.
                    if (shown.getOrNull(row.index)?.id != work.id) return@detectDragGestures
                    val top = row.offset + lift
                    // Near an edge the list scrolls under the row.
                    val past = when {
                        top < layout.viewportStartOffset -> top - layout.viewportStartOffset
                        top + row.size > layout.viewportEndOffset -> top + row.size - layout.viewportEndOffset
                        else -> 0f
                    }
                    val middle = top + row.size / 2
                    if (past != 0f) lift += state.dispatchRawDelta(past)
                    val over = layout.visibleItemsInfo.firstOrNull { it.key != work.id && middle > it.offset && middle < it.offset + it.size }
                    if (over != null) {
                        // A move of the first row shown would scroll the list after it.
                        if (row.index == state.firstVisibleItemIndex || over.index == state.firstVisibleItemIndex) {
                            state.requestScrollToItem(state.firstVisibleItemIndex, state.firstVisibleItemScrollOffset)
                        }
                        lift += row.offset - over.offset
                        onMove(row.index, over.index)
                    }
                }
            }
            ReorderRow(
                work,
                held,
                if (held) Modifier.zIndex(1f).graphicsLayer { translationY = lift } else Modifier.animateItem(),
                drag,
                up = { onStep(index, -1) }.takeIf { index > 0 },
                down = { onStep(index, 1) }.takeIf { index < works.lastIndex },
            )
        }
    }
}

@Composable
private fun ReorderRow(work: WorkSummary, held: Boolean, modifier: Modifier, drag: Modifier, up: (() -> Unit)?, down: (() -> Unit)?) {
    val title = cardTitle(work)
    val moveUp = stringResource(R.string.mobile_move_up)
    val moveDown = stringResource(R.string.mobile_move_down)
    val handle = stringResource(R.string.mobile_reorder_work, title)
    Surface(
        modifier.fillMaxWidth().height(ReorderRowHeight).testTag("reorderRow:${work.id}"),
        color = if (held) MaterialTheme.colorScheme.surfaceContainerHigh else MaterialTheme.colorScheme.background,
        contentColor = MaterialTheme.colorScheme.onBackground,
        shadowElevation = if (held) 6.dp else 0.dp,
    ) {
        Row(Modifier.padding(start = space(4)), verticalAlignment = Alignment.CenterVertically) {
            Artwork(
                work.images.poster ?: work.images.still,
                ReorderPosterWidth,
                Modifier.width(ReorderPosterWidth).aspectRatio(CardShape.Poster.aspect).clip(RoundedCornerShape(4.dp)),
            )
            Column(Modifier.weight(1f).padding(horizontal = space(3))) {
                Text(title, style = MaterialTheme.typography.bodyLarge, maxLines = 1, overflow = TextOverflow.Ellipsis)
                cardCaption(work)?.let {
                    Text(it, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
            }
            Box(
                Modifier
                    .size(56.dp, ReorderRowHeight)
                    .onKeyEvent { event ->
                        val step = when (event.key) {
                            Key.DirectionUp -> up
                            Key.DirectionDown -> down
                            else -> null
                        }
                        if (step != null && event.type == KeyEventType.KeyDown) step()
                        step != null
                    }
                    .focusable()
                    .then(drag)
                    .semantics {
                        contentDescription = handle
                        customActions = listOfNotNull(
                            up?.let { CustomAccessibilityAction(moveUp) { it(); true } },
                            down?.let { CustomAccessibilityAction(moveDown) { it(); true } },
                        )
                    }
                    .testTag("reorderHandle:${work.id}"),
                contentAlignment = Alignment.Center,
            ) {
                Icon(Icons.Default.DragHandle, null, tint = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
    }
}

/**
 * A list's name, asked for in a dialog: [onSubmit] runs with it and says why
 * it was refused; a name in use shows under the field. [onDone] closes it
 * once it's saved or it's cancelled.
 */
@Composable
private fun ListNameDialog(
    title: String,
    initial: String,
    confirm: String,
    onSubmit: suspend (String) -> RequestProblem?,
    onDone: () -> Unit,
) {
    val scope = rememberCoroutineScope()
    var name by rememberSaveable { mutableStateOf(initial) }
    var problem by remember { mutableStateOf<RequestProblem?>(null) }
    AlertDialog(
        onDismissRequest = onDone,
        title = { Text(title) },
        text = {
            OutlinedTextField(
                value = name,
                onValueChange = {
                    name = it
                    problem = null
                },
                label = { Text(stringResource(R.string.mobile_list_name)) },
                singleLine = true,
                isError = problem != null,
                supportingText = problem?.let { failure -> { Text(nameProblem(failure)) } },
                modifier = Modifier.fillMaxWidth().testTag("listName"),
            )
        },
        confirmButton = {
            TextButton(
                onClick = {
                    scope.launch {
                        problem = onSubmit(name.trim())
                        if (problem == null) onDone()
                    }
                },
                enabled = name.isNotBlank(),
                modifier = Modifier.testTag("confirmName"),
            ) { Text(confirm) }
        },
        dismissButton = { TextButton(onClick = onDone) { Text(stringResource(R.string.mobile_cancel)) } },
    )
}

@Composable
internal fun nameProblem(problem: RequestProblem): String =
    if (problem.code == "nameTaken") stringResource(R.string.mobile_list_name_taken) else sentence(problem)

/** Add to List: New List, then every list ticked when the work's on it, a tap adding or removing it at once; a refused tap goes back and says why. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AddToListSheet(session: ServerSession, workId: String, onDismiss: () -> Unit) {
    val model = viewModel(key = "addToList/$workId") { AddToListViewModel(session.session, workId) }
    val choices by model.choices.collectAsStateWithLifecycle()
    val problem by model.problem.collectAsStateWithLifecycle()
    var naming by rememberSaveable { mutableStateOf(false) }
    ModalBottomSheet(onDismissRequest = onDismiss, modifier = Modifier.testTag("addToListSheet")) {
        Text(
            stringResource(R.string.mobile_add_to_list),
            Modifier.fillMaxWidth().padding(horizontal = space(6), vertical = space(2)),
            style = MaterialTheme.typography.titleMedium,
        )
        LazyColumn(Modifier.fillMaxWidth().heightIn(min = 200.dp)) {
            item {
                ListItem(
                    headlineContent = { Text(stringResource(R.string.mobile_new_list), color = MaterialTheme.colorScheme.primary) },
                    leadingContent = { Icon(Icons.Default.Add, null, tint = MaterialTheme.colorScheme.primary) },
                    colors = ListItemDefaults.colors(containerColor = Color.Transparent),
                    modifier = Modifier.clickable { naming = true }.testTag("newList"),
                )
            }
            itemsIndexed(choices.orEmpty(), key = { _, it -> it.list.id }) { _, choice ->
                ListItem(
                    headlineContent = { Text(choice.list.name, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                    supportingContent = { Text(pluralStringResource(R.plurals.mobile_count_titles, choice.list.workCount, choice.list.workCount)) },
                    trailingContent = { if (choice.holding) Icon(Icons.Default.Check, null, tint = MaterialTheme.colorScheme.primary) },
                    colors = ListItemDefaults.colors(containerColor = Color.Transparent),
                    modifier = Modifier.clickable { model.toggle(choice) }.testTag("choose:${choice.list.name}"),
                )
            }
        }
    }
    if (naming) {
        ListNameDialog(
            title = stringResource(R.string.mobile_new_list),
            initial = "",
            confirm = stringResource(R.string.mobile_create),
            onSubmit = model::create,
            onDone = { naming = false },
        )
    }
    problem?.let { failure ->
        AlertDialog(
            onDismissRequest = model::dismissProblem,
            text = { Text(sentence(failure)) },
            confirmButton = { TextButton(onClick = model::dismissProblem) { Text(stringResource(R.string.mobile_ok)) } },
        )
    }
}
