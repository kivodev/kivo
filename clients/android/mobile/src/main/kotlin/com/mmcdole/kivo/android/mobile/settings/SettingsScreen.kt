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
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
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
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.app.CreditsRoute
import com.mmcdole.kivo.android.mobile.app.LicenseRoute
import com.mmcdole.kivo.android.mobile.app.LicensesRoute
import com.mmcdole.kivo.android.mobile.app.MobileNav
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.android.mobile.ui.Loading
import com.mmcdole.kivo.android.mobile.ui.PageTopBar
import com.mmcdole.kivo.android.settings.AppCredits
import com.mmcdole.kivo.core.Load

/** Settings: the account and server, Sign out and Change server, then About. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(session: ServerSession, nav: MobileNav) {
    val model = viewModel {
        SettingsViewModel(session.session, session.graph.build.versionName, session::signOut, session.graph::changeServer)
    }
    val state by model.state.collectAsStateWithLifecycle()
    var confirmSignOut by rememberSaveable { mutableStateOf(false) }
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
                )
            }
            item { Row(stringResource(R.string.mobile_change_server), "changeServer") { model.changeServer() } }
            item { Row(stringResource(R.string.mobile_sign_out), "signOut", danger = true) { confirmSignOut = true } }
            item { HorizontalDivider(Modifier.padding(vertical = space(2))) }
            item { Label(stringResource(R.string.mobile_about)) }
            item { Info(stringResource(R.string.mobile_app_version), state.appVersion) }
            item {
                when (val version = state.serverVersion) {
                    is Load.Loaded -> Info(stringResource(R.string.mobile_server_version), version.value)
                    is Load.Failed -> Row(stringResource(R.string.mobile_server_version_failed), "serverVersion") { model.retryServerVersion() }
                    Load.Loading -> Info(stringResource(R.string.mobile_server_version), "")
                }
            }
            item { Row(stringResource(R.string.mobile_credits), "credits") { nav.open(CreditsRoute) } }
        }
    }
    if (confirmSignOut) {
        AlertDialog(
            onDismissRequest = { confirmSignOut = false },
            title = { Text(stringResource(R.string.mobile_sign_out_title)) },
            confirmButton = {
                TextButton(onClick = {
                    confirmSignOut = false
                    model.signOut()
                }) { Text(stringResource(R.string.mobile_sign_out)) }
            },
            dismissButton = { TextButton(onClick = { confirmSignOut = false }) { Text(stringResource(R.string.mobile_cancel)) } },
        )
    }
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
            Row(stringResource(R.string.mobile_open_source_licenses), "licenses") { nav.open(LicensesRoute) }
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

@Composable
private fun clear() = ListItemDefaults.colors(containerColor = Color.Transparent)

@Composable
private fun Row(label: String, tag: String, danger: Boolean = false, onClick: () -> Unit) {
    ListItem(
        headlineContent = { Text(label, color = if (danger) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurface) },
        colors = clear(),
        modifier = Modifier.testTag(tag).clickable(onClick = onClick),
    )
}

@Composable
private fun Info(label: String, value: String) {
    ListItem(
        headlineContent = { Text(label) },
        trailingContent = { Text(value, color = MaterialTheme.colorScheme.onSurfaceVariant) },
        colors = clear(),
    )
}

@Composable
private fun Label(text: String) {
    Text(text, Modifier.padding(horizontal = space(4), vertical = space(2)), style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.primary)
}
