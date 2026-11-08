package com.mmcdole.kivo.android.mobile.favorites

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.android.mobile.ui.WorkMarks
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.data.GridPager
import com.mmcdole.kivo.core.session.KivoSession
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.onEach
import kotlinx.coroutines.flow.stateIn

/** The kinds Favorites lists, in order. */
val favoriteKinds = listOf("movie", "series", "episode", "book", "season", "video", "extra")

/** A kind's favorites: the first of them, and how many there are. */
data class FavoriteRow(val kind: String, val items: List<WorkSummary>, val total: Int)

data class FavoritesUiState(
    /** A row for each kind with favorites; null until every kind is read. */
    val rows: List<FavoriteRow>? = null,
    val problem: RequestProblem? = null,
    val refreshing: Boolean = false,
    val serverName: String = "",
)

/** Favorites: a row of each kind the user marked, read again as their marks change, and on a pull to refresh. */
@OptIn(ExperimentalCoroutinesApi::class)
class FavoritesViewModel(private val session: KivoSession) : ViewModel() {
    private val refreshes = MutableStateFlow(0)
    private val refreshing = MutableStateFlow(false)

    val marks = WorkMarks(session.library, viewModelScope)

    private val rows = refreshes.flatMapLatest {
        combine(favoriteKinds.map { kind -> session.library.favorites(kind, ROW_LIMIT) }) { loads -> loads.toList() }
    }.onEach { refreshing.value = false }

    val state: StateFlow<FavoritesUiState> = combine(rows, refreshing) { loads, refreshing ->
        val problem = loads.firstNotNullOfOrNull { it.problemOrNull() }
        val pages = loads.map { it.getOrNull() }
        FavoritesUiState(
            rows = if (pages.all { it != null }) favoriteKinds.zip(pages) { kind, page -> FavoriteRow(kind, page!!.items, page.total) }.filter { it.items.isNotEmpty() } else null,
            problem = problem,
            refreshing = refreshing,
            serverName = session.savedServer.value.name,
        )
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), FavoritesUiState(serverName = session.savedServer.value.name))

    fun refresh() {
        refreshing.value = true
        session.retryNow()
        refreshes.value++
    }

    private companion object {
        const val ROW_LIMIT = 20
    }
}

/** One kind's favorites as a grid, most recently marked first. */
class FavoritesGridViewModel(session: KivoSession, kind: String) : ViewModel() {
    /** The grid, which the paged grid reads and asks pages of as they come into view. */
    val pager: GridPager<WorkSummary> = session.library.pagedFavorites(kind)

    val marks = WorkMarks(session.library, viewModelScope)

    suspend fun refresh() {
        try {
            pager.refresh()
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            // The grid shows why.
        }
    }

    override fun onCleared() {
        pager.close()
    }
}
