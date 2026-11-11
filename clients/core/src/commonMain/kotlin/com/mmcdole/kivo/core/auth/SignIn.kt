package com.mmcdole.kivo.core.auth

import com.mmcdole.kivo.api.ProblemException
import com.mmcdole.kivo.api.check
import com.mmcdole.kivo.api.models.CurrentUser
import com.mmcdole.kivo.api.models.DeviceCodePoll
import com.mmcdole.kivo.api.models.DeviceCodeCreate
import com.mmcdole.kivo.api.models.Session
import com.mmcdole.kivo.api.models.SessionCreate
import com.mmcdole.kivo.api.models.SessionKind
import com.mmcdole.kivo.api.result
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlin.time.Duration.Companion.seconds
import kotlin.time.Instant

/**
 * Signing this app in to a server: by username and password, or by a
 * device code another signed-in client approves, either way as an app
 * session whose token the [connection] keeps. [name] is the session's name,
 * which "Signed-in devices" lists, such as "Living Room"; what the app runs
 * on goes in its identity headers. [clientId] is this
 * install's [ClientId], so signing in again replaces this install's session
 * rather than adding one.
 */
internal class SignIn(
    private val connection: ServerConnection,
    private val name: String,
    private val clientId: String,
) {
    /** Signs in with a password; a wrong one is a [ProblemException]. */
    suspend fun withPassword(username: String, password: String): CurrentUser {
        val session = connection.sessions
            .createSession(SessionCreate(username, password, kind = SessionKind.App, name = name, clientId = clientId))
            .result()
        return signedIn(session)
    }

    /**
     * Starts a link code and polls until it's approved, denied or expires,
     * as often as the server says, or later after a throttled poll when the
     * server says. Collecting again starts a new code; cancelling stops
     * polling. A poll that fails to reach the server is tried again at the
     * next interval.
     */
    fun link(): Flow<LinkState> = flow {
        val link = connection.sessions.createDeviceCode(DeviceCodeCreate(name, clientId = clientId)).result()
        emit(LinkState.Waiting(link.code, link.linkUrl, link.expiresAt))
        val interval = link.interval.seconds
        while (true) {
            var wait = interval
            val done: LinkState? = try {
                val poll = connection.sessions.pollDeviceCode(DeviceCodePoll(link.secret))
                poll.check()
                if (poll.status == 201) LinkState.SignedIn(signedIn(poll.body())) else null
            } catch (e: ProblemException) {
                when (e.code) {
                    "expired" -> LinkState.Expired
                    "denied" -> LinkState.Denied
                    "throttled" -> {
                        e.problem?.retryAfterSeconds?.let { wait = maxOf(wait, it.seconds) }
                        null
                    }
                    else -> throw e
                }
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                null // The server can't be reached: poll again at the next interval.
            }
            if (done != null) {
                emit(done)
                return@flow
            }
            delay(wait)
        }
    }


    private suspend fun signedIn(session: Session): CurrentUser {
        val token = session.token ?: throw IllegalStateException("an app session came without its token")
        connection.signIn(token)
        return connection.users.getCurrentUser().result().also { connection.keep(it) }
    }
}

/** Where a link code is. */
internal sealed interface LinkState {
    /** Show [code], and [linkUrl] as text and as a QR code, until someone answers it. */
    data class Waiting(val code: String, val linkUrl: String, val expiresAt: Instant) : LinkState

    /** Approved: this app is signed in. */
    data class SignedIn(val user: CurrentUser) : LinkState

    /** Denied on the other device; collect again for a new code. */
    data object Denied : LinkState

    /** The code expired unanswered; collect again for a new one. */
    data object Expired : LinkState
}
