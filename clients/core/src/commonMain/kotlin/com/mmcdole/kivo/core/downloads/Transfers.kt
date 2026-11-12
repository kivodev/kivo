package com.mmcdole.kivo.core.downloads

import com.mmcdole.kivo.core.downloads.Downloads.Companion.ARTWORK
import com.mmcdole.kivo.core.downloads.Downloads.Companion.CONVERTING
import com.mmcdole.kivo.core.downloads.Downloads.Companion.DONE
import com.mmcdole.kivo.core.downloads.Downloads.Companion.DOWNLOADING
import com.mmcdole.kivo.core.downloads.Downloads.Companion.FAILED
import com.mmcdole.kivo.core.downloads.Downloads.Companion.PART
import com.mmcdole.kivo.core.downloads.Downloads.Companion.PAUSED
import com.mmcdole.kivo.core.downloads.Downloads.Companion.QUEUED
import com.mmcdole.kivo.core.downloads.Downloads.Companion.READY
import com.mmcdole.kivo.core.downloads.Downloads.Companion.TRANSFERRING
import com.mmcdole.kivo.core.downloads.Downloads.Companion.inOrder
import com.mmcdole.kivo.core.downloads.db.DownloadsQueries
import com.mmcdole.kivo.core.quietly
import com.mmcdole.kivo.core.settings.DeviceSettings
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.getAndUpdate
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import okio.FileSystem
import okio.SYSTEM
import kotlin.coroutines.CoroutineContext
import com.mmcdole.kivo.core.downloads.db.Download as Row
import com.mmcdole.kivo.core.downloads.db.Download_file as FileRow

/**
 * The downloads' files coming to the device ([Downloads]), over the
 * platform's [transport] into [disk]'s folder.
 *
 * At most [MAX_TRANSFERS] downloads come here at once, so the first finish
 * soon rather than all crawling together: each the server has ready waits
 * its turn ([WaitReason.Turn]), in the order they were asked for
 * ([inOrder]), and takes a slot as one finishes, fails, pauses or goes.
 * Artwork comes at once, whatever its download's turn. Each file that
 * arrives is checked against its size, and a conversion's against its
 * SHA-256, and fetched again if it doesn't match; a transfer that fails
 * starts again after a wait, longer each time ([Downloads.backoff]); the
 * transport's progress, held in memory, gives each download's speed.
 *
 * What it can't settle itself it leaves to the downloads: [reread] reads
 * again from the server a download whose file may have changed there,
 * [reask] asks again for one the server forgot, [checkNow] runs the
 * availability check after a refusal, and [completed] follows a download
 * becoming whole.
 */
