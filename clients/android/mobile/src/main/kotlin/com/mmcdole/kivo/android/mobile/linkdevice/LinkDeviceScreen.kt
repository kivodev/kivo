package com.mmcdole.kivo.android.mobile.linkdevice

import android.text.format.DateUtils
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Cancel
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.app.MobileNav
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.android.mobile.ui.PageTopBar
import com.mmcdole.kivo.api.models.ClientPlatform
import com.mmcdole.kivo.api.models.PendingDevice

/**
 * Link a device: scan the QR code another device shows with the camera,
 * or enter its code; then "Allow Living Room to sign in as anna?" with
 * what the server saw of it, and Allow or Don't allow. A device asking
 * from outside the home network, or on another server, is said so.
 * Without a camera the code is typed.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun LinkDeviceScreen(session: ServerSession, link: String?, nav: MobileNav) {
    val context = LocalContext.current
    val camera = remember(context) { hasCamera(context) }
    val model = viewModel { LinkDeviceViewModel(session.session, session.graph.core.servers, link, camera) }
    val state by model.state.collectAsStateWithLifecycle()
    LinkDeviceContent(state, camera, model::show, model::lookUp, model::answer, nav::back)
}

/** Link a device as [state] has it: [camera] says whether it can scan, [onShow] switches between scanning and typing, [onDone] leaves. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun LinkDeviceContent(
    state: LinkUiState,
    camera: Boolean,
    onShow: (LinkStep) -> Unit,
    onLookUp: (String) -> Unit,
    onAnswer: (Boolean) -> Unit,
    onDone: () -> Unit,
) {
    Scaffold(topBar = { PageTopBar(stringResource(R.string.mobile_link_device), onDone) }) { padding ->
        Column(
            Modifier
                .fillMaxSize()
                .padding(padding)
                .verticalScroll(rememberScrollState())
                .padding(space(6))
                .testTag("linkDevice"),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(space(4)),
        ) {
            Column(Modifier.widthIn(max = 480.dp).fillMaxWidth(), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(space(4))) {
                when (val step = state.step) {
                    LinkStep.Scan -> Scan(onLookUp) { onShow(LinkStep.Enter) }
                    LinkStep.Enter -> Enter(state.busy, onLookUp, if (camera) ({ onShow(LinkStep.Scan) }) else null)
                    is LinkStep.OtherServer -> OtherServer(step, state.serverName, onDone)
                    is LinkStep.Confirm -> Confirm(step.device, state.username, state.busy, onAnswer)
                    is LinkStep.Done -> Done(step, onDone)
                }
                state.message?.let { Text(words(it), color = MaterialTheme.colorScheme.error, textAlign = TextAlign.Center, modifier = Modifier.testTag("linkMessage")) }
                if (state.busy) CircularProgressIndicator()
            }
        }
    }
}

/** The camera, reading the other device's QR code, or Enter code instead. */
@Composable
private fun Scan(onScanned: (String) -> Unit, onEnter: () -> Unit) {
    QrScanner(onScanned, Modifier.fillMaxWidth().aspectRatio(1f).clip(MaterialTheme.shapes.large).testTag("scanner"))
    Text(stringResource(R.string.mobile_scan_hint), color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center)
    TextButton(onClick = onEnter, Modifier.testTag("enterCode")) { Text(stringResource(R.string.mobile_enter_code_instead)) }
}

/** Type the code and Continue, or Scan the QR code instead with [scan], null without a camera. */
@Composable
private fun Enter(busy: Boolean, onContinue: (String) -> Unit, scan: (() -> Unit)?) {
    var typed by rememberSaveable { mutableStateOf("") }
    OutlinedTextField(
        value = typed,
        onValueChange = { typed = it.uppercase() },
        label = { Text(stringResource(R.string.mobile_code)) },
        singleLine = true,
        textStyle = MaterialTheme.typography.headlineSmall.copy(fontFamily = FontFamily.Monospace, textAlign = TextAlign.Center),
        keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Characters, autoCorrectEnabled = false, imeAction = ImeAction.Go),
        keyboardActions = KeyboardActions(onGo = { if (typed.isNotBlank()) onContinue(typed) }),
        modifier = Modifier.fillMaxWidth().testTag("codeField"),
    )
    Text(stringResource(R.string.mobile_code_hint), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
    FilledTonalButton(onClick = { onContinue(typed) }, enabled = typed.isNotBlank() && !busy, modifier = Modifier.testTag("continue")) {
        Text(stringResource(R.string.mobile_continue_))
    }
    if (scan != null) {
        TextButton(onClick = scan, Modifier.testTag("scan")) { Text(stringResource(R.string.mobile_scan_instead)) }
    }
}

