package com.mmcdole.kivo.android.mobile.settings

import android.text.format.DateUtils
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.app.LinkDeviceRoute
import com.mmcdole.kivo.android.mobile.app.MobileNav
import com.mmcdole.kivo.android.mobile.app.SettingsPage
import com.mmcdole.kivo.android.mobile.app.SettingsPageRoute
import com.mmcdole.kivo.android.mobile.linkdevice.platformName
import com.mmcdole.kivo.android.mobile.theme.KivoTokens
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.android.mobile.ui.wideWidth
import com.mmcdole.kivo.android.mobile.ui.PageTopBar
import com.mmcdole.kivo.android.mobile.ui.sentence
import com.mmcdole.kivo.api.models.SessionKind
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.session.SignedInDevice

/**
 * Account: Change password, Signed-in devices and Link a device, then
 * Change server and Sign out, which asks first.
 */
@Composable
fun AccountScreen(session: ServerSession, nav: MobileNav) {
    val model = settingsModel(session)
    var confirmSignOut by rememberSaveable { mutableStateOf(false) }
    SettingsList(stringResource(R.string.mobile_account), nav, "accountPage") {
        item { SettingRow(stringResource(R.string.mobile_change_password), "changePassword") { nav.open(SettingsPageRoute(SettingsPage.ChangePassword)) } }
        item { SettingRow(stringResource(R.string.mobile_signed_in_devices), "devices") { nav.open(SettingsPageRoute(SettingsPage.Devices)) } }
        item { SettingRow(stringResource(R.string.mobile_link_device), "linkDevice") { nav.open(LinkDeviceRoute()) } }
        item { HorizontalDivider(Modifier.padding(vertical = space(2))) }
        item { SettingRow(stringResource(R.string.mobile_change_server), "changeServer") { model.changeServer() } }
        item { SettingRow(stringResource(R.string.mobile_sign_out), "signOut", danger = true) { confirmSignOut = true } }
    }
    if (confirmSignOut) SignOutDialog(onDismiss = { confirmSignOut = false }, onSignOut = model::signOut)
}

@Composable
private fun SignOutDialog(onDismiss: () -> Unit, onSignOut: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.mobile_sign_out_title)) },
        confirmButton = {
            TextButton(onClick = {
                onDismiss()
                onSignOut()
            }, Modifier.testTag("confirmSignOut")) { Text(stringResource(R.string.mobile_sign_out)) }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.mobile_cancel)) } },
    )
}

/**
 * Signed-in devices: the user's sessions, one for each browser and app
 * they're signed in on, with what each runs on, most recently used first,
 * each with Sign out, and under them Sign out everywhere else. Every
 * sign-out asks first; signing out this one goes to sign-in.
 */