internal class Transfers(
    private val q: DownloadsQueries,
    private val lock: Mutex,
    private val disk: DownloadDisk,
    private val transport: DownloadTransport,
    private val server: StateFlow<DownloadServer?>,
    private val settings: suspend () -> DeviceSettings,
    private val scope: CoroutineScope,
    private val now: () -> Long,
    private val background: CoroutineContext,
    private val reread: suspend (id: String) -> Unit,
    private val reask: suspend (Row) -> Unit,
    private val checkNow: suspend () -> Unit,
    private val completed: suspend () -> Unit,
) {
    private val fs = FileSystem.SYSTEM

    /** Transfers' progress as the transport reports it, by download, held in memory. */
    private val mutableLive = MutableStateFlow<Map<String, Live>>(emptyMap())
    val live: StateFlow<Map<String, Live>> = mutableLive.asStateFlow()

    /** Failed transfers waiting to start again, and how often each has failed, by transfer. */
    private val retries = mutableMapOf<String, Job>()
    private val attempts = mutableMapOf<String, Int>()

    /** What transfers wait for: the device's network, and whether cellular downloads are allowed. */
    private val mutableConditions = MutableStateFlow(Conditions())
    val conditions: StateFlow<Conditions> = mutableConditions.asStateFlow()

    /** The downloads whose files are handed to the transport, at most [MAX_TRANSFERS]; changed only under [admitting]. */
    private val mutableSlots = MutableStateFlow<Set<String>>(emptySet())
    val slots: StateFlow<Set<String>> = mutableSlots.asStateFlow()
    private val admitting = Mutex()

    data class Live(
        val received: Map<String, Long> = emptyMap(),
        val bytesPerSecond: Long = 0,
        val sampledAt: Long = 0,
        val sampledBytes: Long = 0,
        /** Files whose transfer failed and waits to start again. */
        val failing: Set<String> = emptySet(),
    )

    data class Conditions(val path: NetworkPath = NetworkPath(online = true, expensive = false), val cellular: Boolean = false)

    /** Takes in whether cellular downloads are allowed now; whether that changed. */
    suspend fun followCellular(): Boolean {
        val cellular = settings().cellularDownloads
        return mutableConditions.getAndUpdate { it.copy(cellular = cellular) }.cellular != cellular
    }

    /** Takes in the device's network; whether it's on one again, or a different one. */
    fun network(path: NetworkPath): Boolean {
        val was = mutableConditions.getAndUpdate { it.copy(path = path) }.path
        return path.online && path != was
    }

    /**
     * Hands every file still to come of the downloads holding a slot, and
     * any artwork, to the transport again, from the server's address in
     * use and under the cellular rule now: a transfer under way goes on
     * under them, resuming what it fetched, and one waiting to start again
     * after a failure starts now. A download waiting its turn keeps waiting.
     */
    suspend fun restartAll() {
        val s = server.value ?: return
        for (row in q.downloads(s.connection.serverId).executeAsList()) {
            if (row.state != DOWNLOADING && row.state != QUEUED && row.state != CONVERTING) continue
            for (f in q.files(row.id).executeAsList()) {
                // The transport puts a file at its destination only once it's whole: one there is being checked.
                if (f.state == DONE || row.state != DOWNLOADING && f.role != ARTWORK || fs.exists(disk.path(f))) continue
                start(row, f)
            }
            resetSpeed(row.id)
        }
        admit()
    }

    /**
     * Stops every transfer under way when none may carry the token now, as
     * the server has no HTTPS address and the address in use isn't trusted
     * on this network, keeping what each fetched.
     */
    fun holdUntrusted() {
        val s = server.value ?: return
        if (s.connection.transferBase() != null) return
        val running = transport.running().toSet()
        for (row in q.downloads(s.connection.serverId).executeAsList()) {
            for (f in q.files(row.id).executeAsList()) {
                val id = f.transferId
                if (id in running) transport.stop(id)
            }
        }
    }

    /** From the platform's transport: how a transfer is going. Any thread. */
    fun transferred(event: TransferEvent) {
        val (download, name) = event.id.split('/', limit = 2).takeIf { it.size == 2 } ?: return
        when (event) {
            is TransferEvent.Progress -> mutableLive.update { all ->
                val l = all[download] ?: Live()
                val received = l.received + (name to event.received)
                val total = received.values.sum()
                val t = now()
                val (speed, at, bytes) = if (t - l.sampledAt >= 1000) {
                    val instant = if (l.sampledAt == 0L) 0 else (total - l.sampledBytes) * 1000 / (t - l.sampledAt)
                    Triple(if (l.bytesPerSecond == 0L) instant else (l.bytesPerSecond * 2 + instant) / 3, t, total)
                } else {
                    Triple(l.bytesPerSecond, l.sampledAt, l.sampledBytes)
                }
                all + (download to l.copy(received = received, bytesPerSecond = speed.coerceAtLeast(0), sampledAt = at, sampledBytes = bytes, failing = l.failing - name))
            }
            is TransferEvent.Finished -> scope.launch { quietly { arrived(download, name) } }
            is TransferEvent.Failed -> scope.launch { quietly { failed(download, name, event.status) } }
        }
    }

    /**
     * Picks up where each download's files are, after a relaunch, back on a
     * network or with the server reached: a file that arrived while the app
     * was away is checked, and one the transport isn't fetching, as one
     * waiting to start again after a failure, is started again, resuming
     * what it has. A paused download stays as it is. After a relaunch, a
     * download the transport is still fetching holds its slot, and one past
     * the limit stops, keeping what it fetched; free slots go to the next.
     */
    suspend fun pickUp() {
        val s = server.value ?: return
        val running = transport.running().toSet()
        for (row in q.downloads(s.connection.serverId).executeAsList()) {
            if (row.state == READY || row.state == FAILED || row.state == PAUSED) continue
            for (f in q.files(row.id).executeAsList()) {
                if (f.state == DONE || f.transferId in running || row.state != DOWNLOADING && f.role != ARTWORK) continue
                // The transport puts a file at its destination only once it's whole.
                if (fs.exists(disk.path(f))) arrived(row.id, f.name) else if (f.role == ARTWORK) start(row, f)
            }
        }
        val rows = q.downloads(s.connection.serverId).executeAsList().filter { it.state == DOWNLOADING }
        val under = inOrder(rows).filter { row -> q.files(row.id).executeAsList().any { it.role != ARTWORK && it.state != DONE && it.transferId in running } }
        val held = admitting.withLock {
            val ids = rows.map { it.id }.toSet()
            val kept = mutableSlots.value intersect ids
            mutableSlots.value = kept + under.map { it.id }.filter { it !in kept }.take((MAX_TRANSFERS - kept.size).coerceAtLeast(0))
            mutableSlots.value
        }
        for (row in under) {
            if (row.id in held) continue
            for (f in q.files(row.id).executeAsList()) {
                if (f.role != ARTWORK && f.transferId in running) transport.stop(f.transferId)
            }
            resetSpeed(row.id)
        }
        for (row in rows) {
            if (row.id in held) startFiles(row) { it.transferId in running || fs.exists(disk.path(it)) }
        }
        admit()
    }

    /**
     * Hands a download's artwork to the transport, and, once the server has
     * it ready, the rest of its files that aren't here when it holds a slot
     * or gets one; one that no longer comes here gives its slot up.
     */
    suspend fun startTransfers(id: String) {
        val row = row(id) ?: return
        if (row.state != DOWNLOADING) release(id)
        if (row.state != DOWNLOADING && row.state != QUEUED && row.state != CONVERTING) return
        for (f in q.files(id).executeAsList()) {
            if (f.state != DONE && f.role == ARTWORK) start(row, f)
        }
        if (row.state != DOWNLOADING) return
        if (id in mutableSlots.value) send(row) else admit()
        complete(id)
    }

    /**
     * Gives the free slots to the downloads the server has ready, in the
     * order they come ([inOrder]), and hands each its files.
     */
    private suspend fun admit() {
        val s = server.value ?: return
        val next = admitting.withLock {
            val rows = q.downloads(s.connection.serverId).executeAsList().filter { it.state == DOWNLOADING }
            val held = mutableSlots.value intersect rows.map { it.id }.toSet()
            val next = inOrder(rows).filter { it.id !in held }.take((MAX_TRANSFERS - held.size).coerceAtLeast(0))
            mutableSlots.value = held + next.map { it.id }
            next
        }
        for (row in next) {
            send(row)
            complete(row.id)
        }
    }

    /** A download's slot goes to the next, as it finishes, fails, pauses or goes. */
    suspend fun release(id: String) {
        if (id !in mutableSlots.value) return
        admitting.withLock { mutableSlots.update { it - id } }
        admit()
    }

    /** Hands a download holding a slot its files that aren't here, unless there's no longer room for them. */
    private suspend fun send(row: Row) {
        if (disk.freeBytes() < row.size_bytes - received(row.id)) {
            lock.withLock { q.setState(FAILED, DownloadStatus.NOT_ENOUGH_SPACE, row.id) }
            release(row.id)
            return
        }
        startFiles(row)
    }

    /** Hands a download holding a slot its files still to come, but those to [skip]. */
    private suspend fun startFiles(row: Row, skip: (FileRow) -> Boolean = { false }) {
        for (f in q.files(row.id).executeAsList()) {
            if (f.state != DONE && f.role != ARTWORK && !skip(f)) start(row, f)
        }
    }

    private suspend fun start(row: Row, f: FileRow) {
        val s = server.value?.takeIf { it.connection.serverId == row.server_id } ?: return
        if (row.state == PAUSED) return
        // Artwork comes at once; the rest only while the download holds a slot.
        if (f.role != ARTWORK && row.id !in mutableSlots.value) return
        // Only where the token may go; with none now, it starts once one is proven.
        val url = s.connection.apiUrl(s.connection.transferBase() ?: return, f.url)
        fs.createDirectories(disk.dir(row.id))
        retries.remove(f.transferId)?.cancel()
        mutableLive.update { all -> all[row.id]?.let { all + (row.id to it.copy(failing = it.failing - f.name)) } ?: all }
        q.setFileState(TRANSFERRING, f.received_bytes, f.download_id, f.name)
        transport.start(
            TransferRequest(
                id = f.transferId,
                url = url,
                authorization = s.connection.authorization(url),
                destination = disk.path(f).toString(),
                cellular = settings().cellularDownloads,
            ),
        )
    }

    /** A file is here: checked, kept, and with the last, the download is ready; or fetched again. */
    private suspend fun arrived(id: String, name: String) {
        val f = q.files(id).executeAsList().firstOrNull { it.name == name } ?: return
        if (f.state == DONE) return
        val path = disk.path(f)
        val size = fs.metadataOrNull(path)?.size
        val sizeOk = size != null && (f.size_bytes?.let { it == size } ?: (size > 0))
        val hashOk = sizeOk && (f.sha256 == null || withContext(background) { disk.sha256(path.toString()) } == f.sha256)
        if (sizeOk && hashOk) {
            attempts.remove(f.transferId)
            lock.withLock { q.setFileState(DONE, size, id, name) }
            mutableLive.update { all -> all[id]?.let { all + (id to it.copy(received = it.received - name, failing = it.failing - name)) } ?: all }
            complete(id)
            return
        }
        fs.delete(path, mustExist = false)
        transport.cancel(f.transferId)
        lock.withLock { q.rejectFile(id, name) }
        if (f.rejected + 1 >= CHECKS) {
            lock.withLock { q.setState(FAILED, DownloadStatus.CHECK_FAILED, id) }
            release(id)
            return
        }
        // An original may have changed on the server since: its size and validator are read again.
        quietly { reread(id) }
        row(id)?.let { row -> q.files(id).executeAsList().firstOrNull { it.name == name }?.let { start(row, it) } }
    }

    /**
     * A transfer stopped. The server forgot the download (`404`): it's
     * asked for again. A range it can't satisfy: the file starts over.
     * Refused (`401`, `403`): the availability check asks whether the user
     * may still have it, which deletes it if not, and signs the app out if
     * its session is gone; otherwise it starts again after a wait. With too
     * little room left for the rest, the download fails for want of space,
     * keeping what arrived. Anything else, such as a dropped network or a
     * server that can't be reached, starts again after a wait, resuming
     * what arrived, and the download waits for the server meanwhile.
     */
    private suspend fun failed(id: String, name: String, status: Int) {
        val row = row(id) ?: return
        if (row.state != DOWNLOADING && row.state != QUEUED && row.state != CONVERTING) return
        val f = q.files(id).executeAsList().firstOrNull { it.name == name } ?: return
        val key = f.transferId
        if (row.state == DOWNLOADING && disk.freeBytes() < row.size_bytes - received(id)) {
            lock.withLock { q.setState(FAILED, DownloadStatus.NOT_ENOUGH_SPACE, id) }
            halt(id)
            release(id)
            return
        }
        when (status) {
            404 -> if (f.role == ARTWORK) {
                lock.withLock { q.deleteFile(id, name) } // the work has no artwork: it shows without
            } else {
                reask(row)
                startTransfers(id)
            }
            416 -> {
                transport.cancel(key)
                fs.delete(disk.path(f), mustExist = false)
                start(row, f)
            }
            401, 403 -> {
                quietly { checkNow() }
                if (row(id) != null) later(id, name)
            }
            else -> later(id, name)
        }
    }

    /** Starts a failed transfer again after a wait, longer each time; the download waits for the server meanwhile. */
    private fun later(id: String, name: String) {
        val key = "$id/$name"
        val attempt = attempts[key] ?: 0
        attempts[key] = attempt + 1
        mutableLive.update { all -> all + (id to (all[id] ?: Live()).let { it.copy(bytesPerSecond = 0, sampledAt = 0, failing = it.failing + name) }) }
        retries.remove(key)?.cancel()
        retries[key] = scope.launch {
            delay(Downloads.backoff(attempt))
            row(id)?.let { r -> q.files(id).executeAsList().firstOrNull { it.name == name }?.let { start(r, it) } }
        }
    }

    /** Stops a download's transfers, keeping what they fetched. */
    fun halt(id: String) {
        for (f in q.files(id).executeAsList()) {
            if (f.state == DONE) continue
            transport.stop(f.transferId)
            retries.remove(f.transferId)?.cancel()
        }
        mutableLive.update { all -> all[id]?.let { all + (id to it.copy(bytesPerSecond = 0, sampledAt = 0, failing = emptySet())) } ?: all }
    }

    /** Cancels a download's transfers, which won't go on: it's being deleted. */
    fun cancel(files: List<FileRow>) {
        for (f in files) {
            transport.cancel(f.transferId)
            retries.remove(f.transferId)?.cancel()
        }
    }

    /** Forgets a download's progress: it's gone. */
    fun forget(id: String) {
        mutableLive.update { it - id }
    }

    /** Cancels a file's transfer and deletes what of it is here, as a file a new plan no longer names. */
    fun discard(f: FileRow) {
        transport.cancel(f.transferId)
        fs.delete(disk.path(f), mustExist = false)
    }

    /** Forgets a download's speed, as its transfers go on under a new rule or stop. */
    private fun resetSpeed(id: String) {
        mutableLive.update { all -> all[id]?.let { all + (id to it.copy(bytesPerSecond = 0, sampledAt = 0)) } ?: all }
    }

    /** With every file here, the download is ready, its slot goes to the next, and the downloads hear it's whole. */
    private suspend fun complete(id: String) {
        val files = q.files(id).executeAsList()
        if (files.none { it.role == PART } || files.any { it.state != DONE }) return
        lock.withLock {
            val row = q.download(id).executeAsOneOrNull() ?: return
            if (row.state == READY) return
            q.setCompleted(now(), id)
            mutableLive.update { it - id }
        }
        release(id)
        quietly { completed() }
    }

    /** The bytes of a download here: its files that are done, and what's arrived of the rest. */
    fun received(id: String): Long {
        val files = q.files(id).executeAsList()
        val l = mutableLive.value[id]
        return files.sumOf { if (it.state == DONE) it.size_bytes ?: 0 else l?.received?.get(it.name) ?: 0 }
    }

    private fun row(id: String): Row? = q.download(id).executeAsOneOrNull()

    companion object {
        /**
         * What a download's files coming here wait for, if anything: a
         * network; Wi-Fi, on cellular or another expensive network while
         * cellular downloads aren't allowed; its turn, while it holds no
         * slot; or the server, when every file still to come failed to reach
         * it and waits to start again.
         */
        fun waitingFor(files: List<FileRow>, l: Live?, c: Conditions, slotted: Boolean): WaitReason? = when {
            !c.path.online -> WaitReason.Network
            c.path.expensive && !c.cellular -> WaitReason.Wifi
            !slotted -> WaitReason.Turn
            l != null && l.failing.isNotEmpty() && files.filter { it.state != DONE }.all { it.name in l.failing } -> WaitReason.Server
            else -> null
        }
    }
}

/** Downloads coming here at once. */
internal const val MAX_TRANSFERS = 2

/** Fetches of one file that fail their check before the download fails. */
private const val CHECKS = 3
