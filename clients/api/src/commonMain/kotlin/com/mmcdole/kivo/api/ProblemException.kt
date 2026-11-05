package com.mmcdole.kivo.api

import com.mmcdole.kivo.api.infrastructure.ApiClient
import com.mmcdole.kivo.api.infrastructure.HttpResponse
import com.mmcdole.kivo.api.models.Problem
import io.ktor.client.statement.bodyAsText
import kotlinx.serialization.SerializationException

/**
 * A non-2xx answer, as one typed error: the server's problem
 * document, whose `code` clients translate, or null when the body wasn't one,
 * as from a proxy in front of the server. Written by hand beside the
 * generated client, which returns every answer as it came.
 */
class ProblemException(val status: Int, val problem: Problem?) :
    Exception("HTTP $status: ${problem?.code ?: "no problem document"}") {
    /** The problem's stable code, or `unknown` without a problem document. */
    val code: String get() = problem?.code ?: "unknown"
}

/** The answer's body, or a [ProblemException] for a non-2xx answer. */
suspend fun <T : Any> HttpResponse<T>.result(): T {
    check()
    return body()
}

/** Throws a [ProblemException] for a non-2xx answer; for calls whose body isn't needed. */
suspend fun HttpResponse<*>.check() {
    if (success) return
    val problem = try {
        ApiClient.JSON_DEFAULT.decodeFromString(Problem.serializer(), response.bodyAsText())
    } catch (_: SerializationException) {
        null
    }
    throw ProblemException(status, problem)
}
