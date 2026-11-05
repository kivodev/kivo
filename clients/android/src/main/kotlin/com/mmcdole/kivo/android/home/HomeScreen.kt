package com.mmcdole.kivo.android.home

import androidx.activity.compose.BackHandler
import androidx.compose.animation.Crossfade
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.State
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.R
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.main.tvLibraries
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.ui.Artwork
import com.mmcdole.kivo.android.ui.CardRow
import com.mmcdole.kivo.android.ui.CardShape
import com.mmcdole.kivo.android.ui.EmptyState
import com.mmcdole.kivo.android.ui.Failed
import com.mmcdole.kivo.android.ui.FocusRail
import com.mmcdole.kivo.android.ui.LocalContinueWatchingMenu
import com.mmcdole.kivo.android.ui.LocalWorkMenu
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.android.ui.Loader
import com.mmcdole.kivo.android.ui.with
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.RowCardsTop
import com.mmcdole.kivo.android.ui.RowTitle
import com.mmcdole.kivo.android.ui.RowsHoldPlace
import com.mmcdole.kivo.android.ui.SafeEdge
import com.mmcdole.kivo.android.ui.SafeTop
import com.mmcdole.kivo.android.ui.SeeAllCard
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.android.ui.SkeletonGate
import com.mmcdole.kivo.android.ui.SkeletonLine
import com.mmcdole.kivo.android.ui.SkeletonRow
import com.mmcdole.kivo.android.ui.WorkCard
import com.mmcdole.kivo.android.ui.cardSubtitle
import com.mmcdole.kivo.android.ui.cardTitle
import com.mmcdole.kivo.android.ui.cardWidth
import com.mmcdole.kivo.android.ui.display
import com.mmcdole.kivo.android.ui.fadeSpec
import com.mmcdole.kivo.android.ui.focusKey
import com.mmcdole.kivo.android.ui.formatRating
import com.mmcdole.kivo.android.ui.overlay
import com.mmcdole.kivo.android.ui.rememberFocusMemory
import com.mmcdole.kivo.api.models.HomeRow
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.display.episodeCode
import com.mmcdole.kivo.core.display.runtime
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.launch

/** Home's rows the TV shows, with the libraries that name them. */
data class HomeUi(val rows: List<HomeRow>, val libraries: List<Library>)

/** The row kinds the TV shows: no book rows, and no kind it doesn't know. */
private val tvRows = setOf("continueWatching", "recentlyAdded", "favorites")

