package com.mmcdole.kivo.core.downloads

import app.cash.sqldelight.coroutines.asFlow
import app.cash.sqldelight.coroutines.mapToList
import app.cash.sqldelight.coroutines.mapToOneOrNull
import com.mmcdole.kivo.api.ProblemException
import com.mmcdole.kivo.api.check
import com.mmcdole.kivo.api.models.Capabilities
import com.mmcdole.kivo.api.models.DownloadCreate
import com.mmcdole.kivo.api.models.DownloadEstimate
import com.mmcdole.kivo.api.models.DownloadEstimateItem
import com.mmcdole.kivo.api.models.DownloadEstimateRequest
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.Backoff
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.auth.imagePath
import com.mmcdole.kivo.core.downloads.db.DownloadDatabase
import com.mmcdole.kivo.core.kivoJson
import com.mmcdole.kivo.core.live.Invalidation
import com.mmcdole.kivo.core.quietly
import com.mmcdole.kivo.core.settings.DeviceSettings
import com.mmcdole.kivo.core.settings.DownloadQuality
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Deferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.flow.filterNotNull
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import kotlinx.serialization.SerializationException
import okio.FileSystem
import okio.Path.Companion.toPath
import okio.SYSTEM
import kotlin.coroutines.CoroutineContext
import kotlin.time.Clock
import kotlin.time.Duration
import kotlin.time.Duration.Companion.minutes
import kotlin.time.Duration.Companion.seconds
import com.mmcdole.kivo.api.models.Download as ServerDownload
import com.mmcdole.kivo.api.models.DownloadDelivery
import com.mmcdole.kivo.api.models.TrackDelivery
import com.mmcdole.kivo.core.downloads.db.Download as Row
import com.mmcdole.kivo.core.downloads.db.Download_file as FileRow

/**
 * The device's downloads: the download store and the state machine from the
 * server's queue to a checked file here, with deletion, over the platform's
 * [transport] and [disk]. Its parts are the files coming here
 * ([Transfers]), the progress outbox ([ProgressOutbox]), the availability
 * check and catching up ([Availability]), and storage accounting
 * ([Storage]).
 *
 * A download is asked of the server, which says where it is: `queued`
 * ([DownloadStatus.Queued]), `preparing` ([DownloadStatus.Converting]),
 * `ready` or `failed`. Once ready, each of its files is handed to the
 * transport, which fetches it in the background and resumes it by range;
 * each that arrives is checked against its size, and a conversion's
 * against its SHA-256, and fetched again if it doesn't match. With every
 * file here it's [DownloadStatus.Ready], and the server is told to forget
 * its copy. A server restart, an expired copy (a `404`) or a relaunch
 * never loses a download: [sync] and [pickUp] pick each up where it is.
 *
 * A download coming here waits, rather than fails, for what it needs: a
 * network, Wi-Fi while cellular downloads aren't allowed, or the server
 * after a transfer couldn't reach it. Waiting isn't stored but worked out
 * from those facts as the list is read ([DownloadStatus.Waiting]). The
 * user may pause one coming here ([pause]) and resume it ([resume]),
 * which is stored.
 *
 * At most [MAX_TRANSFERS] downloads come here at once, each the server
 * has ready waiting its turn in the order they were asked for ([inOrder]).
 *
 * Nothing here polls: [invalidated] hears the live events, [network] the
 * device's network, [settingsChanged] the settings, and the app calls
 * [sync] when it reaches the server or returns to the foreground.
 */
