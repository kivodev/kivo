package com.mmcdole.kivo.core.downloads

import com.mmcdole.kivo.api.models.DownloadCheck
import com.mmcdole.kivo.api.models.DownloadRef
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.downloads.db.DownloadsQueries
import com.mmcdole.kivo.core.quietly
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlin.time.Duration
import kotlin.time.Duration.Companion.hours
import kotlin.time.Duration.Companion.seconds
import com.mmcdole.kivo.api.models.Download as ServerDownload
import com.mmcdole.kivo.core.downloads.db.Download as Row

/**
 * Whether the user may still have their downloads, and each download
 * caught up with the server as its offline play begins.
 *
 * The availability check asks the server, at most every [CHECK_EVERY],
 * whether the user may still have each download, and [delete]s those they
 * may not, counting them for the Downloads list. Catching up sends the
 * [outbox], keeps the server's position on the download, and runs the check
 * when it's due.
 */
internal class Availability(
    private val q: DownloadsQueries,
    private val lock: Mutex,
    private val server: StateFlow<DownloadServer?>,
    private val outbox: ProgressOutbox,
    private val scope: CoroutineScope,
    private val now: () -> Long,
    /** A download's plan, or null for one stored in a shape this app no longer reads. */
    private val plan: (Row) -> ServerDownload?,
    private val delete: suspend (id: String) -> Unit,
) {
    /** Each download's latest catching up with the server, by download. */
    private val catchingUp = mutableMapOf<String, Job>()

    /** Runs the check if it's due. */
    suspend fun checkIfDue() {
        val s = server.value ?: return
        val last = q.serverCheck(s.connection.serverId).executeAsOneOrNull()?.checked_at ?: 0
        if (now() - last < CHECK_EVERY.inWholeMilliseconds) return
        checkNow()
    }

    /** Runs the check however recently the server was last asked. */
    suspend fun checkNow() {
        val s = server.value ?: return
        val serverId = s.connection.serverId
        val rows = q.downloads(serverId).executeAsList().mapNotNull { row -> plan(row)?.let { row to it } }
        val answers = if (rows.isEmpty()) {
            emptyList()
        } else {
            s.connection.downloads.checkDownloads(DownloadCheck(rows.map { (row, plan) -> DownloadRef(workId = row.work_id, versionId = plan.versionId) })).result().items
        }
        val gone = rows.filterIndexed { i, _ -> answers.getOrNull(i)?.available == false }
        for ((row, _) in gone) delete(row.id)
        lock.withLock { q.checked(serverId, now(), gone.size.toLong()) }
    }

    /**
     * Catches a download up with [s] in the background, once it answers:
     * sends the outbox, keeps the server's position on it unless offline
     * play has moved it since, and runs the availability check if it's
     * due. One at a time for each download.
     */
    fun catchUp(s: DownloadServer, row: Row): Job {
        catchingUp[row.id]?.takeIf { it.isActive }?.let { return it }
        val job = scope.launch {
            s.reached()
            quietly { outbox.send(s) }
            quietly {
                val state = s.connection.works.getWork(row.work_id).result().userState ?: return@quietly
                val position = if (state.played) 0 else state.positionMs?.toLong() ?: return@quietly
                q.transaction {
                    val now = q.download(row.id).executeAsOneOrNull() ?: return@transaction
                    if (now.position_ms == row.position_ms) q.setPosition(position, row.id)
                }
            }
            quietly { checkIfDue() }
        }
        catchingUp[row.id] = job
        return job
    }
}

/** How often the availability check asks the server, at most. */
internal val CHECK_EVERY: Duration = 1.hours

/** How long a download's play waits to catch up with a server that's answering before it starts from the position stored here. */
internal val CATCH_UP_WAIT: Duration = 1.seconds
