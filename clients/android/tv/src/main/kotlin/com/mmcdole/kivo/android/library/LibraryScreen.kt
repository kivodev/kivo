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
import com.mmcdole.kivo.api.models.CollectionSummary
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.data.GridPager
import com.mmcdole.kivo.core.data.GridPages
import com.mmcdole.kivo.core.data.GridPages.Companion.RAIL_LETTERS
import com.mmcdole.kivo.core.data.GridQuery
import com.mmcdole.kivo.core.data.LibraryFilters
import java.text.NumberFormat
import kotlinx.coroutines.launch

/**
 * A library's grid behind the screen: the section, sort and
 * filters this device last used in it, the grid showing, and what the
 * library has to filter by. A new section, sort or filter shows at once,
 * its placeholders until its first page arrives; a grid seen lately shows
 * at once while it's read again.
 */
class LibraryViewModel(private val session: ServerSession, private val libraryId: String) : ViewModel() {
    /** The section, sort and filters chosen, which the grid shows. */
    var choice by mutableStateOf(gridChoice(session.gridChoices.value, libraryId))
        private set

    /** The grid on screen, the chosen section's. */
    var grid by mutableStateOf(gridOf(choice))
        private set

    /** What the library has to filter by, and whether it has collections, once known; a failed read offers neither. */
    var filters by mutableStateOf<LibraryFilters?>(null)
        private set

    /** Whether that read has answered, or failed, so whether the sections show is known. */
    var filtersRead by mutableStateOf(false)
        private set

    /**
     * The grid on screen replaced another and focus stays where it was,
     * such as on Sort; a grid that replaced the empty state's Clear filters
     * takes focus itself.
     */
    var replaced = false

    /** Whether a grid has shown, after which another shows in its place without the screen's skeleton. */
    var shown = false

    init {
        viewModelScope.launch {
            session.session.library.filters(libraryId).collect { load ->
                load.getOrNull()?.let { filters = it }
                if (load !is Load.Loading) filtersRead = true
            }
        }
    }

    /** Shows [next], keeping it for the library on this device. */
    fun choose(next: GridChoice, keepFocus: Boolean = true) {
        if (next == choice) return
        choice = next
        session.graph.scope.launch { session.graph.savedGrids.save(next) }
        val old = grid
        replaced = keepFocus
        grid = gridOf(next)
        old.pager.close()
    }

    /** Shows the works with [query], keeping the section. */
    fun choose(query: GridQuery, keepFocus: Boolean = true) = choose(choice.copy(query = query), keepFocus)

    /** Reads the grid again, after its first page failed. */
    fun retry() {
        viewModelScope.launch { grid.pager.refresh() }
    }

    override fun onCleared() {
        grid.pager.close()
    }

    private fun gridOf(choice: GridChoice): LibraryGrid = when (choice.section) {
        Section.Titles -> LibraryGrid.Works(session.session.library.pagedWorks(choice.query))
        Section.Collections -> LibraryGrid.Collections(session.session.library.pagedCollections(libraryId))
    }
}

/** The grid a library shows: its works, or its collections. */
sealed interface LibraryGrid {
    val pager: GridPager<*>

    class Works(override val pager: GridPager<WorkSummary>) : LibraryGrid

    class Collections(override val pager: GridPager<CollectionSummary>) : LibraryGrid
}

/** The sorts the TV offers for [library], as every app offers them, with the title sort's letter rail. */
private fun sortsOf(library: Library?) = GridValues.sortsOf(library?.kind)

@Composable
private fun sortName(sort: String) = stringResource(
    when (sort) {
        GridValues.ADDED_AT -> R.string.sort_added
        GridValues.LAST_EPISODE_ADDED_AT -> R.string.sort_last_episode_added
        GridValues.RELEASE_DATE -> R.string.sort_release
        GridValues.RATING -> R.string.sort_rating
        GridValues.RUNTIME -> R.string.sort_runtime
        GridValues.LAST_PLAYED_AT -> R.string.sort_last_played
        GridValues.AUTHOR -> R.string.sort_author
        else -> R.string.sort_title
    },
)

