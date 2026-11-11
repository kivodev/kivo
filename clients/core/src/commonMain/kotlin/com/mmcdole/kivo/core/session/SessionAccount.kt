package com.mmcdole.kivo.core.session

import com.mmcdole.kivo.api.models.ClientPlatform
import com.mmcdole.kivo.api.models.CurrentUser
import com.mmcdole.kivo.api.models.LibraryChoice
import com.mmcdole.kivo.api.models.PlaybackPreferencesUpdate
import com.mmcdole.kivo.api.models.SessionKind
import com.mmcdole.kivo.api.models.SubtitleMode
import com.mmcdole.kivo.core.Done
import com.mmcdole.kivo.core.done
import com.mmcdole.kivo.core.player.Autoplay
import com.mmcdole.kivo.core.settings.UserSettings
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch

/**
 * The signed-in user's account on the server ([KivoSession.account]): who
 * they are, their playback preferences and libraries' order, their password,
 * and where they're signed in.
 */
class SessionAccount internal constructor(
    private val userSettings: UserSettings,
    private val autoplay: Autoplay,
    private val library: SessionLibrary,
    private val scope: CoroutineScope,
) {
    @Throws(Exception::class)
    suspend fun currentUser(): CurrentUser = userSettings.current()

    /** Changes the audio and subtitle preferences [update] gives, leaving the rest; up next in every player follows its autoplay. */
    @Throws(CancellationException::class)
    suspend fun updatePlayback(update: PlaybackPreferencesUpdate): Done<CurrentUser> =
        done { userSettings.updatePlayback(update).also(::follow) }

    /** The preferred audio language (BCP 47), or null for the original language. */
    @Throws(CancellationException::class)
    suspend fun setAudioLanguage(language: String?): Done<CurrentUser> =
        updatePlayback(PlaybackPreferencesUpdate(audioLanguage = language.orEmpty()))

    /** `none`, `forced`, `smart` or `always`. */
    @Throws(CancellationException::class)
    suspend fun setSubtitleMode(mode: SubtitleMode): Done<CurrentUser> = updatePlayback(PlaybackPreferencesUpdate(subtitleMode = mode))

    /** The preferred subtitle language (BCP 47), or null for the server's metadata language. */
    @Throws(CancellationException::class)
    suspend fun setSubtitleLanguage(language: String?): Done<CurrentUser> =
        updatePlayback(PlaybackPreferencesUpdate(subtitleLanguage = language.orEmpty()))

    @Throws(CancellationException::class)
    suspend fun setRememberTracks(remember: Boolean): Done<CurrentUser> =
        updatePlayback(PlaybackPreferencesUpdate(rememberTracks = remember))

    /** Whether up next plays the next episode by itself. */
    @Throws(CancellationException::class)
    suspend fun setAutoplayNext(autoplay: Boolean): Done<CurrentUser> =
        updatePlayback(PlaybackPreferencesUpdate(autoplayNext = autoplay))

    /** Up next plays on by itself as [user]'s preferences say, in every player of the app. */
    private fun follow(user: CurrentUser) {
        autoplay.enabled = user.playback.autoplayNext
    }

    /** Reads the user again for up next, as its autoplay may have changed on another device; a failed read keeps the last. */
    internal fun readAutoplay() {
        scope.launch {
            try {
                follow(currentUser())
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                // Up next keeps the preference last read.
            }
        }
    }

    /** The libraries' order and which are pinned, every library the user sees; [SessionLibrary.libraries] reads them again. */
    @Throws(CancellationException::class)
    suspend fun setLibraries(choices: List<LibraryChoice>): Done<CurrentUser> =
        done { userSettings.updateLibraries(choices).also { library.rereadLibraries() } }

    /**
     * Changes the user's password, keeping this app signed in; with
     * [signOutOthers], every other browser and app of the user is signed out.
     * A wrong [current] password is a refusal, never a sign-out.
     */
    @Throws(CancellationException::class)
    suspend fun changePassword(current: String, password: String, signOutOthers: Boolean): Done<Unit> =
        done { userSettings.changePassword(current, password, signOutOthers) }

    /** Where the user is signed in, most recently used first. */
    @Throws(Exception::class)
    suspend fun sessions(): List<SignedInDevice> = userSettings.sessions().map {
        SignedInDevice(it.id, it.name, it.kind, it.device?.platform, it.lastUsedAt.toEpochMilliseconds(), it.current)
    }

    @Throws(CancellationException::class)
    suspend fun signOutSession(id: String): Done<Unit> = done { userSettings.signOut(id) }

    /** Signs every other session of the user out, keeping this app's. */
    @Throws(CancellationException::class)
    suspend fun signOutEverywhereElse(): Done<Unit> = done { userSettings.signOutEverywhereElse() }
}

/** One of the browsers and apps the user is signed in on, as "Signed-in devices" lists them. */
data class SignedInDevice(
    val id: String,
    val name: String,
    val kind: SessionKind,
    /** Null when the client gave none. */
    val platform: ClientPlatform?,
    val lastUsedEpochMs: Long,
    /** This app's own. */
    val current: Boolean,
)
