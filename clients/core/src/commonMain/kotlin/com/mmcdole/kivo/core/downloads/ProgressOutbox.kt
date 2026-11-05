package com.mmcdole.kivo.core.downloads

import com.mmcdole.kivo.api.ProblemException
import com.mmcdole.kivo.api.models.UserStateUpdate
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.downloads.db.DownloadsQueries
import kotlin.time.Instant
import com.mmcdole.kivo.core.downloads.db.Download as Row

/**
 * The progress outbox: where offline play of each work got to, kept until
 * the server has it, each work's latest only, which the server applies when
 * it's newer than what it has.
 */
internal class ProgressOutbox(private val q: DownloadsQueries, private val now: () -> Long) {
    /** Records where offline play of [row] on [versionId] is, in the outbox and on the download. */
    fun record(row: Row, versionId: String, positionMs: Long) {
        q.transaction {
            q.record(row.server_id, row.account_id, row.work_id, versionId, positionMs, now())
            q.setPosition(positionMs, row.id)
        }
    }

    /** Sends [s] the outbox: each work's latest offline position. */
    suspend fun send(s: DownloadServer) {
        for (entry in q.outbox(s.connection.serverId).executeAsList()) {
            try {
                s.connection.works.updateWorkUserState(
                    entry.work_id,
                    UserStateUpdate(positionMs = entry.position_ms.toInt(), playedAt = Instant.fromEpochMilliseconds(entry.at), versionId = entry.version_id),
                ).result()
            } catch (e: ProblemException) {
                if (e.status != 404) throw e
                // the work or its version is gone: nothing to send it to
            }
            q.sent(entry.server_id, entry.account_id, entry.work_id, entry.at)
        }
    }
}
