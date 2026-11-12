package com.mmcdole.kivo.android.mobile.ui

import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.automirrored.filled.PlaylistAdd
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.RemoveCircleOutline
import androidx.compose.material.icons.filled.Replay
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.api.models.HomeRowKind
import com.mmcdole.kivo.api.models.ListRef
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.display.episodeCode
import com.mmcdole.kivo.core.session.WorkMarks
import kotlinx.coroutines.launch

/** What a card's long press offers. */
enum class WorkAction { Play, PlayFromStart, TogglePlayed, AddToList, RemoveFromList, RemoveFromContinue, GoToSeries }

/** A long-pressed card: the work, the Home row it's in when that's a continue row, and the list it's shown in, when one. */
data class WorkMenu(val work: WorkSummary, val row: HomeRowKind? = null, val list: ListRef? = null)

/**
 * The actions of [menu], in order: Play or Resume, Play from beginning once
 * there's progress, Mark watched, Add to List, Remove from the list it's shown
 * in, Remove from the continue row it's in, and Go to series for an episode, unless it's on [seriesPage]'s page.
 */
fun workActions(menu: WorkMenu, seriesPage: String? = null): List<WorkAction> = buildList {
    val work = menu.work
    if (playable(work)) {
        add(WorkAction.Play)
        if (work.userState?.positionMs != null) add(WorkAction.PlayFromStart)
    }
    add(WorkAction.TogglePlayed)
    add(WorkAction.AddToList)
    if (menu.list != null) add(WorkAction.RemoveFromList)
    if (menu.row == HomeRowKind.ContinueWatching || menu.row == HomeRowKind.ContinueListening) add(WorkAction.RemoveFromContinue)
    val series = work.episode?.seriesId
    if (series != null && series != seriesPage) add(WorkAction.GoToSeries)
}


/** Carries out [action]'s mark on [work]; the others are the screen's to do. */
fun WorkMarks.mark(action: WorkAction, work: WorkSummary) {
    when (action) {
        WorkAction.TogglePlayed -> setPlayed(work.id, work.userState?.played != true)
        WorkAction.RemoveFromContinue -> removeFromContinue(work.id)
        else -> Unit
    }
}

/** A long-pressed card's actions, in a bottom sheet with a drag handle; [onAction] does the one chosen. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun WorkActionsSheet(menu: WorkMenu, onAction: (WorkAction) -> Unit, onDismiss: () -> Unit, seriesPage: String? = null) {
    val state = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    val scope = rememberCoroutineScope()
    val work = menu.work
    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = state, modifier = Modifier.testTag("workActions")) {
        Text(
            listOfNotNull(cardTitle(work), work.episode?.let { display.episodeCode(it) }).joinToString(" · "),
            Modifier.padding(horizontal = space(6), vertical = space(2)),
            style = MaterialTheme.typography.titleMedium,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        for (action in workActions(menu, seriesPage)) {
            val (icon, label) = actionLook(action, menu)
            ListItem(
                headlineContent = { Text(label) },
                leadingContent = { Icon(icon, null) },
                colors = ListItemDefaults.colors(containerColor = Color.Transparent),
                modifier = Modifier
                    .testTag("action:${action.name}")
                    .tapAndHold(
                        onClick = {
                            scope.launch { state.hide() }.invokeOnCompletion {
                                onDismiss()
                                onAction(action)
                            }
                        },
                        onLongClick = null,
                    ),
            )
        }
    }
}

@Composable
private fun actionLook(action: WorkAction, menu: WorkMenu): Pair<ImageVector, String> {
    val work = menu.work
    val played = work.userState?.played == true
    return when (action) {
        WorkAction.Play -> Icons.Default.PlayArrow to stringResource(if (work.userState?.positionMs != null) R.string.mobile_resume else R.string.mobile_play)
        WorkAction.PlayFromStart -> Icons.Default.Replay to stringResource(R.string.mobile_play_from_beginning)
        WorkAction.TogglePlayed -> (if (played) Icons.Default.CheckCircle else Icons.Outlined.CheckCircle) to markPlayedLabel(played, work.kind)
        WorkAction.AddToList -> Icons.AutoMirrored.Filled.PlaylistAdd to stringResource(R.string.mobile_add_to_list)
        WorkAction.RemoveFromList -> Icons.Default.RemoveCircleOutline to stringResource(R.string.mobile_remove_from_list, menu.list?.name.orEmpty())
        WorkAction.RemoveFromContinue -> Icons.Default.RemoveCircleOutline to
            stringResource(if (menu.row == HomeRowKind.ContinueListening) R.string.mobile_remove_from_continue_listening else R.string.mobile_remove_from_continue_watching)
        WorkAction.GoToSeries -> Icons.AutoMirrored.Filled.ArrowForward to stringResource(R.string.mobile_go_to_series)
    }
}

/**
 * Gives [content] a way to open a card's actions, and shows them while
 * one is open; [onAction] does the one chosen.
 */
@Composable
fun WorkMenuHost(onAction: (WorkAction, WorkMenu) -> Unit, seriesPage: String? = null, content: @Composable (open: (WorkMenu) -> Unit) -> Unit) {
    var menu by remember { mutableStateOf<WorkMenu?>(null) }
    content { menu = it }
    menu?.let { open -> WorkActionsSheet(open, { onAction(it, open) }, { menu = null }, seriesPage) }
}
