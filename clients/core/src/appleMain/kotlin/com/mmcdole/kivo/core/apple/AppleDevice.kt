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
import com.mmcdole.kivo.core.settings.takeEarlierAppSettings
import okio.Path.Companion.toPath

/**
 * An iPhone or iPad as the core runs on it: the Keychain, VideoToolbox and
 * AVAudioSession, and downloads by the app's background `URLSession`.
 *
 * [directory] is the app's Application Support folder, which holds the
 * app's preferences, the download store and the downloads; [name] is the
 * device's name; [language] is the BCP 47 tag of the language the app
 * shows; [identity] is the app and device as every request says them, its
 * platform `iPhone` or `iPad`; [clientId], when given, is the client ID it
 * signs in with in place of this install's own, as UI tests launch the app
 * with; [subtitles] are the subtitles the app shows,
 * each with the deliveries it shows it in, and [trackSelection] whether its
 * player plays any audio track of a file, for the capability report;
 * [transport] is the app's background `URLSession`, which reports to
 * [com.mmcdole.kivo.core.session.KivoClient.transferred].
 */
class AppleDevice(
    directory: String,
    override val name: String,
    override val language: String,
    override val identity: ClientIdentity,
    clientId: String?,
    subtitles: List<SubtitleEntry>,
    trackSelection: Boolean,
    transport: DownloadTransport,
) : Device {
    override val preferences: DataStore<Preferences> = PreferenceDataStoreFactory.createWithPath(
        migrations = listOfNotNull(clientId?.let(::fixedClientId)),
        produceFile = { "$directory/kivo.preferences_pb".toPath() },
    )

    override val secureStore: SecureStore = KeychainSecureStore()

    override val capabilities: CapabilityProbe = AppleCapabilityProbe(subtitles, trackSelection)

    /**
     * The settings an earlier version kept in [preferences] for the app's
     * own screens, which the app keeps itself now: `theme`, `subtitleSize`,
     * `subtitleBackground` and `keepScreenOn`, those set, as text. They're
     * removed as they're taken, so the app takes them once.
     */
    @Throws(Exception::class)
    suspend fun takeAppSettings(): Map<String, String> = takeEarlierAppSettings(preferences)

    override val downloads = DownloadDevice(
        NativeSqliteDriver(DownloadDatabase.Schema, "downloads.db", onConfiguration = {
            it.copy(extendedConfig = DatabaseConfiguration.Extended(basePath = directory))
        }),
        AppleDownloadDisk("$directory/Downloads"),
        transport,
    )
}
