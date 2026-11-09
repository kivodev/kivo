package com.mmcdole.kivo.android.mobile.downloads

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.ErrorOutline
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SwipeToDismissBox
import androidx.compose.material3.SwipeToDismissBoxValue
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.rememberSwipeToDismissBoxState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import kotlinx.coroutines.launch
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import coil3.compose.AsyncImage
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.app.MobileNav
import com.mmcdole.kivo.android.mobile.app.ShowDownloadsRoute
import com.mmcdole.kivo.android.mobile.theme.LocalKivo
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.android.mobile.ui.wideWidth
import com.mmcdole.kivo.android.mobile.ui.CardCorners
import com.mmcdole.kivo.android.mobile.ui.Loading
import com.mmcdole.kivo.android.mobile.ui.Message
import com.mmcdole.kivo.android.mobile.ui.PageTopBar
import com.mmcdole.kivo.android.mobile.ui.TabTopBar
import com.mmcdole.kivo.android.mobile.ui.display
import com.mmcdole.kivo.android.mobile.ui.tapAndHold
import com.mmcdole.kivo.core.display.size
import com.mmcdole.kivo.core.downloads.DownloadItem
import com.mmcdole.kivo.core.downloads.DownloadStatus
import com.mmcdole.kivo.core.downloads.WaitReason
import java.io.File

/**
 * Downloads: Books, Movies and Shows, a show opening to its episodes by
 * season, each with its artwork, title and one line of state, newest first,
 * and while it's under way a ring at its end. A row swipes away to delete, a
 * show all its episodes once confirmed; a long press offers what the row can
 * do. Its foot gives the space they take and what's free. A ready download
 * plays here from its files, with or without the server.
 */
@Composable
fun DownloadsScreen(session: ServerSession, nav: MobileNav) {
    val downloads = session.session.downloads ?: return
    val model = viewModel { DownloadsViewModel(downloads) }
    val state by model.state.collectAsStateWithLifecycle()
    DisposableEffect(model) { onDispose(model::deletionsSeen) }
    DownloadsList(state, rowActions(model, nav))
}

/** What a row of Downloads does: play a ready one, open a show, delete, pause, resume or retry. */
class RowActions(
    val play: (DownloadItem) -> Unit,
    val openShow: (ShowDownloads) -> Unit,
    val delete: (List<String>) -> Unit,
    val pause: (String) -> Unit,
    val resume: (String) -> Unit,
    val retry: (String) -> Unit,
    val problemSeen: () -> Unit,
)

private fun rowActions(model: DownloadsViewModel, nav: MobileNav) = RowActions(
    play = nav::playDownload,
    openShow = { nav.open(ShowDownloadsRoute(it.seriesId, it.title)) },
    delete = model::delete,
    pause = model::pause,
    resume = model::resume,
    retry = model::retry,
    problemSeen = model::problemSeen,
)

