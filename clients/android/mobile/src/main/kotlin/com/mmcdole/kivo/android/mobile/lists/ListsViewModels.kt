package com.mmcdole.kivo.android.mobile.lists

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.android.mobile.ui.WorkMarks
import com.mmcdole.kivo.api.models.WorkList
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.data.GridPager
import com.mmcdole.kivo.core.session.KivoSession
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.onEach
import kotlinx.coroutines.flow.onStart
import kotlinx.coroutines.flow.shareIn
import kotlinx.coroutines.flow.stateIn

/** The list kind the phone shows; others are skipped. */
private const val MANUAL = "manual"

/** How many of My List's works the Lists tab shows as its covers. */
internal const val MY_LIST_COVERS = 4

data class ListsUiState(
    /** The user's lists, My List first; null until read. */
    val lists: List<WorkList>? = null,
    /** My List's most recently added works. */
    val covers: List<WorkSummary> = emptyList(),
    /** Why the lists couldn't be read, while there's nothing to show. */
    val problem: RequestProblem? = null,
    /** A pull to refresh waiting on its answer. */
    val refreshing: Boolean = false,
    val serverName: String = "",
) {
    /** My List, when it has anything in it. */
    val myList: WorkList? get() = lists?.firstOrNull { it.builtIn }?.takeIf { it.workCount > 0 }

    /** The lists the user made. */
    val manual: List<WorkList> get() = lists.orEmpty().filter { !it.builtIn && it.kind == MANUAL }
}

/** The Lists tab: the lists, and My List's covers, read again as they change and on a pull to refresh. */
@OptIn(ExperimentalCoroutinesApi::class)
class ListsViewModel(private val session: KivoSession) : ViewModel() {
    private val refreshes = MutableStateFlow(0)
    private val refreshing = MutableStateFlow(false)

    private val lists = refreshes.flatMapLatest { session.library.lists() }
        .onEach { if (it !is Load.Loading) refreshing.value = false }
        .shareIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), replay = 1)

    private val covers = lists
        .map { load -> load.getOrNull()?.firstOrNull { it.builtIn }?.id }
        .distinctUntilChanged()
        .flatMapLatest { id -> if (id == null) flowOf(null) else session.library.listWorks(id, MY_LIST_COVERS) }

    val state: StateFlow<ListsUiState> = combine(lists, covers.onStart { emit(null) }, refreshing) { lists, covers, refreshing ->
        ListsUiState(
            lists = lists.getOrNull(),
            covers = covers?.getOrNull()?.items.orEmpty(),
            problem = lists.problemOrNull(),
            refreshing = refreshing,
            serverName = session.savedServer.value.name,
        )
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), ListsUiState(serverName = session.savedServer.value.name))

    fun refresh() {
        refreshing.value = true
        session.retryNow()
        refreshes.value++
    }
}

/** An open list: its works as a grid, most recently added first; a list the user made can be renamed and deleted. */
class ListViewModel(private val session: KivoSession, private val listId: String) : ViewModel() {
    /** The grid, which the paged grid reads and asks pages of as they come into view. */
    val pager: GridPager<WorkSummary> = session.library.pagedListWorks(listId)

    val marks = WorkMarks(session.library, viewModelScope)

    /** The list as the lists last read it; null until then, or when it's gone. */
    val list: StateFlow<WorkList?> = session.library.lists()
        .map { load -> load.getOrNull()?.firstOrNull { it.id == listId } }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), null)

    suspend fun refresh() {
        try {
            pager.refresh()
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            // The grid shows why.
        }
    }

    /** Renames the list; why not, or null once it's done. */
    suspend fun rename(name: String): RequestProblem? = attempt { session.library.renameList(listId, name) }

    /** Deletes the list; why not, or null once it's done. */
    suspend fun delete(): RequestProblem? = attempt { session.library.deleteList(listId) }

    override fun onCleared() {
        pager.close()
    }
}

/** One of the chooser's rows: a list, and whether the work is on it. */
data class ListChoice(val list: WorkList, val holding: Boolean)

/** Add to List: every list the work can go on, ticked when it's on it, adding and removing as they're tapped. */
class AddToListViewModel(private val session: KivoSession, private val workId: String) : ViewModel() {
    private val marks = WorkMarks(session.library, viewModelScope)

    /** The lists, My List first, with the work's marks; null until both are read. */
    val choices: StateFlow<List<ListChoice>?> = combine(session.library.lists(), session.library.work(workId)) { lists, work ->
        val held = work.getOrNull()?.listIds.orEmpty()
        lists.getOrNull()?.filter { it.kind == MANUAL }?.map { ListChoice(it, it.id in held) }
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), null)

    fun toggle(choice: ListChoice) = marks.setOnList(choice.list.id, workId, !choice.holding)

    /** Makes a list with the work on it; why not, or null once it's done. */
    suspend fun create(name: String): RequestProblem? = attempt {
        session.library.addToList(session.library.createList(name).id, workId)
    }
}

/** Runs [call]; the problem when it fails, else null. */
private suspend fun attempt(call: suspend () -> Unit): RequestProblem? = try {
    call()
    null
} catch (e: CancellationException) {
    throw e
} catch (e: Exception) {
    RequestProblem.of(e)
}
