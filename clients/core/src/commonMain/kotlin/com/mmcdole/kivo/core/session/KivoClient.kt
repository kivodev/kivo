package com.mmcdole.kivo.core.session

import com.mmcdole.kivo.api.models.ServerInfo
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.Compatibility
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.auth.ClientId
import com.mmcdole.kivo.core.auth.LinkState
import com.mmcdole.kivo.core.auth.NetworkEpoch
import com.mmcdole.kivo.core.auth.isLinkLocal
import com.mmcdole.kivo.core.auth.probeClient
import com.mmcdole.kivo.core.auth.SavedServer
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.auth.ServerList
import com.mmcdole.kivo.core.auth.SignIn
import com.mmcdole.kivo.core.caps.CapabilityReport
import com.mmcdole.kivo.core.discovery.Discovery
import com.mmcdole.kivo.core.discovery.FoundAddress
import com.mmcdole.kivo.core.discovery.FoundServer

import com.mmcdole.kivo.core.discovery.serviceUrl
import com.mmcdole.kivo.core.display.Display
import com.mmcdole.kivo.core.downloads.Downloads
import com.mmcdole.kivo.core.downloads.NetworkPath
import com.mmcdole.kivo.core.downloads.TransferEvent
import com.mmcdole.kivo.core.downloads.db.DownloadDatabase
import com.mmcdole.kivo.core.play.Bandwidth
import com.mmcdole.kivo.core.player.Autoplay
import com.mmcdole.kivo.core.settings.DeviceSettings
import com.mmcdole.kivo.core.settings.DeviceSettingsStore
import com.mmcdole.kivo.core.settings.DownloadQuality
import com.mmcdole.kivo.core.settings.Quality

import io.ktor.client.engine.HttpClientEngine
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.MainScope
import kotlinx.coroutines.channels.BufferOverflow
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.flow.emitAll
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.map
import kotlin.coroutines.CoroutineContext
import kotlin.coroutines.EmptyCoroutineContext

/**
 * The core as an app holds it, one for the app's life, on the [device] it
 * runs on: the servers it has signed in to and signing in to one, this
 * device's settings, its downloads and display tables. Each server it opens
 * is a [KivoSession].
 *
 * What it gives the apps keeps to what SKIE bridges safely: every suspend
 * function declares `@Throws`, so a failure reaches Swift as an error rather
 * than ending the app, and no Flow an app collects can fail, since a failure
 * escaping a Flow ends a Swift app.
 *
 * Its work runs on the main thread, for the app's life ([scope]); a test
 * gives its own scope, an HTTP [engine] in place of the platform's, and a
 * [clock] for the video player's waits, such as a test dispatcher's virtual
 * time, in place of the main thread's.
 */
