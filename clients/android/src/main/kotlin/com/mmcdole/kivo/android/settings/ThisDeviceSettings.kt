package com.mmcdole.kivo.android.settings

import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.BuildConfig
import com.mmcdole.kivo.android.R
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.ui.Choice
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.android.ui.QrCode
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.android.ui.focusKey
import com.mmcdole.kivo.android.ui.display
import com.mmcdole.kivo.android.ui.rememberFocusMemory
import com.mmcdole.kivo.api.models.QualityRung
import com.mmcdole.kivo.api.models.ServerInfo
import com.mmcdole.kivo.core.display.bitrate
import com.mmcdole.kivo.core.display.quality
import com.mmcdole.kivo.core.display.qualityMenu
import com.mmcdole.kivo.core.settings.DeviceSettings
import com.mmcdole.kivo.core.settings.Quality
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

/** This TV's settings, the core's and the screens' own, kept on the device, each change kept as it's made. */
class ThisDeviceViewModel(private val session: ServerSession) : ViewModel() {
    val settings: StateFlow<DeviceSettings?> =
        session.graph.core.settings.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), null)

    val screen: StateFlow<ScreenSettings?> =
        session.graph.screenSettings.values.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), null)

    fun update(transform: (DeviceSettings) -> DeviceSettings) {
        viewModelScope.launch { session.graph.core.updateSettings(transform) }
    }

    fun updateScreen(change: suspend ScreenSettingsStore.() -> Unit) {
        viewModelScope.launch { session.graph.screenSettings.change() }
    }
}

/**
 * What the server tells Quality: its quality ladder and the user's remote
 * streaming limit, each read quietly, the page offering Auto and Original
 * without the ladder.
 */
class QualityViewModel(private val session: ServerSession) : ViewModel() {
    /** The server's ladder; null until read, and if it can't be. */
    var rungs by mutableStateOf<List<QualityRung>?>(null)
        private set

    /** The most the user's remote streams may use, when the server limits them. */
    var remoteCapBps by mutableStateOf<Long?>(null)
        private set

    init {
        viewModelScope.launch { quietly { rungs = session.session.playback.qualityLadder() } }
        viewModelScope.launch { quietly { remoteCapBps = session.session.account.currentUser().limits.remoteQualityCapBps } }
    }
}

private suspend fun quietly(call: suspend () -> Unit) {
    try {
        call()
    } catch (e: CancellationException) {
        throw e
    } catch (_: Exception) {
        // The page shows what it has without it.
    }
}

/**
 * Quality, on this TV: the most a play uses at home and away, a stored rung
 * the server doesn't offer showing as Auto.
 */
@Composable
internal fun QualityPage(session: ServerSession) {
    val model = viewModel { ThisDeviceViewModel(session) }
    val server = viewModel { QualityViewModel(session) }
    val settings by model.settings.collectAsStateWithLifecycle()
    val memory = rememberFocusMemory()
    var picking by remember { mutableStateOf<Picking?>(null) }
    val pick: (Picking) -> Unit = { picking = it }

    SettingsPage(SettingsGroup.Quality, memory, initial = "localQuality".takeIf { settings != null }) {
        val device = settings ?: return@SettingsPage
        val rungs = server.rungs
        val shown = rungs?.let { device.onLadder(it) } ?: device
        item { ThisTvHeading() }
        item {
            val title = stringResource(R.string.home_quality)
            val choices = qualityChoices(rungs.orEmpty())
            SettingRow(title, choices.label(shown.localQuality), {
                pick(Picking(title, choices, shown.localQuality.key) { id -> model.update { it.copy(localQuality = Quality.of(id)) } })
            }, Modifier.focusKey(memory, "localQuality"))
        }
        item {
            val title = stringResource(R.string.remote_quality)
            val choices = qualityChoices(rungs.orEmpty())
            val cap = server.remoteCapBps?.let { stringResource(R.string.limited_by_server, display.bitrate(it)) }
            SettingRow(title, choices.label(shown.remoteQuality), {
                pick(Picking(title, choices, shown.remoteQuality.key) { id -> model.update { it.copy(remoteQuality = Quality.of(id)) } })
            }, Modifier.focusKey(memory, "remoteQuality"), supporting = cap)
        }
    }
    picking?.let { PickingPanel(it) { picking = null } }
}

