package com.mmcdole.kivo.android.downloads

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.File
import java.io.FileOutputStream
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL
import kotlinx.coroutines.currentCoroutineContext

/** How a fetch ended: the whole file at its destination, or stopped with the server's [status], 0 when it wasn't reached. */
sealed interface FetchEnd {
    data object Whole : FetchEnd

    data class Stopped(val status: Int, val message: String) : FetchEnd
}

/**
 * Fetches [url] into [destination], going on from what an earlier fetch
 * left beside it (`.part`) by an HTTP range, as long as the file's ETag is
 * the one that fetch began with; a server that sends the whole file again
 * starts it over. Only a whole file is moved to [destination]; one cut short,
 * by a drop or by cancelling, keeps what arrived for the next fetch.
 * [progress] hears the bytes here and the file's size (-1 while unknown),
 * at most every [PROGRESS_MS] and once more at the end.
 */
class RangeFetch(
    private val url: String,
    private val authorization: String?,
    private val destination: File,
    private val progress: suspend (received: Long, expected: Long) -> Unit,
) {
    private val part = partOf(destination)
    private val etag = File(destination.path + ".etag")

    suspend fun run(): FetchEnd = withContext(Dispatchers.IO) {
        val offset = if (part.exists()) part.length() else 0
        val connection = (URL(url).openConnection() as HttpURLConnection).apply {
            connectTimeout = TIMEOUT_MS
            readTimeout = TIMEOUT_MS
            authorization?.let { setRequestProperty("Authorization", it) }
            if (offset > 0) {
                setRequestProperty("Range", "bytes=$offset-")
                etag.takeIf { it.exists() }?.readText()?.let { setRequestProperty("If-Range", it) }
            }
        }
        // Cancelling closes the connection, so a read that waits on it ends at once.
        val closing = launch {
            try {
                awaitCancellation()
            } finally {
                connection.disconnect()
            }
        }
        try {
            fetch(connection, offset)
        } catch (e: IOException) {
            ensureActive()
            FetchEnd.Stopped(0, e.message ?: e.javaClass.simpleName)
        } finally {
            closing.cancel()
            connection.disconnect()
        }
    }

    private suspend fun fetch(connection: HttpURLConnection, offset: Long): FetchEnd {
        val status = connection.responseCode
        val resumed = status == HttpURLConnection.HTTP_PARTIAL && offset > 0
        if (status != HttpURLConnection.HTTP_OK && !resumed) return FetchEnd.Stopped(status, connection.responseMessage.orEmpty())
        val start = if (resumed) offset else 0
        if (!resumed) {
            part.parentFile?.mkdirs()
            connection.getHeaderField("ETag")?.let(etag::writeText) ?: etag.delete()
        }
        val length = connection.contentLengthLong
        val expected = if (length < 0) -1 else start + length
        var received = start
        var reported = 0L
        connection.inputStream.use { input ->
            FileOutputStream(part, resumed).use { output ->
                val buffer = ByteArray(BUFFER)
                while (true) {
                    currentCoroutineContext().ensureActive()
                    val read = input.read(buffer)
                    if (read < 0) break
                    output.write(buffer, 0, read)
                    received += read
                    val now = System.currentTimeMillis()
                    if (now - reported >= PROGRESS_MS) {
                        reported = now
                        progress(received, expected)
                    }
                }
                output.fd.sync()
            }
        }
        progress(received, expected)
        if (expected >= 0 && received != expected) return FetchEnd.Stopped(0, "ended at $received of $expected bytes")
        currentCoroutineContext().ensureActive()
        destination.delete()
        if (!part.renameTo(destination)) throw IOException("couldn't move ${part.name} into place")
        etag.delete()
        return FetchEnd.Whole
    }

    companion object {
        /** What has arrived of [destination] so far. */
        fun partOf(destination: File) = File(destination.path + ".part")

        /** Forgets what arrived of [destination], so its next fetch starts over. */
        fun discard(destination: File) {
            partOf(destination).delete()
            File(destination.path + ".etag").delete()
        }

        const val PROGRESS_MS = 500L
        private const val TIMEOUT_MS = 30_000
        private const val BUFFER = 64 * 1024
    }
}
