package com.mmcdole.kivo.core.session

import co.touchlab.skie.configuration.annotations.DefaultArgumentInterop
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.LibraryKind
import com.mmcdole.kivo.api.models.WorkQuery
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.data.FilterGroup
import com.mmcdole.kivo.core.data.FilterGroupKind
import com.mmcdole.kivo.core.data.GridFilter
import com.mmcdole.kivo.core.data.GridFilters
import com.mmcdole.kivo.core.data.GridPager
import com.mmcdole.kivo.core.data.GridQuery
import com.mmcdole.kivo.core.data.GridSort
import com.mmcdole.kivo.core.data.LibraryFilters
import com.mmcdole.kivo.core.data.LibraryTab
import com.mmcdole.kivo.core.data.SavedGrid
import com.mmcdole.kivo.core.data.SavedGrids
import com.mmcdole.kivo.core.data.SmartLists
import com.mmcdole.kivo.core.data.activeFilters
import com.mmcdole.kivo.core.data.filterOffer
import com.mmcdole.kivo.core.data.sortsOf
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/** The shape of a grid's cards. */
enum class GridCardShape { Poster, Landscape, Square }

/** What a grid with nothing in it says: the tab is empty, nothing has the watch status chosen, or nothing matches the filters. */
enum class GridEmptyCase { Unfiltered, WatchStatus, Filtered }

/** How the read of what a library's filters offer stands: not answered yet, answered, or failed without ever answering. */
enum class OfferRead { Pending, Read, Failed }

/**
 * A library's grid as its screen shows it ([LibraryGrid.state]): the
 * [library], once it's read; the [tab] showing among its [tabs]; the works'
 * [sort] and [filters]; what its filters offer ([offer]), in the order to
 * show the groups; and the grid of the tab ([pager]), a new one with each
 * tab, sort or filter.
 */
@ConsistentCopyVisibility
data class LibraryGridState internal constructor(
    val libraryId: String,
    val library: Library?,
    val tab: LibraryTab,
    val sort: GridSort,
    /** The filters chosen, which the grid shows at once or once they've held still. */
    val filters: GridFilters,
    /** The groups the filters offer, each with its values; a group with none is left out, and all but the watch status until they're read. */
    val offer: List<FilterGroup>,
    val offerRead: OfferRead,
    /** Whether the library has collections; null until it's known. */
    internal val collections: Boolean?,
    val pager: GridPager,
) {
    private val kind: LibraryKind? get() = library?.kind

    /** The tabs: an audiobooks library's works, authors and series; another's works, with its collections when it has any. */
    val tabs: List<LibraryTab> = when {
        kind == LibraryKind.Audiobooks -> listOf(LibraryTab.Titles, LibraryTab.Authors, LibraryTab.BookSeries)
        collections == true || tab == LibraryTab.Collections -> listOf(LibraryTab.Titles, LibraryTab.Collections)
        else -> listOf(LibraryTab.Titles)
    }

    /** Whether [tabs] are the library's own yet: a video library's wait on whether it has collections. */
    val tabsKnown: Boolean = kind == LibraryKind.Audiobooks || collections != null || tab == LibraryTab.Collections

    /** The sorts the library offers for its works, the title's first. */
    val sorts: List<GridSort> = sortsOf(kind)

    /** The filters that are on, in the order their groups show; none on a tab that isn't the works'. */
    val activeFilters: List<GridFilter> = if (tab == LibraryTab.Titles) activeFilters(filters, offer) else emptyList()

    /** What the grid says when it has nothing. */
    val emptyCase: GridEmptyCase = when {
        activeFilters.isEmpty() -> GridEmptyCase.Unfiltered
        activeFilters.all { it.group == FilterGroupKind.WatchStatus } -> GridEmptyCase.WatchStatus
        else -> GridEmptyCase.Filtered
    }

    /** The tab's cards: authors and book series square, collections posters, and works by the library's kind. */
    val cardShape: GridCardShape = when {
        tab == LibraryTab.Authors || tab == LibraryTab.BookSeries -> GridCardShape.Square
        tab == LibraryTab.Collections -> GridCardShape.Poster
        kind == LibraryKind.Audiobooks -> GridCardShape.Square
        kind == LibraryKind.Videos -> GridCardShape.Landscape
        else -> GridCardShape.Poster
    }

    /** The query for the works chosen: the library, its sort and its filters, for Save as Smart List. */
    val smartListQuery: WorkQuery get() = SmartLists.fromGrid(filters.query(libraryId, sort), kind)
}

/**
 * A library's grid behind its screen ([SessionLibrary.libraryGrid]): the
 * tab, sort and filters this device last used in it, kept as they change,
 * and the paged grid they choose. A new tab, sort or filter shows a new
 * grid at once, its placeholders until its first page comes, and the one it
 * replaces stops reading. It follows the library and what its filters
 * offer while it's open.
 *
 * Opened sorted one way for once, it starts from the works in that sort
 * with no filters, and keeps none of what's chosen.
 *
 * Called on the main thread, as the core's work runs; [close] it when the
 * screen goes.
 */
