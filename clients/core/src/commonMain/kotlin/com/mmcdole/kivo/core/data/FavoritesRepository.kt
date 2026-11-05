package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.WorkSummaryPage
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.live.Invalidation
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.flow.map

/**
 * The works the user marked a favorite, one kind at a time, most recently
 * marked first: a kind's first page, refetched as Home is, and its pages by
 * offset, for a grid that loads any position.
 */
internal class FavoritesRepository(private val connection: ServerConnection, private val invalidations: Flow<Invalidation>) {
    /** The first page of [kind]'s favorites; its `total` says whether there are any. */
    fun favorites(kind: String, limit: Int? = null): Flow<WorkSummaryPage> =
        refreshing(invalidations, ::mayChange) {
            connection.works.listFavorites(kind = kind, limit = limit).result()
        }

    /** How many works of every kind the user marked a favorite: the first page's total, read one summary long. */
    fun count(): Flow<Int> =
        refreshing(invalidations, ::mayChange) {
            connection.works.listFavorites(limit = 1).result().total
        }

    private fun mayChange(invalidation: Invalidation) =
        invalidation !is Invalidation.PlayEnded && invalidation != Invalidation.DownloadsChanged

    /** [limit] of [kind]'s favorites from [offset], or the server's page; every page carries the total. */
    suspend fun page(kind: String, offset: Int, limit: Int? = null): WorkSummaryPage =
        connection.works.listFavorites(kind = kind, offset = offset.takeIf { it > 0 }, limit = limit).result()

    /** Emits when the favorites may have changed: reload the loaded pages, keeping the place. */
    fun changes(): Flow<Unit> = invalidations
        .filter {
            it == Invalidation.Everything ||
                it is Invalidation.UserStateChanged ||
                it is Invalidation.WorkChanged ||
                it is Invalidation.LibraryChanged
        }
        .map { }
}