/** The sort in a word or two, for the count line: "A–Z", "Recently added". */
@Composable
private fun sortShort(sort: String) = stringResource(
    when (sort) {
        GridValues.ADDED_AT -> R.string.sort_short_added
        GridValues.LAST_EPISODE_ADDED_AT -> R.string.sort_short_last_episode_added
        GridValues.RELEASE_DATE -> R.string.sort_short_release
        GridValues.RATING -> R.string.sort_short_rating
        GridValues.RUNTIME -> R.string.sort_short_runtime
        GridValues.LAST_PLAYED_AT -> R.string.sort_short_last_played
        GridValues.AUTHOR -> R.string.sort_author
        else -> R.string.sort_short_title
    },
)

/** "237 movies", "5 collections": how many of what the section lists. */
@Composable
private fun countNoun(count: Int, section: Section, library: Library?): String {
    val plural = when {
        section == Section.Collections -> R.plurals.count_collections
        library?.kind == "movies" -> R.plurals.count_movies
        library?.kind == "shows" -> R.plurals.count_shows
        else -> R.plurals.count_videos
    }
    return pluralStringResource(plural, count, NumberFormat.getIntegerInstance().format(count))
}

/** "No unwatched movies" for the watch status alone, else "No movies match these filters". */
@Composable
private fun emptyFiltered(query: GridQuery, library: Library?): String {
    val noun = stringResource(
        when (library?.kind) {
            "movies" -> R.string.noun_movies
            "shows" -> R.string.noun_shows
            else -> R.string.noun_videos
        },
    )
    val played = query.played
    return if (played != null && query.unfiltered().copy(played = played) == query) {
        val state = stringResource(
            when (played) {
                GridValues.UNPLAYED -> R.string.state_unplayed
                GridValues.IN_PROGRESS -> R.string.state_in_progress
                else -> R.string.state_played
            },
        )
        stringResource(R.string.empty_played, state, noun)
    } else {
        stringResource(R.string.empty_filters, noun)
    }
}

private fun resolutionLabel(r: String) = when (r) {
    "4k" -> "4K"
    "sd" -> "SD"
    else -> r
}

private fun dynamicRangeLabel(range: String) = if (range == GridValues.DOLBY_VISION) "Dolby Vision" else "HDR"

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
    val libraries by session.libraryList.collectAsStateWithLifecycle()
    val library = libraries?.firstOrNull { it.id == libraryId }
    val shownGrid = model.grid
    val pages by shownGrid.pager.state.collectAsStateWithLifecycle()
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
    if (pages.total != null && model.filtersRead) model.shown = true
    SkeletonGate(Unit.takeIf { model.shown }, { LibrarySkeleton(library) }) {
        val choice = model.choice
        val sections = hasSections(model, choice)
        val top = GridTop + if (sections) SectionsHeight else 0.dp
        // Kept here, for the header to scroll with the grid on screen.
        val list = rememberSaveable(shownGrid, saver = LazyListState.Saver) { LazyListState() }
        val gridEntry = remember { FocusRequester() }
        Box(Modifier.fillMaxSize()) {
            // A new section, sort or filter's grid fades in over the one it replaces. What's under the fade saves its
            // state by the content key, so the list stays out of it: it's a new object each time it's restored.
            updateTransition(Triple(shownGrid, choice, list), label = "grid").Crossfade(
                animationSpec = fadeSpec(),
                contentKey = { (grid, gridChoice) -> grid to gridChoice },
            ) { (grid, gridChoice, gridList) ->
                val gridPages by grid.pager.state.collectAsStateWithLifecycle()
                if (gridPages.total == null) {
                    SkeletonCards(library, gridChoice, top)
                } else {
                    GridWithRail(grid, gridList, gridPages, gridChoice, library, navigator, model, top, gridEntry, onClearFilters = { model.choose(choice.query.unfiltered(), keepFocus = false) })
                }
            }
            Header(model, choice, library, sections, pages.total, list, gridEntry, letterRail = pages.letters.isNotEmpty())
        }
    }
}

