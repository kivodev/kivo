package com.mmcdole.kivo.core.auth

import com.mmcdole.kivo.api.check
import com.mmcdole.kivo.api.models.PendingDevice
import com.mmcdole.kivo.api.result

/**
 * Answers another device's link code from this signed-in app ("Link a
 * device"): shows who is asking, then approves it, so it signs in as this
 * user, or denies it. A code that's unknown, expired or already answered is
 * a `ProblemException` with status 404.
 */
internal class LinkDevice(private val connection: ServerConnection) {
    /** The device waiting on [code]: its name, platform, whether it's nearby and when it asked. */
    suspend fun pendingDevice(code: String): PendingDevice = connection.sessions.getDeviceCode(code).result()

    /** Lets the device waiting on [code] sign in as this user. */
    suspend fun approve(code: String) {
        connection.sessions.approveDeviceCode(code).check()
    }

    /** Refuses the device waiting on [code]; it hears so on its next poll. */
    suspend fun deny(code: String) {
        connection.sessions.denyDeviceCode(code).check()
    }
}

/** A link code as typed or scanned, with the ID of the server the asking device is on when the QR code named it. */
data class LinkCode(val code: String, val server: String?) {
    companion object {
        /**
         * The link code in what was typed or scanned: the `code` and `server`
         * of a `/link?code=K7Q2M9&server=…` address or `kivo://link?…` link
         * from a QR code, or a typed code in any case, with or without spaces
         * or a hyphen; null when there's no six-character code.
         */
        fun parseOrNull(text: String): LinkCode? {
            val params = text.substringAfter('?', "").split('&').associate { it.substringBefore('=') to it.substringAfter('=', "") }
            val code = (params["code"] ?: text).filter { it.isLetterOrDigit() }.uppercase()
            if (code.length != LINK_CODE_LENGTH) return null
            return LinkCode(code, params["server"]?.takeIf { it.isNotEmpty() })
        }
    }
}

private const val LINK_CODE_LENGTH = 6
