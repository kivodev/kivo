package com.mmcdole.kivo.core.auth

import kotlinx.cinterop.ExperimentalForeignApi
import kotlinx.cinterop.addressOf
import kotlinx.cinterop.convert
import kotlinx.cinterop.ptr
import kotlinx.cinterop.reinterpret
import kotlinx.cinterop.usePinned
import platform.CoreFoundation.CFDataCreate
import platform.CoreFoundation.CFDataRef
import platform.CoreFoundation.CFDictionaryAddValue
import platform.CoreFoundation.CFDictionaryCreateMutable
import platform.CoreFoundation.CFRelease
import platform.CoreFoundation.kCFTypeDictionaryKeyCallBacks
import platform.CoreFoundation.kCFTypeDictionaryValueCallBacks
import platform.Security.SecKeyCreateWithData
import platform.Security.SecKeyVerifySignature
import platform.Security.SecRandomCopyBytes
import platform.Security.errSecSuccess
import platform.Security.kSecAttrKeyClass
import platform.Security.kSecAttrKeyClassPublic
import platform.Security.kSecAttrKeyType
import platform.Security.kSecAttrKeyTypeECSECPrimeRandom
import platform.Security.kSecKeyAlgorithmECDSASignatureMessageX962SHA256
import platform.Security.kSecRandomDefault

@OptIn(ExperimentalForeignApi::class)
internal actual fun verifyP256(publicKey: ByteArray, message: ByteArray, signature: ByteArray): Boolean {
    val attributes = CFDictionaryCreateMutable(null, 2, kCFTypeDictionaryKeyCallBacks.ptr, kCFTypeDictionaryValueCallBacks.ptr)
    CFDictionaryAddValue(attributes, kSecAttrKeyType, kSecAttrKeyTypeECSECPrimeRandom)
    CFDictionaryAddValue(attributes, kSecAttrKeyClass, kSecAttrKeyClassPublic)
    val keyData = data(publicKey)
    val messageData = data(message)
    val signatureData = data(signature)
    val key = SecKeyCreateWithData(keyData, attributes, null)
    val verified = key != null && SecKeyVerifySignature(key, kSecKeyAlgorithmECDSASignatureMessageX962SHA256, messageData, signatureData, null)
    listOfNotNull(attributes, keyData, messageData, signatureData, key).forEach { CFRelease(it) }
    return verified
}

@OptIn(ExperimentalForeignApi::class)
private fun data(bytes: ByteArray): CFDataRef? =
    if (bytes.isEmpty()) CFDataCreate(null, null, 0) else bytes.usePinned { CFDataCreate(null, it.addressOf(0).reinterpret(), bytes.size.convert()) }

@OptIn(ExperimentalForeignApi::class)
internal actual fun secureRandom(size: Int): ByteArray {
    val out = ByteArray(size)
    if (size == 0) return out
    val status = out.usePinned { SecRandomCopyBytes(kSecRandomDefault, size.convert(), it.addressOf(0)) }
    check(status == errSecSuccess) { "no secure random bytes: $status" }
    return out
}
