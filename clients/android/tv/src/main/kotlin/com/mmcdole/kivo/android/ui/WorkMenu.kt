package com.mmcdole.kivo.android.ui

import android.view.KeyEvent
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.nativeKeyCode
import androidx.compose.ui.input.key.onKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mmcdole.kivo.android.tv.R
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.api.models.ListKind
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.display.episodeCode

/** Opens a work's menu: what a long press of OK, or the Menu key, on its card or row does. */
val LocalWorkMenu = staticCompositionLocalOf<(WorkSummary) -> Unit> { {} }

/** Opens the menu of a card in Home's Continue Watching, which can also remove it from the row. */
val LocalContinueWatchingMenu = staticCompositionLocalOf<(WorkSummary) -> Unit> { {} }

/** The Menu key on this card or row opens [work]'s menu. */
fun Modifier.menuKey(work: WorkSummary, open: (WorkSummary) -> Unit): Modifier = onKeyEvent { event ->
    if (event.key.nativeKeyCode != KeyEvent.KEYCODE_MENU) return@onKeyEvent false
    if (event.type == KeyEventType.KeyUp) open(work)
    true
}

/**
 * Gives the screens under it a work's menu: Play or Resume,
 * Play from beginning once there's progress, Mark as watched or unwatched,
 * Remove from the list that's open, Remove from Continue Watching in that row,
 * Go to series for an episode (except on its series' page), and Go to
 * library outside a library's grid.
 */
@Composable
fun WorkMenuHost(session: ServerSession, navigator: Navigator, content: @Composable () -> Unit) {
    var open by remember { mutableStateOf<Pair<WorkSummary, Boolean>?>(null) }
    CompositionLocalProvider(
        LocalWorkMenu provides { open = it to false },
        LocalContinueWatchingMenu provides { open = it to true },
        content = content,
    )
    open?.let { (work, continueWatching) -> WorkMenu(work, continueWatching, session, navigator) { open = null } }
}

@Composable
private fun WorkMenu(work: WorkSummary, continueWatching: Boolean, session: ServerSession, navigator: Navigator, onClose: () -> Unit) {
    val screen = navigator.current.screen
    val playable = work.available && work.kind in setOf(WorkKind.Movie, WorkKind.Episode, WorkKind.Video, WorkKind.Extra)
    val started = work.userState?.positionMs != null
    val played = work.userState?.played == true
    val lists by remember(session) { session.session.library.lists() }.collectAsStateWithLifecycle(Load.Loading)
    // A smart list's works are its rules', so only a manual list offers Remove.
    val openList = (screen as? Screen.ListWorks)?.takeIf { open -> lists.getOrNull()?.any { it.id == open.id && it.kind == ListKind.Manual } == true }
    val seriesId = work.episode?.seriesId
    val libraryId = work.libraryId
    val choices = buildList {
        if (playable) {
            add(Choice("play", stringResource(if (started) R.string.resume else R.string.play)))
            if (started) add(Choice("fromStart", stringResource(R.string.play_from_beginning)))
        }
        add(Choice("played", stringResource(if (played) R.string.mark_unplayed else R.string.mark_played)))
        if (openList != null) add(Choice("removeFromList", stringResource(R.string.remove_from_list, openList.name)))
        if (continueWatching) add(Choice("removeFromContinue", stringResource(R.string.remove_from_continue_watching)))
        if (seriesId != null && (screen as? Screen.Work)?.id != seriesId) add(Choice("series", stringResource(R.string.go_to_series)))
        if (screen !is Screen.Library && libraryId != null) add(Choice("library", stringResource(R.string.go_to_library)))
    }
    val marks = session.session.library.marks
    ChoicePanel(
        title = listOfNotNull(cardTitle(work), work.episode?.let(display::episodeCode)).joinToString(" · "),
        choices = choices,
        selected = null,
        onSelect = { id ->
            onClose()
            when (id) {
                "play" -> navigator.push(Screen.Player(work.id))
                "fromStart" -> navigator.push(Screen.Player(work.id, fromStart = true))
                "played" -> marks.setPlayed(work.id, !played)
                "removeFromList" -> openList?.let { list -> marks.setOnList(list.id, work.id, on = false) }
                "removeFromContinue" -> marks.removeFromContinue(work.id)
                "series" -> work.episode?.let { navigator.push(Screen.Work(it.seriesId, it)) }
                "library" -> libraryId?.let { navigator.open(Screen.Library(it)) }
            }
        },
        onDismiss = onClose,
    )
}
