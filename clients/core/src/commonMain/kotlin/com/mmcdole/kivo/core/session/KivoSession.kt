package com.mmcdole.kivo.core.session

import com.mmcdole.kivo.api.models.ServerInfo
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.Compatibility
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.auth.AddressChange
import com.mmcdole.kivo.core.auth.LinkDevice
import com.mmcdole.kivo.core.auth.SavedServer
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.auth.SignedInUser
import com.mmcdole.kivo.core.downloads.DownloadServer
import com.mmcdole.kivo.core.live.Invalidation
import com.mmcdole.kivo.core.live.LiveEvents
import com.mmcdole.kivo.core.settings.UserSettings
import io.ktor.client.HttpClient
import io.ktor.client.call.body
import io.ktor.client.request.get
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.FlowPreview
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.debounce
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.flow.filterIsInstance
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlin.time.Duration.Companion.minutes
import kotlin.time.Duration.Companion.seconds

/**
 * One server and this app's session on it, from opening it to signing out
 * or leaving it for another ([close]); every app has one at a time, made by
 * [KivoClient]. It holds where the server is reached and whether it answers
 * ([unreachable]), and what the app does with it, by area: the reads of its
 * libraries ([library]), the user's account ([account]), playing here
 * ([playback]), casting ([cast]), Link a device ([link]) and this server's
 * [downloads]. Its reads are Flows of [Load] that read again as the live
 * events say and ask again while the server can't be reached.
 *
 * A signed-out session stays as it was made, for the app to sign in again;
 * one that was signed in deletes this server's downloads as it signs out.
 *
 * [opened] is what the server said of itself as the session opened; null
 * when it didn't answer.
 */
