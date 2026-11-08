package com.mmcdole.kivo.android.downloads

import android.content.Context
import androidx.work.Constraints
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.workDataOf
import com.mmcdole.kivo.core.downloads.DownloadTransport
import com.mmcdole.kivo.core.downloads.TransferEvent
import com.mmcdole.kivo.core.downloads.TransferRequest
import kotlinx.coroutines.sync.Mutex
import java.io.File
import java.util.concurrent.ConcurrentHashMap

/**
 * The core's transfers on WorkManager: each file is one unique work, by its
 * transfer's ID, run by [DownloadWorker] in the foreground with a progress
 * notification, under a network constraint, Wi-Fi only or any network as
 * the request's cellular rule says. WorkManager keeps the work across
 * relaunches and starts it again when its network comes back; each run goes
 * on from what the last fetched ([RangeFetch]).
 *
 * A request's token is kept in memory only, never in WorkManager's store: a
 * run in a process the app started afresh, with no request here yet, fails
 * at once, and the core starts it again with one.
 */
class WorkTransport(context: Context, private val folder: File) : DownloadTransport {
    private val work = WorkManager.getInstance(context)

    /** Hears each transfer's progress, end or failure: the core. */
    var events: (TransferEvent) -> Unit = {}

    /** A download's title for its notification, by its ID, while it's known. */
    var titleOf: (downloadId: String) -> String? = { null }

    /** The requests started in this process, by transfer. */
    private val requests = ConcurrentHashMap<String, TransferRequest>()

    /** One run at a time for each transfer, as one replaced lets go of its file. */
    private val running = ConcurrentHashMap<String, Mutex>()

    override fun start(request: TransferRequest) {
        val last = requests.put(request.id, request)
        // Under way with the same rule and from the same URL, it's left alone; else it goes on under this request.
        val same = last != null && last.url == request.url && last.cellular == request.cellular
        work.enqueueUniqueWork(
            name(request.id),
            if (same) ExistingWorkPolicy.KEEP else ExistingWorkPolicy.REPLACE,
            OneTimeWorkRequestBuilder<DownloadWorker>()
                .setConstraints(Constraints(requiredNetworkType = if (request.cellular) NetworkType.CONNECTED else NetworkType.UNMETERED))
                .setInputData(workDataOf(DownloadWorker.ID to request.id))
                .addTag(TAG)
                .addTag(TRANSFER + request.id)
                .build(),
        )
    }

    override fun stop(id: String) {
        work.cancelUniqueWork(name(id))
    }

    override fun cancel(id: String) {
        work.cancelUniqueWork(name(id))
        RangeFetch.discard(destination(id))
        requests.remove(id)
    }

    override fun running(): List<String> = work.getWorkInfosByTag(TAG).get()
        .filterNot { it.state.isFinished }
        .mapNotNull { info -> info.tags.firstOrNull { it.startsWith(TRANSFER) }?.removePrefix(TRANSFER) }

    /** The request [id] was started with in this process, with its token; null when it wasn't. */
    internal fun request(id: String): TransferRequest? = requests[id]

    internal fun lock(id: String): Mutex = running.getOrPut(id) { Mutex() }

    private fun destination(id: String) = requests[id]?.destination?.let(::File) ?: File(folder, id)

    private fun name(id: String) = "download:$id"

    private companion object {
        const val TAG = "kivo-download"
        const val TRANSFER = "transfer:"
    }
}
