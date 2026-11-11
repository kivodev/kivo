package com.mmcdole.kivo.core

import com.mmcdole.kivo.api.ProblemException
import com.mmcdole.kivo.api.models.Problem
import kotlinx.io.IOException

/**
 * Why a request to the server failed, as facts each client words and acts
 * on: no answer came ([Unreachable]), the session ended ([SignedOut]), the
 * server answered with a problem ([Refused]), or the app couldn't read what
 * came back ([Unexpected]). Only [Unreachable] is worth asking again for
 * ([Reachability]).
 */
sealed class RequestProblem {
    /**
     * The code the clients word it by: the server's problem code for
     * [Refused], and `network`, `unauthenticated` or `unknown` for the
     * others.
     */
    abstract val code: String

    /** [code] as one of the codes the apps act on themselves. */
    val knownCode: ProblemCode get() = ProblemCode.of(code)

    /**
     * No answer came from the server: the request couldn't be sent or timed
     * out, or a gateway in front of the server answered 502, 503 or 504
     * without a problem document, saying it's down.
     */
    data object Unreachable : RequestProblem() {
        override val code: String get() = "network"
    }

    /** The server no longer takes this app's session: a `401` without another problem, such as a wrong password. */
    data object SignedOut : RequestProblem() {
        override val code: String get() = "unauthenticated"
    }

    /**
     * The server answered with a problem: [code] is its problem code, or
     * `unknown` when the answer had no problem document; [message] is the
     * words it gives for the user, when it gives any, and [problem] the
     * whole document, with the details of some codes.
     */
    data class Refused(
        val status: Int,
        override val code: String,
        val message: String?,
        val problem: Problem?,
    ) : RequestProblem()

    /** Something other than the server failed, such as an answer the app couldn't read. */
    data object Unexpected : RequestProblem() {
        override val code: String get() = "unknown"
    }

    companion object {
        /** The problem a failed request's [error] means. */
        fun of(error: Throwable): RequestProblem = when (error) {
            is ProblemException -> when {
                error.problem == null && error.status in GATEWAY_DOWN -> Unreachable
                error.status == 401 && (error.problem == null || error.code == SignedOut.code) -> SignedOut
                else -> Refused(error.status, error.code, error.problem?.message, error.problem)
            }
            is IOException -> Unreachable
            else -> Unexpected
        }

        /** A gateway's answers for a server it can't reach, which aren't the server's own unless they carry a problem document. */
        private val GATEWAY_DOWN = 502..504
    }
}

/**
 * The codes the apps act on themselves rather than word as any other: a
 * server's problem codes and a cast receiver's errors, by [value]. Every
 * other code is [Unknown].
 */
enum class ProblemCode(val value: String) {
    InvalidCredentials("invalidCredentials"),
    InvalidRequest("invalidRequest"),
    NameTaken("nameTaken"),
    NoFallback("noFallback"),
    NotFound("notFound"),
    NotPlayable("notPlayable"),
    PlaybackFailed("playbackFailed"),
    PlayEnded("playEnded"),
    ReceiverOutdated("receiverOutdated"),
    ServerOutdated("serverOutdated"),
    SignedOut("signedOut"),
    StreamLimit("streamLimit"),
    WrongPassword("wrongPassword"),
    Unknown(""),
    ;

    override fun toString(): String = value

    companion object {
        /** The code [value] names; [Unknown] for any other. */
        fun of(value: String?): ProblemCode = entries.firstOrNull { it != Unknown && it.value == value } ?: Unknown
    }
}