/** The Downloads tab as [state] has it. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DownloadsList(state: DownloadsUiState, actions: RowActions) {
    val scroll = TopAppBarDefaults.exitUntilCollapsedScrollBehavior()
    val notice = deletedNotice(state.deleted)
    Scaffold(
        modifier = Modifier.nestedScroll(scroll.nestedScrollConnection),
        topBar = { TabTopBar(stringResource(R.string.mobile_tab_downloads), scroll) },
    ) { padding ->
        when {
            !state.read -> Loading(Modifier.padding(padding))
            state.empty -> Message(
                stringResource(R.string.mobile_no_downloads),
                listOfNotNull(stringResource(R.string.mobile_no_downloads_sentence), notice).joinToString("\n\n"),
                Modifier.padding(padding),
            )
            else -> LazyColumn(Modifier.wideWidth().fillMaxSize().testTag("downloads"), contentPadding = padding) {
                notice?.let { item(key = "deleted") { Note(it) } }
                section(R.string.mobile_books, state.books, actions)
                section(R.string.mobile_movies, state.movies, actions)
                if (state.shows.isNotEmpty()) {
                    item(key = "showsHeading") { Heading(stringResource(R.string.mobile_shows)) }
                    items(state.shows, key = { "show:" + it.seriesId }) { show ->
                        ShowRow(show, onOpen = { actions.openShow(show) }, onDelete = { actions.delete(show.episodes.map { it.id }) })
                    }
                }
                item(key = "storage") { StorageLine(state) }
            }
        }
    }
    Problem(state.problem, actions.problemSeen)
}

/** A show's downloaded episodes, by season and number. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ShowDownloadsScreen(session: ServerSession, seriesId: String, title: String, nav: MobileNav) {
    val downloads = session.session.downloads ?: return
    val model = viewModel(key = "show:$seriesId") { DownloadsViewModel(downloads, seriesId) }
    val state by model.state.collectAsStateWithLifecycle()
    // Once its last episode goes, the page goes too.
    LaunchedEffect(state.empty) { if (state.empty) nav.back() }
    val actions = rowActions(model, nav)
    Scaffold(topBar = { PageTopBar(title, nav::back) }) { padding ->
        LazyColumn(Modifier.wideWidth().fillMaxSize().testTag("showDownloads"), contentPadding = padding) {
            for (season in state.seasons) {
                val heading = if (season.season == 0) R.string.mobile_specials else null
                item(key = "season:${season.season}") {
                    Heading(heading?.let { stringResource(it) } ?: stringResource(R.string.mobile_season_n, season.season))
                }
                items(season.episodes, key = { it.id }) { DownloadRow(it, actions, numbered = true) }
            }
            item(key = "storage") { StorageLine(state) }
        }
    }
    Problem(state.problem, model::problemSeen)
}

private fun LazyListScope.section(heading: Int, items: List<DownloadItem>, actions: RowActions) {
    if (items.isEmpty()) return
    item(key = "heading:$heading") { Heading(stringResource(heading)) }
    items(items, key = { it.id }) { DownloadRow(it, actions) }
}

@Composable
private fun deletedNotice(count: Int): String? =
    if (count > 0) pluralStringResource(R.plurals.mobile_downloads_removed, count, count) else null

@Composable
private fun Heading(text: String) {
    Text(text, Modifier.padding(start = space(4), end = space(4), top = space(4), bottom = space(1)), style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.primary)
}

@Composable
private fun Note(text: String) {
    Text(text, Modifier.padding(horizontal = space(4), vertical = space(2)), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
}

/** "3.2 GB used · 41 GB free". */
@Composable
private fun StorageLine(state: DownloadsUiState) {
    val storage = state.storage ?: return
    Text(
        stringResource(R.string.mobile_storage_line, display.size(storage.usedBytes), display.size(storage.freeBytes)),
        Modifier.fillMaxWidth().padding(space(4)).testTag("storage"),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        textAlign = TextAlign.Center,
    )
}

@Composable
private fun Problem(problem: String?, onSeen: () -> Unit) {
    if (problem == null) return
    AlertDialog(
        onDismissRequest = onSeen,
        text = { Text(problem) },
        confirmButton = { TextButton(onClick = onSeen) { Text(stringResource(R.string.mobile_ok)) } },
    )
}

