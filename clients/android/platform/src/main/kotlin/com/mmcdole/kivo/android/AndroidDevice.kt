package com.mmcdole.kivo.android

import android.content.Context
import android.content.pm.PackageManager
import android.os.Build
import android.provider.Settings
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import androidx.datastore.preferences.core.Preferences
import app.cash.sqldelight.driver.android.AndroidSqliteDriver
import com.mmcdole.kivo.android.downloads.WorkTransport
import com.mmcdole.kivo.android.player.AndroidCapabilityProbe
import com.mmcdole.kivo.core.auth.ClientIdentity
import com.mmcdole.kivo.core.auth.KeystoreSecureStore
import com.mmcdole.kivo.core.auth.SecureStore
import com.mmcdole.kivo.core.caps.CapabilityProbe
import com.mmcdole.kivo.core.downloads.androidDownloads
import com.mmcdole.kivo.core.session.Device
import com.mmcdole.kivo.core.session.DownloadDevice
import java.io.File

/**
 * This TV or phone as the core runs on it: the Keystore, MediaCodec and the
 * HDMI sink, and the app's preferences, which the app's own screens keep
 * their memory in too. A phone or tablet keeps downloads, their store in the
 * app's databases and their files in its own storage, coming by
 * [WorkTransport]; a TV keeps none.
 */
class AndroidDevice(context: Context, appVersion: String) : Device {
    /** This TV's name, such as "Living Room". */
    override val name: String = deviceName(
        Settings.Global.getString(context.contentResolver, Settings.Global.DEVICE_NAME),
        // Readable up to Android 12; later versions refuse it.
        runCatching { Settings.Secure.getString(context.contentResolver, BLUETOOTH_NAME) }.getOrNull(),
        Build.MODEL,
    )

    override val identity: ClientIdentity = AndroidIdentity(context, appVersion)

    /** The language the app shows: the one its strings resolved to. */

    override val preferences: DataStore<Preferences> =
        PreferenceDataStoreFactory.create { File(context.filesDir, "datastore/kivo.preferences_pb") }

    override val secureStore: SecureStore = KeystoreSecureStore(context)

    override val capabilities: CapabilityProbe = AndroidCapabilityProbe(context)

    /** A phone's download transfers; null on a TV. */
    val transport: WorkTransport? =
        if (identity.platform == AndroidIdentity.PHONE) WorkTransport(context, File(context.filesDir, DOWNLOADS)) else null

    override val downloads: DownloadDevice? = transport?.let { transport ->
        androidDownloads(File(context.filesDir, DOWNLOADS).path, transport) { schema -> AndroidSqliteDriver(schema, context, "downloads.db") }
    }

    private companion object {
        /** The folder in the app's own storage downloads are kept in, which nothing backs up. */
        const val DOWNLOADS = "downloads"
    }
}

/** This app on this device, as every request to a server says it, [appVersion] being the app's version name. */
class AndroidIdentity(context: Context, override val appVersion: String) : ClientIdentity {
    override val app = "kivo-android"
    override val platform = if (context.packageManager.hasSystemFeature(PackageManager.FEATURE_LEANBACK)) "androidTv" else PHONE
    override val osVersion: String = Build.VERSION.RELEASE
    override val deviceModel: String = Build.MODEL

    companion object {
        const val PHONE = "androidPhone"
    }
}

/** The system setting that holds the name the TV gives other devices over Bluetooth. */
private const val BLUETOOTH_NAME = "bluetooth_name"

/**
 * A TV's name for people: the one set in the system's settings ([setName]),
 * else the one it shows over Bluetooth ([bluetoothName]), else its model.
 */
internal fun deviceName(setName: String?, bluetoothName: String?, model: String): String =
    setName?.takeIf { it.isNotBlank() } ?: bluetoothName?.takeIf { it.isNotBlank() } ?: model
