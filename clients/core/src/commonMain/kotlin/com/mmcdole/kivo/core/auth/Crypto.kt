package com.mmcdole.kivo.core.auth

/** The SHA-512 of [data]. */
internal expect fun sha512(data: ByteArray): ByteArray

/** [size] bytes from the platform's cryptographically secure random source. */
internal expect fun secureRandom(size: Int): ByteArray
