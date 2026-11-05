package com.mmcdole.kivo.core

/**
 * What a client shows of a read from the server: [Loading] until its first
 * answer, the answer ([Loaded]), or why there's none ([Failed]). A read
 * that's kept goes from one answer to the next without [Loading] between
 * ([Reachability.load]).
 */
sealed class Load<out T : Any> {
    data object Loading : Load<Nothing>()

    data class Loaded<out T : Any>(val value: T) : Load<T>()

    data class Failed(val problem: RequestProblem) : Load<Nothing>()

    /** The answer when [Loaded], else null. */
    fun getOrNull(): T? = (this as? Loaded<T>)?.value

    /** The problem when [Failed], else null. */
    fun problemOrNull(): RequestProblem? = (this as? Failed)?.problem
}
