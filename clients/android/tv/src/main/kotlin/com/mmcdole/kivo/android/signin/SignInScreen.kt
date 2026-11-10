package com.mmcdole.kivo.android.signin

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
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
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.AppGraph
import com.mmcdole.kivo.android.tv.R
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.ui.EmptyState
import com.mmcdole.kivo.android.ui.FocusOnOpen
import com.mmcdole.kivo.android.ui.Incompatible
import com.mmcdole.kivo.android.ui.KivoButton
import com.mmcdole.kivo.android.ui.Loading
import com.mmcdole.kivo.android.ui.QrCode
import com.mmcdole.kivo.android.ui.TextInput
import com.mmcdole.kivo.android.ui.display
import com.mmcdole.kivo.core.Compatibility
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
internal sealed interface Step {
    data object Finding : Step

    data object Address : Step

    data class Code(val server: FoundServer) : Step

    data class Password(val server: FoundServer) : Step

    /** The app and [server] don't work together, for [problem]: one of them needs an update. */
    data class Incompatible(val server: FoundServer, val problem: Compatibility) : Step
}

/** Where a server found goes, by how the app and it stand: its sign-in code when they work together, else why they don't. */
internal fun stepFor(server: FoundServer, compatibility: Compatibility): Step =
    if (compatibility == Compatibility.Compatible) Step.Code(server) else Step.Incompatible(server, compatibility)

private fun AppGraph.stepFor(server: FoundServer): Step = stepFor(server, core.compatibility(server.info))

/**
 * Signing in on the TV: the saved server first, then the servers found on
 * the network, choosing the one found by itself and listing several, with
 * "Enter address" always a step away; then the sign-in code, with the
 * password as the alternative. A session that ended goes straight to the
 * code on its server. A server the app doesn't work with says which of
 * them needs an update, in place of its code.
 */
