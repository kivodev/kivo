package com.mmcdole.kivo.core.downloads

import kotlinx.cinterop.BetaInteropApi
import kotlinx.cinterop.ExperimentalForeignApi
import kotlinx.cinterop.UByteVar
import kotlinx.cinterop.alloc
import kotlinx.cinterop.allocArray
import kotlinx.cinterop.autoreleasepool
import kotlinx.cinterop.convert
import kotlinx.cinterop.get
import kotlinx.cinterop.memScoped
import kotlinx.cinterop.ptr
import platform.CoreCrypto.CC_SHA256_CTX
import platform.CoreCrypto.CC_SHA256_DIGEST_LENGTH
import platform.CoreCrypto.CC_SHA256_Final
import platform.CoreCrypto.CC_SHA256_Init
import platform.CoreCrypto.CC_SHA256_Update
import platform.Foundation.NSFileHandle
import platform.Foundation.NSFileManager
import platform.Foundation.NSFileSystemFreeSize
import platform.Foundation.NSNumber
import platform.Foundation.NSURL
import platform.Foundation.NSURLIsExcludedFromBackupKey
import platform.Foundation.closeFile
import platform.Foundation.fileHandleForReadingAtPath
import platform.Foundation.readDataOfLength

/**
 * Downloads' storage on an Apple device: [folder] in Application
 * Support, kept out of the device's backups, the space free on its volume,
 * and SHA-256 by CommonCrypto, a megabyte at a time.
 */
@OptIn(ExperimentalForeignApi::class, BetaInteropApi::class)
internal class AppleDownloadDisk(override val folder: String) : DownloadDisk {
    init {
        NSFileManager.defaultManager.createDirectoryAtPath(folder, withIntermediateDirectories = true, attributes = null, error = null)
        NSURL.fileURLWithPath(folder, isDirectory = true).setResourceValue(true, forKey = NSURLIsExcludedFromBackupKey, error = null)
    }

    override fun freeBytes(): Long {
        val attributes = NSFileManager.defaultManager.attributesOfFileSystemForPath(folder, error = null)
        return (attributes?.get(NSFileSystemFreeSize) as? NSNumber)?.longLongValue ?: 0
    }

    override fun sha256(path: String): String = memScoped {
        val ctx = alloc<CC_SHA256_CTX>()
        CC_SHA256_Init(ctx.ptr)
        val file = NSFileHandle.fileHandleForReadingAtPath(path) ?: throw IllegalStateException("can't read $path")
        try {
            while (true) {
                val read = autoreleasepool {
                    val data = file.readDataOfLength(CHUNK.convert())
                    if (data.length.toLong() > 0) CC_SHA256_Update(ctx.ptr, data.bytes, data.length.convert())
                    data.length.toLong()
                }
                if (read == 0L) break
            }
        } finally {
            file.closeFile()
        }
        val digest = allocArray<UByteVar>(CC_SHA256_DIGEST_LENGTH)
        CC_SHA256_Final(digest, ctx.ptr)
        (0 until CC_SHA256_DIGEST_LENGTH).joinToString("") { digest[it].toString(16).padStart(2, '0') }
    }

    private companion object {
        const val CHUNK = 1 shl 20
    }
}
