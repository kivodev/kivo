package com.mmcdole.kivo.android.mobile.lists

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.grid.LazyGridState
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
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
import androidx.compose.ui.draw.clip
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
import com.mmcdole.kivo.android.mobile.ui.Artwork
import com.mmcdole.kivo.android.mobile.ui.CardCorners
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
import com.mmcdole.kivo.android.mobile.ui.cardImage
import com.mmcdole.kivo.android.mobile.ui.sentence
import com.mmcdole.kivo.android.mobile.ui.shapeOf
import com.mmcdole.kivo.api.models.ListRef
import com.mmcdole.kivo.api.models.WorkList
import com.mmcdole.kivo.api.models.WorkListSmart
import com.mmcdole.kivo.api.models.WorkQuery
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.session.SessionLibrary
import kotlinx.coroutines.launch

/** The grouped rows' corners. */
private val GroupCorners = RoundedCornerShape(22.dp)

/** The gap between My List's covers. */
private val CoverGap = 8.dp

private val FirstCoverWidth = 44.dp

/**
 * Lists: My List as a row of covers with its count, then the lists the user
 * made, then the smart lists, each with its count; a section with nothing in
 * it is left out. The + makes a list, or opens the smart list builder. Pull
 * to refresh; the live events refresh it too.
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
                    ListsContent(state, session.session.library, nav)
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
            DropdownMenuItem(text = { Text(stringResource(R.string.mobile_new_list)) }, onClick = { open = false; onNewList() }, modifier = Modifier.testTag("newList"))
            DropdownMenuItem(text = { Text(stringResource(R.string.mobile_smart_new)) }, onClick = { open = false; onNewSmart() }, modifier = Modifier.testTag("newSmartList"))
        }
    }
}

@Composable
private fun ListsContent(state: ListsUiState, library: SessionLibrary, nav: MobileNav) {
    val myList = state.myList
    val manual = state.manual
    LazyColumn(
        Modifier.fillMaxSize().testTag("lists"),
        contentPadding = PaddingValues(start = space(4), end = space(4), top = space(2), bottom = space(6)),
        verticalArrangement = Arrangement.spacedBy(space(5)),
    ) {
        if (myList == null && manual.isEmpty() && state.smart.isEmpty()) {
            item { Message(stringResource(R.string.mobile_no_lists), stringResource(R.string.mobile_no_lists_sentence), Modifier.height(400.dp)) }
        }
        if (myList != null) {
            item(key = "myList") { MyListSection(myList, state.covers) { nav.open(ListRoute(myList.id, myList.name)) } }
        }
        if (manual.isNotEmpty()) {
            item(key = "manual") {
                ListsSection(stringResource(R.string.mobile_tab_lists), manual, library, nav) { pluralStringResource(R.plurals.mobile_count_titles, it.workCount, it.workCount) }
            }
        }
        if (state.smart.isNotEmpty()) {
            item(key = "smart") { ListsSection(stringResource(R.string.mobile_smart_lists), state.smart, library, nav) { smartDetail(it) } }
        }
    }
}

@Composable
private fun MyListSection(list: WorkList, covers: List<WorkSummary>, onClick: () -> Unit) {
    Column(
        Modifier.fillMaxWidth().clickable(onClick = onClick).testTag("list:${list.name}"),
        verticalArrangement = Arrangement.spacedBy(space(2)),
    ) {
        Row(Modifier.fillMaxWidth().semantics(mergeDescendants = true) { heading() }, verticalAlignment = Alignment.CenterVertically) {
            Text(list.name, style = MaterialTheme.typography.titleMedium)
            Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, null, tint = MaterialTheme.colorScheme.onSurfaceVariant)
            Spacer(Modifier.weight(1f))
            Text(list.workCount.toString(), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        BoxWithConstraints(Modifier.fillMaxWidth()) {
            val width = (maxWidth - CoverGap * (MY_LIST_COVERS - 1)) / MY_LIST_COVERS
            Row(horizontalArrangement = Arrangement.spacedBy(CoverGap)) {
                for (work in covers.take(MY_LIST_COVERS)) {
                    Artwork(cardImage(work, CardShape.Poster), width, Modifier.weight(1f, fill = false).aspectRatio(CardShape.Poster.aspect).clip(CardCorners))
                }
            }
        }
    }
}

/** A smart list's line under its name: its kinds and count. */
@Composable
private fun smartDetail(list: WorkList): String =
    stringResource(R.string.mobile_smart_list_detail, kindsPhrase(list.smart?.query?.kinds.orEmpty()), list.workCount)

