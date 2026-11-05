package com.mmcdole.kivo.android.signin

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.AppGraph
import com.mmcdole.kivo.android.R
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.ui.EmptyState
import com.mmcdole.kivo.android.ui.FocusOnOpen
import com.mmcdole.kivo.android.ui.KivoButton
import com.mmcdole.kivo.android.ui.Loading
import com.mmcdole.kivo.android.ui.QrCode
import com.mmcdole.kivo.android.ui.TextInput
import com.mmcdole.kivo.android.ui.display
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.display.problem
import com.mmcdole.kivo.core.session.LinkProgress
import com.mmcdole.kivo.core.session.KivoSession
import com.mmcdole.kivo.core.auth.SavedServer
import com.mmcdole.kivo.core.discovery.FoundServer
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.launch

/** Each server is signed in to where it answered ([FoundServer.baseUrl]). */
private sealed interface Step {
    data object Finding : Step

    data object Address : Step

    data class Code(val server: FoundServer) : Step

    data class Password(val server: FoundServer) : Step
}

/**
 * Signing in on the TV: the saved server first, then the servers found on
 * the network, choosing the one found by itself and listing several, with
 * "Enter address" always a step away; then the sign-in code, with the
 * password as the alternative. A session that ended goes straight to the
 * code on its server.
 */
@Composable
fun SignInScreen(graph: AppGraph, server: SavedServer?) {
    var step by remember { mutableStateOf<Step>(Step.Finding) }
    var reachingSaved by remember { mutableStateOf(server != null) }
    LaunchedEffect(server) {
        if (server == null) return@LaunchedEffect
        val found = graph.core.reach(server)
        reachingSaved = false
        if (found != null) step = Step.Code(found)
    }
    if (reachingSaved) {
        Loading()
        return
    }

    // A form sits at the top, clear of the keyboard, which opens over the lower half of the screen.
    val alignment = if (step is Step.Code) Alignment.Center else Alignment.TopStart
    Box(Modifier.fillMaxSize().padding(horizontal = 96.dp, vertical = 48.dp), contentAlignment = alignment) {
        when (val s = step) {
            Step.Finding -> Finding(graph, onChoose = { step = Step.Code(it) }, onAddress = { step = Step.Address })
            Step.Address -> Address(graph, onFound = { step = Step.Code(it) }, onBack = { step = Step.Finding })
            is Step.Code -> key(s.server) {
                LinkCode(
                    deviceName = graph.name,
                    serverName = s.server.info.name,
                    link = { graph.core.signInByLink(s.server, graph.name) },
                    onSignedIn = { graph.open(it) },
                    onPassword = { step = Step.Password(s.server) },
                    onChangeServer = { step = Step.Finding },
                )
            }
            is Step.Password -> Password(graph, s.server, onBack = { step = Step.Code(s.server) })
        }
    }
}

/** Whether the system asks the user before the app reaches the local network: Android 17 and later. */
private val askForLocalNetwork = Build.VERSION.SDK_INT >= LOCAL_NETWORK_SDK

/** The Android version that brought the local network permission. */
private const val LOCAL_NETWORK_SDK = 37

/** Where finding servers is: waiting for the user's leave to look on the network, looking, or done. */
private enum class Search { Asking, Refused, Searching, Done }

/**
 * Finding servers on the network. Where the system asks first, a line
 * says why before its prompt; refused, the address is the way in.
 */
@Composable
private fun Finding(graph: AppGraph, onChoose: (FoundServer) -> Unit, onAddress: () -> Unit) {
    val context = LocalContext.current
    val found = remember { mutableStateListOf<FoundServer>() }
    var search by remember {
        val granted = !askForLocalNetwork || context.checkSelfPermission(Manifest.permission.ACCESS_LOCAL_NETWORK) == PackageManager.PERMISSION_GRANTED
        mutableStateOf(if (granted) Search.Searching else Search.Asking)
    }
    val ask = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
        search = if (granted) Search.Searching else Search.Refused
    }
    LaunchedEffect(search) {
        if (search != Search.Searching) return@LaunchedEffect
        try {
            graph.findServers().collect { found += it }
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            // Nothing to browse on: the address is the way in.
        }
        search = Search.Done
        found.singleOrNull()?.let(onChoose)
    }
    val first = remember { FocusRequester() }
    Column(Modifier.widthIn(max = 640.dp), verticalArrangement = Arrangement.spacedBy(Kivo.space(3))) {
        Lockup(Modifier.padding(bottom = LockupGap - Kivo.space(3)))
        Text(stringResource(R.string.sign_in_title), style = TvType.title, color = Kivo.colors.text)
        val status = when {
            search == Search.Asking -> R.string.local_network_why
            search == Search.Refused -> R.string.local_network_refused
            search == Search.Searching -> R.string.looking_for_servers
            found.isEmpty() -> R.string.no_servers_found
            else -> R.string.choose_server
        }
        Text(stringResource(status), style = TvType.body, color = Kivo.colors.textMuted)
        if (search == Search.Asking) {
            KivoButton(
                stringResource(R.string.continue_),
                { if (askForLocalNetwork) ask.launch(Manifest.permission.ACCESS_LOCAL_NETWORK) },
                Modifier.focusRequester(first),
                primary = true,
            )
        }
        for ((index, server) in found.withIndex()) {
            KivoButton(
                text = "${server.info.name} · ${server.baseUrl.removePrefix("http://").removePrefix("https://")}",
                onClick = { onChoose(server) },
                modifier = if (index == 0) Modifier.focusRequester(first) else Modifier,
                primary = index == 0,
            )
        }
        val addressFirst = search != Search.Asking && found.isEmpty()
        KivoButton(stringResource(R.string.enter_address), onAddress, if (addressFirst) Modifier.focusRequester(first) else Modifier)
    }
    FocusOnOpen(first, search, found.size)
}

