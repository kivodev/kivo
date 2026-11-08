package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.Home
import com.mmcdole.kivo.api.models.HomeRowChange
import com.mmcdole.kivo.api.models.HomeRowInfo
import com.mmcdole.kivo.api.models.HomeRowsUpdate
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.live.Invalidation
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.filterNotNull
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/** Home's rows, refetched when root works, works or the user's state change. */
internal class HomeRepository(private val connection: ServerConnection, private val invalidations: Flow<Invalidation>) {
    /** Saves of the layout run one at a time, in order. */
    private val saveTurn = Mutex()

    /** Layout saves in flight, and how many have begun. A read that overlaps one is dropped. */
    private val saves = MutableStateFlow(SaveCount())

    /** The rows Home shows, or with [kind] that kind's rows, shown or not. */
    fun home(kind: String? = null): Flow<Home> = refreshing(invalidations, { it !is Invalidation.PlayEnded && it != Invalidation.DownloadsChanged }) {
        connection.works.getHome(kind?.let(::listOf)).result()
    }

    /**
     * Every row of the user's Home, in order, shown or not, as the editor
     * lists them. A read that overlaps a save is dropped, so it can't undo a
     * change the editor is already showing.
     */
    fun rows(): Flow<List<HomeRowInfo>> = refreshing(invalidations, { it == Invalidation.HomeChanged || it is Invalidation.LibraryChanged }) {
        val before = saves.value
        val read = connection.works.listHomeRows().result().items
        read.takeIf { before.running == 0 && saves.value == before }
    }.filterNotNull()

    /** Sets the rows' order and which are shown, and returns the layout as the server has it. */
    suspend fun setRows(rows: List<HomeRowChange>): List<HomeRowInfo> =
        saving { connection.works.updateHomeRows(HomeRowsUpdate(rows)).result().items }

    /** Forgets the order and hidden rows, and returns the layout as the server has it. */
    suspend fun resetRows(): List<HomeRowInfo> = saving { connection.works.resetHomeRows().result().items }

    private suspend fun <T> saving(save: suspend () -> T): T {
        saves.update { SaveCount(it.begun + 1, it.running + 1) }
        try {
            return saveTurn.withLock { save() }
        } finally {
            saves.update { it.copy(running = it.running - 1) }
        }
    }

    private data class SaveCount(val begun: Int = 0, val running: Int = 0)
}
