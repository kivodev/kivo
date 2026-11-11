package com.mmcdole.kivo.android.mobile.lists

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.android.mobile.ui.WorkMarks
import com.mmcdole.kivo.api.models.ListKind
import com.mmcdole.kivo.api.models.ListSort
import com.mmcdole.kivo.api.models.WorkField
import com.mmcdole.kivo.api.models.WorkList
import com.mmcdole.kivo.api.models.WorkQuery
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.data.GridPager
import com.mmcdole.kivo.core.data.ListSorts
import com.mmcdole.kivo.core.session.KivoSession
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.mapLatest
import kotlinx.coroutines.flow.onEach
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

data class ListsUiState(
    /** The user's lists, My List first, each with its first works; null until read. */
    val lists: List<WorkList>? = null,
    /** Why the lists couldn't be read, while there's nothing to show. */
    val problem: RequestProblem? = null,
    /** A pull to refresh waiting on its answer. */
    val refreshing: Boolean = false,
    val serverName: String = "",
) {
    /** The lists the tab shelves, in their order: My List only with something in it, and a list kind it doesn't know left out. */
    val shelves: List<WorkList> get() = lists.orEmpty().filter { (it.kind == ListKind.Manual || it.kind == ListKind.Smart) && (!it.builtIn || it.workCount > 0) }
}

/** Whether works can be added to and taken off the list. */
val WorkList.isManual: Boolean get() = kind == ListKind.Manual

/** The Lists tab: the lists with their first works, read again as they change and on a pull to refresh. */
@OptIn(ExperimentalCoroutinesApi::class)
class ListsViewModel(private val session: KivoSession) : ViewModel() {
    val marks = WorkMarks(session.library, viewModelScope)
    private val refreshes = MutableStateFlow(0)
    private val refreshing = MutableStateFlow(false)

    private val lists = refreshes.flatMapLatest { session.library.lists(counted = true) }
        .onEach { if (it !is Load.Loading) refreshing.value = false }

    val state: StateFlow<ListsUiState> = combine(lists, refreshing) { lists, refreshing ->
        ListsUiState(
            lists = lists.getOrNull(),
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

/** Reorder: a manual list's works in its custom order, as moved here; null while they're read. */
data class ReorderUiState(val works: List<WorkSummary>? = null)

/**
 * An open list: its works as a grid, a manual list's in its sort, which can
 * be chosen, and in Reorder as rows to move; a list the user made can be
 * renamed and deleted, and a smart list's rules are read to name them.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class ListViewModel(private val session: KivoSession, private val listId: String) : ViewModel() {
    /** The grid, which the paged grid reads and asks pages of as they come into view. */
    val pager: GridPager<WorkSummary> = session.library.pagedListWorks(listId, null)

    val marks = WorkMarks(session.library, viewModelScope)

    private val mutableProblem = MutableStateFlow<RequestProblem?>(null)

    /** Why the list's rules couldn't be named, or a sort or move was refused, until it's dismissed. */
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

    /** The sort just chosen here, until the list is read again. */
    private val chosenSort = MutableStateFlow<ListSort?>(null)

    /** The order a manual list is shown in; null for a smart list or one not read. */
    val sort: StateFlow<ListSort?> = combine(list.onEach { chosenSort.value = null }, chosenSort) { list, chosen -> list?.let(ListSorts::of)?.let { chosen ?: it } }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), null)

    /** Shows the list in [sort] at once and saves it; a refusal goes back and says why. */
    fun chooseSort(sort: ListSort) {
        chosenSort.value = sort
        viewModelScope.launch {
            val failure = attempt { session.library.setListSort(listId, sort) }
            if (failure != null) {
                chosenSort.value = null
                mutableProblem.value = failure
            }
        }
    }

    private val mutableReorder = MutableStateFlow<ReorderUiState?>(null)

    /** Reorder's rows while it's open, else null. */
    val reorder: StateFlow<ReorderUiState?> = mutableReorder

    /** The moves are sent one at a time, in the order they were made. */
    private val sending = Mutex()

    /** Opens Reorder on the list's custom order; a failed read closes it and says why. */
    fun startReorder() {
        mutableReorder.value = ReorderUiState()
        viewModelScope.launch { readCustomOrder(closing = true) }
    }

    private suspend fun readCustomOrder(closing: Boolean) {
        try {
            val works = session.library.listWorksInCustomOrder(listId)
            mutableReorder.update { it?.copy(works = works) }
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            if (closing) {
                mutableReorder.value = null
                mutableProblem.value = RequestProblem.of(e)
            }
        }
    }

    fun endReorder() {
        mutableReorder.value = null
    }

    /** Puts the row at [from] at [to], here alone: a drag passes over rows before it's let go. */
    fun moveRow(from: Int, to: Int) {
        mutableReorder.update { state ->
            val works = state?.works ?: return@update state
            if (from !in works.indices || to !in works.indices || from == to) return@update state
            state.copy(works = works.toMutableList().apply { add(to, removeAt(from)) })
        }
    }

    /** Saves where [workId] now is: after the row above it, or first. A refusal reads the order again and says why. */
    fun saveMove(workId: String) {
        val works = mutableReorder.value?.works ?: return
        val index = works.indexOfFirst { it.id == workId }
        if (index < 0) return
        val after = works.getOrNull(index - 1)?.id
        viewModelScope.launch {
            val failure = sending.withLock { attempt { session.library.moveInList(listId, listOf(workId), after) } }
            if (failure != null) {
                mutableProblem.value = failure
                readCustomOrder(closing = false)
            }
        }
    }

    /** Moves the row at [index] one place up or down, [by] -1 or 1, and saves it. */
    fun step(index: Int, by: Int) {
        val works = mutableReorder.value?.works ?: return
        val work = works.getOrNull(index) ?: return
        if (index + by !in works.indices) return
        moveRow(index, index + by)
        saveMove(work.id)
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
        if (held == null) null else lists.getOrNull()?.filter { it.kind == ListKind.Manual }?.map { ListChoice(it, it.id in held) }
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