@Composable
private fun Address(graph: AppGraph, onFound: (FoundServer) -> Unit, onBack: () -> Unit) {
    val scope = rememberCoroutineScope()
    var address by remember { mutableStateOf("") }
    var error by remember { mutableStateOf<RequestProblem?>(null) }
    var busy by remember { mutableStateOf(false) }
    val field = remember { FocusRequester() }
    BackHandler(onBack = onBack)
    val connect = {
        if (address.isNotBlank() && !busy) {
            busy = true
            error = null
            scope.launch {
                try {
                    onFound(graph.core.serverAt(address))
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    error = RequestProblem.of(e)
                }
                busy = false
            }
        }
    }
    Column(Modifier.widthIn(max = 640.dp), verticalArrangement = Arrangement.spacedBy(Kivo.space(3))) {
        Lockup(Modifier.padding(bottom = LockupGap - Kivo.space(3)))
        Text(stringResource(R.string.enter_address), style = TvType.title, color = Kivo.colors.text)
        TextInput(
            label = stringResource(R.string.server_address),
            value = address,
            onValueChange = { address = it },
            keyboardType = KeyboardType.Uri,
            imeAction = ImeAction.Go,
            onDone = connect,
            modifier = Modifier.focusRequester(field),
        )
        Text(stringResource(R.string.address_hint), style = TvType.small, color = Kivo.colors.textMuted)
        // Anything but a problem document means no Kivo server answered there.
        error?.let { Text(if (it is RequestProblem.Refused && it.problem != null) display.problem(it) else stringResource(R.string.address_unreachable), style = TvType.body, color = Kivo.colors.danger) }
        Row(horizontalArrangement = Arrangement.spacedBy(Kivo.space(3))) {
            KivoButton(stringResource(if (busy) R.string.connecting else R.string.connect), connect, primary = true, enabled = !busy)
            KivoButton(stringResource(R.string.back), onBack)
        }
    }
    FocusOnOpen(field)
}

/**
 * The sign-in code: a QR code of its link, the link's address and the code,
 * until a signed-in device answers it. [link] starts a device code; a code
 * that expires is replaced, and one denied says so over its replacement.
 * [onSignedIn] opens the server's session once a code is approved.
 */
@Composable
internal fun LinkCode(
    deviceName: String,
    serverName: String,
    link: () -> Flow<LinkProgress>,
    onSignedIn: suspend (KivoSession) -> Unit,
    onPassword: () -> Unit,
    onChangeServer: () -> Unit,
) {
    var waiting by remember { mutableStateOf<LinkProgress.Waiting?>(null) }
    var declined by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<RequestProblem?>(null) }
    var attempt by remember { mutableIntStateOf(0) }
    LaunchedEffect(attempt) {
        error = null
        // An expired or denied code is replaced by another.
        while (true) {
            var again = false
            link().collect { progress ->
                when (progress) {
                    is LinkProgress.Waiting -> waiting = progress
                    is LinkProgress.SignedIn -> onSignedIn(progress.session)
                    LinkProgress.Denied -> {
                        declined = true
                        again = true
                    }
                    LinkProgress.Expired -> again = true
                    is LinkProgress.Failed -> error = progress.problem
                }
            }
            if (!again) break
        }
    }
    val password = remember { FocusRequester() }
    BackHandler(onBack = onChangeServer)

    val failed = error
    if (failed != null) {
        EmptyState(stringResource(R.string.couldnt_get_sign_in_code), display.problem(failed)) {
            KivoButton(stringResource(R.string.try_again), { attempt++ }, Modifier.focusRequester(password), primary = true)
            KivoButton(stringResource(R.string.sign_in_with_password), onPassword)
        }
        FocusOnOpen(password)
        return
    }

    Column(verticalArrangement = Arrangement.spacedBy(Kivo.space(6))) {
        Column {
            Lockup(Modifier.padding(bottom = LockupGap))
            Text(stringResource(R.string.sign_in_to_kivo_on, deviceName), style = TvType.title, color = Kivo.colors.text)
            if (declined) {
                Text(stringResource(R.string.sign_in_declined), style = TvType.body, color = Kivo.colors.danger, modifier = Modifier.testTag("signInDeclined"))
            }
        }
        val code = waiting
        Row(verticalAlignment = Alignment.CenterVertically) {
            if (code != null) QrCode(code.linkUrl, QrSize)
            Spacer(Modifier.width(Kivo.space(12)))
            Column(verticalArrangement = Arrangement.spacedBy(Kivo.space(2))) {
                if (code == null) {
                    Text(stringResource(R.string.loading), style = TvType.body, color = Kivo.colors.textMuted)
                } else {
                    Text(stringResource(R.string.link_code_scan), style = TvType.item, color = Kivo.colors.textMuted)
                    Text(code.linkUrl.substringBefore('?').substringAfter("://"), style = TvType.heading, color = Kivo.colors.text)
                    Spacer(Modifier.height(Kivo.space(2)))
                    Text(stringResource(R.string.link_code_enter), style = TvType.item, color = Kivo.colors.textMuted)
                    Text(groupedCode(code.code), style = TvType.display, color = Kivo.colors.accent, modifier = Modifier.testTag("linkCode"))
                    Text(serverName, style = TvType.small, color = Kivo.colors.textSubtle)
                }
            }
        }
        Spacer(Modifier.height(Kivo.space(2)))
        Row(horizontalArrangement = Arrangement.spacedBy(Kivo.space(3))) {
            KivoButton(stringResource(R.string.sign_in_with_password), onPassword, Modifier.focusRequester(password))
            KivoButton(stringResource(R.string.change_server), onChangeServer)
        }
    }
    FocusOnOpen(password)
}

