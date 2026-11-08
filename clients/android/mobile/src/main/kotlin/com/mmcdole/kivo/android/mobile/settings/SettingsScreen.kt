package com.mmcdole.kivo.android.mobile.settings

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.app.CreditsRoute
import com.mmcdole.kivo.android.mobile.app.LicenseRoute
import com.mmcdole.kivo.android.mobile.app.LicensesRoute
import com.mmcdole.kivo.android.mobile.app.LinkDeviceRoute
import com.mmcdole.kivo.android.mobile.app.MobileNav
import com.mmcdole.kivo.android.mobile.app.SettingsPage
import com.mmcdole.kivo.android.mobile.app.SettingsPageRoute
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.android.mobile.ui.Loading
import com.mmcdole.kivo.android.mobile.ui.PageTopBar
import com.mmcdole.kivo.android.settings.AppCredits
import com.mmcdole.kivo.core.Load

/**
 * Settings: the user and their server, which opens Account, and Link a
 * device; a page per topic, each holding the account's settings, kept on
 * the server for every app the user signs in on, and this device's own;
 * then About. There's no Save: each change applies at once.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(session: ServerSession, nav: MobileNav) {
    val model = settingsModel(session)
    val state by model.state.collectAsStateWithLifecycle()
    fun page(page: SettingsPage) = nav.open(SettingsPageRoute(page))
    Scaffold(topBar = { PageTopBar(stringResource(R.string.mobile_settings), nav::back) }) { padding ->
        LazyColumn(Modifier.fillMaxSize().testTag("settings"), contentPadding = padding) {
            item {
                ListItem(
                    headlineContent = { Text(state.username, style = MaterialTheme.typography.titleMedium) },
                    supportingContent = { Text(state.serverName) },
                    leadingContent = {
                        Box(Modifier.size(56.dp).background(MaterialTheme.colorScheme.primary, CircleShape), contentAlignment = Alignment.Center) {
                            Text(state.username.take(1).uppercase(), color = MaterialTheme.colorScheme.onPrimary, style = MaterialTheme.typography.headlineSmall)
                        }
                    },
                    colors = clear(),
                    modifier = Modifier.testTag("account").clickable { page(SettingsPage.Account) },
                )
            }
            item { SettingRow(stringResource(R.string.mobile_link_device), "linkDevice") { nav.open(LinkDeviceRoute()) } }
            item { HorizontalDivider(Modifier.padding(vertical = space(2))) }
            item { SettingRow(stringResource(R.string.mobile_settings_playback), "playback") { page(SettingsPage.Playback) } }
            item { SettingRow(stringResource(R.string.mobile_audio), "audio") { page(SettingsPage.Audio) } }
            item { SettingRow(stringResource(R.string.mobile_subtitles), "subtitles") { page(SettingsPage.Subtitles) } }
            item { SettingRow(stringResource(R.string.mobile_quality), "quality") { page(SettingsPage.Quality) } }
            if (session.session.downloads != null) item { SettingRow(stringResource(R.string.mobile_tab_downloads), "downloads") { page(SettingsPage.Downloads) } }
            item { SettingRow(stringResource(R.string.mobile_tab_libraries), "libraries") { page(SettingsPage.Libraries) } }
            item { HorizontalDivider(Modifier.padding(vertical = space(2))) }
            item { SettingRow(stringResource(R.string.mobile_about), "about") { page(SettingsPage.About) } }
        }
    }
}

/** About: the app's and the server's versions, and Credits. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AboutScreen(session: ServerSession, nav: MobileNav) {
    val model = settingsModel(session)
    val state by model.state.collectAsStateWithLifecycle()
    SettingsList(stringResource(R.string.mobile_about), nav, "about") {
        item { Info(stringResource(R.string.mobile_app_version), state.appVersion) }
        item {
            when (val version = state.serverVersion) {
                is Load.Loaded -> Info(stringResource(R.string.mobile_server_version), version.value)
                is Load.Failed -> SettingRow(stringResource(R.string.mobile_server_version_failed), "serverVersion") { model.retryServerVersion() }
                Load.Loading -> Info(stringResource(R.string.mobile_server_version), "")
            }
        }
        item { SettingRow(stringResource(R.string.mobile_credits), "credits") { nav.open(CreditsRoute) } }
    }
}

@Composable
internal fun settingsModel(session: ServerSession) = viewModel {
    SettingsViewModel(session.session, session.graph.build.versionName, session::signOut, session.graph::changeServer)
}

/** Credits: TMDB's logo and the notice its terms ask for, Audnexus and jellyfin-ffmpeg, then Open source licenses. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun CreditsScreen(nav: MobileNav) {
    Scaffold(topBar = { PageTopBar(stringResource(R.string.mobile_credits), nav::back) }) { padding ->
        Column(Modifier.padding(padding).verticalScroll(rememberScrollState())) {
            Column(Modifier.padding(space(4)), verticalArrangement = Arrangement.spacedBy(space(3))) {
                Image(painterResource(R.drawable.mobile_tmdb_logo), stringResource(R.string.mobile_tmdb), Modifier.height(space(5)))
                Text(stringResource(R.string.mobile_tmdb_notice), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                Text(stringResource(R.string.mobile_credit_audnexus), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                Text(stringResource(R.string.mobile_credit_jellyfin_ffmpeg), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            HorizontalDivider()
            SettingRow(stringResource(R.string.mobile_open_source_licenses), "licenses") { nav.open(LicensesRoute) }
        }
    }
}

/** Open source licenses: each library with its version and license, opening the license's text. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun LicensesScreen(nav: MobileNav) {
    val context = LocalContext.current.applicationContext
    val model = viewModel { LicensesViewModel { AppCredits.get(context) } }
    val credits by model.state.collectAsStateWithLifecycle()
    Scaffold(topBar = { PageTopBar(stringResource(R.string.mobile_open_source_licenses), nav::back) }) { padding ->
        val list = credits
        if (list == null) {
            Loading(Modifier.padding(padding))
        } else {
            LazyColumn(Modifier.fillMaxSize(), contentPadding = padding) {
                items(list, key = { it.id }) { credit ->
                    ListItem(
                        headlineContent = { Text(credit.name) },
                        supportingContent = { Text(listOfNotNull(credit.version, credit.license).joinToString(" · ")) },
                        colors = clear(),
                        modifier = Modifier.clickable(enabled = credit.text != null) { nav.open(LicenseRoute(credit.id)) },
                    )
                }
            }
        }
    }
}

/** One license's text. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun LicenseScreen(id: String, nav: MobileNav) {
    val context = LocalContext.current.applicationContext
    val model = viewModel { LicensesViewModel { AppCredits.get(context) } }
    val credit = model.state.collectAsStateWithLifecycle().value?.firstOrNull { it.id == id }
    Scaffold(topBar = { PageTopBar(credit?.name.orEmpty(), nav::back) }) { padding ->
        LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(start = space(4), end = space(4), top = padding.calculateTopPadding(), bottom = space(8))) {
            item { Text(credit?.text.orEmpty(), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
        }
    }
}

/** A settings page: its title over a list of rows. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun SettingsList(title: String, nav: MobileNav, tag: String, content: LazyListScope.() -> Unit) {
    Scaffold(topBar = { PageTopBar(title, nav::back) }) { padding ->
        LazyColumn(Modifier.fillMaxSize().testTag(tag), contentPadding = padding, content = content)
    }
}

@Composable
internal fun clear() = ListItemDefaults.colors(containerColor = Color.Transparent)

/** A row that opens a page or acts, in the danger colour for a destructive one. */
@Composable
internal fun SettingRow(label: String, tag: String, danger: Boolean = false, detail: String? = null, onClick: () -> Unit) {
    ListItem(
        headlineContent = { Text(label, color = if (danger) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurface) },
        supportingContent = detail?.let { { Text(it) } },
        colors = clear(),
        modifier = Modifier.testTag(tag).clickable(onClick = onClick),
    )
}

