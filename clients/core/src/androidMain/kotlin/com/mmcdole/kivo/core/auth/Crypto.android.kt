package com.mmcdole.kivo.core.auth

import java.security.GeneralSecurityException
import java.security.KeyFactory
import java.security.SecureRandom
import java.security.Signature
import java.security.spec.X509EncodedKeySpec

/** A P-256 SubjectPublicKeyInfo up to its point, which follows. */
private val P256_SPKI = "3059301306072a8648ce3d020106082a8648ce3d030107034200".chunked(2).map { it.toInt(16).toByte() }.toByteArray()

internal actual fun verifyP256(publicKey: ByteArray, message: ByteArray, signature: ByteArray): Boolean = try {
    val key = KeyFactory.getInstance("EC").generatePublic(X509EncodedKeySpec(P256_SPKI + publicKey))
    Signature.getInstance("SHA256withECDSA").run {
        initVerify(key)
        update(message)
        verify(signature)
    }
} catch (_: GeneralSecurityException) {
    false
}

private val random = SecureRandom()

internal actual fun secureRandom(size: Int): ByteArray = ByteArray(size).also(random::nextBytes)
