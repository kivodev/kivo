package com.mmcdole.kivo.core

import kotlinx.coroutines.CancellationException

/** [block]'s answer; null when it fails. A cancellation still cancels. */
internal suspend fun <T> quietly(block: suspend () -> T): T? = try {
    block()
} catch (e: CancellationException) {
    throw e
} catch (_: Exception) {
    null
}
