package com.mmcdole.kivo.android.mobile.signin

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.History
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.google.zxing.BarcodeFormat
import com.google.zxing.qrcode.QRCodeWriter
import com.mmcdole.kivo.android.AppGraph
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.platform.R as PlatformR
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.android.mobile.ui.Incompatible
import com.mmcdole.kivo.android.mobile.ui.sentence
import com.mmcdole.kivo.core.auth.SavedServer

/** Whether the system asks before the app reaches the local network: Android 17 and later. */
private val askForLocalNetwork = Build.VERSION.SDK_INT >= 37

/**
 * Signing in: choose a server, the last signed in to, one found on the
 * network or a typed address; then its password, or a code a signed-in
 * device approves. A server the app doesn't work with says which of them
 * needs an update, in place of its password. A line under the last server,
 * and over its password, warns when its session ended as the server's
 * identity changed ([identityChanged]).
 */
@Composable
fun SignInScreen(graph: AppGraph, saved: SavedServer?, identityChanged: Boolean = false) {
    val model = viewModel {
        SignInViewModel(graph.core, graph::findServers, graph.name, saved, identityChanged) { graph.core.signedIn(it) }
    }
    val state by model.state.collectAsStateWithLifecycle()
    BackHandler(enabled = state.step != SignInStep.Find) { model.back() }
    when (val step = state.step) {
        is SignInStep.Incompatible -> Incompatible(step.problem, step.server.info.name, model::retryCompatibility, onChangeServer = { model.back() })
        else -> SignInContent(state, model)
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SignInContent(state: SignInUiState, model: SignInViewModel) {
    val step = state.step
    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Text(
                        stringResource(if (step is SignInStep.Link) R.string.mobile_sign_in_another_device else R.string.mobile_sign_in),
                    )
                },
                navigationIcon = {
                    if (step != SignInStep.Find) {
                        IconButton(onClick = { model.back() }) { Icon(Icons.AutoMirrored.Filled.ArrowBack, stringResource(R.string.mobile_back)) }
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background),
            )
        },
    ) { padding ->
        Column(
            Modifier
                .fillMaxSize()
                .padding(padding)
                .imePadding()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = space(4), vertical = space(2)),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Column(Modifier.widthIn(max = 480.dp).fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(space(3))) {
                when (step) {
                    SignInStep.Find -> FindServer(state, model)
                    is SignInStep.Password -> Password(state, step, model)
                    is SignInStep.Link -> LinkCode(state.link, step.server.info.name, model::retryLink)
                    is SignInStep.Incompatible -> Unit
                }
            }
        }
    }
}

@Composable
private fun FindServer(state: SignInUiState, model: SignInViewModel) {
    val context = LocalContext.current
    var refused by rememberSaveable { mutableStateOf(false) }
    val ask = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
        if (granted) model.search() else refused = true
    }
    LaunchedEffect(Unit) {
        val granted = !askForLocalNetwork || context.checkSelfPermission(Manifest.permission.ACCESS_LOCAL_NETWORK) == PackageManager.PERMISSION_GRANTED
        if (granted) model.search() else ask.launch(Manifest.permission.ACCESS_LOCAL_NETWORK)
    }
    Image(painterResource(R.drawable.mobile_logo_lockup), stringResource(PlatformR.string.app_name), Modifier.padding(vertical = space(4)).height(28.dp))
    val saved = state.saved
    if (saved != null) {
        SectionLabel(stringResource(R.string.mobile_last_signed_in))
        ServerRow(saved.name, null, Icons.Default.History, enabled = !state.busy, tag = "saved") { model.chooseSaved() }
        if (state.identityChanged) IdentityChanged()
    }
    SectionLabel(stringResource(R.string.mobile_on_this_network))
    for (server in state.found.servers) {
        ServerRow(server.info.name, server.baseUrl.substringAfter("://"), Icons.Default.Dns, enabled = !state.busy, tag = "server:${server.info.name}") {
            model.choose(server)
        }
    }
    val found = state.found
    when {
        refused -> Muted(stringResource(R.string.mobile_local_network_refused))
        found.searching -> Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(space(3))) {
            CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
            Muted(stringResource(R.string.mobile_looking_for_servers))
        }
        found.started && found.servers.isEmpty() -> Muted(stringResource(R.string.mobile_no_servers_found))
    }
    HorizontalDivider(Modifier.padding(vertical = space(2)))
    SectionLabel(stringResource(R.string.mobile_enter_address))
    OutlinedTextField(
        value = state.address,
        onValueChange = model::setAddress,
        modifier = Modifier.fillMaxWidth().testTag("address"),
        placeholder = { Text(stringResource(R.string.mobile_address_hint)) },
        singleLine = true,
        enabled = !state.busy,
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri, imeAction = ImeAction.Go, autoCorrectEnabled = false, capitalization = KeyboardCapitalization.None),
        keyboardActions = KeyboardActions(onGo = { model.connect() }),
    )
    Button(onClick = model::connect, enabled = state.address.isNotBlank() && !state.busy, modifier = Modifier.fillMaxWidth()) {
        Text(stringResource(if (state.busy) R.string.mobile_connecting else R.string.mobile_continue_))
    }
    ErrorLine(state)
}

