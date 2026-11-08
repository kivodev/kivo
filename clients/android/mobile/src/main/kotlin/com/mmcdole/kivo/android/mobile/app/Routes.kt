package com.mmcdole.kivo.android.mobile.app

import androidx.annotation.StringRes
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.Favorite
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.VideoLibrary
import androidx.compose.material.icons.outlined.Download
import androidx.compose.material.icons.outlined.FavoriteBorder
import androidx.compose.material.icons.outlined.Home
import androidx.compose.material.icons.outlined.Search
import androidx.compose.material.icons.outlined.VideoLibrary
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.navigation.NavBackStackEntry
import androidx.navigation.NavDestination.Companion.hasRoute
import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.NavHostController
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.ui.WorkAction
import com.mmcdole.kivo.android.mobile.ui.WorkMarks
import com.mmcdole.kivo.api.models.EpisodeInfo
import com.mmcdole.kivo.api.models.WorkSummary
import kotlinx.serialization.Serializable

// The phone's destinations, type-safe: each tab is a graph with its first
// screen, and the pages any tab opens sit beside them, on the back stack of
// the tab that opened them.

@Serializable data object HomeTab

@Serializable data object LibrariesTab

@Serializable data object FavoritesTab

@Serializable data object DownloadsTab

@Serializable data object SearchTab

@Serializable data object HomeRoute

@Serializable data object LibrariesRoute

@Serializable data object FavoritesRoute

@Serializable data object DownloadsRoute

@Serializable data object SearchRoute

/** A library's grid; with [sort], a one-off view in that sort that leaves the saved one alone. */
@Serializable data class LibraryRoute(val id: String, val sort: String? = null)

/** A work's page; a series' opens on the season [seasonId] or [seasonNumber] names, as Go to series asks. */
@Serializable data class WorkRoute(val id: String, val seasonId: String? = null, val seasonNumber: Int? = null)

@Serializable data class CollectionRoute(val id: String)

@Serializable data class PersonRoute(val id: String)

@Serializable data class BookSeriesRoute(val id: String)

/** One kind's favorites as a grid. */
@Serializable data class FavoritesGridRoute(val kind: String)

/**
 * The video player, full screen: [workId] from where it was left, or from
 * the start; with the version and tracks a work's page chose, or the
 * server's where they're null.
 */
@Serializable data class PlayerRoute(
    val workId: String,
    val fromStart: Boolean = false,
    val versionId: String? = null,
    val audioTrackId: String? = null,
    val subtitleTrackId: String? = null,
)

/** The book playing, full screen over the tabs. */
@Serializable data object NowPlayingRoute

@Serializable data object SettingsRoute

@Serializable data object CreditsRoute

@Serializable data object LicensesRoute

@Serializable data class LicenseRoute(val id: String)

/** The navigation suite's destinations, each with its own back stack. */
enum class Tab(val graph: Any, val root: Any, @param:StringRes val label: Int, val icon: ImageVector, val selectedIcon: ImageVector) {
    Home(HomeTab, HomeRoute, R.string.mobile_tab_home, Icons.Outlined.Home, Icons.Filled.Home),
    Libraries(LibrariesTab, LibrariesRoute, R.string.mobile_tab_libraries, Icons.Outlined.VideoLibrary, Icons.Filled.VideoLibrary),
    Favorites(FavoritesTab, FavoritesRoute, R.string.mobile_tab_favorites, Icons.Outlined.FavoriteBorder, Icons.Filled.Favorite),
    Downloads(DownloadsTab, DownloadsRoute, R.string.mobile_tab_downloads, Icons.Outlined.Download, Icons.Filled.Download),
    Search(SearchTab, SearchRoute, R.string.mobile_tab_search, Icons.Outlined.Search, Icons.Filled.Search),
}

/**
 * Where the screens go, over the app's navigation: pages pushed on the
 * current tab's back stack, the tabs, and back.
 */
class MobileNav(private val nav: NavHostController, private val playBook: (workId: String, fromStart: Boolean) -> Unit = { _, _ -> }) {
    /** The tab showing: the one chosen last, or the one whose first screen back has come to. */
    var tab by mutableStateOf(Tab.Home)
        private set

    /** Follows the back stack to [entry]: a tab's first screen is that tab's, as back from another tab's first screen comes Home. */
    fun follow(entry: NavBackStackEntry?) {
        val destination = entry?.destination ?: return
        Tab.entries.firstOrNull { destination.hasRoute(it.root::class) }?.let { tab = it }
    }

    fun open(route: Any) = nav.navigate(route)

    fun openWork(work: WorkSummary) = open(WorkRoute(work.id))

    /** An episode's series, on the episode's season. */
    fun openSeries(episode: EpisodeInfo) = open(WorkRoute(episode.seriesId, episode.seasonId, episode.seasonNumber))

    /** Plays a video in the player, with the version and tracks a work's page chose. */
    fun play(workId: String, fromStart: Boolean = false, versionId: String? = null, audioTrackId: String? = null, subtitleTrackId: String? = null) =
        open(PlayerRoute(workId, fromStart, versionId, audioTrackId, subtitleTrackId))

    /** Plays [work]: a book in the book player, which the mini player shows; anything else in the player. */
    fun play(work: WorkSummary, fromStart: Boolean = false) {
        if (work.kind == "book") playBook(work.id, fromStart) else play(work.id, fromStart)
    }

    /** [route] in Now Playing's place, as Go to book opens the book's page. */
    fun replaceNowPlaying(route: Any) = nav.navigate(route) {
        popUpTo<NowPlayingRoute> { inclusive = true }
    }

    /** [route] in the player's place: the next episode's player, or the series' page after its last. */
    fun replacePlayer(route: Any) = nav.navigate(route) {
        popUpTo<PlayerRoute> { inclusive = true }
    }

    fun back() {
        nav.popBackStack()
    }

    /**
     * Shows [tab] with its back stack as it was left, keeping the one left;
     * the tab showing goes back to its first screen.
     */
    fun select(tab: Tab) {
        if (tab == this.tab) {
            nav.popBackStack(tab.root, inclusive = false)
            return
        }
        this.tab = tab
        nav.navigate(tab.graph) {
            popUpTo(nav.graph.findStartDestination().id) { saveState = true }
            launchSingleTop = true
            restoreState = true
        }
    }

    /** A library's grid, from the Libraries tab. */
    fun openLibrary(libraryId: String) {
        if (tab != Tab.Libraries) select(Tab.Libraries)
        open(LibraryRoute(libraryId))
    }
}

/** Does a card's [action] on [work]: playing and Go to series here, the marks by [marks]. */
fun MobileNav.handle(action: WorkAction, work: WorkSummary, marks: WorkMarks) {
    when (action) {
        WorkAction.Play -> play(work)
        WorkAction.PlayFromStart -> play(work, fromStart = true)
        WorkAction.GoToSeries -> work.episode?.let(::openSeries)
        else -> marks.apply(action, work)
    }
}