class KivoSession internal constructor(
    saved: SavedServer,
    internal val connection: ServerConnection,
    internal val core: KivoClient,
    opened: ServerInfo? = null,
) {
    private val mutableServer = MutableStateFlow(saved)
    private val mutableCompatibility = MutableStateFlow(opened?.let(core::compatibility) ?: Compatibility.Compatible)

    /**
     * Whether this app and the server work together, by what the server
     * last said of itself ([serverInfo]): as the session opened, as the app
     * comes to the foreground, as the live events reconnect, which they do
     * when the server restarts on another version, and as the server
     * answers after it didn't. Compatible until the server first answers.
     * An app shows an incompatible server as needing an update, or itself,
     * in place of the server's screens, and reads [serverInfo] again as the
     * person tries again.
     */
    val compatibility: StateFlow<Compatibility> = mutableCompatibility.asStateFlow()

    /**
     * The server, as saved: its ID, name and addresses. Its name follows the
     * server's each time the server says what it is ([serverInfo]), as it
     * does when the app comes to the foreground ([runLiveEvents]) and when
     * it answers after it didn't, and the saved server follows it.
     */
    val savedServer: StateFlow<SavedServer> = mutableServer.asStateFlow()

    /** What the session runs while it's open: what it hears of the server, for itself and its players. */
    internal val scope = CoroutineScope(core.scope.coroutineContext + SupervisorJob(core.scope.coroutineContext[Job]))

    private val live = LiveEvents(connection)
    private val invalidations: Flow<Invalidation> = live.invalidations
    internal val reachability get() = connection.reachability
    internal val userSettings = UserSettings(connection)

    /** The user's plays that ended elsewhere: by an admin, another client or expiry. */
    internal val playsEnded: Flow<Invalidation.PlayEnded> = invalidations.filterIsInstance<Invalidation.PlayEnded>()

    /** The server's libraries as the user sees them, and the user's marks on works. */
    val library = SessionLibrary(connection, invalidations, reachability, scope, core.device.preferences)

    /** The signed-in user's account: their preferences, password and signed-in devices. */
    val account = SessionAccount(userSettings, core.autoplay, library, scope)

    /** The video and book players, the quality ladder, scrub previews and subtitle search. */
    val playback = SessionPlayback(this)

    /** Casting to a Cast device. */
    val cast = SessionCast(this)

    /** Link a device. */
    val link = SessionLink(LinkDevice(connection))

    /** This server's downloads on the device; null on a device that keeps none. */
    val downloads: SessionDownloads? = core.downloads?.let { SessionDownloads(it) }

    /**
     * Deletes this server's downloads once the session signs out, however it
     * does. It runs for the app's life rather than the session's, so a
     * session closed as it signs out still deletes them.
     */
    private val forgetOnSignOut: Job? = core.downloads?.takeIf { connection.signedIn.value }?.let { downloads ->
        downloads.attach(DownloadServer(connection, { core.report.report() }, unreachable, { awaitReachable() }))
        core.scope.launch {
            connection.signedIn.first { !it }
            downloads.forget(saved.id)
        }
    }

    // The device's network changing picks the server's address again.
    // The server answering after it didn't, as when it didn't as the session opened, reads its name again.
    init {
        scope.launch {
            reachability.unreachable.filter { it }.collect {
                reachability.unreachable.first { !it }
                readServerInfo()
            }
        }
        @OptIn(FlowPreview::class)
        scope.launch {
            core.networkChanges.debounce(NETWORK_SETTLE).collect { connection.address.repick(AddressChange.Network) }
        }
        // A plain-HTTP address isn't trusted on a new network: off it at once, to the server's HTTPS address when it has one.
        scope.launch {
            core.network.epoch.drop(1).collect { if (connection.signedIn.value) connection.address.untrust() }
        }
    }

    /**
     * The base URL this app reaches the server at, as it moves between the
     * server's addresses: a local one when one answers, else a public one,
     * picked again as the device's network changes, as a request gets no
     * answer, and as the app comes to the foreground. Requests and the live
     * events follow it, and media as a play starts, staying where the play
     * started for as long as it lasts; [baseUrl] is where it is now.
     *
     * The session's token goes only to an address that has proven, on the
     * device's network as it is now, that it's the server: an HTTPS one, or
     * a plain-HTTP one whose answer to a probe proves it. On a new network
     * a plain-HTTP address is left at once for the server's HTTPS one, when
     * it has one, until a probe proves it again.
     */
    val address: StateFlow<String> get() = connection.address.current

    /** The base URL this app reaches the server at now, which media and image URLs are made at. */
    val baseUrl: String get() = connection.baseUrl

    /**
     * The `Authorization` header for the app's own request to [url], such
     * as its image loader's or its player's: null when signed out, or when
     * [url] isn't at an address the session's token may go to now.
     */
    fun authorization(url: String): String? = connection.authorization(url)

    /** The HTTP client the session's requests go through, carrying its token, for a platform's image loader. */
    val http: HttpClient get() = connection.http

    val isSignedIn: Boolean get() = connection.signedIn.value

    /**
     * Who this app is signed in as, kept on the device so it shows without
     * the server. Each answer naming the user updates it
     * ([SessionAccount.currentUser], [awaitReachable], the settings), as does
     * each reconnect of the live events; null once signed out.
     */
    val user: StateFlow<SignedInUser?> get() = connection.user

    /** Whether the last read got no answer from the server; false once one is answered. */
    val unreachable: StateFlow<Boolean> get() = reachability.unreachable

    /** Asks again now, for the reads waiting to. */
    fun retryNow() = reachability.retryNow()

    /**
     * Returns once the server answers, asking as the Flows' reads do, so a
     * screen whose read failed can read again; throws when the answer is a
     * refusal.
     */
    @Throws(Exception::class)
    suspend fun awaitReachable() {
        reachability.untilAnswered { userSettings.current() }
    }

    /** Returns once this app is signed out: by the user, a `401`, or the live events closing with 4401. */
    @Throws(Exception::class)
    suspend fun awaitSignOut() {
        connection.signedIn.first { !it }
    }

    /**
     * Keeps the live events open until cancelled, as the app goes to the
     * background, or signed out; tells the downloads what they say, and
     * reads the user again after each reconnect. The app runs it in the
     * foreground, so coming back picks the server's address again and reads
     * what the server says of itself, as each reconnect does, and on a
     * public address a local one is looked for now and then and as a stream
     * stops playing, while none plays.
     */
    @Throws(Exception::class)
    suspend fun runLiveEvents() = coroutineScope {
        val downloads = core.downloads?.let { d -> launch { invalidations.collect { d.invalidated(it) } } }
        val user = launch { userSettings.refreshOn(invalidations) }
        val info = launch {
            launch { readServerInfo() }
            invalidations.filterIsInstance<Invalidation.Everything>().collect { readServerInfo() }
        }
        val address = launch {
            connection.address.repick(AddressChange.Foreground)
            launch { connection.address.streaming.drop(1).filter { !it }.collect { connection.address.preferLocal() } }
            while (true) {
                delay(LOCAL_CHECK_EVERY)
                connection.address.preferLocal()
            }
        }
        try {
            live.run()
        } finally {
            downloads?.cancel()
            user.cancel()
            info.cancel()
            address.cancel()
        }
    }

    /** Ends this app's session on the server where it can, forgets the token, and deletes this server's downloads. */
    @Throws(Exception::class)
    suspend fun signOut() {
        core.downloads?.forget(savedServer.value.id)
        connection.endSession()
    }

    /** Leaves the server, signed in or not, as the app signs out or opens another: what the session runs stops. */
    fun close() {
        playback.stopBook()
        if (isSignedIn) forgetOnSignOut?.cancel()
        scope.cancel()
    }

    /**
     * The server's name, version and addresses, as it says now; a new name
     * is saved ([savedServer]), and whether the app works with it follows
     * ([compatibility]).
     */
    @Throws(Exception::class)
    suspend fun serverInfo(): ServerInfo = connection.setup.getServer().result().also {
        mutableCompatibility.value = core.compatibility(it)
        renamed(it.name)
    }

    /** Reads what the server says of itself; one that doesn't answer stays as it last said. */
    private suspend fun readServerInfo() {
        try {
            serverInfo()
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            // Read again the next time the server answers.
        }
    }

    /** Saves the server's [name] when it has changed, while this app is signed in to it. */
    private suspend fun renamed(name: String) {
        val now = savedServer.value.copy(name = name)
        if (now == savedServer.value || !connection.signedIn.value) return
        mutableServer.value = now
        try {
            core.resave(now)
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            // Saved the next time the server says its name.
        }
    }

    /** A URL the API gives relative to its root, such as a play's media, made absolute; an absolute one stays as it is. */
    fun mediaUrl(relative: String): String = connection.apiUrl(relative)

    /** An image by ID at a width in pixels; the server serves the smallest stored width at least that. */
    fun imageUrl(id: String, width: Int): String = connection.imageUrl(id, width)

    /** What's at [url], one the API gave ([mediaUrl]) or an [imageUrl], fetched with this app's session: a font or artwork. */
    @Throws(Exception::class)
    suspend fun bytes(url: String): ByteArray = connection.http.get(mediaUrl(url)).body()
}

/** How long the device's network is let settle, as it changes in a burst, before the address is picked again. */
private val NETWORK_SETTLE = 1.seconds

/** How often the foreground app on a public address looks for a local one. */
private val LOCAL_CHECK_EVERY = 2.minutes