/**
 * One download: its artwork, title and line of state, and at its end a ring
 * while it's under way, Pause in it while it comes or waits and Resume while
 * it's paused, Retry when it failed, and nothing once it's here. On a show's
 * page ([numbered]) an episode's number leads its title. Tapping a ready one
 * plays it from its files; it swipes away to delete, and a long press offers
 * the same actions as its end and Delete.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun DownloadRow(item: DownloadItem, model: RowActions, numbered: Boolean = false) {
    var actions by remember { mutableStateOf(false) }
    Swipeable(onDelete = { model.delete(listOf(item.id)) }) {
        Row(
            Modifier
                .fillMaxWidth()
                .background(MaterialTheme.colorScheme.background)
                .tapAndHold(onClick = { if (item.status == DownloadStatus.Ready) model.play(item) }, onLongClick = { actions = true })
                .padding(horizontal = space(4), vertical = space(2))
                .testTag("download:${item.title}"),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(space(3)),
        ) {
            val (width, height) = when (item.kind) {
                "episode" -> 96.dp to 54.dp
                "book" -> 56.dp to 56.dp
                else -> 56.dp to 84.dp
            }
            LocalArtwork(item.artworkPath, Modifier.size(width, height))
            RowText(downloadTitle(item, numbered), downloadLine(item), failed = item.status is DownloadStatus.Failed, Modifier.weight(1f))
            RowEnd(item, model)
        }
    }
    if (actions) {
        ActionsSheet(item.title, rowActions(item), onDismiss = { actions = false }) { action ->
            when (action) {
                RowAction.Pause -> model.pause(item.id)
                RowAction.Resume -> model.resume(item.id)
                RowAction.Retry -> model.retry(item.id)
                RowAction.Delete -> model.delete(listOf(item.id))
            }
        }
    }
}

/** The end of a row: what it can do while it's under way, or failed; nothing once it's here. */
@Composable
private fun RowEnd(item: DownloadItem, model: RowActions) {
    val progress = DownloadWords.progress(item)
    Box(Modifier.size(48.dp), contentAlignment = Alignment.Center) {
        when (val status = item.status) {
            is DownloadStatus.Downloading -> RingButton(progress, Icons.Default.Pause, stringResource(R.string.mobile_pause_download), "pause") { model.pause(item.id) }
            is DownloadStatus.Waiting -> if (status.reason == WaitReason.Turn) {
                progress?.let { ProgressRing(it) }
            } else {
                RingButton(progress, Icons.Default.Pause, stringResource(R.string.mobile_pause_download), "pause") { model.pause(item.id) }
            }
            is DownloadStatus.Paused -> RingButton(progress, Icons.Default.Download, stringResource(R.string.mobile_resume_download), "resume") { model.resume(item.id) }
            is DownloadStatus.Failed -> IconButton(onClick = { model.retry(item.id) }, Modifier.testTag("retry")) {
                Icon(Icons.Default.Refresh, stringResource(R.string.mobile_retry))
            }
            DownloadStatus.Ready -> Unit
            else -> progress?.let { ProgressRing(it) }
        }
    }
}

@Composable
private fun RingButton(progress: DownloadProgress?, center: ImageVector, label: String, tag: String, onClick: () -> Unit) {
    // The ring says the state; the button says what it does.
    IconButton(onClick = onClick, Modifier.testTag(tag).semantics { contentDescription = label }) {
        if (progress != null) ProgressRing(progress, center = center) else Icon(center, null)
    }
}

/**
 * A show in the list: its poster, its name, and how many episodes are here
 * and their size, or while any is coming one state for the whole show. At
 * its end, a ring while any is coming, a warning when one failed, else a
 * chevron. It swipes away to delete every episode, once confirmed.
 */
