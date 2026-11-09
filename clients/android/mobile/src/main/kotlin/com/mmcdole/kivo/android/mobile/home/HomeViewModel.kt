package com.mmcdole.kivo.android.mobile.home

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.android.mobile.lists.isManual
import com.mmcdole.kivo.android.mobile.ui.CONTINUE_LISTENING
import com.mmcdole.kivo.android.mobile.ui.CONTINUE_WATCHING
import com.mmcdole.kivo.android.mobile.ui.WorkAction
import com.mmcdole.kivo.android.mobile.ui.WorkMarks
import com.mmcdole.kivo.api.models.HomeRow
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.ListRef
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.session.KivoSession
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.onEach
import kotlinx.coroutines.flow.stateIn

/** A row as Home shows it: its kind and items, and for Recently Added the library its heading opens. */
data class HomeRowUi(val id: String, val kind: String, val items: List<WorkSummary>, val library: Library?, val libraryName: String?, val list: ListRef?)

data class HomeUiState(
    val serverName: String,
    /** The signed-in user's initial, for the avatar. */
    val initial: String = "",
    /** Home's rows, in the server's order; null until read. */
    val rows: List<HomeRowUi>? = null,
    /** There are no rows because the user hid every one. */
    val allHidden: Boolean = false,
    /** Why Home couldn't be read, while there's nothing to show. */
    val problem: RequestProblem? = null,
    /** A pull to refresh waiting on its answer. */
    val refreshing: Boolean = false,
)

/** The row kinds the phone shows. */
internal val shownKinds = setOf(CONTINUE_WATCHING, CONTINUE_LISTENING, "recentlyAdded", "list")

/** The rows Home shows of [rows]: the kinds it knows, with items, each with its library among [libraries]. */
fun shownRows(rows: List<HomeRow>, libraries: List<Library>): List<HomeRowUi> {
    val byId = libraries.associateBy { it.id }
    return rows.filter { it.kind in shownKinds && it.items.isNotEmpty() }.map { row ->
        HomeRowUi(row.id, row.kind, row.items, row.library?.id?.let(byId::get), row.library?.name, row.list)
    }
}

/**
 * Home: the server's rows as the core reads them, read again as the live
 * events say and on a pull to refresh, with the server's name and the
 * user's initial for the top bar.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class HomeViewModel(private val session: KivoSession) : ViewModel() {
    private val refreshes = MutableStateFlow(0)
    private val refreshing = MutableStateFlow(false)

    val marks = WorkMarks(session.library, viewModelScope)

    /** The lists a card can be taken off from Home: a smart list's works are its rules'. */
    val manualListIds: StateFlow<Set<String>> = session.library.lists()
        .map { load -> load.getOrNull().orEmpty().filter { it.isManual }.map { it.id }.toSet() }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), emptySet())

    // A new collection after a pull answers in place: the core emits no Loading before it.
    private val home = refreshes.flatMapLatest { session.library.home() }.onEach { if (it !is Load.Loading) refreshing.value = false }

    val state: StateFlow<HomeUiState> = combine(
        home,
        session.library.libraries(),
        session.savedServer,
        session.user,
        refreshing,
    ) { home, libraries, server, user, refreshing ->
        HomeUiState(
            serverName = server.name,
            initial = user?.initial.orEmpty(),
            rows = home.getOrNull()?.let { shownRows(it.rows, libraries.getOrNull().orEmpty()) },
            allHidden = home.getOrNull()?.allHidden == true,
            problem = home.problemOrNull(),
            refreshing = refreshing,
        )
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), HomeUiState(session.savedServer.value.name))

    fun refresh() {
        refreshing.value = true
        session.retryNow()
        refreshes.value++
    }

    fun apply(action: WorkAction, work: WorkSummary) = marks.apply(action, work)
}
