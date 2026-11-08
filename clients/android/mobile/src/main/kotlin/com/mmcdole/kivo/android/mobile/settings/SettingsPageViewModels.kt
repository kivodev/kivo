package com.mmcdole.kivo.android.mobile.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.android.mobile.ui.display
import com.mmcdole.kivo.android.mobile.ui.sentence
import com.mmcdole.kivo.android.settings.ScreenSettings
import com.mmcdole.kivo.android.settings.ScreenSettingsStore
import com.mmcdole.kivo.android.settings.SubtitleBackground
import com.mmcdole.kivo.android.settings.SubtitleSize
import com.mmcdole.kivo.api.ProblemException
import com.mmcdole.kivo.api.models.CurrentUser
import com.mmcdole.kivo.android.mobile.home.shownKinds
import com.mmcdole.kivo.api.models.HomeRowChange
import com.mmcdole.kivo.api.models.HomeRowInfo
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.LibraryChoice
import com.mmcdole.kivo.api.models.SubtitleMode
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.display.QualityOption
import com.mmcdole.kivo.core.display.quality
import com.mmcdole.kivo.core.downloads.DownloadStorage
import com.mmcdole.kivo.core.session.KivoClient
import com.mmcdole.kivo.core.session.KivoSession
import com.mmcdole.kivo.core.session.SignedInDevice
import com.mmcdole.kivo.core.settings.DeviceSettings
import com.mmcdole.kivo.core.settings.DownloadQuality
import com.mmcdole.kivo.core.settings.Quality
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class PreferencesUiState(
    /** The account's settings, kept on the server, once read. */
    val user: Load<CurrentUser> = Load.Loading,
    /** This device's settings the core plays and downloads by. */
    val device: DeviceSettings? = null,
    /** This device's settings only the app's screens follow. */
    val screen: ScreenSettings = ScreenSettings(),
    /** Auto, Original and the server's ladder, once read; empty while it isn't. */
    val qualities: List<QualityOption> = emptyList(),
    /** Why reading the ladder failed. */
    val qualitiesFailed: String? = null,
    /** The space downloads take and what's free, on a device that keeps them. */
    val storage: DownloadStorage? = null,
    /** Whether this device keeps downloads. */
    val downloads: Boolean = false,
    /** Why the last change wasn't saved, with what Retry sends again. */
    val failure: String? = null,
)

/**
 * A page of preferences, as Playback, Audio, Subtitles, Quality and
 * Downloads show them: the account's, kept on the server for every app the
 * user signs in on, and this device's own. Each change applies at once and
 * says nothing unless it fails, when the page says why, with Retry.
 */
