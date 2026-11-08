package com.mmcdole.kivo.core.session

import app.cash.sqldelight.db.QueryResult
import app.cash.sqldelight.db.SqlDriver
import app.cash.sqldelight.db.SqlSchema
import com.mmcdole.kivo.core.downloads.DownloadTransport
import com.mmcdole.kivo.core.downloads.FileDownloadDisk
import com.mmcdole.kivo.core.downloads.db.DownloadDatabase

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
