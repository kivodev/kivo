package com.mmcdole.kivo.android.library

import android.view.KeyEvent
import androidx.compose.animation.Crossfade
import androidx.compose.animation.ExperimentalAnimationApi
import androidx.compose.animation.core.updateTransition
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusProperties
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.nativeKeyCode
import androidx.compose.ui.input.key.onKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.drawText
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.tv.material3.Border
import androidx.tv.material3.ClickableSurfaceDefaults
import androidx.tv.material3.Surface
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.tv.R
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.main.RailWidth
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.ui.CardGap
import com.mmcdole.kivo.android.ui.CardShape
import com.mmcdole.kivo.android.ui.ChipChoices
import com.mmcdole.kivo.android.ui.Choice
import com.mmcdole.kivo.android.ui.ChoiceList
import com.mmcdole.kivo.android.ui.CollectionCard
import com.mmcdole.kivo.android.ui.EmptyState
import com.mmcdole.kivo.android.ui.Failed
import com.mmcdole.kivo.android.ui.InlineFailed
import com.mmcdole.kivo.android.ui.KivoButton
import com.mmcdole.kivo.android.ui.KivoIcon
import com.mmcdole.kivo.android.ui.KivoIconView
import com.mmcdole.kivo.android.ui.Lit
import com.mmcdole.kivo.android.ui.MenuPopup
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.SafeEdge
import com.mmcdole.kivo.android.ui.SafeTop
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.android.ui.SkeletonCard
import com.mmcdole.kivo.android.ui.SkeletonGate
import com.mmcdole.kivo.android.ui.SkeletonLine
import com.mmcdole.kivo.android.ui.fadeSpec
import com.mmcdole.kivo.android.ui.focusWhenPlaced
import com.mmcdole.kivo.android.ui.rememberFocusMemory
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.LibraryKind
import com.mmcdole.kivo.api.models.Resolution
import com.mmcdole.kivo.core.data.DynamicRange
import com.mmcdole.kivo.core.data.GridItem
import com.mmcdole.kivo.core.data.GridPager
import com.mmcdole.kivo.core.data.GridPages
import com.mmcdole.kivo.core.data.GridPages.Companion.RAIL_LETTERS
import com.mmcdole.kivo.core.data.FilterGroupKind
import com.mmcdole.kivo.core.data.GridFilter
import com.mmcdole.kivo.core.data.GridSort
import com.mmcdole.kivo.core.data.LibraryTab
import com.mmcdole.kivo.core.data.WorkPlayState
import com.mmcdole.kivo.core.session.GridCardShape
import com.mmcdole.kivo.core.session.GridEmptyCase
import com.mmcdole.kivo.core.session.LibraryGrid
import com.mmcdole.kivo.core.session.LibraryGridState
import com.mmcdole.kivo.core.session.OfferRead
import java.text.NumberFormat
import kotlinx.coroutines.launch

/**
 * A library's screen: the core's [grid], which holds the section, sort and
 * filters this device last used in it and the grid they show, for as long
 * as the screen lasts.
 */
class LibraryViewModel(session: ServerSession, libraryId: String) : ViewModel() {
    val grid: LibraryGrid = session.session.library.libraryGrid(libraryId)

    /**
     * The grid on screen replaced another and focus stays where it was,
     * such as on Sort; a grid that replaced the empty state's Clear filters
     * takes focus itself.
     */
    var replaced = false

    /** Whether a grid has shown, after which another shows in its place without the screen's skeleton. */
    var shown = false

    /** Makes [change] to what's chosen; a grid it shows in place of the one on screen leaves focus where it is when [keepFocus]. */
    fun choose(keepFocus: Boolean = true, change: LibraryGrid.() -> Unit) {
        val before = grid.state.value.pager
        grid.change()
        if (grid.state.value.pager !== before) replaced = keepFocus
    }

    /** Reads the grid again, after its first page failed. */
    fun retry() {
        viewModelScope.launch { grid.state.value.pager.refresh() }
    }

    override fun onCleared() {
        grid.close()
    }
}

/** What the grid on screen shows, for as long as it fades: its section, its cards' shape, and what it says when empty. */
private data class Shown(val tab: LibraryTab, val shape: CardShape, val empty: GridEmptyCase, val played: WorkPlayState?)

private val LibraryGridState.shown: Shown
    get() = Shown(tab, if (cardShape == GridCardShape.Landscape) CardShape.Landscape else CardShape.Poster, emptyCase, filters.played)