/** A headed group of [lists], each with its first cover, name and [detail]. */
@Composable
private fun ListsSection(title: String, lists: List<WorkList>, library: SessionLibrary, nav: MobileNav, detail: @Composable (WorkList) -> String) {
    Column(verticalArrangement = Arrangement.spacedBy(space(2))) {
        Text(
            title,
            Modifier.semantics { heading() },
            style = MaterialTheme.typography.titleMedium,
        )
        Column(Modifier.fillMaxWidth().clip(GroupCorners).background(MaterialTheme.colorScheme.surfaceContainerHigh)) {
            lists.forEachIndexed { index, list ->
                if (index > 0) HorizontalDivider()
                ListItem(
                    headlineContent = { Text(list.name, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                    leadingContent = { FirstCover(library, list.id) },
                    supportingContent = { Text(detail(list)) },
                    trailingContent = { Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, null, tint = MaterialTheme.colorScheme.onSurfaceVariant) },
                    colors = ListItemDefaults.colors(containerColor = Color.Transparent),
                    modifier = Modifier.clickable { nav.open(ListRoute(list.id, list.name)) }.testTag("list:${list.name}"),
                )
            }
        }
    }
}

/** A list's most recently added work's poster, or an empty cover for a list without one. */
@Composable
private fun FirstCover(library: SessionLibrary, listId: String) {
    val load by remember(listId) { library.listWorks(listId, 1, null) }.collectAsStateWithLifecycle(Load.Loading)
    val work = load.getOrNull()?.items?.firstOrNull()
    Artwork(work?.let { cardImage(it, CardShape.Poster) }, FirstCoverWidth, Modifier.width(FirstCoverWidth).aspectRatio(CardShape.Poster.aspect).clip(CardCorners))
}

/**
 * An open list: its works in a grid, most recently added first, each card its
 * own work's shape; a list the user made has Rename and Delete in its
 * overflow. A smart list also has its count and a line of its rules at the
 * top, and Edit, which opens the builder on its query.
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
    val title = list?.name ?: name
    val smart = list?.smart
    val rulesWords by model.rulesWords.collectAsStateWithLifecycle()
    Scaffold(
        topBar = {
            PageTopBar(title, nav::back) {
                if (smart != null) {
                    TextButton(onClick = { nav.open(smartListBuilderRoute(title, smart.query, id)) }, modifier = Modifier.testTag("smartListEdit")) {
                        Text(stringResource(R.string.mobile_smart_edit_short))
                    }
                }
                if (list?.builtIn == false) {
                    ListMenu(onRename = { renaming = true }, onDelete = { deleting = true })
                }
            }
        },
    ) { padding ->
        Column(Modifier.padding(padding).fillMaxSize()) {
            if (smart != null) SmartListLine(smart.query, pages.total ?: list?.workCount ?: 0, rulesWords)
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
                                        work?.let(::shapeOf) ?: CardShape.Poster,
                                        cardWidth,
                                        { if (work != null) nav.openWork(work) },
                                        { if (work != null) openMenu(WorkMenu(work, list = list?.takeIf { it.isManual }?.let { ListRef(id, title) })) },
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
            text = problem?.let { failure -> { Text(sentence(failure)) } },
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
    Text(
        if (rules.isEmpty()) count else stringResource(R.string.mobile_smart_count_rules, count, rules),
        Modifier.fillMaxWidth().padding(horizontal = space(4)).testTag("smartListSummary"),
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

/** Why a list shows nothing: rules that no longer run, rules matching nothing, or nothing added. */
@Composable
private fun emptyMessage(smart: WorkListSmart?): String = when {
    smart == null -> stringResource(R.string.mobile_list_empty)
    !smart.errors.isNullOrEmpty() -> stringResource(R.string.mobile_smart_rules_failed)
    else -> stringResource(R.string.mobile_smart_nothing_matches)
}

@Composable
private fun ListMenu(onRename: () -> Unit, onDelete: () -> Unit) {
    var open by remember { mutableStateOf(false) }
    Box {
        IconButton(onClick = { open = true }, modifier = Modifier.testTag("listMenu")) { Icon(Icons.Default.MoreVert, stringResource(R.string.mobile_more)) }
        DropdownMenu(open, onDismissRequest = { open = false }) {
            DropdownMenuItem(text = { Text(stringResource(R.string.mobile_rename_list)) }, onClick = { open = false; onRename() }, modifier = Modifier.testTag("renameList"))
            DropdownMenuItem(text = { Text(stringResource(R.string.mobile_delete_list)) }, onClick = { open = false; onDelete() }, modifier = Modifier.testTag("deleteList"))
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

/** Add to List: New List, then every list ticked when the work's on it, adding and removing as they're tapped. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AddToListSheet(session: ServerSession, workId: String, onDismiss: () -> Unit) {
    val model = viewModel(key = "addToList/$workId") { AddToListViewModel(session.session, workId) }
    val choices by model.choices.collectAsStateWithLifecycle()
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
}