@Composable
fun SignInScreen(graph: AppGraph, server: SavedServer?) {
    var step by remember { mutableStateOf<Step>(Step.Finding) }
    var reachingSaved by remember { mutableStateOf(server != null) }
    LaunchedEffect(server) {
        if (server == null) return@LaunchedEffect
        val found = graph.core.reach(server)
        reachingSaved = false
        if (found != null) step = graph.stepFor(found)
    }
    if (reachingSaved) {
        Loading()
        return
    }

    // A form sits at the top, clear of the keyboard, which opens over the lower half of the screen.
    val centred = step is Step.Code || step is Step.Incompatible
    Box(Modifier.fillMaxSize().padding(top = if (centred) 0.dp else FormTop), contentAlignment = if (centred) Alignment.Center else Alignment.TopCenter) {
        when (val s = step) {
            Step.Finding -> FormPanel { Finding(graph, onChoose = { step = graph.stepFor(it) }, onAddress = { step = Step.Address }) }
            Step.Address -> FormPanel { Address(graph, onFound = { step = graph.stepFor(it) }, onBack = { step = Step.Finding }) }
            is Step.Code -> key(s.server) {
                LinkCode(
                    serverName = s.server.info.name,
                    link = { graph.core.signInByLink(s.server, graph.name) },
                    onSignedIn = { graph.open(it) },
                    onPassword = { step = Step.Password(s.server) },
                    onChangeServer = { step = Step.Finding },
                )
            }
            is Step.Password -> FormPanel { Password(graph, s.server, onBack = { step = Step.Code(s.server) }) }
            is Step.Incompatible -> {
                val scope = rememberCoroutineScope()
                BackHandler { step = Step.Finding }
                Incompatible(
                    s.problem,
                    s.server.info.name,
                    onRetry = {
                        scope.launch {
                            try {
                                step = graph.stepFor(graph.core.serverAt(s.server.baseUrl))
                            } catch (e: CancellationException) {
                                throw e
                            } catch (_: Exception) {
                                // It stays as it last said it is.
                            }
                        }
                    },
                    onChangeServer = { step = Step.Finding },
                )
            }
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
 * The sign-in code, in a panel: the logo, the heading, how to use it and the
 * code, with Sign in with password below, focused on arrival, and a QR code
 * of its link at the right, until a signed-in device answers it. [link] starts a device code; a code
 * that expires is replaced, and one denied says so over its replacement.
 * [onSignedIn] opens the server's session once a code is approved.
 */
@Composable
internal fun LinkCode(
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

    val code = waiting
    Row(
        Modifier
            .width(CodePanelWidth)
            .heightIn(min = CodePanelHeight)
            .signInPanel()
            .padding(CodePanelPadding),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(Modifier.width(CodeStepsWidth)) {
            Lockup(Modifier.padding(bottom = LockupGap))
            Text(stringResource(R.string.sign_in_title), style = TvType.title, color = Kivo.colors.text)
            Spacer(Modifier.height(CodeStepsGap))
            if (declined) {
                Text(stringResource(R.string.sign_in_declined), style = TvType.body, color = Kivo.colors.danger, modifier = Modifier.testTag("signInDeclined"))
                Spacer(Modifier.height(CodeStepsGap))
            }
            Text(stringResource(R.string.link_code_hint), style = TvType.body, color = Kivo.colors.textMuted)
            Spacer(Modifier.height(CodeStepsGap))
            // While the code is on its way its place is kept, so nothing moves as it arrives.
            Box(Modifier.height(CodeHeight), contentAlignment = Alignment.CenterStart) {
                if (code == null) {
                    Text(stringResource(R.string.loading), style = TvType.body, color = Kivo.colors.textMuted)
                } else {
                    Text(groupedCode(code.code), style = CodeStyle, color = Kivo.colors.text, maxLines = 1, modifier = Modifier.testTag("linkCode"))
                }
            }
            Box(Modifier.height(CodeActionsGap), contentAlignment = Alignment.CenterStart) {
                Text(serverName, style = TvType.small, color = Kivo.colors.textSubtle, maxLines = 1)
            }
            Row(horizontalArrangement = Arrangement.spacedBy(Kivo.space(3))) {
                KivoButton(stringResource(R.string.sign_in_with_password), onPassword, Modifier.focusRequester(password).height(CodeActionHeight).widthIn(min = PasswordWidth))
                KivoButton(stringResource(R.string.change_server), onChangeServer, Modifier.height(CodeActionHeight))
            }
        }
        Spacer(Modifier.width(CodeQrGap))
        if (code != null) {
            QrCode(code.linkUrl, QrSize)
        } else {
            Box(Modifier.size(QrSize).background(Kivo.colors.surfaceRaised, RoundedCornerShape(KivoTokens.Radius.md)))
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
    Image(painterResource(R.drawable.logo_lockup), stringResource(com.mmcdole.kivo.android.platform.R.string.app_name), modifier.height(LockupHeight))
}

/** A panel of the sign-in steps: the surface, its edge and its corners, flat. */
@Composable
private fun Modifier.signInPanel(): Modifier {
    val shape = RoundedCornerShape(KivoTokens.Radius.lg)
    return background(Kivo.colors.surface, shape).border(1.dp, Kivo.colors.border, shape)
}

/** A form's panel, under [FormTop]. */
@Composable
private fun FormPanel(content: @Composable () -> Unit) {
    Box(Modifier.width(FormWidth).signInPanel().padding(FormPadding)) { content() }
}

/** How far a form's panel sits from the top of the screen. */
private val FormTop = 24.dp

/** A form's panel's width. */
private val FormWidth = 640.dp

/** A form's panel's padding, smaller than the code's, to leave a form clear of the keyboard. */
private val FormPadding = 32.dp

/** The logo's height, read from across the room without outweighing the title. */
private val LockupHeight = 24.5.dp

/** The space between the logo and the title under it. */
private val LockupGap = 21.dp

// The code's panel on the TV canvas, a dp being two of the Samsung TV app's pixels.

/** The code's panel's size: the same whether the code has come yet or not; a declined code's line can grow it. */
private val CodePanelWidth = 785.dp
private val CodePanelHeight = 370.5.dp
private val CodePanelPadding = 42.5.dp

/** The column of the logo, the heading, the steps and the actions. */
private val CodeStepsWidth = 420.dp

/** The gap between the heading, each step and the code. */
private val CodeStepsGap = 21.dp

/** The code's line. */
private val CodeHeight = 44.dp

/** The code in monospace, so its letters and digits can't be mistaken for each other across the room. */
private val CodeStyle = TvType.display.copy(fontFamily = FontFamily.Monospace, fontWeight = KivoTokens.FontWeights.semibold)

/** The gap between the code and the actions under it, which holds the server's name. */
private val CodeActionsGap = 42.dp

/** The actions' height. */
private val CodeActionHeight = 42.dp

/** Sign in with password's width at least. */
private val PasswordWidth = 184.dp

/** The gap between the steps and the QR code. */
private val CodeQrGap = 56.dp

/** The sign-in code's QR code's size: big enough to scan from a seat across the room. */
private val QrSize = 224.dp

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