@Composable
private fun sortName(sort: GridSort) = stringResource(
    when (sort) {
        GridSort.AddedAt -> R.string.sort_added
        GridSort.LastEpisodeAddedAt -> R.string.sort_last_episode_added
        GridSort.ReleaseDate -> R.string.sort_release
        GridSort.Rating -> R.string.sort_rating
        GridSort.Runtime -> R.string.sort_runtime
        GridSort.LastPlayedAt -> R.string.sort_last_played
        GridSort.Author -> R.string.sort_author
        else -> R.string.sort_title
    },
)

/** The sort in a word or two, for the count line: "A–Z", "Recently added". */
@Composable
private fun sortShort(sort: GridSort) = stringResource(
    when (sort) {
        GridSort.AddedAt -> R.string.sort_short_added
        GridSort.LastEpisodeAddedAt -> R.string.sort_short_last_episode_added
        GridSort.ReleaseDate -> R.string.sort_short_release
        GridSort.Rating -> R.string.sort_short_rating
        GridSort.Runtime -> R.string.sort_short_runtime
        GridSort.LastPlayedAt -> R.string.sort_short_last_played
        GridSort.Author -> R.string.sort_author
        else -> R.string.sort_short_title
    },
)

/** "237 movies", "5 collections": how many of what the section lists. */
@Composable
private fun countNoun(count: Int, tab: LibraryTab, library: Library?): String {
    val plural = when {
        tab == LibraryTab.Collections -> R.plurals.count_collections
        library?.kind == LibraryKind.Movies -> R.plurals.count_movies
        library?.kind == LibraryKind.Shows -> R.plurals.count_shows
        else -> R.plurals.count_videos
    }
    return pluralStringResource(plural, count, NumberFormat.getIntegerInstance().format(count))
}

/** "No unwatched movies" for the watch status alone, else "No movies match these filters". */
@Composable
private fun emptyFiltered(shown: Shown, library: Library?): String {
    val noun = stringResource(
        when (library?.kind) {
            LibraryKind.Movies -> R.string.noun_movies
            LibraryKind.Shows -> R.string.noun_shows
            else -> R.string.noun_videos
        },
    )
    val played = shown.played
    return if (played != null && shown.empty == GridEmptyCase.WatchStatus) {
        val state = stringResource(
            when (played) {
                WorkPlayState.Unplayed -> R.string.state_unplayed
                WorkPlayState.InProgress -> R.string.state_in_progress
                else -> R.string.state_played
            },
        )
        stringResource(R.string.empty_played, state, noun)
    } else {
        stringResource(R.string.empty_filters, noun)
    }
}

private fun resolutionLabel(r: Resolution) = when (r) {
    Resolution.FourK -> "4K"
    Resolution.Sd -> "SD"
    else -> r.value
}

private fun dynamicRangeLabel(range: DynamicRange) = if (range == DynamicRange.DolbyVision) "Dolby Vision" else "HDR"

/** Where the library's title starts. */
private val TitleTop = 39.5.dp

/** How far in from the rail's edge a screen's content starts. */
private val ContentStart = 24.dp

/** Where a library's name sits when it's all there is. */
private val LibraryPadding = PaddingValues(start = ContentStart, top = TitleTop, end = SafeEdge)

/** How much further down the grid starts for its section tabs. */
private val SectionsHeight = 38.5.dp

/** The filter menu, under the Filter button, as wide as the Samsung TV app's. */
private val FilterMenuTop = 72.dp
private val FilterMenuWidth = 560.dp

/** The sort menu, under the count line that opens it. */
private val SortMenuWidth = 280.dp

/** The room a long library name leaves at the screen's margin for Filter beside it. */
private val FilterRoom = 96.dp

/** The letter rail's width: the web's token is the web's own. */
internal val LetterRailWidth = 42.dp

/** Where the letter rail's top sits, below the Filter button. */
private val LetterRailTop = 83.5.dp

/**
 * A library's grid: the library's name and Filter, which gets a dot while
 * any filter is on; its sections (Collections when it has any); the count
 * line, "237 movies · A–Z", which opens Sort; posters, or landscape cards
 * for other videos; and the letter rail at the right for the title sort,
 * reached with Right from the last column, which stays put as the rest
 * scrolls. Channel Up and Down page the grid.
 */
