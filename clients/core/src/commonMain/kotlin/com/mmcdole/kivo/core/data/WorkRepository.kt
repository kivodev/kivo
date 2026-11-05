package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.BookSeries
import com.mmcdole.kivo.api.models.Collection
import com.mmcdole.kivo.api.models.Person
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.live.Invalidation
import kotlinx.coroutines.flow.Flow

/**
 * Detail pages: a work, and a series' seasons or a season's episodes; a
 * collection's page with its root works; an author's or narrator's page and a
 * book series' page.
 */
internal class WorkRepository(private val connection: ServerConnection, private val invalidations: Flow<Invalidation>) {
    fun work(id: String): Flow<WorkDetail> = refreshing(invalidations, { it.concerns(id) }) {
        connection.works.getWork(id).result()
    }

    /** A person's page: their root works and books, refetched as works or their played marks change. */
    fun person(id: String): Flow<Person> = refreshing(invalidations, { it.touchesBooks() }) {
        connection.works.getPerson(id).result()
    }

    /** A book series' page: its books in series order, refetched as books or their played marks change. */
    fun bookSeries(id: String): Flow<BookSeries> = refreshing(invalidations, { it.touchesBooks() }) {
        connection.works.getBookSeries(id).result()
    }

    /** A collection's page: its root works in release order, refetched as the libraries or played marks change. */
    fun collection(id: String): Flow<Collection> =
        refreshing(invalidations, { it is Invalidation.LibraryChanged || it is Invalidation.UserStateChanged }) {
            connection.works.getCollection(id).result()
        }

    fun children(workId: String): Flow<List<WorkSummary>> =
        refreshing(invalidations, { it is Invalidation.LibraryChanged || it is Invalidation.UserStateChanged || it.concerns(workId) }) {
            connection.works.listWorkChildren(workId).result().items
        }

    private fun Invalidation.concerns(id: String) =
        (this is Invalidation.WorkChanged && workId == id) || (this is Invalidation.UserStateChanged && id in workIds)

    private fun Invalidation.touchesBooks() =
        this is Invalidation.LibraryChanged || this is Invalidation.WorkChanged || this is Invalidation.UserStateChanged
}
