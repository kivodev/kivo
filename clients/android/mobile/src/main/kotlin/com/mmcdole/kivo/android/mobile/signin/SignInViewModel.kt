package com.mmcdole.kivo.android.mobile.signin

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.auth.SavedServer
import com.mmcdole.kivo.core.discovery.FoundServer
import com.mmcdole.kivo.core.session.KivoClient
import com.mmcdole.kivo.core.session.KivoSession
import com.mmcdole.kivo.core.session.LinkProgress
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** Where signing in is: choosing a server, its password, or a code another device approves. */
sealed interface SignInStep {
    data object Find : SignInStep

    data class Password(val server: FoundServer) : SignInStep

    data class Link(val server: FoundServer) : SignInStep
}

/** Why the last step didn't go on. */
sealed interface SignInError {
    /** The last server signed in to answers at none of its addresses. */
    data class CantReach(val name: String) : SignInError

    /** No Kivo server answered at the typed address. */
    data object NoServer : SignInError

    data class Problem(val problem: RequestProblem) : SignInError
}

/** The code to approve on a signed-in device, once it's come; whether the last was declined, or why none could be had. */
data class LinkUi(val code: String? = null, val linkUrl: String? = null, val declined: Boolean = false, val problem: RequestProblem? = null)

/** The servers found on the network so far, whether it's still looking, and whether it has started to. */
data class Found(val servers: List<FoundServer> = emptyList(), val searching: Boolean = false, val started: Boolean = false)

data class SignInUiState(
    val step: SignInStep = SignInStep.Find,
    /** The last server signed in to, offered first. */
    val saved: SavedServer? = null,
    val found: Found = Found(),
    val address: String = "",
    val busy: Boolean = false,
    val error: SignInError? = null,
    val link: LinkUi = LinkUi(),
)

/**
 * Signing in: the last server signed in to, the servers found on the
 * network and a typed address; then a password, or a code another signed-in
 * device approves. Signed in, [open] opens the server's session.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class SignInViewModel(
    private val client: KivoClient,
    private val findServers: () -> Flow<FoundServer>,
    private val deviceName: String,
    saved: SavedServer?,
    private val open: suspend (KivoSession) -> Unit,
) : ViewModel() {
    private val form = MutableStateFlow(SignInUiState(saved = saved))

    /** Bumped to look for servers, once the network may be searched. */
    private val searches = MutableStateFlow(0)

    /** Bumped for a new sign-in code after one failed. */
    private val linkAttempts = MutableStateFlow(0)

    private val found: Flow<Found> = searches.flatMapLatest { search -> if (search == 0) flowOf(Found()) else lookForServers() }

    private val link: Flow<LinkUi> = combine(
        form.map { (it.step as? SignInStep.Link)?.server }.distinctUntilChanged(),
        linkAttempts,
    ) { server, _ -> server }.flatMapLatest { server -> if (server == null) flowOf(LinkUi()) else linkCodes(server) }

    val state: StateFlow<SignInUiState> = combine(form, found, link) { form, found, link ->
        form.copy(found = found, link = link)
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), form.value)

    /** Looks for servers on the network, as it may now be searched. */
    fun search() {
        if (searches.value == 0) searches.value = 1
    }

    fun setAddress(address: String) = form.update { it.copy(address = address, error = null) }

    /** The last server signed in to, at the first of its addresses that answers as it. */
    fun chooseSaved() {
        val saved = form.value.saved ?: return
        busy {
            val server = saved.addresses.firstNotNullOfOrNull { address ->
                try {
                    client.serverAt(address).takeIf { it.info.id == saved.id }
                } catch (e: CancellationException) {
                    throw e
                } catch (_: Exception) {
                    null
                }
            }
            if (server == null) fail(SignInError.CantReach(saved.name)) else go(SignInStep.Password(server))
        }
    }

    fun choose(server: FoundServer) = go(SignInStep.Password(server))

    /** The server at the typed address. */
    fun connect() {
        val address = form.value.address.trim()
        if (address.isEmpty()) return
        busy {
            try {
                go(SignInStep.Password(client.serverAt(address)))
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                // Anything but a problem document means no Kivo server answered there.
                val problem = RequestProblem.of(e)
                fail(if (problem is RequestProblem.Refused && problem.problem != null) SignInError.Problem(problem) else SignInError.NoServer)
            }
        }
    }

    fun signIn(username: String, password: String) {
        val server = (form.value.step as? SignInStep.Password)?.server ?: return
        if (username.isBlank() || password.isEmpty()) return
        busy {
            try {
                open(client.signIn(server, username.trim(), password, deviceName))
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                fail(SignInError.Problem(RequestProblem.of(e)))
            }
        }
    }

    /** Signing in by a code another device approves, on the server the password was for. */
    fun useAnotherDevice() {
        val server = (form.value.step as? SignInStep.Password)?.server ?: return
        go(SignInStep.Link(server))
    }

    fun retryLink() {
        linkAttempts.value++
    }

    /** Back a step: from the code to the password, from the password to choosing a server. */
    fun back(): Boolean {
        when (val step = form.value.step) {
            SignInStep.Find -> return false
            is SignInStep.Password -> go(SignInStep.Find)
            is SignInStep.Link -> go(SignInStep.Password(step.server))
        }
        return true
    }

    private fun go(step: SignInStep) = form.update { it.copy(step = step, error = null, busy = false) }

    private fun fail(error: SignInError) = form.update { it.copy(error = error, busy = false) }

    private fun busy(work: suspend () -> Unit) {
        if (form.value.busy) return
        form.update { it.copy(busy = true, error = null) }
        viewModelScope.launch { work() }
    }

    /**
     * The servers on the network as they're found; once the search is over,
     * a lone server goes on to its password, as nothing else is on offer.
     */
    private fun lookForServers(): Flow<Found> = flow {
        var servers = emptyList<FoundServer>()
        emit(Found(searching = true, started = true))
        try {
            findServers().collect { server ->
                servers = servers.filter { it.info.id != server.info.id } + server
                emit(Found(servers, searching = true, started = true))
            }
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            // Nothing to browse on: the address is the way in.
        }
        emit(Found(servers, searching = false, started = true))
        val now = form.value
        if (servers.size == 1 && now.saved == null && now.step == SignInStep.Find && now.address.isBlank()) go(SignInStep.Password(servers.single()))
    }

    /**
     * Codes for [server] until one is approved, which opens the session: an
     * expired code is replaced, and a declined one says so over its
     * replacement.
     */
    private fun linkCodes(server: FoundServer): Flow<LinkUi> = flow {
        var declined = false
        while (true) {
            var again = false
            client.signInByLink(server, deviceName).collect { progress ->
                when (progress) {
                    is LinkProgress.Waiting -> emit(LinkUi(progress.code, progress.linkUrl, declined))
                    is LinkProgress.SignedIn -> open(progress.session)
                    LinkProgress.Denied -> {
                        declined = true
                        again = true
                    }
                    LinkProgress.Expired -> again = true
                    is LinkProgress.Failed -> emit(LinkUi(problem = progress.problem))
                }
            }
            if (!again) break
        }
    }
}

/** A code as people read it: two groups of three, "K7Q-2M9". */
fun groupedCode(code: String): String = if (code.length == 6) code.take(3) + "-" + code.drop(3) else code
