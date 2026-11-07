package com.mmcdole.kivo.android

import android.content.Context
import android.content.pm.PackageManager
import android.os.Build
import android.provider.Settings
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import androidx.datastore.preferences.core.Preferences
import com.mmcdole.kivo.android.player.AndroidCapabilityProbe
import com.mmcdole.kivo.core.auth.ClientIdentity
import com.mmcdole.kivo.core.auth.KeystoreSecureStore
import com.mmcdole.kivo.core.auth.SecureStore
import com.mmcdole.kivo.core.caps.CapabilityProbe
import com.mmcdole.kivo.core.session.Device
import java.io.File

/**
 * This TV as the core runs on it: the Keystore, MediaCodec and the HDMI
 * sink, and the app's preferences, which the app's own screens keep their
 * memory in too. A TV keeps no downloads.
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
}

/** This app on this device, as every request to a server says it, [appVersion] being the app's version name. */
class AndroidIdentity(context: Context, override val appVersion: String) : ClientIdentity {
    override val app = "kivo-android"
    override val platform = if (context.packageManager.hasSystemFeature(PackageManager.FEATURE_LEANBACK)) "androidTv" else "androidPhone"
    override val osVersion: String = Build.VERSION.RELEASE
    override val deviceModel: String = Build.MODEL
}

/** The system setting that holds the name the TV gives other devices over Bluetooth. */
private const val BLUETOOTH_NAME = "bluetooth_name"

/**
 * A TV's name for people: the one set in the system's settings ([setName]),
 * else the one it shows over Bluetooth ([bluetoothName]), else its model.
 */
internal fun deviceName(setName: String?, bluetoothName: String?, model: String): String =
    setName?.takeIf { it.isNotBlank() } ?: bluetoothName?.takeIf { it.isNotBlank() } ?: model
