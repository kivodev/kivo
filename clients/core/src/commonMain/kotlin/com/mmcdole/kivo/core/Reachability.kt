package com.mmcdole.kivo.core

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.withTimeoutOrNull
import kotlin.time.Duration
import kotlin.time.Duration.Companion.seconds

/**
 * Whether a server answers, as the reads of it find, and the one retry
 * policy for them: a read that gets no answer ([RequestProblem.Unreachable])
 * is asked again after a wait, 1 s doubling to 30 s, or at once on
 * [retryNow]; any answer, a refusal too, is final. The first read answered
 * after none were wakes the others, so every screen fills in at once.
 * Before each wait it has the server's address picked again ([repick]):
 * when that moves it, the read is asked again at once.
 */
internal class Reachability(
    private val backoff: Backoff = Backoff(1.seconds, 30.seconds),
    private val repick: suspend () -> Boolean = { false },
) {
    private val mutableUnreachable = MutableStateFlow(false)

    /** Retries waiting to ask again. */
    private val wakes = MutableSharedFlow<Unit>(extraBufferCapacity = 1)

    /** Whether the last read got no answer from the server; false once one is answered. */
    val unreachable: StateFlow<Boolean> = mutableUnreachable.asStateFlow()

    /** A request outside [load] and [untilAnswered] got no answer, as opening the server found. */
    fun unanswered() {
        mutableUnreachable.value = true
    }

    /** Asks again now, for the reads waiting to. */
    fun retryNow() {
        wakes.tryEmit(Unit)
    }

    /**
     * [source], a read that's kept, as what a client shows of it: each
     * answer [Load.Loaded]. Without an answer it's asked again after the
     * wait, [Load.Failed] with [RequestProblem.Unreachable] meanwhile if
     * nothing was loaded yet, while an answer already loaded stays. Any
     * other problem is [Load.Failed] and ends it. An answer [known] at once,
     * such as one read earlier, is the first, and stays as a loaded one does.
     */
    fun <T : Any> load(source: Flow<T>, known: T? = null): Flow<Load<T>> = flow {
        var attempt = 0
        var shown: Load<T> = known?.let { Load.Loaded(it) } ?: Load.Loading
        if (known != null) emit(shown)
        while (true) {
            var problem: RequestProblem? = null
            source
                .catch { e ->
                    if (e is CancellationException) throw e
                    problem = RequestProblem.of(e)
                }
                .collect {
                    answered()
                    attempt = 0
                    shown = Load.Loaded(it)
                    emit(shown)
                }
            val failed = problem ?: return@flow
            if (failed !is RequestProblem.Unreachable) {
                answered()
                emit(Load.Failed(failed))
                return@flow
            }
            mutableUnreachable.value = true
            if (shown == Load.Loading) {
                shown = Load.Failed(failed)
                emit(shown)
            }
            wait(attempt++)
        }
    }

    /**
     * [read]'s answer, asking again after the wait while no answer comes.
     * Any other failure is thrown at once.
     */
    suspend fun <T> untilAnswered(read: suspend () -> T): T {
        var attempt = 0
        while (true) {
            try {
                return read().also { answered() }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                if (RequestProblem.of(e) !is RequestProblem.Unreachable) {
                    answered()
                    throw e
                }
                mutableUnreachable.value = true
            }
            wait(attempt++)
        }
    }

    /**
     * [read]'s answer, asked again at most [retries] times while no answer
     * comes, after the wait. Each failure goes to [failed] first; the last,
     * or any other, is thrown.
     */
    suspend fun <T> retrying(retries: Int, failed: (RequestProblem) -> Unit, read: suspend () -> T): T {
        var attempt = 0
        while (true) {
            try {
                return read().also { answered() }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                val problem = RequestProblem.of(e)
                if (problem is RequestProblem.Unreachable) mutableUnreachable.value = true else answered()
                failed(problem)
                if (attempt >= retries || problem !is RequestProblem.Unreachable) throw e
                wait(attempt)
                attempt++
            }
        }
    }

    private suspend fun wait(attempt: Int) {
        if (repick()) return
        withTimeoutOrNull(backoff.wait(attempt)) { wakes.first() }
    }

    private fun answered() {
        if (mutableUnreachable.value) {
            mutableUnreachable.value = false
            retryNow()
        }
    }
}

/** Waits that grow with each attempt: [first], doubling, to at most [cap]. */
internal class Backoff(private val first: Duration, private val cap: Duration) {
    /** The wait before retry [attempt], from 0. */
    fun wait(attempt: Int): Duration {
        var wait = first
        repeat(attempt) {
            if (wait >= cap) return cap
            wait *= 2
        }
        return minOf(wait, cap)
    }
}
