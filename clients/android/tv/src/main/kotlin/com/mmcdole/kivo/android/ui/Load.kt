package com.mmcdole.kivo.android.ui

import com.mmcdole.kivo.core.Load
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.stateIn

/**
 * A screen's read from the session, which asks again itself while the
 * server doesn't answer: [source] is collected while the screen is on (and
 * for 5 seconds after, across a rotation or a quick trip away), so coming
 * back to the foreground fetches again, and an answer stays on screen while
 * the next is fetched, keeping focus and scroll.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class Loader<T : Any>(scope: CoroutineScope, private val source: () -> Flow<Load<T>>) {
    private val attempts = MutableStateFlow(0)

    val state: StateFlow<Load<T>> = attempts
        .flatMapLatest { source() }
        .stateIn(scope, SharingStarted.WhileSubscribed(5_000), Load.Loading)

    /** Reads again now, as Retry does. */
    fun retry() {
        attempts.value++
    }
}

/** The answer [transform]ed once it's loaded. */
fun <T : Any, R : Any> Load<T>.map(transform: (T) -> R): Load<R> = when (this) {
    is Load.Loaded -> Load.Loaded(transform(value))
    is Load.Failed -> this
    Load.Loading -> Load.Loading
}

/** Both reads' answers as one, [combine]d once both are loaded: the first failure, else loading until both are in. */
fun <A : Any, B : Any, R : Any> Load<A>.with(other: Load<B>, combine: (A, B) -> R): Load<R> = when {
    this is Load.Failed -> this
    other is Load.Failed -> other
    this is Load.Loaded && other is Load.Loaded -> Load.Loaded(combine(value, other.value))
    else -> Load.Loading
}

/** Every read's answer, in order, once all are loaded: the first failure, else loading until all are in. */
fun <T : Any> List<Load<T>>.all(): Load<List<T>> =
    fold(Load.Loaded(emptyList<T>()) as Load<List<T>>) { all, load -> all.with(load) { list, value -> list + value } }
