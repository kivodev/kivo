package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.check
import com.mmcdole.kivo.api.models.ListCreate
import com.mmcdole.kivo.api.models.ListUpdate
import com.mmcdole.kivo.api.models.ListWorkAdd
import com.mmcdole.kivo.api.models.SmartQuery
import com.mmcdole.kivo.api.models.WorkField
import com.mmcdole.kivo.api.models.WorkFieldValue
import com.mmcdole.kivo.api.models.WorkQuery
import com.mmcdole.kivo.api.models.WorkQueryRequest
import com.mmcdole.kivo.api.models.WorkList
import com.mmcdole.kivo.api.models.WorkSummaryPage
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.live.Invalidation
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.flow.map

/** The user's lists and the works in them, most recently added first; the live events tell every screen, this app's too. */
internal class ListsRepository(private val connection: ServerConnection, private val invalidations: Flow<Invalidation>) {
    /** The user's lists, My List first; a smart list's count is 0 unless [counted], which runs its rules. */
    fun lists(counted: Boolean): Flow<List<WorkList>> = refreshing(invalidations, ::mayChange) {
        connection.works.listLists(counts = counted.takeIf { it }).result().items
    }

    /** The first page of a list's works of [kinds], or every kind when null, [limit] long or the server's page. */
    fun works(listId: String, limit: Int? = null, kinds: List<String>? = null): Flow<WorkSummaryPage> = refreshing(invalidations, ::mayChange) {
        connection.works.listListWorks(listId, kind = kinds, limit = limit).result()
    }

    private fun mayChange(invalidation: Invalidation) =
        invalidation !is Invalidation.PlayEnded && invalidation != Invalidation.DownloadsChanged && invalidation != Invalidation.HomeChanged

    /** [limit] of a list's works of [kinds] from [offset], or the server's page; the first page carries the total. */
    suspend fun page(listId: String, offset: Int, limit: Int? = null, kinds: List<String>? = null): WorkSummaryPage =
        connection.works.listListWorks(listId, kind = kinds, offset = offset.takeIf { it > 0 }, limit = limit).result()

    /** Emits when a list's works may have changed: reload the loaded pages, keeping the place. */
    fun changes(): Flow<Unit> = invalidations.filter(::mayChange).map { }

    suspend fun create(name: String, query: WorkQuery? = null): WorkList =
        connection.works.createList(ListCreate(name, query?.let(::SmartQuery))).result()

    suspend fun update(id: String, name: String?, query: WorkQuery? = null): WorkList =
        connection.works.updateList(id, ListUpdate(name, query?.let(::SmartQuery))).result()

    /** How many works [query] selects: the first page's total, read one summary long. */
    suspend fun countMatching(query: WorkQuery): Int =
        connection.works.queryWorks(WorkQueryRequest(query, limit = 1)).result().total

    suspend fun fields(): List<WorkField> = connection.works.listWorkFields().result().items

    suspend fun values(field: String, search: String?, ids: List<String>?): List<WorkFieldValue> =
        connection.works.listWorkFieldValues(field, search, ids).result().items

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
