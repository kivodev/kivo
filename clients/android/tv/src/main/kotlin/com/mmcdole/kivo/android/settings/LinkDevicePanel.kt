package com.mmcdole.kivo.android.settings

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.tv.R
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.ui.KivoButton
import com.mmcdole.kivo.android.ui.SidePanel
import com.mmcdole.kivo.android.ui.TextInput
import com.mmcdole.kivo.android.ui.display
import com.mmcdole.kivo.api.models.ClientPlatform
import com.mmcdole.kivo.core.ProblemCode
import com.mmcdole.kivo.core.Done
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.android.ui.focusWhenPlaced
import com.mmcdole.kivo.core.display.problem
import com.mmcdole.kivo.api.models.PendingDevice
import com.mmcdole.kivo.core.auth.LinkCode
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.launch
import kotlin.time.Clock
import kotlin.time.Duration.Companion.minutes
import kotlin.time.Instant

/** What Link a device does with a code: who is asking, and answering it as this user. */
internal interface LinkActions {
    suspend fun pendingDevice(code: String): PendingDevice

    suspend fun approve(code: String): Done<Unit>

    suspend fun deny(code: String): Done<Unit>
}

private sealed interface LinkStep {
    data object Enter : LinkStep

    data class Asking(val code: String, val request: PendingDevice) : LinkStep

    data class Answered(val name: String, val allowed: Boolean) : LinkStep
}

/**
 * Link a device: the code another device shows, typed with the remote's
 * keyboard in capitals, then "Allow Living Room to sign in as anna?" with
 * what it runs on, whether it's on the home network and when it asked, a
 * warning when it isn't on the home network, and Don't allow, focused
 * first, beside Allow; then what was done.
 */
@Composable
internal fun LinkDevicePanel(username: String, actions: LinkActions, onDismiss: () -> Unit) {
    val scope = rememberCoroutineScope()
    var step by remember { mutableStateOf<LinkStep>(LinkStep.Enter) }
    var busy by remember { mutableStateOf(false) }
    var problem by remember { mutableStateOf<String?>(null) }

    /** Runs [action], showing its problem, if any, under the panel. */
    fun run(action: suspend () -> Unit) {
        if (busy) return
        busy = true
        problem = null
        scope.launch {
            try {
                action()
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                problem = RequestProblem.of(e).code
            } finally {
                busy = false
            }
        }
    }

    /** Allows [asking] or doesn't; a refusal shows its problem and stays on the question. */
    suspend fun answer(asking: LinkStep.Asking, allowed: Boolean) {
        when (val done = if (allowed) actions.approve(asking.code) else actions.deny(asking.code)) {
            is Done.Ok -> step = LinkStep.Answered(asking.request.name, allowed)
            is Done.Failed -> problem = done.problem.code
        }
    }

    SidePanel(stringResource(R.string.link_a_device), onDismiss) {
        when (val s = step) {
            LinkStep.Enter -> EnterCode(busy) { typed ->
                val code = LinkCode.parseOrNull(typed)?.code
                if (code == null) {
                    problem = ProblemCode.NotFound.value
                } else {
                    run { step = LinkStep.Asking(code, actions.pendingDevice(code)) }
                }
            }
            is LinkStep.Asking -> Ask(
                username,
                s.request,
                busy,
                onDeny = { run { answer(s, allowed = false) } },
                onAllow = { run { answer(s, allowed = true) } },
            )
            is LinkStep.Answered -> Answered(s, onDismiss)
        }
        problem?.let {
            Text(linkProblem(it), style = TvType.small, color = Kivo.colors.danger, modifier = Modifier.testTag("linkProblem"))
        }
    }
}

/** A code that didn't work says to look for a new one; anything else is its problem. */
@Composable
private fun linkProblem(code: String): String =
    if (ProblemCode.of(code) == ProblemCode.NotFound) stringResource(R.string.link_code_didnt_work) else display.problem(code)

