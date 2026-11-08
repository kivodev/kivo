package com.mmcdole.kivo.android

import android.content.Context
import androidx.annotation.RawRes
import com.mmcdole.kivo.android.discovery.NsdServiceBrowser
import com.mmcdole.kivo.android.discovery.servers
import com.mmcdole.kivo.android.library.SavedGrids
import com.mmcdole.kivo.android.player.SessionPlayers
import com.mmcdole.kivo.android.settings.ScreenSettingsStore
import com.mmcdole.kivo.core.auth.SavedServer
import com.mmcdole.kivo.core.discovery.FoundServer
import com.mmcdole.kivo.core.display.Display
import com.mmcdole.kivo.core.session.Device
import com.mmcdole.kivo.core.session.KivoClient
import com.mmcdole.kivo.core.session.KivoSession
import kotlinx.coroutines.MainScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlin.time.Duration.Companion.seconds

/** Where the app is: starting, signing in, unable to reach its server, or signed in to one. */
sealed interface AppState {
    data object Starting : AppState

    /** Signing in; to [server] by a device code when it's known (a session that ended), else from discovery. */
    data class SigningIn(val server: SavedServer? = null) : AppState

    /** The saved server answers at none of its addresses. */
    data class Unreachable(val server: SavedServer) : AppState

    data class SignedIn(val session: ServerSession) : AppState
}

/** What the app's build knows that its screens show: its [versionName], and the raw resource listing the [libraries] it's built from. */
class AppBuild(val versionName: String, @param:RawRes val libraries: Int)

/** The application, which holds the [AppGraph], for what runs outside its activity, such as a background worker. */
interface HasAppGraph {
    val graph: AppGraph
}

/**
 * Everything the app holds for its whole life: the core on this TV, the
 * browse for servers on the network, this TV's own settings and saved grids,
 * and where the app is. Screens get what they need from here and from [ServerSession];
 * the core never calls back into the app.
 */
class AppGraph(
    val context: Context,
    val build: AppBuild,
    device: Device = AndroidDevice(context, build.versionName),
    /** Told as the app opens a server, such as to fill the TV launcher's Watch Next row. */
    private val onOpened: () -> Unit = {},
    /** Told as the app leaves its server, signed out or choosing another. */
    private val onLeft: () -> Unit = {},
) {
    /** Work that outlives a screen. On the main thread. */
    val scope = MainScope()

    val core = KivoClient(device)

    /** This TV's name, such as "Living Room": its sessions' name. */
    val name: String = device.name

    val display: Display get() = core.display

    /** The library grids chosen on this TV, kept in the app's preferences. */
    val savedGrids = SavedGrids(device.preferences)

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

    private val mutableState = MutableStateFlow<AppState>(AppState.Starting)
    val state: StateFlow<AppState> = mutableState.asStateFlow()

    /** Opens the last server signed in to that this TV still has a session on, else signs in. */
    fun start() {
        scope.launch {
            val saved = core.lastSignedIn()
            if (saved == null) mutableState.value = AppState.SigningIn() else reach(saved)
        }
    }

    /** Opens [server] at the first of its addresses that answers as it. */
    fun reach(server: SavedServer) {
        mutableState.value = AppState.Starting
        scope.launch {
            var session = core.restore(server)
            // The network may still be waking as the app starts, so it's only unreachable once it's stayed so.
            repeat(REACH_RETRIES) {
                if (session.unreachable.value) {
                    session.close()
                    delay(REACH_RETRY_MS)
                    session = core.restore(server)
                }
            }
            when {
                session.unreachable.value -> {
                    session.close()
                    mutableState.value = AppState.Unreachable(server)
                }
                !session.isSignedIn -> {
                    session.close()
                    mutableState.value = AppState.SigningIn(server)
                }
                else -> open(session)
            }
        }
    }

    /** Signed in: shows [session]'s server until it's signed out, then signs in to it again. */
    suspend fun open(session: KivoSession) {
        val signedIn = ServerSession(this, session, savedGrids.all.first())
        mutableState.value = AppState.SignedIn(signedIn)
        onOpened()
        scope.launch {
            session.awaitSignOut()
            if (state.value == AppState.SignedIn(signedIn)) {
                signedIn.close()
                mutableState.value = AppState.SigningIn(session.savedServer.value)
                onLeft()
            }
        }
    }

    /** Leaves the signed-in server for the sign-in screens, choosing a server again. */
    fun changeServer() {
        (state.value as? AppState.SignedIn)?.session?.close()
        mutableState.value = AppState.SigningIn()
        onLeft()
    }
}

/** How long a browse for servers looks. */
private val FIND_TIME = 3.seconds

/** How many more times opening a server tries before saying it can't be reached, and how long it waits between. */
private const val REACH_RETRIES = 2
private const val REACH_RETRY_MS = 1500L
