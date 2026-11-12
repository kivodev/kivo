package com.mmcdole.kivo.core.downloads

import com.mmcdole.kivo.core.downloads.Downloads.Companion.CONVERTING
import com.mmcdole.kivo.core.downloads.Downloads.Companion.DOWNLOADING
import com.mmcdole.kivo.core.downloads.Downloads.Companion.PAUSED
import com.mmcdole.kivo.core.downloads.Downloads.Companion.QUEUED
import com.mmcdole.kivo.core.downloads.db.DownloadsQueries
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.IO
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.withContext
import okio.FileSystem
import okio.Path.Companion.toPath
import okio.SYSTEM

/**
 * The space downloads take in [disk]'s folder and what's free beside it,
 * and whether another fits beside those under way, by what of each has
 * arrived ([received]).
 */
internal class Storage(
    private val q: DownloadsQueries,
    private val disk: DownloadDisk,
    private val server: StateFlow<DownloadServer?>,
    private val received: (id: String) -> Long,
) {
    private val fs = FileSystem.SYSTEM

    /** The space downloads take, and what's free beside them, read off the main thread. */
    suspend fun storage(): DownloadStorage = withContext(Dispatchers.IO) {
        val folder = disk.folder.toPath()
        val used = if (fs.exists(folder)) fs.listRecursively(folder).sumOf { fs.metadataOrNull(it)?.size ?: 0 } else 0
        DownloadStorage(used, disk.freeBytes())
    }

    /** Throws [NotEnoughSpaceException] unless [bytes] fit beside the downloads under way, with headroom. */
    fun ensureRoom(bytes: Long, excluding: String? = null) {
        val s = server.value ?: return
        val pending = q.downloads(s.connection.serverId).executeAsList()
            .filter { it.id != excluding && (it.state == QUEUED || it.state == CONVERTING || it.state == DOWNLOADING || it.state == PAUSED) }
            .sumOf { (it.size_bytes - received(it.id)).coerceAtLeast(0) }
        val short = bytes + pending + HEADROOM_BYTES - disk.freeBytes()
        if (short > 0) throw NotEnoughSpaceException(short)
    }
}

/** The space downloads take on the device, and the space free beside them. */
data class DownloadStorage(val usedBytes: Long, val freeBytes: Long)

/** The device hasn't room for a download: [shortBytes] more must be freed. */
internal class NotEnoughSpaceException(val shortBytes: Long) : Exception("not enough space: $shortBytes bytes short")

/** Room kept free beyond the downloads under way, so the device isn't filled to the last byte. */
internal const val HEADROOM_BYTES: Long = 256L * 1024 * 1024
