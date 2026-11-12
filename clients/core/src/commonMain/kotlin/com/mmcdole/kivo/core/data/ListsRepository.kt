package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.check
import com.mmcdole.kivo.api.models.ListCreate
import com.mmcdole.kivo.api.models.ListKind
import com.mmcdole.kivo.api.models.ListMove
import com.mmcdole.kivo.api.models.ListSort
import com.mmcdole.kivo.api.models.ListUpdate
import com.mmcdole.kivo.api.models.ListWorkAdd
import com.mmcdole.kivo.api.models.SmartQuery
import com.mmcdole.kivo.api.models.WorkField
import com.mmcdole.kivo.api.models.WorkFieldValue
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkQuery
import com.mmcdole.kivo.api.models.WorkQueryRequest
import com.mmcdole.kivo.api.models.WorkList
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.api.models.WorkSummaryPage
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.live.Invalidation
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.flow.map

/** The orders a manual list is shown in. */
object ListSorts {
    /** A manual list's sort, [ListSort.Custom] for one this app doesn't know; null for a smart list, which has none. */
    fun of(list: WorkList): ListSort? = list.sort?.let { if (it == ListSort.Unknown) ListSort.Custom else it }
}

/** Whether the Lists page gives the list a shelf: a kind this app knows, and the built-in list only with something in it. */
val WorkList.hasShelf: Boolean get() = kind != ListKind.Unknown && (!builtIn || workCount > 0)

/** The user's lists and the works in them, a manual list's in its sort; the live events tell every screen, this app's too. */
internal class ListsRepository(private val connection: ServerConnection, private val invalidations: Flow<Invalidation>) {
    /** The user's lists, My List first; [counted] runs a smart list's rules for its count, 0 without it, and gives each list its first works. */
    fun lists(counted: Boolean): Flow<List<WorkList>> = refreshing(invalidations, ::mayChange) {
        connection.works.listLists(counts = counted.takeIf { it }).result().items
    }

    private fun mayChange(invalidation: Invalidation) =
        invalidation !is Invalidation.PlayEnded && invalidation != Invalidation.DownloadsChanged && invalidation != Invalidation.HomeChanged

    /** [limit] of a list's works of [kinds] from [offset], or the server's page, in [sort] or else the list's own; the first page carries the total. */
    suspend fun page(listId: String, offset: Int, limit: Int? = null, kinds: List<WorkKind>? = null, sort: ListSort? = null): WorkSummaryPage =
        connection.works.listListWorks(listId, kind = kinds, sort = sort, offset = offset.takeIf { it > 0 }, limit = limit).result()

    /** Every work of a manual list in its custom order, whatever its sort: what a screen that moves them shows. */
    suspend fun inCustomOrder(listId: String): List<WorkSummary> = buildList {
        do {
            val page = page(listId, size, CUSTOM_ORDER_PAGE, sort = ListSort.Custom)
            addAll(page.items)
        } while (page.items.size == CUSTOM_ORDER_PAGE)
    }

    /** Emits when a list's works may have changed: reload the loaded pages, keeping the place. */
    fun changes(): Flow<Unit> = invalidations.filter(::mayChange).map { }

    suspend fun create(name: String, query: WorkQuery? = null): WorkList =
        connection.works.createList(ListCreate(name, query?.let(::SmartQuery))).result()

    suspend fun update(id: String, name: String?, query: WorkQuery? = null): WorkList =
        connection.works.updateList(id, ListUpdate(name, query?.let(::SmartQuery))).result()

    suspend fun setSort(id: String, sort: ListSort): WorkList = connection.works.updateList(id, ListUpdate(sort = sort)).result()

    /** Puts [workIds], in that order, just after [after] in a manual list's custom order, or first without it; the list is then shown in that order. */
    suspend fun move(listId: String, workIds: List<String>, after: String?) =
        connection.works.moveListWorks(listId, ListMove(workIds, after)).check()

    /** How many works [query] selects: the first page's total, read one summary long. */
    suspend fun countMatching(query: WorkQuery): Int =
        connection.works.queryWorks(WorkQueryRequest(query, limit = 1)).result().total ?: 0

    suspend fun fields(): List<WorkField> = connection.works.listWorkFields().result().items

    suspend fun values(field: String, search: String?, ids: List<String>?): List<WorkFieldValue> =
        connection.works.listWorkFieldValues(field, search, ids = ids).result().items

    /** The names of [query]'s chosen values, by [SmartLists.nameKey]; a field whose values can't be read is left out. */
    suspend fun names(query: WorkQuery, fields: List<WorkField>): Map<String, String> = buildMap {
        for ((field, values) in SmartLists.valuesToName(query, fields)) {
            try {
                for (found in values(field, null, values.toList())) put(SmartLists.nameKey(field, found.value), found.name)
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                // The values show as saved.
            }
        }
    }

    suspend fun delete(id: String) = connection.works.deleteList(id).check()

    suspend fun add(listId: String, workId: String) = connection.works.addListWork(listId, ListWorkAdd(workId)).check()

    suspend fun remove(listId: String, workId: String) = connection.works.removeListWork(listId, workId).check()
}

/** The most works the server gives in a page. */
private const val CUSTOM_ORDER_PAGE = 200
