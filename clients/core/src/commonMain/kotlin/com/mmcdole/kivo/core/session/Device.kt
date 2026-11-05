package com.mmcdole.kivo.core.session

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import app.cash.sqldelight.db.SqlDriver
import com.mmcdole.kivo.core.auth.ClientIdentity
import com.mmcdole.kivo.core.auth.SecureStore
import com.mmcdole.kivo.core.caps.CapabilityProbe
import com.mmcdole.kivo.core.downloads.DownloadDisk
import com.mmcdole.kivo.core.downloads.DownloadTransport

/**
 * The device the app runs on, as the platform gives it to the core
 * ([KivoClient]): what its sessions are called, what the app says of itself
 * to servers, the language it shows, where it keeps its settings and session
 * tokens, what it plays, and, on a device that keeps downloads, where they
 * go and how they come.
 */
interface Device {
    /**
     * The device's name, such as "Living Room": the session name "Signed-in
     * devices" lists unless the person gives another at sign-in.
     */
    val name: String

    /** The app, its version and the device, as every request says them. */
    val identity: ClientIdentity

    /** What the app runs on, as its sessions say: the identity's platform. */
    val platform: String get() = identity.platform

    /** The app's preferences, which hold the saved servers, this device's settings and who each session is for. */
    val preferences: DataStore<Preferences>

    /** Where each server's session token is kept. */
    val secureStore: SecureStore

    /** What the device decodes, for the capability report its plays send. */
    val capabilities: CapabilityProbe


    /** Where downloads go and how they come; null on a device that doesn't keep them. */
    val downloads: DownloadDevice? get() = null
}

/**
 * A device's downloads, as its platform makes them: the download store, the
 * folder their files go in, and the background transfers that fetch them.
 */
class DownloadDevice internal constructor(
    internal val driver: SqlDriver,
    internal val disk: DownloadDisk,
    internal val transport: DownloadTransport,
)