@Composable
internal fun Info(label: String, value: String) {
    ListItem(
        headlineContent = { Text(label) },
        trailingContent = { Text(value, color = MaterialTheme.colorScheme.onSurfaceVariant) },
        colors = clear(),
    )
}

/** A group's heading: whose settings follow, the account's or this device's. */
@Composable
internal fun Label(text: String, note: String? = null) {
    Column(Modifier.padding(start = space(4), end = space(4), top = space(4), bottom = space(1))) {
        Text(text, style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.primary)
        note?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
    }
}

/** A line under a group, saying what it does. */
@Composable
internal fun Footnote(text: String) {
    Text(text, Modifier.padding(horizontal = space(4), vertical = space(1)), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
}

/** A switch row: the whole row toggles it. */
@Composable
internal fun Toggle(label: String, checked: Boolean, tag: String, detail: String? = null, enabled: Boolean = true, onChange: (Boolean) -> Unit) {
    ListItem(
        headlineContent = { Text(label) },
        supportingContent = detail?.let { { Text(it) } },
        trailingContent = { Switch(checked, onCheckedChange = null, enabled = enabled) },
        colors = clear(),
        modifier = Modifier.testTag(tag).selectable(checked, enabled = enabled, role = Role.Switch) { onChange(!checked) },
    )
}

/** One of a choice row's choices: its ID and name. */
internal data class Choice<T>(val id: T, val label: String)

/** A row showing the choice made, which opens a dialog of the choices, one picked. */
@Composable
internal fun <T> ChoiceRow(label: String, choices: List<Choice<T>>, selected: T, tag: String, enabled: Boolean = true, onPick: (T) -> Unit) {
    var open by rememberSaveable { mutableStateOf(false) }
    ListItem(
        headlineContent = { Text(label) },
        supportingContent = { Text(choices.firstOrNull { it.id == selected }?.label ?: choices.firstOrNull()?.label.orEmpty()) },
        colors = clear(),
        modifier = Modifier.testTag(tag).clickable(enabled = enabled) { open = true },
    )
    if (!open) return
    AlertDialog(
        onDismissRequest = { open = false },
        title = { Text(label) },
        text = {
            LazyColumn {
                items(choices) { choice ->
                    ListItem(
                        headlineContent = { Text(choice.label) },
                        leadingContent = { RadioButton(choice.id == selected, onClick = null) },
                        colors = clear(),
                        modifier = Modifier.testTag("choice:${choice.label}").selectable(choice.id == selected, role = Role.RadioButton) {
                            open = false
                            onPick(choice.id)
                        },
                    )
                }
            }
        },
        confirmButton = { TextButton(onClick = { open = false }) { Text(stringResource(R.string.mobile_cancel)) } },
    )
}

/** A change that wasn't saved: why, in the danger colour, with Retry. */
@Composable
internal fun FailureRow(message: String, onRetry: () -> Unit) {
    ListItem(
        headlineContent = { Text(message, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium) },
        trailingContent = { TextButton(onClick = onRetry) { Text(stringResource(R.string.mobile_retry)) } },
        colors = clear(),
        modifier = Modifier.testTag("failure"),
    )
}
