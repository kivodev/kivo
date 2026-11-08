package com.mmcdole.kivo.android.mobile.app

import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.consumeWindowInsets
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.layout.Column
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.NavigationRail
import androidx.compose.material3.NavigationRailItem
import androidx.compose.material3.NavigationRailItemDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.adaptive.currentWindowAdaptiveInfo
import androidx.compose.material3.adaptive.navigationsuite.NavigationSuite
import androidx.compose.material3.adaptive.navigationsuite.NavigationSuiteDefaults
import androidx.compose.material3.adaptive.navigationsuite.NavigationSuiteScaffoldDefaults
import androidx.compose.material3.adaptive.navigationsuite.NavigationSuiteScaffoldLayout
import androidx.compose.material3.adaptive.navigationsuite.NavigationSuiteType
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavGraphBuilder
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.navigation
import androidx.navigation.compose.rememberNavController
import androidx.navigation.toRoute
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.mobile.detail.BookSeriesScreen
import com.mmcdole.kivo.android.mobile.detail.CollectionScreen
import com.mmcdole.kivo.android.mobile.detail.DetailScreen
import com.mmcdole.kivo.android.mobile.detail.PersonScreen
import com.mmcdole.kivo.android.mobile.downloads.DownloadsScreen
import com.mmcdole.kivo.android.mobile.favorites.FavoritesGridScreen
import com.mmcdole.kivo.android.mobile.favorites.FavoritesScreen
import com.mmcdole.kivo.android.mobile.home.HomeScreen
import com.mmcdole.kivo.android.mobile.library.LibrariesScreen
import com.mmcdole.kivo.android.mobile.library.LibraryScreen
import com.mmcdole.kivo.android.mobile.library.libraryIcon
import com.mmcdole.kivo.android.mobile.search.SearchScreen
import com.mmcdole.kivo.android.mobile.settings.CreditsScreen
import com.mmcdole.kivo.android.mobile.settings.LicenseScreen
import com.mmcdole.kivo.android.mobile.settings.LicensesScreen
import com.mmcdole.kivo.android.mobile.settings.SettingsScreen
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.core.session.KivoSession
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn

/** The signed-in shell's state: the pinned libraries the rail lists. */
class MainViewModel(session: KivoSession) : ViewModel() {
    val pinned: StateFlow<List<Library>> = session.library.libraries()
        .map { load -> load.getOrNull()?.filter { it.pinned }.orEmpty() }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), emptyList())
}

/**
 * Signed in: the navigation suite, a bottom bar on a phone and a rail with
 * the pinned libraries under the destinations on a tablet, over the
 * destinations' screens, each tab with its own back stack.
 */
@Composable
fun MainScaffold(session: ServerSession) {
    val controller = rememberNavController()
    val nav = remember(controller) { MobileNav(controller) }
    val entry by controller.currentBackStackEntryAsState()
    LaunchedEffect(entry) { nav.follow(entry) }
    val current = nav.tab
    val model = viewModel { MainViewModel(session.session) }
    val pinned by model.pinned.collectAsStateWithLifecycle()
    val type = NavigationSuiteScaffoldDefaults.calculateFromAdaptiveInfo(currentWindowAdaptiveInfo())

    NavigationSuiteScaffoldLayout(
        layoutType = type,
        navigationSuite = {
            if (type == NavigationSuiteType.NavigationRail) {
                Rail(current, pinned, nav)
            } else {
                Bar(current, nav, type)
            }
        },
    ) {
        // The bar or rail keeps clear of the system bars on its own side, so the screens beside it don't again.
        val side = if (type == NavigationSuiteType.NavigationRail) WindowInsetsSides.Start else WindowInsetsSides.Bottom
        val suiteInsets = WindowInsets.safeDrawing.only(side)
        NavHost(controller, startDestination = HomeTab, modifier = Modifier.fillMaxSize().consumeWindowInsets(suiteInsets)) {
            tabs(session, nav)
            pages(session, nav)
        }
    }
}

