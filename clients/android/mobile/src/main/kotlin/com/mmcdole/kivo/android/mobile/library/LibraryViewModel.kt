package com.mmcdole.kivo.android.mobile.library

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.android.mobile.ui.CardShape
import com.mmcdole.kivo.core.data.GridSort
import com.mmcdole.kivo.core.session.GridCardShape
import com.mmcdole.kivo.core.session.KivoSession
import com.mmcdole.kivo.core.session.LibraryGrid
import com.mmcdole.kivo.core.session.LibraryGridState
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch

/** The grid's cards as the phone draws them. */
val GridCardShape.card: CardShape
    get() = when (this) {
        GridCardShape.Poster -> CardShape.Poster
        GridCardShape.Landscape -> CardShape.Landscape
        GridCardShape.Square -> CardShape.Square
    }

/**
 * A library's screen: the core's [grid], which holds the tab, sort and
 * filters and what they show, for as long as the screen lasts; with a
 * [oneOffSort], as from Recently Added's heading, the works in that sort
 * for once.
 */
class LibraryViewModel(session: KivoSession, libraryId: String, oneOffSort: GridSort? = null) : ViewModel() {
    val grid: LibraryGrid = session.library.libraryGrid(libraryId, oneOffSort, GridSettleMs)

    val state: StateFlow<LibraryGridState> get() = grid.state

    val marks = session.library.marks

    /** Reads the grid again in place, as a pull to refresh does; returns once its first page answers. */
    suspend fun refresh() {
        try {
            state.value.pager.refresh()
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            // The grid shows why.
        }
    }

    fun retry() {
        viewModelScope.launch { refresh() }
    }

    override fun onCleared() {
        grid.close()
    }
}
