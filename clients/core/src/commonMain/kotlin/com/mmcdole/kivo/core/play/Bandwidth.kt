package com.mmcdole.kivo.core.play

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.longPreferencesKey
import com.mmcdole.kivo.core.auth.ServerConnection
import io.ktor.client.request.parameter
import io.ktor.client.request.prepareGet
import io.ktor.client.statement.bodyAsChannel
import io.ktor.http.HttpStatusCode
import io.ktor.utils.io.readAvailable
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.TimeoutCancellationException
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.withTimeout
import kotlin.time.Clock
import kotlin.time.Duration
import kotlin.time.Duration.Companion.seconds
import kotlin.time.TimeSource

/**
 * The download rate Auto quality goes by, kept in the app's DataStore: the
 * last one measured away from the server's local networks within the hour,
 * else one timed download of the server's bandwidth test before the play,
 * which a client on a local network skips.
 */
internal class Bandwidth(
    private val store: DataStore<Preferences>,
    private val now: () -> Long = { Clock.System.now().toEpochMilliseconds() },
) {
    private val key = longPreferencesKey("kivo.bandwidth")
    private val atKey = longPreferencesKey("kivo.bandwidth.at")

    /** The rate measured within the hour, else one measured now from [connection]; null on a local network. */
    suspend fun forAuto(connection: ServerConnection): Long? =
        last() ?: measureBandwidth(connection)?.also { remember(it) }

    /** The rate measured within the hour, in bits per second. */
    suspend fun last(): Long? {
        val prefs = store.data.first()
        val at = prefs[atKey] ?: return null
        if (now() - at >= VALID_MS) return null
        return prefs[key]?.takeIf { it > 0 }
    }

    /** Keeps a rate measured away from the server's local networks, for the next hour's plays. */
    suspend fun remember(bps: Long) {
        store.edit {
            it[key] = bps
            it[atKey] = now()
        }
    }
}

/** How long a measured rate stands for the connection. */
private const val VALID_MS = 60 * 60 * 1000L

/** The test's size: a few seconds at the rates remote quality matters for. */
private const val TEST_BYTES = 5_000_000

/**
 * Times one download of the server's bandwidth test, stopping at [limit]
 * and counting what arrived; null for a client on the server's local
 * networks, which gets nothing, or when nothing arrived.
 */
internal suspend fun measureBandwidth(connection: ServerConnection, limit: Duration = 3.seconds): Long? {
    val started = TimeSource.Monotonic.markNow()
    var bytes = 0L
    try {
        withTimeout(limit) {
            connection.http.prepareGet(connection.apiUrl("bandwidth-test")) {
                parameter("size", TEST_BYTES)
            }.execute { response ->
                if (response.status != HttpStatusCode.OK) return@execute
                val body = response.bodyAsChannel()
                val buffer = ByteArray(64 * 1024)
                while (true) {
                    val read = body.readAvailable(buffer)
                    if (read < 0) break
                    bytes += read
                }
            }
        }
    } catch (_: TimeoutCancellationException) {
        // The deadline: what arrived counts.
    } catch (e: CancellationException) {
        throw e
    } catch (_: Exception) {
        // A failure: what arrived counts.
    }
    if (bytes == 0L) return null
    return bytes * 8 * 1000 / started.elapsedNow().inWholeMilliseconds.coerceAtLeast(1)
}