/** Whether the library's sections show: when it has collections to show beside its works. */
private fun hasSections(model: LibraryViewModel, choice: GridChoice) =
    model.filters?.collections == true || choice.section == Section.Collections

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
    choice: GridChoice,
    library: Library?,
    sections: Boolean,
    total: Int?,
    grid: LazyListState,
    gridEntry: FocusRequester,
    letterRail: Boolean,
) {
    var panel by remember { mutableStateOf<HeaderPanel?>(null) }
    val query = choice.query
    val scope = rememberCoroutineScope()
    val downToGrid = Modifier.focusProperties { down = gridEntry }
    // Over Collections' grid there's no Sort between the tabs and the cards.
    val tabs = if (choice.section == Section.Collections) downToGrid else Modifier
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
                            "movies" -> R.string.results_movies
                            "shows" -> R.string.results_shows
                            else -> R.string.section_videos
                        },
                    )
                    SectionTab(titles, choice.section == Section.Titles, { model.choose(choice.copy(section = Section.Titles)) }, tabs.testTag("section:titles"))
                    SectionTab(
                        stringResource(R.string.collections),
                        choice.section == Section.Collections,
                        { model.choose(choice.copy(section = Section.Collections)) },
                        tabs.testTag("section:collections"),
                    )
                }
            }
            CountLine(choice, library, total, gridEntry, Modifier.padding(top = if (sections) 10.5.dp else 13.dp)) { panel = HeaderPanel.Sort }
        }
        if (choice.section == Section.Titles) {
            FilterButton(query.filtered, (if (letterRail) Modifier else downToGrid).align(Alignment.TopEnd).padding(top = 40.5.dp, end = SafeEdge)) { panel = HeaderPanel.Filter }
        }
    }

    when (panel) {
        HeaderPanel.Sort -> {
            val sorts = sortsOf(library)
            MenuPopup(
                onDismiss = { panel = null },
                alignment = Alignment.TopStart,
                margin = PaddingValues(start = RailWidth + ContentStart, top = GridTop + if (sections) SectionsHeight else 0.dp, bottom = SafeTop),
                width = SortMenuWidth,
            ) {
                ChoiceList(
                    choices = sorts.map { Choice(it, sortName(it)) },
                    selected = query.sort,
                    onSelect = { id ->
                        panel = null
                        sorts.firstOrNull { it == id }?.let { model.choose(query.copy(sort = it)) }
                    },
                )
            }
        }
        HeaderPanel.Filter -> FilterPanel(model, query, onClose = { panel = null })
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
private fun CountLine(choice: GridChoice, library: Library?, total: Int?, gridEntry: FocusRequester, modifier: Modifier = Modifier, onSort: () -> Unit) {
    val colors = Kivo.colors
    val count = total?.let { countNoun(it, choice.section, library) }
    if (choice.section != Section.Titles) {
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
                Text(choice.query.sort?.let { sortShort(it) }.orEmpty(), style = TvType.smallStrong)
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
private fun FilterPanel(model: LibraryViewModel, query: GridQuery, onClose: () -> Unit) {
    val filters = model.filters
    val playedState = stringResource(R.string.played_state)
    val decade = stringResource(R.string.decade)
    val genre = stringResource(R.string.genre)
    val contentRating = stringResource(R.string.content_rating)
    val resolution = stringResource(R.string.resolution)
    val dynamicRange = stringResource(R.string.dynamic_range)
    val choices = buildList {
        add(Choice("played:", stringResource(R.string.any), section = playedState))
        add(Choice("played:${GridValues.UNPLAYED}", stringResource(R.string.unplayed), section = playedState))
        add(Choice("played:${GridValues.IN_PROGRESS}", stringResource(R.string.filter_in_progress), section = playedState))
        add(Choice("played:${GridValues.PLAYED}", stringResource(R.string.played), section = playedState))
        filters?.let { f ->
            add(Choice("decade:", stringResource(R.string.any), section = decade))
            decadesOf(f.years).forEach { add(Choice("decade:$it", stringResource(R.string.decade_label, it), section = decade)) }
            f.genres.forEach { add(Choice("genre:$it", it, section = genre)) }
            f.contentRatings.forEach { add(Choice("contentRating:$it", it, section = contentRating)) }
            GridValues.resolutions.filter { it in f.resolutions }
                .forEach { add(Choice("resolution:$it", resolutionLabel(it), section = resolution)) }
            GridValues.dynamicRanges.filter { it in f.dynamicRanges }
                .forEach { add(Choice("dynamicRange:$it", dynamicRangeLabel(it), section = dynamicRange)) }
        }
        if (query.filtered) add(Choice("clear", stringResource(R.string.clear_filters), section = " "))
    }
    val ticked = buildSet {
        add("played:${query.played.orEmpty()}")
        add("decade:${query.yearFrom ?: ""}")
        query.genres?.forEach { add("genre:$it") }
        query.contentRatings?.forEach { add("contentRating:$it") }
        query.resolutions?.forEach { add("resolution:$it") }
        query.dynamicRanges?.forEach { add("dynamicRange:$it") }
    }
    val select = { id: String ->
        val (kind, value) = id.split(':', limit = 2).let { it[0] to it.getOrElse(1) { "" } }
        when (kind) {
            "played" -> model.choose(query.copy(played = value.takeIf { it in GridValues.playedStates }))
            "decade" -> {
                val from = value.toIntOrNull()
                model.choose(query.copy(yearFrom = from, yearTo = from?.plus(9)))
            }
            "genre" -> {
                val now = query.genres.orEmpty()
                model.choose(query.copy(genres = (if (value in now) now - value else now + value).ifEmpty { null }))
            }
            "contentRating" -> {
                val now = query.contentRatings.orEmpty()
                model.choose(query.copy(contentRatings = (if (value in now) now - value else now + value).ifEmpty { null }))
            }
            "resolution" -> {
                val now = query.resolutions.orEmpty()
                model.choose(query.copy(resolutions = (if (value in now) now - value else now + value).ifEmpty { null }))
            }
            "dynamicRange" -> {
                val now = query.dynamicRanges.orEmpty()
                model.choose(query.copy(dynamicRanges = (if (value in now) now - value else now + value).ifEmpty { null }))
            }
            "clear" -> {
                onClose()
                model.choose(query.unfiltered())
            }
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
        SkeletonCards(library, null, GridTop)
    }
}

/** A line of placeholder cards, in the grid's shape for [choice], [top] down the screen. */
@Composable
private fun SkeletonCards(library: Library?, choice: GridChoice?, top: Dp) {
    val shape = gridShape(library, choice)
    Row(Modifier.padding(start = ContentStart, top = top), horizontalArrangement = Arrangement.spacedBy(CardGap)) {
        repeat(gridColumns(shape)) { SkeletonCard(shape, gridCardWidth(shape)) }
    }
}

/** Landscape cards for a videos library's works, else posters. */
private fun gridShape(library: Library?, choice: GridChoice?) =
    if (library?.kind == "videos" && choice?.section != Section.Collections) CardShape.Landscape else CardShape.Poster

@Composable
private fun GridWithRail(
    shown: LibraryGrid,
    grid: LazyListState,
    pages: GridPages<*>,
    choice: GridChoice,
    library: Library?,
    navigator: Navigator,
    model: LibraryViewModel,
    top: Dp,
    entry: FocusRequester,
    onClearFilters: () -> Unit,
) {
    val total = pages.total ?: 0
    val query = choice.query
    val scope = rememberCoroutineScope()
    if (total == 0) {
        if (choice.section == Section.Titles && query.filtered) {
            val clear = remember { FocusRequester() }
            EmptyState(emptyFiltered(query, library), null, Modifier.padding(top = top)) {
                KivoButton(stringResource(R.string.clear_filters), onClearFilters, Modifier.focusRequester(clear), primary = true)
            }
            LaunchedEffect(Unit) { if (model.replaced) model.replaced = false else focusWhenPlaced(clear) }
        } else {
            EmptyState(stringResource(R.string.empty_library), null, Modifier.padding(top = top))
        }
        return
    }
    val shape = gridShape(library, choice)
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
        when (shown) {
            is LibraryGrid.Works -> PagedGrid(shown.pager, grid, shape, navigator, top = top, memory = memory, takesFocus = takesFocus, entry = entry)
            is LibraryGrid.Collections -> PagedCards(shown.pager, grid, shape, { it.id }, top = top, memory = memory, takesFocus = takesFocus, entry = entry) { c, width, cardModifier ->
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
                    if (offset != NOT_JUMPED) scope.launch { focusWhenPlaced { shown.itemKey(offset)?.let(memory::requester) } }
                    offset != NOT_JUMPED
                },
                modifier = Modifier.align(Alignment.TopEnd).padding(top = LetterRailTop, end = SafeEdge),
            )
        }
    }
}

/** The rail hasn't jumped since it took focus. */
private const val NOT_JUMPED = -1

/** The focus key of the item at [index], once its page is read. */
private fun LibraryGrid.itemKey(index: Int): String? = when (this) {
    is LibraryGrid.Works -> pager.state.value.item(index)?.id
    is LibraryGrid.Collections -> pager.state.value.item(index)?.id
}

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