/** Display, on this TV below Android 12: frame-rate matching. */
@Composable
internal fun DisplayPage(session: ServerSession) {
    val model = viewModel { ThisDeviceViewModel(session) }
    val settings by model.screen.collectAsStateWithLifecycle()
    val memory = rememberFocusMemory()

    SettingsPage(SettingsGroup.Display, memory, initial = "frameRate".takeIf { settings != null }) {
        val screen = settings ?: return@SettingsPage
        item { ThisTvHeading() }
        item {
            Toggle(stringResource(R.string.frame_rate_matching), screen.frameRateMatching, Modifier.focusKey(memory, "frameRate")) { on ->
                model.updateScreen { setFrameRateMatching(on) }
            }
        }
    }
}

/** Auto, Original, then each rung of the ladder. */
@Composable
private fun qualityChoices(rungs: List<QualityRung>): List<Choice> {
    val autoDetail = stringResource(R.string.auto_detail)
    return qualityMenu(rungs).map { option ->
        Choice(option.quality.key, display.quality(option, null), autoDetail.takeIf { option.quality == Quality.Auto })
    }
}

private fun List<Choice>.label(quality: Quality) = firstOrNull { it.id == quality.key }?.label ?: first().label

/** The server's name and version, for About. */
class AboutViewModel(private val session: ServerSession) : ViewModel() {
    var server by mutableStateOf<Load<ServerInfo>>(Load.Loading)
        private set

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            server = try {
                Load.Loaded(session.session.serverInfo())
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                Load.Failed(RequestProblem.of(e))
            }
        }
    }
}

/**
 * About: this app's version and the server's, rows that take no focus, and
 * under them the server's address with a QR code, to manage it from a browser.
 */
@Composable
internal fun AboutPage(session: ServerSession) {
    val model = viewModel { AboutViewModel(session) }
    val memory = rememberFocusMemory()
    val server = model.server
    SettingsPage(
        SettingsGroup.About,
        memory,
        initial = null,
        readOnly = server !is Load.Failed,
    ) {
        item { InfoRow(stringResource(R.string.app_version), BuildConfig.VERSION_NAME) }
        when (server) {
            Load.Loading -> Unit
            is Load.Failed -> item { LoadFailed(server.problem, model::load) }
            is Load.Loaded -> item { InfoRow(stringResource(R.string.server_version), server.value.version) }
        }
        // The credits sit beside the address: the page doesn't scroll, and under it they'd run off the screen.
        item {
            Row(verticalAlignment = Alignment.Bottom, horizontalArrangement = Arrangement.spacedBy(Kivo.space(12))) {
                ServerAddress(server.getOrNull()?.let(::serverAddress) ?: session.session.baseUrl)
                Credits()
            }
        }
    }
}

/** Where the metadata and the transcoder come from: TMDB's logo and the notice its API terms ask for, then Audnexus and jellyfin-ffmpeg. */
@Composable
private fun Credits() {
    Column(verticalArrangement = Arrangement.spacedBy(Kivo.space(1))) {
        Image(
            painterResource(R.drawable.tmdb_logo),
            stringResource(R.string.tmdb),
            Modifier.padding(bottom = Kivo.space(1)).height(Kivo.space(3)),
        )
        Text(stringResource(R.string.tmdb_notice), style = TvType.small, color = Kivo.colors.textMuted)
        Text(stringResource(R.string.credit_audnexus), style = TvType.small, color = Kivo.colors.textMuted)
        Text(stringResource(R.string.credit_jellyfin_ffmpeg), style = TvType.small, color = Kivo.colors.textMuted)
    }
}

/**
 * The server's address, for a phone to find the server or a browser to
 * manage it: its public URL when it has one, else its first local URL.
 */
private fun serverAddress(info: ServerInfo): String? = info.publicUrl ?: info.localUrls?.firstOrNull()

/** The server's address [url] as a QR code and as text. */
@Composable
private fun ServerAddress(url: String) {
    Column(Modifier.padding(start = Kivo.space(4), top = Kivo.space(6)), verticalArrangement = Arrangement.spacedBy(Kivo.space(3))) {
        Text(stringResource(R.string.server_address), style = TvType.heading, color = Kivo.colors.text)
        QrCode(url, 160.dp)
        Column {
            Text(stringResource(R.string.manage_from_browser_help), style = TvType.body, color = Kivo.colors.textMuted)
            Text(url.substringAfter("://"), style = TvType.bodyStrong, color = Kivo.colors.text)
        }
    }
}
