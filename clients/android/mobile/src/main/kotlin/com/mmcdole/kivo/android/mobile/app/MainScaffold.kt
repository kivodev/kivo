package com.mmcdole.kivo.android.mobile.app

import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.consumeWindowInsets
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.TextAutoSize
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.layout.Column
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.NavigationRail
import androidx.compose.material3.NavigationRailItem
import androidx.compose.material3.NavigationRailItemDefaults
import androidx.compose.material3.LocalTextStyle
import androidx.compose.material3.Text
import androidx.compose.material3.adaptive.currentWindowAdaptiveInfo
import androidx.compose.material3.adaptive.navigationsuite.NavigationSuite
import androidx.compose.material3.adaptive.navigationsuite.NavigationSuiteDefaults
import androidx.compose.material3.adaptive.navigationsuite.NavigationSuiteScaffoldDefaults
import androidx.compose.material3.adaptive.navigationsuite.NavigationSuiteScaffoldLayout
import androidx.compose.material3.adaptive.navigationsuite.NavigationSuiteType
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavDestination.Companion.hasRoute
import androidx.navigation.NavGraphBuilder
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.navigation
import androidx.navigation.compose.rememberNavController
import androidx.navigation.toRoute
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.mobile.books.MiniPlayer
import com.mmcdole.kivo.android.mobile.books.MiniPlayerViewModel
import com.mmcdole.kivo.android.mobile.books.NowPlayingScreen
import com.mmcdole.kivo.android.mobile.detail.BookSeriesScreen
import com.mmcdole.kivo.android.mobile.detail.CollectionScreen
import com.mmcdole.kivo.android.mobile.detail.DetailScreen
import com.mmcdole.kivo.android.mobile.detail.PersonScreen
import com.mmcdole.kivo.android.mobile.cast.CastMiniController
import com.mmcdole.kivo.android.mobile.cast.CastRemoteScreen
import com.mmcdole.kivo.android.mobile.cast.CastViewModel
import com.mmcdole.kivo.android.mobile.cast.GoogleCastLink
import com.mmcdole.kivo.android.mobile.cast.LocalCast
import com.mmcdole.kivo.android.mobile.downloads.DownloadsScreen
import com.mmcdole.kivo.android.mobile.downloads.ShowDownloadsScreen
import com.mmcdole.kivo.android.mobile.linkdevice.LinkDeviceScreen
import com.mmcdole.kivo.android.mobile.lists.AddToListSheet
import com.mmcdole.kivo.android.mobile.lists.ListScreen
import com.mmcdole.kivo.android.mobile.lists.ListsScreen
import com.mmcdole.kivo.android.mobile.lists.SmartListBuilderScreen
import com.mmcdole.kivo.android.mobile.home.HomeScreen
import com.mmcdole.kivo.android.mobile.library.LibrariesScreen
import com.mmcdole.kivo.android.mobile.library.LibraryScreen
import com.mmcdole.kivo.android.mobile.library.libraryIcon
import com.mmcdole.kivo.android.mobile.player.PlayerScreen
import com.mmcdole.kivo.android.mobile.search.SearchScreen
import com.mmcdole.kivo.android.mobile.settings.AboutScreen
import com.mmcdole.kivo.android.mobile.settings.AccountScreen
import com.mmcdole.kivo.android.mobile.settings.AudioSettingsScreen
import com.mmcdole.kivo.android.mobile.settings.ChangePasswordScreen
import com.mmcdole.kivo.android.mobile.settings.CreditsScreen
import com.mmcdole.kivo.android.mobile.settings.DownloadSettingsScreen
import com.mmcdole.kivo.android.mobile.settings.HomeSettingsScreen
import com.mmcdole.kivo.android.mobile.settings.LibrariesSettingsScreen
import com.mmcdole.kivo.android.mobile.settings.PlaybackSettingsScreen
import com.mmcdole.kivo.android.mobile.settings.QualitySettingsScreen
import com.mmcdole.kivo.android.mobile.settings.SessionsScreen
import com.mmcdole.kivo.android.mobile.settings.SubtitleSettingsScreen
import com.mmcdole.kivo.android.mobile.settings.LicenseScreen
import com.mmcdole.kivo.android.mobile.settings.LicensesScreen
import com.mmcdole.kivo.android.mobile.settings.SettingsScreen
import com.mmcdole.kivo.android.mobile.ui.Incompatible
import com.mmcdole.kivo.android.mobile.ui.LocalWide
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.core.Compatibility
import com.mmcdole.kivo.core.data.GridSort
import com.mmcdole.kivo.core.session.KivoSession
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.filterNotNull
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
 * destinations' screens, each tab with its own back stack, and the mini
 * player above the bar while a book is loaded, or the cast's while
 * something plays on a TV. The player, Now Playing and the cast's remote
 * take the whole screen. The window that has the rail has the wide
 * layouts too ([LocalWide]). A `kivo://link` link opens Link a device.
 *
 * While the app and the server don't work together, which of them needs an
 * update shows in place of the screens. A phone with downloads keeps its
 * bar and its Downloads tab, whose downloads play without the server.
 */
