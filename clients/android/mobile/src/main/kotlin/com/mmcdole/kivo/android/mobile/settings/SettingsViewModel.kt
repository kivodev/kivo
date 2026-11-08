package com.mmcdole.kivo.android.mobile.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.android.settings.Credit
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.session.KivoSession
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.stateIn

data class SettingsUiState(
    val username: String = "",
    val serverName: String = "",
    val appVersion: String = "",
    /** The server's version, as it says. */
    val serverVersion: Load<String> = Load.Loading,
)

/** Settings: who's signed in where, signing out or choosing another server, and the versions for About. */
@OptIn(ExperimentalCoroutinesApi::class)
class SettingsViewModel(
    private val session: KivoSession,
    appVersion: String,
    private val onSignOut: () -> Unit,
    private val onChangeServer: () -> Unit,
) : ViewModel() {
    private val versionReads = MutableStateFlow(0)

    private val serverVersion = versionReads.flatMapLatest {
        flow {
            emit(Load.Loading)
            emit(
                try {
                    Load.Loaded(session.serverInfo().version)
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    Load.Failed(RequestProblem.of(e))
                },
            )
        }
    }

    val state: StateFlow<SettingsUiState> = combine(session.user, session.savedServer, serverVersion) { user, server, version ->
        SettingsUiState(user?.username.orEmpty(), server.name, appVersion, version)
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), SettingsUiState(appVersion = appVersion))

    fun signOut() = onSignOut()

    fun changeServer() = onChangeServer()

    fun retryServerVersion() {
        versionReads.value++
    }
}

/** The open source licenses, read once, off the main thread. */
class LicensesViewModel(load: suspend () -> List<Credit>) : ViewModel() {
    val state: StateFlow<List<Credit>?> = flow { emit(load()) }.stateIn(viewModelScope, SharingStarted.Eagerly, null)
}
