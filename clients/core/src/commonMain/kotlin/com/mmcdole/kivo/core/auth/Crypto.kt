package com.mmcdole.kivo.core.auth

/**
 * Whether [signature], ASN.1 DER, is the ECDSA P-256 signature with SHA-256
 * of [message] by [publicKey], an uncompressed X9.63 point, by the
 * platform's own crypto.
 */
internal expect fun verifyP256(publicKey: ByteArray, message: ByteArray, signature: ByteArray): Boolean

/** [size] bytes from the platform's cryptographically secure random source. */
internal expect fun secureRandom(size: Int): ByteArray
