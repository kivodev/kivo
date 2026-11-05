package com.mmcdole.kivo.android.settings

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.R
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.ui.focusKey
import com.mmcdole.kivo.android.ui.rememberFocusMemory
import kotlinx.coroutines.flow.map

/** What the Account page asks before doing: each signs this TV out. */
private enum class AccountAction(@param:StringRes val title: Int, @param:StringRes val action: Int, @param:StringRes val help: Int?) {
    SwitchAccount(R.string.switch_account_confirm, R.string.switch_account, R.string.switch_account_help),
    ChangeServer(R.string.change_server_confirm, R.string.change_server, R.string.change_server_help),
    SignOut(R.string.sign_out_this_tv_confirm, R.string.sign_out, null),
}

/**
 * Account: who's signed in, to which server, then Switch account and
 * Change server, which sign this TV out for a sign-in code or for choosing
 * a server, Link a device, and Sign out. Each sign-out asks first.
 */
@Composable
internal fun AccountPage(session: ServerSession) {
    val user by session.session.user.collectAsStateWithLifecycle()
    val serverName by remember(session) {
        session.graph.core.servers.map { all -> all.firstOrNull { it.id == session.session.savedServer.value.id }?.name }
    }.collectAsStateWithLifecycle(null)
    val memory = rememberFocusMemory()
    var confirming by remember { mutableStateOf<AccountAction?>(null) }
    var linking by remember { mutableStateOf(false) }

    SettingsPage(SettingsGroup.Account, memory, initial = "switchAccount") {
        item {
            Column(Modifier.padding(horizontal = Kivo.space(4)).padding(bottom = Kivo.space(2))) {
                Text(user?.username.orEmpty(), style = TvType.heading, color = Kivo.colors.text)
                serverName?.let { Text(stringResource(R.string.signed_in_to, it), style = TvType.body, color = Kivo.colors.textMuted) }
            }
        }
        item {
            ActionRow(stringResource(R.string.switch_account), { confirming = AccountAction.SwitchAccount }, Modifier.focusKey(memory, "switchAccount"))
        }
        item {
            ActionRow(stringResource(R.string.change_server), { confirming = AccountAction.ChangeServer }, Modifier.focusKey(memory, "changeServer"))
        }
        item {
            ActionRow(
                stringResource(R.string.link_a_device),
                { linking = true },
                Modifier.focusKey(memory, "link").testTag("linkADevice"),
                opensPage = true,
            )
        }
        item {
            ActionRow(stringResource(R.string.sign_out), { confirming = AccountAction.SignOut }, Modifier.focusKey(memory, "signOut"))
        }
    }

    confirming?.let { action ->
        Confirm(
            title = stringResource(action.title),
            action = stringResource(action.action),
            help = action.help?.let { stringResource(it) },
            onConfirm = {
                when (action) {
                    AccountAction.SwitchAccount, AccountAction.SignOut -> session.signOut()
                    AccountAction.ChangeServer -> session.signOutAndChangeServer()
                }
            },
            onDismiss = { confirming = null },
        )
    }
    if (linking) {
        val link = remember(session) { SessionLink(session) }
        LinkDevicePanel(username = user?.username.orEmpty(), actions = link, onDismiss = { linking = false })
    }
}

/** Link a device's answers, as the signed-in user. */
private class SessionLink(private val session: ServerSession) : LinkActions {
    override suspend fun pendingDevice(code: String) = session.session.link.pendingDevice(code)

    override suspend fun approve(code: String) = session.session.link.approve(code)

    override suspend fun deny(code: String) = session.session.link.deny(code)
}