@Composable
private fun Password(state: SignInUiState, step: SignInStep.Password, model: SignInViewModel) {
    var username by rememberSaveable { mutableStateOf("") }
    var password by rememberSaveable { mutableStateOf("") }
    val submit = { model.signIn(username, password) }
    Spacer(Modifier.height(space(2)))
    Text(stringResource(R.string.mobile_sign_in_to, step.server.info.name), style = MaterialTheme.typography.headlineSmall)
    Muted(step.server.baseUrl.substringAfter("://"))
    if (state.identityChanged && step.server.info.id == state.saved?.id) IdentityChanged()
    OutlinedTextField(
        value = username,
        onValueChange = { username = it },
        modifier = Modifier.fillMaxWidth().testTag("username"),
        label = { Text(stringResource(R.string.mobile_username)) },
        singleLine = true,
        enabled = !state.busy,
        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Next, autoCorrectEnabled = false, capitalization = KeyboardCapitalization.None),
    )
    OutlinedTextField(
        value = password,
        onValueChange = { password = it },
        modifier = Modifier.fillMaxWidth().testTag("password"),
        label = { Text(stringResource(R.string.mobile_password)) },
        singleLine = true,
        enabled = !state.busy,
        visualTransformation = PasswordVisualTransformation(),
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Go),
        keyboardActions = KeyboardActions(onGo = { submit() }),
    )
    Button(
        onClick = submit,
        enabled = username.isNotBlank() && password.isNotEmpty() && !state.busy,
        modifier = Modifier.fillMaxWidth().testTag("signIn"),
    ) {
        Text(stringResource(if (state.busy) R.string.mobile_signing_in else R.string.mobile_sign_in))
    }
    ErrorLine(state)
    HorizontalDivider(Modifier.padding(vertical = space(2)))
    OutlinedButton(onClick = model::useAnotherDevice, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.mobile_sign_in_another_device)) }
    Muted(stringResource(R.string.mobile_another_device_hint))
}

/** The code to approve on a signed-in device, and its link as a QR code, replaced as each expires. */
@Composable
private fun LinkCode(link: LinkUi, serverName: String, onRetry: () -> Unit) {
    val problem = link.problem
    Spacer(Modifier.height(space(2)))
    if (problem != null) {
        Text(stringResource(R.string.mobile_couldnt_get_code), style = MaterialTheme.typography.titleMedium)
        Muted(sentence(problem))
        Button(onClick = onRetry) { Text(stringResource(R.string.mobile_try_again)) }
        return
    }
    Text(stringResource(R.string.mobile_link_code_hint), style = MaterialTheme.typography.bodyLarge)
    if (link.declined) Text(stringResource(R.string.mobile_sign_in_declined), color = MaterialTheme.colorScheme.error)
    Column(Modifier.fillMaxWidth(), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(space(4))) {
        val url = link.linkUrl
        if (url != null) QrCode(url, 220.dp) else Spacer(Modifier.size(220.dp))
        Text(
            link.code?.let(::groupedCode) ?: "",
            Modifier.testTag("linkCode"),
            style = MaterialTheme.typography.displaySmall.copy(fontFamily = FontFamily.Monospace),
        )
        Muted(serverName)
    }
}

/** A QR code of [text], dark on light. */
@Composable
private fun QrCode(text: String, size: Dp) {
    val matrix = remember(text) { QRCodeWriter().encode(text, BarcodeFormat.QR_CODE, 0, 0) }
    Canvas(Modifier.size(size).clip(RoundedCornerShape(12.dp)).background(Color.White).padding(space(3))) {
        val cell = this.size.width / matrix.width
        for (y in 0 until matrix.height) {
            for (x in 0 until matrix.width) {
                if (matrix.get(x, y)) drawRect(Color.Black, Offset(x * cell, y * cell), Size(cell + 0.5f, cell + 0.5f))
            }
        }
    }
}

@Composable
private fun ServerRow(name: String, address: String?, icon: ImageVector, enabled: Boolean, tag: String, onClick: () -> Unit) {
    ListItem(
        headlineContent = { Text(name) },
        supportingContent = address?.let { { Text(it) } },
        leadingContent = { Icon(icon, null) },
        colors = ListItemDefaults.colors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow),
        modifier = Modifier
            .clip(MaterialTheme.shapes.medium)
            .testTag(tag)
            .clickable(enabled = enabled, onClick = onClick),
    )
}

@Composable
private fun SectionLabel(text: String) {
    Text(text, style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.primary, modifier = Modifier.padding(top = space(2)))
}

@Composable
private fun Muted(text: String) {
    Text(text, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
}

/** The warning that the last server's session ended as its identity changed. */
@Composable
private fun IdentityChanged() {
    Text(
        stringResource(R.string.mobile_identity_changed),
        color = MaterialTheme.colorScheme.error,
        style = MaterialTheme.typography.bodyMedium,
        modifier = Modifier.testTag("identityChanged"),
    )
}

@Composable
private fun ErrorLine(state: SignInUiState) {
    val text = when (val error = state.error) {
        null -> return
        is SignInError.CantReach -> stringResource(R.string.mobile_cant_reach_saved, error.name)
        SignInError.NoServer -> stringResource(R.string.mobile_no_server_at_address)
        is SignInError.Problem -> sentence(error.problem)
    }
    Text(text, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.testTag("signInError"))
}
