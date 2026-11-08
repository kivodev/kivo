@file:OptIn(ExperimentalFoundationApi::class)

package com.mmcdole.kivo.android.home

import android.view.KeyEvent
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListPrefetchStrategy
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.layout.LazyLayoutCacheWindow
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.nativeKeyCode
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.compose.viewModel
import com.mmcdole.kivo.android.tv.R
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.main.tvLibraries
import com.mmcdole.kivo.android.ui.ArtworkDwellMs
import com.mmcdole.kivo.android.ui.CardRow
import com.mmcdole.kivo.android.ui.CardShape
import com.mmcdole.kivo.android.ui.EmptyState
import com.mmcdole.kivo.android.ui.Failed
import com.mmcdole.kivo.android.ui.FocusRail
import com.mmcdole.kivo.android.ui.LocalArtworkShown
import com.mmcdole.kivo.android.ui.LocalCaptionsShown
import com.mmcdole.kivo.android.ui.LocalCardLight
import com.mmcdole.kivo.android.ui.LocalContinueWatchingMenu
import com.mmcdole.kivo.android.ui.LocalWorkMenu
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.android.ui.Loader
import com.mmcdole.kivo.android.ui.with
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.RowTitle
import com.mmcdole.kivo.android.ui.RowsHoldPlace
import com.mmcdole.kivo.android.ui.SafeEdge
import com.mmcdole.kivo.android.ui.SeeAllCard
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.android.ui.SkeletonGate
import com.mmcdole.kivo.android.ui.SkeletonRow
import com.mmcdole.kivo.android.ui.WorkCard
import com.mmcdole.kivo.android.ui.awaitSettled
import com.mmcdole.kivo.android.ui.cardWidth
import com.mmcdole.kivo.android.ui.focusKey
import com.mmcdole.kivo.android.ui.prepareCard
import com.mmcdole.kivo.android.ui.raisedOnFocus
import com.mmcdole.kivo.android.ui.rememberFocusMemory
import com.mmcdole.kivo.android.ui.rememberCardLight
import com.mmcdole.kivo.android.ui.rememberOneMovePerFrame
import com.mmcdole.kivo.api.models.HomeRow
import com.mmcdole.kivo.api.models.Library
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.onEach
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull

/** Home's rows the TV shows, with the libraries it shows. */
data class HomeUi(val rows: List<ShownRow>, val libraries: List<Library>)

/** A row as Home shows it: the library that names it, if it's a library's, and its cards' shape. */
data class ShownRow(val row: HomeRow, val library: Library?, val shape: CardShape)

/** The row kinds the TV shows: no book rows, and no kind it doesn't know. */
internal val tvRows = setOf("continueWatching", "recentlyAdded", "favorites")

/**
 * The [rows] the TV shows, among the [libraries] it shows: empty rows and
 * rows of libraries it doesn't show left out. Each row's library and shape
 * are worked out here once, rather than each time the row is composed.
 */
fun shownRows(rows: List<HomeRow>, libraries: List<Library>): List<ShownRow> {
    val byId = libraries.associateBy { it.id }
    return rows.mapNotNull { row ->
        val id = row.library?.id
        val library = id?.let(byId::get)
        when {
            row.kind !in tvRows || row.items.isEmpty() || (id != null && library == null) -> null
            // Continue Watching is the frames where you stopped.
            row.kind == "continueWatching" || library?.kind == "videos" -> ShownRow(row, library, CardShape.Landscape)
            else -> ShownRow(row, library, CardShape.Poster)
        }
    }
}

class HomeViewModel(session: ServerSession) : ViewModel() {
    val home = Loader(viewModelScope) {
        combine(session.session.library.home(), session.session.library.libraries()) { home, libraries ->
            home.with(libraries) { home, libraries ->
                val shown = tvLibraries(libraries)
                HomeUi(shownRows(home.rows, shown), shown)
            }
        }.onEach { load ->
            // The cards' blurhashes are decoded here, off the main thread, so a row that comes into view draws
            // them at once rather than decoding them as it's composed.
            val rows = load.getOrNull()?.rows ?: return@onEach
            withContext(Dispatchers.Default) {
                rows.forEach { shown -> shown.row.items.forEach { prepareCard(it, shown.shape) } }
            }
        }
    }
}

/**
 * Home: the server's rows, empty ones left out, refreshed in
 * place by live events, keeping focus and scroll. Back away from the
 * first card moves focus to it; an empty Home hands focus to the rail.
 */