@OptIn(ExperimentalCoroutinesApi::class)
internal class Downloads(
    database: DownloadDatabase,
    private val disk: DownloadDisk,
    transport: DownloadTransport,
    private val settings: suspend () -> DeviceSettings,
    private val scope: CoroutineScope,
    private val now: () -> Long = { Clock.System.now().toEpochMilliseconds() },
    private val background: CoroutineContext = Dispatchers.Default,
) {
    private val q = database.downloadsQueries
    private val fs = FileSystem.SYSTEM
    private val lock = Mutex()
    private val server = MutableStateFlow<DownloadServer?>(null)

    /** The works being asked for now, each with the download it's becoming; changed under [lock]. */
    private val inFlight = MutableStateFlow<Map<String, Deferred<String>>>(emptyMap())

    private val transfers = Transfers(
        q, lock, disk, transport, server, settings, scope, now, background,
        reread = ::reread,
        reask = ::reask,
        checkNow = { availability.checkNow() },
        completed = ::forgetFetched,
    )
    private val outbox = ProgressOutbox(q, now)
    private val availability = Availability(q, lock, server, outbox, scope, now, ::readablePlan, ::delete)
    private val room = Storage(q, disk, server, transfers::received)

    /** The attached server's downloads, newest first. */
    val items: Flow<List<DownloadItem>> = server.flatMapLatest { s ->
        if (s == null) {
            flowOf(emptyList())
        } else {
            combine(
                q.downloads(s.connection.serverId).asFlow().mapToList(background),
                q.everyFile().asFlow().mapToList(background),
                transfers.live,
                transfers.conditions,
                transfers.slots,
            ) { rows, files, live, conditions, slots ->
                val byDownload = files.groupBy { it.download_id }
                rows.map { item(it, byDownload[it.id].orEmpty(), live[it.id], conditions, it.id in slots) }
            }
        }
    }

    /** The works being asked of the server now, before their downloads are listed. */
    val asking: Flow<Set<String>> = inFlight.map { it.keys }

    /** How many downloads the last checks deleted because the user may no longer have them, until [deletionsSeen]. */
    val deleted: Flow<Int> = server.flatMapLatest { s ->
        if (s == null) flowOf(0) else q.serverCheck(s.connection.serverId).asFlow().mapToOneOrNull(background).map { it?.removed?.toInt() ?: 0 }
    }

    /**
     * Looks after [server]'s downloads: picks up where each is, and syncs
     * with the server. When the server moves to another of its addresses,
     * every file still to come goes on from there ([Transfers.restartAll]).
     */
    fun attach(server: DownloadServer) {
        this.server.value = server
        scope.launch {
            transfers.followCellular()
            pickUp()
            sync()
        }
    }

    init {
        scope.launch {
            server.filterNotNull().flatMapLatest { it.connection.address.current.drop(1) }.collect { quietly { transfers.restartAll() } }
        }
        // Transfers from a plain-HTTP address stop when it's no longer trusted, as on a new network, and go on once it's proven again.
        scope.launch {
            server.filterNotNull().flatMapLatest { it.connection.address.trusted.drop(1) }.collect { trusted ->
                quietly { if (trusted) transfers.restartAll() else transfers.holdUntrusted() }
            }
        }
    }

    /**
     * The device's network changed, as the platform sees it. On a network
     * again, or a different one, every transfer that isn't under way, as
     * one waiting to start again after a failure, starts now.
     */
    fun network(path: NetworkPath) {
        if (transfers.network(path)) scope.launch { quietly { pickUp() } }
    }

    /**
     * The device's settings changed. When cellular downloads are allowed or
     * no longer are, every file still to come is handed to the transport
     * again under the new rule ([Transfers.restartAll]).
     */
    suspend fun settingsChanged() {
        if (transfers.followCellular()) transfers.restartAll()
    }

    /**
     * Pauses a download coming here: its transfers stop, keeping what they
     * fetched, until [resume], and its slot goes to the next. One the server
     * is preparing goes on there.
     */
    suspend fun pause(id: String) {
        val row = row(id) ?: return
        if (row.state != DOWNLOADING) return
        lock.withLock { q.setState(PAUSED, null, id) }
        transfers.halt(id)
        transfers.release(id)
    }

    /** Resumes a paused download, each file from where it stopped, once it has a slot. */
    suspend fun resume(id: String) {
        val row = row(id) ?: return
        if (row.state != PAUSED) return
        lock.withLock { q.setState(DOWNLOADING, null, id) }
        transfers.startTransfers(id)
    }

    /** What the server hears from the live events: the user's downloads moved. */
    fun invalidated(invalidation: Invalidation) {
        when (invalidation) {
            Invalidation.DownloadsChanged, Invalidation.Everything -> scope.launch { quietly { refresh() } }
            else -> {}
        }
    }

    /**
     * Everything that needs the server, once it's reached: where the
     * downloads it's preparing are, the transfers waiting to start again
     * after a failure, the progress outbox, the availability check (at
     * most hourly), and forgetting the copies of downloads the device has.
     * While the app's reads get no answer from the server, it waits until
     * they do. Failures wait for the next sync.
     */
    suspend fun sync() {
        server.value?.reached()
        quietly { refresh() }
        quietly { pickUp() }
        quietly { sendOutbox() }
        quietly { availability.checkIfDue() }
        quietly { forgetFetched() }
    }

    /**
     * Hands the downloads the server has finished preparing to the
     * transport, as the platform's background refresh does while the app
     * isn't open. Returns whether any is still in line or being prepared
     * on the server.
     */
    suspend fun startReady(): Boolean {
        val s = server.value ?: return false
        quietly { refresh() }
        return q.downloads(s.connection.serverId).executeAsList().any { it.state == QUEUED || it.state == CONVERTING }
    }

    /**
     * What downloading these works would be at each quality, from the
     * server, for the download menu's sizes; a season's or a series'
     * episodes in one call.
     */
    suspend fun estimate(items: List<DownloadEstimateItem>): List<DownloadEstimate> {
        val s = attached()
        return s.connection.downloads.estimateDownloads(DownloadEstimateRequest(items = items, capabilities = s.capabilities())).result().items
    }

    /**
     * Downloads a work: asks the server for it, unless this work is
     * downloaded or on its way already, which is returned as it is (a failed
     * one asked again). Refused with [NotEnoughSpaceException] when the
     * estimate wouldn't fit beside the downloads under way, or
     * [DownloadRefusedException] when the server can't make it for this app.
     * Asked again while it's being asked for, it's the same download.
     * Returns the download's ID.
     */
    suspend fun download(request: DownloadRequest): String {
        val work = request.workId
        val mine = CompletableDeferred<String>()
        val pending = lock.withLock {
            val asked = inFlight.value[work]
            if (asked == null) inFlight.update { it + (work to mine) }
            asked
        }
        if (pending != null) return pending.await()
        try {
            return ask(request).also { mine.complete(it) }
        } catch (e: Throwable) {
            mine.completeExceptionally(e)
            throw e
        } finally {
            withContext(NonCancellable) { lock.withLock { inFlight.update { it - work } } }
        }
    }

    private suspend fun ask(request: DownloadRequest): String {
        val s = attached()
        val c = s.connection
        q.downloads(c.serverId).executeAsList().firstOrNull { it.work_id == request.workId }?.let {
            if (it.state == FAILED) retry(it.id)
            return it.id
        }
        val quality = request.quality ?: settings().downloadQuality
        val caps = s.capabilities()
        val estimate = c.downloads.estimateDownloads(
            DownloadEstimateRequest(
                items = listOf(DownloadEstimateItem(workId = request.workId, versionId = request.versionId, audioTrackId = request.audioTrackId, subtitleTrackId = request.subtitleTrackId)),
                capabilities = caps,
            ),
        ).result().items.single()
        if (!estimate.playable) throw DownloadRefusedException(estimate.reason ?: DownloadStatus.NOT_PLAYABLE)
        val option = estimate.options.firstOrNull { it.quality == quality.value } ?: estimate.options.first()
        room.ensureRoom(option.sizeBytes)
        val user = c.users.getCurrentUser().result().id
        val work = c.works.getWork(request.workId).result()
        val created = create(s, request, quality.value)
        val id = randomId()
        val episode = work.episode
        // A download of this work stored meanwhile stays; the server's copy asked for here goes, unless it's the same one.
        val existing = lock.withLock {
            val stored = q.downloads(c.serverId).executeAsList().firstOrNull { it.work_id == work.id }
            if (stored != null) return@withLock stored
            q.insertDownload(
                Row(
                    id = id, server_id = c.serverId, account_id = user, remote_id = created.id, work_id = work.id,
                    asked_version = request.versionId, asked_audio = request.audioTrackId, asked_subtitle = request.subtitleTrackId,
                    quality = created.quality, state = QUEUED, progress = null, remaining_ms = null, reason = null,
                    plan = kivoJson.encodeToString(ServerDownload.serializer(), created),
                    work = kivoJson.encodeToString(WorkDetail.serializer(), work), title = work.title, kind = work.kind.value,
                    series_id = episode?.seriesId, series_title = episode?.seriesTitle, season = episode?.seasonNumber?.toLong(), episode = episode?.number?.toLong(),
                    size_bytes = created.sizeBytes,
                    created_at = now(), completed_at = null, position_ms = null,
                ),
            )
            artwork(work)?.let { q.insertFile(fileRow(id, ARTWORK, ARTWORK_NAME, it)) }
            poster(work)?.let { q.insertFile(fileRow(id, ARTWORK, POSTER_NAME, it)) }
            null
        }
        if (existing != null) {
            if (existing.remote_id != created.id) quietly { c.downloads.deleteDownload(created.id).check() }
            return existing.id
        }
        apply(id, created)
        transfers.startTransfers(id)
        return id
    }

    /**
     * Asks again for a failed download: the server retries its own
     * failure, and the device fetches again what failed here. A download
     * the server forgot is asked for afresh.
     */
    suspend fun retry(id: String) {
        val row = row(id) ?: return
        if (readablePlan(row) == null) {
            renew(row)
            transfers.startTransfers(id)
            return
        }
        val c = attached().connection
        if (row.reason == DownloadStatus.NOT_ENOUGH_SPACE) room.ensureRoom(row.size_bytes - transfers.received(id), excluding = id)
        val remote = row.remote_id
        val d = try {
            if (remote != null) c.downloads.retryDownload(remote).result() else null
        } catch (e: ProblemException) {
            if (e.status != 404) throw refusal(e)
            null
        }
        if (d != null) {
            lock.withLock { q.setServerState(QUEUED, null, null, null, row.plan, row.size_bytes, id) }
            apply(id, d)
        } else {
            reask(row)
        }
        transfers.startTransfers(id)
    }

    /**
     * Deletes a download from the device: its transfers stop, its files go,
     * and the server forgets its copy.
     */
    suspend fun delete(id: String) {
        val row = row(id) ?: return
        lock.withLock {
            transfers.cancel(q.files(id).executeAsList())
            q.transaction {
                q.deleteFiles(id)
                q.deleteDownload(id)
            }
            transfers.forget(id)
        }
        transfers.release(id)
        fs.deleteRecursively(dir(id))
        val remote = row.remote_id ?: return
        quietly { server.value?.takeIf { it.connection.serverId == row.server_id }?.connection?.downloads?.deleteDownload(remote)?.check() }
    }

    /**
     * Deletes several downloads at once, as Downloads deletes a show or a
     * selection: each as [delete] does, so one still in line,
     * converting or coming here stops, and the server forgets it.
     */
    suspend fun delete(ids: List<String>) {
        for (id in ids) delete(id)
    }

    /** Whether the device keeps downloads of the server [serverId]. */
    suspend fun has(serverId: String): Boolean = withContext(background) { q.downloads(serverId).executeAsList().isNotEmpty() }

    /** Deletes every download of a server the app signed out of, with its outbox. */
    suspend fun forget(serverId: String) {
        delete(q.downloads(serverId).executeAsList().map { it.id })
        lock.withLock {
            q.clearOutbox(serverId)
            q.forgetServer(serverId)
        }
    }

    /** The space downloads take, and what's free beside them, read off the main thread. */
    suspend fun storage(): DownloadStorage = room.storage()

    /** The list has said how many downloads the checks deleted. */
    fun deletionsSeen() {
        server.value?.let { q.deletionsSeen(it.connection.serverId) }
    }

    /** From the platform's transport: how a transfer is going. Any thread. */
    fun transferred(event: TransferEvent) = transfers.transferred(event)

    /**
     * Picks up where each download is, after a relaunch, back on a network
     * or with the server reached ([Transfers.pickUp]).
     */
    suspend fun pickUp() = transfers.pickUp()

    /**
     * Where the downloads the server is preparing are, from its list; one it
     * has ready starts coming here, and one it no longer has is asked for
     * again.
     */
    suspend fun refresh() {
        val s = server.value ?: return
        val rows = q.downloads(s.connection.serverId).executeAsList().filter { (it.state == QUEUED || it.state == CONVERTING) && readablePlan(it) != null }
        if (rows.isEmpty()) return
        val listed = s.connection.downloads.listDownloads().result().items.associateBy { it.id }
        for (row in rows) {
            val d = row.remote_id?.let { listed[it] }
            if (d == null) reask(row) else apply(row.id, d)
        }
        // Once all are taken in, so the slots go in order.
        for (row in rows) transfers.startTransfers(row.id)
    }

    /** Records where offline play of a download is, in the outbox and on the download. */
    fun record(id: String, positionMs: Long) {
        val row = row(id) ?: return
        outbox.record(row, plan(row).versionId, positionMs)
    }

    /** Sends the outbox ([ProgressOutbox]). */
    suspend fun sendOutbox() {
        server.value?.let { outbox.send(it) }
    }

    /**
     * A download ready to play with no server: its play as the player
     * plays it, from its files here, starting where the user last left
     * the work, as stored here.
     *
     * Catching up with the server goes on beside it: the outbox is sent,
     * the server's position is read and kept on the download, and the
     * availability check runs when it's due. While the server is answering
     * the app's reads, play waits up to [CATCH_UP_WAIT] for that, so a work
     * continued on another device resumes from there; past it, or while the
     * server doesn't answer, play starts at once and the position the server
     * gives later is the next play's.
     */
    suspend fun offline(id: String): OfflinePlay {
        var row = row(id) ?: throw NoSuchElementException("no download $id")
        check(row.state == READY) { "download $id isn't ready" }
        val s = server.value?.takeIf { it.connection.serverId == row.server_id }
        if (s != null) {
            val catchUp = availability.catchUp(s, row)
            if (!s.unreachable.value) withTimeoutOrNull(CATCH_UP_WAIT) { catchUp.join() }
            // The availability check catching up may have deleted it.
            row = row(id) ?: throw NoSuchElementException("download $id is no longer available")
        }
        val work = kivoJson.decodeFromString(WorkDetail.serializer(), withTrackNames(row.work))
        val position = row.position_ms ?: work.userState?.positionMs?.toLong() ?: 0
        val plan = plan(row)
        return OfflinePlay(this, id, startedOffline(row, plan, work, position), work, plan)
    }

    /** [workId] as it was downloaded, when the attached server's download of it is ready; null otherwise. */
    suspend fun storedWork(workId: String): WorkDetail? {
        val s = server.value ?: return null
        val row = withContext(background) { q.downloads(s.connection.serverId).executeAsList() }
            .firstOrNull { it.work_id == workId && it.state == READY } ?: return null
        return kivoJson.decodeFromString(WorkDetail.serializer(), withTrackNames(row.work))
    }


    /**
     * A work's download, to play rather than stream, when one of the
     * attached server's is ready; null otherwise, as for one the
     * availability check deleted as play began.
     */
    suspend fun playable(workId: String): OfflinePlay? {
        val s = server.value ?: return null
        val row = q.downloads(s.connection.serverId).executeAsList()
            .firstOrNull { it.work_id == workId && it.state == READY && readablePlan(it) != null } ?: return null
        return try {
            offline(row.id)
        } catch (_: NoSuchElementException) {
            null
        }
    }

    /** Offline play of a download ended: where it got to goes to the server. */
    internal fun played() {
        val s = server.value ?: return
        scope.launch {
            s.reached()
            quietly { outbox.send(s) }
        }
    }

    /** Tells the server to forget its copies of the downloads every file of which is here. */
    private suspend fun forgetFetched() {
        val s = server.value ?: return
        for (row in q.downloads(s.connection.serverId).executeAsList()) {
            val remote = row.remote_id ?: continue
            if (row.state != READY) continue
            try {
                s.connection.downloads.deleteDownload(remote).check()
            } catch (e: ProblemException) {
                if (e.status != 404) throw e
            }
            q.setRemote(null, row.id)
        }
    }

    /**
     * Asks the server again for a download it forgot, as it was asked the
     * first time. One whose work or version is gone (`404`) goes through the
     * availability check, which deletes it and says so; one still there
     * after it fails with the server's reason.
     */
    private suspend fun reask(row: Row) {
        val s = attached()
        val d = try {
            create(s, DownloadRequest(row.work_id, row.asked_version, row.asked_audio, row.asked_subtitle), row.quality)
        } catch (e: DownloadRefusedException) {
            lock.withLock { q.setState(FAILED, e.reason, row.id) }
            return
        } catch (e: ProblemException) {
            if (e.status != 404) throw e
            quietly { availability.checkNow() }
            lock.withLock { q.setState(FAILED, e.problem?.reason ?: e.code, row.id) }
            return
        }
        lock.withLock {
            q.setRemote(d.id, row.id)
            q.setServerState(QUEUED, null, null, null, row.plan, row.size_bytes, row.id)
        }
        apply(row.id, d)
    }

    /** Asks the server for [request]'s download at [quality], as the server names it; [request]'s own quality is ignored. */
    private suspend fun create(s: DownloadServer, request: DownloadRequest, quality: String): ServerDownload = try {
        s.connection.downloads.createDownload(
            DownloadCreate(
                workId = request.workId,
                capabilities = s.capabilities(),
                versionId = request.versionId,
                audioTrackId = request.audioTrackId,
                subtitleTrackId = request.subtitleTrackId,
                quality = quality,
            ),
        ).result()
    } catch (e: ProblemException) {
        throw refusal(e)
    }

    private fun refusal(e: ProblemException): Exception =
        if (e.status == 409) DownloadRefusedException(e.problem?.reason ?: e.code) else e

    /**
     * Takes in where the server has a download: in line, being prepared,
     * failed, or ready, when its files are listed to fetch, any already
     * here and unchanged kept.
     */
    private suspend fun apply(id: String, d: ServerDownload) = lock.withLock {
        val row = q.download(id).executeAsOneOrNull() ?: return@withLock
        if (row.state == READY || row.state == DOWNLOADING && d.state == "ready" && row.plan == planJson(d)) return@withLock
        val plan = planJson(d)
        when (d.state) {
            "queued" -> q.setServerState(QUEUED, null, null, null, plan, d.sizeBytes, id)
            "preparing" -> q.setServerState(CONVERTING, d.progress, d.remainingMs?.toLong(), null, plan, d.sizeBytes, id)
            "failed" -> q.setServerState(FAILED, null, null, d.reason ?: "transcodeFailed", plan, d.sizeBytes, id)
            "ready" -> {
                val kept = q.files(id).executeAsList().associateBy { it.name }
                val files = filesOf(id, d)
                // A file an earlier plan named that this one doesn't goes, so it neither takes room nor counts as here.
                val dropped = kept.values.filter { it.role != ARTWORK && files.none { f -> f.name == it.name } }
                q.transaction {
                    for (f in dropped) q.deleteFile(id, f.name)
                    for (f in files) {
                        val old = kept[f.name]
                        q.insertFile(if (old != null && old.state == DONE && old.size_bytes == f.size_bytes && old.sha256 == f.sha256) old else f)
                    }
                    q.setServerState(if (row.state == PAUSED) PAUSED else DOWNLOADING, null, null, null, plan, d.sizeBytes, id)
                }
                for (f in dropped) transfers.discard(f)
            }
        }
    }

    /** Reads a download again from the server and takes in where it is, as when an original may have changed there. */
    private suspend fun reread(id: String) {
        val row = row(id) ?: return
        row.remote_id?.let { apply(id, attached().connection.downloads.getDownload(it).result()) }
    }

    private fun item(row: Row, files: List<FileRow>, l: Transfers.Live?, c: Transfers.Conditions, slotted: Boolean): DownloadItem {
        val plan = readablePlan(row)
        val received = files.filter { it.role == PART }.sumOf { if (it.state == DONE) it.size_bytes ?: 0 else l?.received?.get(it.name) ?: 0 }
        val status = when {
            plan == null -> DownloadStatus.Failed(row.reason ?: PLAN_UNREADABLE)
            row.state == QUEUED -> DownloadStatus.Queued
            row.state == CONVERTING -> DownloadStatus.Converting(row.progress ?: 0.0, row.remaining_ms)
            row.state == DOWNLOADING -> Transfers.waitingFor(files, l, c, slotted)?.let { DownloadStatus.Waiting(it, received, row.size_bytes) }
                ?: DownloadStatus.Downloading(received, row.size_bytes, l?.bytesPerSecond ?: 0)
            row.state == PAUSED -> DownloadStatus.Paused(received, row.size_bytes)
            row.state == READY -> DownloadStatus.Ready
            else -> DownloadStatus.Failed(row.reason ?: "unknown")
        }
        fun artwork(name: String) = files.firstOrNull { it.role == ARTWORK && it.name == name && it.state == DONE }?.let { path(it).toString() }
        return DownloadItem(
            id = row.id, workId = row.work_id, versionId = plan?.versionId ?: row.asked_version.orEmpty(), title = row.title, kind = WorkKind.of(row.kind),
            seriesId = row.series_id, seriesTitle = row.series_title, season = row.season?.toInt(), episode = row.episode?.toInt(),
            authors = authorsOf(row), quality = DownloadQuality.parseOrNull(row.quality),
            durationMs = plan?.durationMs?.toLong(),
            sizeBytes = row.size_bytes, sizeExact = status == DownloadStatus.Ready || plan?.sizeExact == true,
            prepared = plan?.delivery == DownloadDelivery.Conversion, status = status,
            artworkPath = artwork(ARTWORK_NAME), posterPath = artwork(POSTER_NAME),
            createdAtMs = row.created_at,
        )
    }

    /** A book's authors, from its work as downloaded. */
    private fun authorsOf(row: Row): String? = if (row.kind != WorkKind.Book.value) {
        null
    } else {
        kivoJson.decodeFromString(WorkDetail.serializer(), withTrackNames(row.work)).book?.authors?.takeIf { it.isNotEmpty() }?.joinToString(", ") { it.name }
    }

    /** The files of a download the server has ready: its parts, its subtitle files, their fonts. */
    private fun filesOf(id: String, d: ServerDownload): List<FileRow> = buildList {
        d.parts.forEachIndexed { i, p ->
            add(FileRow(id, "part-$i.${extension(p.contentType)}", PART, p.url, p.sizeBytes, p.etag, p.sha256, WAITING, 0, 0))
        }
        for (sub in d.tracks.filter { it.delivery == TrackDelivery.External }) {
            val url = sub.url ?: continue
            add(FileRow(id, subtitleName(sub.trackId, sub.codec.orEmpty()), SUBTITLE, url, null, null, null, WAITING, 0, 0))
            for (font in sub.fonts.orEmpty()) add(FileRow(id, fontName(font), FONT, font, null, null, null, WAITING, 0, 0))
        }
    }.distinctBy { it.name }

    private fun fileRow(id: String, role: String, name: String, url: String) = FileRow(id, name, role, url, null, null, null, WAITING, 0, 0)

    private fun row(id: String): Row? = q.download(id).executeAsOneOrNull()

    private fun attached(): DownloadServer = server.value ?: throw IllegalStateException("no server attached")

    internal fun plan(row: Row): ServerDownload =
        // Migration (downloads): remove after every client has updated past this release.
        kivoJson.decodeFromString(ServerDownload.serializer(), withSizeExact(row.plan))

    /** [plan], or null for one stored in a shape this app no longer reads. */
    private fun readablePlan(row: Row): ServerDownload? = try {
        plan(row)
    } catch (_: SerializationException) {
        null
    }

    /**
     * Asks afresh for a download whose plan this app no longer reads, as
     * its Retry does, with the app's capabilities now; the server forgets
     * the old one. Files already here that the new plan names unchanged
     * are kept.
     */
    private suspend fun renew(row: Row) {
        val remote = row.remote_id
        if (remote != null) quietly { attached().connection.downloads.deleteDownload(remote).check() }
        reask(row)
    }

    private fun planJson(d: ServerDownload) = kivoJson.encodeToString(ServerDownload.serializer(), d)

    internal fun dir(id: String) = disk.dir(id)

    internal fun path(f: FileRow) = disk.path(f)

    internal fun files(id: String) = q.files(id).executeAsList()

    internal companion object {
        // A download's states, as stored.
        const val QUEUED = "queued"
        const val CONVERTING = "converting"
        const val DOWNLOADING = "downloading"
        const val READY = "ready"
        const val FAILED = "failed"
        const val PAUSED = "paused"

        // A file's.
        const val WAITING = "waiting"
        const val TRANSFERRING = "transferring"
        const val DONE = "done"

        // Its roles.
        const val PART = "part"
        const val SUBTITLE = "subtitle"
        const val FONT = "font"
        const val ARTWORK = "artwork"

        /** The server's reason for a download whose plan can't be read. */
        const val PLAN_UNREADABLE = "planUnreadable"

        /**
         * Downloads in the order they come here: as they were asked for,
         * and a show's episodes asked for together, each within [TOGETHER]
         * of the one before, in season and episode order.
         */
        fun inOrder(rows: List<Row>): List<Row> {
            val asked = rows.sortedBy { it.created_at }
            val batch = mutableMapOf<String, Long>()
            // Each series' latest batch: when it began, and when its last episode was asked for.
            val latest = mutableMapOf<String, Pair<Long, Long>>()
            for (row in asked) {
                val series = row.series_id
                if (series == null) {
                    batch[row.id] = row.created_at
                    continue
                }
                val last = latest[series]
                val began = if (last != null && row.created_at - last.second <= TOGETHER.inWholeMilliseconds) last.first else row.created_at
                latest[series] = began to row.created_at
                batch[row.id] = began
            }
            return asked.sortedWith(
                compareBy<Row>({ batch[it.id] }, { it.series_id.orEmpty() }, { it.season ?: 0 }, { it.episode ?: 0 }, { it.created_at }),
            )
        }

        /** The wait before a failed transfer starts again: 15 s, doubling to 5 minutes. */
        fun backoff(attempt: Int): Duration = TRANSFER_BACKOFF.wait(attempt)

        private val TRANSFER_BACKOFF = Backoff(15.seconds, 5.minutes)

        /** A subtitle file's name by its track and codec: `.vtt`, `.ass` or `.sup`. */
        fun subtitleName(trackId: String, codec: String) = "subtitle-$trackId." + when (codec) {
            "webvtt" -> "vtt"
            "hdmv_pgs_subtitle" -> "sup"
            else -> codec
        }

        fun fontName(url: String) = "font-" + url.substringAfterLast('/')

        /** A file's extension by its type, as AVPlayer and Media3 need it. */
        fun extension(contentType: String): String = when (contentType.substringBefore(';').trim()) {
            "video/mp4" -> "mp4"
            "audio/mp4" -> "m4a"
            "audio/mpeg" -> "mp3"
            "audio/aac" -> "aac"
            "audio/flac" -> "flac"
            "audio/ogg" -> "ogg"
            "video/x-matroska" -> "mkv"
            "video/webm" -> "webm"
            "video/mp2t" -> "ts"
            "video/x-msvideo" -> "avi"
            else -> "bin"
        }

        // The artwork files' names.
        const val ARTWORK_NAME = "artwork"
        const val POSTER_NAME = "poster"

        /** The work's artwork as its row shows it: an episode's still, else its poster, still or backdrop. */
        fun artwork(work: WorkDetail): String? {
            val images = work.images
            val first = if (work.episode != null) images.still ?: images.poster else images.poster ?: images.still
            return (first ?: images.backdrop)?.let { imagePath(it.id, 480) }
        }

        /** An episode's series poster, as its show's row shows it; null for anything else. */
        fun poster(work: WorkDetail): String? = work.episode?.let { work.images.poster }?.let { imagePath(it.id, 480) }
    }
}

