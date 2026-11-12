package com.mmcdole.kivo.core

import kotlinx.coroutines.CancellationException

/**
 * What came of a change asked of the server: made, with what the server
 * gives back ([Ok]), or why not ([Failed]), in the words of a read's
 * [Load]. A change never throws but for its caller being cancelled.
 */
sealed class Done<out T : Any> {
    data class Ok<out T : Any>(val value: T) : Done<T>()

    data class Failed(val problem: RequestProblem) : Done<Nothing>()

    /** What the server gave back when [Ok], else null. */
    fun getOrNull(): T? = (this as? Ok<T>)?.value

    /** The problem when [Failed], else null. */
    fun problemOrNull(): RequestProblem? = (this as? Failed)?.problem
}

/** [change]'s answer as [Done]: its failure as the problem it means; cancellation goes on to the caller. */
internal suspend fun <T : Any> done(change: suspend () -> T): Done<T> = try {
    Done.Ok(change())
} catch (e: CancellationException) {
    throw e
} catch (e: Exception) {
    Done.Failed(RequestProblem.of(e))
}
