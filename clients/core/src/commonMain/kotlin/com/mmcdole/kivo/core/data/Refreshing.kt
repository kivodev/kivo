package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.core.live.Invalidation
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.channelFlow
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.launch

/**
 * [fetch]'s answer, fetched again whenever an invalidation [refetchOn]
 * accepts arrives, as the live events say. A burst of invalidations during a
 * fetch is one more fetch. The answer is held only by the flow: there's no
 * persistent cache.
 */
internal fun <T> refreshing(
    invalidations: Flow<Invalidation>,
    refetchOn: (Invalidation) -> Boolean,
    fetch: suspend () -> T,
): Flow<T> = channelFlow {
    val stale = Channel<Unit>(Channel.CONFLATED)
    stale.send(Unit)
    launch(start = CoroutineStart.UNDISPATCHED) {
        invalidations.filter { it == Invalidation.Everything || refetchOn(it) }.collect { stale.send(Unit) }
    }
    for (signal in stale) send(fetch())
}