@Composable
fun MainScaffold(session: ServerSession) {
    val controller = rememberNavController()
    val context = LocalContext.current
    val mini = viewModel { MiniPlayerViewModel(session.books.model) }
    val cast = viewModel { CastViewModel(session.session, GoogleCastLink.available(context.applicationContext)) }
    val nav = remember(controller) { MobileNav(controller, playBook = mini::play, readyDownload = { session.readyDownload(it)?.id }, cast = cast::cast) }
    val entry by controller.currentBackStackEntryAsState()
    LaunchedEffect(entry) { nav.follow(entry) }
    LaunchedEffect(Unit) {
        session.graph.linking.filterNotNull().collect { link ->
            session.graph.linking.value = null
            nav.open(LinkDeviceRoute(link))
        }
    }
    // Opened without the server, for what the device keeps of it: its downloads.
    var landed by rememberSaveable { mutableStateOf(false) }
    LaunchedEffect(Unit) {
        if (!landed && session.session.unreachable.value) nav.select(Tab.Downloads)
        landed = true
    }
    val casting by cast.state.collectAsStateWithLifecycle()
    val current = nav.tab
    val model = viewModel { MainViewModel(session.session) }
    val pinned by model.pinned.collectAsStateWithLifecycle()
    val book by mini.state.collectAsStateWithLifecycle()
    val destination = entry?.destination
    val fullScreen = destination != null &&
        (destination.hasRoute(PlayerRoute::class) || destination.hasRoute(NowPlayingRoute::class) || destination.hasRoute(CastRemoteRoute::class) ||
            destination.hasRoute(SmartListBuilderRoute::class))
    val suite = NavigationSuiteScaffoldDefaults.calculateFromAdaptiveInfo(currentWindowAdaptiveInfo())
    val compatibility by session.session.compatibility.collectAsStateWithLifecycle()
    val downloads by session.downloads.collectAsStateWithLifecycle()
    val incompatible = compatibility != Compatibility.Compatible
    val blocked = incompatible && (downloads.isEmpty() || current != Tab.Downloads)
    val type = if (fullScreen || (incompatible && downloads.isEmpty())) NavigationSuiteType.None else suite

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
        val suiteInsets = when (type) {
            NavigationSuiteType.NavigationRail -> WindowInsets.safeDrawing.only(WindowInsetsSides.Start)
            NavigationSuiteType.None -> WindowInsets(0)
            else -> WindowInsets.safeDrawing.only(WindowInsetsSides.Bottom)
        }
        CompositionLocalProvider(LocalCast provides cast.takeIf { it.offered }, LocalWide provides (suite == NavigationSuiteType.NavigationRail)) {
            if (blocked) {
                Incompatible(compatibility, session.session.savedServer.collectAsStateWithLifecycle().value.name, session::checkCompatibility, session.graph::changeServer)
            } else Column(Modifier.fillMaxSize()) {
                NavHost(controller, startDestination = HomeTab, modifier = Modifier.weight(1f).fillMaxWidth().consumeWindowInsets(suiteInsets)) {
                    tabs(session, nav)
                    pages(session, nav, cast)
                }
                when {
                    fullScreen -> Unit
                    casting.active -> CastMiniController(cast, onOpen = { nav.open(CastRemoteRoute) })
                    book.shown -> MiniPlayer(book, onOpen = { nav.open(NowPlayingRoute) }, onBack = mini::skipBack, onToggle = mini::togglePlay)
                }
            }
            nav.addingToList?.let { AddToListSheet(session, it, nav::closeAddToList) }
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
                label = { BarLabel(stringResource(tab.label)) },
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
                    label = { BarLabel(stringResource(tab.label)) },
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
                        label = { BarLabel(library.name) },
                        colors = colors,
                        modifier = Modifier.testTag("rail:${library.name}"),
                    )
                }
            }
        }
    }
}

/**
 * A bar or rail item's label, on one line, its text growing with the font
 * size only to 1.3 times, as the bar's room doesn't, and smaller where a
 * long word still doesn't fit; TalkBack reads it whole.
 */
