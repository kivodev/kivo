package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.Home
import com.mmcdole.kivo.api.models.HomeRowChange
import com.mmcdole.kivo.api.models.HomeRowInfo
import com.mmcdole.kivo.api.models.HomeRowsUpdate
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.live.Invalidation
import kotlinx.coroutines.flow.Flow

/** Home's rows, refetched when root works, works or the user's state change. */
internal class HomeRepository(private val connection: ServerConnection, private val invalidations: Flow<Invalidation>) {
    /** The rows Home shows, or with [kind] that kind's rows, shown or not. */
    fun home(kind: String? = null): Flow<Home> = refreshing(invalidations, { it !is Invalidation.PlayEnded && it != Invalidation.DownloadsChanged }) {
        connection.works.getHome(kind?.let(::listOf)).result()
    }

    /** Every row of the user's Home, in order, shown or not, as the editor lists them. */
    fun rows(): Flow<List<HomeRowInfo>> = refreshing(invalidations, { it == Invalidation.HomeChanged || it == Invalidation.ListsChanged || it is Invalidation.LibraryChanged }) {
        connection.works.listHomeRows().result().items
    }

    /** Sets the rows' order and which are shown, and returns the layout as the server has it. */
    suspend fun setRows(rows: List<HomeRowChange>): List<HomeRowInfo> =
        connection.works.updateHomeRows(HomeRowsUpdate(rows)).result().items

    /** Forgets the order and hidden rows, and returns the layout as the server has it. */
    suspend fun resetRows(): List<HomeRowInfo> = connection.works.resetHomeRows().result().items
}
