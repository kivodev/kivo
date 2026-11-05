package com.mmcdole.kivo.core.auth

/** The SHA-256 of [data]. */
internal expect fun sha256(data: ByteArray): ByteArray

/** HMAC-SHA256 of [data] keyed by [key]. */
internal expect fun hmacSha256(key: ByteArray, data: ByteArray): ByteArray

/** [size] bytes from the platform's cryptographically secure random source. */
internal expect fun secureRandom(size: Int): ByteArray

/** Whether [a] and [b] are equal, in a time that doesn't depend on where they differ. */
internal fun constantTimeEquals(a: ByteArray, b: ByteArray): Boolean {
    if (a.size != b.size) return false
    var diff = 0
    for (i in a.indices) diff = diff or (a[i].toInt() xor b[i].toInt())
    return diff == 0
}
