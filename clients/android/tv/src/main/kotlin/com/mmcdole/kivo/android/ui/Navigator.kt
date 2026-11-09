package com.mmcdole.kivo.android.ui

import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.saveable.SaveableStateHolder
import androidx.lifecycle.ViewModelStore
import androidx.lifecycle.ViewModelStoreOwner
import androidx.lifecycle.viewmodel.compose.LocalViewModelStoreOwner
import com.mmcdole.kivo.api.models.EpisodeInfo

/** The TV interface's screens: the rail's destinations, an open list, detail pages and the player. */
sealed interface Screen {
    /** A screen the rail opens. */
    sealed interface Destination : Screen

    data object Home : Destination

    data object Search : Destination

    /** Settings: its sections, and the focused one's settings beside them. */
    data object Settings : Destination

    /** Credits, opened from About. */
    data object Credits : Screen

    /** Open source licenses, opened from Credits. */
    data object Licenses : Screen

    /** One license's text, opened from Open source licenses; [id] is its library's. */
    data class LicenseText(val id: String) : Screen

    data class Library(val id: String) : Destination

    /** The user's lists: My List's row, then the lists they made. */
    data object Lists : Destination

    /** A list's works in a grid, [name] as its title. */
    data class ListWorks(val id: String, val name: String) : Screen

    /**
     * A work's page: a movie, series, season (the series page on that
     * season), episode or video. A series' page opened by Go to
     * series opens on the season of the [episode] it came from.
     */
    data class Work(val id: String, val episode: EpisodeInfo? = null) : Screen

    /** A collection's page: a movie collection or a videos library's folder, with its root works. */
    data class Collection(val id: String) : Screen

    /** A person's page: the movies, shows and videos they're credited on. */
    data class Person(val id: String) : Screen

    /** The player, with the detail page's choices; [fromStart] plays from the beginning rather than resuming. */
    data class Player(
        val workId: String,
        val versionId: String? = null,
        val audioTrackId: String? = null,
        val subtitleTrackId: String? = null,
        val fromStart: Boolean = false,
    ) : Screen
}

/** A screen on the back stack, with its own view models, cleared when it's popped. */
class Entry(val id: Int, val screen: Screen) : ViewModelStoreOwner {
    override val viewModelStore = ViewModelStore()
}

/** What Back does where the navigator stands. */
enum class Back {
    /** Goes back to the screen below. */
    Pop,

    /** Opens the rail on the current screen: a destination sitting on Home. */
    Rail,

    /** Asks to leave the app: Home. */
    Exit,
}

/**
 * The back stack. The rail's destinations sit on Home; detail pages,
 * Credits and the player stack above. Back from a destination opens
 * the rail and Back at Home asks to leave ([back]). Each entry keeps its
 * saved state (scroll and focus) and view models until it's popped, so Back
 * returns to the exact card it left from.
 */
class Navigator {
    private var nextId = 0
    val stack = mutableStateListOf(Entry(nextId++, Screen.Home))
    private var onRemoved: (Int) -> Unit = {}

    val current: Entry get() = stack.last()

    /** The destination the current screen sits on, which the rail marks: Home, or the one opened on it. */
    val destination: Screen.Destination
        get() = stack.getOrNull(1)?.screen as? Screen.Destination ?: Screen.Home

    val back: Back
        get() = when {
            stack.size == 1 -> Back.Exit
            stack.size == 2 && current.screen is Screen.Destination -> Back.Rail
            else -> Back.Pop
        }

    fun push(screen: Screen) {
        stack += Entry(nextId++, screen)
    }

    /** Replaces the top screen, as the player does going to the next episode. */
    fun replace(screen: Screen) {
        remove(stack.removeAt(stack.lastIndex))
        push(screen)
    }

    /** Replaces the top screen with [screen], or goes back to it when it's the one below, a work's page whatever season it opened on. */
    fun returnTo(screen: Screen) {
        val below = stack.getOrNull(stack.lastIndex - 1)?.screen
        val same = below == screen || (below is Screen.Work && screen is Screen.Work && below.id == screen.id)
        if (stack.size > 1 && same) pop() else replace(screen)
    }

    /** Opens a rail destination: Home alone, or the destination on Home. */
    fun open(screen: Screen) {
        while (stack.size > 1) remove(stack.removeAt(stack.lastIndex))
        if (screen != Screen.Home) push(screen)
    }

    /** Goes back a screen; false at Home. */
    fun pop(): Boolean {
        if (stack.size <= 1) return false
        remove(stack.removeAt(stack.lastIndex))
        return true
    }

    private fun remove(entry: Entry) {
        entry.viewModelStore.clear()
        onRemoved(entry.id)
    }

    /**
     * Shows the current screen with its saved state, kept in [holder], and its
     * view models. The holder outlives the places a screen can be shown (in
     * the rail's frame, or full screen for the player), so leaving one keeps
     * the screens below it as they were.
     */
    @Composable
    fun Host(holder: SaveableStateHolder, content: @Composable (Screen) -> Unit) {
        DisposableEffect(holder) {
            onRemoved = { holder.removeState(it) }
            onDispose { onRemoved = {} }
        }
        val entry = current
        holder.SaveableStateProvider(entry.id) {
            CompositionLocalProvider(LocalViewModelStoreOwner provides entry) { content(entry.screen) }
        }
    }

    /** Clears every screen's view models, as the app leaves its server. */
    fun clear() {
        stack.forEach { it.viewModelStore.clear() }
    }
}
