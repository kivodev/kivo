package com.mmcdole.kivo.core.session

import com.mmcdole.kivo.api.models.PendingDevice
import com.mmcdole.kivo.core.Done
import com.mmcdole.kivo.core.auth.LinkDevice
import com.mmcdole.kivo.core.done
import kotlinx.coroutines.CancellationException

/**
 * Link a device ([KivoSession.link]): answering another device's link code,
 * as [com.mmcdole.kivo.core.auth.LinkCode.parseOrNull] reads it, signing that device
 * in as this user.
 */
class SessionLink internal constructor(private val link: LinkDevice) {
    /**
     * The device waiting on [code], to ask before approving: what it says it
     * is, whether it's on the server's local networks, and when it asked.
     */
    @Throws(Exception::class)
    suspend fun pendingDevice(code: String): PendingDevice = link.pendingDevice(code)

    @Throws(CancellationException::class)
    suspend fun approve(code: String): Done<Unit> = done { link.approve(code) }

    @Throws(CancellationException::class)
    suspend fun deny(code: String): Done<Unit> = done { link.deny(code) }
}
