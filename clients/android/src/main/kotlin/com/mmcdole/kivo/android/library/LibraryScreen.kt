package com.mmcdole.kivo.android.library

import android.view.KeyEvent
import androidx.compose.animation.Crossfade
import androidx.compose.foundation.background
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
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
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.scale
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.nativeKeyCode
import androidx.compose.ui.input.key.onKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.drawText
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.R
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.ui.CardGap
import com.mmcdole.kivo.android.ui.CardShape
import com.mmcdole.kivo.android.ui.CollectionCard
import com.mmcdole.kivo.android.ui.Chip
import com.mmcdole.kivo.android.ui.Choice
import com.mmcdole.kivo.android.ui.ChoicePanel
import com.mmcdole.kivo.android.ui.EmptyState
import com.mmcdole.kivo.android.ui.Failed
import com.mmcdole.kivo.android.ui.InlineFailed
import com.mmcdole.kivo.android.ui.KivoButton
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.android.ui.SafeEdge
import com.mmcdole.kivo.android.ui.SafeTop
import com.mmcdole.kivo.android.ui.SkeletonCard
import com.mmcdole.kivo.android.ui.SkeletonGate
import com.mmcdole.kivo.android.ui.SkeletonLine
import com.mmcdole.kivo.android.ui.fadeSpec
import com.mmcdole.kivo.android.ui.focusWhenPlaced
import com.mmcdole.kivo.api.models.CollectionSummary
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.LibraryFacets
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.data.GridPager
import com.mmcdole.kivo.core.data.GridPages
import com.mmcdole.kivo.core.data.GridPages.Companion.RAIL_LETTERS
import com.mmcdole.kivo.core.data.GridQuery
import kotlinx.coroutines.launch
import java.text.NumberFormat

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
    var facets by mutableStateOf<LibraryFacets?>(null)
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
            session.session.library.facets(libraryId).collect { load -> load.getOrNull()?.let { facets = it } }
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

/**
 * The sorts the TV offers, with the title sort's letter rail; a
 * shows library also sorts by its last episode added.
 */
private fun sortsOf(library: Library?) =
    GridValues.sorts.filter { it != GridValues.LAST_EPISODE_ADDED_AT || library?.kind == "shows" }

@Composable
private fun sortName(sort: String) = stringResource(
    when (sort) {
        GridValues.ADDED_AT -> R.string.sort_added
        GridValues.LAST_EPISODE_ADDED_AT -> R.string.sort_last_episode_added
        GridValues.RELEASE_DATE -> R.string.sort_release
        GridValues.RATING -> R.string.sort_rating
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

/** Where the grid sits beside the rail. */
private val LibraryPadding = PaddingValues(start = 24.dp, top = SafeTop, end = 12.dp)

/**
 * A library's grid: the library's name with its sections
 * (Collections when it has any), Sort and Filter, which gets a dot while
 * any filter is on; the count line, "237 movies · A–Z"; posters, or
 * landscape cards for other videos; and the letter rail at the right for
 * the title sort, reached with Right from the last column. Channel Up and
 * Down page the grid.
 */
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
                Text(library.name, style = TvType.heading, color = Kivo.colors.text)
                InlineFailed(failed, session.session.baseUrl, model::retry, Modifier.padding(top = GridTop), focus = true)
            }
        }
        return
    }
    if (pages.total != null) model.shown = true
    SkeletonGate(Unit.takeIf { model.shown }, { LibrarySkeleton(library) }) {
        val choice = model.choice
        val query = choice.query
        val total = pages.total
        Column(Modifier.fillMaxSize().padding(LibraryPadding)) {
            Header(model, choice, library)
            // Another section, sort or filter's count and cards show placeholders until its first page arrives.
            if (total == null) {
                SkeletonLine(180.dp, TvType.body, Modifier.padding(top = Kivo.space(2)))
            } else {
                Text(
                    listOfNotNull(
                        countNoun(total, choice.section, library),
                        query.sort?.takeIf { choice.section == Section.Titles }?.let { sortShort(it) },
                    ).joinToString(" · "),
                    style = TvType.body,
                    color = Kivo.colors.textMuted,
                    modifier = Modifier.padding(top = Kivo.space(2)),
                )
            }
            // A new section, sort or filter's grid fades in over the one it replaces.
            Crossfade(shownGrid to choice, animationSpec = fadeSpec(), label = "grid") { (grid, gridChoice) ->
                val gridPages by grid.pager.state.collectAsStateWithLifecycle()
                if (gridPages.total == null) {
                    SkeletonCards(library, gridChoice)
                } else {
                    GridWithRail(grid, gridPages, gridChoice, library, navigator, model, onClearFilters = { model.choose(query.unfiltered(), keepFocus = false) })
                }
            }
        }
    }
}

/** The panel the header has open. */
private enum class HeaderPanel { Sort, Filter }