/** "Allow Living Room to sign in as anna?", what the server saw of it, a warning when it's away from home, and the answers. */
@Composable
private fun Confirm(device: PendingDevice, username: String, busy: Boolean, onAnswer: (Boolean) -> Unit) {
    Text(
        stringResource(R.string.mobile_allow_question, device.name, username),
        style = MaterialTheme.typography.headlineSmall,
        textAlign = TextAlign.Center,
        modifier = Modifier.testTag("linkQuestion"),
    )
    Text(context(device), color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center, modifier = Modifier.testTag("linkContext"))
    if (!device.nearby) {
        Row(horizontalArrangement = Arrangement.spacedBy(space(2))) {
            Icon(Icons.Default.Warning, null, tint = MaterialTheme.colorScheme.error)
            Text(stringResource(R.string.mobile_not_home_warning), color = MaterialTheme.colorScheme.error, modifier = Modifier.testTag("linkWarning"))
        }
    }
    Row(horizontalArrangement = Arrangement.spacedBy(space(3))) {
        OutlinedButton(onClick = { onAnswer(false) }, enabled = !busy, modifier = Modifier.testTag("deny")) { Text(stringResource(R.string.mobile_dont_allow)) }
        Button(onClick = { onAnswer(true) }, enabled = !busy, modifier = Modifier.testTag("allow")) { Text(stringResource(R.string.mobile_allow)) }
    }
}

@Composable
private fun OtherServer(step: LinkStep.OtherServer, serverName: String, onDone: () -> Unit) {
    Icon(Icons.Default.Warning, null, Modifier.size(44.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant)
    Text(
        step.name?.let { stringResource(R.string.mobile_signing_in_to_named, it) } ?: stringResource(R.string.mobile_signing_in_to_other),
        style = MaterialTheme.typography.headlineSmall,
        textAlign = TextAlign.Center,
    )
    Text(stringResource(R.string.mobile_other_server_hint, serverName), color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center)
    Button(onClick = onDone) { Text(stringResource(R.string.mobile_done)) }
}

@Composable
private fun Done(step: LinkStep.Done, onDone: () -> Unit) {
    Icon(
        if (step.allowed) Icons.Default.CheckCircle else Icons.Default.Cancel,
        null,
        Modifier.size(56.dp),
        tint = if (step.allowed) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant,
    )
    Text(
        stringResource(if (step.allowed) R.string.mobile_device_signed_in else R.string.mobile_device_not_signed_in, step.name),
        style = MaterialTheme.typography.headlineSmall,
        textAlign = TextAlign.Center,
        modifier = Modifier.testTag("linkDone"),
    )
    Button(onClick = onDone) { Text(stringResource(R.string.mobile_done)) }
}

@Composable
private fun words(message: LinkMessage): String = when (message) {
    LinkMessage.NotACode -> stringResource(R.string.mobile_not_a_code)
    LinkMessage.CodeFailed -> stringResource(R.string.mobile_code_failed)
    is LinkMessage.Problem -> message.sentence
}

/** What the server saw of the asking device: "Android TV · On your home network · Just now". */
@Composable
private fun context(device: PendingDevice): String {
    val started = device.startedAt.toEpochMilliseconds()
    val now = System.currentTimeMillis()
    val asked = if (now - started < DateUtils.MINUTE_IN_MILLIS) {
        stringResource(R.string.mobile_just_now)
    } else {
        DateUtils.getRelativeTimeSpanString(started, now, DateUtils.MINUTE_IN_MILLIS).toString()
    }
    return listOfNotNull(
        platformName(device.platform),
        stringResource(if (device.nearby) R.string.mobile_on_home_network else R.string.mobile_not_on_home_network),
        asked,
    ).joinToString(" · ")
}

/** The platforms a client says it runs on, for people. */
@Composable
fun platformName(platform: ClientPlatform?): String? = when (platform) {
    ClientPlatform.AndroidTv -> stringResource(R.string.mobile_platform_android_tv)
    ClientPlatform.AppleTv -> stringResource(R.string.mobile_platform_apple_tv)
    ClientPlatform.IPhone -> stringResource(R.string.mobile_platform_iphone)
    ClientPlatform.IPad -> stringResource(R.string.mobile_platform_ipad)
    ClientPlatform.AndroidPhone -> stringResource(R.string.mobile_platform_android_phone)
    ClientPlatform.Browser -> stringResource(R.string.mobile_platform_browser)
    else -> null
}
