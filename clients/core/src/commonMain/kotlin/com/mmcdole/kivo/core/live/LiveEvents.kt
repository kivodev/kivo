package com.mmcdole.kivo.core.live

import com.mmcdole.kivo.api.models.EventMessage
import com.mmcdole.kivo.api.models.EventStreamMessage
import com.mmcdole.kivo.core.Backoff
import com.mmcdole.kivo.core.auth.AddressChange
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.kivoJson
import io.ktor.client.plugins.websocket.webSocket
import io.ktor.websocket.CloseReason
import io.ktor.websocket.Frame
import io.ktor.websocket.readText
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.dropWhile
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.merge
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.serialization.SerializationException
import kotlin.time.Duration
import kotlin.time.Duration.Companion.seconds

/**
 * The client side of the events WebSocket. [run] keeps `/api/v1/events` open
 * with the session token while the app is in the foreground, reconnecting
 * with backoff after a dropped socket or the server going away (1001), and
 * publishes [invalidations] for each screen's view model to map to refetches.
 * Closing with 4401 signs the app out and stops it. Admin events are ignored:
 * the apps have no admin screens. [backoff] is the wait before each
 * reconnect: 1 s, doubling to 30 s, from the start again once a socket opens.
 * The socket is at the server's address in use: when it moves, or is no
 * longer trusted with the session, as on a new network, the socket opens
 * again at once, where the token may go, and a socket that can't be kept
 * has the address picked again before the wait.
 */
internal class LiveEvents(
    private val connection: ServerConnection,
    private val backoff: (attempt: Int) -> Duration = Backoff(1.seconds, 30.seconds)::wait,
) {
    private val mutableInvalidations = MutableSharedFlow<Invalidation>(extraBufferCapacity = 64)
    val invalidations: SharedFlow<Invalidation> = mutableInvalidations.asSharedFlow()

    /** Whether a socket has delivered a snapshot before: the next one means events may have been missed. */
    private var opened = false

    /**
     * Keeps the socket open until cancelled, as the app goes to the
     * background, or until the app is signed out. Running it again on
     * returning refetches everything once the socket opens.
     */
    suspend fun run() {
        var attempt = 0
        while (connection.signedIn.value) {
            val at = connection.baseUrl
            val close = try {
                untilMoved(at) { connect { attempt = 0 } }
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                null
            }
            if (close?.code == CLOSE_UNAUTHORIZED) {
                connection.signOut()
                return
            }
            if (!connection.signedIn.value) return
            if (connection.baseUrl != at) continue
            if (close == null && connection.address.repick(AddressChange.NoAnswer)) continue
            delay(backoff(attempt++))
        }
    }

    /** [socket]'s close, or null once the server's address moves from [at] or isn't trusted any more, which ends it. */
    private suspend fun untilMoved(at: String, socket: suspend () -> CloseReason?): CloseReason? = coroutineScope {
        val open = async { socket() }
        val moved = launch {
            // Trust lost once the socket's address had it, not the wait for it to be proven as the socket opens.
            val untrusted = connection.address.trusted.dropWhile { !it }.filter { !it }
            merge(connection.address.current.filter { it != at }.map { }, untrusted.map { }).first()
            open.cancel()
        }
        try {
            open.await()
        } catch (e: CancellationException) {
            ensureActive()
            if (!moved.isCompleted) throw e
            null
        } finally {
            moved.cancel()
        }
    }

    /** One socket, until it closes: how it closed, or null when it dropped without a close frame. */
    private suspend fun connect(onOpen: () -> Unit): CloseReason? {
        var reason: CloseReason? = null
        connection.http.webSocket(connection.apiUrl("events").replaceFirst("http", "ws")) {
            onOpen()
            var reconnect = opened
            for (frame in incoming) {
                if (frame !is Frame.Text) continue
                val message = try {
                    kivoJson.decodeFromString(EventStreamMessage.serializer(), frame.readText())
                } catch (_: SerializationException) {
                    continue
                }
                if (message is EventStreamMessage.Snapshot) {
                    if (reconnect) mutableInvalidations.emit(Invalidation.Everything)
                    opened = true
                    reconnect = false
                    continue
                }
                invalidationOf(message)?.let { mutableInvalidations.emit(it) }
            }
            reason = closeReason.await()
        }
        return reason
    }
}

/** What a screen refetches, from the live events. */
internal sealed interface Invalidation {
    /** Root works were added to or removed from these libraries: Home, the library list and their grids, keeping the place. */
    data class LibraryChanged(val libraryIds: List<String>) : Invalidation

    /** A work's detail changed: its summaries and its detail page. */
    data class WorkChanged(val workId: String) : Invalidation

    /** The user's state for these works changed, here or on another client: played and progress, and the Continue rows. */
    data class UserStateChanged(val workIds: List<String>) : Invalidation

    /** The user's play was stopped by an admin or another client, or expired, with the admin's [message] to the viewer if there is one. */
    data class PlayEnded(val playId: String, val message: String? = null) : Invalidation

    /** The user's downloads moved on the server: in line, in their preparation, or to ready or failed. */
    data object DownloadsChanged : Invalidation

    /** The user's Home layout changed on another client: Home and the editor. */
    data object HomeChanged : Invalidation

    /** Events may have been missed (a reconnect, a resync, a new server version): refetch everything visible. */
    data object Everything : Invalidation
}

/** WebSocket close code for a session that is no longer valid: sign out, don't reconnect. */
internal const val CLOSE_UNAUTHORIZED: Short = 4401

/** What a frame invalidates; null for snapshots, keep-alives, admin events and anything unknown. */
internal fun invalidationOf(message: EventStreamMessage): Invalidation? = when (message) {
    is EventStreamMessage.Resync -> Invalidation.Everything
    is EventStreamMessage.Event -> invalidationOf(message.message)
    is EventStreamMessage.Snapshot, is EventStreamMessage.KeepAlive, EventStreamMessage.Unknown -> null
}

private fun invalidationOf(event: EventMessage): Invalidation? = when (event.kind) {
    "libraryChanged" -> Invalidation.LibraryChanged(event.libraryIds.orEmpty())
    "workChanged" -> event.workId?.let { Invalidation.WorkChanged(it) }
    "userStateChanged" -> Invalidation.UserStateChanged(event.workIds.orEmpty())
    "playEnded" -> event.playId?.let { Invalidation.PlayEnded(it, event.message) }
    "downloadsChanged" -> Invalidation.DownloadsChanged
    "homeChanged" -> Invalidation.HomeChanged
    else -> null
}