/**
 * A signed-in server whose downloads [Downloads] looks after, with what this
 * app plays: [unreachable] while the app's reads get no answer from it, and
 * [awaitReachable], which returns once it answers again.
 */
internal class DownloadServer(
    val connection: ServerConnection,
    val capabilities: suspend () -> Capabilities,
    val unreachable: StateFlow<Boolean> = MutableStateFlow(false),
    val awaitReachable: suspend () -> Unit = {},
) {
    /** Returns once it answers: at once unless the app's reads are getting no answer from it. */
    suspend fun reached() {
        if (unreachable.value) quietly { awaitReachable() }
    }
}

/** What to download: a work, with the detail page's choices, at a quality (null: the download quality setting's). */
internal data class DownloadRequest(
    val workId: String,
    val versionId: String? = null,
    val audioTrackId: String? = null,
    /** A subtitle track's ID, or `off`; null: by the user's preferences. */
    val subtitleTrackId: String? = null,
    val quality: DownloadQuality? = null,
)

/** The server can't make a download for this app, for [reason], a playback reason code. */
internal class DownloadRefusedException(val reason: String) : Exception("download refused: $reason")

/** How soon after the one before a show's episode must be asked for to come in season and episode order with it. */
internal val TOGETHER: Duration = 1.minutes

private fun randomId(): String {
    val bytes = kotlin.random.Random.nextBytes(16)
    return bytes.joinToString("") { (it.toInt() and 0xff).toString(16).padStart(2, '0') }
}

/** A download's folder in [DownloadDisk.folder]. */
internal fun DownloadDisk.dir(id: String) = folder.toPath() / id

/** Where a download's file goes. */
internal fun DownloadDisk.path(f: FileRow) = dir(f.download_id) / f.name

/** A file's transfer, as the transport knows it: its download and name. */
internal val FileRow.transferId: String get() = "${download_id}/${name}"
