package com.mmcdole.kivo.android.mobile.linkdevice

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.android.mobile.ui.sentence
import com.mmcdole.kivo.api.models.PendingDevice
import com.mmcdole.kivo.core.Done
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.auth.LinkCode
import com.mmcdole.kivo.core.auth.SavedServer
import com.mmcdole.kivo.core.session.KivoSession
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** Where Link a device is. */
sealed interface LinkStep {
    /** Scanning the other device's QR code with the camera. */
    data object Scan : LinkStep

    /** Typing the other device's code. */
    data object Enter : LinkStep

    /** The code is for another server, [name] when this app has signed in to it. */
    data class OtherServer(val name: String?) : LinkStep

    /** Allow or Don't allow [device], waiting on [code]. */
    data class Confirm(val code: String, val device: PendingDevice) : LinkStep

    /** Answered: [name] was signed in, or not. */
    data class Done(val name: String, val allowed: Boolean) : LinkStep
}

/** Why a code didn't go through. */
sealed interface LinkMessage {
    /** What was typed or scanned holds no code. */
    data object NotACode : LinkMessage

    /** No device waits on the code: unknown, expired or answered. */
    data object CodeFailed : LinkMessage

    data class Problem(val sentence: String) : LinkMessage
}

data class LinkUiState(
    val step: LinkStep = LinkStep.Enter,
    val busy: Boolean = false,
    val message: LinkMessage? = null,
    /** Who the device signs in as. */
    val username: String = "",
    val serverName: String = "",
)

/**
 * Link a device: the code another device shows, scanned from its QR code or
 * typed, then that device as the server saw it, to Allow or Don't allow,
 * signing it in as this user. A code for another server, as its QR code
 * says, is said so. [servers] are those this app has signed in to, to name
 * that server; [initial] is a code or link to look up at once, as a
 * `kivo://link` link carries. It opens on the camera when [camera] says
 * there's one, else on the typed code.
 */
class LinkDeviceViewModel(private val session: KivoSession, private val servers: Flow<List<SavedServer>>, initial: String?, camera: Boolean = false) : ViewModel() {
    private val mutableState = MutableStateFlow(
        LinkUiState(
            step = if (camera) LinkStep.Scan else LinkStep.Enter,
            username = session.user.value?.username.orEmpty(),
            serverName = session.savedServer.value.name,
        ),
    )
    val state: StateFlow<LinkUiState> = mutableState.asStateFlow()

    init {
        initial?.let(::lookUp)
    }

    /** Looks up the device waiting on the code in [text], typed or scanned. */
    fun lookUp(text: String) {
        if (state.value.busy) return
        val link = LinkCode.parseOrNull(text)
        if (link == null) {
            mutableState.update { it.copy(message = LinkMessage.NotACode) }
            return
        }
        run {
            val other = link.server?.takeIf { it != session.savedServer.value.id }
            if (other != null) {
                val name = servers.first().firstOrNull { it.id == other }?.name
                mutableState.update { it.copy(step = LinkStep.OtherServer(name), message = null) }
            } else {
                val device = session.link.pendingDevice(link.code)
                mutableState.update { it.copy(step = LinkStep.Confirm(link.code, device), message = null) }
            }
        }
    }

    /** Allows the device asking, or doesn't. */
    fun answer(allow: Boolean) {
        val confirm = state.value.step as? LinkStep.Confirm ?: return
        run {
            when (val done = if (allow) session.link.approve(confirm.code) else session.link.deny(confirm.code)) {
                is Done.Ok -> mutableState.update { it.copy(step = LinkStep.Done(confirm.device.name, allow), message = null) }
                is Done.Failed -> mutableState.update { it.copy(message = message(done.problem)) }
            }
        }
    }

    /** Enter code instead, or Scan the QR code instead. */
    fun show(step: LinkStep) {
        mutableState.update { it.copy(step = step, message = null) }
    }

    /** A code the server doesn't know is its own message; any other problem is worded. */
    private fun message(problem: RequestProblem): LinkMessage =
        if ((problem as? RequestProblem.Refused)?.status == 404) LinkMessage.CodeFailed else LinkMessage.Problem(sentence(problem))

    private fun run(call: suspend () -> Unit) {
        mutableState.update { it.copy(busy = true) }
        viewModelScope.launch {
            try {
                call()
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                mutableState.update { it.copy(message = message(RequestProblem.of(e))) }
            } finally {
                mutableState.update { it.copy(busy = false) }
            }
        }
    }
}