@Composable
fun HomeScreen(session: ServerSession, navigator: Navigator) {
    val model = viewModel { HomeViewModel(session) }
    val state by model.home.state.collectAsStateWithLifecycle()
    val user by session.session.user.collectAsStateWithLifecycle()

    when (val s = state) {
        is Load.Failed -> Failed(s.problem, session.session.baseUrl, model.home::retry, session.graph::changeServer)
        else -> SkeletonGate(s.getOrNull(), { HomeSkeleton() }) { ui ->
            val empty = when {
                ui.libraries.isEmpty() && user?.admin == true -> R.string.empty_no_libraries_admin to R.string.empty_no_libraries_admin_sentence
                ui.libraries.isEmpty() -> R.string.empty_no_libraries to R.string.empty_no_libraries_sentence
                ui.rows.isEmpty() -> R.string.empty_nothing_yet to R.string.empty_nothing_yet_sentence
                else -> null
            }
            if (empty == null) {
                Rows(ui, navigator)
            } else {
                EmptyState(stringResource(empty.first), stringResource(empty.second))
                FocusRail()
            }
        }
    }
}

/** Around a row's title, on a screen of rows, for the title's line and the cards below it to sit as on the Samsung TV app. */
val RowTitlePadding = PaddingValues(top = 6.dp, end = SafeEdge, bottom = 1.5.dp)

/** How far below a row's top its cards sit: its title with [RowTitlePadding], then the cards' room for focus. */
val RowCardsBelow = 45.5.dp

/** The gap between rows: one row to the next is about 270 dp, and the row above a focused one leaves the screen, captions and all. */
val RowGap = 21.dp

/** How far below the screen's top the rows start. */
private val RowsTop = 37.dp

/** Where a focused row's cards stay as the rows move under it, which is where the first row's start. */
private val FocusedCardsTop = RowsTop + RowCardsBelow

/**
 * Home's rows, clear of the rail, running to the screen's edge. Below them is room for the
 * last row to come up to where a focused row stays, rather than stopping short of it.
 */
private val RowsPadding = PaddingValues(start = 24.dp, top = RowsTop, bottom = 250.dp)

/** Home while it loads: a row of landscape cards and a row of posters, as Continue Watching and Favorites are laid out. */
@Composable
private fun HomeSkeleton() {
    Column(Modifier.fillMaxSize().padding(start = 24.dp, top = RowsTop), verticalArrangement = Arrangement.spacedBy(RowGap)) {
        SkeletonRow(CardShape.Landscape, titlePadding = RowTitlePadding)
        SkeletonRow(CardShape.Poster, titlePadding = RowTitlePadding)
    }
}

/**
 * How far past the screen Home keeps rows composed: the next row down, so
 * Down lands on a row already built, and half a row up, for Up after Down.
 * No more, as each row is some 8 cards and the TV may have 2 GB of memory.
 */
private val RowsCacheWindow = LazyLayoutCacheWindow(ahead = 320.dp, behind = 160.dp)

/**
 * About how many cards of a row the screen shows, which a row built ahead
 * of the screen composes: its first, seen first. Compose learns the count
 * from the rows it draws; this is its first guess.
 */
private fun cardsAcross(shape: CardShape) = if (shape == CardShape.Landscape) 4 else 8

/** The D-pad keys that move focus from card to card. */
private val MoveKeys = setOf(KeyEvent.KEYCODE_DPAD_UP, KeyEvent.KEYCODE_DPAD_DOWN, KeyEvent.KEYCODE_DPAD_LEFT, KeyEvent.KEYCODE_DPAD_RIGHT)

/** How often a held Up or Down moves a row: each move builds a row, which a slow TV can't do in a frame. */
private const val HeldRowMoveMs = 200L

