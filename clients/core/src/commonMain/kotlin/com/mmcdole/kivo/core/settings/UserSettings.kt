package com.mmcdole.kivo.core.settings

import com.mmcdole.kivo.api.apis.SessionsApi
import com.mmcdole.kivo.api.check
import com.mmcdole.kivo.api.models.CurrentUser
import com.mmcdole.kivo.api.models.CurrentUserPasswordChange
import com.mmcdole.kivo.api.models.CurrentUserUpdate
import com.mmcdole.kivo.api.models.LibraryChoice
import com.mmcdole.kivo.api.models.PlaybackPreferencesUpdate
import com.mmcdole.kivo.api.models.Session
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.live.Invalidation
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.Flow

/**
 * The user's own settings, kept on the server, and the sessions the user is
 * signed in with. Each answer naming the user updates who the connection
 * keeps as signed in.
 */
internal class UserSettings(private val connection: ServerConnection) {
    suspend fun current(): CurrentUser = connection.users.getCurrentUser().result().also { connection.keep(it) }

    /** Changes the audio and subtitle preferences given, leaving the rest. */
    suspend fun updatePlayback(playback: PlaybackPreferencesUpdate): CurrentUser =
        connection.users.updateCurrentUser(CurrentUserUpdate(playback = playback)).result().also { connection.keep(it) }

    /** Sets the libraries' order and which are pinned, replacing the last. */
    suspend fun updateLibraries(libraries: List<LibraryChoice>): CurrentUser =
        connection.users.updateCurrentUser(CurrentUserUpdate(libraries = libraries)).result().also { connection.keep(it) }

    /**
     * Reads the user again each time [invalidations] say everything may have
     * changed, as after a reconnect, until cancelled; a read that fails waits
     * for the next.
     */
    suspend fun refreshOn(invalidations: Flow<Invalidation>) {
        invalidations.collect {
            if (it != Invalidation.Everything) return@collect
            try {
                current()
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                // The stored user stays until a read is answered.
            }
        }
    }

    /**
     * Changes the user's password, given the current one; with
     * [signOutOthers], every other session of the user is signed out. This
     * session stays signed in, also when [current] is wrong.
     */
    suspend fun changePassword(current: String, new: String, signOutOthers: Boolean) {
        connection.users.changeCurrentUserPassword(CurrentUserPasswordChange(current, new, signOutOthers)).check()
    }

    /** Where the user is signed in: its browsers and apps, most recently used first; `current` marks this one. */
    suspend fun sessions(): List<Session> = connection.sessions.listSessions().result().items

    /** Signs another session out: its next request fails and its live events close. */
    suspend fun signOut(id: String) {
        connection.sessions.deleteSession(id).check()
    }

    /** Signs every other session of the user out, keeping this one; API keys stay. */
    suspend fun signOutEverywhereElse() {
        connection.sessions.deleteOtherSessions(SessionsApi.ExceptDeleteOtherSessions.current).check()
    }
}
