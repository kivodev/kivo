package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.check
import com.mmcdole.kivo.api.models.ListCreate
import com.mmcdole.kivo.api.models.ListUpdate
import com.mmcdole.kivo.api.models.ListWorkAdd
import com.mmcdole.kivo.api.models.SmartRules
import com.mmcdole.kivo.api.models.WorkField
import com.mmcdole.kivo.api.models.WorkFieldValue
import com.mmcdole.kivo.api.models.WorkQuery
import com.mmcdole.kivo.api.models.WorkQueryRequest
import com.mmcdole.kivo.api.models.WorkList
import com.mmcdole.kivo.api.models.WorkSummaryPage
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.live.Invalidation
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.flow.map

/** The user's lists and the works in them, most recently added first; the live events tell every screen, this app's too. */
internal class ListsRepository(private val connection: ServerConnection, private val invalidations: Flow<Invalidation>) {
    /** The user's lists, My List first. */
    fun lists(): Flow<List<WorkList>> = refreshing(invalidations, ::mayChange) {
        connection.works.listLists().result().items
    }

    /** The first page of a list's works of [kinds], or every kind when null, [limit] long or the server's page. */
    fun works(listId: String, limit: Int? = null, kinds: List<String>? = null): Flow<WorkSummaryPage> = refreshing(invalidations, ::mayChange) {
        connection.works.listListWorks(listId, kind = kinds, limit = limit).result()
    }

    private fun mayChange(invalidation: Invalidation) =
        invalidation !is Invalidation.PlayEnded && invalidation != Invalidation.DownloadsChanged && invalidation != Invalidation.HomeChanged

    /** [limit] of a list's works of [kinds] from [offset], or the server's page; every page carries the total. */
    suspend fun page(listId: String, offset: Int, limit: Int? = null, kinds: List<String>? = null): WorkSummaryPage =
        connection.works.listListWorks(listId, kind = kinds, offset = offset.takeIf { it > 0 }, limit = limit).result()

    /** Emits when a list's works may have changed: reload the loaded pages, keeping the place. */
    fun changes(): Flow<Unit> = invalidations.filter(::mayChange).map { }

    suspend fun create(name: String, query: WorkQuery? = null): WorkList =
        connection.works.createList(ListCreate(name, query?.let(::SmartRules))).result()

    suspend fun update(id: String, name: String?, query: WorkQuery? = null): WorkList =
        connection.works.updateList(id, ListUpdate(name, query?.let(::SmartRules))).result()

    /** How many works [query] selects: the first page's total, read one summary long. */
    suspend fun countMatching(query: WorkQuery): Int =
        connection.works.queryWorks(WorkQueryRequest(query, limit = 1)).result().total

    suspend fun fields(): List<WorkField> = connection.works.listWorkFields().result().items

    suspend fun values(field: String, search: String?, ids: List<String>?): List<WorkFieldValue> =
        connection.works.listWorkFieldValues(field, search, ids).result().items

    suspend fun delete(id: String) = connection.works.deleteList(id).check()

    suspend fun add(listId: String, workId: String) = connection.works.addListWork(listId, ListWorkAdd(workId)).check()

    suspend fun remove(listId: String, workId: String) = connection.works.removeListWork(listId, workId).check()
}
