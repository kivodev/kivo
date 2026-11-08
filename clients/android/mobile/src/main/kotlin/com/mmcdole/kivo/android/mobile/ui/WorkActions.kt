package com.mmcdole.kivo.android.mobile.ui

import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Favorite
import androidx.compose.material.icons.filled.FavoriteBorder
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
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.display.episodeCode
import com.mmcdole.kivo.core.session.SessionLibrary
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch

/** What a card's long press offers. */
enum class WorkAction { Play, PlayFromStart, TogglePlayed, ToggleFavorite, RemoveFromContinue, GoToSeries }

/** A long-pressed card: the work, and the Home row it's in when that's a continue row. */
data class WorkMenu(val work: WorkSummary, val row: String? = null)

/**
 * The actions of [menu], in order: Play or Resume, Play from beginning once
 * there's progress, Mark watched, Favorite, Remove from the continue row it's
 * in, and Go to series for an episode, unless it's on [seriesPage]'s page.
 */
fun workActions(menu: WorkMenu, seriesPage: String? = null): List<WorkAction> = buildList {
    val work = menu.work
    if (playable(work)) {
        add(WorkAction.Play)
        if (work.userState?.positionMs != null) add(WorkAction.PlayFromStart)
    }
    add(WorkAction.TogglePlayed)
    add(WorkAction.ToggleFavorite)
    if (menu.row == CONTINUE_WATCHING || menu.row == CONTINUE_LISTENING) add(WorkAction.RemoveFromContinue)
    val series = work.episode?.seriesId
    if (series != null && series != seriesPage) add(WorkAction.GoToSeries)
}

const val CONTINUE_WATCHING = "continueWatching"
const val CONTINUE_LISTENING = "continueListening"

/**
 * The user's marks on works, for a screen's view model: each runs on its
 * [scope], and a failure leaves the card as the server has it, since the
 * live events redraw every screen showing the work.
 */
class WorkMarks(private val library: SessionLibrary, private val scope: CoroutineScope) {
    fun setPlayed(work: WorkSummary, played: Boolean) = act { library.setPlayed(work.id, played) }

    fun setFavorite(work: WorkSummary, favorite: Boolean) = act { library.setFavorite(work.id, favorite) }

    fun removeFromContinue(work: WorkSummary) = act { library.removeFromContinue(work.id) }

    /** Carries out [action]'s mark on [work]; the others are the screen's to do. */
    fun apply(action: WorkAction, work: WorkSummary) {
        when (action) {
            WorkAction.TogglePlayed -> setPlayed(work, work.userState?.played != true)
            WorkAction.ToggleFavorite -> setFavorite(work, work.userState?.favorite != true)
            WorkAction.RemoveFromContinue -> removeFromContinue(work)
            else -> Unit
        }
    }

    private fun act(call: suspend () -> Unit) {
        scope.launch {
            try {
                call()
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                // The card stays as the server has it.
            }
        }
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
    val favorite = work.userState?.favorite == true
    return when (action) {
        WorkAction.Play -> Icons.Default.PlayArrow to stringResource(if (work.userState?.positionMs != null) R.string.mobile_resume else R.string.mobile_play)
        WorkAction.PlayFromStart -> Icons.Default.Replay to stringResource(R.string.mobile_play_from_beginning)
        WorkAction.TogglePlayed -> (if (played) Icons.Default.CheckCircle else Icons.Outlined.CheckCircle) to markPlayedLabel(played, work.kind)
        WorkAction.ToggleFavorite -> (if (favorite) Icons.Default.Favorite else Icons.Default.FavoriteBorder) to
            stringResource(if (favorite) R.string.mobile_remove_favorite else R.string.mobile_add_favorite)
        WorkAction.RemoveFromContinue -> Icons.Default.RemoveCircleOutline to
            stringResource(if (menu.row == CONTINUE_LISTENING) R.string.mobile_remove_from_continue_listening else R.string.mobile_remove_from_continue_watching)
        WorkAction.GoToSeries -> Icons.AutoMirrored.Filled.ArrowForward to stringResource(R.string.mobile_go_to_series)
    }
}

/**
 * Gives [content] a way to open a card's actions, and shows them while
 * one is open; [onAction] does the one chosen.
 */
@Composable
fun WorkMenuHost(onAction: (WorkAction, WorkSummary) -> Unit, seriesPage: String? = null, content: @Composable (open: (WorkMenu) -> Unit) -> Unit) {
    var menu by remember { mutableStateOf<WorkMenu?>(null) }
    content { menu = it }
    menu?.let { open -> WorkActionsSheet(open, { onAction(it, open.work) }, { menu = null }, seriesPage) }
}