class LibraryGrid internal constructor(
    private val libraryId: String,
    oneOffSort: GridSort?,
    library: Library?,
    libraries: Flow<Load<List<Library>>>,
    offers: Flow<Load<LibraryFilters>>,
    private val saved: SavedGrids,
    private val pagerOf: (LibraryTab, GridQuery) -> GridPager,
    scope: CoroutineScope,
) {
    private val scope = CoroutineScope(scope.coroutineContext + SupervisorJob(scope.coroutineContext[Job]))

    /** Whether what's chosen is kept for the library on the device. */
    private val keeps = oneOffSort == null

    /** What the library's works have to filter by, as last read. */
    private var values: LibraryFilters? = null

    /** The tab and the works' query the grid showing was made for. */
    private var shown: Pair<LibraryTab, GridQuery>

    /** The filters chosen in a touch sheet, on their way to the grid. */
    private var settling: Job? = null

    private val mutableState: MutableStateFlow<LibraryGridState>

    /** The grid as its screen shows it. */
    val state: StateFlow<LibraryGridState>

    init {
        val kept = saved.of(libraryId)
        val start = if (oneOffSort != null) SavedGrid(sort = oneOffSort) else kept ?: SavedGrid()
        shown = start.tab to start.filters.query(libraryId, start.sort)
        mutableState = MutableStateFlow(
            LibraryGridState(
                libraryId = libraryId,
                library = library,
                tab = start.tab,
                sort = start.sort,
                filters = start.filters,
                offer = filterOffer(library?.kind, null),
                offerRead = OfferRead.Pending,
                collections = kept?.collections,
                pager = pagerOf(shown.first, shown.second),
            ),
        )
        state = mutableState.asStateFlow()
        this.scope.launch {
            libraries.collect { load ->
                val read = load.getOrNull()?.firstOrNull { it.id == libraryId } ?: return@collect
                show { it.copy(library = read, offer = filterOffer(read.kind, values)) }
            }
        }
        this.scope.launch {
            offers.collect { load ->
                load.getOrNull()?.let { read ->
                    values = read
                    saved.update(libraryId) { it.copy(collections = read.collections) }
                    show { it.copy(offer = filterOffer(it.library?.kind, read), offerRead = OfferRead.Read, collections = read.collections) }
                }
                // A library whose read failed is shown as one without collections, until it answers.
                if (load is Load.Failed && values == null) show { it.copy(offerRead = OfferRead.Failed, collections = it.collections ?: false) }
            }
        }
    }

    /** Shows [tab], keeping the works' sort and filters. */
    fun choose(tab: LibraryTab) = grid { it.copy(tab = tab) }

    /** Shows the works in [sort]. */
    fun choose(sort: GridSort) = grid { it.copy(sort = sort) }

    /**
     * Chooses [filters] for the works: the grid shows them at once, or with
     * [settle], as a touch sheet asks, once no others have been chosen for
     * a moment. Choosing what's chosen without [settle] shows it now, as
     * when the sheet closes.
     */
    @DefaultArgumentInterop.Enabled
    fun choose(filters: GridFilters, settle: Boolean = false) {
        settling?.cancel()
        if (!settle) return grid { it.copy(filters = filters) }
        show { it.copy(filters = filters) }
        settling = scope.launch {
            delay(FILTER_SETTLE_MS)
            grid { it }
        }
    }

    /** Turns every filter off, keeping the tab and sort; [settle] as for [choose]. */
    @DefaultArgumentInterop.Enabled
    fun clearFilters(settle: Boolean = false) = choose(GridFilters.None, settle)

    /** Stops reading: the screen is gone. */
    fun close() {
        scope.cancel()
        state.value.pager.close()
    }

    private fun show(change: (LibraryGridState) -> LibraryGridState) {
        mutableState.value = change(mutableState.value)
    }

    /**
     * Shows the grid of what's chosen after [change]: a new one, in place of
     * the one showing, when the tab or the works' query differs from what
     * that one shows; and keeps what's chosen for the library.
     */
    private fun grid(change: (LibraryGridState) -> LibraryGridState) {
        val before = mutableState.value
        val chosen = change(before)
        val query = chosen.filters.query(libraryId, chosen.sort)
        val same = shown.first == chosen.tab && (chosen.tab != LibraryTab.Titles || shown.second == query)
        shown = chosen.tab to query
        mutableState.value = if (same) chosen else chosen.copy(pager = pagerOf(chosen.tab, query))
        if (!same) before.pager.close()
        if (keeps) saved.update(libraryId) { it.copy(tab = chosen.tab, sort = chosen.sort, filters = chosen.filters) }
    }

    private companion object {
        /** How long a touch sheet's choices hold still before the grid follows them. */
        const val FILTER_SETTLE_MS = 300L
    }
}