/** A code as people read it: two groups of three, "K7Q-2M9". */
internal fun groupedCode(code: String): String = if (code.length == 6) code.take(3) + "-" + code.drop(3) else code

/**
 * Signing in with a password, a step at a time, each field with the
 * keyboard to itself: the username, then Next, then the password, which
 * names the username it goes with. Back from the password returns to the
 * username, kept as typed.
 */
@Composable
private fun Password(graph: AppGraph, server: FoundServer, onBack: () -> Unit) {
    val scope = rememberCoroutineScope()
    var username by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var askingPassword by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<RequestProblem?>(null) }
    var busy by remember { mutableStateOf(false) }
    val field = remember { FocusRequester() }
    BackHandler { if (askingPassword) askingPassword = false else onBack() }
    val next = { if (username.isNotBlank()) askingPassword = true }
    val submit = {
        if (username.isNotBlank() && password.isNotEmpty() && !busy) {
            busy = true
            error = null
            scope.launch {
                try {
                    graph.open(graph.core.signIn(server, username.trim(), password, graph.name))
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    error = RequestProblem.of(e)
                    busy = false
                }
            }
        }
    }
    Column(Modifier.widthIn(max = 560.dp).fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(Kivo.space(3))) {
        Lockup(Modifier.padding(bottom = LockupGap - Kivo.space(3)))
        Column {
            Text(stringResource(R.string.sign_in_to, server.info.name), style = TvType.title, color = Kivo.colors.text)
            if (askingPassword) Text(stringResource(R.string.signing_in_as, username.trim()), style = TvType.body, color = Kivo.colors.textMuted)
        }
        if (askingPassword) {
            TextInput(stringResource(R.string.password), password, { password = it }, Modifier.focusRequester(field), password = true, imeAction = ImeAction.Done, onDone = submit)
            error?.let { Text(display.problem(it), style = TvType.body, color = Kivo.colors.danger) }
            Row(horizontalArrangement = Arrangement.spacedBy(Kivo.space(3))) {
                KivoButton(stringResource(if (busy) R.string.signing_in else R.string.sign_in), submit, primary = true, enabled = !busy && password.isNotEmpty())
                KivoButton(stringResource(R.string.back), { askingPassword = false })
            }
        } else {
            TextInput(stringResource(R.string.username), username, { username = it }, Modifier.focusRequester(field), onNext = next)
            Row(horizontalArrangement = Arrangement.spacedBy(Kivo.space(3))) {
                KivoButton(stringResource(R.string.next), next, primary = true, enabled = username.isNotBlank())
                KivoButton(stringResource(R.string.back), onBack)
            }
        }
    }
    FocusOnOpen(field, askingPassword)
}

/** Kivo's logo, over each sign-in step's title. */
@Composable
private fun Lockup(modifier: Modifier = Modifier) {
    Image(painterResource(R.drawable.logo_lockup), stringResource(R.string.app_name), modifier.height(LockupHeight))
}

/** The logo's height, read from across the room without outweighing the title. */
private val LockupHeight = Kivo.space(9)

/** The space between the logo and the title under it. */
private val LockupGap = Kivo.space(6)

/** The sign-in code's QR code's size: big enough to scan from a seat across the room. */
private val QrSize = 240.dp

/** The saved server answers at none of its addresses: retrying, with Retry now and Change server. */
@Composable
fun UnreachableScreen(graph: AppGraph, server: SavedServer) {
    com.mmcdole.kivo.android.ui.Failed(
        problem = RequestProblem.Unreachable,
        server = server.name,
        onRetry = { graph.reach(server) },
        onChangeServer = { graph.changeServer() },
    )
}
