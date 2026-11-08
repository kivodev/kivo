package com.mmcdole.kivo.android.mobile.detail

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.android.mobile.ui.WorkMarks
import com.mmcdole.kivo.api.models.BookSeries
import com.mmcdole.kivo.api.models.Collection
import com.mmcdole.kivo.api.models.ImageRef
import com.mmcdole.kivo.api.models.Person
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.session.KivoSession
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn

/** A page of works: its name and picture as known before it's read, then what's read, or why it couldn't be. */
data class PageUiState<T : Any>(val name: String? = null, val image: ImageRef? = null, val value: T? = null, val problem: RequestProblem? = null)

/**
 * A collection's, person's or book series' page: [read], read again as the
 * live events say, shown at once with the name a list read of it.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class PageViewModel<T : Any>(
    private val session: KivoSession,
    name: String?,
    image: ImageRef?,
    read: () -> Flow<Load<T>>,
) : ViewModel() {
    private val retries = MutableStateFlow(0)

    val marks = WorkMarks(session.library, viewModelScope)

    private val initial = PageUiState<T>(name, image)

    val state: StateFlow<PageUiState<T>> = retries
        .flatMapLatest { read() }
        .map { load -> initial.copy(value = load.getOrNull(), problem = load.problemOrNull()) }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), initial)

    fun retry() {
        session.retryNow()
        retries.value++
    }

    companion object {
        fun collection(session: KivoSession, id: String): PageViewModel<Collection> {
            val card = session.library.knownCollection(id)
            return PageViewModel(session, card?.name, card?.images?.poster) { session.library.collection(id) }
        }

        fun person(session: KivoSession, id: String): PageViewModel<Person> {
            val heading = session.library.knownPerson(id)
            return PageViewModel(session, heading?.name, heading?.image) { session.library.person(id) }
        }

        fun bookSeries(session: KivoSession, id: String): PageViewModel<BookSeries> {
            val heading = session.library.knownBookSeries(id)
            return PageViewModel(session, heading?.name, heading?.image) { session.library.bookSeries(id) }
        }
    }
}