@OptIn(ExperimentalAnimationApi::class)
@Composable
fun LibraryScreen(session: ServerSession, libraryId: String, navigator: Navigator) {
    val model = viewModel(key = libraryId) { LibraryViewModel(session, libraryId) }
    val state by model.grid.state.collectAsStateWithLifecycle()
    val library = state.library
    val shownGrid = state.pager
    val pages by shownGrid.state.collectAsStateWithLifecycle()
    val failed = pages.problem

    if (failed != null) {
        if (library == null) {
            Failed(failed, session.session.baseUrl, model::retry, session.graph::changeServer)
        } else {
            // The library's name stays, with why its grid couldn't be read.
            Column(Modifier.fillMaxSize().padding(LibraryPadding)) {
                Text(library.name, style = TvType.title, color = Kivo.colors.text)
                InlineFailed(failed, session.session.baseUrl, model::retry, Modifier.padding(top = Kivo.space(4)), focus = true)
            }
        }
        return
    }
    // The grid's place depends on whether the sections show, so it waits to know.
    if (pages.total != null && state.offerRead != OfferRead.Pending) model.shown = true
    SkeletonGate(Unit.takeIf { model.shown }, { LibrarySkeleton(library) }) {
        val sections = LibraryTab.Collections in state.tabs
        val top = GridTop + if (sections) SectionsHeight else 0.dp
        // Kept here, for the header to scroll with the grid on screen.
        val list = rememberSaveable(shownGrid, saver = LazyListState.Saver) { LazyListState() }
        val gridEntry = remember { FocusRequester() }
        Box(Modifier.fillMaxSize()) {
            // A new section, sort or filter's grid fades in over the one it replaces. What's under the fade saves its
            // state by the content key, so the list stays out of it: it's a new object each time it's restored.
            updateTransition(Triple(shownGrid, state.shown, list), label = "grid").Crossfade(
                animationSpec = fadeSpec(),
                contentKey = { (grid, shown) -> grid to shown },
            ) { (grid, shown, gridList) ->
                val gridPages by grid.state.collectAsStateWithLifecycle()
                if (gridPages.total == null) {
                    SkeletonCards(shown.shape, top)
                } else {
                    GridWithRail(grid, gridList, gridPages, shown, library, navigator, model, top, gridEntry, onClearFilters = { model.choose(keepFocus = false) { clearFilters() } })
                }
            }
            Header(model, state, sections, pages.total, list, gridEntry, letterRail = pages.letters.isNotEmpty())
        }
    }
}

/** The panel the header has open. */
private enum class HeaderPanel { Sort, Filter }

/**
 * The library's name and Filter, its sections, and the count line, which opens
 * Sort for its works. It scrolls with the [grid], back to the top as it takes focus. Down from
 * it goes to the grid by [gridEntry], or from Filter to the letter rail below it where there is one.
 */
@Composable
private fun Header(
    model: LibraryViewModel,
    state: LibraryGridState,
    sections: Boolean,
    total: Int?,
    grid: LazyListState,
    gridEntry: FocusRequester,
    letterRail: Boolean,
) {
    var panel by remember { mutableStateOf<HeaderPanel?>(null) }
    val library = state.library
    val scope = rememberCoroutineScope()
    val downToGrid = Modifier.focusProperties { down = gridEntry }
    // Over Collections' grid there's no Sort between the tabs and the cards.
    val tabs = if (state.tab == LibraryTab.Collections) downToGrid else Modifier
    Box(
        Modifier
            .fillMaxWidth()
            .scrollsWith(grid)
            .onFocusChanged { if (it.hasFocus) scope.launch { grid.scrollToItem(0) } },
    ) {
        Column(Modifier.padding(start = ContentStart, top = TitleTop, end = SafeEdge + FilterRoom)) {
            Text(library?.name.orEmpty(), style = TvType.title, color = Kivo.colors.text, maxLines = 1, overflow = TextOverflow.Ellipsis)
            if (sections) {
                Row(Modifier.padding(top = 13.dp), horizontalArrangement = Arrangement.spacedBy(7.dp)) {
                    val titles = stringResource(
                        when (library?.kind) {
                            LibraryKind.Movies -> R.string.results_movies
                            LibraryKind.Shows -> R.string.results_shows
                            else -> R.string.section_videos
                        },
                    )
                    SectionTab(titles, state.tab == LibraryTab.Titles, { model.choose { choose(LibraryTab.Titles) } }, tabs.testTag("section:titles"))
                    SectionTab(
                        stringResource(R.string.collections),
                        state.tab == LibraryTab.Collections,
                        { model.choose { choose(LibraryTab.Collections) } },
                        tabs.testTag("section:collections"),
                    )
                }
            }
            CountLine(state, total, gridEntry, Modifier.padding(top = if (sections) 10.5.dp else 13.dp)) { panel = HeaderPanel.Sort }
        }
        if (state.tab == LibraryTab.Titles) {
            FilterButton(!state.filters.isEmpty, (if (letterRail) Modifier else downToGrid).align(Alignment.TopEnd).padding(top = 40.5.dp, end = SafeEdge)) { panel = HeaderPanel.Filter }
        }
    }

    when (panel) {
        HeaderPanel.Sort -> {
            val sorts = state.sorts
            MenuPopup(
                onDismiss = { panel = null },
                alignment = Alignment.TopStart,
                margin = PaddingValues(start = RailWidth + ContentStart, top = GridTop + if (sections) SectionsHeight else 0.dp, bottom = SafeTop),
                width = SortMenuWidth,
            ) {
                ChoiceList(
                    choices = sorts.map { Choice(it.value, sortName(it)) },
                    selected = state.sort.value,
                    onSelect = { id ->
                        panel = null
                        sorts.firstOrNull { it.value == id }?.let { sort -> model.choose { choose(sort) } }
                    },
                )
            }
        }
        HeaderPanel.Filter -> FilterPanel(model, state, onClose = { panel = null })
        null -> Unit
    }
}

