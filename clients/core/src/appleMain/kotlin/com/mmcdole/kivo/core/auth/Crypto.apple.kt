package com.mmcdole.kivo.core.auth

import kotlinx.cinterop.ExperimentalForeignApi
import kotlinx.cinterop.UByteVar
import kotlinx.cinterop.addressOf
import kotlinx.cinterop.convert
import kotlinx.cinterop.reinterpret
import kotlinx.cinterop.usePinned
import platform.CoreCrypto.CC_SHA512
import platform.CoreCrypto.CC_SHA512_DIGEST_LENGTH
import platform.Security.SecRandomCopyBytes
import platform.Security.errSecSuccess
import platform.Security.kSecRandomDefault

@OptIn(ExperimentalForeignApi::class)
internal actual fun sha512(data: ByteArray): ByteArray {
    val out = ByteArray(CC_SHA512_DIGEST_LENGTH)
    val input = if (data.isEmpty()) ByteArray(1) else data
    input.usePinned { d ->
        out.usePinned { o -> CC_SHA512(d.addressOf(0), data.size.convert(), o.addressOf(0).reinterpret<UByteVar>()) }
    }
    return out
}

@OptIn(ExperimentalForeignApi::class)
internal actual fun secureRandom(size: Int): ByteArray {
    val out = ByteArray(size)
    if (size == 0) return out
    val status = out.usePinned { SecRandomCopyBytes(kSecRandomDefault, size.convert(), it.addressOf(0)) }
    check(status == errSecSuccess) { "no secure random bytes: $status" }
    return out
}