class KivoClient internal constructor(
    internal val device: Device,
    private val engine: HttpClientEngine?,
    internal val scope: CoroutineScope,
    internal val clock: CoroutineContext = EmptyCoroutineContext,
) {
    /** The core on [device], over the platform's HTTP client. */
    constructor(device: Device) : this(device, null, MainScope())

    /** The core on [device] over [engine], as tests give Ktor's mock engine. */
    constructor(device: Device, engine: HttpClientEngine) : this(device, engine, MainScope())

    /** The core on [device] over [engine], with the video player's waits on [clock], as UI tests give a test dispatcher. */
    constructor(device: Device, engine: HttpClientEngine, clock: CoroutineContext) : this(device, engine, MainScope(), clock)

    /** Up next's run of episodes played on without input, across the app's players. */
    internal val autoplay = Autoplay()

    private val preferences = device.preferences
    private val serverList = ServerList(preferences)
    private val deviceSettings = DeviceSettingsStore(preferences, DownloadQuality.defaultOn(device.platform))
    private val clientIds = ClientId(preferences)

    /** Asks addresses who's there, for finding servers and every connection's probes, for the app's life. */
    private val probes = probeClient(engine, device.identity)
    private val discovery = Discovery(probes)

    internal val report = CapabilityReport(device.capabilities) { settingsNow().audio }

    /** The download rate Auto quality goes by. */
    internal val bandwidth = Bandwidth(preferences)

    /** The device's downloads, of whichever server is signed in; null on a device without them. */
    internal val downloads: Downloads? = device.downloads?.let {
        Downloads(DownloadDatabase(it.driver), it.disk, it.transport, ::settingsNow, scope)
    }

    /** The servers this app has signed in to, the last signed in to last. */
    val servers: Flow<List<SavedServer>> = serverList.servers.catch { emit(emptyList()) }

    /** This device's settings ("This device" in Settings), defaults where unset. */
    val settings: Flow<DeviceSettings> = deviceSettings.values.catch { emit(DeviceSettings()) }

    internal suspend fun settingsNow(): DeviceSettings = settings.first()

    internal suspend fun clientId(): String = clientIds.get()

    /** The display tables. */
    val display: Display = Display()

    /** From the platform's background transfers: how a transfer is going. Any thread. */
    fun transferChanged(event: TransferEvent) {
        downloads?.transferred(event)
    }

    /** The device's network attachments: a plain-HTTP address is trusted with a session only on the one it was proven on. */
    internal val network = NetworkEpoch()

    /** The device's network changed, and it has one: the open session picks its server's address again. */
    internal val networkChanges = MutableSharedFlow<Unit>(extraBufferCapacity = 1, onBufferOverflow = BufferOverflow.DROP_OLDEST)

    /**
     * From the platform, each time the device's network changes, as to
     * another Wi-Fi or a VPN: whether the device has a network, and whether
     * it's cellular or another expensive one. Any thread.
     */
    fun networkChanged(online: Boolean, expensive: Boolean) {
        // First, so no request after this carries a token to an address proven on the network before.
        network.changed()
        downloads?.network(NetworkPath(online, expensive))
        if (online) networkChanges.tryEmit(Unit)
    }


    /**
     * A server the platform's DNS-SD browser resolved: at the first of
     * [hosts] on [port] whose `GET /api/v1/server` answers with the [id] its
     * TXT record names, over HTTPS when [tls], under [path]; null when none
     * does.
     */
    @Throws(Exception::class)
    suspend fun confirmDiscovered(id: String, hosts: List<String>, port: Int, tls: Boolean, path: String): FoundServer? =
        discovery.confirm(FoundAddress(id, hosts.map { serviceUrl(it, port, tls, path) }))

    /** The server at a typed address, such as `192.168.1.20` or `https://kivo.example.com`. */
    @Throws(Exception::class)
    suspend fun serverAt(address: String): FoundServer = discovery.serverAt(address)

    /** A saved server at a local address that answers as it, else a public one; null when none does. */
    @Throws(Exception::class)
    suspend fun reach(server: SavedServer): FoundServer? = discovery.reach(server)

    /**
     * Whether this app and the server that says [info] of itself work
     * together: what an app asks of a server it found before it offers
     * sign-in, and again as the person tries again.
     */
    fun compatibility(info: ServerInfo): Compatibility =
        Compatibility.of(info.version, info.minAppVersion, device.identity.appVersion)

    /** The last server signed in to that this device still holds a session for; null when there's none. */
    @Throws(Exception::class)
    suspend fun lastSignedIn(): SavedServer? =
        servers.first().lastOrNull { ServerConnection.holdsSession(device.secureStore, it.id) }

    /**
     * A saved server, with this device's session on it restored when one is
     * held ([KivoSession.isSignedIn]). It's opened at the address [reach]
     * would pick, a plain-HTTP one only when it proves itself to the
     * session, and kept with the addresses it gives now; else at its first,
     * as [KivoSession.unreachable].
     */
    @Throws(Exception::class)
    suspend fun restore(server: SavedServer): KivoSession {
        val addresses = server.addresses.filterNot(::isLinkLocal).ifEmpty { server.addresses }
        val connection = connection(server.id, addresses.first(), addresses)
        connection.restore()
        val found = connection.reach()?.let { (at, info) -> FoundServer(at, info) }
        val saved = if (found != null && connection.signedIn.value) save(connection, found.info) else server
        val session = KivoSession(saved, connection, this, found?.info)
        if (found == null) connection.reachability.unanswered()
        return session
    }

    /**
     * Signs in to [found] with a password, as a session named [name], and
     * saves the server; a wrong password is a [RequestProblem.Refused].
     */
    @Throws(Exception::class)
    suspend fun signIn(found: FoundServer, username: String, password: String, name: String): KivoSession {
        val connection = connection(found.info.id, found.baseUrl)
        SignIn(connection, name, clientId()).withPassword(username, password)
        return signedIn(found, connection)
    }

    /**
     * Signs in to [found] by linking, as a session named [name]: the
     * code to show while it waits, then the signed-in session, with the
     * server saved as [signIn] saves it, or why it ended. Collecting again
     * starts a new code. It never fails: a failure is
     * [LinkProgress.Failed].
     */
    fun signInByLink(found: FoundServer, name: String): Flow<LinkProgress> = flow {
        val connection = connection(found.info.id, found.baseUrl)
        val states = SignIn(connection, name, clientId()).link().map { state ->
            when (state) {
                is LinkState.Waiting ->
                    LinkProgress.Waiting(state.code, state.linkUrl, state.expiresAt.toEpochMilliseconds())
                is LinkState.SignedIn -> LinkProgress.SignedIn(signedIn(found, connection))
                LinkState.Denied -> LinkProgress.Denied
                LinkState.Expired -> LinkProgress.Expired
            }
        }
        emitAll(states)
    }.catch { e ->
        if (e is CancellationException) throw e
        emit(LinkProgress.Failed(RequestProblem.of(e)))
    }

    private fun connection(serverId: String, baseUrl: String, addresses: List<String> = listOf(baseUrl)) =
        ServerConnection(
            serverId, baseUrl, device.secureStore, preferences, engine,
            addresses = addresses, network = network, identity = device.identity, probes = probes,
        )

    /** The session [connection] just signed in as, with its server saved ([save]); as [found] said it is when it doesn't answer again. */
    private suspend fun signedIn(found: FoundServer, connection: ServerConnection): KivoSession =
        KivoSession(save(connection, found.info), connection, this, found.info)

    /**
     * Saves the server at [connection] as the server last signed in to, with
     * the addresses it gives a signed-in app: its local URLs, the address it
     * answered at, then its public URL. When it doesn't answer, as
     * [otherwise] says it is.
     */
    private suspend fun save(connection: ServerConnection, otherwise: ServerInfo): SavedServer {
        val info = try {
            connection.setup.getServer().result()
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            otherwise
        }
        val saved = SavedServer(info, connection.baseUrl)
        serverList.save(saved)
        connection.address.remember(saved.addresses)
        return saved
    }

    /** Saves [server] again, as its session hears it's changed. */
    internal suspend fun resave(server: SavedServer) = serverList.save(server)

    /** Saves [transform] of this device's settings; downloads under way go on under the new ones at once. */
    @Throws(Exception::class)
    suspend fun updateSettings(transform: (DeviceSettings) -> DeviceSettings) {
        deviceSettings.update(transform)
        downloads?.settingsChanged()
    }

    @Throws(Exception::class)
    suspend fun setLocalQuality(quality: Quality) = updateSettings { it.copy(localQuality = quality) }

    @Throws(Exception::class)
    suspend fun setRemoteQuality(quality: Quality) = updateSettings { it.copy(remoteQuality = quality) }

    /** Maximum audio channels: 2, 6 or 8, or 0 for Auto. */
    @Throws(Exception::class)
    suspend fun setMaxAudioChannels(channels: Int) =
        updateSettings { it.copy(audio = it.audio.copy(maxChannels = channels.takeIf { n -> n > 0 })) }

    @Throws(Exception::class)
    suspend fun setDialogueBoost(on: Boolean) = updateSettings { it.copy(dialogueBoost = on) }


    @Throws(Exception::class)
    suspend fun setDownloadQuality(quality: DownloadQuality) = updateSettings { it.copy(downloadQuality = quality) }

    /** Allows cellular downloads or no longer does; the downloads under way go on under the new rule at once. */
    @Throws(Exception::class)
    suspend fun setCellularDownloads(allowed: Boolean) = updateSettings { it.copy(cellularDownloads = allowed) }
}

/** Where signing in by linking is ([KivoClient.signInByLink]). */
sealed interface LinkProgress {
    /** Show [code], and [linkUrl] as a QR code, until [expiresAtEpochMs]. */
    data class Waiting(val code: String, val linkUrl: String, val expiresAtEpochMs: Long) : LinkProgress

    /** Approved: the app is signed in, as [session]. */
    data class SignedIn(val session: KivoSession) : LinkProgress

    /** Declined on the other device. */
    data object Denied : LinkProgress

    /** The code expired unanswered. */
    data object Expired : LinkProgress

    /** The link couldn't be started or followed, for [problem]. */
    data class Failed(val problem: RequestProblem) : LinkProgress
}
