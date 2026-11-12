package com.mmcdole.kivo.android

import kotlinx.coroutines.CancellationException

/** [read]'s answer; null when it fails. A cancellation still cancels. */
suspend fun <T> quietly(read: suspend () -> T): T? = try {
    read()
} catch (e: CancellationException) {
    throw e
} catch (_: Exception) {
    null
}
