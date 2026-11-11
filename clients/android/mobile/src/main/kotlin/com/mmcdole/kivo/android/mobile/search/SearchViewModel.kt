package com.mmcdole.kivo.android.mobile.search

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.api.models.SearchResults
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.session.KivoSession
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.FlowPreview
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.debounce
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.scan
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.transformLatest

/** A search's answer: the query it's for, and its results or why there are none. */
data class SearchAnswer(val query: String, val results: SearchResults? = null, val problem: RequestProblem? = null)

data class SearchUiState(
    val query: String = "",
    /** The last answer, kept while the next search runs. */
    val answer: SearchAnswer? = null,
    /** A search under way. */
    val searching: Boolean = false,
)

/**
 * Search across the user's libraries: works, people and collections, a
 * moment after typing stops, the last results staying while the next search runs.
 */
@OptIn(FlowPreview::class, ExperimentalCoroutinesApi::class)
class SearchViewModel(private val session: KivoSession, initial: String = "") : ViewModel() {
    private val query = MutableStateFlow(initial)
    private val retries = MutableStateFlow(0)

    val marks = session.library.marks

    /** The server's name, for a search that can't reach it. */
    val serverName: String = session.savedServer.value.name

    /** Each search's start (null answer) and its answer, for the trimmed query once typing settles. */
    private val searches: Flow<Pair<String, SearchAnswer?>> = combine(query.debounce(SEARCH_DELAY_MS).map { it.trim() }, retries) { q, retry -> q to retry }
        .distinctUntilChanged()
        .transformLatest { (q, _) ->
            emit(q to null)
            if (q.isNotEmpty()) emit(q to search(q))
        }

    private val answers: Flow<Pair<SearchAnswer?, Boolean>> = searches.scan(null as SearchAnswer? to false) { (last, _), (q, answer) ->
        when {
            q.isEmpty() -> null to false
            answer == null -> last to true
            else -> answer to false
        }
    }

    val state: StateFlow<SearchUiState> = combine(query, answers) { q, (answer, searching) ->
        SearchUiState(q, answer, searching)
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), SearchUiState(initial))

    fun setQuery(text: String) {
        query.value = text
    }

    /** Searches again, after a search failed. */
    fun retry() {
        retries.value++
    }

    private suspend fun search(q: String): SearchAnswer = try {
        SearchAnswer(q, results = session.library.search(q, null))
    } catch (e: CancellationException) {
        throw e
    } catch (e: Exception) {
        SearchAnswer(q, problem = RequestProblem.of(e))
    }

    private companion object {
        const val SEARCH_DELAY_MS = 350L
    }
}
