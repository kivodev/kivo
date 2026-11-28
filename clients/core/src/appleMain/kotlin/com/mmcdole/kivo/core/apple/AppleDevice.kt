package com.mmcdole.kivo.core.apple

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import androidx.datastore.preferences.core.Preferences
import app.cash.sqldelight.driver.native.NativeSqliteDriver
import co.touchlab.sqliter.DatabaseConfiguration
import com.mmcdole.kivo.api.models.SubtitleEntry
import com.mmcdole.kivo.core.auth.ClientIdentity
import com.mmcdole.kivo.core.auth.KeychainSecureStore
import com.mmcdole.kivo.core.auth.SecureStore
import com.mmcdole.kivo.core.auth.fixedClientId
import com.mmcdole.kivo.core.caps.AppleCapabilityProbe
import com.mmcdole.kivo.core.caps.CapabilityProbe
import com.mmcdole.kivo.core.downloads.AppleDownloadDisk
import com.mmcdole.kivo.core.downloads.DownloadTransport
import com.mmcdole.kivo.core.downloads.db.DownloadDatabase
import com.mmcdole.kivo.core.session.Device
import com.mmcdole.kivo.core.session.DownloadDevice
import okio.Path.Companion.toPath

/**
 * An iPhone, iPad or Apple TV as the core runs on it: the Keychain,
 * VideoToolbox and AVAudioSession, and on a device that keeps downloads,
 * downloads by the app's background `URLSession`.
 *
 * [directory] is the folder that holds the app's preferences and, with
 * downloads, the download store and the downloads; [name] is the device's
 * name; [identity] is the app and device as every request says them, its
 * platform `iPhone`, `iPad` or `Apple TV`; [clientId], when given, is the client ID it
 * signs in with in place of this install's own, as UI tests launch the app
 * with; [subtitles] are the subtitles the app shows,
 * each with the deliveries it shows it in, and [trackSelection] whether its
 * player plays any audio track of a file, for the capability report;
 * [transport] is the app's background `URLSession`, which reports to
 * [com.mmcdole.kivo.core.session.KivoClient.transferChanged], or null on a
 * device that keeps no downloads.
 */
class AppleDevice(
    directory: String,
    override val name: String,
    override val identity: ClientIdentity,
    clientId: String?,
    subtitles: List<SubtitleEntry>,
    trackSelection: Boolean,
    transport: DownloadTransport?,
) : Device {
    override val preferences: DataStore<Preferences> = PreferenceDataStoreFactory.createWithPath(
        migrations = listOfNotNull(clientId?.let(::fixedClientId)),
        produceFile = { "$directory/kivo.preferences_pb".toPath() },
    )

    override val secureStore: SecureStore = KeychainSecureStore()

    override val capabilities: CapabilityProbe = AppleCapabilityProbe(subtitles, trackSelection)

    override val downloads = transport?.let {
        DownloadDevice(
            NativeSqliteDriver(DownloadDatabase.Schema, "downloads.db", onConfiguration = { config ->
                config.copy(extendedConfig = DatabaseConfiguration.Extended(basePath = directory))
            }),
            AppleDownloadDisk("$directory/Downloads"),
            it,
        )
    }
}