/** A section's tab: a pill, filled with the accent while it's the one showing. */
@Composable
private fun SectionTab(label: String, current: Boolean, onClick: () -> Unit, modifier: Modifier = Modifier) {
    val colors = Kivo.colors
    Surface(
        onClick = onClick,
        modifier = modifier.height(28.dp),
        shape = ClickableSurfaceDefaults.shape(RoundedCornerShape(KivoTokens.Radius.full)),
        colors = ClickableSurfaceDefaults.colors(
            containerColor = if (current) colors.accent else colors.surfaceRaised,
            contentColor = if (current) colors.onAccent else colors.textMuted,
            focusedContainerColor = Lit.container,
            focusedContentColor = Lit.content,
        ),
        scale = ClickableSurfaceDefaults.scale(focusedScale = KivoTokens.Tv.focusScale),
    ) {
        Box(Modifier.height(28.dp).padding(horizontal = 14.dp), contentAlignment = Alignment.Center) {
            Text(label, style = TvType.smallStrong, maxLines = 1)
        }
    }
}

/**
 * The count line, "237 movies · A–Z", one control that opens Sort, its words
 * in line with the title above; a section with no sort just says how many.
 * Until the first page arrives it's a placeholder.
 */
@Composable
private fun CountLine(state: LibraryGridState, total: Int?, gridEntry: FocusRequester, modifier: Modifier = Modifier, onSort: () -> Unit) {
    val colors = Kivo.colors
    val count = total?.let { countNoun(it, state.tab, state.library) }
    if (state.tab != LibraryTab.Titles) {
        if (count != null) {
            Text(count, style = TvType.small, color = colors.textMuted, modifier = modifier)
        } else {
            SkeletonLine(180.dp, TvType.small, modifier)
        }
        return
    }
    Box(modifier) {
        Surface(
            onClick = onSort,
            modifier = Modifier.offset(x = (-8).dp, y = (-2).dp).focusProperties { down = gridEntry }.testTag("sort"),
            shape = ClickableSurfaceDefaults.shape(RoundedCornerShape(KivoTokens.Radius.sm)),
            colors = ClickableSurfaceDefaults.colors(
                containerColor = Color.Transparent,
                contentColor = colors.textMuted,
                focusedContainerColor = Lit.container,
                focusedContentColor = Lit.content,
            ),
            scale = ClickableSurfaceDefaults.scale(focusedScale = KivoTokens.Tv.focusScale),
        ) {
            Row(Modifier.padding(horizontal = 8.dp, vertical = 2.dp), verticalAlignment = Alignment.CenterVertically) {
                if (count != null) Text(count, style = TvType.small) else SkeletonLine(120.dp, TvType.small)
                Text(" · ", style = TvType.small)
                Text(sortShort(state.sort), style = TvType.smallStrong)
                KivoIconView(KivoIcon.ChevronDown, Modifier.padding(start = 3.dp), size = 14.dp)
            }
        }
    }
}

