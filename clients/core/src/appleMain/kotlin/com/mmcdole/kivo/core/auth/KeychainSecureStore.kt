package com.mmcdole.kivo.core.auth

import kotlinx.cinterop.BetaInteropApi
import kotlinx.cinterop.ExperimentalForeignApi
import kotlinx.cinterop.addressOf
import kotlinx.cinterop.alloc
import kotlinx.cinterop.convert
import kotlinx.cinterop.memScoped
import kotlinx.cinterop.ptr
import kotlinx.cinterop.usePinned
import kotlinx.cinterop.value
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.IO
import kotlinx.coroutines.withContext
import platform.CoreFoundation.CFDictionaryAddValue
import platform.CoreFoundation.CFDictionaryCreateMutable
import platform.CoreFoundation.CFDictionaryRef
import platform.CoreFoundation.CFRelease
import platform.CoreFoundation.CFStringRef
import platform.CoreFoundation.CFTypeRef
import platform.CoreFoundation.CFTypeRefVar
import platform.CoreFoundation.kCFAllocatorDefault
import platform.CoreFoundation.kCFBooleanTrue
import platform.CoreFoundation.kCFTypeDictionaryKeyCallBacks
import platform.CoreFoundation.kCFTypeDictionaryValueCallBacks
import platform.Foundation.CFBridgingRelease
import platform.Foundation.CFBridgingRetain
import platform.Foundation.NSData
import platform.Foundation.create
import platform.Security.SecItemAdd
import platform.Security.SecItemCopyMatching
import platform.Security.SecItemDelete
import platform.Security.errSecSuccess
import platform.Security.kSecAttrAccessible
import platform.Security.kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
import platform.Security.kSecAttrAccount
import platform.Security.kSecAttrService
import platform.Security.kSecClass
import platform.Security.kSecClassGenericPassword
import platform.Security.kSecMatchLimit
import platform.Security.kSecMatchLimitOne
import platform.Security.kSecReturnData
import platform.Security.kSecValueData
import platform.posix.memcpy

/**
 * The Apple [SecureStore]: each value a generic password in the Keychain,
 * under [service] and the key as its account. Readable after the device's
 * first unlock, so playing in the background can still sign requests, and
 * never carried to another device by a backup, as the Android Keystore's
 * key isn't.
 */
@OptIn(ExperimentalForeignApi::class, BetaInteropApi::class)
internal class KeychainSecureStore(private val service: String = "com.mmcdole.kivo") : SecureStore {
    override suspend fun read(key: String): String? = withContext(Dispatchers.IO) {
        memScoped {
            val result = alloc<CFTypeRefVar>()
            val status = query(key, kSecReturnData to kCFBooleanTrue, kSecMatchLimit to kSecMatchLimitOne) {
                SecItemCopyMatching(it, result.ptr)
            }
            if (status != errSecSuccess) return@memScoped null
            (CFBridgingRelease(result.value) as? NSData)?.toByteArray()?.decodeToString()
        }
    }

    override suspend fun write(key: String, value: String) = withContext(Dispatchers.IO) {
        query(key) { SecItemDelete(it) }
        val data = CFBridgingRetain(value.encodeToByteArray().toNSData())
        try {
            val status = query(
                key,
                kSecValueData to data,
                kSecAttrAccessible to kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly,
            ) { SecItemAdd(it, null) }
            check(status == errSecSuccess) { "the Keychain refused the write: OSStatus $status" }
        } finally {
            CFBridgingRelease(data)
        }
    }

    override suspend fun delete(key: String) = withContext(Dispatchers.IO) {
        query(key) { SecItemDelete(it) }
        Unit
    }

    /** Runs [block] with the Keychain query for [key] and [extra] attributes, releasing what it made. */
    private fun <T> query(key: String, vararg extra: Pair<CFStringRef?, CFTypeRef?>, block: (CFDictionaryRef?) -> T): T {
        val account = CFBridgingRetain(key)
        val serviceName = CFBridgingRetain(service)
        val entries = listOf<Pair<CFStringRef?, CFTypeRef?>>(
            kSecClass to kSecClassGenericPassword,
            kSecAttrService to serviceName,
            kSecAttrAccount to account,
        ) + extra
        val query = CFDictionaryCreateMutable(
            kCFAllocatorDefault,
            entries.size.convert(),
            kCFTypeDictionaryKeyCallBacks.ptr,
            kCFTypeDictionaryValueCallBacks.ptr,
        )
        try {
            for ((name, value) in entries) CFDictionaryAddValue(query, name, value)
            return block(query)
        } finally {
            CFRelease(query)
            CFBridgingRelease(account)
            CFBridgingRelease(serviceName)
        }
    }
}

@OptIn(ExperimentalForeignApi::class, BetaInteropApi::class)
private fun ByteArray.toNSData(): NSData =
    if (isEmpty()) NSData() else usePinned { NSData.create(bytes = it.addressOf(0), length = size.convert()) }

@OptIn(ExperimentalForeignApi::class)
private fun NSData.toByteArray(): ByteArray {
    val bytes = ByteArray(length.toInt())
    if (bytes.isNotEmpty()) bytes.usePinned { memcpy(it.addressOf(0), this.bytes, length) }
    return bytes
}