class PreferencesViewModel(
    private val session: KivoSession,
    private val core: KivoClient,
    private val screen: ScreenSettingsStore,
) : ViewModel() {
    private val user = MutableStateFlow<Load<CurrentUser>>(Load.Loading)
    private val extras = MutableStateFlow(PreferencesUiState(downloads = session.downloads != null))
    private var retry: (() -> Unit)? = null

    val state: StateFlow<PreferencesUiState> = combine(user, core.settings, screen.values, extras) { user, device, screen, extras ->
        extras.copy(user = user, device = device, screen = screen)
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), extras.value)

    init {
        load()
    }

    /** Reads the account's settings, the server's quality ladder and the space downloads take. */
    fun load() {
        viewModelScope.launch {
            user.value = Load.Loading
            user.value = try {
                Load.Loaded(session.account.currentUser())
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                Load.Failed(RequestProblem.of(e))
            }
        }
        viewModelScope.launch {
            try {
                val qualities = session.playback.qualityMenu()
                extras.update { it.copy(qualities = qualities, qualitiesFailed = null) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                extras.update { it.copy(qualitiesFailed = sentence(RequestProblem.of(e))) }
            }
        }
        readStorage()
    }

    private fun readStorage() {
        val downloads = session.downloads ?: return
        viewModelScope.launch {
            val storage = try {
                downloads.storage()
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                null
            }
            extras.update { it.copy(storage = storage) }
        }
    }

    fun setAutoplayNext(on: Boolean) = account { session.account.setAutoplayNext(on) }

    fun setRememberTracks(on: Boolean) = account { session.account.setRememberTracks(on) }

    fun setAudioLanguage(language: String?) = account { session.account.setAudioLanguage(language) }

    fun setSubtitleMode(mode: SubtitleMode) = account { session.account.setSubtitleMode(mode) }

    fun setSubtitleLanguage(language: String?) = account { session.account.setSubtitleLanguage(language) }

    fun setMaxAudioChannels(channels: Int) = save { core.setMaxAudioChannels(channels) }

    fun setDialogueBoost(on: Boolean) = save { core.setDialogueBoost(on) }

    fun setLocalQuality(quality: Quality) = save { core.setLocalQuality(quality) }

    fun setRemoteQuality(quality: Quality) = save { core.setRemoteQuality(quality) }

    fun setDownloadQuality(quality: DownloadQuality) = save { core.setDownloadQuality(quality) }

    /** Lets downloads use mobile data, or waits for Wi-Fi: the downloads under way go on under the new rule at once. */
    fun setCellularDownloads(allowed: Boolean) = save { core.setCellularDownloads(allowed) }

    fun setSubtitleSize(size: SubtitleSize) = save { screen.setSubtitleSize(size) }

    fun setSubtitleBackground(background: SubtitleBackground) = save { screen.setSubtitleBackground(background) }

    /** Deletes every download of this server, those under way cancelled. */
    fun deleteAllDownloads() {
        val downloads = session.downloads ?: return
        save {
            downloads.delete(downloads.items.first().map { it.id })
            readStorage()
        }
    }

    fun retry() {
        retry?.invoke()
    }

    private fun account(change: suspend () -> CurrentUser) = save { user.value = Load.Loaded(change()) }

    private fun save(change: suspend () -> Unit) {
        retry = { save(change) }
        viewModelScope.launch {
            try {
                change()
                extras.update { it.copy(failure = null) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                extras.update { it.copy(failure = sentence(RequestProblem.of(e))) }
            }
        }
    }
}

data class SessionsUiState(
    val sessions: Load<List<SignedInDevice>> = Load.Loading,
    /** The session whose sign-out failed, and why. */
    val failure: Pair<String?, String>? = null,
)

/**
 * Signed-in devices: the user's sessions, one for each browser and app
 * they're signed in on, most recently used first, each signing out, and
 * signing out everywhere else; [signOutHere] signs this app out.
 */
class SessionsViewModel(private val session: KivoSession, private val signOutHere: () -> Unit) : ViewModel() {
    private val mutableState = MutableStateFlow(SessionsUiState())
    val state: StateFlow<SessionsUiState> = mutableState.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            mutableState.update {
                it.copy(
                    sessions = try {
                        Load.Loaded(session.account.sessions())
                    } catch (e: CancellationException) {
                        throw e
                    } catch (e: Exception) {
                        Load.Failed(RequestProblem.of(e))
                    },
                )
            }
        }
    }

    /** Signs [device] out: this app goes to sign-in, another has to sign in again. */
    fun signOut(device: SignedInDevice) {
        if (device.current) {
            signOutHere()
            return
        }
        act(device.id) { session.account.signOutSession(device.id) }
    }

    fun signOutEverywhereElse() = act(null) { session.account.signOutEverywhereElse() }

    private fun act(id: String?, call: suspend () -> Unit) {
        viewModelScope.launch {
            try {
                call()
                mutableState.update { it.copy(failure = null) }
                load()
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                mutableState.update { it.copy(failure = id to sentence(RequestProblem.of(e))) }
            }
        }
    }
}

data class ChangePasswordUiState(
    val changing: Boolean = false,
    /** The current password given isn't the user's. */
    val wrongPassword: Boolean = false,
    val failure: String? = null,
    /** Changed: the page closes. */
    val changed: Boolean = false,
)

/**
 * Change password: the current password and a new one of at least
 * [MINIMUM_LENGTH] characters, and whether to sign out the user's other
 * browsers and apps. This app stays signed in.
 */
class ChangePasswordViewModel(private val session: KivoSession) : ViewModel() {
    private val mutableState = MutableStateFlow(ChangePasswordUiState())
    val state: StateFlow<ChangePasswordUiState> = mutableState.asStateFlow()

    fun change(current: String, password: String, signOutOthers: Boolean) {
        if (current.isEmpty() || password.length < MINIMUM_LENGTH || state.value.changing) return
        mutableState.value = ChangePasswordUiState(changing = true)
        viewModelScope.launch {
            mutableState.value = try {
                session.account.changePassword(current, password, signOutOthers)
                ChangePasswordUiState(changed = true)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                val code = (e as? ProblemException)?.code
                // A wrong current password is the server's refusal of the password, never of the session.
                if (code == "wrongPassword" || code == "invalidCredentials" || (e as? ProblemException)?.status == 401) {
                    ChangePasswordUiState(wrongPassword = true)
                } else {
                    ChangePasswordUiState(failure = sentence(RequestProblem.of(e)))
                }
            }
        }
    }

    /** The current password was changed, so it may be right now. */
    fun edited() = mutableState.update { it.copy(wrongPassword = false) }