@Composable
private fun Bar(current: Tab, nav: MobileNav, type: NavigationSuiteType) {
    val itemColors = NavigationSuiteDefaults.itemColors(
        navigationBarItemColors = NavigationBarItemDefaults.colors(indicatorColor = MaterialTheme.colorScheme.primary.copy(alpha = 0.2f)),
    )
    NavigationSuite(
        layoutType = type,
        colors = NavigationSuiteDefaults.colors(navigationBarContainerColor = MaterialTheme.colorScheme.surfaceContainerLow),
        modifier = Modifier.testTag("navBar"),
    ) {
        for (tab in Tab.entries) {
            item(
                selected = tab == current,
                onClick = { nav.select(tab) },
                icon = { Icon(if (tab == current) tab.selectedIcon else tab.icon, null) },
                label = { Text(stringResource(tab.label)) },
                modifier = Modifier.testTag("tab:${tab.name}"),
                colors = itemColors,
            )
        }
    }
}

/** The tablet's rail: the destinations, then a divider and the pinned libraries, each opening its grid. */
@Composable
private fun Rail(current: Tab, pinned: List<Library>, nav: MobileNav) {
    val colors = NavigationRailItemDefaults.colors(indicatorColor = MaterialTheme.colorScheme.primary.copy(alpha = 0.2f))
    NavigationRail(containerColor = MaterialTheme.colorScheme.surfaceContainerLow, modifier = Modifier.testTag("navRail")) {
        Column(Modifier.verticalScroll(rememberScrollState())) {
            Spacer(Modifier.height(8.dp))
            for (tab in Tab.entries) {
                NavigationRailItem(
                    selected = tab == current,
                    onClick = { nav.select(tab) },
                    icon = { Icon(if (tab == current) tab.selectedIcon else tab.icon, null) },
                    label = { Text(stringResource(tab.label)) },
                    colors = colors,
                    modifier = Modifier.testTag("tab:${tab.name}"),
                )
            }
            if (pinned.isNotEmpty()) {
                HorizontalDivider(Modifier.width(56.dp).padding(vertical = 12.dp))
                for (library in pinned) {
                    NavigationRailItem(
                        selected = false,
                        onClick = { nav.openLibrary(library.id) },
                        icon = { Icon(libraryIcon(library.kind), null) },
                        label = { Text(library.name, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                        colors = colors,
                        modifier = Modifier.testTag("rail:${library.name}"),
                    )
                }
            }
        }
    }
}

/** Each tab's graph and first screen. */
private fun NavGraphBuilder.tabs(session: ServerSession, nav: MobileNav) {
    navigation<HomeTab>(startDestination = HomeRoute) {
        composable<HomeRoute> { HomeScreen(session, nav, onFavorites = { nav.select(Tab.Favorites) }) }
    }
    navigation<LibrariesTab>(startDestination = LibrariesRoute) {
        composable<LibrariesRoute> { LibrariesScreen(session, nav) }
    }
    navigation<FavoritesTab>(startDestination = FavoritesRoute) {
        composable<FavoritesRoute> { FavoritesScreen(session, nav) }
    }
    navigation<DownloadsTab>(startDestination = DownloadsRoute) {
        composable<DownloadsRoute> { DownloadsScreen() }
    }
    navigation<SearchTab>(startDestination = SearchRoute) {
        composable<SearchRoute> { SearchScreen(session, nav) }
    }
}

/** The pages any tab opens, on that tab's back stack. */
private fun NavGraphBuilder.pages(session: ServerSession, nav: MobileNav) {
    composable<LibraryRoute> { val r = it.toRoute<LibraryRoute>(); LibraryScreen(session, r.id, r.sort, nav) }
    composable<WorkRoute> { val r = it.toRoute<WorkRoute>(); DetailScreen(session, r.id, r.seasonId, r.seasonNumber, nav) }
    composable<CollectionRoute> { CollectionScreen(session, it.toRoute<CollectionRoute>().id, nav) }
    composable<PersonRoute> { PersonScreen(session, it.toRoute<PersonRoute>().id, nav) }
    composable<BookSeriesRoute> { BookSeriesScreen(session, it.toRoute<BookSeriesRoute>().id, nav) }
    composable<FavoritesGridRoute> { FavoritesGridScreen(session, it.toRoute<FavoritesGridRoute>().kind, nav) }
    composable<PlayerRoute> { PlayerPlaceholder(nav) }
    composable<SettingsRoute> { SettingsScreen(session, nav) }
    composable<CreditsRoute> { CreditsScreen(nav) }
    composable<LicensesRoute> { LicensesScreen(nav) }
    composable<LicenseRoute> { LicenseScreen(it.toRoute<LicenseRoute>().id, nav) }
}
