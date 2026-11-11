package com.mmcdole.kivo.android

import android.content.Context
import androidx.annotation.RawRes
import com.mmcdole.kivo.android.discovery.NsdServiceBrowser
import com.mmcdole.kivo.android.downloads.DownloadRefresh
import com.mmcdole.kivo.android.downloads.WorkTransport
import com.mmcdole.kivo.android.discovery.servers
import com.mmcdole.kivo.android.player.SessionPlayers
import com.mmcdole.kivo.android.settings.ScreenSettingsStore
import com.mmcdole.kivo.core.discovery.FoundServer
import com.mmcdole.kivo.core.display.Display
import com.mmcdole.kivo.core.downloads.DownloadStatus
import com.mmcdole.kivo.core.session.AppState
import com.mmcdole.kivo.core.session.Device
import com.mmcdole.kivo.core.session.KivoClient
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.MainScope
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlin.time.Duration.Companion.seconds

/**
 * What the app's build knows: the [versionName] its screens show, the [version] its requests name, a semantic version
 * such as `0.3.0+12`, and the raw resource listing the [libraries] it's built from.
 */
class AppBuild(val versionName: String, val version: String, @param:RawRes val libraries: Int)

/** The application, which holds the [AppGraph], for what runs outside its activity, such as a background worker. */
interface HasAppGraph {
    val graph: AppGraph
}

/**
 * Everything the app holds for its whole life: the core on this device, the
 * browse for servers on the network, a phone's download transfers, this
 * device's own settings, and the signed-in server as the screens keep it
 * ([session]). Where the app is, is the core's to say
 * ([KivoClient.state]); the core never calls back into the app.
 */
class AppGraph(
    val context: Context,
    val build: AppBuild,
    device: Device = AndroidDevice(context, build.version),
    /** Told as the app opens a server, such as to fill the TV launcher's Watch Next row. */
    private val onOpened: () -> Unit = {},
    /** Told as the app leaves its server, signed out or choosing another. */
    private val onLeft: () -> Unit = {},
) {
    /** Work that outlives a screen. On the main thread. */
    val scope = MainScope()

    val core = KivoClient(device)

    /** A phone's download transfers, which tell the core how they go; null on a TV. */
    val transport: WorkTransport? = (device as? AndroidDevice)?.transport?.also { transport ->
        transport.events = core::transferChanged
        transport.titleOf = { id -> session.value?.downloads?.value?.firstOrNull { it.id == id }?.title }
    }

    /** This TV's name, such as "Living Room": its sessions' name. */
    val name: String = device.name

    val display: Display get() = core.display

    /** This TV's settings that only the app's screens follow, kept in the app's preferences. */
    val screenSettings = ScreenSettingsStore(device.preferences)

    /** The player the phone's media session controls, which the app's playback service follows. */
    val sessionPlayers = SessionPlayers(scope)

    /** Browses for servers on the local network by DNS-SD. */
    private val browser = NsdServiceBrowser(context)

    /** Each server on the local network that answers as itself, once each, as found within a few seconds. */
    fun findServers(): Flow<FoundServer> = browser.servers(core, FIND_TIME)

    /** A work a link asked to open, such as a Watch Next program's, until the signed-in screens show it. */
    val opening = MutableStateFlow<String?>(null)

    /** A `kivo://link` link to answer in a phone's Link a device, until it shows it. */
    val linking = MutableStateFlow<String?>(null)

    private val mutableSession = MutableStateFlow<ServerSession?>(null)

    /** The signed-in server as the screens keep it, while the core is signed in to one; made as it opens and closed as it's left. */
    val session: StateFlow<ServerSession?> = mutableSession.asStateFlow()

    init {
        // Without a dispatch, so the screens have their server by the frame that shows the app signed in.
        scope.launch(Dispatchers.Main.immediate) {
            var before: AppState = AppState.Starting
            core.state.collect { state ->
                val signedIn = (state as? AppState.SignedIn)?.session
                if (mutableSession.value?.session !== signedIn) {
                    mutableSession.value?.close()
                    mutableSession.value = signedIn?.let { ServerSession(this@AppGraph, it) }
                }
                when {
                    state is AppState.SignedIn -> onOpened()
                    state is AppState.SignedOut && before != AppState.Starting -> onLeft()
                }
                before = state
            }
        }
    }

    /**
     * The app left the foreground: with a download the server is still
     * preparing, the downloads' refresh is asked for, to start it once it's ready.
     */
    fun leftForeground() {
        val downloads = session.value?.downloads?.value ?: return
        if (downloads.any { it.status is DownloadStatus.Queued || it.status is DownloadStatus.Converting }) DownloadRefresh.schedule(context)
    }

    /** Leaves the signed-in server for the sign-in screens, choosing a server again. */
    fun changeServer() = core.changeServer()
}

/** How long a browse for servers looks. */
private val FIND_TIME = 3.seconds