@Composable
fun SessionsScreen(session: ServerSession, nav: MobileNav) {
    val model = viewModel { SessionsViewModel(session.session, session::signOut) }
    val state by model.state.collectAsStateWithLifecycle()
    var confirming by rememberSaveable { mutableStateOf<String?>(null) }
    var everywhereElse by rememberSaveable { mutableStateOf(false) }
    SettingsList(stringResource(R.string.mobile_signed_in_devices), nav, "sessions") {
        when (val sessions = state.sessions) {
            is Load.Loaded -> {
                for (device in sessions.value) {
                    item(key = device.id) {
                        ListItem(
                            headlineContent = { Text(deviceName(device)) },
                            supportingContent = { Text(deviceLine(device)) },
                            trailingContent = {
                                TextButton(onClick = { confirming = device.id }, Modifier.testTag("signOut:${deviceName(device)}")) {
                                    Text(stringResource(R.string.mobile_sign_out), color = MaterialTheme.colorScheme.error)
                                }
                            },
                            colors = clear(),
                        )
                        state.failure?.takeIf { it.first == device.id }?.let { FailureRow(it.second) { model.signOut(device) } }
                    }
                }
                if (sessions.value.any { !it.current }) {
                    item { HorizontalDivider(Modifier.padding(vertical = space(2))) }
                    item { SettingRow(stringResource(R.string.mobile_sign_out_everywhere_else), "everywhereElse", danger = true) { everywhereElse = true } }
                    state.failure?.takeIf { it.first == null }?.let { item { FailureRow(it.second, model::signOutEverywhereElse) } }
                }
            }
            is Load.Failed -> item { FailureRow(sentence(sessions.problem), model::load) }
            Load.Loading -> item { Column(Modifier.fillMaxWidth().padding(space(6)), horizontalAlignment = androidx.compose.ui.Alignment.CenterHorizontally) { CircularProgressIndicator() } }
        }
    }
    val sessions = state.sessions.getOrNull().orEmpty()
    sessions.firstOrNull { it.id == confirming }?.let { device ->
        AlertDialog(
            onDismissRequest = { confirming = null },
            title = { Text(if (device.current) stringResource(R.string.mobile_sign_out_title) else stringResource(R.string.mobile_sign_out_named, deviceName(device))) },
            text = if (device.current) null else ({ Text(stringResource(R.string.mobile_sign_in_again, deviceName(device))) }),
            confirmButton = {
                TextButton(onClick = {
                    confirming = null
                    model.signOut(device)
                }, Modifier.testTag("confirmSignOut")) { Text(stringResource(R.string.mobile_sign_out), color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = { TextButton(onClick = { confirming = null }) { Text(stringResource(R.string.mobile_cancel)) } },
        )
    }
    if (everywhereElse) {
        AlertDialog(
            onDismissRequest = { everywhereElse = false },
            title = { Text(stringResource(R.string.mobile_sign_out_everywhere_else_title)) },
            text = { Text(stringResource(R.string.mobile_stays_signed_in)) },
            confirmButton = {
                TextButton(onClick = {
                    everywhereElse = false
                    model.signOutEverywhereElse()
                }, Modifier.testTag("confirmEverywhereElse")) { Text(stringResource(R.string.mobile_sign_out_everywhere_else), color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = { TextButton(onClick = { everywhereElse = false }) { Text(stringResource(R.string.mobile_cancel)) } },
        )
    }
}

/** The session's name, or what it is when it has none, as a browser may not. */
@Composable
private fun deviceName(device: SignedInDevice): String = device.name.ifEmpty {
    stringResource(if (device.kind == SessionKind.Browser) R.string.mobile_web_browser else R.string.mobile_app)
}

/** "Android TV · Last used 3 hours ago", or "Android phone · This device". */
@Composable
private fun deviceLine(device: SignedInDevice): String {
    val kind = platformName(device.platform) ?: stringResource(if (device.kind == SessionKind.Browser) R.string.mobile_platform_browser else R.string.mobile_app)
    if (device.current) return "$kind · ${stringResource(R.string.mobile_this_device)}"
    val used = DateUtils.getRelativeTimeSpanString(device.lastUsedEpochMs, System.currentTimeMillis(), DateUtils.MINUTE_IN_MILLIS).toString()
    return "$kind · ${stringResource(R.string.mobile_last_used, used)}"
}

/**
 * Change password: the current password, a new one of at least 8
 * characters, and whether to sign out the user's other browsers and apps,
 * which starts on. This app stays signed in, and the page closes once the
 * server has changed it. A wrong current password says so under its field.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ChangePasswordScreen(session: ServerSession, nav: MobileNav) {
    val model = viewModel { ChangePasswordViewModel(session.session) }
    val state by model.state.collectAsStateWithLifecycle()
    var current by rememberSaveable { mutableStateOf("") }
    var password by rememberSaveable { mutableStateOf("") }
    var signOutOthers by rememberSaveable { mutableStateOf(true) }
    LaunchedEffect(state.changed) { if (state.changed) nav.back() }
    val ready = current.isNotEmpty() && password.length >= ChangePasswordViewModel.MINIMUM_LENGTH && !state.changing
    Scaffold(topBar = { PageTopBar(stringResource(R.string.mobile_change_password), nav::back) }) { padding ->
        Column(
            Modifier.padding(padding).verticalScroll(rememberScrollState()).padding(space(4)).wideWidth(KivoTokens.Container.form).testTag("changePasswordPage"),
            verticalArrangement = Arrangement.spacedBy(space(3)),
        ) {
            OutlinedTextField(
                current,
                onValueChange = {
                    current = it
                    model.edited()
                },
                label = { Text(stringResource(R.string.mobile_current_password)) },
                singleLine = true,
                isError = state.wrongPassword,
                supportingText = if (state.wrongPassword) ({ Text(stringResource(R.string.mobile_wrong_password)) }) else null,
                visualTransformation = PasswordVisualTransformation(),
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
                modifier = Modifier.fillMaxWidth().testTag("currentPassword"),
            )
            OutlinedTextField(
                password,
                onValueChange = { password = it },
                label = { Text(stringResource(R.string.mobile_new_password)) },
                singleLine = true,
                supportingText = { Text(stringResource(R.string.mobile_password_length)) },
                visualTransformation = PasswordVisualTransformation(),
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
                modifier = Modifier.fillMaxWidth().testTag("newPassword"),
            )
            Toggle(stringResource(R.string.mobile_sign_out_other_devices), signOutOthers, "signOutOthers") { signOutOthers = it }
            state.failure?.let { FailureRow(it) { model.change(current, password, signOutOthers) } }
            Button(onClick = { model.change(current, password, signOutOthers) }, enabled = ready, modifier = Modifier.fillMaxWidth().testTag("change")) {
                if (state.changing) CircularProgressIndicator(Modifier.padding(2.dp), strokeWidth = 2.dp) else Text(stringResource(R.string.mobile_change_password))
            }
        }
    }
}
