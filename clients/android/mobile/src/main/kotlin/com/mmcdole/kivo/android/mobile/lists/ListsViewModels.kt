package com.mmcdole.kivo.android.mobile.lists

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.android.mobile.ui.WorkMarks
import com.mmcdole.kivo.api.models.WorkField
import com.mmcdole.kivo.api.models.WorkList
import com.mmcdole.kivo.api.models.WorkQuery
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
import kotlinx.coroutines.flow.mapLatest
import kotlinx.coroutines.flow.onEach
import kotlinx.coroutines.flow.onStart
import kotlinx.coroutines.flow.shareIn
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** The list kinds the phone shows; others are skipped. */
private const val MANUAL = "manual"
private const val SMART = "smart"

/** How many of My List's works the Lists tab shows as its covers. */
internal const val MY_LIST_COVERS = 4

/** How many of My List's works a wide Lists tab's row holds, as many as Home's. */
private const val MY_LIST_ROW = 20

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

    val smart: List<WorkList> get() = lists.orEmpty().filter { it.kind == SMART }
}

/** Whether works can be added to and taken off the list. */
val WorkList.isManual: Boolean get() = kind == MANUAL

/** The Lists tab: the lists, and My List's covers, read again as they change and on a pull to refresh. */
@OptIn(ExperimentalCoroutinesApi::class)
class ListsViewModel(private val session: KivoSession) : ViewModel() {
    val marks = WorkMarks(session.library, viewModelScope)
    private val refreshes = MutableStateFlow(0)
    private val refreshing = MutableStateFlow(false)

    private val lists = refreshes.flatMapLatest { session.library.lists(counted = true) }
        .onEach { if (it !is Load.Loading) refreshing.value = false }
        .shareIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), replay = 1)

    private val covers = lists
        .map { load -> load.getOrNull()?.firstOrNull { it.builtIn }?.id }
        .distinctUntilChanged()
        .flatMapLatest { id -> if (id == null) flowOf(null) else session.library.listWorks(id, MY_LIST_ROW, null) }

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

    /** Makes a list; the problem when it's refused. */
    suspend fun create(name: String): RequestProblem? = attempt { session.library.createList(name) }
}

/** What naming a smart list's rules takes: the catalog and its chosen values' names. */
data class SmartRulesWords(val fields: List<WorkField>, val names: Map<String, String>)

/** An open list: its works as a grid, most recently added first; a list the user made can be renamed and deleted, and a smart list's rules are read to name them. */
@OptIn(ExperimentalCoroutinesApi::class)
class ListViewModel(private val session: KivoSession, private val listId: String) : ViewModel() {
    /** The grid, which the paged grid reads and asks pages of as they come into view. */
    val pager: GridPager<WorkSummary> = session.library.pagedListWorks(listId, null)

    val marks = WorkMarks(session.library, viewModelScope)

    private val mutableProblem = MutableStateFlow<RequestProblem?>(null)

    /** Why the list's rules couldn't be named, until it's dismissed. */
    val problem: StateFlow<RequestProblem?> = mutableProblem

    /** The list as the lists last read it; null until then, or when it's gone. */
    val list: StateFlow<WorkList?> = session.library.lists()
        .map { load -> load.getOrNull()?.firstOrNull { it.id == listId } }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), null)

    val rulesWords: StateFlow<SmartRulesWords?> = list
        .map { it?.smart?.query }
        .distinctUntilChanged()
        .mapLatest { query -> query?.let { readRulesWords(it) } }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), null)

    private suspend fun readRulesWords(query: WorkQuery): SmartRulesWords? = try {
        val fields = session.library.workFields()
        SmartRulesWords(fields, session.library.smartListNames(query, fields))
    } catch (e: CancellationException) {
        throw e
    } catch (e: Exception) {
        mutableProblem.value = RequestProblem.of(e)
        null
    }

    fun dismissProblem() {
        mutableProblem.value = null
    }

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
    suspend fun rename(name: String): RequestProblem? = attempt { session.library.updateList(listId, name, null) }

    /** Deletes the list; why not, or null once it's done. */
    suspend fun delete(): RequestProblem? = attempt { session.library.deleteList(listId) }

    override fun onCleared() {
        pager.close()
    }
}

/** One of the chooser's rows: a list, and whether the work is on it. */
data class ListChoice(val list: WorkList, val holding: Boolean)

/** Add to List: every list the work can go on, ticked when it's on it, a tap adding or removing it at once; a write that fails puts the row back and says why. */
class AddToListViewModel(private val session: KivoSession, private val workId: String) : ViewModel() {
    /** The lists holding the work, as read and as tapped since; null until read. */
    private val held = MutableStateFlow<Set<String>?>(null)

    private val mutableProblem = MutableStateFlow<RequestProblem?>(null)

    /** Why the last tap was refused, until it's dismissed. */
    val problem: StateFlow<RequestProblem?> = mutableProblem

    private val work = session.library.work(workId).onEach { load ->
        load.getOrNull()?.let { read -> held.value = read.listIds.orEmpty().toSet() }
    }

    /** The lists, My List first, with the work's marks; null until both are read. */
    val choices: StateFlow<List<ListChoice>?> = combine(session.library.lists(), work, held) { lists, _, held ->
        if (held == null) null else lists.getOrNull()?.filter { it.kind == MANUAL }?.map { ListChoice(it, it.id in held) }
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), null)

    fun toggle(choice: ListChoice) {
        val id = choice.list.id
        val adding = !choice.holding
        held.update { if (adding) it.orEmpty() + id else it.orEmpty() - id }
        viewModelScope.launch {
            val failure = attempt { if (adding) session.library.addToList(id, workId) else session.library.removeFromList(id, workId) }
            if (failure != null) {
                held.update { if (adding) it.orEmpty() - id else it.orEmpty() + id }
                mutableProblem.value = failure
            }
        }
    }

    fun dismissProblem() {
        mutableProblem.value = null
    }

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