class HomeViewModel(session: ServerSession) : ViewModel() {
    val home = Loader(viewModelScope) {
        combine(session.session.library.home(), session.session.library.libraries()) { home, libraries ->
            home.with(libraries) { home, libraries ->
                val shown = tvLibraries(libraries)
                HomeUi(
                    rows = home.rows.filter { row ->
                        row.kind in tvRows && row.items.isNotEmpty() && (row.library?.id == null || shown.any { it.id == row.library?.id })
                    },
                    libraries = shown,
                )
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

/** How much of the top of Home the focused work's header takes. */
private val HeaderHeight = 150.dp

/** Home's rows under the header, clear of the rail and the screen's edges. */
private val RowsPadding = PaddingValues(start = 24.dp, end = SafeEdge, bottom = SafeEdge)

/** Home while it loads: the header's room, then a row of landscape cards and a row of posters, as Continue Watching and Favorites are laid out. */
@Composable
private fun HomeSkeleton() {
    Column(Modifier.fillMaxSize().padding(RowsPadding)) {
        Box(Modifier.height(HeaderHeight).padding(top = SafeTop)) { SkeletonLine(320.dp, TvType.title) }
        Column(verticalArrangement = Arrangement.spacedBy(28.dp)) {
            SkeletonRow(CardShape.Landscape)
            SkeletonRow(CardShape.Poster)
        }
    }
}

@Composable
private fun Rows(ui: HomeUi, navigator: Navigator) {
    val memory = rememberFocusMemory()
    val list = rememberLazyListState()
    val scope = rememberCoroutineScope()
    val firstKey = ui.rows.first().let { "${it.id}/${it.items.first().id}" }
    var atFirst by remember { mutableStateOf(true) }
    // The focused card, read only by the header and backdrop, so focus moving recomposes nothing else.
    val focused = remember { mutableStateOf<WorkSummary?>(null) }

    BackHandler(enabled = !atFirst) {
        scope.launch {
            list.scrollToItem(0)
            memory.focused(firstKey)
            memory.restore(firstKey)
        }
    }

    Box(Modifier.fillMaxSize()) {
        val shown = restingOn(focused)
        Shown(shown, Modifier.fillMaxSize()) { work -> Backdrop(work) }
        Column(Modifier.fillMaxSize()) {
            Shown(shown, Modifier.height(HeaderHeight)) { work -> Header(work) }
            RowsHoldPlace(RowCardsTop) {
                LazyColumn(
                    state = list,
                    modifier = Modifier.fillMaxSize(),
                    contentPadding = RowsPadding,
                    verticalArrangement = Arrangement.spacedBy(28.dp),
                ) {
                    items(ui.rows, key = { it.id }) { row ->
                        val library = ui.libraries.firstOrNull { it.id == row.library?.id }
                        // Continue Watching is the frames where you stopped.
                        val landscape = row.kind == "continueWatching" || library?.kind == "videos"
                        val shape = if (landscape) CardShape.Landscape else CardShape.Poster
                        val menu = if (row.kind == "continueWatching") LocalContinueWatchingMenu.current else LocalWorkMenu.current
                        Column {
                            RowTitle(rowTitle(row, library), Modifier.padding(bottom = 12.dp))
                            CompositionLocalProvider(LocalWorkMenu provides menu) {
                                CardRow {
                                    items(row.items, key = { it.id }) { work ->
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
                                                        focused.value = work
                                                    }
                                                },
                                        )
                                    }
                                    // Favorites ends in a card that opens the Favorites page.
                                    if (row.kind == "favorites") {
                                        item(key = "all") {
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
        }
    }
    LaunchedEffect(Unit) { memory.restore(firstKey) }
}

/** How long focus rests on a card before the header and backdrop follow it. */
private const val FOCUS_REST_MS = 300L

/** The focused card's work once focus rests on it: at once for the first card, then [FOCUS_REST_MS] after each move. */
@Composable
private fun restingOn(focused: State<WorkSummary?>): State<WorkSummary?> {
    val shown = remember { mutableStateOf<WorkSummary?>(null) }
    LaunchedEffect(focused) {
        snapshotFlow { focused.value }.collectLatest { work ->
            if (shown.value != null) delay(FOCUS_REST_MS)
            shown.value = work
        }
    }
    return shown
}

/** [content] for the work [shown], fading from one work to the next; only this recomposes as it changes. */
@Composable
private fun Shown(shown: State<WorkSummary?>, modifier: Modifier, content: @Composable (WorkSummary?) -> Unit) {
    Crossfade(shown.value, animationSpec = fadeSpec(), label = "shown", modifier = modifier) { content(it) }
}

/** The focused work's backdrop at the top right, dimmed and fading into the page. */
@Composable
private fun Backdrop(work: WorkSummary?) {
    val image = work?.let { it.images.backdrop ?: it.images.still } ?: return
    val colors = Kivo.colors
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.TopEnd) {
        Box(
            Modifier
                .fillMaxWidth(0.72f)
                .overlay(
                    SolidColor(colors.scrimSoft),
                    Brush.horizontalGradient(listOf(colors.bg, Color.Transparent)),
                    Brush.verticalGradient(listOf(Color.Transparent, colors.bg)),
                ),
        ) {
            Artwork(image, "", KivoTokens.Aspect.landscape, 640.dp, Modifier.fillMaxWidth(), corners = 0.dp)
        }
    }
}

/** The focused work: its name, and an episode's code and title, then its year, runtime and ratings. */
@Composable
private fun Header(work: WorkSummary?) {
    work ?: return
    val colors = Kivo.colors
    val meta = listOfNotNull(
        if (work.kind == "series") cardSubtitle(work) else work.year?.toString(),
        work.durationMs?.takeIf { work.kind != "series" }?.let { display.runtime(it.toLong()) },
        work.communityRating?.let { "★ ${formatRating(it)}" },
        work.contentRating,
    ).joinToString(" · ")
    Column(Modifier.padding(start = 24.dp, top = SafeTop, end = SafeEdge).widthIn(max = 560.dp), verticalArrangement = Arrangement.spacedBy(Kivo.space(1))) {
        Text(cardTitle(work), style = TvType.title, color = colors.text, maxLines = 1, overflow = TextOverflow.Ellipsis)
        if (work.kind == "episode") {
            Text(listOfNotNull(work.episode?.let(display::episodeCode), work.title).joinToString(" · "), style = TvType.bodyStrong, color = colors.text, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
        if (meta.isNotEmpty()) Text(meta, style = TvType.body, color = colors.textMuted, maxLines = 1)
    }
}

@Composable
private fun rowTitle(row: HomeRow, library: Library?): String = when (row.kind) {
    "continueWatching" -> stringResource(R.string.row_continue_watching)
    "favorites" -> stringResource(R.string.row_favorites)
    else -> library?.let { stringResource(R.string.row_recently_added_in, it.name) } ?: stringResource(R.string.row_recently_added)
}