/** Filter, at the top right; a dot while any filter is on. */
@Composable
private fun FilterButton(filtered: Boolean, modifier: Modifier = Modifier, onClick: () -> Unit) {
    val colors = Kivo.colors
    Box(modifier) {
        Surface(
            onClick = onClick,
            modifier = Modifier.height(28.dp).testTag("filter"),
            shape = ClickableSurfaceDefaults.shape(RoundedCornerShape(KivoTokens.Radius.md)),
            colors = ClickableSurfaceDefaults.colors(
                containerColor = colors.surfaceRaised,
                contentColor = colors.text,
                focusedContainerColor = Lit.container,
                focusedContentColor = Lit.content,
            ),
            border = ClickableSurfaceDefaults.border(
                border = Border(BorderStroke(1.dp, colors.border), shape = RoundedCornerShape(KivoTokens.Radius.md)),
                focusedBorder = Border.None,
            ),
            scale = ClickableSurfaceDefaults.scale(focusedScale = KivoTokens.Tv.focusScale),
        ) {
            Row(Modifier.height(28.dp).padding(horizontal = 10.dp), verticalAlignment = Alignment.CenterVertically) {
                Text(stringResource(R.string.filter), style = TvType.body, maxLines = 1)
                KivoIconView(KivoIcon.ChevronDown, Modifier.padding(start = 6.dp), size = 16.dp)
            }
        }
        if (filtered) {
            Box(
                Modifier
                    .align(Alignment.TopEnd)
                    .offset(x = 3.dp, y = (-3).dp)
                    .size(10.dp)
                    .background(colors.accent, RoundedCornerShape(KivoTokens.Radius.full)),
            )
        }
    }
}

/**
 * The filters, as the Samsung TV app's: the decade, genre and content rating with the played
 * state, and the resolutions and HDR formats the library has.
 * Each applies as it's chosen, and the panel stays open; Clear filters
 * turns them all off and closes it.
 */
@Composable
private fun FilterPanel(model: LibraryViewModel, state: LibraryGridState, onClose: () -> Unit) {
    val chosen = state.filters
    val any = stringResource(R.string.any)
    val playedState = stringResource(R.string.played_state)
    val decade = stringResource(R.string.decade)
    fun values(kind: FilterGroupKind) = state.offer.firstOrNull { it.kind == kind }?.values.orEmpty()
    val quality = values(FilterGroupKind.Quality)
    // The panel's groups in its order, the resolutions and the dynamic ranges each in the order this app knows them.
    val groups = listOf(
        playedState to values(FilterGroupKind.WatchStatus),
        decade to values(FilterGroupKind.Decade),
        stringResource(R.string.genre) to values(FilterGroupKind.Genre),
        stringResource(R.string.content_rating) to values(FilterGroupKind.ContentRating),
        stringResource(R.string.resolution) to quality.filterIsInstance<GridFilter.Resolution>().sortedBy { it.resolution },
        stringResource(R.string.dynamic_range) to quality.filterIsInstance<GridFilter.DynamicRange>().sortedBy { it.range },
    )
    val offered = groups.flatMap { it.second }
    val choices = buildList {
        for ((section, filters) in groups) {
            if (section == playedState) add(Choice(ANY_PLAYED, any, section = section))
            // The decades show once the library's values are read, as Any alone when it has none.
            if (section == decade && state.offerRead == OfferRead.Read) add(Choice(ANY_DECADE, any, section = section))
            for (filter in filters) add(Choice(filter.key, filterLabel(filter), section = section))
        }
        if (!chosen.isEmpty) add(Choice("clear", stringResource(R.string.clear_filters), section = " "))
    }
    val ticked = buildSet {
        chosen.chosen.forEach { add(it.key) }
        if (chosen.played == null) add(ANY_PLAYED)
        if (chosen.chosen.none { it.group == FilterGroupKind.Decade }) add(ANY_DECADE)
    }
    val select = { id: String ->
        val picked = offered.firstOrNull { it.key == id }
        when {
            id == "clear" -> {
                onClose()
                model.choose { clearFilters() }
            }
            id == ANY_PLAYED -> model.choose { choose(chosen.clearing(FilterGroupKind.WatchStatus)) }
            id == ANY_DECADE -> model.choose { choose(chosen.clearing(FilterGroupKind.Decade)) }
            // The watch status and the decade stay on when chosen again: Any turns them off.
            picked != null -> model.choose { choose(if (picked.group.single) chosen.choosing(picked) else chosen.toggling(picked)) }
        }
    }
    MenuPopup(
        onDismiss = onClose,
        alignment = Alignment.TopEnd,
        margin = PaddingValues(top = FilterMenuTop, end = SafeEdge, bottom = SafeTop),
        width = FilterMenuWidth,
    ) {
        ChipChoices(choices, ticked, select)
    }
}