@Composable
private fun BarLabel(text: String) {
    val density = LocalDensity.current
    CompositionLocalProvider(LocalDensity provides Density(density.density, density.fontScale.coerceAtMost(MaxBarFontScale))) {
        val style = LocalTextStyle.current
        Text(text, maxLines = 1, overflow = TextOverflow.Ellipsis, autoSize = TextAutoSize.StepBased(minFontSize = style.fontSize * 0.75f, maxFontSize = style.fontSize))
    }
}

private const val MaxBarFontScale = 1.3f

/** Each tab's graph and first screen. */
private fun NavGraphBuilder.tabs(session: ServerSession, nav: MobileNav) {
    navigation<HomeTab>(startDestination = HomeRoute) {
        composable<HomeRoute> { HomeScreen(session, nav) }
    }
    navigation<LibrariesTab>(startDestination = LibrariesRoute) {
        composable<LibrariesRoute> { LibrariesScreen(session, nav) }
    }
    navigation<ListsTab>(startDestination = ListsRoute) {
        composable<ListsRoute> { ListsScreen(session, nav) }
    }
    navigation<DownloadsTab>(startDestination = DownloadsRoute) {
        composable<DownloadsRoute> { DownloadsScreen(session, nav) }
    }
    navigation<SearchTab>(startDestination = SearchRoute) {
        composable<SearchRoute> { SearchScreen(session, nav) }
    }
}

/** The pages any tab opens, on that tab's back stack. */
private fun NavGraphBuilder.pages(session: ServerSession, nav: MobileNav, cast: CastViewModel) {
    composable<LibraryRoute> { val r = it.toRoute<LibraryRoute>(); LibraryScreen(session, r.id, r.sort?.let(GridSort::of), nav) }
    composable<WorkRoute> { val r = it.toRoute<WorkRoute>(); DetailScreen(session, r.id, r.seasonId, r.seasonNumber, nav) }
    composable<CollectionRoute> { CollectionScreen(session, it.toRoute<CollectionRoute>().id, nav) }
    composable<PersonRoute> { PersonScreen(session, it.toRoute<PersonRoute>().id, nav) }
    composable<BookSeriesRoute> { BookSeriesScreen(session, it.toRoute<BookSeriesRoute>().id, nav) }
    composable<ListRoute> { val r = it.toRoute<ListRoute>(); ListScreen(session, r.id, r.name, nav) }
    composable<SmartListBuilderRoute> { SmartListBuilderScreen(session, it.toRoute<SmartListBuilderRoute>(), nav) }
    composable<PlayerRoute> { PlayerScreen(session, it.toRoute<PlayerRoute>(), nav) }
    composable<NowPlayingRoute> { NowPlayingScreen(session, nav) }
    composable<SettingsRoute> { SettingsScreen(session, nav) }
    composable<CreditsRoute> { CreditsScreen(nav) }
    composable<LicensesRoute> { LicensesScreen(nav) }
    composable<LicenseRoute> { LicenseScreen(it.toRoute<LicenseRoute>().id, nav) }
    composable<ShowDownloadsRoute> { val r = it.toRoute<ShowDownloadsRoute>(); ShowDownloadsScreen(session, r.seriesId, r.title, nav) }
    composable<LinkDeviceRoute> { LinkDeviceScreen(session, it.toRoute<LinkDeviceRoute>().link, nav) }
    composable<CastRemoteRoute> {
        val state by cast.state.collectAsStateWithLifecycle()
        // Once nothing plays on the TV, or it's let go, the remote closes.
        LaunchedEffect(state.active) { if (!state.active) nav.back() }
        CastRemoteScreen(cast, onClose = nav::back, onContinueHere = nav::replaceCastRemote)
    }
    composable<SettingsPageRoute> {
        when (it.toRoute<SettingsPageRoute>().page) {
            SettingsPage.Account -> AccountScreen(session, nav)
            SettingsPage.Devices -> SessionsScreen(session, nav)
            SettingsPage.ChangePassword -> ChangePasswordScreen(session, nav)
            SettingsPage.Playback -> PlaybackSettingsScreen(session, nav)
            SettingsPage.Audio -> AudioSettingsScreen(session, nav)
            SettingsPage.Subtitles -> SubtitleSettingsScreen(session, nav)
            SettingsPage.Quality -> QualitySettingsScreen(session, nav)
            SettingsPage.Downloads -> DownloadSettingsScreen(session, nav)
            SettingsPage.Home -> HomeSettingsScreen(session, nav)
            SettingsPage.Libraries -> LibrariesSettingsScreen(session, nav)
            SettingsPage.About -> AboutScreen(session, nav)
        }
    }
}