@Composable
private fun EnterCode(busy: Boolean, onContinue: (String) -> Unit) {
    var typed by remember { mutableStateOf("") }
    val field = remember { FocusRequester() }
    val submit = { if (typed.isNotBlank()) onContinue(typed) }
    Column(verticalArrangement = Arrangement.spacedBy(Kivo.space(3))) {
        Text(stringResource(R.string.link_a_device_help), style = TvType.body, color = Kivo.colors.textMuted)
        TextInput(
            stringResource(R.string.code),
            typed,
            { typed = it.take(MAX_TYPED) },
            Modifier.focusRequester(field),
            keyboardType = KeyboardType.Ascii,
            imeAction = ImeAction.Done,
            onDone = submit,
            keyboardSide = Alignment.Start,
            capitalization = KeyboardCapitalization.Characters,
        )
        KivoButton(stringResource(R.string.continue_), submit, Modifier.testTag("linkContinue"), primary = true, enabled = typed.isNotBlank() && !busy)
    }
    LaunchedEffect(Unit) { focusWhenPlaced(field) }
}

/** The longest a typed code may be: six characters with a separator or spaces. */
private const val MAX_TYPED = 9

@Composable
private fun Ask(username: String, request: PendingDevice, busy: Boolean, onDeny: () -> Unit, onAllow: () -> Unit) {
    val deny = remember { FocusRequester() }
    Column(verticalArrangement = Arrangement.spacedBy(Kivo.space(3))) {
        Text(stringResource(R.string.allow_sign_in_as, request.name, username), style = TvType.heading, color = Kivo.colors.text)
        Text(askingContext(request), style = TvType.body, color = Kivo.colors.textMuted, modifier = Modifier.testTag("linkContext"))
        if (!request.nearby) {
            Text(stringResource(R.string.link_not_nearby_warning), style = TvType.body, color = Kivo.colors.danger, modifier = Modifier.testTag("linkWarning"))
        }
        Row(horizontalArrangement = Arrangement.spacedBy(Kivo.space(3))) {
            KivoButton(stringResource(R.string.dont_allow), onDeny, Modifier.focusRequester(deny).testTag("dontAllow"), enabled = !busy)
            KivoButton(stringResource(R.string.allow), onAllow, Modifier.testTag("allow"), primary = true, enabled = !busy)
        }
    }
    LaunchedEffect(Unit) { focusWhenPlaced(deny) }
}

/** What the asking device runs on, whether it's on the home network and when it asked: "Android TV · On your home network · Just now". */
@Composable
private fun askingContext(request: PendingDevice): String = listOfNotNull(
    platformName(request.platform),
    stringResource(if (request.nearby) R.string.on_home_network else R.string.not_on_home_network),
    asked(request.startedAt),
).joinToString(" · ")

/** When a device asked, in words: "Just now" within a minute, else "5 minutes ago". */
@Composable
private fun asked(at: Instant): String =
    if (Clock.System.now() - at < 1.minutes) stringResource(R.string.just_now) else relativeTime(at).replaceFirstChar { it.uppercase() }

@Composable
private fun Answered(answered: LinkStep.Answered, onDone: () -> Unit) {
    val done = remember { FocusRequester() }
    Column(verticalArrangement = Arrangement.spacedBy(Kivo.space(3))) {
        val text = if (answered.allowed) R.string.device_signed_in else R.string.device_not_signed_in
        Text(stringResource(text, answered.name), style = TvType.heading, color = Kivo.colors.text, modifier = Modifier.testTag("linkAnswered"))
        KivoButton(stringResource(R.string.done), onDone, Modifier.focusRequester(done), primary = true)
    }
    LaunchedEffect(Unit) { focusWhenPlaced(done) }
}

/** A platform for people, such as "Android TV"; null for one this app doesn't know. */
@Composable
internal fun platformName(platform: ClientPlatform?): String? = when (platform) {
    ClientPlatform.AndroidTv -> stringResource(R.string.platform_android_tv)
    ClientPlatform.AppleTv -> stringResource(R.string.platform_apple_tv)
    ClientPlatform.IPhone -> stringResource(R.string.platform_iphone)
    ClientPlatform.IPad -> stringResource(R.string.platform_ipad)
    ClientPlatform.AndroidPhone -> stringResource(R.string.platform_android_phone)
    ClientPlatform.Browser -> stringResource(R.string.platform_browser)
    else -> null
}

/** When something happened, in words: "5 minutes ago". */
internal fun relativeTime(at: Instant): String =
    android.text.format.DateUtils.getRelativeTimeSpanString(
        at.toEpochMilliseconds(),
        System.currentTimeMillis(),
        android.text.format.DateUtils.MINUTE_IN_MILLIS,
    ).toString().replaceFirstChar { it.lowercase() }
