package com.mmcdole.kivo.core.session

import com.mmcdole.kivo.core.Compatibility
import com.mmcdole.kivo.core.auth.SavedServer
import com.mmcdole.kivo.core.auth.SignedInUser

/** Where the app is ([KivoClient.state]), which its root shows: starting, signing in, unable to reach its server, or signed in to one. */
sealed interface AppState {
    /** Opening the server last signed in to, when there is one. */
    data object Starting : AppState

    /** Signing in: to [server] when the session on it ended, else to one the person chooses. */
    data class SignedOut(val server: SavedServer?) : AppState

    /**
     * The saved server answers at none of its addresses. With
     * [hasDownloads], the device keeps downloads of it, which play once
     * it's opened without it ([KivoClient.openOffline]).
     */
    data class Unreachable(val server: SavedServer, val hasDownloads: Boolean) : AppState

    /** Signed in to [session]'s server, until it's signed out or left for another. */
    data class SignedIn(val session: KivoSession) : AppState
}

/**
 * A session as its screens show it ([KivoSession.state]): the [server] as
 * saved, who's signed in ([user]), whether the server isn't answering
 * ([unreachable]), whether the app and it work together ([compatibility]),
 * and the base URL it's reached at ([address]).
 */
data class SessionState(
    val server: SavedServer,
    val user: SignedInUser?,
    val unreachable: Boolean,
    val compatibility: Compatibility,
    val address: String,
)