    companion object {
        const val MINIMUM_LENGTH = 8
    }
}

data class LibrariesUiState(
    /** Every row of Home, in order, shown or not. */
    val rows: List<HomeRowInfo> = emptyList(),
    /** Why the rows couldn't be read, if they couldn't. */
    val rowsFailure: String? = null,
    /** Every library the user sees, in their order. */
    val libraries: List<Library> = emptyList(),
    val failure: String? = null,
)

/**
 * Home & Libraries: Home's rows in order, each shown or not, and every
 * library the user sees, in their order, each pinned or not; the same
 * server settings the TV and iPhone show. Pinned libraries are in the
 * sidebar. Each change is sent at once; a refused one says why, with Retry.
 */
class LibrariesSettingsViewModel(private val session: KivoSession) : ViewModel() {
    private val mutableState = MutableStateFlow(LibrariesUiState())
    val state: StateFlow<LibrariesUiState> = mutableState.asStateFlow()

    /** Sends the last change again, for Retry. */
    private var retrying: (suspend () -> Unit)? = null

    private var readingRows: Job? = null

    init {
        viewModelScope.launch {
            val libraries = session.library.libraries().first { it !is Load.Loading }.getOrNull().orEmpty()
            mutableState.update { it.copy(libraries = libraries) }
        }
        loadRows()
    }

    /** Reads Home's rows, and again as they change. */
    fun loadRows() {
        readingRows?.cancel()
        mutableState.update { it.copy(rowsFailure = null) }
        readingRows = viewModelScope.launch {
            session.library.homeRows().collect { load ->
                when (load) {
                    Load.Loading -> Unit
                    is Load.Failed -> mutableState.update { it.copy(rowsFailure = sentence(load.problem)) }
                    is Load.Loaded -> mutableState.update { it.copy(rows = load.value, rowsFailure = null) }
                }
            }
        }
    }

    fun show(id: String, shown: Boolean) = changeRows { rows -> rows.map { if (it.id == id) it.copy(shown = shown) else it } }

    /** Moves the row [id] up ([by] -1) or down (1) past the next one listed, the others staying where they are. */
    fun moveRow(id: String, by: Int) = changeRows { rows -> movedRow(rows, id, by) }

    /** Clears Home's order and hidden rows, and shows the layout as the server has it. */
    fun resetHome() = send { mutableState.update { it.copy(rows = session.library.resetHomeRows()) } }

    fun pin(id: String, pinned: Boolean) = change { all -> all.map { if (it.id == id) it.copy(pinned = pinned) else it } }

    /** Moves the library [id] up ([by] -1) or down (1). */
    fun move(id: String, by: Int) = change { all -> moved(all, all.indexOfFirst { it.id == id }, by) }

    fun retry() {
        val again = retrying ?: return
        send(again)
    }

    private fun changeRows(transform: (List<HomeRowInfo>) -> List<HomeRowInfo>) {
        val after = transform(state.value.rows)
        mutableState.update { it.copy(rows = after) }
        send { session.library.setHomeRows(after.map { HomeRowChange(it.id, it.shown) }) }
    }

    private fun change(transform: (List<Library>) -> List<Library>) {
        val after = transform(state.value.libraries)
        mutableState.update { it.copy(libraries = after) }
        send { session.account.setLibraries(after.map { LibraryChoice(it.id, it.pinned) }) }
    }

    private fun send(call: suspend () -> Unit) {
        retrying = call
        viewModelScope.launch {
            try {
                call()
                mutableState.update { it.copy(failure = null) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                mutableState.update { it.copy(failure = sentence(RequestProblem.of(e))) }
            }
        }
    }
}

/** [all] with the row [id] swapped with the next one the phone lists, up ([by] -1) or down (1); [all] itself when there's none. */
internal fun movedRow(all: List<HomeRowInfo>, id: String, by: Int): List<HomeRowInfo> {
    val listed = all.filter { it.kind in shownKinds }
    val from = listed.indexOfFirst { it.id == id }
    if (from < 0) return all
    val other = listed.getOrNull(from + by) ?: return all
    val a = all.indexOf(listed[from])
    val b = all.indexOf(other)
    return all.toMutableList().apply {
        this[a] = other
        this[b] = listed[from]
    }
}

/** [all] with the item at [from] swapped with its neighbour up ([by] -1) or down (1); [all] itself past either end. */
private fun <T> moved(all: List<T>, from: Int, by: Int): List<T> {
    val to = from + by
    return if (from < 0 || to !in all.indices) all else all.toMutableList().apply { add(to, removeAt(from)) }
}

/** A quality as Quality's menu names it, by the core's words. */
fun qualityName(option: QualityOption): String = display.quality(option, null)
