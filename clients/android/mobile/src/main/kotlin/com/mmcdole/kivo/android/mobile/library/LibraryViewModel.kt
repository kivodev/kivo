package com.mmcdole.kivo.android.mobile.library

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.android.library.GridChoice
import com.mmcdole.kivo.android.library.SavedGrids
import com.mmcdole.kivo.android.library.Section
import com.mmcdole.kivo.android.library.gridChoice
import com.mmcdole.kivo.android.library.unfiltered
import com.mmcdole.kivo.android.mobile.ui.CardShape
import com.mmcdole.kivo.android.mobile.ui.WorkMarks
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.LibraryKind
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.data.GridPager
import com.mmcdole.kivo.core.data.GridQuery
import com.mmcdole.kivo.core.data.GridSort
import com.mmcdole.kivo.core.data.LibraryFilters
import com.mmcdole.kivo.core.session.KivoSession
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.onStart
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

/** A library's tabs: its works, its collections, and an audiobooks library's authors and book series. */
enum class LibraryTab { Titles, Collections, Authors, Series }

data class LibraryUiState(
    /** The library, once the libraries are read. */
    val library: Library?,
    val tab: LibraryTab,
    /** The tabs on offer: just the works' when there's nothing beside them. */
    val tabs: List<LibraryTab>,
    /** The works' sort and filters. */
    val query: GridQuery,
    /** The grid the tab shows. */
    val grid: GridPager,
    /** What the library offers to filter by, once read. */
    val filters: LibraryFilters?,
) {
    /** What the filter sheet offers. */
    val offer: FilterOffer get() = filterOffer(library?.kind, filters)

    val shape: CardShape get() = gridShape(library?.kind, tab)
}

/** A tab's cards: authors and series square, collections posters, and works by the library's kind. */
fun gridShape(kind: LibraryKind?, tab: LibraryTab): CardShape = when {
    tab == LibraryTab.Authors || tab == LibraryTab.Series -> CardShape.Square
    tab == LibraryTab.Collections -> CardShape.Poster
    kind == LibraryKind.Audiobooks -> CardShape.Square
    kind == LibraryKind.Videos -> CardShape.Landscape
    else -> CardShape.Poster
}

/** The tabs a library of [kind] offers: an audiobooks library its authors and series, another its collections when it has any. */
fun tabsOf(kind: LibraryKind?, filters: LibraryFilters?, tab: LibraryTab): List<LibraryTab> = when {
    kind == LibraryKind.Audiobooks -> listOf(LibraryTab.Titles, LibraryTab.Authors, LibraryTab.Series)
    filters?.collections == true || tab == LibraryTab.Collections -> listOf(LibraryTab.Titles, LibraryTab.Collections)
    else -> listOf(LibraryTab.Titles)
}

/**
 * A library's grid: the tab, sort and filters this device last used in it,
 * kept as they change; with a [oneOffSort], as from Recently Added's
 * heading, the works in that sort with no filters, keeping nothing. A new
 * tab, sort or filter shows a new grid at once, its placeholders until its
 * first page comes, and the one it replaces stops reading.
 */
class LibraryViewModel(
    private val session: KivoSession,
    private val savedGrids: SavedGrids,
    saved: Map<String, GridChoice>,
    private val libraryId: String,
    private val oneOffSort: GridSort? = null,
) : ViewModel() {
    private val initial = if (oneOffSort != null) GridChoice(GridQuery(libraryId, sort = oneOffSort)) else gridChoice(saved, libraryId)

    private val choice = MutableStateFlow(initial.query to if (initial.section == Section.Collections) LibraryTab.Collections else LibraryTab.Titles)
    private val grid = MutableStateFlow(gridOf(choice.value.second, choice.value.first))

    val marks = WorkMarks(session.library, viewModelScope)

    /** The filter sheet's choices, on their way to the grid. */
    private var settling: Job? = null

    val state: StateFlow<LibraryUiState> = combine(
        choice,
        grid,
        session.library.libraries().onStart { emit(Load.Loading) },
        session.library.filters(libraryId).onStart { emit(Load.Loading) },
    ) { (query, tab), grid, libraries, filters ->
        val library = libraries.getOrNull()?.firstOrNull { it.id == libraryId }
        val read = filters.getOrNull()
        LibraryUiState(library, tab, tabsOf(library?.kind, read, tab), query, grid, read)
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), LibraryUiState(null, choice.value.second, listOf(choice.value.second), choice.value.first, grid.value, null))

    fun chooseTab(tab: LibraryTab) = choose(choice.value.first, tab)

    fun chooseQuery(query: GridQuery) {
        settling?.cancel()
        choose(query, choice.value.second)
    }

    /** Shows [query] once the filter sheet's choices have held still. */
    fun settle(query: GridQuery) {
        settling?.cancel()
        settling = viewModelScope.launch {
            delay(SETTLE_MILLIS)
            choose(query, choice.value.second)
        }
    }

    fun clearFilters() = chooseQuery(choice.value.first.unfiltered())

    /** Reads the grid again in place, as a pull to refresh does; returns once its first page answers. */
    suspend fun refresh() {
        try {
            grid.value.refresh()
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            // The grid shows why.
        }
    }

    fun retry() {
        viewModelScope.launch { refresh() }
    }

    private fun choose(query: GridQuery, tab: LibraryTab) {
        if (query to tab == choice.value) return
        choice.value = query to tab
        val old = grid.value
        grid.value = gridOf(tab, query)
        old.close()
        // The works' and collections' choice is kept; a one-off sort keeps nothing.
        if (oneOffSort == null && (tab == LibraryTab.Titles || tab == LibraryTab.Collections)) {
            val kept = GridChoice(query, if (tab == LibraryTab.Collections) Section.Collections else Section.Titles)
            viewModelScope.launch {
                try {
                    savedGrids.save(kept)
                } catch (e: CancellationException) {
                    throw e
                } catch (_: Exception) {
                    // The grid still shows; it just isn't kept.
                }
            }
        }
    }

    private fun gridOf(tab: LibraryTab, query: GridQuery): GridPager = when (tab) {
        LibraryTab.Titles -> session.library.pagedWorks(query, GridSettleMs)
        LibraryTab.Collections -> session.library.pagedCollections(libraryId, GridSettleMs)
        LibraryTab.Authors -> session.library.pagedAuthors(libraryId, GridSettleMs)
        LibraryTab.Series -> session.library.pagedBookSeries(libraryId, GridSettleMs)
    }

    override fun onCleared() {
        grid.value.close()
    }

    private companion object {
        /** How long the filter sheet's choices hold still before the grid follows them. */
        const val SETTLE_MILLIS = 300L
    }
}
