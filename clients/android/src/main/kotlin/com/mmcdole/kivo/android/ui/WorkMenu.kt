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
import com.mmcdole.kivo.android.R
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.display.episodeCode
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.launch

/** Opens a work's menu: what a long press of OK, or the Menu key, on its card or row does. */
val LocalWorkMenu = staticCompositionLocalOf<(WorkSummary) -> Unit> { {} }

/** The Menu key on this card or row opens [work]'s menu. */
fun Modifier.menuKey(work: WorkSummary, open: (WorkSummary) -> Unit): Modifier = onKeyEvent { event ->
    if (event.key.nativeKeyCode != KeyEvent.KEYCODE_MENU) return@onKeyEvent false
    if (event.type == KeyEventType.KeyUp) open(work)
    true
}

/**
 * Gives the screens under it a work's menu: Play or Resume,
 * Play from beginning once there's progress, Mark as watched or unwatched, Add
 * to or Remove from favorites, Go to series for an episode (except on its
 * series' page), and Go to library outside a library's grid.
 */
@Composable
fun WorkMenuHost(session: ServerSession, navigator: Navigator, content: @Composable () -> Unit) {
    var open by remember { mutableStateOf<WorkSummary?>(null) }
    CompositionLocalProvider(LocalWorkMenu provides { open = it }, content = content)
    open?.let { work -> WorkMenu(work, session, navigator) { open = null } }
}

@Composable
private fun WorkMenu(work: WorkSummary, session: ServerSession, navigator: Navigator, onClose: () -> Unit) {
    val screen = navigator.current.screen
    val playable = work.available && work.kind in setOf("movie", "episode", "video", "extra")
    val started = work.userState?.positionMs != null
    val played = work.userState?.played == true
    val favorite = work.userState?.favorite == true
    val seriesId = work.episode?.seriesId
    val libraryId = work.libraryId
    val choices = buildList {
        if (playable) {
            add(Choice("play", stringResource(if (started) R.string.resume else R.string.play)))
            if (started) add(Choice("fromStart", stringResource(R.string.play_from_beginning)))
        }
        add(Choice("played", stringResource(if (played) R.string.mark_unplayed else R.string.mark_played)))
        add(Choice("favorite", stringResource(if (favorite) R.string.remove_favorite else R.string.add_favorite)))
        if (seriesId != null && (screen as? Screen.Work)?.id != seriesId) add(Choice("series", stringResource(R.string.go_to_series)))
        if (screen !is Screen.Library && libraryId != null) add(Choice("library", stringResource(R.string.go_to_library)))
    }
    val scope = session.graph.scope
    fun act(call: suspend () -> Unit) {
        scope.launch {
            try {
                call()
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                // The card shows as it was; the server has it as it is.
            }
        }
    }
    ChoicePanel(
        title = listOfNotNull(cardTitle(work), work.episode?.let(display::episodeCode)).joinToString(" · "),
        choices = choices,
        selected = null,
        onSelect = { id ->
            onClose()
            when (id) {
                "play" -> navigator.push(Screen.Player(work.id))
                "fromStart" -> navigator.push(Screen.Player(work.id, fromStart = true))
                "played" -> act { session.session.library.setPlayed(work.id, !played) }
                "favorite" -> act { session.session.library.setFavorite(work.id, !favorite) }
                "series" -> work.episode?.let { navigator.push(Screen.Work(it.seriesId, it)) }
                "library" -> libraryId?.let { navigator.open(Screen.Library(it)) }
            }
        },
        onDismiss = onClose,
    )
}
