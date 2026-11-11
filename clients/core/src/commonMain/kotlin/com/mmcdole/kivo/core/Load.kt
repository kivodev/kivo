package com.mmcdole.kivo.core

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.firstOrNull
import kotlin.experimental.ExperimentalObjCRefinement
import kotlin.native.HiddenFromObjC

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

/**
 * A read's answer, once: the first it gives, or null when that's a
 * failure, a server that isn't answering among them, so it never waits for
 * one to come back. Swift has its own `answer()`, as an extension function
 * loses the answer's type on the way there.
 */
@OptIn(ExperimentalObjCRefinement::class)
@HiddenFromObjC
suspend fun <T : Any> Flow<Load<T>>.answer(): T? = firstOrNull { it !is Load.Loading }?.getOrNull()