@Composable
private fun ShowRow(show: ShowDownloads, onOpen: () -> Unit, onDelete: () -> Unit) {
    var confirming by remember { mutableStateOf(false) }
    var actions by remember { mutableStateOf(false) }
    Swipeable(onDelete = { confirming = true }) {
        Row(
            Modifier
                .fillMaxWidth()
                .background(MaterialTheme.colorScheme.background)
                .tapAndHold(onClick = onOpen, onLongClick = { actions = true })
                .padding(horizontal = space(4), vertical = space(2))
                .testTag("show:${show.title}"),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(space(3)),
        ) {
            LocalArtwork(show.episodes.firstNotNullOfOrNull { it.posterPath } ?: show.episodes.first().artworkPath, Modifier.size(56.dp, 84.dp))
            RowText(show.title, showLine(show.episodes), failed = show.episodes.all { it.status is DownloadStatus.Failed }, Modifier.weight(1f))
            Box(Modifier.size(44.dp), contentAlignment = Alignment.Center) {
                val progress = DownloadWords.progress(show.episodes)
                when {
                    show.episodes.any { it.status is DownloadStatus.Failed } -> Icon(Icons.Default.ErrorOutline, stringResource(R.string.mobile_download_failed), tint = MaterialTheme.colorScheme.error)
                    progress != null -> ProgressRing(progress)
                    else -> Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, null, tint = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        }
    }
    if (actions) ActionsSheet(show.title, listOf(RowAction.Delete), onDismiss = { actions = false }) { confirming = true }
    if (confirming) {
        val count = show.episodes.size
        AlertDialog(
            onDismissRequest = { confirming = false },
            title = { Text(pluralStringResource(R.plurals.mobile_delete_episodes, count, count, display.size(show.episodes.sumOf { it.sizeBytes }))) },
            confirmButton = {
                TextButton(onClick = {
                    confirming = false
                    onDelete()
                }, Modifier.testTag("confirmDelete")) { Text(stringResource(R.string.mobile_delete), color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = { TextButton(onClick = { confirming = false }) { Text(stringResource(R.string.mobile_cancel)) } },
        )
    }
}

/** A row that swipes from the end to [onDelete], over a red Delete. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun Swipeable(onDelete: () -> Unit, content: @Composable () -> Unit) {
    val state = rememberSwipeToDismissBoxState()
    val scope = rememberCoroutineScope()
    SwipeToDismissBox(
        state = state,
        enableDismissFromStartToEnd = false,
        onDismiss = { value ->
            if (value == SwipeToDismissBoxValue.EndToStart) onDelete()
            scope.launch { state.reset() }
        },
        backgroundContent = {
            Box(Modifier.fillMaxSize().background(MaterialTheme.colorScheme.error).padding(horizontal = space(6)), contentAlignment = Alignment.CenterEnd) {
                Icon(Icons.Default.Delete, stringResource(R.string.mobile_delete), tint = MaterialTheme.colorScheme.onError)
            }
        },
    ) { content() }
}

@Composable
private fun RowText(title: String, line: String, failed: Boolean, modifier: Modifier = Modifier) {
    Column(modifier) {
        Text(title, style = MaterialTheme.typography.titleSmall)
        Text(
            line,
            style = MaterialTheme.typography.bodySmall,
            color = if (failed) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.testTag("line"),
        )
    }
}

/** Artwork from a download's file, cropped to fill; the placeholder colour without one. */
@Composable
fun LocalArtwork(path: String?, modifier: Modifier = Modifier) {
    val placeholder = LocalKivo.current.placeholder
    Box(modifier.clip(CardCorners).background(placeholder)) {
        if (path != null) AsyncImage(File(path), null, Modifier.matchParentSize(), contentScale = ContentScale.Crop)
    }
}

/** What a long-pressed row can do. */
private enum class RowAction { Pause, Resume, Retry, Delete }

private fun rowActions(item: DownloadItem): List<RowAction> = listOfNotNull(
    RowAction.Pause.takeIf { item.status is DownloadStatus.Downloading || item.status is DownloadStatus.Waiting },
    RowAction.Resume.takeIf { item.status is DownloadStatus.Paused },
    RowAction.Retry.takeIf { item.status is DownloadStatus.Failed },
    RowAction.Delete,
)

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ActionsSheet(title: String, actions: List<RowAction>, onDismiss: () -> Unit, onAction: (RowAction) -> Unit) {
    ModalBottomSheet(onDismissRequest = onDismiss, modifier = Modifier.testTag("downloadActions")) {
        Text(title, Modifier.padding(horizontal = space(6), vertical = space(2)), style = MaterialTheme.typography.titleMedium)
        for (action in actions) {
            val (icon, label) = when (action) {
                RowAction.Pause -> Icons.Default.Pause to stringResource(R.string.mobile_pause_download)
                RowAction.Resume -> Icons.Default.Download to stringResource(R.string.mobile_resume_download)
                RowAction.Retry -> Icons.Default.Refresh to stringResource(R.string.mobile_retry)
                RowAction.Delete -> Icons.Default.Delete to stringResource(R.string.mobile_delete)
            }
            ListItem(
                headlineContent = { Text(label, color = if (action == RowAction.Delete) MaterialTheme.colorScheme.error else Color.Unspecified) },
                leadingContent = { Icon(icon, null, tint = if (action == RowAction.Delete) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurface) },
                colors = ListItemDefaults.colors(containerColor = Color.Transparent),
                modifier = Modifier.testTag("action:${action.name}").tapAndHold(onClick = {
                    onDismiss()
                    onAction(action)
                }, onLongClick = null),
            )
        }
        Box(Modifier.padding(bottom = space(6)))
    }
}
