package com.mmcdole.kivo.android.mobile.library

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.session.KivoSession
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn

data class LibrariesUiState(
    /** The user's pinned libraries, in their order; null until read. */
    val pinned: List<Library>? = null,
    /** The rest. */
    val more: List<Library> = emptyList(),
    val problem: RequestProblem? = null,
    val serverName: String = "",
)

/** The user's libraries: the pinned ones in their order, then the rest, read again as they change. */
class LibrariesViewModel(private val session: KivoSession) : ViewModel() {
    val state: StateFlow<LibrariesUiState> = combine(session.library.libraries(), session.savedServer) { load, server ->
        val libraries = load.getOrNull()
        LibrariesUiState(
            pinned = libraries?.filter { it.pinned },
            more = libraries?.filterNot { it.pinned }.orEmpty(),
            problem = load.problemOrNull(),
            serverName = server.name,
        )
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), LibrariesUiState(serverName = session.savedServer.value.name))

    /** Asks the server again now, rather than at the next retry. */
    fun retry() = session.retryNow()
}