@Composable
private fun Rows(ui: HomeUi, navigator: Navigator) {
    val memory = rememberFocusMemory()
    val list = rememberLazyListState(cacheWindow = RowsCacheWindow)
    val scope = rememberCoroutineScope()
    val firstKey = ui.rows.first().row.let { "${it.id}/${it.items.first().id}" }
    // A held key moves focus a card or row at most each frame, as in a library grid. Up and Down build a row each
    // move; Left and Right scroll the row a card, so while a frame is slow they too would run ahead of the screen.
    val dropMove = rememberOneMovePerFrame()
    var lastRowMove by remember { mutableLongStateOf(0L) }
    var atFirst by remember { mutableStateOf(true) }
    // The focused card's key, read only for the card light, so focus moving recomposes nothing.
    val focused = remember { mutableStateOf<String?>(null) }
    val cardLight = rememberCardLight { focused.value }

    BackHandler(enabled = !atFirst) {
        scope.launch {
            list.scrollToItem(0)
            memory.focused(firstKey)
            memory.restore(firstKey)
        }
    }

    RowsHoldPlace(FocusedCardsTop) {
        LazyColumn(
            state = list,
            modifier = Modifier
                .fillMaxSize()
                .onPreviewKeyEvent { event ->
                    if (event.type != KeyEventType.KeyDown || event.key.nativeKeyCode !in MoveKeys) return@onPreviewKeyEvent false
                    val native = event.nativeKeyEvent
                    val rowMove = native.keyCode == KeyEvent.KEYCODE_DPAD_UP || native.keyCode == KeyEvent.KEYCODE_DPAD_DOWN
                    if (rowMove && native.repeatCount > 0 && native.eventTime - lastRowMove < HeldRowMoveMs) return@onPreviewKeyEvent true
                    if (dropMove()) return@onPreviewKeyEvent true
                    if (rowMove) lastRowMove = native.eventTime
                    false
                },
            contentPadding = RowsPadding,
            verticalArrangement = Arrangement.spacedBy(RowGap),
        ) {
            items(ui.rows, key = { it.row.id }, contentType = { it.shape }) { (row, library, shape) ->
                val cards = rememberLazyListState(prefetchStrategy = remember { LazyListPrefetchStrategy(cardsAcross(shape)) })
                // A row's artwork loads once it's on screen and neither Home nor the row is scrolling, or it
                // has stayed on screen a while, so rows and cards that scroll past aren't fetched and
                // decoded. As the row scrolls again, the cards it brings in wait for it to settle again.
                val artwork = remember { mutableStateOf(false) }
                // A row built while Home scrolls has its captions once it has first settled, and keeps them.
                val captions = remember { mutableStateOf(!list.isScrollInProgress) }
                LaunchedEffect(Unit) {
                    snapshotFlow { list.layoutInfo.visibleItemsInfo.any { it.key == row.id } }.first { it }
                    while (true) {
                        withTimeoutOrNull(ArtworkDwellMs) { awaitSettled { list.isScrollInProgress || cards.isScrollInProgress } }
                        artwork.value = true
                        captions.value = true
                        snapshotFlow { cards.isScrollInProgress }.first { it }
                        artwork.value = false
                    }
                }
                val menu = if (row.kind == "continueWatching") LocalContinueWatchingMenu.current else LocalWorkMenu.current
                // Raised with its focused card, so the card's glow lies over the row below.
                Column(Modifier.raisedOnFocus()) {
                    RowTitle(rowTitle(row.kind, library?.name), Modifier.padding(RowTitlePadding))
                    CompositionLocalProvider(LocalWorkMenu provides menu, LocalArtworkShown provides artwork, LocalCaptionsShown provides captions, LocalCardLight provides cardLight) {
                        CardRow(state = cards) {
                            items(row.items, key = { it.id }, contentType = { shape }) { work ->
                                val key = "${row.id}/${work.id}"
                                WorkCard(
                                    work = work,
                                    shape = shape,
                                    width = cardWidth(shape),
                                    onClick = { navigator.push(Screen.Work(work.id)) },
                                    modifier = Modifier
                                        .focusKey(memory, key)
                                        .onFocusChanged {
                                            if (it.isFocused) {
                                                atFirst = key == firstKey
                                                focused.value = key
                                            }
                                        },
                                )
                            }
                            // Favorites ends in a card that opens the Favorites page.
                            if (row.kind == "favorites") {
                                item(key = "all", contentType = "all") {
                                    SeeAllCard(
                                        detail = null,
                                        shape = shape,
                                        width = cardWidth(shape),
                                        onClick = { navigator.push(Screen.Favorites) },
                                        modifier = Modifier
                                            .focusKey(memory, "${row.id}/all")
                                            .onFocusChanged { if (it.isFocused) atFirst = false },
                                    )
                                }
                            }
                        }
                    }
                }
            }
        }
    }
    LaunchedEffect(Unit) { memory.restore(firstKey) }
}

@Composable
internal fun rowTitle(kind: String, libraryName: String?): String = when (kind) {
    "continueWatching" -> stringResource(R.string.row_continue_watching)
    "favorites" -> stringResource(R.string.row_favorites)
    else -> libraryName?.let { stringResource(R.string.row_recently_added_in, it) } ?: stringResource(R.string.row_recently_added)
}