/** The choices that turn the watch status and the decade off. */
private const val ANY_PLAYED = "played:"
private const val ANY_DECADE = "decade:"

/** A value to filter by, as its chip names it. */
@Composable
private fun filterLabel(filter: GridFilter): String = when (filter) {
    is GridFilter.Played -> stringResource(
        when (filter.state) {
            WorkPlayState.Unplayed -> R.string.unplayed
            WorkPlayState.InProgress -> R.string.filter_in_progress
            WorkPlayState.Played -> R.string.played
        },
    )
    is GridFilter.Decade -> stringResource(R.string.decade_label, filter.year)
    is GridFilter.Genre -> filter.name
    is GridFilter.ContentRating -> filter.name
    is GridFilter.Resolution -> resolutionLabel(filter.resolution)
    is GridFilter.DynamicRange -> dynamicRangeLabel(filter.range)
    is GridFilter.Narrator -> filter.name.orEmpty()
}

/** A library while its first page loads: its name, the count line and a row of cards. */
@Composable
private fun LibrarySkeleton(library: Library?) {
    Box(Modifier.fillMaxSize()) {
        Column(Modifier.padding(LibraryPadding)) {
            if (library != null) {
                Text(library.name, style = TvType.title, color = Kivo.colors.text)
            } else {
                SkeletonLine(200.dp, TvType.title)
            }
            SkeletonLine(180.dp, TvType.small, Modifier.padding(top = 13.dp))
        }
        SkeletonCards(if (library?.kind == LibraryKind.Videos) CardShape.Landscape else CardShape.Poster, GridTop)
    }
}

/** A line of placeholder cards of [shape], [top] down the screen. */
@Composable
private fun SkeletonCards(shape: CardShape, top: Dp) {
    Row(Modifier.padding(start = ContentStart, top = top), horizontalArrangement = Arrangement.spacedBy(CardGap)) {
        repeat(gridColumns(shape)) { SkeletonCard(shape, gridCardWidth(shape)) }
    }
}

@Composable
private fun GridWithRail(
    shown: GridPager,
    grid: LazyListState,
    pages: GridPages,
    showing: Shown,
    library: Library?,
    navigator: Navigator,
    model: LibraryViewModel,
    top: Dp,
    entry: FocusRequester,
    onClearFilters: () -> Unit,
) {
    val total = pages.total ?: 0
    val scope = rememberCoroutineScope()
    if (total == 0) {
        if (showing.empty != GridEmptyCase.Unfiltered) {
            val clear = remember { FocusRequester() }
            EmptyState(emptyFiltered(showing, library), null, Modifier.padding(top = top)) {
                KivoButton(stringResource(R.string.clear_filters), onClearFilters, Modifier.focusRequester(clear), primary = true)
            }
            LaunchedEffect(Unit) { if (model.replaced) model.replaced = false else focusWhenPlaced(clear) }
        } else {
            EmptyState(stringResource(R.string.empty_library), null, Modifier.padding(top = top))
        }
        return
    }
    val shape = showing.shape
    val columns = gridColumns(shape)
    val memory = rememberFocusMemory()
    // A grid replacing another leaves focus where it is, such as on Sort.
    val takesFocus = {
        val replaced = model.replaced
        model.replaced = false
        !replaced
    }
    val density = LocalDensity.current
    Box(Modifier.fillMaxSize()) {
        when (showing.tab) {
            LibraryTab.Titles, LibraryTab.Authors, LibraryTab.BookSeries -> PagedGrid(shown, grid, shape, navigator, top = top, memory = memory, takesFocus = takesFocus, entry = entry)
            LibraryTab.Collections -> PagedCards(shown, grid, shape, top = top, memory = memory, takesFocus = takesFocus, entry = entry) { item, width, cardModifier ->
                val c = (item as? GridItem.Collection)?.collection
                CollectionCard(c, width, { if (c != null) navigator.push(Screen.Collection(c.id)) }, cardModifier)
            }
        }
        if (pages.letters.isNotEmpty()) {
            val current by remember(pages) { derivedStateOf { pages.letterAt(grid.firstVisibleItemIndex * columns) } }
            // The item the rail last jumped to, which Left from it goes back to.
            val jumpedTo = remember { IntArray(1) { NOT_JUMPED } }
            LetterRail(
                current,
                onEnter = { jumpedTo[0] = NOT_JUMPED },
                onJump = { letter ->
                    val offset = pages.letterOffset(letter)
                    jumpedTo[0] = offset
                    scope.launch { grid.scrollToLine(offset / columns, with(density) { top.roundToPx() }, with(density) { SafeTop.roundToPx() }) }
                },
                onLeft = {
                    val offset = jumpedTo[0]
                    if (offset != NOT_JUMPED) scope.launch { focusWhenPlaced { shown.state.value.item(offset)?.id?.let(memory::requester) } }
                    offset != NOT_JUMPED
                },
                modifier = Modifier.align(Alignment.TopEnd).padding(top = LetterRailTop, end = SafeEdge),
            )
        }
    }
}

