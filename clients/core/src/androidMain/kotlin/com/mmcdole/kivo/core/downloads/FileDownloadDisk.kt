package com.mmcdole.kivo.core.downloads

import java.io.File
import java.security.MessageDigest

/**
 * Downloads' storage in a folder of the app's: the space usable on its
 * volume, and SHA-256 a megabyte at a time.
 */
internal class FileDownloadDisk(override val folder: String) : DownloadDisk {
    init {
        File(folder).mkdirs()
    }

    override fun freeBytes(): Long = File(folder).usableSpace

    override fun sha256(path: String): String {
        val digest = MessageDigest.getInstance("SHA-256")
        File(path).inputStream().use { input ->
            val buffer = ByteArray(CHUNK)
            while (true) {
                val read = input.read(buffer)
                if (read < 0) break
                digest.update(buffer, 0, read)
            }
        }
        return digest.digest().joinToString("") { (it.toInt() and 0xff).toString(16).padStart(2, '0') }
    }

    private companion object {
        const val CHUNK = 1 shl 20
    }
}
