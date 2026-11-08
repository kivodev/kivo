package com.mmcdole.kivo.core.downloads

import app.cash.sqldelight.db.QueryResult
import app.cash.sqldelight.db.SqlDriver
import app.cash.sqldelight.db.SqlSchema
import com.mmcdole.kivo.core.downloads.db.DownloadDatabase
import com.mmcdole.kivo.core.session.DownloadDevice
import java.io.File
import java.security.MessageDigest

/**
 * Downloads on an Android phone or tablet: the download store on the
 * driver [driver] makes for its schema (the app's `AndroidSqliteDriver`, or
 * a test's), the files in [folder], a folder of the app's own that only
 * downloads use, and [transport]'s transfers.
 */
fun androidDownloads(
    folder: String,
    transport: DownloadTransport,
    driver: (SqlSchema<QueryResult.Value<Unit>>) -> SqlDriver,
): DownloadDevice = DownloadDevice(driver(DownloadDatabase.Schema), FileDownloadDisk(folder), transport)

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
