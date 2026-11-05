package com.mmcdole.kivo.core.downloads

/**
 * The platform's background transfers: WorkManager on Android, a background
 * `URLSession` on Apple. They go on with the app closed, resume by range
 * where an earlier attempt stopped, and report through
 * [Downloads.transferred]. The core decides what to fetch, checks what
 * arrives, and starts again whatever isn't under way.
 */
interface DownloadTransport {
    /**
     * Starts [request], or resumes it from what an earlier attempt fetched,
     * at [TransferRequest.url] even when that attempt fetched from another.
     * One already under way is left alone, unless it was started under the
     * other [TransferRequest.cellular] rule or from another URL, as when the
     * server moved to another of its addresses: then it goes on under this
     * request, resuming what it fetched.
     */
    fun start(request: TransferRequest)

    /** Stops a transfer, keeping what it fetched, so starting it again resumes it. */
    fun stop(id: String)

    /** Stops a transfer and forgets what it fetched. */
    fun cancel(id: String)

    /** The transfers under way now, by id, as the platform keeps them across relaunches. */
    fun running(): List<String>
}

/**
 * One file to fetch: [url], with the app's [authorization], into
 * [destination], an absolute path. [cellular] says whether it may use mobile
 * data; without it, it waits for Wi-Fi.
 */
data class TransferRequest(
    val id: String,
    val url: String,
    val authorization: String?,
    val destination: String,
    val cellular: Boolean,
)

/** What a transfer tells the core, from the platform. */
sealed interface TransferEvent {
    val id: String

    /** [received] of [expected] bytes are here; [expected] is -1 when the server didn't say. */
    data class Progress(override val id: String, val received: Long, val expected: Long) : TransferEvent

    /** The whole file is at its destination, to be checked. */
    data class Finished(override val id: String) : TransferEvent

    /** It stopped: [status] is the server's HTTP status, or 0 when it wasn't reached. */
    data class Failed(override val id: String, val status: Int, val message: String) : TransferEvent
}