/** The rail hasn't jumped since it took focus. */
private const val NOT_JUMPED = -1

/**
 * The letter rail: Up and Down move through the letters, OK jumps; the top
 * row's letter is marked. One focusable element draws every letter, the
 * highlighted one in a lit pill while the rail has focus; it takes focus at
 * the marked letter. Up from the first letter leaves it, and so does Left:
 * [onLeft] says it took focus back to the grid itself, else the nearest card does.
 */
@Composable
private fun LetterRail(current: String?, onEnter: () -> Unit, onJump: (String) -> Unit, onLeft: () -> Boolean, modifier: Modifier = Modifier) {
    val colors = Kivo.colors
    val style = TvType.smallStrong.copy(lineHeight = TvType.small.fontSize)
    val measurer = rememberTextMeasurer(cacheSize = 0)
    val layouts = remember(measurer, style) { RAIL_LETTERS.map { measurer.measure(it, style) } }
    var focused by remember { mutableStateOf(false) }
    var highlighted by remember { mutableIntStateOf(0) }
    val letter = RAIL_LETTERS[highlighted]
    // The letters share the rail's height, down to the overscan.
    Box(
        modifier
            .width(LetterRailWidth)
            .fillMaxHeight()
            .padding(bottom = SafeTop)
            .testTag("letters")
            .semantics { contentDescription = letter }
            .onFocusChanged {
                if (it.isFocused && !focused) {
                    highlighted = RAIL_LETTERS.indexOf(current).coerceAtLeast(0)
                    onEnter()
                }
                focused = it.isFocused
            }
            .onKeyEvent { event ->
                val down = event.type == KeyEventType.KeyDown
                when (event.key.nativeKeyCode) {
                    KeyEvent.KEYCODE_DPAD_UP -> {
                        if (highlighted == 0) return@onKeyEvent false
                        if (down) highlighted--
                        true
                    }
                    KeyEvent.KEYCODE_DPAD_DOWN -> {
                        if (down && highlighted < RAIL_LETTERS.lastIndex) highlighted++
                        true
                    }
                    KeyEvent.KEYCODE_DPAD_LEFT -> down && onLeft()
                    KeyEvent.KEYCODE_DPAD_CENTER, KeyEvent.KEYCODE_ENTER, KeyEvent.KEYCODE_NUMPAD_ENTER -> {
                        if (event.type == KeyEventType.KeyUp) onJump(RAIL_LETTERS[highlighted])
                        true
                    }
                    else -> false
                }
            }
            .focusable()
            .drawBehind {
                val slot = size.height / RAIL_LETTERS.size
                RAIL_LETTERS.indices.forEach { i ->
                    val top = slot * i
                    val layout = layouts[i]
                    val origin = Offset((size.width - layout.size.width) / 2, top + (slot - layout.size.height) / 2)
                    if (focused && i == highlighted) {
                        val pill = LetterPillWidth.toPx()
                        drawRoundRect(Lit.container, Offset((size.width - pill) / 2, top), Size(pill, slot), CornerRadius(slot / 2))
                        drawText(layout, Lit.content, origin)
                    } else {
                        drawText(layout, if (RAIL_LETTERS[i] == current) colors.accent else colors.textMuted, origin)
                    }
                }
            },
    )
}

/** How wide the focused letter's pill is. */
private val LetterPillWidth = 35.dp