/** The library's name, its sections, and Sort and Filter for its works. */
@Composable
private fun Header(model: LibraryViewModel, choice: GridChoice, library: Library?) {
    var panel by remember { mutableStateOf<HeaderPanel?>(null) }
    val query = choice.query
    val hasCollections = model.facets?.collections == true || choice.section == Section.Collections
    // Clear of overscan at the right, where the letter rail below reaches nearer the edge.
    Row(Modifier.padding(end = SafeEdge - 12.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(Kivo.space(3))) {
        Text(library?.name.orEmpty(), style = TvType.heading, color = Kivo.colors.text, modifier = Modifier.padding(end = Kivo.space(3)))
        if (hasCollections) {
            val titles = stringResource(
                when (library?.kind) {
                    "movies" -> R.string.results_movies
                    "shows" -> R.string.results_shows
                    else -> R.string.section_videos
                },
            )
            Chip(titles, choice.section == Section.Titles, { model.choose(choice.copy(section = Section.Titles)) }, Modifier.testTag("section:titles"))
            Chip(
                stringResource(R.string.collections),
                choice.section == Section.Collections,
                { model.choose(choice.copy(section = Section.Collections)) },
                Modifier.testTag("section:collections"),
            )
        }
        Spacer(Modifier.weight(1f))
        if (choice.section == Section.Titles) {
            KivoButton(stringResource(R.string.sort), { panel = HeaderPanel.Sort }, Modifier.testTag("sort"))
            Box {
                KivoButton(stringResource(R.string.filter), { panel = HeaderPanel.Filter }, Modifier.testTag("filter"))
                // A dot while any filter is on.
                if (query.filtered) {
                    Box(
                        Modifier
                            .align(Alignment.TopEnd)
                            .offset(x = (-6).dp, y = 6.dp)
                            .size(10.dp)
                            .background(Kivo.colors.accent, RoundedCornerShape(KivoTokens.Radius.full)),
                    )
                }
            }
        }
    }

    when (panel) {
        HeaderPanel.Sort -> {
            val sorts = sortsOf(library)
            ChoicePanel(
                title = stringResource(R.string.sort),
                choices = sorts.map { Choice(it, sortName(it)) },
                selected = query.sort,
                onSelect = { id ->
                    panel = null
                    sorts.firstOrNull { it == id }?.let { model.choose(query.copy(sort = it)) }
                },
                onDismiss = { panel = null },
            )
        }
        HeaderPanel.Filter -> FilterPanel(model, query, onClose = { panel = null })
        null -> Unit
    }
}

/**
 * The filters, as the web's and the iPhone's: the played
 * state, Favorites, and the resolutions and HDR formats the library has.
 * Each applies as it's chosen, and the panel stays open; Clear filters
 * turns them all off and closes it.
 */
@Composable
private fun FilterPanel(model: LibraryViewModel, query: GridQuery, onClose: () -> Unit) {
    val facets = model.facets
    val playedState = stringResource(R.string.played_state)
    val resolution = stringResource(R.string.resolution)
    val dynamicRange = stringResource(R.string.dynamic_range)
    val choices = buildList {
        add(Choice("played:", stringResource(R.string.any), section = playedState))
        add(Choice("played:${GridValues.UNPLAYED}", stringResource(R.string.unplayed), section = playedState))
        add(Choice("played:${GridValues.IN_PROGRESS}", stringResource(R.string.filter_in_progress), section = playedState))
        add(Choice("played:${GridValues.PLAYED}", stringResource(R.string.played), section = playedState))
        add(Choice("favorite", stringResource(R.string.favorites), section = ""))
        facets?.let { f ->
            GridValues.resolutions.filter { it in f.resolutions }
                .forEach { add(Choice("resolution:$it", resolutionLabel(it), section = resolution)) }
            GridValues.dynamicRanges.filter { it in f.dynamicRanges }
                .forEach { add(Choice("dynamicRange:$it", dynamicRangeLabel(it), section = dynamicRange)) }
        }
        if (query.filtered) add(Choice("clear", stringResource(R.string.clear_filters), section = " "))
    }
    val ticked = buildSet {
        add("played:${query.played.orEmpty()}")
        if (query.favorite) add("favorite")
        query.resolutions?.forEach { add("resolution:$it") }
        query.dynamicRanges?.forEach { add("dynamicRange:$it") }
    }
    ChoicePanel(
        title = stringResource(R.string.filter),
        choices = choices,
        selected = null,
        ticked = ticked,
        onSelect = { id ->
            val (kind, value) = id.split(':', limit = 2).let { it[0] to it.getOrElse(1) { "" } }
            when (kind) {
                "played" -> model.choose(query.copy(played = value.takeIf { it in GridValues.playedStates }))
                "favorite" -> model.choose(query.copy(favorite = !query.favorite))
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
        },
        onDismiss = onClose,
    )
}

/** A library while its first page loads: its name, the count line and a row of cards. */
@Composable
private fun LibrarySkeleton(library: Library?) {
    Column(Modifier.fillMaxSize().padding(LibraryPadding)) {
        Box(Modifier.height(44.dp), contentAlignment = Alignment.CenterStart) {
            if (library != null) {
                Text(library.name, style = TvType.heading, color = Kivo.colors.text)
            } else {
                SkeletonLine(200.dp, TvType.heading)
            }
        }
        SkeletonLine(180.dp, TvType.body, Modifier.padding(top = Kivo.space(2)))
        SkeletonCards(library, null)
    }
}

/** A line of placeholder cards, in the grid's shape for [choice]. */
@Composable
private fun SkeletonCards(library: Library?, choice: GridChoice?) {
    val shape = gridShape(library, choice)
    Row(Modifier.padding(top = GridTop), horizontalArrangement = Arrangement.spacedBy(CardGap)) {
        repeat(gridColumns(shape)) { SkeletonCard(shape, gridCardWidth(shape)) }
    }
}

/** Landscape cards for a videos library's works, else posters. */
private fun gridShape(library: Library?, choice: GridChoice?) =
    if (library?.kind == "videos" && choice?.section != Section.Collections) CardShape.Landscape else CardShape.Poster

@Composable
private fun GridWithRail(
    shown: LibraryGrid,
    pages: GridPages<*>,
    choice: GridChoice,
    library: Library?,
    navigator: Navigator,
    model: LibraryViewModel,
    onClearFilters: () -> Unit,
) {
    val total = pages.total ?: 0
    val query = choice.query
    val grid = rememberSaveable(saver = LazyListState.Saver) { LazyListState() }
    val scope = rememberCoroutineScope()
    if (total == 0) {
        if (choice.section == Section.Titles && query.filtered) {
            val clear = remember { FocusRequester() }
            EmptyState(emptyFiltered(query, library), null) {
                KivoButton(stringResource(R.string.clear_filters), onClearFilters, Modifier.focusRequester(clear), primary = true)
            }
            LaunchedEffect(Unit) { if (model.replaced) model.replaced = false else focusWhenPlaced(clear) }
        } else {
            EmptyState(stringResource(R.string.empty_library), null)
        }
        return
    }
    val shape = gridShape(library, choice)
    val columns = gridColumns(shape)
    // A grid replacing another leaves focus where it is, such as on Sort.
    val takesFocus = {
        val replaced = model.replaced
        model.replaced = false
        !replaced
    }
    Row(Modifier.fillMaxSize()) {
        when (shown) {
            is LibraryGrid.Works -> PagedGrid(shown.pager, grid, shape, navigator, Modifier.weight(1f), takesFocus)
            is LibraryGrid.Collections -> PagedCards(shown.pager, grid, shape, { it.id }, Modifier.weight(1f), takesFocus) { c, width, cardModifier ->
                CollectionCard(c, width, { if (c != null) navigator.push(Screen.Collection(c.id)) }, cardModifier)
            }
        }
        if (pages.letters.isNotEmpty()) {
            val current by remember(pages) { derivedStateOf { pages.letterAt(grid.firstVisibleItemIndex * columns) } }
            LetterRail(current) { letter -> scope.launch { grid.scrollToItem(pages.letterOffset(letter) / columns) } }
        }
    }
}

/**
 * The letter rail: Up and Down move through the letters, OK jumps; the top
 * row's letter is marked. One focusable element draws every letter, the
 * highlighted one raised in the focus ring while the rail has focus; it
 * takes focus at the marked letter. Up from the first letter leaves it.
 */
@Composable
private fun LetterRail(current: String?, onJump: (String) -> Unit) {
    val colors = Kivo.colors
    val style = TvType.smallStrong.copy(lineHeight = TvType.small.fontSize)
    val measurer = rememberTextMeasurer(cacheSize = 0)
    val layouts = remember(measurer, style) { RAIL_LETTERS.map { measurer.measure(it, style) } }
    var focused by remember { mutableStateOf(false) }
    var highlighted by remember { mutableIntStateOf(0) }
    val letter = RAIL_LETTERS[highlighted]
    val ring = KivoTokens.Tv.focusRingWidth
    val radius = KivoTokens.Radius.sm
    // The letters share the rail's height, as tall as the grid beside it.
    Box(
        Modifier
            .width(KivoTokens.Layout.railWidth)
            .fillMaxHeight()
            .padding(top = 20.dp, bottom = 12.dp)
            .testTag("letters")
            .semantics { contentDescription = letter }
            .onFocusChanged {
                if (it.isFocused && !focused) highlighted = RAIL_LETTERS.indexOf(current).coerceAtLeast(0)
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
                        scale(FocusedLetterScale, pivot = Offset(size.width / 2, top + slot / 2)) {
                            val corner = CornerRadius(radius.toPx())
                            drawRoundRect(colors.surfaceHover, Offset(0f, top), Size(size.width, slot), corner)
                            val inset = ring.toPx() / 2
                            drawRoundRect(
                                colors.focus,
                                Offset(inset, top + inset),
                                Size(size.width - inset * 2, slot - inset * 2),
                                corner,
                                style = Stroke(ring.toPx()),
                            )
                            drawText(layout, colors.text, origin)
                        }
                    } else {
                        drawText(layout, if (RAIL_LETTERS[i] == current) colors.accent else colors.textMuted, origin)
                    }
                }
            },
    )
}

/** How much the focused letter grows. */
private const val FocusedLetterScale = 1.2f
